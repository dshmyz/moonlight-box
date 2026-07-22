package gomod

import (
	"context"
	"encoding/json"
	"errors"
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
	req := httptest.NewRequest(method, "/repository/go-test/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "go-test", Format: "go", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

// ---------------------------------------------------------------------------
// encodeGoPath
// ---------------------------------------------------------------------------

func TestEncodeGoPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"github.com/gin-gonic/gin/@v/list", "github.com/gin-gonic/gin/@v/list"},
		{"github.com/Azure/azure-sdk-go/@v/list", "github.com/!azure/azure-sdk-go/@v/list"},
		{"github.com/BurntSushi/toml/@latest", "github.com/!burnt!sushi/toml/@latest"},
		{"github.com/foo/bar/@v/v1.0.0.zip", "github.com/foo/bar/@v/v1.0.0.zip"},
		{"github.com/labstack/echo/v4/@v/list", "github.com/labstack/echo/v4/@v/list"},
		{"", ""},
	}
	for _, tt := range tests {
		got := encodeGoPath(tt.input)
		if got != tt.expected {
			t.Errorf("encodeGoPath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// splitModulePath
// ---------------------------------------------------------------------------

func TestSplitModulePath(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	tests := []struct {
		name       string
		path       string
		wantModule string
		wantFile   string
	}{
		{"standard zip", "github.com/gin-gonic/gin/@v/v1.10.0.zip", "github.com/gin-gonic/gin", "v1.10.0.zip"},
		{"info file", "github.com/gin-gonic/gin/@v/v1.10.0.info", "github.com/gin-gonic/gin", "v1.10.0.info"},
		{"mod file", "github.com/gin-gonic/gin/@v/v1.10.0.mod", "github.com/gin-gonic/gin", "v1.10.0.mod"},
		{"latest", "github.com/gin-gonic/gin/@latest", "github.com/gin-gonic/gin", "@latest"},
		{"semantic import version", "github.com/labstack/echo/v4/@v/v4.13.0.zip", "github.com/labstack/echo/v4", "v4.13.0.zip"},
		{"semantic import version latest", "github.com/labstack/echo/v4/@latest", "github.com/labstack/echo/v4", "@latest"},
		{"no @v or @latest", "github.com/foo/bar", "github.com/foo/bar", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod, file := p.splitModulePath(tt.path)
			if mod != tt.wantModule {
				t.Errorf("module = %q, want %q", mod, tt.wantModule)
			}
			if file != tt.wantFile {
				t.Errorf("filename = %q, want %q", file, tt.wantFile)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// selectLatestVersion
// ---------------------------------------------------------------------------

func TestSelectLatestVersion(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)

	tests := []struct {
		name     string
		versions []string
		want     string
	}{
		{
			name:     "single stable version",
			versions: []string{"v1.0.0"},
			want:     "v1.0.0",
		},
		{
			name:     "multiple stable versions, picks highest",
			versions: []string{"v1.0.0", "v1.2.0", "v1.1.0"},
			want:     "v1.2.0",
		},
		{
			name:     "pre-release filtered out when stable exists",
			versions: []string{"v1.0.0", "v2.0.0-rc1", "v2.0.0-alpha.1"},
			want:     "v1.0.0",
		},
		{
			name:     "all pre-release falls back to latest pre-release",
			versions: []string{"v2.0.0-rc1", "v2.0.0-alpha.1"},
			want:     "v2.0.0-rc1",
		},
		{
			name:     "incompatible version (v2 without /v2 in path)",
			versions: []string{"v1.0.0", "v2.4.0"},
			want:     "v2.4.0",
		},
		{
			name:     "semantic import version v4",
			versions: []string{"v4.0.0", "v4.12.0", "v4.13.0"},
			want:     "v4.13.0",
		},
		{
			name:     "v0 versions are valid stable",
			versions: []string{"v0.1.0", "v0.2.0"},
			want:     "v0.2.0",
		},
		{
			name:     "invalid version format skipped",
			versions: []string{"invalid", "v1.0.0"},
			want:     "v1.0.0",
		},
		{
			name:     "empty version string skipped",
			versions: []string{"", "v1.0.0"},
			want:     "v1.0.0",
		},
		{
			name:     "mixed valid and invalid",
			versions: []string{"not-a-version", "v0.0.0", "v3.0.0-beta"},
			want:     "v0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var arts []*runtime.Artifact
			for _, v := range tt.versions {
				arts = append(arts, runtime.NewArtifact(runtime.ArtifactSpec{
					Format:  "go",
					Kind:    runtime.KindVersion,
					Name:    "github.com/example/mod",
					Version: v,
					Qualifiers: map[string]string{
						"module": "github.com/example/mod",
					},
				}))
			}
			result := p.selectLatestVersion(arts)
			if tt.want == "" {
				if result != nil {
					t.Fatalf("expected nil, got %v", result.Version)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected %q, got nil", tt.want)
			}
			if result.Version != tt.want {
				t.Errorf("got %q, want %q", result.Version, tt.want)
			}
		})
	}
}

func TestSelectLatestVersion_SkipsRetracted(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "go",
			Kind:       runtime.KindVersion,
			Name:       "github.com/example/mod",
			Version:    "v1.2.0",
			Qualifiers: map[string]string{"module": "github.com/example/mod"},
			Attributes: map[string]string{"retracted": "true"},
		}),
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "go",
			Kind:       runtime.KindVersion,
			Name:       "github.com/example/mod",
			Version:    "v1.1.0",
			Qualifiers: map[string]string{"module": "github.com/example/mod"},
		}),
	}
	result := p.selectLatestVersion(arts)
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Version != "v1.1.0" {
		t.Fatalf("expected v1.1.0 (v1.2.0 retracted), got %q", result.Version)
	}
}

func TestSelectLatestVersion_ModulePathMajorFiltering(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)

	t.Run("v1 module without /vN suffix accepts all versions", func(t *testing.T) {
		arts := []*runtime.Artifact{
			runtime.NewArtifact(runtime.ArtifactSpec{Format: "go", Kind: runtime.KindVersion, Name: "github.com/foo/bar", Version: "v1.9.0", Qualifiers: map[string]string{"module": "github.com/foo/bar"}}),
			runtime.NewArtifact(runtime.ArtifactSpec{Format: "go", Kind: runtime.KindVersion, Name: "github.com/foo/bar", Version: "v2.0.0", Qualifiers: map[string]string{"module": "github.com/foo/bar"}}),
		}
		result := p.selectLatestVersion(arts)
		if result == nil {
			t.Fatal("expected result")
		}
		if result.Version != "v2.0.0" {
			t.Fatalf("expected v2.0.0 (v1 path accepts all), got %q", result.Version)
		}
	})

	t.Run("v1 module accepts +incompatible v2", func(t *testing.T) {
		arts := []*runtime.Artifact{
			runtime.NewArtifact(runtime.ArtifactSpec{Format: "go", Kind: runtime.KindVersion, Name: "github.com/foo/bar", Version: "v1.9.0", Qualifiers: map[string]string{"module": "github.com/foo/bar"}}),
			runtime.NewArtifact(runtime.ArtifactSpec{Format: "go", Kind: runtime.KindVersion, Name: "github.com/foo/bar", Version: "v2.0.0+incompatible", Qualifiers: map[string]string{"module": "github.com/foo/bar"}}),
		}
		result := p.selectLatestVersion(arts)
		if result == nil {
			t.Fatal("expected result")
		}
		if result.Version != "v2.0.0+incompatible" {
			t.Fatalf("expected v2.0.0+incompatible for v1 module, got %q", result.Version)
		}
	})

	t.Run("v2 module selects highest v2", func(t *testing.T) {
		arts := []*runtime.Artifact{
			runtime.NewArtifact(runtime.ArtifactSpec{Format: "go", Kind: runtime.KindVersion, Name: "github.com/foo/bar/v2", Version: "v1.9.0", Qualifiers: map[string]string{"module": "github.com/foo/bar/v2"}}),
			runtime.NewArtifact(runtime.ArtifactSpec{Format: "go", Kind: runtime.KindVersion, Name: "github.com/foo/bar/v2", Version: "v2.0.0", Qualifiers: map[string]string{"module": "github.com/foo/bar/v2"}}),
		}
		result := p.selectLatestVersion(arts)
		if result == nil {
			t.Fatal("expected result")
		}
		if result.Version != "v2.0.0" {
			t.Fatalf("expected v2.0.0 for v2 module, got %q", result.Version)
		}
	})
}

