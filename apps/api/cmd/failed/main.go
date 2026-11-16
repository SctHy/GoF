package main

import (
	infraconfig "GCFeed/internal/infra/config"
	inframq "GCFeed/internal/infra/mq"
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"
)

func main() {
	queue := flag.String("queue", "action", "action, preheat or embedding")
	replay := flag.String("replay", "", "exact failed event ID to replay (default: list without payload)")
	limit := flag.Int("limit", 50, "messages to inspect, maximum 100")
	flag.Parse()
	cfg, err := infraconfig.LoadConfig("./configs/config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	mq, err := inframq.NewRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		log.Fatal("RabbitMQ connection failed")
	}
	defer mq.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if *replay != "" {
		if err := mq.ReplayFailed(ctx, *queue, *replay); err != nil {
			log.Fatal(err)
		}
		log.Print("failed message republished with original event ID")
		return
	}
	rows, err := mq.ListFailed(ctx, *queue, *limit)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(rows); err != nil {
		log.Fatal(err)
	}
}
