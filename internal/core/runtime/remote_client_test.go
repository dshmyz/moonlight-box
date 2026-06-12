package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
