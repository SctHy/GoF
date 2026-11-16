package domainfeed

import "context"

// Repository 定义 Feed 读取需要的持久化能力。
type Repository interface {
	// FilterVisibleIDs 校验当前视频状态；following 为 true 时同时校验关注关系。
	FilterVisibleIDs(ctx context.Context, videoIDs []int64, viewerID int64, following bool) (map[int64]bool, error)
	// ListTimelinePage 按发布时间倒序读取轻量 Feed 页。
	ListTimelinePage(ctx context.Context, cursor *TimelineCursor, limit int) ([]*FeedPageItem, error)
	// ListHotCandidates 按热度分倒序读取轻量 Feed 页。
	ListHotCandidates(ctx context.Context, limit int) ([]*FeedPageItem, error)
	// ListFollowingPage 按发布时间倒序读取当前用户关注流，直接按当前关注关系和视频状态过滤。
	ListFollowingPage(ctx context.Context, viewerID int64, cursor *TimelineCursor, limit int) ([]*FeedPageItem, error)
	// BatchGetFeedCards 批量读取视频卡片展示字段。
	BatchGetFeedCards(ctx context.Context, videoIDs []int64) (map[int64]*FeedCard, error)
	// BatchGetFeedStats 批量读取视频互动计数。
	BatchGetFeedStats(ctx context.Context, videoIDs []int64) (map[int64]*FeedStat, error)
	// BatchGetViewerActionStates 批量读取当前用户对视频的点赞和收藏状态。
	BatchGetViewerActionStates(ctx context.Context, viewerID int64, videoIDs []int64) (map[int64]*ViewerActionState, error)
}
