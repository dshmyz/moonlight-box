package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type routerAlwaysBlocker struct{}

func (routerAlwaysBlocker) IsBlocked(string, string, string) bool     { return true }
func (routerAlwaysBlocker) BlockReason(string, string, string) string { return "blocked" }
func (routerAlwaysBlocker) IsBlockedWithAttrs(string, string, string, map[string]interface{}) (bool, string) {
	return false, ""
}
func (routerAlwaysBlocker) IsBlockedByPath(string, string) bool     { return true }
func (routerAlwaysBlocker) BlockReasonByPath(string, string) string { return "blocked" }

type recordingAuditLogger struct{ entries []AuditEntry }

func (a *recordingAuditLogger) Log(_ context.Context, entry AuditEntry) {
	a.entries = append(a.entries, entry)
}

type errBlockedPlugin struct{}

func (errBlockedPlugin) Name() string { return "npm" }
func (errBlockedPlugin) Handle(*RequestContext, RepositoryRuntime) error {
	return ErrBlocked
}

type errBlockedWithReasonPlugin struct{}

func (errBlockedWithReasonPlugin) Name() string { return "npm" }
func (errBlockedWithReasonPlugin) Handle(*RequestContext, RepositoryRuntime) error {
	return NewBlockedError("检测到严重安全漏洞")
}

type errUpstreamUnavailablePlugin struct{}

func (errUpstreamUnavailablePlugin) Name() string { return "npm" }
func (errUpstreamUnavailablePlugin) Handle(*RequestContext, RepositoryRuntime) error {
	return fmt.Errorf("open remote: %w", ErrUpstreamUnavailable)
}

type routerTestRuntime struct{}

func (routerTestRuntime) GetArtifact(context.Context, ArtifactKey) (*Artifact, error) {
	return nil, ErrNotFound
}
func (routerTestRuntime) QueryArtifacts(context.Context, ArtifactQuery) ([]*Artifact, error) {
	return nil, nil
}
func (routerTestRuntime) RenderProjection(context.Context, ProjectionQuery) (*ProjectionResult, error) {
	return nil, ErrNotFound
}
func (routerTestRuntime) OpenRemote(context.Context, RemoteOpenRequest) (*RemoteResponse, error) {
	return nil, ErrRemoteUnsupported
}
func (routerTestRuntime) BeginUpload(context.Context, UploadRequest) (UploadSession, error) {
	return nil, ErrNotFound
}
func (routerTestRuntime) DeleteArtifact(context.Context, ArtifactKey) error { return ErrNotFound }

func newRouterForTest(blocker PackageBlocker, audit AuditLogger, plugin ProtocolPlugin) *RepositoryRouter {
	manager := NewDefaultRepositoryManager()
	manager.Set(&Repository{Name: "npm", ID: "1", Format: "npm", Runtime: routerTestRuntime{}})
	router := NewRepositoryRouter(&Nexus3Resolver{}, manager)
	router.Blocker = blocker
	router.AuditLog = audit
	if plugin != nil {
		router.RegisterPlugin("npm", plugin)
	}
	return router
}

