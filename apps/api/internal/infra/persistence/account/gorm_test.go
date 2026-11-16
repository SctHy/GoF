package infraaccount

import (
	"context"
	"errors"
	"testing"

	domainaccount "GCFeed/internal/domain/account"
	domainasset "GCFeed/internal/domain/asset"
	infraasset "GCFeed/internal/infra/persistence/asset"
	"GCFeed/internal/testutil"
)

func TestAuthStateReadsCurrentStatusAndRoleWithoutProfileTables(t *testing.T) {
	db := testutil.Database(t)
	if err := db.AutoMigrate(&UserModel{}); err != nil {
		t.Fatal(err)
	}
	model := UserModel{Account: "alice", Password: "unused", Nickname: "Alice", Status: 1, Role: "admin"}
	if err := db.Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	ctx := context.Background()
	status, role, err := repo.AuthState(ctx, model.ID)
	if err != nil || status != 1 || role != "admin" {
		t.Fatalf("initial state: %d %s %v", status, role, err)
	}
	if err := db.Model(&model).Updates(map[string]any{"status": 0, "role": "user"}).Error; err != nil {
		t.Fatal(err)
	}
	status, role, err = repo.AuthState(ctx, model.ID)
	if err != nil || status != 0 || role != "user" {
		t.Fatalf("changed state: %d %s %v", status, role, err)
	}
	if _, _, err := repo.AuthState(ctx, model.ID+1); !errors.Is(err, domainaccount.ErrUserNotFound) {
		t.Fatalf("missing user: %v", err)
	}
}

func TestUpdateProfileChecksAvatarOwnership(t *testing.T) {
	db := testutil.Database(t)
	if err := db.AutoMigrate(&UserModel{}, &infraasset.Model{}); err != nil {
		t.Fatal(err)
	}
	model := UserModel{Account: "alice", Password: "unused", Nickname: "Alice", Status: 1, Role: "user"}
	if err := db.Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := New(db)
	user := domainaccount.RestoreUser(model.ID, model.Account, model.Password, model.Nickname, model.AvatarURL, "", model.Status, model.Role)
	user.Nickname = "updated"
	if err := repo.UpdateProfile(ctx, user); err != nil {
		t.Fatalf("empty avatar: %v", err)
	}
	assets := infraasset.New(db)
	for _, asset := range []struct {
		owner     int64
		url, kind string
	}{{model.ID, "/uploads/own.png", "avatar"}, {model.ID + 1, "/uploads/other.png", "avatar"}, {model.ID, "/uploads/video.mp4", "video"}} {
		if err := assets.Register(ctx, asset.owner, asset.url, asset.kind, 100); err != nil {
			t.Fatal(err)
		}
	}
	for _, url := range []string{"/uploads/other.png", "/uploads/video.mp4", "https://external.example/a.png"} {
		user.AvatarURL = url
		user.Nickname = "must not persist"
		if err := repo.UpdateProfile(ctx, user); !errors.Is(err, domainasset.ErrNotOwned) {
			t.Fatalf("URL %q: %v", url, err)
		}
		var stored UserModel
		if err := db.First(&stored, model.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Nickname != "updated" || stored.AvatarURL != model.AvatarURL {
			t.Fatal("failed asset check changed profile")
		}
	}
	user.AvatarURL = "/uploads/own.png"
	user.Nickname = "owner"
	if err := repo.UpdateProfile(ctx, user); err != nil {
		t.Fatal(err)
	}
	user.AvatarURL = ""
	if err := repo.UpdateProfile(ctx, user); err != nil {
		t.Fatalf("clear avatar: %v", err)
	}
	var stored UserModel
	if err := db.First(&stored, model.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.AvatarURL != "" {
		t.Fatal("avatar not cleared")
	}
}
