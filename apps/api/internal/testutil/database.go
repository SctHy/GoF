package testutil

import (
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
	"time"
)

// Database 只创建并删除随机命名的测试库；绝不清空连接串中指定的开发库。
func Database(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GCFEED_TEST_MYSQL_ADMIN_DSN")
	if dsn == "" {
		t.Skip("set GCFEED_TEST_MYSQL_ADMIN_DSN for isolated real MySQL regression")
	}
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid test database DSN")
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	cfg.Timeout = 3 * time.Second
	cfg.ReadTimeout = 10 * time.Second
	cfg.WriteTimeout = 10 * time.Second
	config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(mysql.Open(cfg.FormatDSN()), config)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	sqlAdmin, _ := admin.DB()
	t.Cleanup(func() { _ = sqlAdmin.Close() })
	name := fmt.Sprintf("gcfeed_system_test_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE `" + name + "`").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE `" + name + "`").Error; err != nil {
			t.Error(err)
		}
	})
	cfg.DBName = name
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), config)
	if err != nil {
		t.Fatal("open isolated database failed")
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
