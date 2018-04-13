package beater

import (
	"context"
	"fmt"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"

	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"time"

	"github.com/cjimti/rtbeat/config"
	"github.com/cjimti/rxtx/rtq"
	"github.com/gin-gonic/gin"
)

type Rtbeat struct {
	done   chan struct{}
	config config.Config
	client beat.Client
}

// New Creates beater
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	c := config.DefaultConfig
	if err := cfg.Unpack(&c); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	bt := &Rtbeat{
		done:   make(chan struct{}),
		config: c,
	}
	return bt, nil
}

// Run the beat
func (bt *Rtbeat) Run(b *beat.Beat) error {
	logp.Info("rtbeat is running! Hit CTRL-C to stop it.")

	var err error
	bt.client, err = b.Publisher.Connect()
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

		events := make([]beat.Event, 1)
		var event = beat.Event{}

		for _, message := range msg.Messages {
			event = beat.Event{
				Timestamp: time.Now(),
				Fields: common.MapStr{
					"type":    b.Info.Name,
					"rxtxMsg": message,
				},
			}
			events = append(events, event)
		}

		bt.client.PublishAll(events)

		c.JSON(200, gin.H{
			"status": "OK",
		})

	})

	srv := &http.Server{
		Addr:    ":" + bt.config.Port,
		Handler: r,
	}

	go func() {
		// service connections
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	for {
		select {
		case <-bt.done:
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
