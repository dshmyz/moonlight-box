package conntrack

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

func newTestTracker() *Tracker {
	return NewTracker()
}

func TestRegisterUnregister(t *testing.T) {
	tr := newTestTracker()
	h := tr.Register(Entry{Kind: "request", Remote: "1.2.3.4:5"})

	entries, counts := tr.Snapshot()
	if len(entries) != 1 || counts["request"] != 1 {
		t.Fatalf("after register: entries=%d counts=%v", len(entries), counts)
	}
	if entries[0].Remote != "1.2.3.4:5" {
		t.Fatalf("remote = %q", entries[0].Remote)
	}
	if entries[0].ID == 0 {
		t.Fatal("ID should be assigned")
	}

	h.Unregister()
	entries, counts = tr.Snapshot()
	if len(entries) != 0 || counts["request"] != 0 {
		t.Fatalf("after unregister: entries=%d counts=%v", len(entries), counts)
	}

	// 幂等
	h.Unregister()
	if _, counts := tr.Snapshot(); counts["request"] != 0 {
		t.Fatal("double unregister should be idempotent")
	}
}

func TestAddReadWritten(t *testing.T) {
	tr := newTestTracker()
	h := tr.Register(Entry{Kind: "upstream", Remote: "upstream:443"})
	h.AddRead(100)
	h.AddWritten(50)
	h.AddRead(0) // no-op

	entries, _ := tr.Snapshot()
	if entries[0].BytesRead != 100 || entries[0].BytesWritten != 50 {
		t.Fatalf("bytes = read %d write %d", entries[0].BytesRead, entries[0].BytesWritten)
	}
}