func TestRepositoryRouterLogsBlockForEarlyRuleMatch(t *testing.T) {
	audit := &recordingAuditLogger{}
	router := newRouterForTest(routerAlwaysBlocker{}, audit, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != "block" || audit.entries[0].ResponseStatus != http.StatusForbidden {
		t.Fatalf("entries = %#v, want one 403 block entry", audit.entries)
	}
	if audit.entries[0].ResourceName != "left-pad" {
		t.Fatalf("resource name = %q, want %q", audit.entries[0].ResourceName, "left-pad")
	}
}

func TestRepositoryRouterLogsBlockForRuntimeRejection(t *testing.T) {
	audit := &recordingAuditLogger{}
	router := newRouterForTest(nil, audit, errBlockedPlugin{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != "block" || audit.entries[0].ResponseStatus != http.StatusForbidden {
		t.Fatalf("entries = %#v, want one 403 block entry", audit.entries)
	}
	if audit.entries[0].ResourceName != "left-pad" {
		t.Fatalf("resource name = %q, want %q", audit.entries[0].ResourceName, "left-pad")
	}
}

func TestRepositoryRouterReturnsRuntimeBlockReason(t *testing.T) {
	router := newRouterForTest(nil, nil, errBlockedWithReasonPlugin{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if body := response.Body.String(); body != "Blocked: 检测到严重安全漏洞\n" {
		t.Fatalf("body = %q, want runtime block reason", body)
	}
}

func TestRouterMapsUpstreamUnavailableToBadGateway(t *testing.T) {
	router := newRouterForTest(nil, nil, errUpstreamUnavailablePlugin{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", response.Code)
	}
}

// 阻断审计日志必须包含 reason，否则用户无法从日志看到为什么被阻断。
func TestRepositoryRouterLogsBlockReasonForEarlyRuleMatch(t *testing.T) {
	audit := &recordingAuditLogger{}
	router := newRouterForTest(routerAlwaysBlocker{}, audit, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if len(audit.entries) != 1 {
		t.Fatalf("entries = %#v, want one block entry", audit.entries)
	}
	if audit.entries[0].Reason != "blocked" {
		t.Fatalf("Reason = %q, want %q (must record blocker.BlockReason)", audit.entries[0].Reason, "blocked")
	}
}

func TestRepositoryRouterLogsBlockReasonForRuntimeRejection(t *testing.T) {
	audit := &recordingAuditLogger{}
	router := newRouterForTest(nil, audit, errBlockedWithReasonPlugin{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if len(audit.entries) != 1 {
		t.Fatalf("entries = %#v, want one block entry", audit.entries)
	}
	if audit.entries[0].Reason != "检测到严重安全漏洞" {
		t.Fatalf("Reason = %q, want %q (must propagate BlockedError.Reason)", audit.entries[0].Reason, "检测到严重安全漏洞")
	}
}

// --- ErrCircuitOpen → 503 + Retry-After 测试（P0-B）---

// errCircuitOpenPlugin 模拟 Plugin 返回熔断打开错误。
type errCircuitOpenPlugin struct {
	retryAfter int
}

func (p *errCircuitOpenPlugin) Name() string { return "npm" }
func (p *errCircuitOpenPlugin) Handle(*RequestContext, RepositoryRuntime) error {
	return NewCircuitOpenError(p.retryAfter)
}

// TestRouterMapsCircuitOpenToServiceUnavailable
// 验证 ErrCircuitOpen 被映射为 503 + Retry-After 头。
func TestRouterMapsCircuitOpenToServiceUnavailable(t *testing.T) {
	router := newRouterForTest(nil, nil, &errCircuitOpenPlugin{retryAfter: 42})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "42" {
		t.Fatalf("Retry-After = %q, want %q", got, "42")
	}
}

// TestRouterMapsCircuitOpenWithZeroRetryAfter
// 验证 RetryAfter=0 时也返回 503（但 Retry-After 头为 "0"）。
// 这对应熔断即将转 half_open 的边界场景。
func TestRouterMapsCircuitOpenWithZeroRetryAfter(t *testing.T) {
	router := newRouterForTest(nil, nil, &errCircuitOpenPlugin{retryAfter: 0})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	// Retry-After: 0 是合法值（RFC 7231 允许），表示"立即重试可能成功"
	if got := response.Header().Get("Retry-After"); got != "0" {
		t.Fatalf("Retry-After = %q, want %q", got, "0")
	}
}

// TestRouterCircuitOpenDoesNotLogAsBlock
// 验证熔断打开不被误记为 block 审计日志（熔断不是安全规则阻断）。
func TestRouterCircuitOpenDoesNotLogAsBlock(t *testing.T) {
	audit := &recordingAuditLogger{}
	router := newRouterForTest(nil, audit, &errCircuitOpenPlugin{retryAfter: 30})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if len(audit.entries) != 0 {
		t.Fatalf("audit entries = %d, want 0 (circuit open is not a block)", len(audit.entries))
	}
}

// errCircuitOpenWrappedPlugin 模拟 Plugin 返回被包装的 ErrCircuitOpen
// （如 group.GetArtifact 返回的 firstErr 可能被 fmt.Errorf 包装）。
type errCircuitOpenWrappedPlugin struct{}

func (errCircuitOpenWrappedPlugin) Name() string { return "npm" }
func (errCircuitOpenWrappedPlugin) Handle(*RequestContext, RepositoryRuntime) error {
	return fmt.Errorf("group get artifact: %w", NewCircuitOpenError(60))
}

// TestRouterHandlesWrappedCircuitOpenError
// 验证 router 用 errors.Is 正确识别被包装的 ErrCircuitOpen。
func TestRouterHandlesWrappedCircuitOpenError(t *testing.T) {
	router := newRouterForTest(nil, nil, errCircuitOpenWrappedPlugin{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/repository/npm/left-pad", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (wrapped ErrCircuitOpen must be detected via errors.Is)", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want %q (must extract via errors.As)", got, "60")
	}
}
