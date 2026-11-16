package infracache

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuthRateLimiterRedisSharedConcurrentExpiryAndFailure(t *testing.T) {
	client := isolatedRedis(t)
	ctx := context.Background()
	first, second := NewAuthRateLimiter(client), NewAuthRateLimiter(client)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			limiter := first
			if i%2 == 0 {
				limiter = second
			}
			ok, ttl, err := limiter.Allow(ctx, "login", "192.0.2.1", 10, time.Minute)
			if err != nil {
				t.Error(err)
				return
			}
			if ttl <= 0 {
				t.Error("missing expiry")
			}
			if ok {
				allowed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("allowed %d, want 10", allowed.Load())
	}
	keys, err := client.Keys(ctx, "auth:rate:*").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || strings.Contains(keys[0], "192.0.2.1") {
		t.Fatalf("invalid keys: %v", keys)
	}
	if ok, _, err := second.Allow(ctx, "register", "192.0.2.1", 5, time.Minute); err != nil || !ok {
		t.Fatalf("separate scope: %v %v", ok, err)
	}
	// Shorten only the isolated test key; a real Redis TTL expiry must reset the window.
	if err := client.PExpire(ctx, keys[0], time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if ok, _, err := first.Allow(ctx, "login", "192.0.2.1", 10, time.Minute); err != nil || !ok {
		t.Fatalf("expiry: %v %v", ok, err)
	}
	_ = client.Close()
	if ok, _, err := second.Allow(ctx, "login", "192.0.2.1", 10, time.Minute); err == nil || ok {
		t.Fatal("Redis failure must fail closed")
	}
}

func TestAuthRateLimiterLocalBoundAndExpiry(t *testing.T) {
	limiter := NewAuthRateLimiter(nil)
	ctx := context.Background()
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := limiter.Allow(ctx, "login", "one", 10, time.Minute)
			if err != nil {
				t.Error(err)
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("allowed %d", allowed.Load())
	}
	for i := 1; i < maxLocalAuthKeys; i++ {
		if _, _, err := limiter.Allow(ctx, "login", fmt.Sprint(i), 10, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if ok, _, err := limiter.Allow(ctx, "login", "overflow", 10, time.Minute); ok || err == nil {
		t.Fatal("capacity must be bounded")
	}
	for k, v := range limiter.windows {
		v.expires = time.Now().Add(-time.Second)
		limiter.windows[k] = v
	}
	if ok, _, err := limiter.Allow(ctx, "login", "overflow", 10, time.Minute); !ok || err != nil {
		t.Fatal("expired slots not reclaimed")
	}
	if len(limiter.windows) != 1 {
		t.Fatalf("stale entries retained: %d", len(limiter.windows))
	}
}
