package service

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/gin-gonic/gin"
)

// newTestCASService 构造一个仅依赖 s.cfg（configSvc=nil）的 CASService，
// 使 getEffectiveConfig 直接走配置分支，便于隔离 TestConnection 的出站行为。
func newTestCASService(t *testing.T, serverURL string) *CASService {
	t.Helper()
	return NewCASService(&config.AuthConfig{
		CAS: config.CASConfig{
			Enabled:      true,
			ServerURL:    serverURL,
			LoginPath:    DefaultCASLoginPath,
			ValidatePath: DefaultCASValidatePath,
			ServiceURL:   "https://registry.example.com/api/v1/auth/cas/callback",
		},
	}, nil, nil, nil, nil)
}

func TestCASTestConnectionReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := newTestCASService(t, srv.URL).TestConnection(); err != nil {
		t.Fatalf("expected reachable CAS to return nil, got %v", err)
	}
}

func TestCASTestConnectionRedirectCountsAsReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CAS 登录端点典型行为：302 重定向到登录页，最终 200。
		if r.URL.Path == DefaultCASLoginPath {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := newTestCASService(t, srv.URL).TestConnection(); err != nil {
		t.Fatalf("expected 302-then-200 to count as reachable, got %v", err)
	}
}

func TestCASTestConnectionNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := newTestCASService(t, srv.URL).TestConnection()
	if err == nil {
		t.Fatal("expected 404 to be reported as unreachable, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected error to mention status 404, got %v", err)
	}
}

func TestCASTestConnectionUnreachableFailsFast(t *testing.T) {
	// 取一个空闲端口并立即关闭，连接会被拒绝——验证 casHTTPClient 在网络层失败时立即返回，
	// 而非依赖 server 端 WriteTimeout（默认 0）兜底。
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	err = newTestCASService(t, "http://"+addr).TestConnection()
	if err == nil {
		t.Fatal("expected unreachable CAS to return error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to connect to CAS server") {
		t.Fatalf("expected wrapped connection error, got %v", err)
	}
}

func TestCASTestConnectionNoServerURL(t *testing.T) {
	svc := NewCASService(&config.AuthConfig{
		CAS: config.CASConfig{Enabled: true},
	}, nil, nil, nil, nil)

	err := svc.TestConnection()
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected missing-server-url error, got %v", err)
	}
}

func TestCASValidateTicketFailsOnUnreachable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	// newTestCASService 配置了静态 ServiceURL，resolveServiceURL 走静态分支，
	// 上下文仅用于满足签名。
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/cas/callback?ticket=ST-test", nil)

	_, _, _, err = newTestCASService(t, "http://"+addr).ValidateTicket(c, "ST-test")
	if err == nil {
		t.Fatal("expected unreachable CAS to fail ticket validation, got nil")
	}
	if !strings.Contains(err.Error(), "CAS validation request failed") {
		t.Fatalf("expected wrapped validation error, got %v", err)
	}
}

// newDynamicCASService 构造一个 service_url 留空、依赖动态推导的 CASService。
func newDynamicCASService(t *testing.T, allowedHosts []string) *CASService {
	t.Helper()
	return NewCASService(&config.AuthConfig{
		CAS: config.CASConfig{
			Enabled:      true,
			ServerURL:    "https://cas.example.com/cas",
			LoginPath:    DefaultCASLoginPath,
			ValidatePath: DefaultCASValidatePath,
			AllowedHosts: allowedHosts,
		},
	}, nil, nil, nil, nil)
}

// testCtx 构造带指定 Host / 协议头的 gin 上下文。
func testCtx(t *testing.T, host string, forwardedHost, forwardedProto string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/cas/login", nil)
	c.Request.Host = host
	if forwardedHost != "" {
		c.Request.Header.Set("X-Forwarded-Host", forwardedHost)
	}
	if forwardedProto != "" {
		c.Request.Header.Set("X-Forwarded-Proto", forwardedProto)
	}
	return c
}

func TestCASResolveServiceURLStaticTakesPrecedence(t *testing.T) {
	svc := newTestCASService(t, "https://cas.example.com")
	cfg := svc.getEffectiveConfig()
	got, err := svc.resolveServiceURL(testCtx(t, "evil.example.net", "", ""), cfg)
	if err != nil {
		t.Fatalf("expected static service_url to be used regardless of Host, got err %v", err)
	}
	if got != "https://registry.example.com/api/v1/auth/cas/callback" {
		t.Fatalf("expected static service_url, got %q", got)
	}
}

func TestCASResolveServiceURLDynamicFromHost(t *testing.T) {
	svc := newDynamicCASService(t, []string{"repo-a.corp.com", "*.corp.com"})
	cfg := svc.getEffectiveConfig()

	got, err := svc.resolveServiceURL(testCtx(t, "repo-a.corp.com", "", "https"), cfg)
	if err != nil {
		t.Fatalf("expected exact host match to succeed, got %v", err)
	}
	if got != "https://repo-a.corp.com/login" {
		t.Fatalf("expected https://repo-a.corp.com/login, got %q", got)
	}

	// 通配后缀 + 反代透传的 X-Forwarded-Host / Proto
	got, err = svc.resolveServiceURL(testCtx(t, "internal:8080", "repo-b.corp.com", "https"), cfg)
	if err != nil {
		t.Fatalf("expected wildcard host match via X-Forwarded-Host to succeed, got %v", err)
	}
	if got != "https://repo-b.corp.com/login" {
		t.Fatalf("expected https://repo-b.corp.com/login, got %q", got)
	}
}

