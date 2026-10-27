package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coreruntime "github.com/dshmyz/moonlight-box/internal/core/runtime"
)

func TestRemoteClientGetBytesRejectsOversizedContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "67108865")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := NewRemoteClientWithRetry(NewTransportManager(time.Second, NewDNSResolver(nil)), 10, 1, time.Millisecond)
	_, _, err := client.GetBytes(context.Background(), srv.URL, RequestOptions{ReadTimeout: time.Second}, nil)
	if err == nil {
		t.Fatal("expected oversized response error")
	}
	if !strings.Contains(err.Error(), "response body too large") {
		t.Fatalf("error = %v, want response body too large", err)
	}
}

func TestTransportManagerSeparatesSecureAndInsecureTLSConfig(t *testing.T) {
	manager := NewTransportManager(time.Second, NewDNSResolver(nil))

	secure := manager.GetTransport(false)
	insecure := manager.GetTransport(true)
	if secure == insecure {
		t.Fatal("secure and insecure transports should be separate instances")
	}
	if secure.TLSClientConfig != nil && secure.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("secure transport must verify TLS certificates")
	}
	if insecure.TLSClientConfig == nil || !insecure.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("insecure transport should skip TLS verification only when requested")
	}
}

func TestRemoteClientBuildClientFallsBackToDefaultReadTimeout(t *testing.T) {
	// 调用方未指定 ReadTimeout 时，buildClient 必须兜底为 defaultReadTimeout，
	// 防止 client.Timeout=0 导致上游 accept 后不发响应时请求无限挂起。
	c := NewRemoteClientWithRetry(NewTransportManager(time.Second, NewDNSResolver(nil)), 10, 1, time.Millisecond)
	got := c.buildClient(RequestOptions{})
	if got.Timeout != c.defaultReadTimeout {
		t.Fatalf("buildClient() Timeout = %v, want default %v", got.Timeout, c.defaultReadTimeout)
	}
	if got.Timeout <= 0 {
		t.Fatal("buildClient() must not produce a client with zero Timeout")
	}
}

func TestRemoteClientBuildClientRespectsExplicitReadTimeout(t *testing.T) {
	c := NewRemoteClientWithRetry(NewTransportManager(time.Second, NewDNSResolver(nil)), 10, 1, time.Millisecond)
	got := c.buildClient(RequestOptions{ReadTimeout: 7 * time.Second})
	if got.Timeout != 7*time.Second {
		t.Fatalf("buildClient() Timeout = %v, want 7s", got.Timeout)
	}
}

// 共享 Transport 的 DialContext 按请求解析建连超时：仓库级值优先，否则全局值。
func TestResolveDialTimeoutPrefersRepoContextValue(t *testing.T) {
	if got := resolveDialTimeout(context.Background(), 10*time.Second); got != 10*time.Second {
		t.Fatalf("fallback dial timeout = %s, want 10s", got)
	}
	ctx := coreruntime.WithConnectTimeout(context.Background(), 3*time.Second)
	if got := resolveDialTimeout(ctx, 10*time.Second); got != 3*time.Second {
		t.Fatalf("repo dial timeout = %s, want 3s", got)
	}
}
