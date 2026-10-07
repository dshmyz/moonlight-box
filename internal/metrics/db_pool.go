package metrics

import (
	"database/sql"
	"time"

	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var dbPoolConnections = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "moonlight_db_pool_connections",
		Help: "Database connection pool state (max_open / in_use / idle)",
	},
	[]string{"state"},
)

var dbPoolWaitCount = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "moonlight_db_pool_wait_count",
		Help: "Cumulative number of connections waited for (pool exhausted signal)",
	},
)

var dbPoolWaitDuration = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "moonlight_db_pool_wait_duration_seconds",
		Help: "Cumulative time spent waiting for a connection, in seconds",
	},
)

// StartDBPoolCollector 周期性采集 sql.DB 连接池统计到 Prometheus 指标。
// wait_count / wait_duration 持续增长说明连接池成为瓶颈（本会话排查的
// "写锁车队钉死连接"在指标上的直接表现就是 wait_count 快速上涨）。
// 进程级常驻，随进程退出结束。
func StartDBPoolCollector(db *sql.DB, interval time.Duration) {
	if db == nil || interval <= 0 {
		return
	}
	util.SafeGo("metrics.db-pool", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			stats := db.Stats()
			dbPoolConnections.WithLabelValues("max_open").Set(float64(stats.MaxOpenConnections))
			dbPoolConnections.WithLabelValues("in_use").Set(float64(stats.InUse))
			dbPoolConnections.WithLabelValues("idle").Set(float64(stats.Idle))
			dbPoolWaitCount.Set(float64(stats.WaitCount))
			dbPoolWaitDuration.Set(stats.WaitDuration.Seconds())
		}
	})
}
