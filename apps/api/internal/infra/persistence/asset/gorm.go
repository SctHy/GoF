package infraasset

import (
	domainasset "GCFeed/internal/domain/asset"
	"context"
	"gorm.io/gorm"
	"time"
)

type Model struct {
	ID        int64 `gorm:"primaryKey"`
	OwnerID   int64
	URL       string
	Kind      string
	Size      int64
	CreatedAt time.Time
}

func (Model) TableName() string { return "upload_asset" }

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Register(ctx context.Context, ownerID int64, url, kind string, size int64) error {
	return r.db.WithContext(ctx).Create(&Model{OwnerID: ownerID, URL: url, Kind: kind, Size: size}).Error
}

// CheckOwned 可复用调用方事务；资产登记后不可转移所有者。
func CheckOwned(ctx context.Context, db *gorm.DB, ownerID int64, url, kind string) error {
	var count int64
	if err := db.WithContext(ctx).Model(&Model{}).Where("owner_id = ? AND url = ? AND kind = ?", ownerID, url, kind).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return domainasset.ErrNotOwned
	}
	return nil
}
