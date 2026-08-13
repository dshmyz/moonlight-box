package maven

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/plugins/testhelper"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func newHostedMavenRuntime(t *testing.T) runtime.RepositoryRuntime {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.Artifact{}, &model.Blob{}, &model.ArtifactBlob{}, &model.Package{}); err != nil {
		t.Fatal(err)
	}
	artifactSvc := service.NewArtifactService(db)
	metadataStore := storage.NewMetadataStoreWithArtifactService(db, artifactSvc)
	backend, err := storage.NewLocalStorage(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	blobStore := storage.NewCASBlobStore(backend, db)
	return &runtime.HostedRuntime{
		MetadataStore: metadataStore,
		BlobStore:     blobStore,
		RepositoryID:  "1",
	}
}

// ---------------------------------------------------------------------------
// parseMavenPath – artifact download path parsing
// ---------------------------------------------------------------------------

func TestParseMavenPath(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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
			if key.Qualifiers["group"] != tt.group {
				t.Errorf("group = %q, want %q", key.Qualifiers["group"], tt.group)
			}
			if key.Qualifiers["artifact"] != tt.artifact {
				t.Errorf("artifact = %q, want %q", key.Qualifiers["artifact"], tt.artifact)
			}
			if key.Version != tt.version {
				t.Errorf("version = %q, want %q", key.Version, tt.version)
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

	p := NewMavenPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "com/google/guava/guava/maven-metadata.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 version artifacts, got %d", len(arts))
	}
	for _, a := range arts {
		if a.Qualifiers["group"] != "com.google.guava" {
			t.Errorf("group = %q, want 'com.google.guava'", a.Qualifiers["group"])
		}
		if a.Qualifiers["artifact"] != "guava" {
			t.Errorf("artifact = %q, want 'guava'", a.Qualifiers["artifact"])
		}
		if a.Name != "com.google.guava:guava" {
			t.Errorf("name = %q, want 'com.google.guava:guava'", a.Name)
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

	p := NewMavenPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "javax/validation/validation-api/maven-metadata.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("expected 3 version artifacts, got %d", len(arts))
	}
	for _, a := range arts {
		if a.Qualifiers["group"] != "javax.validation" {
			t.Errorf("group = %q, want 'javax.validation'", a.Qualifiers["group"])
		}
		if a.Qualifiers["artifact"] != "validation-api" {
			t.Errorf("artifact = %q, want 'validation-api'", a.Qualifiers["artifact"])
		}
		if a.Version == "validation-api" {
			t.Errorf("version should NOT be 'validation-api', got %q", a.Version)
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

			p := NewMavenPlugin(http.DefaultClient)
			arts, err := p.FetchRemote(context.Background(), srv.URL, tt.path)
			if err != nil {
				t.Fatalf("FetchRemote failed: %v", err)
			}
			if len(arts) != len(tt.expectVers) {
				t.Fatalf("expected %d artifacts, got %d", len(tt.expectVers), len(arts))
			}
			for _, a := range arts {
				if a.Qualifiers["group"] != tt.expectGroup {
					t.Errorf("group = %q, want %q", a.Qualifiers["group"], tt.expectGroup)
				}
				if a.Qualifiers["artifact"] != tt.expectArt {
					t.Errorf("artifact = %q, want %q", a.Qualifiers["artifact"], tt.expectArt)
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

	p := NewMavenPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "com/example/lib/1.0-SNAPSHOT/maven-metadata.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) < 1 {
		t.Fatalf("expected at least 1 artifact, got %d", len(arts))
	}
	a := arts[0]
	if a.Qualifiers["group"] != "com.example" {
		t.Errorf("group = %q, want 'com.example'", a.Qualifiers["group"])
	}
	if a.Qualifiers["artifact"] != "lib" {
		t.Errorf("artifact = %q, want 'lib'", a.Qualifiers["artifact"])
	}
	if a.Qualifiers["base_version"] != "1.0-SNAPSHOT" {
		t.Errorf("base_version = %q, want '1.0-SNAPSHOT'", a.Qualifiers["base_version"])
	}
}

func TestFetchRemote_MetadataParsesFourteenDigitLastUpdated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<metadata modelVersion="1.1.0">
  <groupId>com.example</groupId>
  <artifactId>lib</artifactId>
  <versioning>
    <latest>1.2.3</latest>
    <release>1.2.3</release>
    <versions>
      <version>1.2.3</version>
    </versions>
    <lastUpdated>20230101120000</lastUpdated>
  </versioning>
</metadata>`))
	}))
	defer srv.Close()

	p := NewMavenPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "com/example/lib/maven-metadata.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 version artifact, got %d", len(arts))
	}
	if got := arts[0].Attributes["published_at"]; got != "2023-01-01T12:00:00Z" {
		t.Fatalf("published_at = %q, want 2023-01-01T12:00:00Z", got)
	}
}

func TestFetchRemote_Metadata_SNAPSHOT_VersionPathIncludesSnapshotFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<metadata modelVersion="1.1.0">
  <groupId>com.example</groupId>
  <artifactId>lib</artifactId>
  <version>1.0-SNAPSHOT</version>
  <versioning>
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
      <snapshotVersion>
        <extension>jar</extension>
        <classifier>sources</classifier>
        <value>1.0-20230101.120000-1</value>
        <updated>20230101120000</updated>
      </snapshotVersion>
      <snapshotVersion>
        <extension>pom</extension>
        <value>1.0-20230101.120000-1</value>
        <updated>20230101120000</updated>
      </snapshotVersion>
    </snapshotVersions>
    <lastUpdated>20230101120000</lastUpdated>
  </versioning>
</metadata>`))
	}))
	defer srv.Close()

	p := NewMavenPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "com/example/lib/1.0-SNAPSHOT/maven-metadata.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 4 {
		t.Fatalf("expected logical snapshot version plus 3 files, got %d", len(arts))
	}

	files := map[string]*runtime.Artifact{}
	for _, a := range arts {
		if a.Kind == runtime.KindArtifact {
			files[a.Filename] = a
		}
	}
	for _, filename := range []string{
		"lib-1.0-20230101.120000-1.jar",
		"lib-1.0-20230101.120000-1-sources.jar",
		"lib-1.0-20230101.120000-1.pom",
	} {
		file := files[filename]
		if file == nil {
			t.Fatalf("missing snapshot file artifact %s", filename)
		}
		if file.Version != "1.0-SNAPSHOT" {
			t.Fatalf("%s version = %q, want 1.0-SNAPSHOT", filename, file.Version)
		}
		if _, ok := file.Attributes["default_visible"]; ok {
			t.Fatalf("%s should not persist default_visible, got %q", filename, file.Attributes["default_visible"])
		}
		if _, ok := file.Attributes["display_group"]; ok {
			t.Fatalf("%s should not persist display_group, got %q", filename, file.Attributes["display_group"])
		}
	}
	if files["lib-1.0-20230101.120000-1-sources.jar"].Qualifiers["classifier"] != "sources" {
		t.Fatalf("sources classifier = %q", files["lib-1.0-20230101.120000-1-sources.jar"].Qualifiers["classifier"])
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

	p := NewMavenPlugin(http.DefaultClient)
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

	p := NewMavenPlugin(http.DefaultClient)
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

	p := NewMavenPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "com/google/guava/guava/31.1-jre/guava-31.1-jre.jar")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	a := arts[0]
	if a.Filename != "guava-31.1-jre.jar" {
		t.Errorf("filename = %q, want 'guava-31.1-jre.jar'", a.Filename)
	}
	if a.Qualifiers["group"] != "com.google.guava" {
		t.Errorf("group = %q, want 'com.google.guava'", a.Qualifiers["group"])
	}
	if a.Qualifiers["artifact"] != "guava" {
		t.Errorf("artifact = %q, want 'guava'", a.Qualifiers["artifact"])
	}
	if a.Version != "31.1-jre" {
		t.Errorf("version = %q, want '31.1-jre'", a.Version)
	}
}

