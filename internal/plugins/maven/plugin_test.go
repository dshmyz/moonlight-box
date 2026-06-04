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

type closeTrackingReadCloser struct {
	*strings.Reader
	closed bool
}

func (r *closeTrackingReadCloser) Close() error {
	r.closed = true
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// parseMavenPath – artifact download path parsing
// ---------------------------------------------------------------------------

func TestParseMavenPath(t *testing.T) {
	p := NewMavenPlugin()
	tests := []struct {
		name     string
		path     string
		group    string
		artifact string
		version  string
		filename string
		wantErr  bool
	}{
		{
			name:     "standard jar",
			path:     "com/google/guava/guava/31.1-jre/guava-31.1-jre.jar",
			group:    "com.google.guava",
			artifact: "guava",
			version:  "31.1-jre",
			filename: "guava-31.1-jre.jar",
		},
		{
			name:     "pom file",
			path:     "org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.pom",
			group:    "org.apache.commons",
			artifact: "commons-lang3",
			version:  "3.12.0",
			filename: "commons-lang3-3.12.0.pom",
		},
		{
			name:     "sources jar",
			path:     "io/github/user/my-lib/1.0.0/my-lib-1.0.0-sources.jar",
			group:    "io.github.user",
			artifact: "my-lib",
			version:  "1.0.0",
			filename: "my-lib-1.0.0-sources.jar",
		},
		{
			name:     "classifier with platform",
			path:     "org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1-natives-windows.jar",
			group:    "org.lwjgl",
			artifact: "lwjgl",
			version:  "3.3.1",
			filename: "lwjgl-3.3.1-natives-windows.jar",
		},
		{
			name:     "version with dots and suffix",
			path:     "javax/validation/validation-api/2.0.1.Final/validation-api-2.0.1.Final.jar",
			group:    "javax.validation",
			artifact: "validation-api",
			version:  "2.0.1.Final",
			filename: "validation-api-2.0.1.Final.jar",
		},
		{
			name:     "single-char group segments",
			path:     "x/y/z/1.0/z-1.0.jar",
			group:    "x.y",
			artifact: "z",
			version:  "1.0",
			filename: "z-1.0.jar",
		},
		{
			name:     "deep group path",
			path:     "org/springframework/boot/spring-boot-starter-web/3.3.1/spring-boot-starter-web-3.3.1.jar",
			group:    "org.springframework.boot",
			artifact: "spring-boot-starter-web",
			version:  "3.3.1",
			filename: "spring-boot-starter-web-3.3.1.jar",
		},
		{
			name:     "snapshot version",
			path:     "com/example/lib/1.0-SNAPSHOT/lib-1.0-20230101.120000-1.jar",
			group:    "com.example",
			artifact: "lib",
			version:  "1.0-SNAPSHOT",
			filename: "lib-1.0-20230101.120000-1.jar",
		},
		{
			name:    "too short path",
			path:    "a/b",
			wantErr: true,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := p.parseMavenPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for path %q, got nil", tt.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMavenPath(%q) unexpected error: %v", tt.path, err)
			}
			if key.Coordinates["group"] != tt.group {
				t.Errorf("group = %q, want %q", key.Coordinates["group"], tt.group)
			}
			if key.Coordinates["artifact"] != tt.artifact {
				t.Errorf("artifact = %q, want %q", key.Coordinates["artifact"], tt.artifact)
			}
			if key.Coordinates["version"] != tt.version {
				t.Errorf("version = %q, want %q", key.Coordinates["version"], tt.version)
			}
			if key.Filename != tt.filename {
				t.Errorf("filename = %q, want %q", key.Filename, tt.filename)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FetchRemote – maven-metadata.xml parsing (XML is source of truth)
// ---------------------------------------------------------------------------

func TestFetchRemote_Metadata_Standard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<metadata>
  <groupId>com.google.guava</groupId>
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
	for _, a := range arts {
		if a.Coordinates["group"] != "com.google.guava" {
			t.Errorf("group = %q, want 'com.google.guava'", a.Coordinates["group"])
		}
		if a.Coordinates["artifact"] != "guava" {
			t.Errorf("artifact = %q, want 'guava'", a.Coordinates["artifact"])
		}
		if a.Coordinates["name"] != "com.google.guava:guava" {
			t.Errorf("name = %q, want 'com.google.guava:guava'", a.Coordinates["name"])
		}
	}
}

func TestFetchRemote_Metadata_ValidationApi(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<metadata>
  <groupId>javax.validation</groupId>
  <artifactId>validation-api</artifactId>
  <versioning>
    <latest>2.0.1.Final</latest>
    <release>2.0.1.Final</release>
    <versions>
      <version>1.1.0.Final</version>
      <version>2.0.0.Final</version>
      <version>2.0.1.Final</version>
    </versions>
  </versioning>
</metadata>`))
	}))
	defer srv.Close()

	p := NewMavenPlugin()
	arts, err := p.FetchRemote(context.Background(), srv.URL, "javax/validation/validation-api/maven-metadata.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("expected 3 version artifacts, got %d", len(arts))
	}
	for _, a := range arts {
		if a.Coordinates["group"] != "javax.validation" {
			t.Errorf("group = %q, want 'javax.validation'", a.Coordinates["group"])
		}
		if a.Coordinates["artifact"] != "validation-api" {
			t.Errorf("artifact = %q, want 'validation-api'", a.Coordinates["artifact"])
		}
		if a.Coordinates["version"] == "validation-api" {
			t.Errorf("version should NOT be 'validation-api', got %q", a.Coordinates["version"])
		}
	}
}

func TestFetchRemote_Metadata_ArtifactIdLooksLikeVersion(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		xmlGroup    string
		xmlArtifact string
		expectGroup string
		expectArt   string
		expectVers  []string
	}{
		{
			name:        "artifactId is validation-api",
			path:        "javax/validation/validation-api/maven-metadata.xml",
			xmlGroup:    "javax.validation",
			xmlArtifact: "validation-api",
			expectGroup: "javax.validation",
			expectArt:   "validation-api",
			expectVers:  []string{"1.0.0", "2.0.0"},
		},
		{
			name:        "artifactId contains SNAPSHOT word",
			path:        "com/example/snapshot-tools/maven-metadata.xml",
			xmlGroup:    "com.example",
			xmlArtifact: "snapshot-tools",
			expectGroup: "com.example",
			expectArt:   "snapshot-tools",
			expectVers:  []string{"1.0.0"},
		},
		{
			name:        "artifactId is api",
			path:        "com/example/api/maven-metadata.xml",
			xmlGroup:    "com.example",
			xmlArtifact: "api",
			expectGroup: "com.example",
			expectArt:   "api",
			expectVers:  []string{"0.1.0", "0.2.0"},
		},
		{
			name:        "deep groupId",
			path:        "org/springframework/boot/spring-boot-starter-web/maven-metadata.xml",
			xmlGroup:    "org.springframework.boot",
			xmlArtifact: "spring-boot-starter-web",
			expectGroup: "org.springframework.boot",
			expectArt:   "spring-boot-starter-web",
			expectVers:  []string{"3.0.0", "3.1.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xmlBody := `<?xml version="1.0"?>
<metadata>
  <groupId>` + tt.xmlGroup + `</groupId>
  <artifactId>` + tt.xmlArtifact + `</artifactId>
  <versioning>
    <latest>` + tt.expectVers[len(tt.expectVers)-1] + `</latest>
    <release>` + tt.expectVers[len(tt.expectVers)-1] + `</release>
    <versions>`
			for _, v := range tt.expectVers {
				xmlBody += `<version>` + v + `</version>`
			}
			xmlBody += `</versions>
  </versioning>
</metadata>`

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.Write([]byte(xmlBody))
			}))
			defer srv.Close()

			p := NewMavenPlugin()
			arts, err := p.FetchRemote(context.Background(), srv.URL, tt.path)
			if err != nil {
				t.Fatalf("FetchRemote failed: %v", err)
			}
			if len(arts) != len(tt.expectVers) {
				t.Fatalf("expected %d artifacts, got %d", len(tt.expectVers), len(arts))
			}
			for _, a := range arts {
				if a.Coordinates["group"] != tt.expectGroup {
					t.Errorf("group = %q, want %q", a.Coordinates["group"], tt.expectGroup)
				}
				if a.Coordinates["artifact"] != tt.expectArt {
					t.Errorf("artifact = %q, want %q", a.Coordinates["artifact"], tt.expectArt)
				}
			}
		})
	}
}

func TestFetchRemote_Metadata_SNAPSHOT_VersionPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<metadata modelVersion="1.1.0">
  <groupId>com.example</groupId>
  <artifactId>lib</artifactId>
  <version>1.0-SNAPSHOT</version>
  <versioning>
    <latest>1.0-20230101.120000-1</latest>
    <release>1.0-20230101.120000-1</release>
    <snapshot>
      <timestamp>20230101.120000</timestamp>
      <buildNumber>1</buildNumber>
    </snapshot>
    <snapshotVersions>
      <snapshotVersion>
        <extension>jar</extension>
        <value>1.0-20230101.120000-1</value>
        <updated>20230101120000</updated>
      </snapshotVersion>
    </snapshotVersions>
    <lastUpdated>20230101120000</lastUpdated>
  </versioning>
</metadata>`))
	}))
	defer srv.Close()

	p := NewMavenPlugin()
	arts, err := p.FetchRemote(context.Background(), srv.URL, "com/example/lib/1.0-SNAPSHOT/maven-metadata.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) < 1 {
		t.Fatalf("expected at least 1 artifact, got %d", len(arts))
	}
	a := arts[0]
	if a.Coordinates["group"] != "com.example" {
		t.Errorf("group = %q, want 'com.example'", a.Coordinates["group"])
	}
	if a.Coordinates["artifact"] != "lib" {
		t.Errorf("artifact = %q, want 'lib'", a.Coordinates["artifact"])
	}
	if a.Coordinates["base_version"] != "1.0-SNAPSHOT" {
		t.Errorf("base_version = %q, want '1.0-SNAPSHOT'", a.Coordinates["base_version"])
	}
}

