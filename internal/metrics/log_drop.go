package metrics

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var logDroppedTotal = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "moonlight_log_dropped_total",
		Help: "Cumulative log lines dropped due to full async queue (disk slower than log rate)",
	},
)

// StartLogDropCollector 周期性采集异步日志丢弃计数。
// 该指标持续增长说明日志产生速率超过磁盘写出速率，应提高采样率或排查磁盘。
func StartLogDropCollector(interval time.Duration) {
	if interval <= 0 {
		return
	}
	util.SafeGo("metrics.log-drop", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			logDroppedTotal.Set(float64(util.LogDroppedCount()))
		}
	})
}
