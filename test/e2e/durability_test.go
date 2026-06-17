//go:build e2e

// Package e2e black-box tests the rtbeat binary's durability guarantees against
// a real libbeat logstash output talking to a controllable lumberjack server.
//
// Run with: go test -tags e2e ./test/e2e/...
//
// It proves, end to end (real binary, real TCP, real lumberjack ack protocol,
// real SIGTERM path), that:
//   - POST /in returns 200 only AFTER the output acknowledges the batch (and
//     not before — see TestE2EWaitsForAckBeforeOK, which delays the ack);
//   - POST /in returns 504 when the output never acks (so the sender retries);
//   - a single /in batch is acked correctly even when the output fragments the
//     acks across the wire (TestE2EMultiMessageFragmentedAck);
//   - an in-flight request is still delivered when SIGTERM arrives mid-flight
//     (graceful drain), and the process exits cleanly.
//
// Out of scope (NOT proven here, by design): durability across a hard crash
// (SIGKILL/panic/power loss) and across process restarts. rtbeat holds
// in-flight events in libbeat's in-memory queue; crash-safety relies on the
// sender retrying on a non-2xx response and, optionally, libbeat's disk
// queue/spool — neither is exercised by this suite.
package e2e

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/elastic/go-lumber/lj"
	"github.com/elastic/go-lumber/server"
)

const validBatch = `{"uuid":"e2e","size":1,"messages":[{"seq":"1","producer":"test","payload":{"k":"v"}}]}`

var rtbeatBin string

func TestMain(m *testing.M) {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: locate repo root:", err)
		os.Exit(1)
	}

	bin := filepath.Join(os.TempDir(), fmt.Sprintf("rtbeat-e2e-%d", os.Getpid()))
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build rtbeat: %v\n%s\n", err, out)
		os.Exit(1)
	}
	rtbeatBin = bin

	code := m.Run()
	_ = os.Remove(bin)
	os.Exit(code)
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found above test directory")
		}
		dir = parent
	}
}

// ackPolicy controls how the lumberjack server acknowledges received batches.
type ackPolicy int

const (
	ackImmediate ackPolicy = iota // ack as soon as received -> 200
	ackNever                      // never ack -> handler times out -> 504
	ackDelayed                    // ack after a delay -> exercises ordering / drain
)

// lumberServer is a controllable lumberjack (logstash) endpoint.
type lumberServer struct {
	srv    server.Server
	addr   string
	mu     sync.Mutex
	events int
}

func startLumber(t *testing.T, policy ackPolicy, delay time.Duration) *lumberServer {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("lumber listen: %v", err)
	}
	s, err := server.NewWithListener(l, server.V2(true))
	if err != nil {
		_ = l.Close()
		t.Fatalf("lumber server: %v", err)
	}
	ls := &lumberServer{srv: s, addr: l.Addr().String()}

	go func() {
		for batch := range s.ReceiveChan() {
			ls.mu.Lock()
			ls.events += len(batch.Events)
			ls.mu.Unlock()

			switch policy {
			case ackImmediate:
				batch.ACK()
			case ackDelayed:
				go func(b *lj.Batch) {
					time.Sleep(delay)
					b.ACK()
				}(batch)
			case ackNever:
				// hold the ack to force the rtbeat handler to time out
			}
		}
	}()

	return ls
}

func (ls *lumberServer) received() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.events
}

func (ls *lumberServer) close() { _ = ls.srv.Close() }

// rtbeatProc is a running rtbeat binary under test. A single goroutine owns
// cmd.Wait (its result is delivered on waitErr) so signalling and killing never
// race on a second Wait.
type rtbeatProc struct {
	cmd      *exec.Cmd
	httpPort int
	stderr   *syncBuffer
	waitErr  chan error
}

func startRtbeat(t *testing.T, lsAddr string, timeoutSec int) *rtbeatProc {
	return startRtbeatCfg(t, lsAddr, timeoutSec, 0)
}

func startRtbeatCfg(t *testing.T, lsAddr string, timeoutSec, bulkMaxSize int) *rtbeatProc {
	t.Helper()
	httpPort := freePort(t)

	bulk := ""
	if bulkMaxSize > 0 {
		bulk = fmt.Sprintf("\n  bulk_max_size: %d", bulkMaxSize)
	}
	cfg := fmt.Sprintf(`rtbeat:
  port: "%d"
  timeout: %d
  shutdown_timeout: 10
output.logstash:
  hosts: ["%s"]%s
logging.level: warning
`, httpPort, timeoutSec, lsAddr, bulk)

	cfgFile := filepath.Join(t.TempDir(), "rtbeat.yml")
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(rtbeatBin, "-c", cfgFile, "-e")
	buf := &syncBuffer{}
	cmd.Stdout = buf
	cmd.Stderr = buf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rtbeat: %v", err)
	}

	rp := &rtbeatProc{cmd: cmd, httpPort: httpPort, stderr: buf, waitErr: make(chan error, 1)}
	go func() { rp.waitErr <- rp.cmd.Wait() }()

	if err := waitForHTTP(fmt.Sprintf("http://127.0.0.1:%d/metrics", httpPort), 10*time.Second); err != nil {
		rp.kill()
		t.Fatalf("rtbeat did not become ready: %v\n--- rtbeat output ---\n%s", err, buf.String())
	}
	return rp
}

func (rp *rtbeatProc) postIn(t *testing.T, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/in", rp.httpPort),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /in: %v\n--- rtbeat output ---\n%s", err, rp.stderr.String())
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// sigterm sends SIGTERM and returns the channel delivering the process exit
// error (owned by the single Wait goroutine started in startRtbeatCfg).
func (rp *rtbeatProc) sigterm() <-chan error {
	_ = rp.cmd.Process.Signal(syscall.SIGTERM)
	return rp.waitErr
}