func TestFetchRemote_Metadata_MissingGroupId(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<metadata>
  <artifactId>guava</artifactId>
  <versioning>
    <versions><version>1.0</version></versions>
  </versioning>
</metadata>`))
	}))
	defer srv.Close()

	p := NewMavenPlugin()
	_, err := p.FetchRemote(context.Background(), srv.URL, "com/google/guava/guava/maven-metadata.xml")
	if err == nil {
		t.Fatal("expected error for missing groupId, got nil")
	}
	if !strings.Contains(err.Error(), "missing groupId") {
		t.Errorf("error should mention missing groupId, got: %v", err)
	}
}

func TestFetchRemote_Metadata_MissingArtifactId(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<metadata>
  <groupId>com.google.guava</groupId>
  <versioning>
    <versions><version>1.0</version></versions>
  </versioning>
</metadata>`))
	}))
	defer srv.Close()

	p := NewMavenPlugin()
	_, err := p.FetchRemote(context.Background(), srv.URL, "com/google/guava/guava/maven-metadata.xml")
	if err == nil {
		t.Fatal("expected error for missing artifactId, got nil")
	}
	if !strings.Contains(err.Error(), "missing groupId or artifactId") {
		t.Errorf("error should mention missing groupId or artifactId, got: %v", err)
	}
}

