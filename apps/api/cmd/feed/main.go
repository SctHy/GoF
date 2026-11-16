package main

import (
	"log"

	infraconfig "GCFeed/internal/infra/config"
	infradatabase "GCFeed/internal/infra/database"
	infrahttpgin "GCFeed/internal/infra/httpgin"
	inframetrics "GCFeed/internal/infra/metrics"
	interfaceshttprouter "GCFeed/internal/interfaces/http/router"
)

const configPath = "./configs/config.yaml"

func main() {
	cfg, err := infraconfig.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	log.Printf(
		"config loaded: port=%d database=%s:%d/%s jwt_access_ttl=%s",
		cfg.Port,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Name,
		cfg.JWT.AccessTTL,
	)

	// 数据库连接使用 database/sql 连接池，后续会被 GORM 复用。
	db, err := infradatabase.New(cfg.Database)
	if err != nil {
		log.Fatalf("init database failed: %v", err)
	}
	defer db.Close()
	inframetrics.RegisterDatabase(db)
	log.Println("database connection initialized")

	g := infrahttpgin.Init()
	log.Println("gin engine initialized")

	if err := interfaceshttprouter.Register(g, cfg, db); err != nil {
		log.Fatalf("init router failed: %v", err)
	}
	log.Println("router registered")

	log.Println("server is running")
	if err := infrahttpgin.Run(cfg, g); err != nil {
		log.Fatalf("run server failed: %v", err)
	}
}
