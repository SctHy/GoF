package infracache

import (
	domainfeed "GCFeed/internal/domain/feed"
	"context"
	"testing"
	"time"
)

func TestStatsCacheBatchReadsAndInvalidation(t *testing.T) {
	client := isolatedRedis(t)
	cache := NewFeedCache(client)
	ctx := context.Background()
	if err := cache.SetStats(ctx, map[int64]*domainfeed.FeedStat{1005: {VideoID: 1005, LikeCount: 2, CommentCount: 3, FavoriteCount: 1}}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := client.Set(ctx, feedStatKey(1006), "invalid-json", 0).Err(); err != nil {
		t.Fatal(err)
	}
	stats, err := cache.GetStats(ctx, []int64{1005, 1006, 1007})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[1005].LikeCount != 2 || stats[1005].CommentCount != 3 || stats[1005].FavoriteCount != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	if err := cache.InvalidateVideoStats(ctx, 1005); err != nil {
		t.Fatal(err)
	}
	stats, err = cache.GetStats(ctx, []int64{1005})
	if err != nil || len(stats) != 0 {
		t.Fatalf("invalidated stats=%+v err=%v", stats, err)
	}
}
