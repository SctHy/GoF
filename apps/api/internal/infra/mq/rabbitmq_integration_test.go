package inframq

import (
	applicationinteraction "GCFeed/internal/application/interaction"
	infraconfig "GCFeed/internal/infra/config"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestFailedMessageInspectionAndTargetedReplay(t *testing.T) {
	mq := testRabbitMQ(t, amqp.DefaultDial(3*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, event := range []applicationinteraction.ActionChangedEvent{{EventID: "versioned", Version: 2}, {EventID: "second", Version: 1}} {
		body, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if err := mq.publishConfirmed(ctx, "", mq.config.ActionChangedQueue+".failed", amqp.Publishing{MessageId: event.EventID, Body: body, Headers: amqp.Table{"x-gcfeed-error": "test failure"}}); err != nil {
			t.Fatal(err)
		}
	}
	assertFailedCount := func(want int) {
		t.Helper()
		rows, err := mq.ListFailed(ctx, "action", 50)
		if err != nil || len(rows) != want {
			t.Fatalf("failed rows=%v err=%v want=%d", rows, err, want)
		}
		for _, row := range rows {
			if row.Reason != "test failure" {
				t.Fatalf("missing failure metadata: %+v", row)
			}
		}
	}
	assertFailedCount(2)
	assertFailedCount(2)
	if err := mq.ReplayFailed(ctx, "action", "missing"); err == nil {
		t.Fatal("missing ID succeeded")
	}
	assertFailedCount(2)
	if err := mq.ReplayFailed(ctx, "action", "versioned"); err != nil {
		t.Fatal(err)
	}
	assertFailedCount(1)
	message, found, err := mq.consumerChannel.Get(mq.config.ActionChangedQueue, true)
	if err != nil || !found || message.MessageId != "versioned" {
		t.Fatalf("replayed message=%+v found=%v err=%v", message, found, err)
	}
	var event applicationinteraction.ActionChangedEvent
	if err := json.Unmarshal(message.Body, &event); err != nil || event.Version != 2 {
		t.Fatalf("payload changed: %s", message.Body)
	}
	// A confirmed publication that cannot route must not remove its failed copy.
	if _, err := mq.consumerChannel.QueueDelete(mq.config.ActionChangedQueue, false, false, false); err != nil {
		t.Fatal(err)
	}
	if err := mq.ReplayFailed(ctx, "action", "second"); !errors.Is(err, ErrUnroutable) {
		t.Fatalf("unroutable replay=%v", err)
	}
	assertFailedCount(1)
	if _, err := mq.consumerChannel.QueueDeclare(mq.config.ActionChangedQueue, true, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := mq.ReplayFailed(ctx, "action", "second"); err != nil {
		t.Fatal(err)
	}
	assertFailedCount(0)
}

// 使用独立命名的队列和交换机；测试结束删除自己的拓扑，不停止共享 Broker。
func testRabbitMQ(t *testing.T, dial func(string, string) (net.Conn, error)) *RabbitMQ {
	t.Helper()

	url := os.Getenv("GCFEED_TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("set GCFEED_TEST_RABBITMQ_URL for real RabbitMQ regression")
	}
	prefix := fmt.Sprintf("gcfeed.test.%d", time.Now().UnixNano())
	cfg := infraconfig.RabbitMQConfig{
		URL: url, InteractionExchange: prefix + ".interaction", ActionChangedQueue: prefix + ".action", ActionChangedRouting: "action",
		VideoExchange: prefix + ".video", VideoPublishedQueue: prefix + ".preheat", VideoEmbeddingQueue: prefix + ".embedding", VideoPublishedRouting: "video",
	}
	mq, err := newRabbitMQ(cfg, dial)
	if err != nil {
		t.Fatal("cannot connect test broker")
	}
	t.Cleanup(func() { _ = mq.Close() })
	t.Cleanup(func() {
		channel, err := mq.conn.Channel()
		if err != nil {
			t.Error(err)
			return
		}
		defer channel.Close()
		for _, queue := range []string{cfg.ActionChangedQueue, cfg.VideoPublishedQueue, cfg.VideoEmbeddingQueue} {
			if _, err := channel.QueueDelete(queue, false, false, false); err != nil {
				t.Error(err)
			}
			if _, err := channel.QueueDelete(queue+".failed", false, false, false); err != nil {
				t.Error(err)
			}
		}
		for _, exchange := range []string{cfg.InteractionExchange, cfg.VideoExchange} {
			if err := channel.ExchangeDelete(exchange, false, false); err != nil {
				t.Error(err)
			}
		}
	})
	return mq
}

func TestRabbitMQConfirmedDeliveryAndFailureQueue(t *testing.T) {
	mq := testRabbitMQ(t, amqp.DefaultDial(3*time.Second))
	cfg := mq.config

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var retryCalls, poisonCalls atomic.Int32
	recovered := make(chan struct{}, 1)
	if err := mq.ConsumeActionChanged(ctx, func(ctx context.Context, event *applicationinteraction.ActionChangedEvent) error {
		if event.EventID == "retry" {
			if retryCalls.Add(1) == 1 {
				return errors.New("temporary")
			}
			recovered <- struct{}{}
			return nil
		}
		poisonCalls.Add(1)
		return errors.New("permanent")
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"retry", "poison"} {
		if err := mq.PublishOutbox(ctx, "action.changed", id, []byte(fmt.Sprintf(`{"event_id":%q}`, id))); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-recovered:
	case <-ctx.Done():
		t.Fatal("transient failure did not recover")
	}
	for {
		message, ok, err := mq.publishChannel.Get(cfg.ActionChangedQueue+".failed", true)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if message.MessageId != "poison" || poisonCalls.Load() != 3 || retryCalls.Load() != 2 {
				t.Fatalf("failed message=%s retry=%d poison=%d", message.MessageId, retryCalls.Load(), poisonCalls.Load())
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("failed message was not retained")
		case <-time.After(30 * time.Millisecond):
		}
	}
	// 模拟消费通道故障，必须显式通知 Worker，不能留一个空转的健康进程。
	if err := mq.consumerChannel.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-mq.Errors():
	case <-ctx.Done():
		t.Fatal("consumer failure was not reported")
	}
	if err := mq.CheckReady(ctx); err == nil {
		t.Fatal("closed consumer reported ready")
	}
	if err := mq.publishChannel.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mq.PublishOutbox(ctx, "action.changed", "closed", []byte(`{}`)); err == nil {
		t.Fatal("closed publisher returned success")
	}
}

func TestConcurrentPublishSeparatesRoutedAndReturnedMessages(t *testing.T) {
	mq := testRabbitMQ(t, amqp.DefaultDial(3*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		go func(i int) {
			key := mq.config.ActionChangedRouting
			if i%2 == 1 {
				key = "unbound.routing.key"
			}
			err := mq.publishConfirmed(ctx, mq.config.InteractionExchange, key, amqp.Publishing{MessageId: fmt.Sprintf("event-%d", i), DeliveryMode: amqp.Persistent, Body: []byte(`{}`)})
			if i%2 == 1 && !errors.Is(err, ErrUnroutable) {
				results <- fmt.Errorf("unrouted %d reported %v", i, err)
				return
			}
			if i%2 == 0 && err != nil {
				results <- fmt.Errorf("routed %d failed: %w", i, err)
				return
			}
			results <- nil
		}(i)
	}
	for i := 0; i < 32; i++ {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if err := mq.PublishOutbox(ctx, "action.changed", "cancelled", []byte(`{}`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled publish=%v", err)
	}
}
