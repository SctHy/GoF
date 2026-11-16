package infrahttpgin

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustsOnlyConfiguredProxy(t *testing.T) {
	for _, tc := range []struct{ proxies, want string }{{"", "127.0.0.1"}, {"127.0.0.1/32", "203.0.113.7"}} {
		t.Setenv("GCFEED_TRUSTED_PROXIES", tc.proxies)
		g := Init()
		g.GET("/client", func(c *gin.Context) { c.String(200, c.ClientIP()) })
		req := httptest.NewRequest("GET", "/client", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("X-Forwarded-For", "203.0.113.7")
		response := httptest.NewRecorder()
		g.ServeHTTP(response, req)
		if response.Body.String() != tc.want {
			t.Fatalf("proxies=%q ip=%q", tc.proxies, response.Body.String())
		}
	}
}
