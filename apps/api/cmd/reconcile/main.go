// reconcile is a read-only, paginated interaction counter audit. It never migrates or repairs data.
package main

import (
	infraconfig "GCFeed/internal/infra/config"
	infradatabase "GCFeed/internal/infra/database"
	infrainteraction "GCFeed/internal/infra/persistence/interaction"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	config := flag.String("config", "./configs/config.yaml", "config file")
	after := flag.Int64("after", 0, "exclusive video ID cursor")
	limit := flag.Int("limit", 100, "videos to inspect, 1..1000")
	flag.Parse()
	if *after < 0 || *limit < 1 || *limit > 1000 {
		return fmt.Errorf("invalid cursor or limit")
	}
	cfg, err := infraconfig.LoadConfig(*config)
	if err != nil {
		return err
	}
	sqlDB, err := infradatabase.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("database connection failed")
	}
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := infrainteraction.New(db).ReconcileCounts(ctx, *after, *limit)
	if err != nil {
		return err
	}
	differences := []infrainteraction.CountDifference{}
	cursor := *after
	for _, row := range rows {
		cursor = row.VideoID
		if row.StoredLikes != row.ActualLikes || row.StoredFavorites != row.ActualFavorites {
			differences = append(differences, row)
		}
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Scanned     int                                `json:"scanned"`
		NextCursor  int64                              `json:"next_cursor"`
		Differences []infrainteraction.CountDifference `json:"differences"`
	}{len(rows), cursor, differences})
}
