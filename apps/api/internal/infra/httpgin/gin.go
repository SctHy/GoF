package infrahttpgin

import (
	infraconfig "GCFeed/internal/infra/config"
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// Init 创建 Gin 引擎，并使用默认日志和恢复中间件。
func Init() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	g := gin.Default()
	// 仅显式信任的反向代理可提供客户端 IP；默认忽略伪造 X-Forwarded-For。
	var proxies []string
	if raw := strings.TrimSpace(os.Getenv("GCFEED_TRUSTED_PROXIES")); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			proxies = append(proxies, strings.TrimSpace(value))
		}
	}
	if err := g.SetTrustedProxies(proxies); err != nil {
		log.Fatalf("invalid GCFEED_TRUSTED_PROXIES: %v", err)
	}
	g.Use(inframetrics.HTTPMiddleware())
	return g
}

// Run 根据配置端口启动 HTTP 服务。
func Run(cfg *infraconfig.Config, g *gin.Engine) error {
	port := cfg.Port
	addr := ":" + strconv.Itoa(port)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: addr, Handler: g, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
	}()
	err := server.ListenAndServe()
	stop()
	<-stopped
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
