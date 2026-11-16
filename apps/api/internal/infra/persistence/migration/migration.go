package migration

import (
	infrafeed "GCFeed/internal/infra/persistence/feed"
	infravideo "GCFeed/internal/infra/persistence/video"
	"crypto/sha256"
	"embed"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"
)

// SQL 文件是不可变版本。后续 schema 变化请新增版本，不修改已应用的 SQL。
//
//go:embed sql/*.sql
var files embed.FS

const addVersion = "ALTER TABLE interaction_action ADD COLUMN version BIGINT UNSIGNED NOT NULL DEFAULT 0"

type Record struct {
	Version   uint64 `gorm:"primaryKey"`
	Name      string
	Checksum  string
	AppliedAt time.Time
}

func (Record) TableName() string { return "schema_migrations" }

type step struct {
	version  uint64
	name     string
	sql      string
	checksum string
}

func steps() []step {
	names := []string{"001_baseline.sql", "002_durable_events.sql", "003_upload_assets.sql", "004_drop_action_receipts.sql"}
	result := make([]step, 0, len(names))
	for i, name := range names {
		content, err := files.ReadFile("sql/" + name)
		if err != nil {
			panic(err)
		}
		checksumSource := string(content)
		if i == 1 {
			checksumSource += addVersion
		}
		result = append(result, step{uint64(i + 1), name, string(content), fmt.Sprintf("%x", sha256.Sum256([]byte(checksumSource)))})
	}
	return result
}

// Ensure 保持开发环境启动兼容；生产可先运行 cmd/migrate，再设置 GCFEED_AUTO_MIGRATE=false。
func Ensure(db *gorm.DB) error {
	if strings.EqualFold(os.Getenv("GCFEED_AUTO_MIGRATE"), "false") {
		return CheckCurrent(db)
	}
	return Migrate(db)
}

func Migrate(db *gorm.DB) error {
	// GET_LOCK 与所有 DDL 必须使用同一条物理连接；MySQL DDL 隐式提交，不伪装成事务。
	return db.Connection(func(conn *gorm.DB) error {
		// Connection 返回 clone=0 的实例；每条语句需新 Statement，避免 Raw Scan 的 Dest 污染后续模型查询。
		conn = conn.Session(&gorm.Session{NewDB: true})
		var database string
		if err := conn.Raw("SELECT DATABASE()").Scan(&database).Error; err != nil {
			return err
		}
		lock := fmt.Sprintf("gcfeed-migrate-%x", sha256.Sum256([]byte(database)))[:60]
		var acquired int
		if err := conn.Raw("SELECT GET_LOCK(?,30)", lock).Scan(&acquired).Error; err != nil {
			return err
		}
		if acquired != 1 {
			return fmt.Errorf("could not acquire schema migration lock")
		}
		defer conn.Exec("SELECT RELEASE_LOCK(?)", lock)
		if err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
            version BIGINT UNSIGNED NOT NULL PRIMARY KEY,
            name VARCHAR(128) NOT NULL,
            checksum VARCHAR(64) NOT NULL,
            applied_at DATETIME(6) NOT NULL
        )`).Error; err != nil {
			return err
		}
		for _, migration := range steps() {
			var applied Record
			if err := conn.Where("version = ?", migration.version).Limit(1).Find(&applied).Error; err != nil {
				return err
			}
			if applied.Version != 0 {
				if applied.Checksum != migration.checksum {
					return fmt.Errorf("migration %s checksum mismatch", migration.name)
				}
				continue
			}
			if migration.version == 2 && !conn.Migrator().HasColumn("interaction_action", "version") {
				if err := conn.Exec(addVersion).Error; err != nil {
					return err
				}
			}
			for _, statement := range strings.Split(migration.sql, ";\n") {
				if strings.TrimSpace(statement) == "" {
					continue
				}
				if err := conn.Exec(statement).Error; err != nil {
					return fmt.Errorf("migration %s: %w", migration.name, err)
				}
			}
			if migration.version == 1 {
				if err := infravideo.EnsureStats(conn); err != nil {
					return err
				}
				if err := infrafeed.EnsureTimelineIndex(conn); err != nil {
					return err
				}
			}
			if err := conn.Create(&Record{Version: migration.version, Name: migration.name, Checksum: migration.checksum, AppliedAt: time.Now().UTC()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func CheckCurrent(db *gorm.DB) error {
	for _, migration := range steps() {
		var applied Record
		if err := db.Where("version = ?", migration.version).Limit(1).Find(&applied).Error; err != nil {
			return err
		}
		if applied.Checksum != migration.checksum {
			return fmt.Errorf("migration %s missing or changed; run cmd/migrate", migration.name)
		}
	}
	return nil
}
