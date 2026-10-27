package conntrack

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Handler 返回 /debug/connections JSON 端点（放在 pprof 独立监听上）。
// 查询参数：
//   - kind: request / upstream（可省略，省略返回全部）
//   - min_duration_ms: 只返回持续时间超过该值的连接
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entries, counts := Global().Snapshot()

		kind := r.URL.Query().Get("kind")
		if minMs, err := strconv.Atoi(r.URL.Query().Get("min_duration_ms")); err == nil && minMs > 0 {
			minDur := time.Duration(minMs) * time.Millisecond
			filtered := entries[:0]
			for _, e := range entries {
				if e.Duration() >= minDur {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}
		if kind != "" {
			filtered := entries[:0]
			for _, e := range entries {
				if e.Kind == kind {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}

		// 补上 duration_ms；Entry 本身已是 JSON 友好快照（字节数为数字）
		type entryOut struct {
			Entry
			DurationMs int64 `json:"duration_ms"`
		}
		out := make([]entryOut, 0, len(entries))
		for _, e := range entries {
			out = append(out, entryOut{Entry: e, DurationMs: e.Duration().Milliseconds()})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":   counts,
			"entries": out,
		})
	})
}
