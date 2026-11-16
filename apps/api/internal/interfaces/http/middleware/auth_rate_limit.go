package interfaceshttpmiddleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type AuthRateLimiter interface {
	Allow(context.Context, string, string, int, time.Duration) (bool, time.Duration, error)
}

// NewAuthRateLimit 必须放在登录/注册 handler 前；ClientIP 仅接受路由显式信任的代理。
func NewAuthRateLimit(limiter AuthRateLimiter, scope string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, retryAfter, err := limiter.Allow(c.Request.Context(), scope, c.ClientIP(), limit, window)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "authentication temporarily unavailable"})
			return
		}
		if !allowed {
			seconds := max(int64((retryAfter+time.Second-1)/time.Second), 1)
			c.Header("Retry-After", strconv.FormatInt(seconds, 10))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"message": "too many authentication requests"})
			return
		}
		c.Next()
	}
}
