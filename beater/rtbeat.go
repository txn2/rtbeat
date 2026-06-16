package beater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
}

// New Creates beater
func New(_ *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	c := config.DefaultConfig
	if err := cfg.Unpack(&c); err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
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

	batches := promauto.NewCounter(prometheus.CounterOpts{
		Name: "rtbeat_batches_received",
		Help: "Total number batches received",
	})

	messages := promauto.NewCounter(prometheus.CounterOpts{
		Name: "rtbeat_messages_parsed",
		Help: "Total number messages parsed.",
	})

	currentAcks := promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rtbeat_current_acks",
		Help: "Current acks.",
	})

	totalAcks := promauto.NewCounter(prometheus.CounterOpts{
		Name: "rtbeat_acks_received",
		Help: "Total number of acks.",
	})

	var err error
	bt.client, err = b.Publisher.ConnectWith(beat.ClientConfig{
		//PublishMode: beat.GuaranteedSend,
		ACKHandler: acker.RawCounting(func(i int) {
			bt.logger.Info("Run", zapcore.Field{
				Key:     "ACKCount",
				Type:    zapcore.Int32Type,
				Integer: int64(i),
			})
			currentAcks.Set(float64(i))
			totalAcks.Add(float64(i))
		}),
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

	r.POST("/in", inHandler(b.Info.Name, bt.logger, bt.client, batches.Inc, func(n int) {
		messages.Add(float64(n))
	}))

	// Prometheus Metrics
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

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

	// shutdown the web server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = srv.Shutdown(ctx)
	cancel()
	return nil
}

// Stop the beat
func (bt *Rtbeat) Stop() {
	_ = bt.client.Close()
	close(bt.done)
}

// eventPublisher is the subset of beat.Client the /in handler needs. It lets
// tests substitute a fake for the libbeat publisher pipeline.
type eventPublisher interface {
	PublishAll([]beat.Event)
}

// buildEvents converts an rxtx MessageBatch into the beat.Event slice rtbeat
// publishes. Each message becomes one event carrying the original message under
// "rxtxMsg" alongside "type" and "clientIp". The slice is pre-sized with one
// leading zero-value event; this is long-standing behavior, preserved here
// intentionally (changing it is tracked separately).
func buildEvents(beatName, clientIP string, msg *rtq.MessageBatch) []beat.Event {
	events := make([]beat.Event, 1)
	for i, message := range msg.Messages {
		events = append(events, beat.Event{
			Timestamp: time.Now(),
			Fields: common.MapStr{
				"type":     beatName,
				"rxtxMsg":  message,
				"clientIp": clientIP,
			},
			Private: i,
		})
	}
	return events
}

// inHandler builds the POST /in gin handler. Metric updates are injected as
// callbacks (onBatch, onMessages) so the handler can be exercised in tests
// without registering against the global prometheus registry. The handler
// responds before publishing so a slow output never blocks the rxtx client.
func inHandler(beatName string, logger *zap.Logger, pub eventPublisher, onBatch func(), onMessages func(n int)) gin.HandlerFunc {
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

		// respond quickly to avoid getting a re-send from the server
		c.JSON(http.StatusOK, gin.H{"status": "OK"})

		onMessages(len(msg.Messages))
		// Capture the client IP synchronously: the gin context is recycled
		// once the handler returns, so it must not be read from the goroutine.
		events := buildEvents(beatName, c.ClientIP(), msg)

		go pub.PublishAll(events)
	}
}