func TestFetchRemote_ArtifactDownload(t *testing.T) {
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
	a := arts[0]
	if a.Coordinates["filename"] != "guava-31.1-jre.jar" {
		t.Errorf("filename = %q, want 'guava-31.1-jre.jar'", a.Coordinates["filename"])
	}
	if a.Coordinates["group"] != "com.google.guava" {
		t.Errorf("group = %q, want 'com.google.guava'", a.Coordinates["group"])
	}
	if a.Coordinates["artifact"] != "guava" {
		t.Errorf("artifact = %q, want 'guava'", a.Coordinates["artifact"])
	}
	if a.Coordinates["version"] != "31.1-jre" {
		t.Errorf("version = %q, want '31.1-jre'", a.Coordinates["version"])
	}
}

// ---------------------------------------------------------------------------
// Handle – artifact download
// ---------------------------------------------------------------------------

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

func TestHandle_ArtifactNotFound(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{Artifacts: nil}

	ctx, w := newCtx("GET", "com/google/guava/guava/99.9/guava-99.9.jar", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Handle – metadata (local cache query)
// ---------------------------------------------------------------------------

func TestHandle_Metadata_Standard(t *testing.T) {
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
		t.Errorf("missing groupId, body: %s", body)
	}
	if !strings.Contains(body, "<artifactId>guava</artifactId>") {
		t.Errorf("missing artifactId, body: %s", body)
	}
	if !strings.Contains(body, "<version>31.0-jre</version>") || !strings.Contains(body, "<version>31.1-jre</version>") {
		t.Errorf("missing versions, body: %s", body)
	}
}

func TestHandle_Metadata_ValidationApi(t *testing.T) {
	p := NewMavenPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":    "javax.validation",
			"artifact": "validation-api",
			"version":  "2.0.1.Final",
		}, ""),
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":    "javax.validation",
			"artifact": "validation-api",
			"version":  "1.1.0.Final",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "javax/validation/validation-api/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<groupId>javax.validation</groupId>") {
		t.Errorf("expected groupId 'javax.validation', got: %s", body)
	}
	if !strings.Contains(body, "<artifactId>validation-api</artifactId>") {
		t.Errorf("expected artifactId 'validation-api', got: %s", body)
	}
	if strings.Contains(body, "<version>validation-api</version>") {
		t.Errorf("version should NOT be 'validation-api', got: %s", body)
	}
}

