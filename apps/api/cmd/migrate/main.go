package main

import (
	infraconfig "GCFeed/internal/infra/config"
	infradatabase "GCFeed/internal/infra/database"
	"GCFeed/internal/infra/persistence/migration"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
)

func main() {
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
	if err := migration.Migrate(db); err != nil {
		log.Fatal(err)
	}
	log.Print("schema migrations applied and checksums verified")
}
