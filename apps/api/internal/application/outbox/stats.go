package applicationoutbox

type QueueStats struct {
	Pending                 int64   `json:"pending"`
	Failed                  int64   `json:"failed"`
	OldestPendingAgeSeconds float64 `json:"oldest_pending_age_seconds"`
}
