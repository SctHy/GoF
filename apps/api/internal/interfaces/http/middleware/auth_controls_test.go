package interfaceshttpmiddleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainaccount "GCFeed/internal/domain/account"
	infracache "GCFeed/internal/infra/cache"
	infrajwt "GCFeed/internal/infra/jwt"
	"github.com/gin-gonic/gin"
)

type mutableAuthState struct {
	status int
	role   string
	err    error
}

func (s *mutableAuthState) AuthState(context.Context, int64) (int, string, error) {
	return s.status, s.role, s.err
}

func TestJWTAuthRefreshesAlreadyIssuedToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager, err := infrajwt.NewManager("test-secret", "15m")
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.SignAccessToken(1, "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, optional := range []bool{false, true} {
		state := &mutableAuthState{status: 1, role: "admin"}
		router := gin.New()
		auth := NewJWTAuth(manager, state)
		if optional {
			auth = NewOptionalJWTAuth(manager, state)
		}
		router.GET("/", auth, func(c *gin.Context) { c.String(200, c.GetString(ContextRoleKey)) })
		check := func(want int, role string) {
			t.Helper()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != want || (want == 200 && res.Body.String() != role) {
				t.Fatalf("optional=%v response %d %s", optional, res.Code, res.Body.String())
			}
		}
		check(200, "admin")
		state.role = "user"
		check(200, "user")
		state.status = 0
		check(401, "")
		state.status = 2
		check(401, "")
		state.err = domainaccount.ErrUserNotFound
		check(401, "")
		state.err = errors.New("database unavailable")
		check(503, "")
	}
}

type failedAuthLimiter struct{}

func (failedAuthLimiter) Allow(context.Context, string, string, int, time.Duration) (bool, time.Duration, error) {
	return false, 0, errors.New("unavailable")
}

func TestAuthRateLimitRunsBeforeHandlerAndIgnoresUntrustedForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	reached := 0
	router.POST("/login", NewAuthRateLimit(infracache.NewAuthRateLimiter(nil), "login", 2, time.Minute), func(c *gin.Context) { reached++; c.Status(204) })
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		req.Header.Set("X-Forwarded-For", []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}[i])
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		want := 204
		if i == 2 {
			want = 429
			if res.Header().Get("Retry-After") != "60" {
				t.Fatalf("retry after: %s", res.Header().Get("Retry-After"))
			}
		}
		if res.Code != want {
			t.Fatalf("request %d: %d", i, res.Code)
		}
	}
	if reached != 2 {
		t.Fatalf("blocked request reached bcrypt handler: %d", reached)
	}
	router.POST("/failure", NewAuthRateLimit(failedAuthLimiter{}, "login", 2, time.Minute), func(c *gin.Context) { t.Error("unavailable limiter reached handler") })
	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest("POST", "/failure", nil))
	if res.Code != 503 {
		t.Fatalf("failure: %d", res.Code)
	}
}
