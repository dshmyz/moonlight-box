package raw

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/plugins/testhelper"
)

func newCtx(method, path string, body io.Reader) (*runtime.RequestContext, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/repository/test-repo/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "test-repo", Format: "generic", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

func TestHandle_GetDownload(t *testing.T) {
	p := NewGenericPlugin()
	art := testhelper.NewArtifact("generic", "file", map[string]string{"name": "readme.txt", "path": "docs"}, "hello world")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "docs/readme.txt", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "hello world" {
		t.Errorf("expected 'hello world', got %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("expected text/plain, got %s", ct)
	}
}

func TestHandle_GetNotFound(t *testing.T) {
	p := NewGenericPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "missing.txt", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandle_PutUpload(t *testing.T) {
	p := NewGenericPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("PUT", "docs/new.txt", bytes.NewReader([]byte("file content")))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if len(rt.UploadCalls) != 1 {
		t.Fatalf("expected 1 upload call, got %d", len(rt.UploadCalls))
	}
	if rt.UploadedArts[0].Coordinates["name"] != "new.txt" {
		t.Errorf("expected name 'new.txt', got %q", rt.UploadedArts[0].Coordinates["name"])
	}
}

func TestHandle_Delete(t *testing.T) {
	p := NewGenericPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("DELETE", "old.txt", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if len(rt.DeleteCalls) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(rt.DeleteCalls))
	}
}

func TestHandle_EmptyPath(t *testing.T) {
	p := NewGenericPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "", nil)
	ctx.RepositoryPath = ""
	err := p.Handle(ctx, rt)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	_ = w
}

func TestFetchRemote_HTMLDirectoryListing(t *testing.T) {
	html := `<html><body>
<a href="../">../</a>
<a href="v1.0.0/">v1.0.0/</a>
<a href="readme.txt">readme.txt</a>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer srv.Close()

	p := NewGenericPlugin()
	arts, err := p.FetchRemote(context.Background(), srv.URL, "packages")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 artifacts (dir + file), got %d", len(arts))
	}
	if arts[0].Kind != "directory" {
		t.Errorf("first should be directory, got %s", arts[0].Kind)
	}
	if arts[1].Kind != "file" {
		t.Errorf("second should be file, got %s", arts[1].Kind)
	}
}

func TestFetchRemote_DirectFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write([]byte("binary data"))
	}))
	defer srv.Close()

	p := NewGenericPlugin()
	arts, err := p.FetchRemote(context.Background(), srv.URL, "files/archive.zip")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if arts[0].Coordinates["name"] != "archive.zip" {
		t.Errorf("expected name 'archive.zip', got %q", arts[0].Coordinates["name"])
	}
}

func TestContentTypes(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"data.json", "application/json"},
		{"config.xml", "application/xml"},
		{"notes.txt", "text/plain"},
		{"pkg.zip", "application/zip"},
		{"unknown.xyz", "application/octet-stream"},
	}
	for _, tt := range tests {
		p := NewGenericPlugin()
		art := testhelper.NewArtifact("generic", "file", map[string]string{"name": tt.filename, "path": ""}, "data")
		rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

		ctx, w := newCtx("GET", tt.filename, nil)
		p.Handle(ctx, rt)
		if ct := w.Header().Get("Content-Type"); ct != tt.want {
			t.Errorf("%s: expected %s, got %s", tt.filename, tt.want, ct)
		}
	}
}

func TestCoordinatesMapping(t *testing.T) {
	p := NewGenericPlugin()
	art := testhelper.NewArtifact("generic", "file", map[string]string{"name": "test.txt", "path": "a/b/c"}, "content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, _ := newCtx("GET", "a/b/c/test.txt", nil)
	p.Handle(ctx, rt)

	if len(rt.GetCalls) != 1 {
		t.Fatalf("expected 1 get call, got %d", len(rt.GetCalls))
	}
	key := rt.GetCalls[0]
	if key.Coordinates["name"] != "test.txt" {
		t.Errorf("expected name 'test.txt', got %q", key.Coordinates["name"])
	}
	if key.Coordinates["path"] != "a/b/c" {
		t.Errorf("expected path 'a/b/c', got %q", key.Coordinates["path"])
	}
	if key.Filename != "test.txt" {
		t.Errorf("expected filename 'test.txt', got %q", key.Filename)
	}
}

// Ensure FetchRemote returns proper error for empty path.
func TestFetchRemote_EmptyPath(t *testing.T) {
	p := NewGenericPlugin()
	_, err := p.FetchRemote(context.Background(), "http://example.com", "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

// Ensure JSON API returns proper structure.
func TestHandle_JSONContentType(t *testing.T) {
	p := NewGenericPlugin()
	art := testhelper.NewArtifact("generic", "file", map[string]string{"name": "data.json", "path": ""}, `{"key":"value"}`)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "data.json", nil)
	p.Handle(ctx, rt)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result)
	}
}
