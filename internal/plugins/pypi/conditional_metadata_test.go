package pypi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

func TestFetchArtifactMetadataReturnsReleaseAttributes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/simple/requests/":
			_, _ = w.Write([]byte(`<html><body><a href="requests-2.0.0.tar.gz">requests</a></body></html>`))
		case "/pypi/requests/json":
			_, _ = w.Write([]byte(`{"info":{"license":"Apache-2.0"},"releases":{"2.0.0":[{"filename":"requests-2.0.0.tar.gz","url":"https://files.example/requests-2.0.0.tar.gz","upload_time":"2024-01-02T03:04:05Z"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	plugin := NewPyPIPlugin(server.Client())
	metadata, err := plugin.FetchArtifactMetadata(context.Background(), server.URL, runtime.ArtifactKey{Name: "requests", Version: "2.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Attributes["license"] != "Apache-2.0" || metadata.Attributes["published_at"] != "2024-01-02T03:04:05Z" {
		t.Fatalf("attributes = %#v", metadata.Attributes)
	}
}
