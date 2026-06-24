package npm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

func TestFetchArtifactMetadataReturnsVersionAttributes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lodash" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"versions":{"4.17.21":{"license":"MIT"}},"time":{"4.17.21":"2024-01-02T03:04:05Z"}}`))
	}))
	defer server.Close()

	plugin := NewNpmPlugin(server.Client())
	metadata, err := plugin.FetchArtifactMetadata(context.Background(), server.URL, runtime.ArtifactKey{Name: "lodash", Version: "4.17.21"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Attributes["license"] != "MIT" || metadata.Attributes["published_at"] != "2024-01-02T03:04:05Z" {
		t.Fatalf("attributes = %#v", metadata.Attributes)
	}
}
