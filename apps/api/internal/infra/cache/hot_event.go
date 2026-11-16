package infracache

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

var addHotEventScript = redis.NewScript(`
local clock=redis.call('TIME')
local now=tonumber(clock[1])*1000+math.floor(tonumber(clock[2])/1000)
local expires=tonumber(ARGV[3])
if expires<=now then return 0 end
if tonumber(ARGV[4])>now+60000 then return redis.error_reply('hot event is in the future') end
if redis.call('EXISTS',KEYS[2])==1 then return 0 end
-- Validate key types before any write: Lua runtime errors do not roll back prior writes.
local bucketType=redis.call('TYPE',KEYS[1]).ok
if bucketType~='none' and bucketType~='zset' then return redis.error_reply('invalid hot bucket type') end
redis.call('ZINCRBY',KEYS[1],ARGV[1],ARGV[2])
redis.call('PEXPIREAT',KEYS[1],expires)
redis.call('SET',KEYS[2],'1','PXAT',expires)
return 1
`)

func hotEventKey(eventID string) string {
	return fmt.Sprintf("feed:hot:event:v1:%x", sha256.Sum256([]byte(eventID)))
}

// AddHotScoreOnce 原子执行去重和计分。绝对到期时间防止延迟重放复活历史热度。
// 去重与桶同属可重建 Redis 状态，不承诺 Redis 数据丢失后的历史热度恢复。
func (c *FeedCache) AddHotScoreOnce(ctx context.Context, eventID string, videoID int64, delta int, at time.Time) error {
	if eventID == "" || len(eventID) > 128 || videoID <= 0 || at.IsZero() {
		return errors.New("invalid hot event")
	}
	if delta == 0 {
		return nil
	}
	expires := at.UTC().Truncate(time.Minute).Add(hotMinuteBucketTTL)
	return addHotEventScript.Run(ctx, c.client, []string{hotMinuteKey(at), hotEventKey(eventID)}, delta, hotRankMember(videoID), expires.UnixMilli(), at.UnixMilli()).Err()
}