// ---------------------------------------------------------------------------
// Handle - @latest
// ---------------------------------------------------------------------------

func TestHandle_Latest(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/gin-gonic/gin",
			"version":     "v1.10.0",
			"remote_path": "github.com/gin-gonic/gin/@latest",
		}, ""),
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/gin-gonic/gin",
			"version":     "v1.9.0",
			"remote_path": "github.com/gin-gonic/gin/@latest",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "github.com/gin-gonic/gin/@latest", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["Version"] != "v1.10.0" {
		t.Errorf("expected v1.10.0, got %q", result["Version"])
	}
}

func TestHandle_LatestHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/gin-gonic/gin",
			"version":     "v1.10.0",
			"remote_path": "github.com/gin-gonic/gin/@latest",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("HEAD", "github.com/gin-gonic/gin/@latest", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
}

func TestHandle_Latest_SemanticImportVersion(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/labstack/echo/v4",
			"version":     "v4.13.0",
			"remote_path": "github.com/labstack/echo/v4/@latest",
		}, ""),
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/labstack/echo/v4",
			"version":     "v4.12.0",
			"remote_path": "github.com/labstack/echo/v4/@latest",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "github.com/labstack/echo/v4/@latest", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["Version"] != "v4.13.0" {
		t.Errorf("expected v4.13.0, got %q", result["Version"])
	}
}

