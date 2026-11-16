package infracache

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// 每次测试启动独立 Redis Unix socket，不连接开发数据库，也不清理共享 Redis。
func isolatedRedis(t *testing.T) *redis.Client {
	t.Helper()
	executable, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server unavailable: real Redis regression skipped")
	}
	// macOS 的 Unix socket 路径长度有限。
	dir, err := os.MkdirTemp("", "gcfeed-r-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "redis.sock")
	cmd := exec.Command(executable, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no")
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for client.Ping(ctx).Err() != nil {
		if ctx.Err() != nil {
			t.Fatal("isolated Redis failed to start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return client
}