func TestCASResolveServiceURLRejectsUnlistedHost(t *testing.T) {
	svc := newDynamicCASService(t, []string{"repo-a.corp.com"})
	cfg := svc.getEffectiveConfig()

	if _, err := svc.resolveServiceURL(testCtx(t, "evil.example.net", "", ""), cfg); err == nil {
		t.Fatal("expected non-whitelisted host to be rejected, got nil")
	}
}

func TestCASResolveServiceURLNeedsWhitelist(t *testing.T) {
	// 无白名单时动态推导必须失败（安全默认），而不是把用户弹到任意 Host
	svc := newDynamicCASService(t, nil)
	cfg := svc.getEffectiveConfig()
	if _, err := svc.resolveServiceURL(testCtx(t, "repo-a.corp.com", "", ""), cfg); err == nil {
		t.Fatal("expected dynamic derivation without whitelist to fail, got nil")
	}
}

func TestCASResolveServiceURLRejectsBadProtoScheme(t *testing.T) {
	// X-Forwarded-Proto 仅接受 http/https；异常 scheme（代理透传/注入）回退到 TLS 探测（unix socket → http）
	svc := newDynamicCASService(t, []string{"repo-a.corp.com"})
	cfg := svc.getEffectiveConfig()

	got, err := svc.resolveServiceURL(testCtx(t, "repo-a.corp.com", "", "javascript"), cfg)
	if err != nil {
		t.Fatalf("unexpected error for bad scheme, got %v", err)
	}
	if got != "http://repo-a.corp.com/login" {
		t.Fatalf("expected http fallback after bad scheme, got %q", got)
	}
}

func TestCASResolveServiceURLFirstForwardedHostWins(t *testing.T) {
	svc := newDynamicCASService(t, []string{"good.corp.com"})
	cfg := svc.getEffectiveConfig()

	got, err := svc.resolveServiceURL(testCtx(t, "internal:8080", "good.corp.com, evil.example.net", "https"), cfg)
	if err != nil {
		t.Fatalf("expected first X-Forwarded-Host value to win, got %v", err)
	}
	if got != "https://good.corp.com/login" {
		t.Fatalf("expected https://good.corp.com/login, got %q", got)
	}
}

func TestCASIsEnabledInvariant(t *testing.T) {
	// enabled && server_url 且 service_url/allowed_hosts 至少其一 → 启用
	svc := newDynamicCASService(t, []string{"repo-a.corp.com"})
	if !svc.IsEnabled() {
		t.Fatal("expected enabled with allowed_hosts configured")
	}

	// 只填 server_url、既无 service_url 也无 allowed_hosts → 视为未启用，
	// 否则登录/回调必然失败（review: 恢复 enabled ⟹ resolvable service 不变量）
	svc = newDynamicCASService(t, nil)
	if svc.IsEnabled() {
		t.Fatal("expected CAS to be disabled when service_url and allowed_hosts are both empty")
	}

	// 静态 service_url 也满足不变量
	svc = newTestCASService(t, "https://cas.example.com")
	if !svc.IsEnabled() {
		t.Fatal("expected CAS to be enabled with static service_url")
	}
}

func TestCASIsHostAllowedNormalization(t *testing.T) {
	svc := newDynamicCASService(t, []string{"repo-a.corp.com", "*.corp.com", "*.WHITE.com"})
	cfg := svc.getEffectiveConfig()

	cases := []struct {
		host  string
		want  bool
	}{
		// 大小写不一致
		{"REPO-A.CORP.COM", true},
		// 通配后缀 + 大小写
		{"repo-b.Corp.com", true},
		// 裸后缀
		{"corp.com", true},
		// 带端口（host 与 pattern 均忽略端口）
		{"repo-a.corp.com:8080", true},
		// FQDN 结尾点
		{"repo-a.corp.com.", true},
		// 多级子域也命中 *.corp.com（通配即"以 .corp.com 结尾"，与历史语义一致）
		{"a.b.corp.com", true},
		// 不在白名单
		{"evil.example.net", false},
	}
	for _, tc := range cases {
		if got := svc.isHostAllowed(cfg, tc.host); got != tc.want {
			t.Errorf("isHostAllowed(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestCASIsHostAllowedBareSuffixCaseInsensitive(t *testing.T) {
	svc := newDynamicCASService(t, []string{"*.corp.com"})
	cfg := svc.getEffectiveConfig()
	if !svc.isHostAllowed(cfg, "CORP.COM") {
		t.Fatal("expected bare suffix match to be case-insensitive")
	}
}

func TestCASGetLoginURLDynamicService(t *testing.T) {
	svc := newDynamicCASService(t, []string{"repo-a.corp.com"})
	got, err := svc.GetLoginURL(testCtx(t, "repo-a.corp.com", "", "https"), "/admin/dashboard")
	if err != nil {
		t.Fatalf("expected login URL to build, got %v", err)
	}
	want := "https://cas.example.com/cas/cas/login?service=https%3A%2F%2Frepo-a.corp.com%2Flogin%3Fredirect%3D%252Fadmin%252Fdashboard"
	if got != want {
		t.Fatalf("login URL mismatch:\n got %q\nwant %q", got, want)
	}
}
