package infracache

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestHotEventConcurrentDeduplicationAndHistoricalWindow(t *testing.T) {
	client := isolatedRedis(t)
	cache := NewFeedCache(client)
	ctx := context.Background()
	at := time.Now().UTC()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := cache.AddHotScoreOnce(ctx, "like", 42, 3, at); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	score, err := client.ZScore(ctx, hotMinuteKey(at), hotRankMember(42)).Result()
	if err != nil || score != 3 {
		t.Fatalf("duplicate score=%v err=%v", score, err)
	}
	if err := cache.AddHotScoreOnce(ctx, "unlike", 42, -3, at); err != nil {
		t.Fatal(err)
	}
	if err := cache.AddHotScoreOnce(ctx, "like", 42, 3, at); err != nil {
		t.Fatal(err)
	}
	score, err = client.ZScore(ctx, hotMinuteKey(at), hotRankMember(42)).Result()
	if err != nil || score != 0 {
		t.Fatalf("out of order replay score=%v err=%v", score, err)
	}
	expected := at.Truncate(time.Minute).Add(hotMinuteBucketTTL).UnixMilli()
	for _, key := range []string{hotMinuteKey(at), hotEventKey("like"), hotEventKey("unlike")} {
		actual, err := client.PExpireTime(ctx, key).Result()
		if err != nil || actual.Milliseconds() != expected {
			t.Fatalf("expiration %s=%v want=%v err=%v", key, actual, expected, err)
		}
	}
	old := at.Add(-3 * time.Hour)
	if err := cache.AddHotScoreOnce(ctx, "old", 42, 3, old); err != nil {
		t.Fatal(err)
	}
	if client.Exists(ctx, hotMinuteKey(old), hotEventKey("old")).Val() != 0 {
		t.Fatal("expired event resurrected hot bucket")
	}
	if err := cache.AddHotScoreOnce(ctx, "future", 42, 3, at.Add(10*time.Minute)); err == nil {
		t.Fatal("future event accepted")
	}
}
func TestHotEventFailureDoesNotBurnDeduplicationMarker(t *testing.T) {
	client := isolatedRedis(t)
	cache := NewFeedCache(client)
	ctx := context.Background()
	at := time.Now()
	if err := client.Set(ctx, hotMinuteKey(at), "wrong-type", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := cache.AddHotScoreOnce(ctx, "retry", 42, 3, at); err == nil {
		t.Fatal("invalid bucket type accepted")
	}
	if client.Exists(ctx, hotEventKey("retry")).Val() != 0 {
		t.Fatal("failed update marked delivered")
	}
	if err := client.Del(ctx, hotMinuteKey(at)).Err(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := cache.AddHotScoreOnce(ctx, "retry", 42, 3, at); err != nil {
			t.Fatal(err)
		}
	}
	if score := client.ZScore(ctx, hotMinuteKey(at), hotRankMember(42)).Val(); score != 3 {
		t.Fatalf("retry score=%v", score)
	}
}
