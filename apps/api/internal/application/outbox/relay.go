package applicationoutbox

import (
	"context"
	"log"
	"time"
)

type Event struct {
	ID         int64
	EventID    string
	Topic      string
	Payload    []byte
	LeaseToken string
	Attempts   int
}

type Repository interface {
	Claim(context.Context) (*Event, error)
	Complete(context.Context, *Event) error
	Retry(context.Context, *Event, error) error
}

type Publisher interface {
	PublishOutbox(context.Context, string, string, []byte) error
}

type Relay struct {
	repo      Repository
	publisher Publisher
}

func New(repo Repository, publisher Publisher) *Relay { return &Relay{repo, publisher} }

// DispatchOne 将单个已租用事件发送至 broker。进程在 confirm 后退出可导致重投，消费者须幂等。
func (r *Relay) DispatchOne(ctx context.Context) (bool, error) {
	event, err := r.repo.Claim(ctx)
	if err != nil || event == nil {
		return false, err
	}
	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = r.publisher.PublishOutbox(publishCtx, event.Topic, event.EventID, event.Payload)
	cancel()
	if err != nil {
		return true, r.repo.Retry(ctx, event, err)
	}
	return true, r.repo.Complete(ctx, event)
}

func (r *Relay) Run(ctx context.Context) {
	for ctx.Err() == nil {
		found, err := r.DispatchOne(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("outbox dispatch failed: %v", err)
		}
		if !found || err != nil {
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}