func TestHandle_Metadata_ArtifactIdContainsSnapshot(t *testing.T) {
	p := NewMavenPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":    "com.example",
			"artifact": "snapshot-tools",
			"version":  "1.0.0",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "com/example/snapshot-tools/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<groupId>com.example</groupId>") {
		t.Errorf("expected groupId 'com.example', got: %s", body)
	}
	if !strings.Contains(body, "<artifactId>snapshot-tools</artifactId>") {
		t.Errorf("expected artifactId 'snapshot-tools', got: %s", body)
	}
	if !strings.Contains(body, "<version>1.0.0</version>") {
		t.Errorf("expected version '1.0.0', got: %s", body)
	}
}

func TestHandle_Metadata_SNAPSHOT(t *testing.T) {
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
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for SNAPSHOT metadata, got %d", w.Code)
	}
}

func TestHandle_Metadata_NotFound(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{Artifacts: nil}

	ctx, w := newCtx("GET", "com/google/nonexistent/nonexistent/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandle_Metadata_InvalidPath(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{}

	tests := []struct {
		name string
		path string
	}{
		{"single segment", "maven-metadata.xml"},
		{"two segments", "com/maven-metadata.xml"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, w := newCtx("GET", tt.path, nil)
			if err := p.Handle(ctx, rt); err != nil {
				t.Fatalf("Handle failed: %v", err)
			}
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400, got %d", tt.path, w.Code)
			}
		})
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

// ---------------------------------------------------------------------------
// Handle – upload
// ---------------------------------------------------------------------------

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
		t.Errorf("group = %q, want 'com.example'", art.Coordinates["group"])
	}
	if art.Coordinates["artifact"] != "app" {
		t.Errorf("artifact = %q, want 'app'", art.Coordinates["artifact"])
	}
	if art.Coordinates["version"] != "1.0.0" {
		t.Errorf("version = %q, want '1.0.0'", art.Coordinates["version"])
	}
}

