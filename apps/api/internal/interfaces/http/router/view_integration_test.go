package interfaceshttprouter_test

import (
	config "GCFeed/internal/infra/config"
	jwt "GCFeed/internal/infra/jwt"
	account "GCFeed/internal/infra/persistence/account"
	video "GCFeed/internal/infra/persistence/video"
	router "GCFeed/internal/interfaces/http/router"
	"GCFeed/internal/testutil"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestViewHTTPCommitsWithoutBrokerOrOutbox(t *testing.T) {
	db := testutil.Database(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// The API must start and accept durable writes even with an unusable MQ endpoint.
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "isolated-integration-secret", AccessTTL: "15m"}, RabbitMQ: config.RabbitMQConfig{URL: "amqp://invalid:invalid@127.0.0.1:1/"}}
	engine := gin.New()
	if err := router.Register(engine, cfg, sqlDB); err != nil {
		t.Fatalf("API still coupled to broker: %v", err)
	}
	ready := httptest.NewRecorder()
	engine.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	var status struct {
		Ready        bool            `json:"ready"`
		Dependencies map[string]bool `json:"dependencies"`
	}
	if err := json.Unmarshal(ready.Body.Bytes(), &status); err != nil || ready.Code != 200 || !status.Ready {
		t.Fatalf("readiness=%s", ready.Body.String())
	}
	if _, found := status.Dependencies["rabbitmq"]; found {
		t.Fatal("API readiness still depends on broker")
	}
	if err := db.Create(&account.UserModel{ID: 42, Account: "viewer", Password: "unused", Nickname: "viewer", Status: 1, Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Create(&video.VideoModel{ID: 1, AuthorID: 2, Title: "fixture", MediaURL: "/test", CoverURL: "/test", Status: 2, PublishedAt: &now}).Error; err != nil {
		t.Fatal(err)
	}
	manager, err := jwt.NewManager(cfg.JWT.Secret, cfg.JWT.AccessTTL)
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.SignAccessToken(42, "user")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/video-view-events", strings.NewReader(`{"video_id":1,"scene":"timeline","event_type":"complete","watch_ms":1000,"completed":true}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("view write=%d %s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.Table("video_view_events").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("view facts=%d err=%v", count, err)
	}
	if err := db.Table("event_outbox").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("view should not create messages: count=%d err=%v", count, err)
	}
}