// ---------------------------------------------------------------------------
// Handle – artifact download
// ---------------------------------------------------------------------------

func TestHandle_ArtifactDownload(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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

func TestHandle_ArtifactHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("maven", "artifact", map[string]string{
		"name":     "com.google.guava:guava",
		"group":    "com.google.guava",
		"artifact": "guava",
		"version":  "31.1-jre",
		"filename": "guava-31.1-jre.jar",
		"path":     "com/google/guava/guava/31.1-jre",
	}, "jar-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "com/google/guava/guava/31.1-jre/guava-31.1-jre.jar", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if disp := w.Header().Get("Content-Disposition"); disp == "" {
		t.Fatal("expected Content-Disposition header")
	}
}

func TestHandle_ArtifactRangeReturnsPartialContent(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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
	ctx.Request.Header.Set("Range", "bytes=4-10")
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", w.Code)
	}
	if body := w.Body.String(); body != "content" {
		t.Fatalf("expected partial body %q, got %q", "content", body)
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 4-10/11" {
		t.Fatalf("expected Content-Range bytes 4-10/11, got %q", got)
	}
}

func TestHandle_ArtifactNotFound(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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

func TestHandle_Metadata_PrefersOriginalCachedContent(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	original := `<?xml version="1.0"?>
<metadata>
  <groupId>com.google.guava</groupId>
  <artifactId>guava</artifactId>
  <versioning>
    <latest>31.1-jre</latest>
    <versions><version>31.1-jre</version></versions>
  </versioning>
  <!-- original-upstream-marker -->
</metadata>`
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("maven", runtime.KindMetadata, map[string]string{
			"name":        "com.google.guava:guava",
			"path":        "com/google/guava/guava",
			"filename":    "maven-metadata.xml",
			"group":       "com.google.guava",
			"artifact":    "guava",
			"remote_path": "com/google/guava/guava/maven-metadata.xml",
		}, original),
		testhelper.NewArtifact("maven", runtime.KindVersion, map[string]string{
			"name":        "com.google.guava:guava",
			"group":       "com.google.guava",
			"artifact":    "guava",
			"version":     "31.1-jre",
			"remote_path": "com/google/guava/guava/maven-metadata.xml",
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
	if body := w.Body.String(); body != original {
		t.Fatalf("expected original metadata content, got: %s", body)
	}
}

func TestHandle_MetadataHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	original := `<metadata><groupId>com.google.guava</groupId><artifactId>guava</artifactId></metadata>`
	art := testhelper.NewArtifact("maven", runtime.KindMetadata, map[string]string{
		"name":     "com.google.guava:guava",
		"path":     "com/google/guava/guava",
		"filename": "maven-metadata.xml",
		"group":    "com.google.guava",
		"artifact": "guava",
	}, original)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "com/google/guava/guava/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/xml" {
		t.Fatalf("expected application/xml, got %q", ct)
	}
}

func TestHandle_Metadata_Standard(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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

func TestHandle_MetadataUsesMavenVersionOrdering(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":    "com.example",
			"artifact": "app",
			"version":  "1.9.0",
		}, ""),
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":    "com.example",
			"artifact": "app",
			"version":  "1.10.0",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "com/example/app/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<latest>1.10.0</latest>") {
		t.Fatalf("expected latest 1.10.0 with Maven ordering, got: %s", body)
	}
}

