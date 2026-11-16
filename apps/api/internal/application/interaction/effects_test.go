package applicationinteraction

import (
	applicationmessage "GCFeed/internal/application/message"
	domain "GCFeed/internal/domain/interaction"
	"context"
	"errors"
	"testing"
	"time"
)

type effectsRepo struct {
	domain.Repository
}

func (r *effectsRepo) CommitAction(context.Context, int64, int64, string, bool, string) (*domain.Action, int, int, error) {
	return &domain.Action{VideoID: 1, UserID: 2, ActionType: domain.ActionTypeLike, Status: domain.ActionStatusActive}, 1, 1, nil
}

type effectsNotifications struct {
	calls int
	fail  bool
}

func (n *effectsNotifications) ProjectActionNotification(context.Context, *ActionChangedEvent) error {
	n.calls++
	if n.fail {
		return errors.New("notification unavailable")
	}
	return nil
}

type effectsHot struct {
	calls int
	delta int
	fail  bool
}

func (h *effectsHot) AddHotScoreOnce(_ context.Context, _ string, _ int64, delta int, _ time.Time) error {
	h.calls++
	h.delta = delta
	if h.fail {
		return errors.New("Redis unavailable")
	}
	return nil
}
func (h *effectsHot) AddHotScore(context.Context, int64, int, time.Time) error { h.calls++; return nil }

type effectsWriter struct{ calls int }

func (w *effectsWriter) CreateFromActorEvent(context.Context, int64, string, string, string, string, string, int64, string, string) (*applicationmessage.CreateResult, error) {
	w.calls++
	return nil, nil
}

func TestDurableServiceDoesNotRunDirectEffects(t *testing.T) {
	repo := &effectsRepo{}
	hot := &effectsHot{}
	writer := &effectsWriter{}
	service := New(repo, WithHotScoreRecorder(hot), WithMessageWriter(writer))
	if _, err := service.Like(context.Background(), 2, 1, "A"); err != nil {
		t.Fatal(err)
	}
	if hot.calls != 0 || writer.calls != 0 {
		t.Fatalf("direct effects: hot=%d notifications=%d", hot.calls, writer.calls)
	}
}
func TestActionEffectsRetryAndRejectUnsupportedEvents(t *testing.T) {
	notifications := &effectsNotifications{fail: true}
	hot := &effectsHot{}
	worker := NewActionWorker(nil, &effectsInvalidator{}, NewActionEffects(notifications, hot))
	event := &ActionChangedEvent{EffectsVersion: 1, Version: 1, EventID: "event", VideoID: 1, UserID: 2, ActionType: domain.ActionTypeLike, Delta: 1, Active: true, OccurredAt: time.Now()}
	if err := worker.HandleActionChanged(context.Background(), event); err == nil {
		t.Fatal("notification failure swallowed")
	}
	if hot.calls != 0 {
		t.Fatal("advanced after failed projection")
	}
	notifications.fail = false
	hot.fail = true
	if err := worker.HandleActionChanged(context.Background(), event); err == nil {
		t.Fatal("Redis failure swallowed")
	}
	hot.fail = false
	if err := worker.HandleActionChanged(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if notifications.calls != 3 || hot.calls != 2 || hot.delta != 3 {
		t.Fatalf("retry did not revisit side effects: notifications=%+v hot=%+v", notifications, hot)
	}
	legacy := *event
	legacy.EffectsVersion = 0
	if err := worker.HandleActionChanged(context.Background(), &legacy); err == nil {
		t.Fatal("accepted unversioned event")
	}
	if notifications.calls != 3 || hot.calls != 2 {
		t.Fatal("legacy event acquired new side effects")
	}
	event.Delta = 0
	if err := worker.HandleActionChanged(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if notifications.calls != 3 || hot.calls != 2 {
		t.Fatal("no-op action acquired side effects")
	}
	event.EffectsVersion = 2
	if err := worker.HandleActionChanged(context.Background(), event); err == nil {
		t.Fatal("unknown effects protocol silently acknowledged")
	}
}

type effectsInvalidator struct{ fail bool }

func (i *effectsInvalidator) InvalidateVideoStats(context.Context, int64) error {
	if i.fail {
		return errors.New("cache unavailable")
	}
	return nil
}

func TestActionWorkerRetriesInvalidationBeforeEffects(t *testing.T) {
	invalidator := &effectsInvalidator{fail: true}
	hot := &effectsHot{}
	notifications := &effectsNotifications{}
	worker := NewActionWorker(nil, invalidator, NewActionEffects(notifications, hot))
	event := &ActionChangedEvent{EffectsVersion: 1, Version: 1, EventID: "retry", UserID: 2, VideoID: 1, ActionType: domain.ActionTypeLike, Delta: 1, Active: true, OccurredAt: time.Now()}
	if err := worker.HandleActionChanged(context.Background(), event); err == nil || notifications.calls != 0 {
		t.Fatal("invalidation failure swallowed")
	}
	invalidator.fail = false
	if err := worker.HandleActionChanged(context.Background(), event); err != nil || hot.calls != 1 {
		t.Fatalf("retry: %v", err)
	}
}
