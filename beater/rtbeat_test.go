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
// eventPublisher interface in place of a real libbeat client.
type fakePublisher struct {
	got chan []beat.Event
}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{got: make(chan []beat.Event, 1)}
}

func (f *fakePublisher) PublishAll(events []beat.Event) { f.got <- events }

func TestBuildEvents(t *testing.T) {
	msg := &rtq.MessageBatch{
		Uuid: "batch-1",
		Size: 2,
		Messages: []rtq.Message{
			{Seq: "1", Producer: "p", Payload: map[string]interface{}{"a": 1.0}},
			{Seq: "2", Producer: "p", Payload: map[string]interface{}{"b": 2.0}},
		},
	}

	events := buildEvents("rtbeat", "10.0.0.1", msg)

	// One leading zero-value placeholder event (preserved legacy behavior),
	// then one event per message.
	if got, want := len(events), 1+len(msg.Messages); got != want {
		t.Fatalf("len(events) = %d, want %d", got, want)
	}
	if events[0].Fields != nil {
		t.Errorf("events[0] should be the zero-value placeholder, got Fields=%v", events[0].Fields)
	}

	for i, in := range msg.Messages {
		ev := events[i+1]
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
		if ev.Private != i {
			t.Errorf("event %d Private = %v, want %d", i, ev.Private, i)
		}
		if ev.Timestamp.IsZero() {
			t.Errorf("event %d Timestamp is zero", i)
		}
	}
}

func TestBuildEventsEmptyBatch(t *testing.T) {
	events := buildEvents("rtbeat", "1.2.3.4", &rtq.MessageBatch{})
	if len(events) != 1 {
		t.Fatalf("empty batch: len(events) = %d, want 1 (placeholder only)", len(events))
	}
}

func newTestRouter(pub eventPublisher, onBatch func(), onMessages func(int)) *gin.Engine {
	r := gin.New()
	r.POST("/in", inHandler("rtbeat", zap.NewNop(), pub, onBatch, onMessages))
	return r
}

func TestInHandlerValidBatch(t *testing.T) {
	pub := newFakePublisher()
	var batches, messages int
	r := newTestRouter(pub, func() { batches++ }, func(n int) { messages += n })

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
		if len(events) != 2 { // placeholder + one message
			t.Fatalf("published %d events, want 2", len(events))
		}
		// clientIp is read synchronously from the request; assert it
		// propagates through the handler into the published event.
		if got := events[1].Fields["clientIp"]; got != "203.0.113.7" {
			t.Errorf("published clientIp = %v, want 203.0.113.7", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PublishAll was not called")
	}
}

func TestInHandlerBadJSON(t *testing.T) {
	pub := newFakePublisher()
	var batches, messages int
	r := newTestRouter(pub, func() { batches++ }, func(n int) { messages += n })

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

	// On the error path the handler returns before spawning the publish
	// goroutine, so nothing can ever be sent — assert deterministically.
	if len(pub.got) != 0 {
		t.Fatal("PublishAll must not be called on bad JSON")
	}
}
