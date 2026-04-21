package queue

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/Salon-1C/record-service/internal/recordings"
	amqp "github.com/rabbitmq/amqp091-go"
)

type RecordingMessage struct {
	StreamPath    string    `json:"streamPath"`
	SegmentPath   string    `json:"segmentPath"`
	ContentBase64 string    `json:"contentBase64"`
	Timestamp     time.Time `json:"timestamp"`
}

func StartConsumer(ctx context.Context, rabbitURL, queueName string, svc *recordings.Service) error {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}
	_, err = ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}
	msgs, err := ch.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}
	go func() {
		defer func() {
			_ = ch.Close()
			_ = conn.Close()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				var payload RecordingMessage
				if err := json.Unmarshal(msg.Body, &payload); err != nil {
					log.Printf("queue: invalid message: %v", err)
					_ = msg.Nack(false, false)
					continue
				}
				processCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				err := svc.ProcessQueuedSegment(processCtx, payload.StreamPath, payload.SegmentPath, payload.ContentBase64)
				cancel()
				if err != nil {
					log.Printf("queue: reconcile error: %v", err)
					_ = msg.Nack(false, true)
					continue
				}
				_ = msg.Ack(false)
			}
		}
	}()
	return nil
}