func TestHandle_Latest_FiltersPrerelease(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/example/mod",
			"version":     "v2.0.0-rc1",
			"remote_path": "github.com/example/mod/@latest",
		}, ""),
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/example/mod",
			"version":     "v1.5.0",
			"remote_path": "github.com/example/mod/@latest",
		}, ""),
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/example/mod",
			"version":     "v2.0.0-alpha.1",
			"remote_path": "github.com/example/mod/@latest",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "github.com/example/mod/@latest", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["Version"] != "v1.5.0" {
		t.Errorf("expected v1.5.0 (pre-release filtered), got %q", result["Version"])
	}
}

func TestHandle_Latest_OnlyPrerelease(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/example/mod",
			"version":     "v2.0.0-rc1",
			"remote_path": "github.com/example/mod/@latest",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "github.com/example/mod/@latest", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when only pre-release exists (fallback), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "v2.0.0-rc1") {
		t.Errorf("expected body to contain v2.0.0-rc1, got %q", body)
	}
}

func TestHandle_Latest_NotFound(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{Artifacts: nil}

	ctx, w := newCtx("GET", "github.com/nonexist/pkg/@latest", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandle_Latest_MethodNotAllowed(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("POST", "github.com/example/mod/@latest", nil)
	err := p.Handle(ctx, rt)
	if err == nil {
		t.Fatal("expected error for POST method")
	}
	if !strings.Contains(err.Error(), "method not allowed") {
		t.Errorf("expected 'method not allowed', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Handle - @v/list
// ---------------------------------------------------------------------------

func TestHandle_VersionList(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/gin-gonic/gin",
			"version":     "v1.10.0",
			"remote_path": "github.com/gin-gonic/gin/@v/list",
		}, ""),
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/gin-gonic/gin",
			"version":     "v1.9.0",
			"remote_path": "github.com/gin-gonic/gin/@v/list",
		}, ""),
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/gin-gonic/gin",
			"version":     "v1.9.0",
			"remote_path": "github.com/gin-gonic/gin/@v/list",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "github.com/gin-gonic/gin/@v/list", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "v1.10.0\n") || !strings.Contains(body, "v1.9.0\n") {
		t.Errorf("expected versions in list, got: %q", body)
	}
}

