package infravideo_test

import (
	app "GCFeed/internal/application/video"
	domainasset "GCFeed/internal/domain/asset"
	asset "GCFeed/internal/infra/persistence/asset"
	"GCFeed/internal/infra/persistence/migration"
	video "GCFeed/internal/infra/persistence/video"
	"GCFeed/internal/testutil"
	"context"
	"errors"
	"testing"
)

func TestPublishRequiresOwnedAssetsAndPreservesIdempotency(t *testing.T) {
	db := testutil.Database(t)
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	assets := asset.New(db)
	for _, a := range []struct{ url, kind string }{{"/uploads/video/owned.mp4", "video"}, {"/uploads/cover/owned.png", "cover"}} {
		if err := assets.Register(ctx, 42, a.url, a.kind, 100); err != nil {
			t.Fatal(err)
		}
	}
	service := app.New(video.New(db))
	for _, c := range []struct {
		owner        int64
		media, cover string
	}{
		{43, "/uploads/video/owned.mp4", "/uploads/cover/owned.png"},
		{42, "https://example.invalid/video.mp4", "/uploads/cover/owned.png"},
		{42, "/uploads/cover/owned.png", "/uploads/video/owned.mp4"},
	} {
		if _, err := service.CreatePublished(ctx, c.owner, "test", "", c.media, c.cover, ""); !errors.Is(err, domainasset.ErrNotOwned) {
			t.Fatalf("unowned publication: %v", err)
		}
	}
	for _, table := range []string{"video", "video_stat", "event_outbox"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rejected publish left %s rows: %d", table, count)
		}
	}
	first, err := service.CreatePublished(ctx, 42, "valid", "", "/uploads/video/owned.mp4", "/uploads/cover/owned.png", "publish-once")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.CreatePublished(ctx, 42, "valid", "", "/uploads/video/owned.mp4", "/uploads/cover/owned.png", "publish-once")
	if err != nil || replay.Created || first.Video.ID != replay.Video.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
}
