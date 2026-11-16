package infrahealth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Checks 只返回依赖名称及状态，不把数据库地址、凭据或底层错误暴露到公开探针。
type Checks map[string]func(context.Context) error

func Readiness(checks Checks) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		states := make(map[string]bool, len(checks))
		ready := true
		for name, check := range checks {
			ok := ctx.Err() == nil && check(ctx) == nil
			states[name] = ok
			ready = ready && ok
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(struct {
			Ready        bool            `json:"ready"`
			Dependencies map[string]bool `json:"dependencies"`
		}{ready, states})
	})
}
