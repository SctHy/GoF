package infracache

import (
	"GCFeed/internal/application/pagination"
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"time"
)

// A shared cap protects Redis from unbounded per-user snapshots. TTL bounds freshness;
// evicted/expired cursors fail explicitly instead of silently reranking the next page.
const maxSharedBlobs = 1024
const blobIndexKey = "feed:blob:index:v1"
const blobKeyPrefix = "feed:blob:v1:"

var writeBlobScript = redis.NewScript(`
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000+math.floor(tonumber(clock[2])/1000)
local expired = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', now)
for _, key in ipairs(expired) do redis.call('DEL',key); redis.call('ZREM',KEYS[1],key) end
redis.call('SET',KEYS[2],ARGV[2],'PX',ARGV[1])
redis.call('ZADD',KEYS[1],now+ARGV[1],KEYS[2])
local overflow = redis.call('ZCARD',KEYS[1])-tonumber(ARGV[3])
if overflow>0 then
  local victims=redis.call('ZRANGE',KEYS[1],0,overflow-1)
  for _, key in ipairs(victims) do redis.call('DEL',key); redis.call('ZREM',KEYS[1],key) end
end
redis.call('PEXPIRE',KEYS[1],ARGV[4])
return 1
`)

func (c *FeedCache) ReadBlob(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := c.client.Get(ctx, blobKeyPrefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	return value, err == nil, err
}
func (c *FeedCache) WriteBlob(ctx context.Context, key string, content []byte, ttl time.Duration) error {
	if len(content) > pagination.MaxBlobBytes || ttl <= 0 || ttl > pagination.SnapshotTTL {
		return errors.New("invalid snapshot size or TTL")
	}
	return writeBlobScript.Run(ctx, c.client, []string{blobIndexKey, blobKeyPrefix + key}, ttl.Milliseconds(), content, maxSharedBlobs, (pagination.SnapshotTTL + time.Minute).Milliseconds()).Err()
}
func (c *FeedCache) DeleteBlob(ctx context.Context, key string) error {
	pipe := c.client.Pipeline()
	pipe.Del(ctx, blobKeyPrefix+key)
	pipe.ZRem(ctx, blobIndexKey, blobKeyPrefix+key)
	_, err := pipe.Exec(ctx)
	return err
}
