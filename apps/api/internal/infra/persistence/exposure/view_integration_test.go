package infraexposure_test

import (
	domain "GCFeed/internal/domain/exposure"
	exposure "GCFeed/internal/infra/persistence/exposure"
	"GCFeed/internal/infra/persistence/migration"
	"GCFeed/internal/testutil"
	"context"
	"errors"
	"gorm.io/gorm"
	"testing"
)

func TestViewingFactsAndExposureAggregateAreAtomic(t *testing.T) {
	db := testutil.Database(t)
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO video (id,author_id,title,description,media_url,cover_url,status,published_at) VALUES (7,9,'fixture','','/v','/c',2,NOW())").Error; err != nil {
		t.Fatal(err)
	}
	repo := exposure.New(db)
	for _, kind := range []string{"exposed", "play", "complete", "skip"} {
		event, err := domain.NewViewEvent(42, 7, "timeline", "request", kind, 1000, false)
		if err != nil {
			t.Fatal(err)
		}
		saved, aggregate, err := repo.SaveViewEvent(context.Background(), event)
		if err != nil || saved.ID <= 0 || saved.EventType != kind {
			t.Fatalf("save %s: %+v %v", kind, saved, err)
		}
		if (aggregate != nil) != (kind == "exposed") {
			t.Fatalf("aggregate for %s: %+v", kind, aggregate)
		}
	}
	injected := errors.New("aggregate write unavailable")
	db.Callback().Create().Before("gorm:create").Register("test:aggregate_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "exposures" {
			tx.AddError(injected)
		}
	})
	t.Cleanup(func() { db.Callback().Create().Remove("test:aggregate_failure") })
	event, _ := domain.NewViewEvent(42, 7, "following", "failed", "exposed", 0, false)
	if _, _, err := repo.SaveViewEvent(context.Background(), event); !errors.Is(err, injected) {
		t.Fatalf("error=%v", err)
	}
	var facts, aggregates, messages int64
	if err := db.Table("video_view_events").Count(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("exposures").Count(&aggregates).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("event_outbox").Count(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if facts != 4 || aggregates != 1 || messages != 0 {
		t.Fatalf("facts=%d aggregates=%d messages=%d", facts, aggregates, messages)
	}
}
