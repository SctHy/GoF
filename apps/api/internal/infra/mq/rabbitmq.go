package inframq

import (
	applicationinteraction "GCFeed/internal/application/interaction"
	applicationvideo "GCFeed/internal/application/video"
	infraconfig "GCFeed/internal/infra/config"
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const defaultInteractionExchange = "gcfeed.interaction"
const defaultActionChangedQueue = "gcfeed.interaction.action_changed"
const defaultActionChangedRouting = "interaction.action_changed"
const defaultVideoExchange = "gcfeed.video"
const defaultVideoPublishedQueue = "gcfeed.video.published"
const defaultVideoEmbeddingQueue = "gcfeed.video.embedding"
const defaultVideoPublishedRouting = "video.published"

var ErrUnroutable = errors.New("rabbitmq message was not routed to any queue")

var ErrEmptyRabbitMQURL = errors.New("rabbitmq url is empty")

type RabbitMQ struct {
	conn            *amqp.Connection
	publishChannel  *amqp.Channel
	consumerChannel *amqp.Channel
	config          infraconfig.RabbitMQConfig
	errors          chan error
	publicationID   atomic.Uint64
	pendingReturns  sync.Map
	returnBarriers  chan chan struct{}
	returnsDone     chan struct{}
}

func NewRabbitMQ(cfg infraconfig.RabbitMQConfig) (*RabbitMQ, error) {
	return newRabbitMQ(cfg, amqp.DefaultDial(3*time.Second))
}

func newRabbitMQ(cfg infraconfig.RabbitMQConfig, dial func(string, string) (net.Conn, error)) (*RabbitMQ, error) {
	cfg = normalizeRabbitMQConfig(cfg)
	if cfg.URL == "" {
		return nil, ErrEmptyRabbitMQURL
	}
	connectionConfig := amqp.Config{Dial: dial, Heartbeat: 5 * time.Second}
	conn, err := amqp.DialConfig(cfg.URL, connectionConfig)
	if err != nil {
		return nil, err
	}
	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	consumerChannel, err := conn.Channel()
	if err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}

	client := &RabbitMQ{
		conn:            conn,
		publishChannel:  channel,
		consumerChannel: consumerChannel,
		config:          cfg,
		errors:          make(chan error, 1),
	}
	if err := client.ensureTopology(); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := channel.Confirm(false); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := consumerChannel.Qos(32, 0, false); err != nil {
		_ = client.Close()
		return nil, err
	}
	client.startReturnDispatcher()
	client.watchClose(conn.NotifyClose(make(chan *amqp.Error, 1)), "connection")
	client.watchClose(channel.NotifyClose(make(chan *amqp.Error, 1)), "publisher")
	client.watchClose(consumerChannel.NotifyClose(make(chan *amqp.Error, 1)), "consumer")
	return client, nil
}

func (r *RabbitMQ) watchClose(ch <-chan *amqp.Error, name string) {
	go func() {
		if err, ok := <-ch; ok && err != nil {
			r.reportError(fmt.Errorf("rabbitmq %s closed: %w", name, err))
		}
	}()
}

func (r *RabbitMQ) CheckReady(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.conn.IsClosed() || r.publishChannel.IsClosed() || r.consumerChannel.IsClosed() {
		return errors.New("rabbitmq is not ready")
	}
	return nil
}