func TestSortMavenVersionsUsesQualifierOrdering(t *testing.T) {
	versions := []string{
		"1.0-sp-1",
		"1.0",
		"1.0-SNAPSHOT",
		"1.0-beta-1",
		"1.0-rc-1",
		"1.0-alpha-1",
		"1.0.Final",
	}

	sortMavenVersions(versions)

	want := []string{
		"1.0-alpha-1",
		"1.0-beta-1",
		"1.0-rc-1",
		"1.0-SNAPSHOT",
		"1.0",
		"1.0.Final",
		"1.0-sp-1",
	}
	if !reflect.DeepEqual(versions, want) {
		t.Fatalf("versions = %#v, want %#v", versions, want)
	}
}

func TestHandle_HostedReleaseMetadataGeneratedFromUploadedArtifact(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	uploadCtx, uploadW := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar", bytes.NewReader([]byte("jar-content")))
	if err := p.Handle(uploadCtx, rt); err != nil {
		t.Fatalf("upload Handle failed: %v", err)
	}
	if uploadW.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d", uploadW.Code)
	}

	ctx, w := newCtx("GET", "com/example/app/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("metadata Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"<groupId>com.example</groupId>",
		"<artifactId>app</artifactId>",
		"<version>1.0.0</version>",
		"<latest>1.0.0</latest>",
		"<release>1.0.0</release>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metadata missing %s: %s", want, body)
		}
	}
}

func TestHandle_HostedReleaseMetadataIgnoresUploadedMetadataWhenArtifactsExist(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	metaCtx, metaW := newCtx("PUT", "com/example/app/maven-metadata.xml", strings.NewReader("<metadata><stale>true</stale></metadata>"))
	if err := p.Handle(metaCtx, rt); err != nil {
		t.Fatalf("metadata upload Handle failed: %v", err)
	}
	if metaW.Code != http.StatusCreated {
		t.Fatalf("expected metadata upload 201, got %d body=%q", metaW.Code, metaW.Body.String())
	}

	uploadCtx, uploadW := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar", bytes.NewReader([]byte("jar-content")))
	if err := p.Handle(uploadCtx, rt); err != nil {
		t.Fatalf("artifact upload Handle failed: %v", err)
	}
	if uploadW.Code != http.StatusCreated {
		t.Fatalf("expected artifact upload 201, got %d body=%q", uploadW.Code, uploadW.Body.String())
	}

	ctx, w := newCtx("GET", "com/example/app/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("metadata Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "<stale>true</stale>") {
		t.Fatalf("expected hosted metadata projection, got uploaded metadata: %s", body)
	}
	if !strings.Contains(body, "<version>1.0.0</version>") {
		t.Fatalf("expected generated metadata to include uploaded version, got: %s", body)
	}
}

