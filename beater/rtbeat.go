package beater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/common/acker"
	"github.com/gin-gonic/gin"
	"github.com/txn2/rtbeat/config"
	"github.com/txn2/rxtx/rtq"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Rtbeat struct {
	done   chan struct{}
	config config.Config
	client beat.Client
	logger *zap.Logger

	// Prometheus registry seam. Both nil in production, which resolves to the
	// global default registerer/gatherer (preserving the original behavior).
	// Tests inject a private registry so repeated Run calls don't collide on
	// the global registry's duplicate-registration guard.
	registerer prometheus.Registerer
	gatherer   prometheus.Gatherer
}

// New Creates beater
func New(_ *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	c := config.DefaultConfig
	if err := cfg.Unpack(&c); err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	// Guard the durability footguns: a non-positive ack timeout would 504 every
	// request, and a non-positive shutdown timeout would silently disable the
	// drain. Fall back to the defaults.
	if c.Timeout <= 0 {
		c.Timeout = config.DefaultConfig.Timeout
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = config.DefaultConfig.ShutdownTimeout
	}

	zapCfg := zap.NewProductionConfig()
	zapCfg.DisableCaller = true
	zapCfg.DisableStacktrace = true

	logger, err := zapCfg.Build()
	if err != nil {
		fmt.Printf("Can not build logger: %s\n", err.Error())
		return nil, err
	}

	bt := &Rtbeat{
		done:   make(chan struct{}),
		config: c,
		logger: logger,
	}
	return bt, nil
}