func TestHandle_VersionList_Dedup(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/example/mod",
			"version":     "v1.0.0",
			"remote_path": "github.com/example/mod/@v/list",
		}, ""),
		testhelper.NewArtifact("go", "version", map[string]string{
			"module":      "github.com/example/mod",
			"version":     "v1.0.0",
			"remote_path": "github.com/example/mod/@v/list",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "github.com/example/mod/@v/list", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "v1.0.0" {
		t.Errorf("expected deduplicated list 'v1.0.0', got: %q", body)
	}
}

func TestHandle_VersionList_MethodNotAllowed(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("POST", "github.com/example/mod/@v/list", nil)
	err := p.Handle(ctx, rt)
	if err == nil {
		t.Fatal("expected error for POST method")
	}
	if !strings.Contains(err.Error(), "method not allowed") {
		t.Errorf("expected 'method not allowed', got: %v", err)
	}
}

func TestHandle_VersionList_NotFound(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	// QueryArtifacts 返回 ErrNotFound，应返回 404 而不是 500
	rt := &testhelper.MockRuntime{QueryErr: runtime.ErrNotFound}

	ctx, w := newCtx("GET", "github.com/nonexistent/mod/@v/list", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle should not return error for ErrNotFound, got: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent module, got %d body=%q", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Handle - module download (@v/*.info, *.mod, *.zip)
// ---------------------------------------------------------------------------

func TestHandle_ModuleDownload_Info(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("go", "info-file", map[string]string{
		"name":     "github.com/gin-gonic/gin",
		"module":   "github.com/gin-gonic/gin",
		"version":  "v1.10.0",
		"path":     "github.com/gin-gonic/gin/@v",
		"ext":      "info",
		"filename": "v1.10.0.info",
	}, `{"Version":"v1.10.0","Time":"2024-01-01T00:00:00Z"}`)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "github.com/gin-gonic/gin/@v/v1.10.0.info", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["Version"] != "v1.10.0" {
		t.Errorf("expected Version v1.10.0, got %q", result["Version"])
	}
}

func TestHandle_ModuleDownload_Mod(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("go", "module-file", map[string]string{
		"name":     "github.com/gin-gonic/gin",
		"module":   "github.com/gin-gonic/gin",
		"version":  "v1.10.0",
		"path":     "github.com/gin-gonic/gin/@v",
		"ext":      "mod",
		"filename": "v1.10.0.mod",
	}, "module golang.org/x/net")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "github.com/gin-gonic/gin/@v/v1.10.0.mod", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("expected text/plain, got %s", ct)
	}
}

func TestHandle_ModuleDownload_Zip(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("go", "module-file", map[string]string{
		"name":     "github.com/gin-gonic/gin",
		"module":   "github.com/gin-gonic/gin",
		"version":  "v1.10.0",
		"path":     "github.com/gin-gonic/gin/@v",
		"ext":      "zip",
		"filename": "v1.10.0.zip",
	}, "zip-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "github.com/gin-gonic/gin/@v/v1.10.0.zip", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("expected application/zip, got %s", ct)
	}
	if w.Body.String() != "zip-content" {
		t.Errorf("expected 'zip-content', got %q", w.Body.String())
	}
}

func TestHandle_ModuleDownload_ModHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("go", "module-file", map[string]string{
		"name":     "github.com/gin-gonic/gin",
		"module":   "github.com/gin-gonic/gin",
		"version":  "v1.10.0",
		"path":     "github.com/gin-gonic/gin/@v",
		"ext":      "mod",
		"filename": "v1.10.0.mod",
	}, "module github.com/gin-gonic/gin")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "github.com/gin-gonic/gin/@v/v1.10.0.mod", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("expected text/plain, got %q", ct)
	}
}

