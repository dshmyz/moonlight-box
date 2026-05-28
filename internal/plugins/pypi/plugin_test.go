package pypi

import (
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
	req := httptest.NewRequest(method, "/repository/pypi-test/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "pypi-test", Format: "pypi", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

func TestHandle_SimpleIndex(t *testing.T) {
	p := NewPyPIPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "package-index", map[string]string{"name": "requests", "package": "requests"}, ""),
		testhelper.NewArtifact("pypi", "package-index", map[string]string{"name": "flask", "package": "flask"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "requests") || !strings.Contains(body, "flask") {
		t.Errorf("expected package names in HTML, got: %s", body)
	}
}

func TestHandle_SimpleIndexJSON(t *testing.T) {
	p := NewPyPIPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "package-index", map[string]string{"name": "requests", "package": "requests"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/", nil)
	ctx.Request.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	p.Handle(ctx, rt)

	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("expected JSON content type, got %s", ct)
	}
}

func TestHandle_PackageList(t *testing.T) {
	p := NewPyPIPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "package-file", map[string]string{
			"name":     "requests",
			"package":  "requests",
			"version":  "2.28.0",
			"filename": "requests-2.28.0.tar.gz",
		}, ""),
	}
	arts[0].Properties = map[string]string{"remote_path": "ab/cd/requests-2.28.0.tar.gz"}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "requests-2.28.0.tar.gz") {
		t.Errorf("expected filename in HTML, got: %s", body)
	}
}

func TestHandle_PackageDownload(t *testing.T) {
	p := NewPyPIPlugin()
	art := testhelper.NewArtifact("pypi", "package", map[string]string{
		"package":  "requests",
		"version":  "2.28.0",
		"filename": "requests-2.28.0.tar.gz",
		"path":     "packages/ab/cd",
	}, "package-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "packages/ab/cd/requests-2.28.0.tar.gz", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestNormalizePackageName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"My_Package", "my-package"},
		{"MY_PACKAGE", "my-package"},
		{"requests", "requests"},
	}
	for _, tt := range tests {
		got := normalizePackageName(tt.input)
		if got != tt.want {
			t.Errorf("normalizePackageName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidatePyPIPath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"simple/", false},
		{"simple/requests/", false},
		{"packages/ab/cd/file.tar.gz", false},
		{"../etc/passwd", true},
		{"simple/$inject", true},
	}
	for _, tt := range tests {
		err := validatePyPIPath(tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("validatePyPIPath(%q): err=%v, wantErr=%v", tt.path, err, tt.wantErr)
		}
	}
}

func TestIsValidWheelFilename(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"requests-2.28.0-py3-none-any.whl", true},
		{"pkg-1.0-cp39-cp39-linux_x86_64.whl", true},
		{"invalid.whl", false},
		{"requests-2.28.0.tar.gz", false},
	}
	for _, tt := range tests {
		got := isValidWheelFilename(tt.name)
		if got != tt.want {
			t.Errorf("isValidWheelFilename(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsValidSdistFilename(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"requests-2.28.0.tar.gz", true},
		{"pkg-1.0.zip", true},
		{"pkg-1.0.tar.bz2", true},
		{"requests-2.28.0-py3-none-any.whl", false},
	}
	for _, tt := range tests {
		got := isValidSdistFilename(tt.name)
		if got != tt.want {
			t.Errorf("isValidSdistFilename(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestExtractVersionFromFilename(t *testing.T) {
	p := NewPyPIPlugin()
	tests := []struct {
		filename, want string
	}{
		{"requests-2.28.0.tar.gz", "2.28.0"},
		{"requests-2.28.0-py3-none-any.whl", "2.28.0"},
		{"Flask-2.3.2.tar.gz", "2.3.2"},
	}
	for _, tt := range tests {
		got := p.extractVersionFromFilename(tt.filename)
		if got != tt.want {
			t.Errorf("extractVersion(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestHandle_JsonAPI(t *testing.T) {
	p := NewPyPIPlugin()
	art := testhelper.NewArtifact("pypi", "package-file", map[string]string{
		"package":  "requests",
		"version":  "2.28.0",
		"filename": "requests-2.28.0.tar.gz",
	}, "")
	art.Properties = map[string]string{"remote_path": "ab/cd/requests-2.28.0.tar.gz"}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "pypi/requests/json", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestHandle_QueryRemotePath(t *testing.T) {
	p := NewPyPIPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "simple/requests/", nil)
	p.Handle(ctx, rt)

	if len(rt.QueryCalls) != 1 {
		t.Fatalf("expected 1 query call, got %d", len(rt.QueryCalls))
	}
	if rt.QueryCalls[0].RemotePath != "simple/requests/" {
		t.Errorf("unexpected RemotePath: %q", rt.QueryCalls[0].RemotePath)
	}
}

func TestFetchRemote_SimpleIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
<a href="requests/">requests</a><br>
<a href="flask/">flask</a><br>
</body></html>`))
	}))
	defer srv.Close()

	p := NewPyPIPlugin()
	arts, err := p.FetchRemote(context.Background(), srv.URL, "simple")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(arts))
	}
	if arts[0].Coordinates["package"] != "requests" {
		t.Errorf("expected 'requests', got %q", arts[0].Coordinates["package"])
	}
}

func TestHandle_HtmlEscaping(t *testing.T) {
	p := NewPyPIPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "package-index", map[string]string{"name": "<script>alert(1)</script>", "package": "<script>alert(1)</script>"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/", nil)
	p.Handle(ctx, rt)

	body := w.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("HTML output should escape package names, found unescaped <script>")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("expected escaped HTML entities")
	}
}
