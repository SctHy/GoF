package infracache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// 计数和首次设置过期时间必须原子执行，超限请求不会延长窗口。
var authRateScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return {count, redis.call('PTTL', KEYS[1])}
`)

const maxLocalAuthKeys = 4096

type authWindow struct {
	count   int
	expires time.Time
}

// AuthRateLimiter 使用 Redis 共享窗口；未配置 Redis 的单进程开发环境使用有界内存。
// Redis 已配置但发生故障时返回错误，不切换本地计数绕开共享限额。
type AuthRateLimiter struct {
	client  *redis.Client
	mu      sync.Mutex
	windows map[string]authWindow
}

func NewAuthRateLimiter(client *redis.Client) *AuthRateLimiter {
	return &AuthRateLimiter{client: client, windows: make(map[string]authWindow)}
}

func (l *AuthRateLimiter) Allow(ctx context.Context, scope, ip string, limit int, window time.Duration) (bool, time.Duration, error) {
	digest := sha256.Sum256([]byte(ip))
	key := "auth:rate:" + scope + ":" + hex.EncodeToString(digest[:])
	if l.client != nil {
		result, err := authRateScript.Run(ctx, l.client, []string{key}, window.Milliseconds()).Int64Slice()
		if err != nil {
			return false, 0, err
		}
		return result[0] <= int64(limit), time.Duration(result[1]) * time.Millisecond, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	value, exists := l.windows[key]
	if !exists || !now.Before(value.expires) {
		if !exists && len(l.windows) >= maxLocalAuthKeys {
			for k, v := range l.windows {
				if !now.Before(v.expires) {
					delete(l.windows, k)
				}
			}
			if len(l.windows) >= maxLocalAuthKeys {
				return false, 0, errors.New("local authentication limiter capacity reached")
			}
		}
		value = authWindow{expires: now.Add(window)}
	}
	value.count++
	l.windows[key] = value
	return value.count <= limit, value.expires.Sub(now), nil
}
