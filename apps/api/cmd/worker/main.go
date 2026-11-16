package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"

	applicationembedding "GCFeed/internal/application/embedding"
	applicationinteraction "GCFeed/internal/application/interaction"
	applicationoutbox "GCFeed/internal/application/outbox"
	applicationvideo "GCFeed/internal/application/video"
	infracache "GCFeed/internal/infra/cache"
	infraconfig "GCFeed/internal/infra/config"
	infradatabase "GCFeed/internal/infra/database"
	infrahealth "GCFeed/internal/infra/health"
	inframetrics "GCFeed/internal/infra/metrics"
	inframq "GCFeed/internal/infra/mq"
	infraembedding "GCFeed/internal/infra/persistence/embedding"
	infrafeed "GCFeed/internal/infra/persistence/feed"
	infrainteraction "GCFeed/internal/infra/persistence/interaction"
	migration "GCFeed/internal/infra/persistence/migration"
	infraoutbox "GCFeed/internal/infra/persistence/outbox"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const configPath = "./configs/config.yaml"

func main() {
	cfg, err := infraconfig.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	if cfg.RabbitMQ.URL == "" {
		log.Fatal("rabbitmq url is required for worker")
	}
	if cfg.Redis.Addr == "" {
		log.Fatal("redis addr is required for worker")
	}

	sqlDB, err := infradatabase.New(cfg.Database)
	if err != nil {
		log.Fatalf("init database failed: %v", err)
	}
	defer closeSQL(sqlDB)
	inframetrics.RegisterDatabase(sqlDB)

	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		log.Fatalf("init gorm failed: %v", err)
	}
	if err := migration.Ensure(gormDB); err != nil {
		log.Fatalf("auto migrate failed: %v", err)
	}

	rabbitMQ, err := inframq.NewRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		log.Fatalf("init rabbitmq failed: %v", err)
	}
	defer closeRabbitMQ(rabbitMQ)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	redisClient := infracache.NewRedisClient(cfg.Redis)
	defer redisClient.Close()
	feedCache := infracache.NewFeedCache(redisClient)
	if err := startWorkers(ctx, gormDB, rabbitMQ, feedCache); err != nil {
		log.Fatalf("start workers failed: %v", err)
	}
	outboxRepo := infraoutbox.New(gormDB)
	go inframetrics.RunOutboxMonitor(ctx, outboxRepo)
	relay := applicationoutbox.New(outboxRepo, rabbitMQ)
	// 每个 relay 只租用当前消息；并行等待 confirm，避免串行发布成为瓶颈。
	for i := 0; i < 4; i++ {
		go relay.Run(ctx)
	}
	readiness := infrahealth.Readiness(infrahealth.Checks{
		"mysql":    sqlDB.PingContext,
		"redis":    func(ctx context.Context) error { return redisClient.Ping(ctx).Err() },
		"rabbitmq": rabbitMQ.CheckReady,
	})
	go func() {
		addr := os.Getenv("GCFEED_WORKER_METRICS_ADDR")
		if addr == "" {
			addr = ":9091"
		}
		if err := inframetrics.RunServer(ctx, addr, readiness); err != nil {
			log.Printf("metrics server failed: %v", err)
		}
	}()

	log.Println("gcfeed worker is running")
	select {
	case <-ctx.Done():
	case err := <-rabbitMQ.Errors():
		// 让进程管理器重启，避免监控端口存活但所有消费已停止。
		stop()
		_ = rabbitMQ.Close()
		log.Fatalf("worker consumer failed: %v", err)
	}
	log.Println("gcfeed worker stopped")
}

func startWorkers(ctx context.Context, gormDB *gorm.DB, rabbitMQ *inframq.RabbitMQ, feedCache *infracache.FeedCache) error {
	interactionRepo := infrainteraction.New(gormDB)
	actionWorker := applicationinteraction.NewActionWorker(rabbitMQ, feedCache, applicationinteraction.NewActionEffects(interactionRepo, feedCache))
	if err := actionWorker.Start(ctx); err != nil {
		return err
	}

	feedRepo := infrafeed.New(gormDB)
	feedPreheater := applicationvideo.NewFeedPreheater(feedRepo, feedCache)
	preheatWorker := applicationvideo.NewPreheatWorker(rabbitMQ, feedPreheater)
	if err := preheatWorker.Start(ctx); err != nil {
		return err
	}

	embeddingRepo := infraembedding.New(gormDB)
	embeddingService := applicationembedding.New(embeddingRepo, nil)
	embeddingWorker := applicationembedding.NewVideoEmbeddingWorker(embeddingService, rabbitMQ)
	return embeddingWorker.Start(ctx)
}

func closeSQL(db *sql.DB) {
	if db != nil {
		_ = db.Close()
	}
}

func closeRabbitMQ(rabbitMQ *inframq.RabbitMQ) {
	if rabbitMQ != nil {
		_ = rabbitMQ.Close()
	}
}
