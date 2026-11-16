package infraconfig

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

var ErrEmptyConfigPath = errors.New("config file path is empty")
var ErrReadConfigFailed = errors.New("read config file failed")
var ErrUnmarshalConfigFailed = errors.New("unmarshal config failed")

// LoadConfig 读取 YAML 配置文件，并反序列化为应用启动配置。
func LoadConfig(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, ErrEmptyConfigPath
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrReadConfigFailed
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(content, cfg); err != nil {
		return nil, ErrUnmarshalConfigFailed
	}

	// 环境变量用于本地和容器部署；未设置时保持 YAML 配置兼容。
	for key, target := range map[string]*string{
		"GCFEED_JWT_SECRET":        &cfg.JWT.Secret,
		"GCFEED_INTERNAL_TOKEN":    &cfg.Internal.Token,
		"GCFEED_DATABASE_HOST":     &cfg.Database.Host,
		"GCFEED_DATABASE_USER":     &cfg.Database.User,
		"GCFEED_DATABASE_PASSWORD": &cfg.Database.Password,
		"GCFEED_DATABASE_NAME":     &cfg.Database.Name,
		"GCFEED_REDIS_ADDR":        &cfg.Redis.Addr,
		"GCFEED_REDIS_PASSWORD":    &cfg.Redis.Password,
		"GCFEED_RABBITMQ_URL":      &cfg.RabbitMQ.URL,
	} {
		if value, ok := os.LookupEnv(key); ok {
			*target = value
		}
	}
	for key, target := range map[string]*int{
		"GCFEED_PORT":          &cfg.Port,
		"GCFEED_DATABASE_PORT": &cfg.Database.Port,
	} {
		if value, ok := os.LookupEnv(key); ok {
			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 {
				return nil, fmt.Errorf("invalid port in %s", key)
			}
			*target = port
		}
	}
	return cfg, nil
}
