package runtime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
