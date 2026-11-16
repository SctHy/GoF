package applicationfeed

import (
	"GCFeed/internal/application/pagination"
	domainfeed "GCFeed/internal/domain/feed"
	"context"
	"encoding/json"
	"strings"
	"time"
)

// NewHotStrategy 创建热榜排序策略。
func NewHotStrategy(repo domainfeed.Repository) *HotStrategy {
	return &HotStrategy{repo: repo, snapshots: pagination.NewMemoryCache(256)}
}

// Scene 返回热榜场景。
func (s *HotStrategy) Scene() domainfeed.Scene {
	return domainfeed.SceneHot
}

// List 读取热榜；Redis 场景使用最近一小时分钟桶，基础场景使用仓储累计热度。
func (s *HotStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	limit := normalizeLimit(req.Limit)
	cursor, err := pagination.Decode(strings.TrimSpace(req.Cursor))
	if err != nil {
		return nil, domainfeed.ErrInvalidCursor
	}
	var snapshot hotSnapshot
	id, offset := "", 0
	if cursor != nil {
		id, offset = cursor.ID, cursor.Offset
		content, found, err := s.snapshots.ReadBlob(ctx, "hot:"+id)
		if err != nil {
			return nil, ErrLoadFeedFailed
		}
		if !found || json.Unmarshal(content, &snapshot) != nil || snapshot.ViewerID != req.ViewerID || offset > len(snapshot.Items) {
			return nil, domainfeed.ErrInvalidCursor
		}
	} else {
		var ranked []*domainfeed.FeedPageItem
		if s.cache != nil {
			ranked, err = s.cache.ListHotWindowPage(ctx, time.Now().UTC().Truncate(time.Minute), 0, pagination.MaxItems)
		} else {
			ranked, err = s.repo.ListHotCandidates(ctx, pagination.MaxItems)
		}
		if err != nil {
			return nil, ErrLoadFeedFailed
		}
		if len(ranked) > pagination.MaxItems {
			ranked = ranked[:pagination.MaxItems]
		}
		snapshot = hotSnapshot{ViewerID: req.ViewerID, Items: ranked}
		if len(ranked) > 0 {
			id, err = pagination.NewID()
			if err != nil {
				return nil, ErrLoadFeedFailed
			}
			content, err := json.Marshal(snapshot)
			if err != nil || s.snapshots.WriteBlob(ctx, "hot:"+id, content, pagination.SnapshotTTL) != nil {
				return nil, ErrLoadFeedFailed
			}
		}
	}
	end := offset + limit
	if end > len(snapshot.Items) {
		end = len(snapshot.Items)
	}
	items, err := assembleFeedItems(ctx, s.repo, s.cache, snapshot.Items[offset:end], req.ViewerID)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}
	next := ""
	if id != "" && end > offset {
		next = pagination.Encode(id, end)
	}
	return &FeedResult{Scene: domainfeed.SceneHot, Items: items, NextCursor: next, HasMore: end < len(snapshot.Items)}, nil
}

type hotSnapshot struct {
	ViewerID int64
	Items    []*domainfeed.FeedPageItem
}