// kill force-terminates and reaps via the single Wait goroutine.
func (rp *rtbeatProc) kill() {
	_ = rp.cmd.Process.Signal(syscall.SIGKILL)
	select {
	case <-rp.waitErr:
	case <-time.After(5 * time.Second):
	}
}

func TestE2EAckAfterDelivery(t *testing.T) {
	ls := startLumber(t, ackImmediate, 0)
	defer ls.close()
	rp := startRtbeat(t, ls.addr, 5)
	defer rp.kill()

	code, body := rp.postIn(t, validBatch)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s\n--- rtbeat ---\n%s", code, body, rp.stderr.String())
	}
	if !waitFor(func() bool { return ls.received() >= 1 }, 5*time.Second) {
		t.Fatalf("output did not receive the event (got %d)", ls.received())
	}
}

// TestE2EWaitsForAckBeforeOK positively proves the 200 is gated on the ack:
// the output delays its ack, and the response must not arrive before then.
func TestE2EWaitsForAckBeforeOK(t *testing.T) {
	const delay = 800 * time.Millisecond
	ls := startLumber(t, ackDelayed, delay)
	defer ls.close()
	rp := startRtbeat(t, ls.addr, 5)
	defer rp.kill()

	start := time.Now()
	code, body := rp.postIn(t, validBatch)
	elapsed := time.Since(start)

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s\n--- rtbeat ---\n%s", code, body, rp.stderr.String())
	}
	if elapsed < delay {
		t.Errorf("200 returned after %s, before the %s ack delay — handler acked on receipt, not delivery", elapsed, delay)
	}
}

func TestE2EStallReturns504(t *testing.T) {
	ls := startLumber(t, ackNever, 0)
	defer ls.close()
	rp := startRtbeat(t, ls.addr, 2) // short ack budget to keep the test quick
	defer rp.kill()

	start := time.Now()
	code, body := rp.postIn(t, validBatch)
	elapsed := time.Since(start)

	if code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504; body=%s\n--- rtbeat ---\n%s", code, body, rp.stderr.String())
	}
	// It must have actually waited for the (never-arriving) ack, not failed
	// fast (a connection error or parse error would return sub-second).
	if elapsed < time.Second {
		t.Errorf("returned 504 after %s, expected to wait ~ack timeout (2s)", elapsed)
	}
	// The event did reach the output; it is in-flight and unacked, so the
	// sender (which got a 504) retries rather than dropping it.
	if !waitFor(func() bool { return ls.received() >= 1 }, 5*time.Second) {
		t.Fatalf("output did not receive the in-flight event")
	}
}

// TestE2EMultiMessageFragmentedAck sends a multi-message batch with
// bulk_max_size=1, so the output ships each event as its own lumberjack window
// and acks them separately. The single /in batch must wait for ALL fragment
// acks before returning 200 — exercising the per-batch ack correlation under
// fragmented, multi-callback acks over the wire.
func TestE2EMultiMessageFragmentedAck(t *testing.T) {
	const n = 20
	ls := startLumber(t, ackImmediate, 0)
	defer ls.close()
	rp := startRtbeatCfg(t, ls.addr, 5, 1)
	defer rp.kill()

	code, body := rp.postIn(t, makeBatch(n))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s\n--- rtbeat ---\n%s", code, body, rp.stderr.String())
	}
	if !waitFor(func() bool { return ls.received() >= n }, 5*time.Second) {
		t.Fatalf("output received %d events, want >= %d", ls.received(), n)
	}
}

// TestE2EDrainsInFlightOnShutdown proves a request that is in-flight when
// SIGTERM arrives is still delivered (graceful drain), not dropped.
func TestE2EDrainsInFlightOnShutdown(t *testing.T) {
	// The output acks ~1s after receiving, so the request is still in-flight
	// once we confirm the event reached the output and then signal shutdown.
	ls := startLumber(t, ackDelayed, time.Second)
	defer ls.close()
	rp := startRtbeat(t, ls.addr, 5)

	codeCh := make(chan int, 1)
	go func() {
		code, _ := rp.postIn(t, validBatch)
		codeCh <- code
	}()

	// Gate on the event actually reaching the output (more robust than a bare
	// sleep): at this point its ack is still pending, so SIGTERM lands mid-flight.
	if !waitFor(func() bool { return ls.received() >= 1 }, 5*time.Second) {
		rp.kill()
		t.Fatal("event never reached the output before shutdown")
	}
	exit := rp.sigterm()

	select {
	case code := <-codeCh:
		if code != http.StatusOK {
			t.Fatalf("in-flight request during shutdown returned %d, want 200 (drained)\n--- rtbeat ---\n%s", code, rp.stderr.String())
		}
	case <-time.After(8 * time.Second):
		rp.kill()
		t.Fatal("in-flight request did not complete during graceful shutdown")
	}

	select {
	case err := <-exit:
		if err != nil {
			t.Errorf("rtbeat exited with error after SIGTERM: %v\n--- rtbeat ---\n%s", err, rp.stderr.String())
		}
	case <-time.After(12 * time.Second):
		rp.kill()
		t.Fatal("rtbeat did not exit after SIGTERM within the shutdown budget")
	}
}

// --- small helpers ---

func makeBatch(n int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `{"uuid":"e2e","size":%d,"messages":[`, n)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"seq":"%d","producer":"test","payload":{"i":%d}}`, i, i)
	}
	sb.WriteString(`]}`)
	return sb.String()
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForHTTP(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("%s not ready within %s", url, timeout)
}

func waitFor(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// syncBuffer is a goroutine-safe buffer for capturing subprocess output.
type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
