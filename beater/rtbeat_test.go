package beater

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/gin-gonic/gin"
	"github.com/txn2/rxtx/rtq"
	"go.uber.org/zap"
)

func init() { gin.SetMode(gin.TestMode) }

// fakePublisher records the events handed to PublishAll, satisfying the
// eventPublisher interface in place of a real libbeat client. When autoAck is
// set it simulates the output delivering the batch by acking the batchAck
// carried in the events' Private field.
type fakePublisher struct {
	got     chan []beat.Event
	autoAck bool
}

func newFakePublisher(autoAck bool) *fakePublisher {
	return &fakePublisher{got: make(chan []beat.Event, 1), autoAck: autoAck}
}

func (f *fakePublisher) PublishAll(events []beat.Event) {
	f.got <- events
	if f.autoAck && len(events) > 0 {
		if a, ok := events[0].Private.(*batchAck); ok && a != nil {
			a.ack(len(events))
		}
	}
}

func TestBuildEvents(t *testing.T) {
	msg := &rtq.MessageBatch{
		Uuid: "batch-1",
		Size: 2,
		Messages: []rtq.Message{
			{Seq: "1", Producer: "p", Payload: map[string]interface{}{"a": 1.0}},
			{Seq: "2", Producer: "p", Payload: map[string]interface{}{"b": 2.0}},
		},
	}
	ack := newBatchAck(len(msg.Messages))

	events := buildEvents("rtbeat", "10.0.0.1", msg, ack)

	// One event per message — no placeholder.
	if got, want := len(events), len(msg.Messages); got != want {
		t.Fatalf("len(events) = %d, want %d", got, want)
	}

	for i, in := range msg.Messages {
		ev := events[i]
		if ev.Fields["type"] != "rtbeat" {
			t.Errorf("event %d type = %v, want rtbeat", i, ev.Fields["type"])
		}
		if ev.Fields["clientIp"] != "10.0.0.1" {
			t.Errorf("event %d clientIp = %v, want 10.0.0.1", i, ev.Fields["clientIp"])
		}
		got, ok := ev.Fields["rxtxMsg"].(rtq.Message)
		if !ok {
			t.Fatalf("event %d rxtxMsg is %T, want rtq.Message", i, ev.Fields["rxtxMsg"])
		}
		if got.Seq != in.Seq {
			t.Errorf("event %d rxtxMsg.Seq = %q, want %q", i, got.Seq, in.Seq)
		}
		// Every event carries the shared batch ack token in Private.
		if ev.Private != ack {
			t.Errorf("event %d Private = %v, want the batchAck", i, ev.Private)
		}
		if ev.Timestamp.IsZero() {
			t.Errorf("event %d Timestamp is zero", i)
		}
	}
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestBatchAckPartial(t *testing.T) {
	a := newBatchAck(3)

	a.ack(2)
	if isClosed(a.done) {
		t.Fatal("done closed after 2 of 3 acks")
	}

	a.ack(1)
	if !isClosed(a.done) {
		t.Fatal("done not closed after all 3 acks")
	}

	// Extra acks must not panic or re-close.
	a.ack(1)
}

func TestResolveBatchAcks(t *testing.T) {
	// Two batches whose events are interleaved in a single ack callback,
	// plus an unrelated/nil entry that must be ignored.
	a := newBatchAck(2)
	b := newBatchAck(1)

	resolveBatchAcks([]interface{}{a, b, a, nil, 42})

	if !isClosed(a.done) {
		t.Error("batch a (2 events, both acked) should be done")
	}
	if !isClosed(b.done) {
		t.Error("batch b (1 event, acked) should be done")
	}
}

func TestResolveBatchAcksSplitAcrossCallbacks(t *testing.T) {
	// A 3-event batch acked as 2 then 1 across two reporter callbacks.
	a := newBatchAck(3)

	resolveBatchAcks([]interface{}{a, a})
	if isClosed(a.done) {
		t.Fatal("batch closed after only 2 of 3 acks")
	}

	resolveBatchAcks([]interface{}{a})
	if !isClosed(a.done) {
		t.Fatal("batch not closed after the remaining ack")
	}
}

func TestBuildEventsEmptyBatch(t *testing.T) {
	events := buildEvents("rtbeat", "1.2.3.4", &rtq.MessageBatch{}, newBatchAck(0))
	if len(events) != 0 {
		t.Fatalf("empty batch: len(events) = %d, want 0", len(events))
	}
}

func newTestRouter(pub eventPublisher, ackTimeout time.Duration, onBatch func(), onMessages func(int)) *gin.Engine {
	r := gin.New()
	r.POST("/in", inHandler("rtbeat", zap.NewNop(), pub, ackTimeout, onBatch, onMessages))
	return r
}

func TestInHandlerDeliveredBatch(t *testing.T) {
	pub := newFakePublisher(true) // simulate the output acking the batch
	var batches, messages int
	r := newTestRouter(pub, 2*time.Second, func() { batches++ }, func(n int) { messages += n })

	body := `{"uuid":"b1","size":1,"messages":[{"seq":"1","payload":{"hello":"world"}}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/in", bytes.NewBufferString(body))
	req.RemoteAddr = "203.0.113.7:54321"
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if batches != 1 {
		t.Errorf("onBatch called %d times, want 1", batches)
	}
	if messages != 1 {
		t.Errorf("onMessages total = %d, want 1", messages)
	}

	select {
	case events := <-pub.got:
		if len(events) != 1 { // one message, no placeholder
			t.Fatalf("published %d events, want 1", len(events))
		}
		if got := events[0].Fields["clientIp"]; got != "203.0.113.7" {
			t.Errorf("published clientIp = %v, want 203.0.113.7", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PublishAll was not called")
	}
}

func TestInHandlerAckTimeout(t *testing.T) {
	pub := newFakePublisher(false) // output never acks
	r := newTestRouter(pub, 50*time.Millisecond, func() {}, func(int) {})

	body := `{"uuid":"b1","size":1,"messages":[{"seq":"1","payload":{"hello":"world"}}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/in", bytes.NewBufferString(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504; body=%s", w.Code, w.Body.String())
	}
	// The batch was still published (it may yet be delivered; the sender
	// retries on the 504).
	select {
	case <-pub.got:
	case <-time.After(time.Second):
		t.Fatal("PublishAll was not called")
	}
}

func TestInHandlerBadJSON(t *testing.T) {
	pub := newFakePublisher(true)
	var batches, messages int
	r := newTestRouter(pub, 2*time.Second, func() { batches++ }, func(n int) { messages += n })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/in", bytes.NewBufferString(`{not valid json`))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	if batches != 1 {
		t.Errorf("onBatch called %d times, want 1", batches)
	}
	if messages != 0 {
		t.Errorf("onMessages total = %d, want 0 on parse failure", messages)
	}

	// On the error path the handler returns before publishing.
	if len(pub.got) != 0 {
		t.Fatal("PublishAll must not be called on bad JSON")
	}
}

func TestInHandlerEmptyBatch(t *testing.T) {
	pub := newFakePublisher(true)
	var messages int
	r := newTestRouter(pub, 2*time.Second, func() {}, func(n int) { messages += n })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/in", bytes.NewBufferString(`{"uuid":"b","size":0,"messages":[]}`))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for empty batch", w.Code)
	}
	if messages != 0 {
		t.Errorf("onMessages total = %d, want 0", messages)
	}
	// Nothing to publish for an empty batch.
	if len(pub.got) != 0 {
		t.Fatal("PublishAll must not be called for an empty batch")
	}
}
