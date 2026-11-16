package applicationvideo

import (
	domainfeed "GCFeed/internal/domain/feed"
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"time"
)

type PublishedEventConsumer interface {
	ConsumeVideoPublished(context.Context, func(context.Context, *PublishedEvent) error) error
}
type FeedPreheatSource interface {
	BatchGetFeedCards(context.Context, []int64) (map[int64]*domainfeed.FeedCard, error)
	BatchGetFeedStats(context.Context, []int64) (map[int64]*domainfeed.FeedStat, error)
}
type FeedPreheater interface {
	PreheatFeedVideo(context.Context, int64, time.Duration) error
}
type FeedCacheWriter interface {
	SetCards(context.Context, map[int64]*domainfeed.FeedCard, time.Duration) error
	SetStats(context.Context, map[int64]*domainfeed.FeedStat, time.Duration) error
}

// PreheatWorker warms published video cards and counts; following pages read SQL directly.
type PreheatWorker struct {
	consumer  PublishedEventConsumer
	preheater FeedPreheater
}

func NewPreheatWorker(consumer PublishedEventConsumer, preheater FeedPreheater) *PreheatWorker {
	return &PreheatWorker{consumer: consumer, preheater: preheater}
}
func (w *PreheatWorker) Start(ctx context.Context) error {
	return w.consumer.ConsumeVideoPublished(ctx, w.HandleVideoPublished)
}
func (w *PreheatWorker) HandleVideoPublished(ctx context.Context, event *PublishedEvent) (err error) {
	start := time.Now()
	defer func() { inframetrics.ObserveWorkerJob("video_preheat", time.Since(start), err) }()
	if event == nil || event.VideoID <= 0 {
		return nil
	}
	return w.preheater.PreheatFeedVideo(ctx, event.VideoID, 15*time.Minute)
}

type FeedPreheaterAdapter struct {
	source FeedPreheatSource
	cache  FeedCacheWriter
}

func NewFeedPreheater(source FeedPreheatSource, cache FeedCacheWriter) *FeedPreheaterAdapter {
	return &FeedPreheaterAdapter{source: source, cache: cache}
}

func (p *FeedPreheaterAdapter) PreheatFeedVideo(ctx context.Context, videoID int64, ttl time.Duration) error {
	cards, err := p.source.BatchGetFeedCards(ctx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(cards) > 0 {
		if err := p.cache.SetCards(ctx, cards, ttl); err != nil {
			return err
		}
	}
	stats, err := p.source.BatchGetFeedStats(ctx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(stats) > 0 {
		return p.cache.SetStats(ctx, stats, 15*time.Second)
	}
	return nil
}
