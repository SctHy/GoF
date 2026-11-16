package infracache

import (
	applicationfeed "GCFeed/internal/application/feed"
	domainfeed "GCFeed/internal/domain/feed"
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const hotWindowMinutes = 60
const hotMinuteBucketTTL = 2 * time.Hour
const hotWindowCacheTTL = 2 * time.Minute

type redisCacheClient interface {
	redis.Cmdable
	Pipeline() redis.Pipeliner
}

type redisStatCacheClient interface {
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
}

// FeedCache 使用 Redis 保存 Feed 查询结果。
type FeedCache struct {
	client redisCacheClient
}

// NewFeedCache 创建 Feed 结果缓存。
func NewFeedCache(client redisCacheClient) *FeedCache {
	return &FeedCache{client: client}
}

// GetPage 读取缓存中的轻量 Feed 页。
func (c *FeedCache) GetPage(ctx context.Context, key string) (*applicationfeed.FeedPage, bool, error) {
	content, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		inframetrics.ObserveCacheRead("page", 1, 0, nil)
		return nil, false, nil
	}
	if err != nil {
		inframetrics.ObserveCacheRead("page", 1, 0, err)
		return nil, false, err
	}

	var page applicationfeed.FeedPage
	if err := json.Unmarshal(content, &page); err != nil {
		inframetrics.ObserveCacheRead("page", 1, 0, err)
		return nil, false, err
	}
	inframetrics.ObserveCacheRead("page", 1, 1, nil)
	return &page, true, nil
}

// SetPage 写入轻量 Feed 页，并设置过期时间。
func (c *FeedCache) SetPage(ctx context.Context, key string, page *applicationfeed.FeedPage, ttl time.Duration) error {
	content, err := json.Marshal(page)
	if err != nil {
		inframetrics.ObserveCacheWrite("page", 1, err)
		return err
	}
	err = c.client.Set(ctx, key, content, ttl).Err()
	inframetrics.ObserveCacheWrite("page", 1, err)
	return err
}

// GetCards 批量读取视频卡片缓存。
func (c *FeedCache) GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	cards := map[int64]*domainfeed.FeedCard{}
	if len(videoIDs) == 0 {
		return cards, nil
	}

	values, err := c.client.MGet(ctx, cacheKeys(videoIDs, feedCardKey)...).Result()
	if err != nil {
		inframetrics.ObserveCacheRead("card", len(videoIDs), 0, err)
		return nil, err
	}
	for index, value := range values {
		content, ok := cacheValueBytes(value)
		if !ok {
			continue
		}
		var card domainfeed.FeedCard
		if err := json.Unmarshal(content, &card); err != nil {
			continue
		}
		if card.VideoID <= 0 {
			card.VideoID = videoIDs[index]
		}
		cards[card.VideoID] = &card
	}
	inframetrics.ObserveCacheRead("card", len(videoIDs), len(cards), nil)
	return cards, nil
}

// SetCards 批量写入视频卡片缓存。
func (c *FeedCache) SetCards(ctx context.Context, cards map[int64]*domainfeed.FeedCard, ttl time.Duration) error {
	pipe := c.client.Pipeline()
	queued := false

	for _, card := range cards {
		if card == nil || card.VideoID <= 0 {
			continue
		}
		content, err := json.Marshal(card)
		if err != nil {
			return err
		}
		pipe.Set(ctx, feedCardKey(card.VideoID), content, ttl)
		queued = true
	}
	if !queued {
		return nil
	}
	_, err := pipe.Exec(ctx)
	inframetrics.ObserveCacheWrite("card", len(cards), err)
	return err
}

// GetStats 批量读取视频计数缓存。
func (c *FeedCache) GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	return getStats(ctx, c.client, videoIDs)
}

func getStats(ctx context.Context, client redisStatCacheClient, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	stats := map[int64]*domainfeed.FeedStat{}
	if len(videoIDs) == 0 {
		return stats, nil
	}

	values, err := client.MGet(ctx, cacheKeys(videoIDs, feedStatKey)...).Result()
	if err != nil {
		inframetrics.ObserveCacheRead("stat", len(videoIDs), 0, err)
		return nil, err
	}
	for index, value := range values {
		content, ok := cacheValueBytes(value)
		if !ok {
			continue
		}
		var stat domainfeed.FeedStat
		if err := json.Unmarshal(content, &stat); err != nil {
			continue
		}
		if stat.VideoID <= 0 {
			stat.VideoID = videoIDs[index]
		}
		stats[stat.VideoID] = &stat
	}
	// JSON miss 统一由应用层批量回源数据库。

	inframetrics.ObserveCacheRead("stat", len(videoIDs), len(stats), nil)
	return stats, nil
}

