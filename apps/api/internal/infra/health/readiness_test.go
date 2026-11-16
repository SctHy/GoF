package infrahealth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadinessReflectsDependenciesWithoutLeakingErrors(t *testing.T) {
	healthy := true
	handler := Readiness(Checks{
		"mysql": func(ctx context.Context) error { return nil },
		"rabbitmq": func(ctx context.Context) error {
			if !healthy {
				return errors.New("secret-password-and-private-host")
			}
			return nil
		},
	})
	for _, available := range []bool{true, false, true} {
		healthy = available
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
		expected := http.StatusOK
		if !available {
			expected = http.StatusServiceUnavailable
		}
		if response.Code != expected {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "secret") {
			t.Fatal("underlying dependency error leaked")
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("readiness must not be cached")
		}
	}
}

func TestReadinessHonorsCancellationAndSuppliesDeadline(t *testing.T) {
	called := false
	handler := Readiness(Checks{"database": func(ctx context.Context) error {
		called = true
		if _, ok := ctx.Deadline(); !ok {
			t.Error("missing dependency deadline")
		}
		return nil
	}})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ready", nil))
	if !called {
		t.Fatal("dependency was not checked")
	}
	called = false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(ctx))
	if response.Code != http.StatusServiceUnavailable || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
}
