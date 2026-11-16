package inframetrics

import (
	outbox "GCFeed/internal/application/outbox"
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

var outboxPending = prometheus.NewGauge(prometheus.GaugeOpts{Name: "gcfeed_outbox_pending", Help: "Unfinished, non-quarantined events, including leased and delayed events."})
var outboxFailed = prometheus.NewGauge(prometheus.GaugeOpts{Name: "gcfeed_outbox_failed", Help: "Quarantined events requiring investigation and explicit replay."})
var outboxOldestAge = prometheus.NewGauge(prometheus.GaugeOpts{Name: "gcfeed_outbox_oldest_pending_age_seconds", Help: "Age of the oldest pending event at the last successful sample."})
var outboxStatsUp = prometheus.NewGauge(prometheus.GaugeOpts{Name: "gcfeed_outbox_stats_up", Help: "Whether the latest database queue stats sample succeeded."})
var outboxStatsUpdated = prometheus.NewGauge(prometheus.GaugeOpts{Name: "gcfeed_outbox_stats_updated_timestamp_seconds", Help: "Unix time of the latest successful queue stats sample."})

func init() {
	prometheus.MustRegister(outboxPending, outboxFailed, outboxOldestAge, outboxStatsUp, outboxStatsUpdated)
}

type OutboxStatsSource interface {
	QueueStats(context.Context) (outbox.QueueStats, error)
}

// RunOutboxMonitor keeps database reads off the scrape path and bounds query time.
// Multiple workers observe the same global backlog; aggregate gauges with max, not sum.
func RunOutboxMonitor(ctx context.Context, source OutboxStatsSource) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		stats, err := source.QueueStats(queryCtx)
		cancel()
		if err != nil {
			outboxStatsUp.Set(0)
		} else {
			outboxPending.Set(float64(stats.Pending))
			outboxFailed.Set(float64(stats.Failed))
			outboxOldestAge.Set(stats.OldestPendingAgeSeconds)
			outboxStatsUp.Set(1)
			outboxStatsUpdated.Set(float64(time.Now().Unix()))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