func TestHandle_ModuleDownload_ZipRangeReturnsPartialContent(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("go", "module-file", map[string]string{
		"name":     "github.com/gin-gonic/gin",
		"module":   "github.com/gin-gonic/gin",
		"version":  "v1.10.0",
		"path":     "github.com/gin-gonic/gin/@v",
		"ext":      "zip",
		"filename": "v1.10.0.zip",
	}, "zip-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "github.com/gin-gonic/gin/@v/v1.10.0.zip", nil)
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

func TestHandle_ModuleDownload_NotFound(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{Artifacts: nil}

	ctx, w := newCtx("GET", "github.com/nonexist/pkg/@v/v1.0.0.zip", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandle_ModuleDownloadMissQueriesRemotePathBeforeRetry(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	rt := &goQueryThenGetRuntime{
		artifact: testhelper.NewArtifact("go", "module-file", map[string]string{
			"name":     "github.com/Azure/azure-sdk-for-go",
			"module":   "github.com/Azure/azure-sdk-for-go",
			"version":  "v1.2.3",
			"path":     "github.com/Azure/azure-sdk-for-go/@v",
			"ext":      "zip",
			"filename": "v1.2.3.zip",
		}, "zip-content"),
	}

	ctx, w := newCtx("GET", "github.com/Azure/azure-sdk-for-go/@v/v1.2.3.zip", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after QueryArtifacts retry, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.queryCalls) != 1 {
		t.Fatalf("expected one QueryArtifacts call, got %d", len(rt.queryCalls))
	}
	if got := rt.queryCalls[0].RemotePath; got != "github.com/Azure/azure-sdk-for-go/@v/v1.2.3.zip" {
		t.Fatalf("RemotePath = %q", got)
	}
	if rt.getCalls != 2 {
		t.Fatalf("expected two GetArtifact calls, got %d", rt.getCalls)
	}
}

type goQueryThenGetRuntime struct {
	artifact   *runtime.Artifact
	getCalls   int
	queryCalls []runtime.ArtifactQuery
	queried    bool
}

func (r *goQueryThenGetRuntime) GetArtifact(ctx context.Context, key runtime.ArtifactKey) (*runtime.Artifact, error) {
	r.getCalls++
	if !r.queried {
		return nil, runtime.ErrNotFound
	}
	return r.artifact, nil
}

func (r *goQueryThenGetRuntime) QueryArtifacts(ctx context.Context, query runtime.ArtifactQuery) ([]*runtime.Artifact, error) {
	r.queryCalls = append(r.queryCalls, query)
	r.queried = true
	return []*runtime.Artifact{r.artifact}, nil
}

func (r *goQueryThenGetRuntime) RenderProjection(ctx context.Context, query runtime.ProjectionQuery) (*runtime.ProjectionResult, error) {
	return nil, runtime.ErrNotFound
}

func (r *goQueryThenGetRuntime) OpenRemote(context.Context, runtime.RemoteOpenRequest) (*runtime.RemoteResponse, error) {
	return nil, runtime.ErrRemoteUnsupported
}

func (r *goQueryThenGetRuntime) BeginUpload(ctx context.Context, req runtime.UploadRequest) (runtime.UploadSession, error) {
	return nil, runtime.ErrReadOnly
}

func (r *goQueryThenGetRuntime) DeleteArtifact(ctx context.Context, key runtime.ArtifactKey) error {
	return runtime.ErrReadOnly
}

func TestHandle_ModuleDownload_InvalidPath(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "invalid-path-no-atv", nil)
	if err := p.Handle(ctx, rt); err == nil {
		t.Fatal("expected error for invalid path")
	}
	_ = w
}

func TestHandle_QueryRemotePath(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "github.com/gin-gonic/gin/@v/list", nil)
	p.Handle(ctx, rt)

	if len(rt.QueryCalls) != 1 {
		t.Fatalf("expected 1 query call, got %d", len(rt.QueryCalls))
	}
	if rt.QueryCalls[0].RemotePath != "github.com/gin-gonic/gin/@v/list" {
		t.Errorf("unexpected RemotePath: %q", rt.QueryCalls[0].RemotePath)
	}
}

// ---------------------------------------------------------------------------
// FetchRemote - @v/list
// ---------------------------------------------------------------------------

func TestFetchRemote_VersionList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v1.0.0\nv1.1.0\nv1.2.0\n"))
	}))
	defer srv.Close()

	p := NewGoPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "github.com/example/mod/@v/list")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(arts))
	}
	for _, a := range arts {
		if a.Qualifiers["module"] != "github.com/example/mod" {
			t.Errorf("module = %q, want 'github.com/example/mod'", a.Qualifiers["module"])
		}
	}
}

func TestFetchRemote_VersionList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(""))
	}))
	defer srv.Close()

	p := NewGoPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "github.com/example/mod/@v/list")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 0 {
		t.Fatalf("expected 0 artifacts for empty list, got %d", len(arts))
	}
}

// TestFetchRemote_VersionList_Upstream5xxReturnsUpstreamUnavailable
// 验证上游返回 5xx 时，FetchRemote 返回 ErrUpstreamUnavailable（而非普通 error），
// 让 router 能正确映射为 502 Bad Gateway 而非 500 Internal Server Error。
func TestFetchRemote_VersionList_Upstream5xxReturnsUpstreamUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := NewGoPlugin(http.DefaultClient)
	_, err := p.FetchRemote(context.Background(), srv.URL, "github.com/example/mod/@v/list")
	if err == nil {
		t.Fatal("expected error for upstream 503, got nil")
	}
	if !errors.Is(err, runtime.ErrUpstreamUnavailable) {
		t.Errorf("expected ErrUpstreamUnavailable, got: %v", err)
	}
}

