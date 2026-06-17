package beater

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/prometheus/client_golang/prometheus"
)

// fakeClient implements beat.Client, recording published events and signaling
// when it is closed.
type fakeClient struct {
	published chan []beat.Event
	closed    chan struct{}
}

func newFakeClient() *fakeClient {
	return &fakeClient{published: make(chan []beat.Event, 8), closed: make(chan struct{})}
}

func (c *fakeClient) Publish(e beat.Event)       { c.published <- []beat.Event{e} }
func (c *fakeClient) PublishAll(es []beat.Event) { c.published <- es }
func (c *fakeClient) Close() error               { close(c.closed); return nil }

// fakePipeline implements beat.Pipeline and captures the ClientConfig so the
// test can drive the ACK handler.
type fakePipeline struct {
	client *fakeClient
	cfg    beat.ClientConfig
}

func (p *fakePipeline) ConnectWith(cc beat.ClientConfig) (beat.Client, error) {
	p.cfg = cc
	return p.client, nil
}

func (p *fakePipeline) Connect() (beat.Client, error) { return p.client, nil }

// freePort reserves and releases an ephemeral port, returning it for the beat
// to bind. There is a small reuse window, which is acceptable for a test.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = l.Close() }()
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("split port: %v", err)
	}
	return port
}

func waitForServer(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:gosec // fixed localhost test URL
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not start within timeout")
}

// TestRunLifecycle drives the full beat through a fake libbeat pipeline:
// New -> Run -> serve /in and /metrics -> ACK -> Stop.
func TestRunLifecycle(t *testing.T) {
	port := freePort(t)

	cfg, err := common.NewConfigFrom(map[string]interface{}{"port": port})
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	bter, err := New(nil, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	bt, ok := bter.(*Rtbeat)
	if !ok {
		t.Fatalf("New returned %T, want *Rtbeat", bter)
	}
	// Inject a private registry so repeated runs don't collide on the global
	// prometheus registry's duplicate-registration guard.
	reg := prometheus.NewRegistry()
	bt.registerer = reg
	bt.gatherer = reg

	client := newFakeClient()
	pipe := &fakePipeline{client: client}
	b := &beat.Beat{
		Info:      beat.Info{Name: "rtbeat"},
		Publisher: pipe,
	}

	runErr := make(chan error, 1)
	go func() { runErr <- bt.Run(b) }()

	base := "http://127.0.0.1:" + port
	waitForServer(t, base+"/metrics")

	// POST a batch and confirm it is published through the pipeline.
	body := `{"uuid":"b1","size":1,"messages":[{"seq":"1","payload":{"k":"v"}}]}`
	resp, err := http.Post(base+"/in", "application/json", strings.NewReader(body)) //nolint:gosec // fixed localhost test URL
	if err != nil {
		t.Fatalf("POST /in: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /in status = %d, want 200", resp.StatusCode)
	}

	select {
	case events := <-client.published:
		if len(events) != 2 { // placeholder + one message
			t.Errorf("published %d events, want 2", len(events))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline did not receive published events")
	}

	// /metrics serves prometheus output.
	mResp, err := http.Get(base + "/metrics") //nolint:gosec // fixed localhost test URL
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	_, _ = io.Copy(io.Discard, mResp.Body)
	_ = mResp.Body.Close()
	if mResp.StatusCode != http.StatusOK {
		t.Errorf("GET /metrics status = %d, want 200", mResp.StatusCode)
	}

	// Exercise the ACK callback wired up in Run.
	if pipe.cfg.ACKHandler == nil {
		t.Fatal("Run did not register an ACK handler")
	}
	pipe.cfg.ACKHandler.ACKEvents(3)

	// Stop closes the client and unblocks Run.
	bt.Stop()

	select {
	case <-client.closed:
	case <-time.After(2 * time.Second):
		t.Error("Stop did not close the client")
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
}