// SetStats 批量写入视频计数缓存。
func (c *FeedCache) SetStats(ctx context.Context, stats map[int64]*domainfeed.FeedStat, ttl time.Duration) error {
	pipe := c.client.Pipeline()
	queued := false

	for _, stat := range stats {
		if stat == nil || stat.VideoID <= 0 {
			continue
		}
		content, err := json.Marshal(stat)
		if err != nil {
			return err
		}
		pipe.Set(ctx, feedStatKey(stat.VideoID), content, ttl)
		queued = true
	}
	if !queued {
		return nil
	}
	_, err := pipe.Exec(ctx)
	inframetrics.ObserveCacheWrite("stat", len(stats), err)
	return err
}

// AddHotScore 把一次互动热度写入 1 分钟粒度的热榜桶。
func (c *FeedCache) AddHotScore(ctx context.Context, videoID int64, scoreDelta int, at time.Time) error {
	if videoID <= 0 || scoreDelta == 0 {
		return nil
	}

	key := hotMinuteKey(at)
	if err := c.client.ZIncrBy(ctx, key, float64(scoreDelta), hotRankMember(videoID)).Err(); err != nil {
		return err
	}
	return c.client.Expire(ctx, key, hotMinuteBucketTTL).Err()
}

// ListHotWindowPage 合并最近 60 个分钟桶，返回一小时滑动窗口内的热榜页。
func (c *FeedCache) ListHotWindowPage(ctx context.Context, windowEnd time.Time, offset int, limit int) ([]*domainfeed.FeedPageItem, error) {
	items := []*domainfeed.FeedPageItem{}
	if limit <= 0 {
		return items, nil
	}
	if offset < 0 {
		offset = 0
	}

	windowEnd = windowEnd.UTC().Truncate(time.Minute)
	windowKey := hotWindowKey(windowEnd)
	exists, err := c.client.Exists(ctx, windowKey).Result()
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		if err := c.rebuildHotWindow(ctx, windowKey, windowEnd); err != nil {
			return nil, err
		}
	}

	return c.listHotWindowPage(ctx, windowKey, offset, limit)
}

func (c *FeedCache) rebuildHotWindow(ctx context.Context, windowKey string, windowEnd time.Time) error {
	if _, err := c.client.ZUnionStore(ctx, windowKey, &redis.ZStore{
		Keys:      hotWindowMinuteKeys(windowEnd),
		Aggregate: "SUM",
	}).Result(); err != nil {
		return err
	}

	pipe := c.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, windowKey, "-inf", "0")
	pipe.Expire(ctx, windowKey, hotWindowCacheTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *FeedCache) listHotWindowPage(ctx context.Context, windowKey string, offset int, limit int) ([]*domainfeed.FeedPageItem, error) {
	items := []*domainfeed.FeedPageItem{}
	values, err := c.client.ZRevRangeWithScores(ctx, windowKey, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		member, ok := value.Member.(string)
		if !ok {
			continue
		}
		videoID, ok := hotRankVideoID(member)
		if !ok {
			continue
		}
		items = append(items, &domainfeed.FeedPageItem{
			VideoID:  videoID,
			HotScore: int(value.Score),
		})
	}
	return items, nil
}

func cacheKeys(videoIDs []int64, build func(int64) string) []string {
	keys := make([]string, 0, len(videoIDs))
	for _, videoID := range videoIDs {
		keys = append(keys, build(videoID))
	}
	return keys
}

func feedCardKey(videoID int64) string {
	return fmt.Sprintf("video:card:v1:%d", videoID)
}

func feedStatKey(videoID int64) string {
	return fmt.Sprintf("video:stat:v1:%d", videoID)
}

func hotMinuteKey(at time.Time) string {
	return fmt.Sprintf("feed:hot:minute:v1:%s", at.UTC().Truncate(time.Minute).Format("200601021504"))
}

func hotWindowKey(windowEnd time.Time) string {
	return fmt.Sprintf("feed:hot:window:v1:%d", windowEnd.UTC().Truncate(time.Minute).Unix())
}

func hotWindowMinuteKeys(windowEnd time.Time) []string {
	keys := make([]string, 0, hotWindowMinutes)
	for index := hotWindowMinutes - 1; index >= 0; index-- {
		keys = append(keys, hotMinuteKey(windowEnd.Add(-time.Duration(index)*time.Minute)))
	}
	return keys
}

func hotRankMember(videoID int64) string {
	return fmt.Sprintf("%020d", videoID)
}

func hotRankVideoID(member string) (int64, bool) {
	value := strings.TrimLeft(member, "0")
	if value == "" {
		return 0, false
	}
	videoID, err := strconv.ParseInt(value, 10, 64)
	return videoID, err == nil && videoID > 0
}

func cacheValueBytes(value any) ([]byte, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		return []byte(typed), true
	case []byte:
		return typed, true
	default:
		return nil, false
	}
}

// InvalidateVideoStats removes the derived count cache.
// Authority remains in MySQL; misses are batch-loaded by the feed service.
func (c *FeedCache) InvalidateVideoStats(ctx context.Context, videoID int64) error {
	if videoID <= 0 {
		return nil
	}
	return c.client.Del(ctx, feedStatKey(videoID)).Err()
}
