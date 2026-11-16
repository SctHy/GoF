package infrafeed

import (
	domainfeed "GCFeed/internal/domain/feed"
	domaininteraction "GCFeed/internal/domain/interaction"
	domainrelation "GCFeed/internal/domain/relation"
	domainvideo "GCFeed/internal/domain/video"
	"context"

	"gorm.io/gorm"
)

const hotScoreExpression = "COALESCE(vs.like_count, 0) * 3 + COALESCE(vs.comment_count, 0) * 5 + COALESCE(vs.favorite_count, 0) * 4"

type Repository struct {
	db *gorm.DB
}

type viewerActionStateModel struct {
	VideoID    int64
	ActionType string
}

// New 创建 Feed 仓储实现。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// EnsureTimelineIndex 创建 timeline 回源查询所需索引。
func EnsureTimelineIndex(db *gorm.DB) error {
	var count int64
	err := db.Raw(
		"SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
		"video",
		"idx_video_timeline",
	).Scan(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.Exec("CREATE INDEX idx_video_timeline ON video (status, published_at DESC, id DESC)").Error
}

// ListTimelinePage 查询时间线 Feed 轻量页，卡片和计数由应用层批量组装。
func (r *Repository) ListTimelinePage(ctx context.Context, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	var models []domainfeed.FeedPageItem
	query := r.basePageQuery(ctx)

	if cursor != nil {
		// 游标分页条件和排序字段保持一致：published_at DESC, id DESC。
		query = query.Where(
			"(v.published_at < ? OR (v.published_at = ? AND v.id < ?))",
			cursor.PublishedAt,
			cursor.PublishedAt,
			cursor.VideoID,
		)
	}

	err := query.
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	return feedPageItemsFromModels(models), nil
}

// ListHotCandidates 查询热榜 Feed 轻量页，按互动热度倒序稳定分页。
func (r *Repository) ListHotCandidates(ctx context.Context, limit int) ([]*domainfeed.FeedPageItem, error) {
	var models []domainfeed.FeedPageItem
	query := r.baseHotPageQuery(ctx)

	err := query.
		Order("hot_score DESC").
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}

	return feedPageItemsFromModels(models), nil
}

// ListFollowingPage 按关注关系读取关注流，按发布时间和视频 ID 稳定分页。
func (r *Repository) ListFollowingPage(ctx context.Context, viewerID int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	var models []domainfeed.FeedPageItem
	query := r.db.WithContext(ctx).
		Table("video AS v").
		Select("v.id AS video_id, v.author_id, v.published_at").
		Joins("JOIN user_follow AS f ON f.target_user_id = v.author_id").
		Where("f.user_id = ? AND f.status = ? AND v.status = ? AND v.published_at IS NOT NULL", viewerID, domainrelation.FollowStatusActive, domainvideo.StatusPublished)

	if cursor != nil {
		query = query.Where(
			"(v.published_at < ? OR (v.published_at = ? AND v.id < ?))",
			cursor.PublishedAt,
			cursor.PublishedAt,
			cursor.VideoID,
		)
	}

	err := query.
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	return feedPageItemsFromModels(models), nil
}

func (r *Repository) BatchGetFeedCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	cards := map[int64]*domainfeed.FeedCard{}
	if len(videoIDs) == 0 {
		return cards, nil
	}

	var models []domainfeed.FeedCard
	err := r.db.WithContext(ctx).
		Table("video AS v").
		Select("v.id AS video_id, v.author_id, a.nickname AS author_nickname, a.avatar_url AS author_avatar_url, v.title, v.description, v.media_url, v.cover_url, v.published_at").
		Joins("LEFT JOIN account AS a ON a.id = v.author_id").
		Where("v.id IN ? AND v.status = ? AND v.published_at IS NOT NULL", videoIDs, domainvideo.StatusPublished).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for index := range models {
		cards[models[index].VideoID] = &models[index]
	}
	return cards, nil
}

// BatchGetFeedStats 批量读取互动计数，缺失统计记录时按 0 处理。
func (r *Repository) BatchGetFeedStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	stats := map[int64]*domainfeed.FeedStat{}
	if len(videoIDs) == 0 {
		return stats, nil
	}
	for _, videoID := range videoIDs {
		stats[videoID] = &domainfeed.FeedStat{VideoID: videoID}
	}

	var models []domainfeed.FeedStat
	err := r.db.WithContext(ctx).
		Table("video_stat").
		Select("video_id, like_count, comment_count, favorite_count").
		Where("video_id IN ?", videoIDs).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for index := range models {
		stats[models[index].VideoID] = &models[index]
	}
	return stats, nil
}

// BatchGetViewerActionStates 批量读取当前用户对视频的点赞和收藏状态。
func (r *Repository) BatchGetViewerActionStates(ctx context.Context, viewerID int64, videoIDs []int64) (map[int64]*domainfeed.ViewerActionState, error) {
	states := map[int64]*domainfeed.ViewerActionState{}
	if viewerID <= 0 || len(videoIDs) == 0 {
		return states, nil
	}
	for _, videoID := range videoIDs {
		states[videoID] = &domainfeed.ViewerActionState{VideoID: videoID}
	}

	var models []viewerActionStateModel
	err := r.db.WithContext(ctx).
		Table("interaction_action").
		Select("video_id, action_type").
		Where("user_id = ? AND video_id IN ? AND status = ?", viewerID, videoIDs, domaininteraction.ActionStatusActive).
		Where("action_type IN ?", []string{domaininteraction.ActionTypeLike, domaininteraction.ActionTypeFavorite}).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		state := states[model.VideoID]
		if state == nil {
			state = &domainfeed.ViewerActionState{VideoID: model.VideoID}
			states[model.VideoID] = state
		}
		switch model.ActionType {
		case domaininteraction.ActionTypeLike:
			state.Liked = true
		case domaininteraction.ActionTypeFavorite:
			state.Favorited = true
		}
	}
	return states, nil
}

func (r *Repository) basePageQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("video AS v").
		Select("v.id AS video_id, v.author_id, v.published_at").
		Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
}

func (r *Repository) baseHotPageQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("video AS v").
		Select("v.id AS video_id, v.author_id, ("+hotScoreExpression+") AS hot_score, v.published_at").
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
		Where("v.status = ? AND v.published_at IS NOT NULL", domainvideo.StatusPublished)
}

func feedPageItemsFromModels(models []domainfeed.FeedPageItem) []*domainfeed.FeedPageItem {
	items := make([]*domainfeed.FeedPageItem, 0, len(models))
	for index := range models {
		items = append(items, &models[index])
	}
	return items
}

// FilterVisibleIDs 始终读取数据库，防止缓存中的已删除视频或过期关注关系泄漏。
func (r *Repository) FilterVisibleIDs(ctx context.Context, videoIDs []int64, viewerID int64, following bool) (map[int64]bool, error) {
	visible := make(map[int64]bool, len(videoIDs))
	if len(videoIDs) == 0 {
		return visible, nil
	}
	query := r.db.WithContext(ctx).Table("video AS v").Select("v.id").
		Where("v.id IN ? AND v.status = ? AND v.published_at IS NOT NULL", videoIDs, domainvideo.StatusPublished)
	if following {
		query = query.Joins("JOIN user_follow AS f ON f.target_user_id = v.author_id").
			Where("f.user_id = ? AND f.status = ?", viewerID, domainrelation.FollowStatusActive)
	}
	var ids []int64
	if err := query.Scan(&ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		visible[id] = true
	}
	return visible, nil
}
