package infraoutbox

import (
	applicationoutbox "GCFeed/internal/application/outbox"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const TopicActionChanged = "action.changed"
const TopicVideoPublished = "video.published"

// Model 是持久化待发送事件；独立于任何业务仓储，调用方必须传入业务事务。
type Model struct {
	ID          int64      `gorm:"primaryKey;autoIncrement"`
	EventID     string     `gorm:"size:128;not null;uniqueIndex:uk_outbox_event"`
	Topic       string     `gorm:"size:64;not null"`
	Payload     string     `gorm:"type:longtext;not null"`
	Attempts    int        `gorm:"not null;default:0"`
	AvailableAt time.Time  `gorm:"precision:6;not null;index:idx_outbox_pending,priority:3"`
	LeaseUntil  time.Time  `gorm:"precision:6;not null"`
	LeaseToken  string     `gorm:"size:64;not null;default:''"`
	ProcessedAt *time.Time `gorm:"precision:6;index:idx_outbox_pending,priority:1"`
	FailedAt    *time.Time `gorm:"precision:6;index:idx_outbox_pending,priority:2"`
	LastError   string     `gorm:"size:1000;not null;default:''"`
	CreatedAt   time.Time  `gorm:"precision:6;autoCreateTime"`
}

func (Model) TableName() string { return "event_outbox" }

func Enqueue(tx *gorm.DB, topic, eventID string, payload any) error {
	if eventID == "" || len(eventID) > 128 || (topic != TopicActionChanged && topic != TopicVideoPublished) {
		return errors.New("invalid outbox event")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	model := Model{EventID: eventID, Topic: topic, Payload: string(body), AvailableAt: now, LeaseUntil: time.Unix(0, 0).UTC()}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model).Error
}

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db} }

func (r *Repository) Claim(ctx context.Context) (*applicationoutbox.Event, error) {
	var row Model
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	lease := hex.EncodeToString(token)
	now := time.Now().UTC()
	claimed := false
	attempts := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("processed_at IS NULL AND failed_at IS NULL AND available_at <= ? AND lease_until <= ?", now, now).
			Order("id ASC").Limit(1).Find(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if row.Attempts >= 12 {
			return tx.Model(&row).Updates(map[string]any{"failed_at": now, "lease_token": "", "last_error": "lease expired after maximum attempts"}).Error
		}
		claimed = true
		attempts = row.Attempts + 1
		return tx.Model(&row).Updates(map[string]any{"lease_token": lease, "lease_until": now.Add(30 * time.Second), "attempts": gorm.Expr("attempts + 1")}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, nil
	}
	// GORM 对表达式字段不会写回可靠的数值，重新从领取前值计算。
	return &applicationoutbox.Event{ID: row.ID, EventID: row.EventID, Topic: row.Topic, Payload: []byte(row.Payload), LeaseToken: lease, Attempts: attempts}, nil
}

func (r *Repository) Complete(ctx context.Context, event *applicationoutbox.Event) error {
	result := r.db.WithContext(ctx).Model(&Model{}).Where("id = ? AND lease_token = ? AND processed_at IS NULL", event.ID, event.LeaseToken).
		Updates(map[string]any{"processed_at": time.Now().UTC(), "lease_token": "", "last_error": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("outbox lease lost before completion")
	}
	return nil
}

func (r *Repository) Retry(ctx context.Context, event *applicationoutbox.Event, cause error) error {
	attempts := event.Attempts
	if attempts > 8 {
		attempts = 8
	}
	delay := time.Duration(1<<attempts) * 250 * time.Millisecond
	message := []rune(cause.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	updates := map[string]any{"lease_token": "", "lease_until": time.Unix(0, 0).UTC(), "available_at": time.Now().UTC().Add(delay), "last_error": string(message)}
	if event.Attempts >= 12 {
		updates["failed_at"] = time.Now().UTC()
	}
	result := r.db.WithContext(ctx).Model(&Model{}).Where("id = ? AND lease_token = ? AND processed_at IS NULL", event.ID, event.LeaseToken).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("outbox lease lost before retry")
	}
	return nil
}

func (r *Repository) List(ctx context.Context, limit int) ([]Model, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var rows []Model
	err := r.db.WithContext(ctx).Where("processed_at IS NULL").Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

// Replay 只允许重新排队已隔离的失败事件，保留原 EventID 和 payload 以供消费者去重。
func (r *Repository) Replay(ctx context.Context, eventID string) error {
	result := r.db.WithContext(ctx).Model(&Model{}).Where("event_id = ? AND failed_at IS NOT NULL AND processed_at IS NULL", eventID).
		Updates(map[string]any{"failed_at": nil, "attempts": 0, "available_at": time.Now().UTC(), "lease_until": time.Unix(0, 0).UTC(), "lease_token": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("failed event not found: %s", eventID)
	}
	return nil
}
