package infrainteraction

import (
	app "GCFeed/internal/application/interaction"
	domain "GCFeed/internal/domain/interaction"
	account "GCFeed/internal/infra/persistence/account"
	message "GCFeed/internal/infra/persistence/message"
	outbox "GCFeed/internal/infra/persistence/outbox"
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"testing"
	"time"
)

type notificationTestHot struct {
	fail  bool
	calls int
}

func (h *notificationTestHot) AddHotScoreOnce(context.Context, string, int64, int, time.Time) error {
	h.calls++
	if h.fail {
		return errors.New("Redis temporarily unavailable")
	}
	return nil
}

func TestActionNotificationSurvivesProjectionFailureAndDuplicateReplay(t *testing.T) {
	db := isolatedDB(t)
	if err := db.AutoMigrate(&account.UserModel{}, &message.MessageModel{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&account.UserModel{ID: 42, Account: "actor", Password: "unused", Nickname: "actor", Status: 1, Role: "USER"}).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	ctx := context.Background()
	if _, _, _, err := repo.CommitAction(ctx, 42, 1, domain.ActionTypeLike, true, "like"); err != nil {
		t.Fatal(err)
	}
	var row outbox.Model
	if err := db.Order("id").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	var event app.ActionChangedEvent
	if err := json.Unmarshal([]byte(row.Payload), &event); err != nil {
		t.Fatal(err)
	}
	if event.EffectsVersion != 1 || event.Delta != 1 {
		t.Fatalf("missing durable side effect: %+v", event)
	}
	hot := &notificationTestHot{}
	worker := app.NewActionWorker(nil, noopInvalidator{}, app.NewActionEffects(repo, hot))
	callback := "test:fail_notification"
	if err := db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_message" {
			tx.AddError(errors.New("injected notification write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := worker.HandleActionChanged(ctx, &event); err == nil {
		t.Fatal("message write failure was acknowledged")
	}
	if hot.calls != 0 {
		t.Fatal("hot projection advanced after failure")
	}
	if err := db.Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	// Retrying after a partial projection must revisit notification and Redis.
	hot.fail = true
	if err := worker.HandleActionChanged(ctx, &event); err == nil {
		t.Fatal("hot write failure was acknowledged")
	}
	hot.fail = false
	for i := 0; i < 3; i++ {
		if err := worker.HandleActionChanged(ctx, &event); err != nil {
			t.Fatal(err)
		}
	}
	var rows []message.MessageModel
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].UserID != 9 || rows[0].ActorID != 42 || rows[0].ActorNickname != "actor" || *rows[0].EventID != "interaction:like:1:42" {
		t.Fatalf("notification identity/count=%+v", rows)
	}
	// A later re-like still follows the previous product contract: one notification per actor/video.
	if _, _, _, err := repo.CommitAction(ctx, 42, 1, domain.ActionTypeLike, false, "unlike"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := repo.CommitAction(ctx, 42, 1, domain.ActionTypeLike, true, "relike"); err != nil {
		t.Fatal(err)
	}
	var events []outbox.Model
	if err := db.Order("id DESC").Limit(1).Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(events[0].Payload), &event); err != nil {
		t.Fatal(err)
	}
	if err := worker.HandleActionChanged(ctx, &event); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&message.MessageModel{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("relike duplicated notification: %d %v", count, err)
	}
}

type noopInvalidator struct{}

func (noopInvalidator) InvalidateVideoStats(context.Context, int64) error { return nil }
