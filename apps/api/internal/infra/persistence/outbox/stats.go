package infraoutbox

import (
	app "GCFeed/internal/application/outbox"
	"context"
	"time"
)

// QueueStats only scans unfinished rows through the pending index. Leased or
// delayed messages remain pending: a connected broker alone does not mean caught up.
func (r *Repository) QueueStats(ctx context.Context) (app.QueueStats, error) {
	var row struct {
		Pending int64
		Failed  int64
		Oldest  *time.Time
	}
	err := r.db.WithContext(ctx).Model(&Model{}).
		Select("COALESCE(SUM(failed_at IS NULL),0) AS pending, COALESCE(SUM(failed_at IS NOT NULL),0) AS failed, MIN(CASE WHEN failed_at IS NULL THEN created_at END) AS oldest").
		Where("processed_at IS NULL").Scan(&row).Error
	result := app.QueueStats{Pending: row.Pending, Failed: row.Failed}
	if row.Oldest != nil {
		result.OldestPendingAgeSeconds = max(0, time.Since(*row.Oldest).Seconds())
	}
	return result, err
}
