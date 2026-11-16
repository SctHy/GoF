package test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	applicationexposure "GCFeed/internal/application/exposure"
	domainexposure "GCFeed/internal/domain/exposure"
	infrajwt "GCFeed/internal/infra/jwt"
	interfaceshttpexposure "GCFeed/internal/interfaces/http/exposure"
	interfaceshttpmiddleware "GCFeed/internal/interfaces/http/middleware"

	"github.com/gin-gonic/gin"
)

type exposureAPIResponse struct {
	Event    exposureEventAPIResponse  `json:"event"`
	Exposure *exposureIndexAPIResponse `json:"exposure"`
}

type exposureEventAPIResponse struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	VideoID   int64     `json:"video_id"`
	Scene     string    `json:"scene"`
	RequestID string    `json:"request_id"`
	EventType string    `json:"event_type"`
	WatchMs   int       `json:"watch_ms"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

type exposureIndexAPIResponse struct {
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	FirstExposedAt time.Time `json:"first_exposed_at"`
	LastExposedAt  time.Time `json:"last_exposed_at"`
	ExposureCount  int       `json:"exposure_count"`
	LastScene      string    `json:"last_scene"`
}

// memoryExposureRepo 是曝光测试用内存仓储，模拟观看流水和曝光聚合索引。
type memoryExposureRepo struct {
	mu        sync.Mutex
	nextID    int64
	published map[int64]bool
	events    []*domainexposure.ViewEvent
	exposures map[string]*domainexposure.Exposure
}

func newMemoryExposureRepo() *memoryExposureRepo {
	return &memoryExposureRepo{
		nextID:    1,
		published: map[int64]bool{1001: true, 1002: true},
		events:    []*domainexposure.ViewEvent{},
		exposures: map[string]*domainexposure.Exposure{},
	}
}

// SaveViewEvent 模拟写入观看流水，并在 exposed 事件时维护聚合索引。
func (r *memoryExposureRepo) SaveViewEvent(ctx context.Context, event *domainexposure.ViewEvent) (*domainexposure.ViewEvent, *domainexposure.Exposure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.published[event.VideoID] {
		return nil, nil, domainexposure.ErrVideoNotFound
	}

	now := time.Now()
	saved := cloneExposureViewEvent(event)
	saved.ID = r.nextID
	r.nextID++
	saved.CreatedAt = now
	r.events = append(r.events, cloneExposureViewEvent(saved))

	if !saved.CountsAsExposure() {
		return cloneExposureViewEvent(saved), nil, nil
	}

	key := memoryExposureKey(saved.UserID, saved.VideoID)
	exposure, exists := r.exposures[key]
	if !exists {
		exposure = &domainexposure.Exposure{
			ID:             int64(len(r.exposures) + 1),
			UserID:         saved.UserID,
			VideoID:        saved.VideoID,
			FirstExposedAt: saved.CreatedAt,
			LastExposedAt:  saved.CreatedAt,
			ExposureCount:  1,
			LastScene:      saved.Scene,
			CreatedAt:      saved.CreatedAt,
			UpdatedAt:      saved.CreatedAt,
		}
		r.exposures[key] = exposure
		return cloneExposureViewEvent(saved), cloneExposure(exposure), nil
	}

	exposure.LastExposedAt = saved.CreatedAt
	exposure.ExposureCount++
	exposure.LastScene = saved.Scene
	exposure.UpdatedAt = saved.CreatedAt
	return cloneExposureViewEvent(saved), cloneExposure(exposure), nil
}

func (r *memoryExposureRepo) EventCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// TestExposureAPIFlow 覆盖首次曝光、重复曝光聚合和普通观看事件。
func TestExposureAPIFlow(t *testing.T) {
	router, jwtManager, repo := newExposureRouter(t)
	token := signTestToken(t, jwtManager, 42)

	firstResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/api/video-view-events",
		`{"video_id":1001,"scene":"timeline","request_id":"req-1","event_type":"exposed","watch_ms":0,"completed":false}`,
		token,
	)
	requireStatus(t, firstResponse, http.StatusCreated)

	var first exposureAPIResponse
	decodeJSON(t, firstResponse, &first)
	if first.Event.ID == 0 || first.Event.UserID != 42 || first.Event.Scene != "timeline" || first.Event.EventType != domainexposure.EventTypeExposed {
		t.Fatalf("unexpected first exposure response: %+v", first)
	}
	if first.Exposure == nil || first.Exposure.ExposureCount != 1 || first.Exposure.LastScene != "timeline" {
		t.Fatalf("unexpected first exposure index: %+v", first.Exposure)
	}

	secondResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/api/video-view-events",
		`{"video_id":1001,"scene":"hot","request_id":"req-2","event_type":"exposed","watch_ms":800,"completed":false}`,
		token,
	)
	requireStatus(t, secondResponse, http.StatusCreated)

	var second exposureAPIResponse
	decodeJSON(t, secondResponse, &second)
	if second.Exposure == nil || second.Exposure.ExposureCount != 2 || second.Exposure.LastScene != "hot" {
		t.Fatalf("unexpected repeated exposure index: %+v", second.Exposure)
	}
	if !second.Exposure.FirstExposedAt.Equal(first.Exposure.FirstExposedAt) {
		t.Fatalf("first exposed time changed: first=%s second=%s", first.Exposure.FirstExposedAt, second.Exposure.FirstExposedAt)
	}

	completeResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/api/video-view-events",
		`{"video_id":1001,"scene":"recommend","request_id":"req-3","event_type":"complete","watch_ms":12000,"completed":false}`,
		token,
	)
	requireStatus(t, completeResponse, http.StatusCreated)

	var completed exposureAPIResponse
	decodeJSON(t, completeResponse, &completed)
	if completed.Exposure != nil || !completed.Event.Completed || completed.Event.EventType != domainexposure.EventTypeComplete {
		t.Fatalf("unexpected complete response: %+v", completed)
	}
	if repo.EventCount() != 3 {
		t.Fatalf("unexpected event count: %d", repo.EventCount())
	}
}

// TestExposurePublishesViewEvent 覆盖观看行为落库后发布画像更新事件。
// TestExposureAPIValidation 覆盖鉴权、参数错误和视频可见性校验。
func TestExposureAPIValidation(t *testing.T) {
	router, jwtManager, _ := newExposureRouter(t)
	token := signTestToken(t, jwtManager, 42)

	unauthorizedResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/api/video-view-events",
		`{"video_id":1001,"scene":"timeline","event_type":"exposed"}`,
		"",
	)
	requireStatus(t, unauthorizedResponse, http.StatusUnauthorized)

	emptySceneResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/api/video-view-events",
		`{"video_id":1001,"scene":" ","event_type":"exposed"}`,
		token,
	)
	requireStatus(t, emptySceneResponse, http.StatusBadRequest)

	badEventResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/api/video-view-events",
		`{"video_id":1001,"scene":"timeline","event_type":"progress"}`,
		token,
	)
	requireStatus(t, badEventResponse, http.StatusBadRequest)

	negativeWatchResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/api/video-view-events",
		`{"video_id":1001,"scene":"timeline","event_type":"play","watch_ms":-1}`,
		token,
	)
	requireStatus(t, negativeWatchResponse, http.StatusBadRequest)

	missingVideoResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/api/video-view-events",
		`{"video_id":404,"scene":"timeline","event_type":"exposed"}`,
		token,
	)
	requireStatus(t, missingVideoResponse, http.StatusNotFound)
}

func newExposureRouter(t *testing.T) (*gin.Engine, *infrajwt.Manager, *memoryExposureRepo) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtManager, err := infrajwt.NewManager("test-secret", "15m")
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}

	repo := newMemoryExposureRepo()
	service := applicationexposure.New(repo)
	handler := interfaceshttpexposure.New(service)
	authMiddleware := interfaceshttpmiddleware.NewJWTAuth(jwtManager)

	api := router.Group("/api")
	api.POST("/video-view-events", authMiddleware, handler.CreateViewEvent)

	return router, jwtManager, repo
}

func cloneExposureViewEvent(event *domainexposure.ViewEvent) *domainexposure.ViewEvent {
	cloned := *event
	return &cloned
}

func cloneExposure(exposure *domainexposure.Exposure) *domainexposure.Exposure {
	cloned := *exposure
	return &cloned
}

func memoryExposureKey(userID int64, videoID int64) string {
	return fmt.Sprintf("%d:%d", userID, videoID)
}
