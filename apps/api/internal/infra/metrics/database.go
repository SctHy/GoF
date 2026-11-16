package inframetrics

import (
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterDatabase 在每个进程入口调用一次；采集直接读取连接池快照，不额外查询数据库。
func RegisterDatabase(db *sql.DB) {
	values := []struct {
		name, help string
		value      func(sql.DBStats) float64
	}{
		{"open_connections", "Open database connections.", func(s sql.DBStats) float64 { return float64(s.OpenConnections) }},
		{"in_use_connections", "Database connections currently in use.", func(s sql.DBStats) float64 { return float64(s.InUse) }},
		{"idle_connections", "Idle database connections.", func(s sql.DBStats) float64 { return float64(s.Idle) }},
		{"max_open_connections", "Configured database connection limit.", func(s sql.DBStats) float64 { return float64(s.MaxOpenConnections) }},
	}
	for _, v := range values {
		prometheus.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: "gcfeed", Subsystem: "db", Name: v.name, Help: v.help}, func() float64 { return v.value(db.Stats()) }))
	}
	prometheus.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: "gcfeed", Subsystem: "db", Name: "wait_total", Help: "Total waits for a database connection."}, func() float64 { return float64(db.Stats().WaitCount) }))
	prometheus.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: "gcfeed", Subsystem: "db", Name: "wait_duration_seconds_total", Help: "Total time waiting for database connections."}, func() float64 { return db.Stats().WaitDuration.Seconds() }))
}
