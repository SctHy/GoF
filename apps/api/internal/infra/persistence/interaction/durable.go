package infrainteraction

import (
	applicationinteraction "GCFeed/internal/application/interaction"
	domaininteraction "GCFeed/internal/domain/interaction"
	outbox "GCFeed/internal/infra/persistence/outbox"
	"context"
	"errors"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

// CommitAction 将事实、单调版本、历史幂等结果及事件放在同一事务；Redis 不再决定事实。
func (r *Repository) CommitAction(ctx context.Context, userID, videoID int64, actionType string, active bool, key string) (*domaininteraction.Action, int, int, error) {
	actionType, err := domaininteraction.NormalizeActionType(actionType)
	if err != nil {
		return nil, 0, 0, err
	}
	key = strings.TrimSpace(key)
	var action *domaininteraction.Action
	var count, delta int
	for attempt := 0; attempt < 3; attempt++ {
		err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := lockActionVideo(tx, videoID); err != nil {
				return err
			}
			// INSERT ... ON DUPLICATE KEY 负责不存在记录时的并发建行，避免先查空造成 gap-lock 升级。
			seed := ActionModel{UserID: userID, VideoID: videoID, ActionType: actionType, Status: domaininteraction.ActionStatusCanceled}
			if err := tx.Clauses(clause.OnConflict{DoUpdates: clause.AssignmentColumns([]string{"user_id"})}).Create(&seed).Error; err != nil {
				return err
			}
			var current ActionModel
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND video_id = ? AND action_type = ?", userID, videoID, actionType).Take(&current).Error; err != nil {
				return err
			}
			if key != "" {
				var previous ActionRequestModel
				err := tx.Where("user_id = ? AND video_id = ? AND action_type = ? AND request_key = ?", userID, videoID, actionType, key).Take(&previous).Error
				if err == nil {
					action = restoreAction(current)
					action.Status = previous.Status
					action.Version = previous.Version
					action.IdempotencyKey = key
					action.Stat = &domaininteraction.VideoStat{VideoID: videoID, LikeCount: previous.LikeCount, FavoriteCount: previous.FavoriteCount}
					count = previous.Count
					delta = 0
					return nil
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			}
			nextStatus := actionStatusFromActive(active)
			delta = 0
			if current.Status != nextStatus {
				if nextStatus == domaininteraction.ActionStatusActive {
					delta = 1
				} else {
					delta = -1
				}
			}
			current.Status = nextStatus
			current.Version++
			current.IdempotencyKey = idempotencyKeyPtr(key)
			current.UpdatedAt = time.Now()
			if err := tx.Model(&ActionModel{}).Where("id = ?", current.ID).Updates(map[string]any{"status": current.Status, "version": current.Version, "idempotency_key": current.IdempotencyKey, "updated_at": current.UpdatedAt}).Error; err != nil {
				return err
			}
			if delta != 0 {
				field := "like_count"
				if actionType == domaininteraction.ActionTypeFavorite {
					field = "favorite_count"
				}
				if err := tx.Table("video_stat").Where("video_id = ?", videoID).UpdateColumn(field, gorm.Expr("GREATEST("+field+" + ?, 0)", delta)).Error; err != nil {
					return err
				}
			}
			action = restoreAction(current)
			var err error
			action.Stat, err = New(tx).GetVideoStat(ctx, videoID)
			if err != nil {
				return err
			}
			count = action.Stat.LikeCount
			if actionType == domaininteraction.ActionTypeFavorite {
				count = action.Stat.FavoriteCount
			}

			if key != "" {
				if err := tx.Create(&ActionRequestModel{UserID: userID, VideoID: videoID, ActionType: actionType, RequestKey: key, Status: action.Status, Version: action.Version, Count: count, LikeCount: action.Stat.LikeCount, FavoriteCount: action.Stat.FavoriteCount}).Error; err != nil {
					return err
				}
			}
			event := applicationinteraction.NewActionChangedEvent(userID, videoID, actionType, action.Active(), key)
			event.Version = action.Version
			event.EffectsVersion = 1
			event.Delta = delta
			return outbox.Enqueue(tx, "action.changed", event.EventID, event)
		})
		if err == nil {
			return action, count, delta, nil
		}
		var dbErr *mysql.MySQLError
		if !errors.As(err, &dbErr) || (dbErr.Number != 1213 && dbErr.Number != 1205) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, 0, 0, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 10 * time.Millisecond):
		}
	}
	return nil, 0, 0, err
}

type CountDifference struct {
	VideoID         int64 `json:"video_id"`
	StoredLikes     int64 `json:"stored_likes"`
	ActualLikes     int64 `json:"actual_likes"`
	StoredFavorites int64 `json:"stored_favorites"`
	ActualFavorites int64 `json:"actual_favorites"`
}

// ReconcileCounts 默认只读，单条聚合 SQL 在同一快照比较计数，无写修复和缓存清空。
func (r *Repository) ReconcileCounts(ctx context.Context, afterID int64, limit int) ([]CountDifference, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var result []CountDifference
	err := r.db.WithContext(ctx).Raw(`SELECT s.video_id, s.like_count AS stored_likes, s.favorite_count AS stored_favorites,
 COALESCE(a.likes,0) AS actual_likes, COALESCE(a.favorites,0) AS actual_favorites
 FROM (SELECT * FROM video_stat WHERE video_id > ? ORDER BY video_id LIMIT ?) AS s
 LEFT JOIN (SELECT video_id, SUM(action_type='LIKE' AND status=1) AS likes,
 SUM(action_type='FAVORITE' AND status=1) AS favorites FROM interaction_action
 WHERE video_id IN (SELECT video_id FROM (SELECT video_id FROM video_stat WHERE video_id > ? ORDER BY video_id LIMIT ?) AS page_ids)
 GROUP BY video_id) AS a ON a.video_id=s.video_id ORDER BY s.video_id`, afterID, limit, afterID, limit).Scan(&result).Error
	return result, err
}

func lockActionVideo(tx *gorm.DB, videoID int64) error {
	var id int64
	err := tx.Table("video").Select("id").Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND status = 2", videoID).Take(&id).Error
	return mapVideoError(err)
}
