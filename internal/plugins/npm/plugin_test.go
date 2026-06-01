package npm

import (
	"bytes"
	"context"
	"encoding/json"
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
	req := httptest.NewRequest(method, "/repository/npm-test/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "npm-test", Format: "npm", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

func TestHandle_PackageMetadata(t *testing.T) {
	p := NewNpmPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.18.2"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.17.3"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "express", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)

	if result["name"] != "express" {
		t.Errorf("expected name 'express', got %v", result["name"])
	}
	distTags := result["dist-tags"].(map[string]interface{})
	if distTags["latest"] != "4.18.2" {
		t.Errorf("expected latest '4.18.2', got %v", distTags["latest"])
	}
	versions := result["versions"].(map[string]interface{})
	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}
}

func TestHandle_ScopedPackage(t *testing.T) {
	p := NewNpmPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "@scope/pkg", "version": "1.0.0"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "@scope/pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["name"] != "@scope/pkg" {
		t.Errorf("expected '@scope/pkg', got %v", result["name"])
	}
}

func TestHandle_TarballDownload(t *testing.T) {
	p := NewNpmPlugin()
	art := testhelper.NewArtifact("npm", "tarball", map[string]string{
		"name":     "express",
		"version":  "4.18.2",
		"path":     "express/-",
		"filename": "express-4.18.2.tgz",
	}, "tarball-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "express/-/express-4.18.2.tgz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "tarball-content" {
		t.Errorf("expected 'tarball-content', got %q", w.Body.String())
	}
}

func TestHandle_Ping(t *testing.T) {
	p := NewNpmPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "-/npm/ping", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["ok"] != true {
		t.Errorf("expected ok=true, got %v", result["ok"])
	}
}

func TestHandle_SecurityAudit(t *testing.T) {
	p := NewNpmPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("POST", "-/npm/v1/security/advisories/bulk", strings.NewReader(`{}`))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "{}" {
		t.Errorf("expected '{}', got %q", w.Body.String())
	}
}

func TestHandle_AllPackages(t *testing.T) {
	p := NewNpmPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "lodash", "version": "4.17.21"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.18.2"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "-/all", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandle_Upload(t *testing.T) {
	p := NewNpmPlugin()
	rt := &testhelper.MockRuntime{}

	body, _ := json.Marshal(map[string]interface{}{
		"name":    "my-pkg",
		"version": "1.0.0",
	})
	ctx, w := newCtx("PUT", "my-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestHandle_UploadWithTarball(t *testing.T) {
	p := NewNpmPlugin()
	rt := &testhelper.MockRuntime{}

	// npm publish body with _attachments
	body, _ := json.Marshal(map[string]interface{}{
		"name":    "my-pkg",
		"version": "1.0.0",
		"_attachments": map[string]interface{}{
			"my-pkg-1.0.0.tgz": map[string]interface{}{
				"content_type": "application/octet-stream",
				"data":         "dGVzdC10YXJiYWxs", // base64 "test-tarball"
			},
		},
	})
	ctx, w := newCtx("PUT", "my-pkg", bytes.NewReader(body))
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	// Should have 2 artifacts: tarball + metadata
	if len(rt.UploadedArts) != 2 {
		t.Fatalf("expected 2 uploaded artifacts (tarball + metadata), got %d", len(rt.UploadedArts))
	}
}

func TestHandle_QueryRemotePath(t *testing.T) {
	p := NewNpmPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "express", nil)
	p.Handle(ctx, rt)

	if len(rt.QueryCalls) != 1 {
		t.Fatalf("expected 1 query call, got %d", len(rt.QueryCalls))
	}
	if rt.QueryCalls[0].RemotePath != "express" {
		t.Errorf("expected RemotePath 'express', got %q", rt.QueryCalls[0].RemotePath)
	}
}

func TestFetchRemote_ParsesVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "lodash",
			"versions": map[string]interface{}{
				"4.17.20": map[string]interface{}{},
				"4.17.21": map[string]interface{}{},
			},
		})
	}))
	defer srv.Close()

	p := NewNpmPlugin()
	arts, err := p.FetchRemote(context.Background(), srv.URL, "lodash")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(arts))
	}
}

func TestFetchRemote_ScopedPackageEncoding(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RawPath
		if capturedPath == "" {
			capturedPath = r.URL.Path
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":     "@scope/pkg",
			"versions": map[string]interface{}{"1.0.0": map[string]interface{}{}},
		})
	}))
	defer srv.Close()

	p := NewNpmPlugin()
	p.FetchRemote(context.Background(), srv.URL, "@scope/pkg")

	if !strings.Contains(capturedPath, "%40scope%2Fpkg") {
		t.Errorf("expected URL-encoded scoped package, got path: %s", capturedPath)
	}
}

func TestDistTags_SemverSorting(t *testing.T) {
	p := NewNpmPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "v1.0.0"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "v2.0.0"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "v1.5.0"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "pkg", nil)
	p.Handle(ctx, rt)

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	distTags := result["dist-tags"].(map[string]interface{})
	if distTags["latest"] != "v2.0.0" {
		t.Errorf("expected latest 'v2.0.0', got %v", distTags["latest"])
	}
}
