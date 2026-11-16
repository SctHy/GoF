package main

import (
	infraconfig "GCFeed/internal/infra/config"
	infradatabase "GCFeed/internal/infra/database"
	infraoutbox "GCFeed/internal/infra/persistence/outbox"
	"context"
	"encoding/json"
	"flag"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"os"
	"time"
)

func main() {
	replay := flag.String("replay", "", "requeue one failed event ID, preserving payload and identity")
	limit := flag.Int("limit", 50, "maximum pending events to list (1-200)")
	flag.Parse()
	cfg, err := infraconfig.LoadConfig("./configs/config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	sqlDB, err := infradatabase.New(cfg.Database)
	if err != nil {
		log.Fatal("database connection failed")
	}
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	repo := infraoutbox.New(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if *replay != "" {
		if err := repo.Replay(ctx, *replay); err != nil {
			log.Fatal(err)
		}
		log.Print("failed event requeued")
		return
	}
	rows, err := repo.List(ctx, *limit)
	if err != nil {
		log.Fatal(err)
	}
	// 默认不输出 payload（可能含用户内容）。
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{"event_id": row.EventID, "topic": row.Topic, "attempts": row.Attempts, "failed": row.FailedAt != nil, "available_at": row.AvailableAt})
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		log.Fatal(err)
	}
}