func TestHandle_HostedSnapshotMetadataGeneratedFromUploadedArtifact(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	uploadCtx, uploadW := newCtx("PUT", "com/example/app/1.0-SNAPSHOT/app-1.0-20260609.120000-1.jar", bytes.NewReader([]byte("jar-snapshot-content")))
	if err := p.Handle(uploadCtx, rt); err != nil {
		t.Fatalf("snapshot upload Handle failed: %v", err)
	}
	if uploadW.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d", uploadW.Code)
	}
	arts, err := rt.QueryArtifacts(context.Background(), runtime.ArtifactQuery{
		RepositoryID: "1",
		Format:       "maven",
		Name:         "com.example:app",
		Version:      "1.0-SNAPSHOT",
	})
	if err != nil {
		t.Fatalf("query uploaded snapshot artifact: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 uploaded snapshot artifact, got %d", len(arts))
	}
	if _, ok := arts[0].Attributes["default_visible"]; ok {
		t.Fatalf("snapshot upload should not persist default_visible, got %q", arts[0].Attributes["default_visible"])
	}
	if _, ok := arts[0].Attributes["display_group"]; ok {
		t.Fatalf("snapshot upload should not persist display_group, got %q", arts[0].Attributes["display_group"])
	}

	ctx, w := newCtx("GET", "com/example/app/1.0-SNAPSHOT/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("snapshot metadata Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"<groupId>com.example</groupId>",
		"<artifactId>app</artifactId>",
		"<version>1.0-SNAPSHOT</version>",
		"<snapshot>",
		"<buildNumber>1</buildNumber>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metadata missing %s: %s", want, body)
		}
	}
}

func TestHandle_HostedSnapshotMetadataUsesLatestTimestampedBuild(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	uploads := []string{
		"com/example/app/1.0-SNAPSHOT/app-1.0-20260609.100000-1.jar",
		"com/example/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2.jar",
	}
	for _, path := range uploads {
		uploadCtx, uploadW := newCtx("PUT", path, bytes.NewReader([]byte(path)))
		if err := p.Handle(uploadCtx, rt); err != nil {
			t.Fatalf("upload %s failed: %v", path, err)
		}
		if uploadW.Code != http.StatusCreated {
			t.Fatalf("upload %s status = %d", path, uploadW.Code)
		}
	}

	ctx, w := newCtx("GET", "com/example/app/1.0-SNAPSHOT/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("metadata Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"<timestamp>20260609.120000</timestamp>",
		"<buildNumber>2</buildNumber>",
		"<value>1.0-20260609.120000-2</value>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metadata missing latest build marker %s: %s", want, body)
		}
	}
	if strings.Contains(body, "1.0-20260609.100000-1</value>") {
		t.Fatalf("metadata should not select older build: %s", body)
	}
}

func TestHandle_HostedSnapshotMetadataIncludesClassifiers(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	uploads := []string{
		"com/example/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2.jar",
		"com/example/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2-sources.jar",
		"com/example/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2-javadoc.jar",
	}
	for _, path := range uploads {
		uploadCtx, uploadW := newCtx("PUT", path, bytes.NewReader([]byte(path)))
		if err := p.Handle(uploadCtx, rt); err != nil {
			t.Fatalf("upload %s failed: %v", path, err)
		}
		if uploadW.Code != http.StatusCreated {
			t.Fatalf("upload %s status = %d", path, uploadW.Code)
		}
	}

	ctx, w := newCtx("GET", "com/example/app/1.0-SNAPSHOT/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("metadata Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"<classifier>sources</classifier>",
		"<classifier>javadoc</classifier>",
		"<extension>jar</extension>",
		"<value>1.0-20260609.120000-2</value>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metadata missing classifier marker %s: %s", want, body)
		}
	}
}

