package maven

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

func TestFetchArtifactMetadataReadsPOMLicense(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/org/example/demo/1.0.0/demo-1.0.0.pom" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<project><licenses><license><name>Apache-2.0</name></license></licenses></project>`))
	}))
	defer server.Close()

	plugin := NewMavenPlugin(server.Client())
	metadata, err := plugin.FetchArtifactMetadata(context.Background(), server.URL, runtime.ArtifactKey{
		Name: "org.example:demo", Version: "1.0.0", Qualifiers: map[string]string{"group": "org.example", "artifact": "demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Attributes["license"] != "Apache-2.0" {
		t.Fatalf("attributes = %#v", metadata.Attributes)
	}
}
