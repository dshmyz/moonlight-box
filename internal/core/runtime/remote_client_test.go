package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
