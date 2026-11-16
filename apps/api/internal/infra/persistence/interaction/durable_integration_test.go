package infrainteraction

import (
	domain "GCFeed/internal/domain/interaction"
	outbox "GCFeed/internal/infra/persistence/outbox"
	video "GCFeed/internal/infra/persistence/video"
	"GCFeed/internal/testutil"
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"
)

func isolatedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.Database(t)
	if err := db.AutoMigrate(&ActionModel{}, &ActionRequestModel{}, &video.VideoModel{}, &video.VideoStatModel{}, &outbox.Model{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Create(&video.VideoModel{ID: 1, AuthorID: 9, Title: "test", MediaURL: "/test", CoverURL: "/test", Status: 2, PublishedAt: &now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&video.VideoStatModel{VideoID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}
func TestDurableActionHistoryOrderingAndReconcile(t *testing.T) {
	db := isolatedDB(t)
	repo := New(db)
	ctx := context.Background()
	first, count, _, err := repo.CommitAction(ctx, 42, 1, domain.ActionTypeLike, true, "A")
	if err != nil || count != 1 {
		t.Fatalf("like: %v %d", err, count)
	}
	second, count, _, err := repo.CommitAction(ctx, 42, 1, domain.ActionTypeLike, false, "B")
	if err != nil || count != 0 || second.Version <= first.Version {
		t.Fatalf("unlike: %v %+v", err, second)
	}
	replay, count, delta, err := repo.CommitAction(ctx, 42, 1, domain.ActionTypeLike, true, "A")
	if err != nil || !replay.Active() || count != 1 || delta != 0 {
		t.Fatalf("historical replay: %v %+v %d", err, replay, delta)
	}
	current, _ := repo.GetAction(ctx, 42, 1, domain.ActionTypeLike)
	if current.Active() || current.Version != second.Version {
		t.Fatal("historical request regressed current state")
	}
	rows, err := repo.ReconcileCounts(ctx, 0, 100)
	if err != nil || len(rows) != 1 || rows[0].ActualLikes != rows[0].StoredLikes {
		t.Fatalf("reconcile: %+v %v", rows, err)
	}
	if err := db.Model(&video.VideoStatModel{}).Where("video_id=1").Update("like_count", 9).Error; err != nil {
		t.Fatal(err)
	}
	rows, err = repo.ReconcileCounts(ctx, 0, 100)
	if err != nil || rows[0].StoredLikes != 9 || rows[0].ActualLikes != 0 {
		t.Fatal("reconcile missed drift")
	}
}
func TestDurableActionOutboxRollbackAndConcurrentWriters(t *testing.T) {
	db := isolatedDB(t)
	repo := New(db)
	ctx := context.Background()
	// 事务中 enqueue 失败必须回滚业务事实及计数。
	callback := "test:fail_outbox"
	db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == (&outbox.Model{}).TableName() {
			tx.AddError(fmt.Errorf("injected enqueue failure"))
		}
	})
	if _, _, _, err := repo.CommitAction(ctx, 42, 1, domain.ActionTypeLike, true, "rollback"); err == nil {
		t.Fatal("expected failure")
	}
	db.Callback().Create().Remove(callback)
	stat, _ := repo.GetVideoStat(ctx, 1)
	if stat.LikeCount != 0 {
		t.Fatal("failed outbox committed count")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, _, _, err := repo.CommitAction(ctx, int64(i+100), 1, domain.ActionTypeLike, true, fmt.Sprintf("key-%d", i)); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	stat, _ = repo.GetVideoStat(ctx, 1)
	if stat.LikeCount != 12 {
		t.Fatalf("concurrent writes lost: %d", stat.LikeCount)
	}
	var count int64
	db.Model(&outbox.Model{}).Count(&count)
	if count != 12 {
		t.Fatalf("outbox count=%d", count)
	}
	b, _ := json.Marshal(stat)
	t.Log(string(b))
}
