package infracache

import (
	"context"
	"github.com/redis/go-redis/v9"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Actual local Redis cold-cache path only: no MySQL, network proxy, or HTTP load is included.
// Measures the current batch cache implementation with an isolated Redis instance.
func BenchmarkFeedStatsColdPage(b *testing.B) {
	executable, err := exec.LookPath("redis-server")
	if err != nil {
		b.Skip("requires redis-server")
	}
	dir, err := os.MkdirTemp("", "gcfeed-bench-")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "r.sock")
	command := exec.Command(executable, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no")
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if err := command.Start(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, MaxRetries: -1})
	b.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for client.Ping(ctx).Err() != nil {
		if ctx.Err() != nil {
			b.Fatal(ctx.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cache := NewFeedCache(client)
	ids := make([]int64, 20)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cache.GetStats(context.Background(), ids); err != nil {
			b.Fatal(err)
		}
	}
}
