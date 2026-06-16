package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