func TestHandle_UploadArtifactLevelMetadataUsesCorrectCoordinates(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("PUT", "com/example/app/maven-metadata.xml", strings.NewReader("<metadata/>"))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.UploadedArts) != 1 {
		t.Fatalf("expected 1 uploaded artifact, got %d", len(rt.UploadedArts))
	}
	art := rt.UploadedArts[0]
	if art.Coordinates["name"] != "com.example:app" {
		t.Fatalf("name = %q, want com.example:app", art.Coordinates["name"])
	}
	if art.Coordinates["group"] != "com.example" {
		t.Fatalf("group = %q, want com.example", art.Coordinates["group"])
	}
	if art.Coordinates["artifact"] != "app" {
		t.Fatalf("artifact = %q, want app", art.Coordinates["artifact"])
	}
	if art.Coordinates["version"] != "" {
		t.Fatalf("version = %q, want empty artifact-level metadata version", art.Coordinates["version"])
	}
	if art.Coordinates["filename"] != "maven-metadata.xml" {
		t.Fatalf("filename = %q", art.Coordinates["filename"])
	}
}

func TestHandle_UploadSnapshotMetadataUsesCorrectCoordinates(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("PUT", "com/example/app/1.0-SNAPSHOT/maven-metadata.xml", strings.NewReader("<metadata/>"))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.UploadedArts) != 1 {
		t.Fatalf("expected 1 uploaded artifact, got %d", len(rt.UploadedArts))
	}
	art := rt.UploadedArts[0]
	if art.Coordinates["name"] != "com.example:app" {
		t.Fatalf("name = %q, want com.example:app", art.Coordinates["name"])
	}
	if art.Coordinates["version"] != "1.0-SNAPSHOT" {
		t.Fatalf("version = %q, want 1.0-SNAPSHOT", art.Coordinates["version"])
	}
	if art.Coordinates["path"] != "com/example/app/1.0-SNAPSHOT" {
		t.Fatalf("path = %q", art.Coordinates["path"])
	}
}

func TestHandle_ChecksumDownloadLooksUpOriginalArtifactCoordinates(t *testing.T) {
	p := NewMavenPlugin()
	art := testhelper.NewArtifact("maven", "artifact", map[string]string{
		"name":       "com.example:app",
		"group":      "com.example",
		"artifact":   "app",
		"version":    "1.0.0",
		"filename":   "app-1.0.0.jar",
		"path":       "com/example/app/1.0.0",
		"classifier": "",
	}, "jar-data")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "com/example/app/1.0.0/app-1.0.0.jar.sha1", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.GetCalls) != 1 {
		t.Fatalf("expected 1 GetArtifact call, got %d", len(rt.GetCalls))
	}
	if got := rt.GetCalls[0].Coordinates["filename"]; got != "app-1.0.0.jar" {
		t.Fatalf("lookup filename = %q, want original artifact filename", got)
	}
}

func TestHandle_UploadClosesExistingArtifactContent(t *testing.T) {
	p := NewMavenPlugin()
	content := &closeTrackingReadCloser{Reader: strings.NewReader("old")}
	existing := testhelper.NewArtifact("maven", "artifact", map[string]string{
		"name":       "com.example:app",
		"group":      "com.example",
		"artifact":   "app",
		"version":    "1.0.0",
		"filename":   "app-1.0.0.jar",
		"path":       "com/example/app/1.0.0",
		"classifier": "",
	}, "")
	existing.Content = content
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{existing}}

	ctx, w := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar", bytes.NewReader([]byte("new")))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 update, got %d", w.Code)
	}
	if !content.closed {
		t.Fatal("expected existing artifact content to be closed")
	}
}

// ---------------------------------------------------------------------------
// Handle – delete
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Handle – invalid path
// ---------------------------------------------------------------------------

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

func TestHandle_MethodNotAllowed(t *testing.T) {
	p := NewMavenPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("PATCH", "com/example/app/1.0.0/app-1.0.0.jar", nil)
	err := p.Handle(ctx, rt)
	if err == nil {
		t.Fatal("expected error for PATCH method, got nil")
	}
	if !strings.Contains(err.Error(), "method not allowed") {
		t.Errorf("expected 'method not allowed' error, got: %v", err)
	}
}
