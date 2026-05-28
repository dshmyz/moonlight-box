package maven

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/plugins/testhelper"
)

func newCtx(method, path string, body io.Reader) (*runtime.RequestContext, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/repository/maven-test/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "maven-test", Format: "maven", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

func TestHandle_ArtifactDownload(t *testing.T) {
	p := NewMavenPlugin()
	art := testhelper.NewArtifact("maven", "artifact", map[string]string{
		"name":     "com.google.guava:guava",
		"group":    "com.google.guava",
		"artifact": "guava",
		"version":  "31.1-jre",
		"filename": "guava-31.1-jre.jar",
		"path":     "com/google/guava/guava/31.1-jre",
	}, "jar-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "com/google/guava/guava/31.1-jre/guava-31.1-jre.jar", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "jar-content" {
		t.Errorf("expected 'jar-content', got %q", w.Body.String())
	}
}

func TestHandle_Metadata(t *testing.T) {
	p := NewMavenPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":    "com.google.guava",
			"artifact": "guava",
			"version":  "31.1-jre",
		}, ""),
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":    "com.google.guava",
			"artifact": "guava",
			"version":  "31.0-jre",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "com/google/guava/guava/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/xml" {
		t.Errorf("expected application/xml, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<groupId>com.google.guava</groupId>") {
		t.Errorf("expected groupId in XML, got: %s", body)
	}
	if !strings.Contains(body, "<artifactId>guava</artifactId>") {
		t.Errorf("expected artifactId in XML")
	}
	if !strings.Contains(body, "<version>31.0-jre</version>") || !strings.Contains(body, "<version>31.1-jre</version>") {
		t.Errorf("expected both versions in XML")
	}
}

func TestHandle_Upload(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar", bytes.NewReader([]byte("jar-data")))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if len(rt.UploadedArts) != 1 {
		t.Fatalf("expected 1 uploaded artifact, got %d", len(rt.UploadedArts))
	}
	art := rt.UploadedArts[0]
	if art.Coordinates["group"] != "com.example" {
		t.Errorf("expected group 'com.example', got %q", art.Coordinates["group"])
	}
	if art.Coordinates["artifact"] != "app" {
		t.Errorf("expected artifact 'app', got %q", art.Coordinates["artifact"])
	}
	if art.Coordinates["version"] != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", art.Coordinates["version"])
	}
}

func TestHandle_Delete(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("DELETE", "com/example/app/1.0.0/app-1.0.0.jar", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestParseMavenPath(t *testing.T) {
	p := NewMavenPlugin()
	tests := []struct {
		path       string
		group      string
		artifact   string
		version    string
		filename   string
	}{
		{"com/google/guava/guava/31.1-jre/guava-31.1-jre.jar", "com.google.guava", "guava", "31.1-jre", "guava-31.1-jre.jar"},
		{"org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.pom", "org.apache.commons", "commons-lang3", "3.12.0", "commons-lang3-3.12.0.pom"},
		{"io/github/user/my-lib/1.0.0/my-lib-1.0.0-sources.jar", "io.github.user", "my-lib", "1.0.0", "my-lib-1.0.0-sources.jar"},
	}
	for _, tt := range tests {
		key, err := p.parseMavenPath(tt.path)
		if err != nil {
			t.Errorf("parseMavenPath(%q) error: %v", tt.path, err)
			continue
		}
		if key.Coordinates["group"] != tt.group {
			t.Errorf("%s: group %q, want %q", tt.path, key.Coordinates["group"], tt.group)
		}
		if key.Coordinates["artifact"] != tt.artifact {
			t.Errorf("%s: artifact %q, want %q", tt.path, key.Coordinates["artifact"], tt.artifact)
		}
		if key.Coordinates["version"] != tt.version {
			t.Errorf("%s: version %q, want %q", tt.path, key.Coordinates["version"], tt.version)
		}
		if key.Filename != tt.filename {
			t.Errorf("%s: filename %q, want %q", tt.path, key.Filename, tt.filename)
		}
	}
}

func TestHandle_QueryRemotePath(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "com/google/guava/guava/maven-metadata.xml", nil)
	p.Handle(ctx, rt)

	if len(rt.QueryCalls) != 1 {
		t.Fatalf("expected 1 query call, got %d", len(rt.QueryCalls))
	}
	if rt.QueryCalls[0].RemotePath != "com/google/guava/guava/maven-metadata.xml" {
		t.Errorf("unexpected RemotePath: %q", rt.QueryCalls[0].RemotePath)
	}
}

func TestFetchRemote_ParsesMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<metadata>
  <groupId>com.google</groupId>
  <artifactId>guava</artifactId>
  <versioning>
    <latest>31.1-jre</latest>
    <release>31.1-jre</release>
    <versions>
      <version>31.0-jre</version>
      <version>31.1-jre</version>
    </versions>
  </versioning>
</metadata>`))
	}))
	defer srv.Close()

	p := NewMavenPlugin()
	arts, err := p.FetchRemote(context.Background(), srv.URL, "com/google/guava/guava/maven-metadata.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 version artifacts, got %d", len(arts))
	}
	if arts[0].Coordinates["group"] != "com.google" {
		t.Errorf("expected group 'com.google', got %q", arts[0].Coordinates["group"])
	}
}

func TestFetchRemote_ArtifactPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewMavenPlugin()
	arts, err := p.FetchRemote(context.Background(), srv.URL, "com/google/guava/guava/31.1-jre/guava-31.1-jre.jar")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if arts[0].Coordinates["filename"] != "guava-31.1-jre.jar" {
		t.Errorf("expected filename 'guava-31.1-jre.jar', got %q", arts[0].Coordinates["filename"])
	}
}

func TestHandle_InvalidPath(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "too/short", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSnapshotMetadata(t *testing.T) {
	p := NewMavenPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":    "com.example",
			"artifact": "app",
			"version":  "1.0-20230101.120000-1",
		}, ""),
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":    "com.example",
			"artifact": "app",
			"version":  "1.0-SNAPSHOT",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "com/example/app/1.0-SNAPSHOT/maven-metadata.xml", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for SNAPSHOT metadata, got %d", w.Code)
	}
}
