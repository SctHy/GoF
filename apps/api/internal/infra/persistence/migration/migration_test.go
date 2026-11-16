package migration

import (
	interaction "GCFeed/internal/infra/persistence/interaction"
	video "GCFeed/internal/infra/persistence/video"
	"GCFeed/internal/testutil"
	"context"
	"strings"
	"sync"
	"testing"
)

func TestMigrationsPreserveLegacyDataAndSerialize(t *testing.T) {
	db := testutil.Database(t)
	baseline := steps()[0]
	for _, statement := range strings.Split(baseline.sql, ";\n") {
		if strings.TrimSpace(statement) != "" {
			if err := db.Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// 模拟已有旧库及一次中途停止的 version 列迁移。
	if err := db.Exec(`INSERT INTO video (id,author_id,title,description,media_url,cover_url,status,published_at) VALUES (7,9,'legacy','preserved','/video','/cover',2,NOW())`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(addVersion).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- Migrate(db) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckCurrent(db); err != nil {
		t.Fatal(err)
	}
	var old video.VideoModel
	if err := db.First(&old, 7).Error; err != nil || old.Title != "legacy" {
		t.Fatalf("legacy data lost: %+v %v", old, err)
	}
	var count int64
	if err := db.Model(&Record{}).Count(&count).Error; err != nil || count != int64(len(steps())) {
		t.Fatalf("versions=%d err=%v", count, err)
	}
	// 静态 DDL 必须与真实新版写入兼容，而非只验证表存在。
	_, n, _, err := interaction.New(db).CommitAction(context.Background(), 42, 7, "LIKE", true, "MigrationCaseSensitive")
	if err != nil || n != 1 {
		t.Fatalf("durable write after upgrade: %v count=%d", err, n)
	}
	if err := db.Model(&Record{}).Where("version = 1").Update("checksum", "tampered").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err == nil {
		t.Fatal("changed applied migration silently accepted")
	}
}