func TestHandle_HostedSnapshotMetadataIncludesJarAndPomExtensions(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	uploads := []string{
		"com/example/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2.jar",
		"com/example/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2.pom",
	}
	for _, path := range uploads {
		uploadCtx, uploadW := newCtx("PUT", path, bytes.NewReader([]byte(path)))
		if err := p.Handle(uploadCtx, rt); err != nil {
			t.Fatalf("upload %s failed: %v", path, err)
		}
		if uploadW.Code != http.StatusCreated {
			t.Fatalf("upload %s status = %d", path, uploadW.Code)
		}
	}

	ctx, w := newCtx("GET", "com/example/app/1.0-SNAPSHOT/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("metadata Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Count(body, "<extension>jar</extension>") != 1 {
		t.Fatalf("expected one jar snapshotVersion, got: %s", body)
	}
	if strings.Count(body, "<extension>pom</extension>") != 1 {
		t.Fatalf("expected one pom snapshotVersion, got: %s", body)
	}
}

func TestHandle_HostedSnapshotMetadataIncludesFileExtension(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	uploads := []string{
		"com/example/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2.jar",
	}
	for _, path := range uploads {
		uploadCtx, uploadW := newCtx("PUT", path, bytes.NewReader([]byte(path)))
		if err := p.Handle(uploadCtx, rt); err != nil {
			t.Fatalf("upload %s failed: %v", path, err)
		}
		if uploadW.Code != http.StatusCreated {
			t.Fatalf("upload %s status = %d", path, uploadW.Code)
		}
	}

	ctx, w := newCtx("GET", "com/example/app/1.0-SNAPSHOT/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("metadata Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// fileExtension 字段应出现在 snapshotVersion 中
	if !strings.Contains(body, "<fileExtension>") {
		t.Fatalf("expected <fileExtension> in snapshot metadata, got: %s", body)
	}
}

func TestHandle_HostedSnapshotMetadataTarGzDoubleExtension(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	// .tar.gz 双扩展名应被正确识别为 "tar.gz"，而不是 "gz"
	uploadPath := "com/example/app/1.0-SNAPSHOT/app-1.0-20260609.120000-2.tar.gz"
	uploadCtx, uploadW := newCtx("PUT", uploadPath, bytes.NewReader([]byte("tarball-content")))
	if err := p.Handle(uploadCtx, rt); err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if uploadW.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", uploadW.Code)
	}

	ctx, w := newCtx("GET", "com/example/app/1.0-SNAPSHOT/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("metadata Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// extension 应为 "tar.gz"，不是 "gz"
	if !strings.Contains(body, "<extension>tar.gz</extension>") {
		t.Fatalf("expected <extension>tar.gz</extension> in snapshot metadata, got: %s", body)
	}
	if strings.Contains(body, "<extension>gz</extension>") {
		t.Fatalf("should not have <extension>gz</extension> for .tar.gz file, got: %s", body)
	}
}

func TestHandle_MetadataChecksumForDynamicMetadata(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	uploadCtx, uploadW := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar", bytes.NewReader([]byte("jar-content")))
	if err := p.Handle(uploadCtx, rt); err != nil {
		t.Fatalf("upload Handle failed: %v", err)
	}
	if uploadW.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d", uploadW.Code)
	}

	ctx, w := newCtx("GET", "com/example/app/maven-metadata.xml.sha1", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("metadata checksum Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for metadata checksum, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "maven-metadata.xml") {
		t.Errorf("expected checksum body to contain 'maven-metadata.xml', got %q", body)
	}
	sha1Hex := strings.TrimSpace(strings.Split(body, " ")[0])
	if len(sha1Hex) != 40 {
		t.Errorf("expected 40 hex chars for SHA1, got %d: %q", len(sha1Hex), body)
	}
}

func TestHandle_MetadataChecksumMatchesDynamicMetadataBytes(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	uploadCtx, uploadW := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar", bytes.NewReader([]byte("jar-content")))
	if err := p.Handle(uploadCtx, rt); err != nil {
		t.Fatalf("upload Handle failed: %v", err)
	}
	if uploadW.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d", uploadW.Code)
	}

	metaCtx, metaW := newCtx("GET", "com/example/app/maven-metadata.xml", nil)
	if err := p.Handle(metaCtx, rt); err != nil {
		t.Fatalf("metadata Handle failed: %v", err)
	}
	if metaW.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d", metaW.Code)
	}

	checksumCtx, checksumW := newCtx("GET", "com/example/app/maven-metadata.xml.sha1", nil)
	if err := p.Handle(checksumCtx, rt); err != nil {
		t.Fatalf("metadata checksum Handle failed: %v", err)
	}
	if checksumW.Code != http.StatusOK {
		t.Fatalf("expected checksum 200, got %d", checksumW.Code)
	}

	sum := sha1.Sum(metaW.Body.Bytes())
	want := hex.EncodeToString(sum[:])
	got := strings.TrimSpace(strings.Split(checksumW.Body.String(), " ")[0])
	if got != want {
		t.Fatalf("metadata sha1 = %s, want %s for body %q", got, want, metaW.Body.String())
	}
}

// TestHandle_MetadataChecksumMatchesServedContentWithUploadedMetadata 复现 mvn deploy 场景：
// 客户端上传制品后又上传 maven-metadata.xml（内容与聚合结果不同）。GET metadata 返回
// 动态聚合内容，checksum 必须与 GET 返回的内容同源，而不是与上传的 metadata blob 同源，
// 否则客户端会报 "Checksum validation failed" 警告。
func TestHandle_MetadataChecksumMatchesServedContentWithUploadedMetadata(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	uploadCtx, uploadW := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar", bytes.NewReader([]byte("jar-content")))
	if err := p.Handle(uploadCtx, rt); err != nil {
		t.Fatalf("upload Handle failed: %v", err)
	}
	if uploadW.Code != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d", uploadW.Code)
	}

	// deploy 会上传 maven-metadata.xml（及其 checksum），模拟一份与聚合内容不同的 blob
	uploadedMeta := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <groupId>com.example</groupId>
  <artifactId>app</artifactId>
  <versioning>
    <latest>1.0.0</latest>
    <versions>
      <version>1.0.0</version>
    </versions>
  </versioning>
