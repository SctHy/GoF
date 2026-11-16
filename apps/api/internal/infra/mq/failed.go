package inframq

import (
	"context"
	"errors"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"time"
)

type FailedMessage struct {
	EventID string `json:"event_id"`
	Bytes   int    `json:"bytes"`
	Reason  string `json:"reason"`
}

func (r *RabbitMQ) failedSource(kind string) (string, error) {
	switch kind {
	case "action":
		return r.config.ActionChangedQueue, nil
	case "preheat":
		return r.config.VideoPublishedQueue, nil
	case "embedding":
		return r.config.VideoEmbeddingQueue, nil
	default:
		return "", errors.New("queue must be action, preheat or embedding")
	}
}

// WithFailedBatch 只用于独立管理连接。未成功 ACK 的消息始终重新入队，默认不输出业务 payload。
func (r *RabbitMQ) withFailedBatch(ctx context.Context, kind string, limit int, visit func(string, *amqp.Delivery) (bool, error)) error {
	source, err := r.failedSource(kind)
	if err != nil {
		return err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	deliveries := make([]amqp.Delivery, 0, limit)
	defer func() {
		for _, delivery := range deliveries {
			if delivery.DeliveryTag != 0 {
				_ = delivery.Nack(false, true)
			}
		}
	}()
	for i := 0; i < limit; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		delivery, found, err := r.consumerChannel.Get(source+".failed", false)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		deliveries = append(deliveries, delivery)
		done, err := visit(source, &deliveries[len(deliveries)-1])
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return nil
}

func (r *RabbitMQ) ListFailed(ctx context.Context, kind string, limit int) ([]FailedMessage, error) {
	rows := []FailedMessage{}
	err := r.withFailedBatch(ctx, kind, limit, func(_ string, delivery *amqp.Delivery) (bool, error) {
		reason, _ := delivery.Headers["x-gcfeed-error"].(string)
		rows = append(rows, FailedMessage{delivery.MessageId, len(delivery.Body), reason})
		return false, nil
	})
	return rows, err
}

// ReplayFailed 要求精确 EventID，确认送回原消费队列后才移除失败副本。
// 无法路由/确认失败时保留失败消息；confirm 与 ACK 之间崩溃可能重复，业务消费者须幂等。
func (r *RabbitMQ) ReplayFailed(ctx context.Context, kind, eventID string) error {
	if eventID == "" {
		return errors.New("exact event ID is required")
	}
	replayed := false
	err := r.withFailedBatch(ctx, kind, 100, func(source string, delivery *amqp.Delivery) (bool, error) {
		if delivery.MessageId != eventID {
			return false, nil
		}
		if err := r.publishConfirmed(ctx, "", source, amqp.Publishing{ContentType: delivery.ContentType, DeliveryMode: amqp.Persistent, MessageId: delivery.MessageId, Body: delivery.Body, Timestamp: time.Now().UTC()}); err != nil {
			return false, err
		}
		if err := delivery.Ack(false); err != nil {
			return false, err
		}
		delivery.DeliveryTag = 0
		replayed = true
		return true, nil
	})
	if err != nil {
		return err
	}
	if !replayed {
		return fmt.Errorf("event %s not found in first 100 failed messages", eventID)
	}
	return nil
}
