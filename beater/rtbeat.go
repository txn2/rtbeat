package beater

import (
	"context"
	"fmt"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/txn2/rtbeat/config"
	"github.com/txn2/rxtx/rtq"
	"github.com/gin-gonic/gin"
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
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
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
	bt.logger.Info("Run", zapcore.Field{
		Key:    "Status",
		Type:   zapcore.StringType,
		String: "rtBeat is running! Hit CTRL-C to stop it.",
	})

	var err error
	bt.client, err = b.Publisher.ConnectWith(beat.ClientConfig{
		//PublishMode: beat.GuaranteedSend,
		ACKCount: func(i int) {
			bt.logger.Info("Run", zapcore.Field{
				Key:    "ACKCount",
				Type:   zapcore.Int32Type,
				Integer: int64(i),
			})
		},
	})
	if err != nil {
		return err
	}

	// gin config
	gin.SetMode(gin.ReleaseMode)
	gin.DisableConsoleColor()

	// discard default logger
	gin.DefaultWriter = ioutil.Discard

	//get a router
	r := gin.Default()

	r.POST("/in", func(c *gin.Context) {

		msg := &rtq.MessageBatch{}

		rawData, _ := c.GetRawData()

		err := json.Unmarshal(rawData, &msg)
		if err != nil {
			c.JSON(500, gin.H{
				"status":  "FAIL",
				"message": fmt.Sprintf("could not unmarshal json: %s", rawData),
			})

			logp.Error(err)
			return
		}

		// respond quickly to avoid getting a re-send from the server
		c.JSON(200, gin.H{
			"status": "OK",
		})

		// fie this in the background
		go func() {
			events := make([]beat.Event, 1)
			var event = beat.Event{}

			for i, message := range msg.Messages {
				event = beat.Event{
					Timestamp: time.Now(),
					Fields: common.MapStr{
						"type":     b.Info.Name,
						"rxtxMsg":  message,
						"clientIp": c.ClientIP(),
					},
					Private: i,
				}
				events = append(events, event)
			}

			// TODO check for published state
			bt.client.PublishAll(events)
		}()

	})

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
	for {
		select {
		case <-bt.done:
			bt.logger.Info("Run",
				zapcore.Field{
					Key:    "Status",
					Type:   zapcore.StringType,
					String: "Shutting down web server.",
				},
			)

			// shutdown the web server
			ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
			srv.Shutdown(ctx)
			return nil
		}
	}

}

// Stop the beat
func (bt *Rtbeat) Stop() {
	bt.client.Close()
	close(bt.done)
}