</metadata>`
	metaCtx, metaW := newCtx("PUT", "com/example/app/maven-metadata.xml", strings.NewReader(uploadedMeta))
	if err := p.Handle(metaCtx, rt); err != nil {
		t.Fatalf("metadata upload Handle failed: %v", err)
	}
	if metaW.Code != http.StatusCreated {
		t.Fatalf("expected metadata upload 201, got %d body=%q", metaW.Code, metaW.Body.String())
	}

	metaGetCtx, metaGetW := newCtx("GET", "com/example/app/maven-metadata.xml", nil)
	if err := p.Handle(metaGetCtx, rt); err != nil {
		t.Fatalf("metadata Handle failed: %v", err)
	}
	if metaGetW.Code != http.StatusOK {
		t.Fatalf("expected metadata 200, got %d", metaGetW.Code)
	}

	checksumCtx, checksumW := newCtx("GET", "com/example/app/maven-metadata.xml.sha1", nil)
	if err := p.Handle(checksumCtx, rt); err != nil {
		t.Fatalf("metadata checksum Handle failed: %v", err)
	}
	if checksumW.Code != http.StatusOK {
		t.Fatalf("expected checksum 200, got %d", checksumW.Code)
	}

	sum := sha1.Sum(metaGetW.Body.Bytes())
	want := hex.EncodeToString(sum[:])
	got := strings.TrimSpace(strings.Split(checksumW.Body.String(), " ")[0])
	if got != want {
		t.Fatalf("metadata sha1 = %s, want %s (must match served metadata content)", got, want)
	}
	uploadedSum := sha1.Sum([]byte(uploadedMeta))
	if got == hex.EncodeToString(uploadedSum[:]) {
		t.Fatalf("checksum must not be computed from the uploaded metadata blob")
	}
}

func TestHandle_Metadata_ValidationApi(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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
	p := NewMavenPlugin(http.DefaultClient)
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
	p := NewMavenPlugin(http.DefaultClient)
	// version 记录在生产中由 fetchMetadata 回源产生（无 RemotePath），
	// ProxyRuntime.QueryArtifacts 回源后直接返回 fetched 列表，不应用 RemotePath 过滤。
	// MockRuntime 无 FetchRemote，需给 artifact 设置 RemotePath 以模拟回源后命中缓存的效果。
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":       "com.example",
			"artifact":    "app",
			"version":     "1.0-20230101.120000-1",
			"remote_path": "com/example/app/1.0-SNAPSHOT/maven-metadata.xml",
		}, ""),
		testhelper.NewArtifact("maven", "version", map[string]string{
			"group":       "com.example",
			"artifact":    "app",
			"version":     "1.0-SNAPSHOT",
			"remote_path": "com/example/app/1.0-SNAPSHOT/maven-metadata.xml",
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

func TestHandle_Metadata_SNAPSHOT_PathBAggregatesTimestampFromArtifactFilenames(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	// 模拟 proxy 回源 fetchMetadata 后的场景：
	// - 1 个 KindVersion 记录（version=1.0-SNAPSHOT，无 Filename）
	// - 2 个 KindArtifact 记录（Filename 为时间戳文件名，模拟两次构建）
	// 路径B应从文件名解析最新 timestamp+buildNumber，生成正确的 <value>。
	// 修复前路径B用 a.Version 作为 <value>（即 1.0-SNAPSHOT），导致客户端无法下载时间戳文件。
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("maven", runtime.KindVersion, map[string]string{
			"group":       "com.example",
			"artifact":    "app",
			"version":     "1.0-SNAPSHOT",
			"remote_path": "com/example/app/1.0-SNAPSHOT/maven-metadata.xml",
		}, ""),
		testhelper.NewArtifact("maven", runtime.KindArtifact, map[string]string{
			"group":       "com.example",
			"artifact":    "app",
			"version":     "1.0-SNAPSHOT",
			"filename":    "app-1.0-20260609.100000-1.jar",
			"remote_path": "com/example/app/1.0-SNAPSHOT/maven-metadata.xml",
		}, ""),
		testhelper.NewArtifact("maven", runtime.KindArtifact, map[string]string{
			"group":       "com.example",
			"artifact":    "app",
			"version":     "1.0-SNAPSHOT",
			"filename":    "app-1.0-20260609.120000-2.jar",
			"remote_path": "com/example/app/1.0-SNAPSHOT/maven-metadata.xml",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "com/example/app/1.0-SNAPSHOT/maven-metadata.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// 应选最新构建 20260609.120000-2
	for _, want := range []string{
		"<timestamp>20260609.120000</timestamp>",
		"<buildNumber>2</buildNumber>",
		"<value>1.0-20260609.120000-2</value>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metadata missing %s: %s", want, body)
		}
	}
	// 不应出现用 1.0-SNAPSHOT 作为 value 的旧 bug
	if strings.Contains(body, "<value>1.0-SNAPSHOT</value>") {
		t.Fatalf("metadata should not use base version as value: %s", body)
	}
}

func TestHandle_Metadata_NotFound(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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
	p := NewMavenPlugin(http.DefaultClient)
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
	p := NewMavenPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "com/google/guava/guava/maven-metadata.xml", nil)
	p.Handle(ctx, rt)

	if len(rt.QueryCalls) < 1 {
		t.Fatalf("expected at least 1 query call, got %d", len(rt.QueryCalls))
	}
	if rt.QueryCalls[0].RemotePath != "com/google/guava/guava/maven-metadata.xml" {
		t.Errorf("unexpected RemotePath: %q", rt.QueryCalls[0].RemotePath)
	}
}

// ---------------------------------------------------------------------------
// Handle – upload
// ---------------------------------------------------------------------------

func TestHandle_Upload(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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
	if art.Qualifiers["group"] != "com.example" {
		t.Errorf("group = %q, want 'com.example'", art.Qualifiers["group"])
	}
	if art.Qualifiers["artifact"] != "app" {
		t.Errorf("artifact = %q, want 'app'", art.Qualifiers["artifact"])
	}
	if art.Version != "1.0.0" {
		t.Errorf("version = %q, want '1.0.0'", art.Version)
	}
}

func TestHandle_UploadChecksumSidecarStoresChecksumKind(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar.sha1", strings.NewReader("abc123"))
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
	if art.Kind != runtime.KindChecksum {
		t.Fatalf("kind = %q, want checksum", art.Kind)
	}
	if art.Name != "com.example:app" {
		t.Fatalf("name = %q, want com.example:app", art.Name)
	}
	if art.Filename != "app-1.0.0.jar.sha1" {
		t.Fatalf("filename = %q", art.Filename)
	}
	if art.Properties["checksum_algorithm"] != "sha1" {
		t.Fatalf("checksum_algorithm = %q, want sha1", art.Properties["checksum_algorithm"])
	}
	if art.Properties["checksum_for"] != "app-1.0.0.jar" {
		t.Fatalf("checksum_for = %q, want app-1.0.0.jar", art.Properties["checksum_for"])
	}
}

func TestHandle_UploadJarAndPomWithHostedRuntime(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	jarCtx, jarW := newCtx("PUT", "com/test/test-http/1.0.0/test-http-1.0.0.jar", strings.NewReader("jar-data"))
	if err := p.Handle(jarCtx, rt); err != nil {
		t.Fatalf("jar upload Handle failed: %v", err)
	}
	if jarW.Code != http.StatusCreated {
		t.Fatalf("jar upload status = %d body=%q", jarW.Code, jarW.Body.String())
	}

	pomCtx, pomW := newCtx("PUT", "com/test/test-http/1.0.0/test-http-1.0.0.pom", strings.NewReader("<project/>"))
	if err := p.Handle(pomCtx, rt); err != nil {
		t.Fatalf("pom upload Handle failed: %v", err)
	}
	if pomW.Code != http.StatusCreated {
		t.Fatalf("pom upload status = %d body=%q", pomW.Code, pomW.Body.String())
	}

	getJarCtx, getJarW := newCtx("GET", "com/test/test-http/1.0.0/test-http-1.0.0.jar", nil)
	if err := p.Handle(getJarCtx, rt); err != nil {
		t.Fatalf("jar download Handle failed: %v", err)
	}
	if getJarW.Code != http.StatusOK {
		t.Fatalf("jar download status = %d body=%q", getJarW.Code, getJarW.Body.String())
	}
	if getJarW.Body.String() != "jar-data" {
		t.Fatalf("jar body = %q", getJarW.Body.String())
	}
}

func TestHandle_UploadPomExtractsLicenseAttribute(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}
	pom := `<project>
  <licenses>
    <license>
      <name>Apache License, Version 2.0</name>
    </license>
  </licenses>