// TestFetchRemote_VersionList_Upstream4xxReturnsGenericError
// 验证上游返回 4xx（非 404）时，FetchRemote 返回普通 error（非 ErrUpstreamUnavailable）。
// 4xx 是客户端错误，不应映射为 502。
func TestFetchRemote_VersionList_Upstream4xxReturnsGenericError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	p := NewGoPlugin(http.DefaultClient)
	_, err := p.FetchRemote(context.Background(), srv.URL, "github.com/example/mod/@v/list")
	if err == nil {
		t.Fatal("expected error for upstream 403, got nil")
	}
	if errors.Is(err, runtime.ErrUpstreamUnavailable) {
		t.Errorf("4xx should not return ErrUpstreamUnavailable, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// FetchRemote - @latest
// ---------------------------------------------------------------------------

func TestFetchRemote_Latest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Version": "v1.2.0", "Time": "2023-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	p := NewGoPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "github.com/example/mod/@latest")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if arts[0].Version != "v1.2.0" {
		t.Errorf("version = %q, want 'v1.2.0'", arts[0].Version)
	}
}

func TestFetchRemote_Latest_EmptyVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Version": "", "Time": ""}`))
	}))
	defer srv.Close()

	p := NewGoPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "github.com/example/mod/@latest")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if arts != nil {
		t.Fatalf("expected nil for empty version, got %d artifacts", len(arts))
	}
}

func TestFetchRemote_Latest_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewGoPlugin(http.DefaultClient)
	_, err := p.FetchRemote(context.Background(), srv.URL, "github.com/example/mod/@latest")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// ---------------------------------------------------------------------------
// FetchRemote - generic paths
// ---------------------------------------------------------------------------

func TestFetchRemote_ModuleFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewGoPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "github.com/example/mod/@v/v1.0.0.zip")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if arts[0].Filename != "v1.0.0.zip" {
		t.Errorf("filename = %q, want 'v1.0.0.zip'", arts[0].Filename)
	}
}

func TestFetchRemote_ModuleFileFieldsMatchDownloadKey(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), "http://example.test", "github.com/example/mod/@v/v1.2.3.zip")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}

	got := arts[0]
	if got.Name != "github.com/example/mod" || got.Version != "v1.2.3" || got.Path != "github.com/example/mod/@v" || got.Filename != "v1.2.3.zip" {
		t.Fatalf("unexpected artifact fields: name=%q version=%q path=%q filename=%q", got.Name, got.Version, got.Path, got.Filename)
	}
	if got.Qualifiers["module"] != "github.com/example/mod" || got.Qualifiers["ext"] != "zip" {
		t.Fatalf("unexpected qualifiers: %#v", got.Qualifiers)
	}
}

func TestFetchRemote_EmptyPath(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	_, err := p.FetchRemote(context.Background(), "http://example.com", "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

// ---------------------------------------------------------------------------
// FetchRemote - semantic import version
// ---------------------------------------------------------------------------

func TestFetchRemote_SemanticImportVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v4.12.0\nv4.13.0\n"))
	}))
	defer srv.Close()

	p := NewGoPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "github.com/labstack/echo/v4/@v/list")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(arts))
	}
	if arts[0].Qualifiers["module"] != "github.com/labstack/echo/v4" {
		t.Errorf("module = %q, want 'github.com/labstack/echo/v4'", arts[0].Qualifiers["module"])
	}
}

// ---------------------------------------------------------------------------
// ErrBlocked 透传：runtime 返回 ErrBlocked 时，plugin.Handle 必须 return ErrBlocked，
// 让 router 统一处理 403 响应 + 审计日志。不能被插件吞成 500/404 后 return nil。
// ---------------------------------------------------------------------------

func TestHandle_ModuleDownloadGetBlockedPropagatesErrBlocked(t *testing.T) {
	p := NewGoPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{GetErr: runtime.ErrBlocked}

	ctx, _ := newCtx("GET", "github.com/labstack/echo/v4/@v/v4.0.0.zip", nil)
	err := p.Handle(ctx, rt)
	if !errors.Is(err, runtime.ErrBlocked) {
		t.Fatalf("Handle err = %v, want ErrBlocked (must propagate to router for audit log)", err)
	}
}
