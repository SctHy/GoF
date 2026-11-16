package infraconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("port: 8080\ndatabase:\n  host: mysql\n  port: 3306\nredis:\n  password: old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GCFEED_PORT", "18080")
	t.Setenv("GCFEED_DATABASE_HOST", "127.0.0.1")
	t.Setenv("GCFEED_REDIS_PASSWORD", "")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 18080 || cfg.Database.Host != "127.0.0.1" || cfg.Database.Port != 3306 || cfg.Redis.Password != "" {
		t.Fatal("environment overrides or YAML fallback did not apply")
	}
	t.Setenv("GCFEED_PORT", "invalid")
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("invalid port must fail before application startup")
	}
}
