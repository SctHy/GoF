package interfaceshttprouter

import (
	applicationaccount "GCFeed/internal/application/account"
	applicationexposure "GCFeed/internal/application/exposure"
	applicationfeed "GCFeed/internal/application/feed"
	applicationinteraction "GCFeed/internal/application/interaction"
	applicationmessage "GCFeed/internal/application/message"
	applicationplayback "GCFeed/internal/application/playback"
	applicationrecommendation "GCFeed/internal/application/recommendation"
	applicationrelation "GCFeed/internal/application/relation"
	applicationvideo "GCFeed/internal/application/video"
	infracache "GCFeed/internal/infra/cache"
	infraconfig "GCFeed/internal/infra/config"
	infrahealth "GCFeed/internal/infra/health"
	infrajwt "GCFeed/internal/infra/jwt"
	infraaccount "GCFeed/internal/infra/persistence/account"
	infraasset "GCFeed/internal/infra/persistence/asset"
	infraexposure "GCFeed/internal/infra/persistence/exposure"
	infrafeed "GCFeed/internal/infra/persistence/feed"
	infrainteraction "GCFeed/internal/infra/persistence/interaction"
	inframessage "GCFeed/internal/infra/persistence/message"
	migration "GCFeed/internal/infra/persistence/migration"
	infraplayback "GCFeed/internal/infra/persistence/playback"
	infrarecommendation "GCFeed/internal/infra/persistence/recommendation"
	infrarelation "GCFeed/internal/infra/persistence/relation"
	infravideo "GCFeed/internal/infra/persistence/video"
	interfaceshttpaccount "GCFeed/internal/interfaces/http/account"
	interfaceshttpexposure "GCFeed/internal/interfaces/http/exposure"
	interfaceshttpfeed "GCFeed/internal/interfaces/http/feed"
	interfaceshttpinteraction "GCFeed/internal/interfaces/http/interaction"
	interfaceshttpmessage "GCFeed/internal/interfaces/http/message"
	interfaceshttpmiddleware "GCFeed/internal/interfaces/http/middleware"
	interfaceshttpplayback "GCFeed/internal/interfaces/http/playback"
	interfaceshttprecommendation "GCFeed/internal/interfaces/http/recommendation"
	interfaceshttprelation "GCFeed/internal/interfaces/http/relation"
	interfaceshttpupload "GCFeed/internal/interfaces/http/upload"
	interfaceshttpvideo "GCFeed/internal/interfaces/http/video"
	"context"
	"database/sql"
	"github.com/redis/go-redis/v9"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Register(g *gin.Engine, cfg *infraconfig.Config, db *sql.DB) error {
	// database/sql 连接池交给 GORM 复用，避免维护两套数据库连接。
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn: db,
	}), &gorm.Config{})
	if err != nil {
		return err
	}

	// 执行缺失的版本化迁移，或按部署配置仅检查版本与校验和。
	if err := migration.Ensure(gormDB); err != nil {
		return err
	}

	// JWT Manager 同时被账号服务用于签发 token，也被鉴权中间件用于校验 token。
	jwtManager, err := infrajwt.NewManager(cfg.JWT.Secret, cfg.JWT.AccessTTL)
	if err != nil {
		return err
	}

	accountRepo := infraaccount.New(gormDB)
	accountService := applicationaccount.New(accountRepo, jwtManager)
	accountHandler := interfaceshttpaccount.New(accountService)
	videoRepo := infravideo.New(gormDB)
	feedRepo := infrafeed.New(gormDB)
	recommendationRepo := infrarecommendation.New(gormDB)
	feedOptions := []applicationfeed.Option{}
	interactionOptions := []applicationinteraction.Option{}
	exposureOptions := []applicationexposure.Option{}
	var feedCache *infracache.FeedCache
	var redisClient *redis.Client
	readyChecks := infrahealth.Checks{"mysql": db.PingContext}
	if cfg.Redis.Addr != "" {
		redisClient = infracache.NewRedisClient(cfg.Redis)
		readyChecks["redis"] = func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }
		feedCache = infracache.NewFeedCache(redisClient)
		feedOptions = append(feedOptions, applicationfeed.WithFeedCache(feedCache))
		interactionOptions = append(interactionOptions, applicationinteraction.WithHotScoreRecorder(feedCache))
		interactionOptions = append(interactionOptions, applicationinteraction.WithStatCache(feedCache))
	}
	recommendationOptions := []applicationrecommendation.Option{}
	if feedCache != nil {
		recommendationOptions = append(recommendationOptions, applicationrecommendation.WithSnapshotStore(feedCache))
	}
	recommendationService := applicationrecommendation.New(recommendationRepo, recommendationOptions...)
	recommendationHandler := interfaceshttprecommendation.New(recommendationService)
	feedOptions = append(feedOptions, applicationfeed.WithRecommender(recommendationService))
	exposureOptions = append(exposureOptions, applicationexposure.WithInterestInvalidator(recommendationService))
	feedService := applicationfeed.New(feedRepo, feedOptions...)
	feedHandler := interfaceshttpfeed.New(feedService)
	interactionRepo := infrainteraction.New(gormDB)
	messageRepo := inframessage.New(gormDB)
	messageService := applicationmessage.New(messageRepo)
	messageHandler := interfaceshttpmessage.New(messageService)
	playbackRepo := infraplayback.New(gormDB)
	playbackService := applicationplayback.New(playbackRepo)
	playbackHandler := interfaceshttpplayback.New(playbackService)
	// 视频和互动事件由数据库 Outbox 接力，API 不连接或等待 RabbitMQ。
	messageWriter := messageService
	interactionOptions = append(interactionOptions, applicationinteraction.WithMessageWriter(messageWriter))
	relationOptions := []applicationrelation.Option{applicationrelation.WithMessageWriter(messageWriter)}
	videoService := applicationvideo.New(videoRepo)
	videoHandler := interfaceshttpvideo.New(videoService)
	interactionService := applicationinteraction.New(interactionRepo, interactionOptions...)
	interactionHandler := interfaceshttpinteraction.New(interactionService)
	exposureRepo := infraexposure.New(gormDB)
	exposureService := applicationexposure.New(exposureRepo, exposureOptions...)
	exposureHandler := interfaceshttpexposure.New(exposureService)
	relationRepo := infrarelation.New(gormDB)
	relationService := applicationrelation.New(relationRepo, relationOptions...)
	relationHandler := interfaceshttprelation.New(relationService)
	uploadHandler := interfaceshttpupload.New("./uploads", infraasset.New(gormDB))

	g.GET("/health", HealthCheck)
	g.GET("/ready", gin.WrapH(infrahealth.Readiness(readyChecks)))
	g.GET("/metrics", gin.WrapH(promhttp.Handler()))
	// 静态文件路由让上传后的文件可以通过 /uploads/... 访问。
	g.Static("/uploads", "./uploads")

	authMiddleware := interfaceshttpmiddleware.NewJWTAuth(jwtManager, accountRepo)
	optionalAuthMiddleware := interfaceshttpmiddleware.NewOptionalJWTAuth(jwtManager, accountRepo)
	api := g.Group("/api")
	authLimiter := infracache.NewAuthRateLimiter(redisClient)

	sessions := api.Group("/sessions")
	sessions.POST("", interfaceshttpmiddleware.NewAuthRateLimit(authLimiter, "login", 10, time.Minute), accountHandler.Login)
	sessions.DELETE("/current", authMiddleware, accountHandler.Logout)

	users := api.Group("/users")
	users.POST("", interfaceshttpmiddleware.NewAuthRateLimit(authLimiter, "register", 5, time.Minute), accountHandler.Register)
	users.GET("/me", authMiddleware, accountHandler.Me)
	users.PATCH("/me", authMiddleware, accountHandler.UpdateMe)
	users.GET("/me/videos", authMiddleware, videoHandler.ListMine)
	users.PUT("/me/following/:targetUserId", authMiddleware, relationHandler.Follow)
	users.DELETE("/me/following/:targetUserId", authMiddleware, relationHandler.Unfollow)
	users.GET("/me/following", authMiddleware, relationHandler.ListFollowing)
	users.GET("/me/followers", authMiddleware, relationHandler.ListFollowers)
	users.GET("/:userId", accountHandler.Get)
	users.GET("/:userId/videos", videoHandler.ListByAuthor)

	videos := api.Group("/videos")
	videos.POST("", authMiddleware, videoHandler.Create)
	videos.GET("/:videoId", videoHandler.Get)
	videos.DELETE("/:videoId", authMiddleware, videoHandler.Delete)
	videos.PUT("/:videoId/like", authMiddleware, interactionHandler.Like)
	videos.DELETE("/:videoId/like", authMiddleware, interactionHandler.Unlike)
	videos.PUT("/:videoId/favorite", authMiddleware, interactionHandler.Favorite)
	videos.DELETE("/:videoId/favorite", authMiddleware, interactionHandler.Unfavorite)
	videos.POST("/:videoId/comments", authMiddleware, interactionHandler.CreateComment)
	videos.GET("/:videoId/comments", interactionHandler.ListComments)

	uploads := api.Group("/uploads", authMiddleware)
	uploads.POST("", uploadHandler.Create)

	// Feed 暴露为条目集合，客户端通过游标和 limit 控制分页。
	api.GET("/feed-items", optionalAuthMiddleware, feedHandler.ListFeedItems)
	api.POST("/feed-queries", optionalAuthMiddleware, feedHandler.Query)
	api.POST("/video-view-events", authMiddleware, exposureHandler.CreateViewEvent)
	// 删除评论只需要评论自身 ID，所以放在顶层 comments 资源下。
	api.DELETE("/comments/:commentId", authMiddleware, interactionHandler.DeleteComment)
	api.GET("/messages", authMiddleware, messageHandler.List)
	api.PATCH("/messages", authMiddleware, messageHandler.MarkRead)
	api.GET("/message-stats/unread", authMiddleware, messageHandler.CountUnread)
	api.GET("/playback-config", authMiddleware, playbackHandler.GetConfig)
	api.GET("/preload-videos", authMiddleware, playbackHandler.ListPreloadVideos)
	api.POST("/playback-qos-reports", authMiddleware, playbackHandler.CreateQoSReport)

	internal := g.Group("/internal", interfaceshttpmiddleware.NewInternalTokenAuth(cfg.Internal.Token))
	internal.POST("/recommendation-candidates", recommendationHandler.ListCandidates)
	internal.POST("/exposure-decisions", recommendationHandler.DecideExposures)
	internal.POST("/exposures", recommendationHandler.SaveExposures)
	internal.POST("/messages", messageHandler.Create)
	internal.POST("/playback-qos-reports", playbackHandler.CreateInternalQoSReport)

	return nil
}

// HealthCheck 提供基础健康检查接口，方便本地调试和容器探活。
func HealthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "All is well",
	})
}
