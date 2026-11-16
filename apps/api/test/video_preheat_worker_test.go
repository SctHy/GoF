package test

import (
	applicationvideo "GCFeed/internal/application/video"
	"context"
	"errors"
	"testing"
	"time"
)

type memoryFeedPreheater struct {
	videoID int64
	failure error
}

func (p *memoryFeedPreheater) PreheatFeedVideo(ctx context.Context, videoID int64, ttl time.Duration) error {
	p.videoID = videoID
	return p.failure
}
func TestPublishedVideoPreheatsCacheAndPropagatesRetry(t *testing.T) {
	preheater := &memoryFeedPreheater{failure: errors.New("cache unavailable")}
	worker := applicationvideo.NewPreheatWorker(nil, preheater)
	event := &applicationvideo.PublishedEvent{VideoID: 99, AuthorID: 7}
	if err := worker.HandleVideoPublished(context.Background(), event); !errors.Is(err, preheater.failure) {
		t.Fatalf("lost retry error: %v", err)
	}
	preheater.failure = nil
	if err := worker.HandleVideoPublished(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if preheater.videoID != 99 {
		t.Fatalf("preheated video=%d", preheater.videoID)
	}
}
