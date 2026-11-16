package infraoutbox_test

import (
	app "GCFeed/internal/application/outbox"
	"GCFeed/internal/infra/persistence/migration"
	outbox "GCFeed/internal/infra/persistence/outbox"
	"GCFeed/internal/testutil"
	"context"
	"errors"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"
)

type publisher struct {
	calls int
	fail  bool
}

func (p *publisher) PublishOutbox(context.Context, string, string, []byte) error {
	p.calls++
	if p.fail {
		return errors.New("broker unavailable")
	}
	return nil
}

func TestOutboxRollbackLeaseRetryAndRecovery(t *testing.T) {
	db := testutil.Database(t)
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rollback := errors.New("business rollback")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := outbox.Enqueue(tx, outbox.TopicVideoPublished, "rolled-back", map[string]any{"video_id": 1}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var count int64
	db.Model(&outbox.Model{}).Count(&count)
	if count != 0 {
		t.Fatal("outbox escaped business rollback")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return outbox.Enqueue(tx, outbox.TopicVideoPublished, "one", map[string]any{"video_id": 1})
	}); err != nil {
		t.Fatal(err)
	}
	repo := outbox.New(db)
	claimed := make(chan *app.Event, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event, err := repo.Claim(ctx)
			if err != nil {
				t.Error(err)
			}
			claimed <- event
		}()
	}
	wg.Wait()
	close(claimed)
	var event *app.Event
	for value := range claimed {
		if value != nil {
			if event != nil {
				t.Fatal("double lease")
			}
			event = value
		}
	}
	if event == nil || event.Attempts != 1 {
		t.Fatalf("claim=%+v", event)
	}
	stale := *event
	// 模拟发布进程被杀：租约到期后另一轮领取，旧租约不能确认新任务。
	if err := db.Model(&outbox.Model{}).Where("id=?", event.ID).Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	event, err := repo.Claim(ctx)
	if err != nil || event == nil || event.Attempts != 2 {
		t.Fatalf("reclaim=%+v err=%v", event, err)
	}
	if err := repo.Complete(ctx, &stale); err == nil {
		t.Fatal("stale lease completed event")
	}
	if err := repo.Retry(ctx, event, errors.New("offline")); err != nil {
		t.Fatal(err)
	}
	if next, err := repo.Claim(ctx); err != nil || next != nil {
		t.Fatal("backoff ignored")
	}
	db.Model(&outbox.Model{}).Where("id=?", event.ID).Update("available_at", time.Now().Add(-time.Second))
	pub := &publisher{fail: true}
	relay := app.New(repo, pub)
	if found, err := relay.DispatchOne(ctx); err != nil || !found {
		t.Fatalf("failed publish: %v", err)
	}
	var row outbox.Model
	db.First(&row, event.ID)
	if row.ProcessedAt != nil || row.LastError == "" {
		t.Fatal("failed publication marked done")
	}
	db.Model(&row).Update("available_at", time.Now().Add(-time.Second))
	pub.fail = false
	if found, err := relay.DispatchOne(ctx); err != nil || !found {
		t.Fatalf("recovered publish: %v", err)
	}
	db.First(&row, event.ID)
	if row.ProcessedAt == nil || pub.calls != 2 {
		t.Fatalf("recovery failed: %+v", row)
	}
}

func TestExpiredLeaseAttemptLimitIsQuarantined(t *testing.T) {
	db := testutil.Database(t)
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return outbox.Enqueue(tx, outbox.TopicVideoPublished, "crashed", map[string]int{"video_id": 1})
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&outbox.Model{}).Where("event_id=?", "crashed").Update("attempts", 12).Error; err != nil {
		t.Fatal(err)
	}
	repo := outbox.New(db)
	event, err := repo.Claim(context.Background())
	if err != nil || event != nil {
		t.Fatalf("over-limit claim=%+v err=%v", event, err)
	}
	var row outbox.Model
	if err := db.Where("event_id=?", "crashed").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.FailedAt == nil {
		t.Fatal("over-limit event left hanging")
	}
	if err := repo.Replay(context.Background(), "crashed"); err != nil {
		t.Fatal(err)
	}
	event, err = repo.Claim(context.Background())
	if err != nil || event == nil || event.Attempts != 1 {
		t.Fatalf("controlled replay=%+v %v", event, err)
	}
}

func TestQueueStatsIncludesLeasesAndBackoffButExcludesCompleted(t *testing.T) {
	db := testutil.Database(t)
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := outbox.New(db)
	empty, err := repo.QueueStats(ctx)
	if err != nil || empty.Pending != 0 || empty.Failed != 0 || empty.OldestPendingAgeSeconds != 0 {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	for _, id := range []string{"leased", "delayed", "failed", "complete"} {
		if err := outbox.Enqueue(db, outbox.TopicVideoPublished, id, map[string]int{"user_id": 1}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	if err := db.Model(&outbox.Model{}).Where("event_id = ?", "leased").Updates(map[string]any{"lease_until": now.Add(time.Minute), "created_at": now.Add(-time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&outbox.Model{}).Where("event_id = ?", "delayed").Update("available_at", now.Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&outbox.Model{}).Where("event_id = ?", "failed").Update("failed_at", now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&outbox.Model{}).Where("event_id = ?", "complete").Update("processed_at", now).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := repo.QueueStats(ctx)
	if err != nil || stats.Pending != 2 || stats.Failed != 1 || stats.OldestPendingAgeSeconds < 59 || stats.OldestPendingAgeSeconds > 70 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
}
