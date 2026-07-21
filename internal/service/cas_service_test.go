package service

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/config"
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

	_, _, _, err = newTestCASService(t, "http://"+addr).ValidateTicket("ST-test")
	if err == nil {
		t.Fatal("expected unreachable CAS to fail ticket validation, got nil")
	}
	if !strings.Contains(err.Error(), "CAS validation request failed") {
		t.Fatalf("expected wrapped validation error, got %v", err)
	}
}
