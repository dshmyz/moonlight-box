// Package conntrack 提供活跃连接/请求的实时登记与查询。
//
// 两层用途：
//   - Prometheus 指标（internal/metrics）：活跃连接总数、流量字节计数。
//   - /debug/connections 端点：按连接列出远端地址、仓库、路径、已传字节数、
//     持续时间，排查"谁在占带宽/卡住"。
//
// 登记点：
//   - 服务端请求：gin middleware（所有入站请求）。
//   - 上游回源：TransportManager.DialContext（TCP 建连时登记，conn 关闭注销）。
package conntrack

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Entry 一条活跃连接/请求的快照。
//
// 字节数是普通 int64（不是 atomic）：Entry 会直接被管理端序列化成 JSON，
// atomic.Int64 没有导出字段，编码结果会是 {} 而不是数字。原子计数放在
// 内部 connState 里，Snapshot 时再取值。
type Entry struct {
	// ID 唯一标识（自增）。
	ID uint64 `json:"id"`
	// Kind: "request"（服务端入站请求）或 "upstream"（回源出站连接）。
	Kind string `json:"kind"`
	// Remote 客户端地址（request）或上游地址（upstream）。
	Remote string `json:"remote"`
	// Repository 仓库名（request）或目标仓库（upstream），可为空。
	Repository string `json:"repository,omitempty"`
	// Path 请求路径（request）或上游 URL（upstream），可为空。
	Path string `json:"path,omitempty"`
	// StartedAt 开始时间。
	StartedAt time.Time `json:"started_at"`
	// BytesRead / BytesWritten 已传输字节数（登记期间累计）。
	BytesRead    int64 `json:"bytes_read"`
	BytesWritten int64 `json:"bytes_written"`
	// Extra 补充信息（如 method、user-agent）。
	Extra map[string]string `json:"extra,omitempty"`
}

// connState 登记表内部状态：Entry 快照 + 原子字节计数。
// 计数发生在 IO 热路径（每次 Read/Write），不能走 Tracker 的全局锁。
type connState struct {
	Entry
	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
}

// Duration 从开始到现在的时间。
func (e *Entry) Duration() time.Duration { return time.Since(e.StartedAt) }

// Tracker 并发安全的连接登记表。
type Tracker struct {
	mu      sync.RWMutex
	nextID  uint64
	entries map[uint64]*connState
	// per-kind 计数（供 Prometheus gauge 快速读取，免遍历）
	counts map[string]int64
}

// 包级指标：整个进程只有一份，多个 Tracker 实例（如测试）共享。
var (
	bytesReadCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "moonlight_conntrack_bytes_read_total",
		Help: "Total bytes read on tracked connections (requests + upstream)",
	})
	bytesWrittenCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "moonlight_conntrack_bytes_written_total",
		Help: "Total bytes written on tracked connections (requests + upstream)",
	})
)

// NewTracker 创建登记表。
func NewTracker() *Tracker {
	return &Tracker{
		entries: make(map[uint64]*connState),
		counts:  make(map[string]int64),
	}
}

var global = NewTracker()

// Global 返回进程级单例。
func Global() *Tracker { return global }

// Register 登记一条连接/请求，返回句柄用于后续更新与注销。
func (t *Tracker) Register(e Entry) *Handle {
	t.mu.Lock()
	t.nextID++
	e.ID = t.nextID
	if e.StartedAt.IsZero() {
		e.StartedAt = time.Now()
	}
	st := &connState{Entry: e}
	t.entries[e.ID] = st
	t.counts[e.Kind]++
	t.mu.Unlock()

	activeConnections.WithLabelValues(e.Kind).Inc()

	return &Handle{tracker: t, id: e.ID, kind: e.Kind, state: st}
}

// Handle 登记句柄：更新字节计数、注销。
type Handle struct {
	tracker *Tracker
	id      uint64
	kind    string
	state   *connState
	done    atomic.Bool
}

// AddRead 累加已读字节。无锁：计数在 connState 的 atomic 字段上，IO 热路径开销极低。
func (h *Handle) AddRead(n int64) {
	if n == 0 {
		return
	}
	h.state.bytesRead.Add(n)
	bytesReadCounter.Add(float64(n))
}

// AddWritten 累加已写字节。无锁：计数在 connState 的 atomic 字段上，IO 热路径开销极低。
func (h *Handle) AddWritten(n int64) {
	if n == 0 {
		return
	}
	h.state.bytesWritten.Add(n)
	bytesWrittenCounter.Add(float64(n))
}

// Unregister 注销连接/请求。幂等。
func (h *Handle) Unregister() {
	if !h.done.CompareAndSwap(false, true) {
		return
	}
	h.tracker.mu.Lock()
	delete(h.tracker.entries, h.id)
	h.tracker.counts[h.kind]--
	h.tracker.mu.Unlock()
	activeConnections.WithLabelValues(h.kind).Dec()
}

// Snapshot 返回当前所有活跃连接，按开始时间排序（最久在前）。
// count 为各 kind 的总数。
func (t *Tracker) Snapshot() (entries []Entry, counts map[string]int64) {
	t.mu.RLock()
	entries = make([]Entry, 0, len(t.entries))
	for _, st := range t.entries {
		e := st.Entry
		e.BytesRead = st.bytesRead.Load()
		e.BytesWritten = st.bytesWritten.Load()
		entries = append(entries, e)
	}
	counts = make(map[string]int64, len(t.counts))
	for k, v := range t.counts {
		counts[k] = v
	}
	t.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].StartedAt.Before(entries[j].StartedAt) })
	return entries, counts
}

var activeConnections = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "moonlight_connections_active",
		Help: "Number of active connections/requests by kind (request = inbound, upstream = outbound fetch)",
	},
	[]string{"kind"},
)

// ActiveCount 返回某 kind 当前活跃数（0 表示无）。
func (t *Tracker) ActiveCount(kind string) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.counts[kind]
}
