package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPRemoteClientOpenPreservesStatusHeadersAndUnreadBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("retry later"))
	}))
	defer srv.Close()

	result, err := NewHTTPRemoteClient(srv.Client()).Open(context.Background(), RemoteRequest{
		URL: srv.URL, Method: http.MethodGet,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusServiceUnavailable || result.Header.Get("ETag") != `"v1"` {
		t.Fatal("response lost")
	}
	body, _ := io.ReadAll(result.Body)
	if string(body) != "retry later" {
		t.Fatalf("body = %q", body)
	}
}

func TestHTTPRemoteClientOpenPropagatesHEADMethod(t *testing.T) {
	method := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	result, err := NewHTTPRemoteClient(srv.Client()).Open(context.Background(), RemoteRequest{
		URL: srv.URL, Method: http.MethodHead,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()

	if method != http.MethodHead {
		t.Fatalf("upstream method = %q, want HEAD", method)
	}
}

func TestHTTPRemoteClientOpenPreservesRedirectResponse(t *testing.T) {
	redirectTargetReached := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			redirectTargetReached = true
			w.WriteHeader(http.StatusOK)
			return
		}

		w.Header().Set("Location", "/target")
		w.Header().Set("X-Upstream", "redirect")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	result, err := NewHTTPRemoteClient(srv.Client()).Open(context.Background(), RemoteRequest{
		URL: srv.URL, Method: http.MethodGet,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()

	if result.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusTemporaryRedirect)
	}
	if result.Header.Get("Location") != "/target" || result.Header.Get("X-Upstream") != "redirect" {
		t.Fatalf("headers = %#v, want upstream redirect headers", result.Header)
	}
	if redirectTargetReached {
		t.Fatal("Open followed the upstream redirect")
	}
}

func TestHTTPRemoteClientFetchMetadataFallsBackToGETWhenHEADUnsupported(t *testing.T) {
	var sawHead, sawGet bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			sawHead = true
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			sawGet = true
			w.Header().Set("ETag", `"abc123"`)
			w.Header().Set("Content-Length", "42")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("metadata"))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()

	client := NewHTTPRemoteClient(srv.Client())
	meta, err := client.FetchMetadata(context.Background(), ArtifactKey{RemoteURL: srv.URL})
	if err != nil {
		t.Fatalf("FetchMetadata returned error: %v", err)
	}
	if !sawHead || !sawGet {
		t.Fatalf("expected HEAD then GET fallback, sawHead=%v sawGet=%v", sawHead, sawGet)
	}
	if !meta.Exists || meta.Digest != "abc123" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestHTTPRemoteClientFetchMetadataCapturesETagAndLastModified(t *testing.T) {
	modified := time.Date(2026, 6, 9, 10, 11, 12, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("unexpected method %s", r.Method)
		}
		w.Header().Set("ETag", `"upstream-etag"`)
		w.Header().Set("Last-Modified", modified.Format(http.TimeFormat))
		w.Header().Set("Content-Length", "42")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPRemoteClient(srv.Client())
	meta, err := client.FetchMetadata(context.Background(), ArtifactKey{RemoteURL: srv.URL})
	if err != nil {
		t.Fatalf("FetchMetadata returned error: %v", err)
	}
	if meta.ETag != "upstream-etag" {
		t.Fatalf("ETag = %q, want upstream-etag", meta.ETag)
	}
	if !meta.ModifiedAt.Equal(modified) {
		t.Fatalf("ModifiedAt = %s, want %s", meta.ModifiedAt, modified)
	}
}

// shortenStreamTimeouts 缩短流式超时变量并在测试结束后恢复。
func shortenStreamTimeouts(t *testing.T, ttfb, idle time.Duration) {
	oldTTFB, oldIdle := streamTTFBTimeout, streamIdleTimeout
	streamTTFBTimeout, streamIdleTimeout = ttfb, idle
	t.Cleanup(func() {
		streamTTFBTimeout, streamIdleTimeout = oldTTFB, oldIdle
	})
}

// TestHTTPRemoteClientFetchBlobTTFBTimeout 验证流式请求在响应头迟迟不到时
// 被 TTFB 定时器切断，而不是无限挂起。
func TestHTTPRemoteClientFetchBlobTTFBTimeout(t *testing.T) {
	shortenStreamTimeouts(t, 100*time.Millisecond, time.Minute)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // 迟迟不发响应头
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPRemoteClient(srv.Client())
	start := time.Now()
	_, err := client.FetchBlob(context.Background(), ArtifactKey{RemoteURL: srv.URL + "/big.tar"})
	if err == nil {
		t.Fatal("expected TTFB timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "TTFB") {
		t.Fatalf("error = %v, want TTFB timeout", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("FetchBlob took %s, TTFB timeout did not fire promptly", elapsed)
	}
}

// TestHTTPRemoteClientFetchBlobIdleTimeout 验证响应体断流超过读空闲阈值时
// Read 返回错误，连接不被无限占用。
func TestHTTPRemoteClientFetchBlobIdleTimeout(t *testing.T) {
	shortenStreamTimeouts(t, time.Minute, 100*time.Millisecond)

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("chunk-1\n")) // 先发一段
		w.(http.Flusher).Flush()
		<-block // 断流：不再发数据也不关闭
	}))
	defer srv.Close()
	defer close(block)

	client := NewHTTPRemoteClient(srv.Client())
	body, err := client.FetchBlob(context.Background(), ArtifactKey{RemoteURL: srv.URL + "/big.tar"})
	if err != nil {
		t.Fatalf("FetchBlob failed: %v", err)
	}
	defer body.Close()

	buf := make([]byte, 64)
	// 第一段应能读到
	n, err := body.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("first read failed: n=%d err=%v", n, err)
	}
	start := time.Now()
	for {
		n, err = body.Read(buf)
		if err != nil {
			break
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("Read blocked beyond idle timeout without error")
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("idle timeout took %s to fire, want ~100ms", elapsed)
	}
}

// TestHTTPRemoteClientFetchBlobSlowStreamSucceeds 验证慢速但持续的流式传输
// 不受任何总时长限制——大文件下载不再被 Client.Timeout 腰斩。
func TestHTTPRemoteClientFetchBlobSlowStreamSucceeds(t *testing.T) {
	shortenStreamTimeouts(t, time.Minute, time.Second)

	var want strings.Builder
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 8; i++ {
			chunk := fmt.Sprintf("chunk-%02d\n", i)
			w.Write([]byte(chunk))
			want.WriteString(chunk)
			w.(http.Flusher).Flush()
			time.Sleep(30 * time.Millisecond)
		}
	}))
	defer srv.Close()

	client := NewHTTPRemoteClient(srv.Client())
	body, err := client.FetchBlob(context.Background(), ArtifactKey{RemoteURL: srv.URL + "/big.tar"})
	if err != nil {
		t.Fatalf("FetchBlob failed: %v", err)
	}
	defer body.Close()

	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != want.String() {
		t.Fatalf("body mismatch:\n got %q\nwant %q", got, want.String())
	}
}