</project>`

	ctx, w := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.pom", strings.NewReader(pom))
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
	if got := art.Attributes["license"]; got != "Apache License, Version 2.0" {
		t.Fatalf("license attribute = %q, want Apache License, Version 2.0", got)
	}
	if got := art.Properties["license"]; got != "Apache License, Version 2.0" {
		t.Fatalf("license property = %q, want Apache License, Version 2.0", got)
	}
}

func TestHandle_ReuploadJarWithHostedRuntimeUpdatesExistingArtifact(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	firstCtx, firstW := newCtx("PUT", "com/test/test-http/1.0.0/test-http-1.0.0.jar", strings.NewReader("old"))
	if err := p.Handle(firstCtx, rt); err != nil {
		t.Fatalf("first upload Handle failed: %v", err)
	}
	if firstW.Code != http.StatusCreated {
		t.Fatalf("first upload status = %d body=%q", firstW.Code, firstW.Body.String())
	}

	secondCtx, secondW := newCtx("PUT", "com/test/test-http/1.0.0/test-http-1.0.0.jar", strings.NewReader("new"))
	if err := p.Handle(secondCtx, rt); err != nil {
		t.Fatalf("second upload Handle failed: %v", err)
	}
	if secondW.Code != http.StatusOK {
		t.Fatalf("second upload status = %d body=%q", secondW.Code, secondW.Body.String())
	}

	getCtx, getW := newCtx("GET", "com/test/test-http/1.0.0/test-http-1.0.0.jar", nil)
	if err := p.Handle(getCtx, rt); err != nil {
		t.Fatalf("download Handle failed: %v", err)
	}
	if getW.Body.String() != "new" {
		t.Fatalf("jar body = %q", getW.Body.String())
	}
}

func TestHandle_ReuploadReleaseKeepsCurrentCompatibleOverwriteBehavior(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := newHostedMavenRuntime(t)

	firstCtx, firstW := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar", strings.NewReader("old"))
	if err := p.Handle(firstCtx, rt); err != nil {
		t.Fatalf("first upload Handle failed: %v", err)
	}
	if firstW.Code != http.StatusCreated {
		t.Fatalf("first upload status = %d body=%q", firstW.Code, firstW.Body.String())
	}

	secondCtx, secondW := newCtx("PUT", "com/example/app/1.0.0/app-1.0.0.jar", strings.NewReader("new"))
	if err := p.Handle(secondCtx, rt); err != nil {
		t.Fatalf("second upload Handle failed: %v", err)
	}
	if secondW.Code != http.StatusOK {
		t.Fatalf("second upload status = %d body=%q", secondW.Code, secondW.Body.String())
	}
}

func TestHandle_UploadArtifactLevelMetadataUsesStructuredFields(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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
	if art.Name != "com.example:app" {
		t.Fatalf("name = %q, want com.example:app", art.Name)
	}
	if art.Qualifiers["group"] != "com.example" {
		t.Fatalf("group = %q, want com.example", art.Qualifiers["group"])
	}
	if art.Qualifiers["artifact"] != "app" {
		t.Fatalf("artifact = %q, want app", art.Qualifiers["artifact"])
	}
	if art.Version != "" {
		t.Fatalf("version = %q, want empty artifact-level metadata version", art.Version)
	}
	if art.Filename != "maven-metadata.xml" {
		t.Fatalf("filename = %q", art.Filename)
	}
}

func TestHandle_UploadMetadataChecksumSidecarStoresChecksumKind(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("PUT", "com/example/app/maven-metadata.xml.sha1", strings.NewReader("abc123"))
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
	if art.Kind != runtime.KindChecksum {
		t.Fatalf("kind = %q, want checksum", art.Kind)
	}
	if art.Name != "com.example:app" {
		t.Fatalf("name = %q, want com.example:app", art.Name)
	}
	if art.Version != "" {
		t.Fatalf("version = %q, want empty artifact-level metadata checksum version", art.Version)
	}
	if art.Properties["checksum_for"] != "maven-metadata.xml" {
		t.Fatalf("checksum_for = %q, want maven-metadata.xml", art.Properties["checksum_for"])
	}
}

func TestHandle_UploadSnapshotMetadataUsesStructuredFields(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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
	if art.Name != "com.example:app" {
		t.Fatalf("name = %q, want com.example:app", art.Name)
	}
	if art.Version != "1.0-SNAPSHOT" {
		t.Fatalf("version = %q, want 1.0-SNAPSHOT", art.Version)
	}
	if art.Path != "com/example/app/1.0-SNAPSHOT" {
		t.Fatalf("path = %q", art.Path)
	}
}

func TestHandle_ChecksumDownloadLooksUpOriginalArtifactFields(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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
	if got := rt.GetCalls[0].Filename; got != "app-1.0.0.jar" {
		t.Fatalf("lookup filename = %q, want original artifact filename", got)
	}
}

func TestHandle_UploadClosesExistingArtifactContent(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
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
	p := NewMavenPlugin(http.DefaultClient)
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
	p := NewMavenPlugin(http.DefaultClient)
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
	p := NewMavenPlugin(http.DefaultClient)
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

// ---------------------------------------------------------------------------
// ErrBlocked 透传：runtime 返回 ErrBlocked 时，plugin.Handle 必须 return ErrBlocked，
// 让 router 统一处理 403 响应 + 审计日志。不能被插件吞成 500/404 后 return nil。
// ---------------------------------------------------------------------------

func TestHandle_ArtifactDownloadGetBlockedPropagatesErrBlocked(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{GetErr: runtime.ErrBlocked}

	ctx, _ := newCtx("GET", "com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.jar", nil)
	err := p.Handle(ctx, rt)
	if !errors.Is(err, runtime.ErrBlocked) {
		t.Fatalf("Handle err = %v, want ErrBlocked (must propagate to router for audit log)", err)
	}
}

func TestHandle_MetadataGetBlockedPropagatesErrBlocked(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{QueryErr: runtime.ErrBlocked}

	ctx, _ := newCtx("GET", "com/google/guava/guava/maven-metadata.xml", nil)
	err := p.Handle(ctx, rt)
	if !errors.Is(err, runtime.ErrBlocked) {
		t.Fatalf("Handle err = %v, want ErrBlocked (must propagate to router for audit log)", err)
	}
}

func TestHandle_ChecksumDownloadGetBlockedPropagatesErrBlocked(t *testing.T) {
	p := NewMavenPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{GetErr: runtime.ErrBlocked}

	ctx, _ := newCtx("GET", "com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.jar.sha1", nil)
	err := p.Handle(ctx, rt)
	if !errors.Is(err, runtime.ErrBlocked) {
		t.Fatalf("Handle err = %v, want ErrBlocked (must propagate to router for audit log)", err)
	}
}