func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	if r.publishChannel != nil {
		_ = r.publishChannel.Close()
	}
	if r.consumerChannel != nil {
		_ = r.consumerChannel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// Errors reports unexpected consumer shutdown; the worker must not remain falsely healthy.
func (r *RabbitMQ) Errors() <-chan error { return r.errors }

func (r *RabbitMQ) reportError(err error) {
	select {
	case r.errors <- err:
	default:
	}
}

func (r *RabbitMQ) publishConfirmed(ctx context.Context, exchange, key string, message amqp.Publishing) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	message.CorrelationId = fmt.Sprintf("gcfeed-publish-%d", r.publicationID.Add(1))
	returned := make(chan amqp.Return, 1)
	r.pendingReturns.Store(message.CorrelationId, returned)
	defer r.pendingReturns.Delete(message.CorrelationId)
	confirmation, err := r.publishChannel.PublishWithDeferredConfirmWithContext(ctx, exchange, key, true, false, message)
	if err != nil {
		return err
	}
	if confirmation == nil {
		return errors.New("publisher confirms are not enabled")
	}
	ack, err := confirmation.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !ack {
		return errors.New("rabbitmq rejected publication")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// basic.return 在 basic.ack 前到达。无缓冲 Return 通道及 barrier 确保分发完成后再判成功，
	// 同一发布通道仍允许并发等待 confirm，不用串行锁降低吞吐。
	barrier := make(chan struct{})
	select {
	case r.returnBarriers <- barrier:
	case <-r.returnsDone:
		return errors.New("rabbitmq return dispatcher closed")
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-barrier:
	case <-r.returnsDone:
		return errors.New("rabbitmq return dispatcher closed")
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case result := <-returned:
		return fmt.Errorf("%w: code=%d exchange=%s routing_key=%s", ErrUnroutable, result.ReplyCode, result.Exchange, result.RoutingKey)
	default:
		return ctx.Err()
	}
}

func (r *RabbitMQ) ConsumeActionChanged(ctx context.Context, handler func(context.Context, *applicationinteraction.ActionChangedEvent) error) error {
	return consumeEvents(r, ctx, r.config.ActionChangedQueue, "mq_action_changed", handler)
}

func (r *RabbitMQ) ConsumeVideoPublished(ctx context.Context, handler func(context.Context, *applicationvideo.PublishedEvent) error) error {
	return consumeEvents(r, ctx, r.config.VideoPublishedQueue, "mq_video_published", handler)
}

func (r *RabbitMQ) ConsumeVideoPublishedForEmbedding(ctx context.Context, handler func(context.Context, *applicationvideo.PublishedEvent) error) error {
	return consumeEvents(r, ctx, r.config.VideoEmbeddingQueue, "mq_video_embedding", handler)
}

func consumeEvents[T any](r *RabbitMQ, ctx context.Context, queue, job string, handler func(context.Context, *T) error) error {
	deliveries, err := r.consumerChannel.ConsumeWithContext(ctx, queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	go func() {
		defer func() {
			if ctx.Err() == nil {
				r.reportError(fmt.Errorf("consumer stopped: %s", queue))
			}
		}()
		for delivery := range deliveries {
			start := time.Now()
			var event T
			err := json.Unmarshal(delivery.Body, &event)
			if err == nil {
				err = retryHandler(ctx, func() error { return handler(ctx, &event) })
			}
			inframetrics.ObserveWorkerJob(job+"_consume", time.Since(start), err)
			if ctx.Err() != nil {
				_ = delivery.Nack(false, true)
				return
			}
			if err != nil {
				// 独立持久化失败队列，不修改既有队列参数；确认保留原消息后才 ACK。
				// 保留 MessageId，供后续人工排查与幂等重放。
				if publishErr := r.publishConfirmed(ctx, "", queue+".failed", amqp.Publishing{
					ContentType: delivery.ContentType, DeliveryMode: amqp.Persistent,
					MessageId: delivery.MessageId, Timestamp: delivery.Timestamp, Body: delivery.Body,
					Headers: amqp.Table{"x-gcfeed-source": queue, "x-gcfeed-error": failureReason(err)},
				}); publishErr != nil {
					_ = delivery.Nack(false, true)
					r.reportError(fmt.Errorf("failed to retain message from %s: %w", queue, publishErr))
					return
				}
			}
			if err := delivery.Ack(false); err != nil {
				r.reportError(fmt.Errorf("ack %s: %w", queue, err))
				return
			}
		}
	}()
	return nil
}

func retryHandler(ctx context.Context, handler func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err = handler(); err == nil {
			return nil
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(1<<attempt) * 250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return err
}

func (r *RabbitMQ) ensureTopology() error {
	for _, queue := range []string{r.config.ActionChangedQueue, r.config.VideoPublishedQueue, r.config.VideoEmbeddingQueue} {
		if _, err := r.publishChannel.QueueDeclare(queue+".failed", true, false, false, false, nil); err != nil {
			return err
		}
	}
	if err := r.publishChannel.ExchangeDeclare(
		r.config.InteractionExchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}
	if _, err := r.publishChannel.QueueDeclare(
		r.config.ActionChangedQueue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.QueueBind(
		r.config.ActionChangedQueue,
		r.config.ActionChangedRouting,
		r.config.InteractionExchange,
		false,
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.ExchangeDeclare(
		r.config.VideoExchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}
	if _, err := r.publishChannel.QueueDeclare(
		r.config.VideoPublishedQueue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.QueueBind(
		r.config.VideoPublishedQueue,
		r.config.VideoPublishedRouting,
		r.config.VideoExchange,
		false,
		nil,
	); err != nil {
		return err
	}
	if _, err := r.publishChannel.QueueDeclare(
		r.config.VideoEmbeddingQueue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}
	if err := r.publishChannel.QueueBind(
		r.config.VideoEmbeddingQueue,
		r.config.VideoPublishedRouting,
		r.config.VideoExchange,
		false,
		nil,
	); err != nil {
		return err
	}
	return nil
}

func normalizeRabbitMQConfig(cfg infraconfig.RabbitMQConfig) infraconfig.RabbitMQConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.InteractionExchange = strings.TrimSpace(cfg.InteractionExchange)
	cfg.ActionChangedQueue = strings.TrimSpace(cfg.ActionChangedQueue)
	cfg.ActionChangedRouting = strings.TrimSpace(cfg.ActionChangedRouting)
	if cfg.InteractionExchange == "" {
		cfg.InteractionExchange = defaultInteractionExchange
	}
	if cfg.ActionChangedQueue == "" {
		cfg.ActionChangedQueue = defaultActionChangedQueue
	}
	if cfg.ActionChangedRouting == "" {
		cfg.ActionChangedRouting = defaultActionChangedRouting
	}
	if cfg.VideoExchange == "" {
		cfg.VideoExchange = defaultVideoExchange
	}
	if cfg.VideoPublishedQueue == "" {
		cfg.VideoPublishedQueue = defaultVideoPublishedQueue
	}
	cfg.VideoEmbeddingQueue = strings.TrimSpace(cfg.VideoEmbeddingQueue)
	if cfg.VideoEmbeddingQueue == "" {
		cfg.VideoEmbeddingQueue = defaultVideoEmbeddingQueue
	}
	if cfg.VideoPublishedRouting == "" {
		cfg.VideoPublishedRouting = defaultVideoPublishedRouting
	}
	return cfg
}

func (r *RabbitMQ) startReturnDispatcher() {
	r.returnBarriers = make(chan chan struct{})
	r.returnsDone = make(chan struct{})
	returns := r.publishChannel.NotifyReturn(make(chan amqp.Return))
	go func() {
		defer close(r.returnsDone)
		for {
			select {
			case result, ok := <-returns:
				if !ok {
					return
				}
				if pending, ok := r.pendingReturns.Load(result.CorrelationId); ok {
					select {
					case pending.(chan amqp.Return) <- result:
					default:
					}
				}
			case barrier := <-r.returnBarriers:
				close(barrier)
			}
		}
	}()
}

// PublishOutbox 保留事件 ID；生产者无需理解业务事件类型，也不重写持久化 payload。
func (r *RabbitMQ) PublishOutbox(ctx context.Context, topic, eventID string, payload []byte) error {
	var exchange, routing string
	switch topic {
	case "action.changed":
		exchange, routing = r.config.InteractionExchange, r.config.ActionChangedRouting
	case "video.published":
		exchange, routing = r.config.VideoExchange, r.config.VideoPublishedRouting
	default:
		return fmt.Errorf("unsupported outbox topic: %s", topic)
	}
	return r.publishConfirmed(ctx, exchange, routing, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: eventID, Timestamp: time.Now(), Body: payload})
}

func failureReason(err error) string {
	if err == nil {
		return ""
	}
	message := []rune(err.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	return string(message)
}