// 管理端直接返回 Snapshot()，字节数必须是 JSON 数字；atomic.Int64 会被编码成 {}。
func TestSnapshotMarshalsByteCountsAsNumbers(t *testing.T) {
	tr := newTestTracker()
	h := tr.Register(Entry{Kind: "request", Remote: "1.2.3.4"})
	h.AddRead(7)
	h.AddWritten(42)

	entries, _ := tr.Snapshot()
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded []struct {
		BytesRead    int64 `json:"bytes_read"`
		BytesWritten int64 `json:"bytes_written"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	if len(decoded) != 1 || decoded[0].BytesRead != 7 || decoded[0].BytesWritten != 42 {
		t.Fatalf("decoded = %+v, want bytes_read=7 bytes_written=42 (raw=%s)", decoded, raw)
	}
}

func TestSnapshotSortsByStartedAt(t *testing.T) {
	tr := newTestTracker()
	h1 := tr.Register(Entry{Kind: "request"})
	time.Sleep(5 * time.Millisecond)
	tr.Register(Entry{Kind: "request"})

	entries, _ := tr.Snapshot()
	if !entries[0].StartedAt.Before(entries[1].StartedAt) {
		t.Fatal("entries should be sorted by started_at ascending")
	}
	h1.Unregister()
}

func TestActiveCount(t *testing.T) {
	tr := newTestTracker()
	h := tr.Register(Entry{Kind: "request"})
	if tr.ActiveCount("request") != 1 {
		t.Fatal("count should be 1")
	}
	h.Unregister()
	if tr.ActiveCount("request") != 0 {
		t.Fatal("count should be 0")
	}
}

func TestHandler(t *testing.T) {
	tr := newTestTracker()
	h := tr.Register(Entry{Kind: "request", Remote: "1.2.3.4:5", Path: "/repository/npm/x"})
	h.AddWritten(42)

	// 临时替换 global，让 Handler 使用测试 tracker
	old := global
	global = tr
	defer func() { global = old }()

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/debug/connections", nil))

	var resp struct {
		Count   map[string]int64 `json:"count"`
		Entries []struct {
			Kind         string `json:"kind"`
			BytesWritten int64  `json:"bytes_written"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if resp.Count["request"] != 1 {
		t.Fatalf("count = %v", resp.Count)
	}
	if len(resp.Entries) != 1 || resp.Entries[0].BytesWritten != 42 {
		t.Fatalf("entries = %+v", resp.Entries)
	}
}

func TestHandlerKindFilter(t *testing.T) {
	tr := newTestTracker()
	tr.Register(Entry{Kind: "request"})
	tr.Register(Entry{Kind: "upstream"})

	old := global
	global = tr
	defer func() { global = old }()

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/debug/connections?kind=upstream", nil))

	var resp struct {
		Count   map[string]int64 `json:"count"`
		Entries []struct {
			Kind string `json:"kind"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Entries) != 1 || resp.Entries[0].Kind != "upstream" {
		t.Fatalf("entries = %+v", resp.Entries)
	}
}

// 入站请求的仓库名必须从路由参数取到：全局中间件执行时 gin 已完成路由匹配，
// c.Param 可用。不能依赖 group 级中间件（它晚于本中间件执行）。
func TestMiddlewareRecordsRepositoryFromRouteParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tr := newTestTracker()
	old := global
	global = tr
	defer func() { global = old }()

	r := gin.New()
	r.Use(Middleware()) // 只注册全局中间件
	g := r.Group("/repository/:repoName")
	g.GET("/*path", func(c *gin.Context) {
		entries, _ := tr.Snapshot()
		if len(entries) != 1 || entries[0].Repository != "myrepo" {
			t.Errorf("repository = %q, want myrepo", entries[0].Repository)
		}
		c.String(200, "ok")
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/repository/myrepo/pkg", nil))
}

// 响应写出有两条路径：Write 与 WriteString（io.StringWriter 快路径）。
// 两者都必须计入 BytesWritten，否则流量统计系统性偏低。
func TestMiddlewareCountsWriteString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tr := newTestTracker()
	old := global
	global = tr
	defer func() { global = old }()

	r := gin.New()
	r.Use(Middleware())
	r.GET("/str", func(c *gin.Context) {
		if _, err := c.Writer.WriteString("12345"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
		entries, _ := tr.Snapshot()
		if len(entries) != 1 || entries[0].BytesWritten != 5 {
			t.Errorf("bytes written = %d, want 5", entries[0].BytesWritten)
		}
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/str", nil))
}

func TestMiddlewareRegistersAndUnregisters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tr := newTestTracker()
	old := global
	global = tr
	defer func() { global = old }()

	r := gin.New()
	r.Use(Middleware())
	r.GET("/ping", func(c *gin.Context) {
		entries, _ := tr.Snapshot()
		if len(entries) != 1 {
			t.Errorf("handler should see 1 active entry, got %d", len(entries))
		}
		c.String(200, "ok")
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/ping", nil))

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if _, counts := tr.Snapshot(); counts["request"] != 0 {
		t.Fatal("entry should be unregistered after request completes")
	}
}

func TestMiddlewareCountResponseBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tr := newTestTracker()
	old := global
	global = tr
	defer func() { global = old }()

	r := gin.New()
	r.Use(Middleware())
	r.GET("/data", func(c *gin.Context) {
		c.String(200, strings.Repeat("x", 100))
		// 请求结束后 middleware 会注销，所以断言在 handler 内做
		entries, _ := tr.Snapshot()
		if len(entries) != 1 {
			t.Errorf("entries = %d", len(entries))
			return
		}
		if entries[0].BytesWritten != 100 {
			t.Errorf("bytes written = %d, want 100", entries[0].BytesWritten)
		}
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/data", nil))
}

// TrackReadCloser 必须把上游响应体读取字节计入 BytesRead，并在 Close 时注销。
func TestTrackReadCloserCountsAndUnregisters(t *testing.T) {
	tr := newTestTracker()
	h := tr.Register(Entry{Kind: "upstream", Remote: "https://up.test/blob"})

	body := TrackReadCloser(io.NopCloser(strings.NewReader("hello")), h)
	n, err := io.Copy(io.Discard, body)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if n != 5 {
		t.Fatalf("read %d bytes, want 5", n)
	}

	entries, counts := tr.Snapshot()
	if counts["upstream"] != 1 {
		t.Fatal("entry must stay registered while body is open")
	}
	if entries[0].BytesRead != 5 {
		t.Fatalf("bytes_read = %d, want 5", entries[0].BytesRead)
	}

	if err := body.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	_, counts = tr.Snapshot()
	if counts["upstream"] != 0 {
		t.Fatal("entry must be unregistered when body closes")
	}
}

// 活跃连接指标使用独立指标名：不能复用旧的 moonlight_active_connections，
// 否则同名指标从「无标签」变成「按 kind 标签」会让既有看板/告警静默失效。
func TestActiveConnectionsMetricName(t *testing.T) {
	tr := newTestTracker()
	h := tr.Register(Entry{Kind: "request"})
	defer h.Unregister()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var found, hasRequest bool
	for _, fam := range families {
		if fam.GetName() != "moonlight_connections_active" {
			continue
		}
		found = true
		for _, m := range fam.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "kind" && lp.GetValue() == "request" {
					hasRequest = true
				}
			}
		}
	}
	if !found {
		t.Fatal("metric moonlight_connections_active not found")
	}
	if !hasRequest {
		t.Fatal("metric has no kind=request series")
	}
}

func TestEntryDuration(t *testing.T) {
	e := Entry{StartedAt: time.Now().Add(-2 * time.Second)}
	if e.Duration() < time.Second {
		t.Fatal("duration should be >= 1s")
	}
	// Zero StartedAt: Duration() = time.Since(zero) 是巨大值，但 Register 会填 now
	e2 := Entry{}
	if e2.Duration() < 0 {
		t.Fatal("duration should be >= 0")
	}
}

func TestMiddlewareSetsWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tr := newTestTracker()
	old := global
	global = tr
	defer func() { global = old }()

	r := gin.New()
	r.Use(Middleware())
	r.GET("/ok", func(c *gin.Context) {
		if _, ok := c.Writer.(*countingResponseWriter); !ok {
			t.Errorf("writer should be wrapped, got %T", c.Writer)
		}
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/ok", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}