// Run the beat
func (bt *Rtbeat) Run(b *beat.Beat) error {

	registerer := bt.registerer
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	gatherer := bt.gatherer
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}
	metrics := promauto.With(registerer)

	batches := metrics.NewCounter(prometheus.CounterOpts{
		Name: "rtbeat_batches_received",
		Help: "Total number batches received",
	})

	messages := metrics.NewCounter(prometheus.CounterOpts{
		Name: "rtbeat_messages_parsed",
		Help: "Total number messages parsed.",
	})

	currentAcks := metrics.NewGauge(prometheus.GaugeOpts{
		Name: "rtbeat_current_acks",
		Help: "Current acks.",
	})

	totalAcks := metrics.NewCounter(prometheus.CounterOpts{
		Name: "rtbeat_acks_received",
		Help: "Total number of acks.",
	})

	var err error
	bt.client, err = b.Publisher.ConnectWith(beat.ClientConfig{
		// GuaranteedSend retries events until the output acknowledges them;
		// WaitClose makes Close() (called from Stop) block until in-flight
		// events are acked or the shutdown budget elapses, so a graceful
		// shutdown drains rather than dropping in-flight events.
		PublishMode: beat.GuaranteedSend,
		WaitClose:   time.Duration(bt.config.ShutdownTimeout) * time.Second,
		ACKHandler: acker.Combine(
			// Existing ack metrics.
			acker.RawCounting(func(i int) {
				bt.logger.Info("Run", zapcore.Field{
					Key:     "ACKCount",
					Type:    zapcore.Int32Type,
					Integer: int64(i),
				})
				currentAcks.Set(float64(i))
				totalAcks.Add(float64(i))
			}),
			// Per-batch ack correlation: each event carries its *batchAck in
			// Private; when all of a batch's events are acked, its waiter is
			// released so POST /in can return 200 (delivered).
			acker.EventPrivateReporter(func(_ int, data []interface{}) {
				resolveBatchAcks(data)
			}),
		),
	})
	if err != nil {
		return err
	}

	// gin config
	gin.SetMode(gin.ReleaseMode)
	gin.DisableConsoleColor()

	// discard default logger
	gin.DefaultWriter = io.Discard

	// get a router
	r := gin.Default()

	ackTimeout := time.Duration(bt.config.Timeout) * time.Second
	r.POST("/in", inHandler(b.Info.Name, bt.logger, bt.client, ackTimeout, batches.Inc, func(n int) {
		messages.Add(float64(n))
	}))

	// Prometheus Metrics. InstrumentMetricHandler mirrors promhttp.Handler()
	// exactly (it adds promhttp_metric_handler_requests_total / _in_flight),
	// but bound to the resolved registry rather than the global default.
	r.GET("/metrics", gin.WrapH(promhttp.InstrumentMetricHandler(
		registerer, promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}),
	)))

	srv := &http.Server{
		Addr:    ":" + bt.config.Port,
		Handler: r,
	}

	go func() {
		// service connections
		bt.logger.Info("Run",
			zapcore.Field{
				Key:    "State",
				Type:   zapcore.StringType,
				String: "Waiting for rxtx POST data to: " + bt.config.Port + ":/in",
			},
		)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// block waiting for done
	<-bt.done
	bt.logger.Info("Run",
		zapcore.Field{
			Key:    "Status",
			Type:   zapcore.StringType,
			String: "Shutting down web server.",
		},
	)

	// Graceful shutdown ordering matters for durability: stop accepting new
	// requests first (Shutdown waits for in-flight handlers to finish waiting
	// on their acks), THEN close the client. Closing was configured with
	// WaitClose + GuaranteedSend, so Close drains outstanding events until
	// acked or the shutdown budget elapses. Closing before intake stops would
	// let a late batch publish into a closing pipeline and be lost.
	shutdownTimeout := time.Duration(bt.config.ShutdownTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	_ = srv.Shutdown(ctx)
	cancel()
	_ = bt.client.Close()
	return nil
}

// Stop signals the beat to shut down. The actual drain (stop intake, finish
// in-flight handlers, then drain and close the publisher) happens in Run's
// shutdown sequence to guarantee the ordering.
func (bt *Rtbeat) Stop() {
	bt.logger.Info("Stop", zap.String("state", "shutting down"))
	close(bt.done)
}

// eventPublisher is the subset of beat.Client the /in handler needs. It lets
// tests substitute a fake for the libbeat publisher pipeline.
type eventPublisher interface {
	PublishAll([]beat.Event)
}

// batchAck tracks the outstanding acknowledgements for a single published
// batch. done is closed once every event in the batch has been acked by the
// output, releasing the HTTP handler waiting on delivery.
type batchAck struct {
	remaining int64
	done      chan struct{}
	once      sync.Once
}

func newBatchAck(n int) *batchAck {
	return &batchAck{remaining: int64(n), done: make(chan struct{})}
}

// ack records n acknowledged events and releases waiters once the batch is
// fully delivered.
func (a *batchAck) ack(n int) {
	if atomic.AddInt64(&a.remaining, -int64(n)) <= 0 {
		a.once.Do(func() { close(a.done) })
	}
}

// resolveBatchAcks groups a slice of acked events' Private values by their
// owning batch and releases each batch that is now fully delivered. The output
// may ack events from several batches in a single callback, and may split a
// batch's acks across callbacks; grouping per *batchAck handles both. Non
// *batchAck / nil entries are ignored.
func resolveBatchAcks(data []interface{}) {
	counts := make(map[*batchAck]int, len(data))
	for _, d := range data {
		if a, ok := d.(*batchAck); ok && a != nil {
			counts[a]++
		}
	}
	for a, n := range counts {
		a.ack(n)
	}
}

// buildEvents converts an rxtx MessageBatch into the beat.Event slice rtbeat
// publishes — one event per message carrying the original message under
// "rxtxMsg" alongside "type" and "clientIp". Each event's Private holds the
// shared *batchAck so the ACK handler can correlate acks back to this batch.
func buildEvents(beatName, clientIP string, msg *rtq.MessageBatch, ack *batchAck) []beat.Event {
	events := make([]beat.Event, 0, len(msg.Messages))
	for _, message := range msg.Messages {
		events = append(events, beat.Event{
			Timestamp: time.Now(),
			Fields: common.MapStr{
				"type":     beatName,
				"rxtxMsg":  message,
				"clientIp": clientIP,
			},
			Private: ack,
		})
	}
	return events
}

// inHandler builds the POST /in gin handler. Metric updates are injected as
// callbacks (onBatch, onMessages) so the handler can be exercised in tests
// without registering against the global prometheus registry.
//
// Durability: the handler publishes the batch and then waits for the output to
// acknowledge delivery before responding 200, bounded by ackTimeout. If the
// ack does not arrive in time it responds 504 so the sender (e.g. rxtx) keeps
// its durable copy and retries, rather than dropping it on a premature 200.
func inHandler(beatName string, logger *zap.Logger, pub eventPublisher, ackTimeout time.Duration, onBatch func(), onMessages func(n int)) gin.HandlerFunc {
	return func(c *gin.Context) {
		onBatch()

		msg := &rtq.MessageBatch{}
		rawData, _ := c.GetRawData()

		if err := json.Unmarshal(rawData, &msg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "FAIL",
				"message": fmt.Sprintf("could not unmarshal json: %s", rawData),
			})
			logger.Error("Run", zap.String("error", "could not unmarshal json"), zap.Error(err))
			return
		}

		n := len(msg.Messages)
		onMessages(n)

		// Nothing to deliver: acknowledge immediately.
		if n == 0 {
			c.JSON(http.StatusOK, gin.H{"status": "OK"})
			return
		}

		// Capture the client IP synchronously: the gin context is recycled
		// once the handler returns, so it must not be read from the goroutine.
		ack := newBatchAck(n)
		events := buildEvents(beatName, c.ClientIP(), msg, ack)

		go pub.PublishAll(events)

		select {
		case <-ack.done:
			c.JSON(http.StatusOK, gin.H{"status": "OK"})
		case <-time.After(ackTimeout):
			logger.Warn("Run",
				zap.String("state", "ack timeout"),
				zap.Int("messages", n),
				zap.Duration("timeout", ackTimeout),
			)
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"status":  "TIMEOUT",
				"message": "events accepted but not acknowledged by the output within timeout",
			})
		}
	}
}
