package npm

import (
	"bytes"
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
	req := httptest.NewRequest(method, "/repository/npm-test/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "npm-test", Format: "npm", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

func TestHandle_PackageMetadata(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.18.2", "remote_path": "express"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.17.3", "remote_path": "express"}, ""),
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

func TestHandle_PackageMetadataHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "express", "version": "4.18.2", "remote_path": "express"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("HEAD", "express", nil)
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

func TestHandle_ScopedPackage(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，scoped 包名含 @ 但无斜杠，remote_path 需与包名一致
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "@scope/pkg", "version": "1.0.0", "remote_path": "@scope/pkg"}, ""),
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
	p := NewNpmPlugin(http.DefaultClient)
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

func TestHandle_TarballHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("npm", "tarball", map[string]string{
		"name":     "express",
		"version":  "4.18.2",
		"path":     "express/-",
		"filename": "express-4.18.2.tgz",
	}, "tarball-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "express/-/express-4.18.2.tgz", nil)
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

func TestHandle_TarballRangeReturnsPartialContent(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("npm", "tarball", map[string]string{
		"name":     "express",
		"version":  "4.18.2",
		"path":     "express/-",
		"filename": "express-4.18.2.tgz",
	}, "tarball-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "express/-/express-4.18.2.tgz", nil)
	ctx.Request.Header.Set("Range", "bytes=8-14")
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", w.Code)
	}
	if body := w.Body.String(); body != "content" {
		t.Fatalf("expected partial body %q, got %q", "content", body)
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 8-14/15" {
		t.Fatalf("expected Content-Range bytes 8-14/15, got %q", got)
	}
}

func TestHandle_Ping(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
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
	p := NewNpmPlugin(http.DefaultClient)
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
	p := NewNpmPlugin(http.DefaultClient)
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
	p := NewNpmPlugin(http.DefaultClient)
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
	p := NewNpmPlugin(http.DefaultClient)
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
	p := NewNpmPlugin(http.DefaultClient)
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

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "lodash")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(arts))
	}
	var versions, tarballs int
	for _, a := range arts {
		switch a.Kind {
		case runtime.KindVersion:
			versions++
		case runtime.KindArtifact:
			if a.Attributes["artifact_type"] == "tarball" {
				tarballs++
			}
		}
	}
	if versions != 2 || tarballs != 2 {
		t.Fatalf("expected 2 version and 2 tarball artifacts, got versions=%d tarballs=%d", versions, tarballs)
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

	p := NewNpmPlugin(http.DefaultClient)
	p.FetchRemote(context.Background(), srv.URL, "@scope/pkg")

	if !strings.Contains(capturedPath, "%40scope%2Fpkg") {
		t.Errorf("expected URL-encoded scoped package, got path: %s", capturedPath)
	}
}

func TestRepoBaseURLUsesForwardedPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal/repository/npm-proxy/lodash", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "registry.example.com")
	req.Header.Set("X-Forwarded-Prefix", "/moonlight")

	got := repoBaseURL(req, "npm-proxy")
	want := "https://registry.example.com/moonlight/repository/npm-proxy"
	if got != want {
		t.Fatalf("repoBaseURL() = %q, want %q", got, want)
	}
}

func TestRepoBaseURLAvoidsDuplicateRepositoryPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal/repository/npm-proxy/lodash", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "registry.example.com")
	req.Header.Set("X-Forwarded-Prefix", "/moonlight/repository/npm-proxy")

	got := repoBaseURL(req, "npm-proxy")
	want := "https://registry.example.com/moonlight/repository/npm-proxy"
	if got != want {
		t.Fatalf("repoBaseURL() = %q, want %q", got, want)
	}
}

func TestRepoBaseURLSupportsRootMountedRepository(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal/repository/npm/lodash", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "registry.example.com")
	req.Header.Set("X-Forwarded-Prefix", "/")

	got := repoBaseURL(req, "npm")
	want := "https://registry.example.com"
	if got != want {
		t.Fatalf("repoBaseURL() = %q, want %q", got, want)
	}
}

func TestDistTags_SemverSorting(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "v1.0.0", "remote_path": "pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "v2.0.0", "remote_path": "pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "v1.5.0", "remote_path": "pkg"}, ""),
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

func TestDistTags_NpmVersionsWithoutVPrefix(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "1.9.0", "remote_path": "pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "1.10.0", "remote_path": "pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "pkg", "version": "2.0.0-beta.1", "remote_path": "pkg"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	distTags := result["dist-tags"].(map[string]interface{})
	if distTags["latest"] != "1.10.0" {
		t.Fatalf("expected latest '1.10.0', got %v", distTags["latest"])
	}
}

func TestHandle_TarballDownload_ScopedPackage(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	packageName := "@scope/pkg"
	// npm 规范：scoped 包的 tarball 文件名不含 scope 前缀
	art := testhelper.NewArtifact("npm", "tarball", map[string]string{
		"name":     packageName,
		"version":  "1.0.0",
		"path":     packageName + "/-",
		"filename": "pkg-1.0.0.tgz",
	}, "scoped-tarball-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	// 请求路径: @scope/pkg/-/pkg-1.0.0.tgz（文件名不含 scope 前缀）
	ctx, w := newCtx("GET", packageName+"/-/pkg-1.0.0.tgz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "scoped-tarball-content" {
		t.Errorf("expected 'scoped-tarball-content', got %q", w.Body.String())
	}
	if ctx.PackageName != packageName {
		t.Errorf("expected PackageName %q, got %q", packageName, ctx.PackageName)
	}
	if ctx.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", ctx.Version)
	}
}

func TestHandle_TarballDownload_InvalidPath(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "foo/-/bar/-/baz.tgz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandle_TarballDownloadMissQueriesPackageMetadataBeforeRetry(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &npmQueryThenGetRuntime{
		artifact: testhelper.NewArtifact("npm", "tarball", map[string]string{
			"name":     "@scope/pkg",
			"version":  "1.0.0",
			"path":     "@scope/pkg/-",
			"filename": "pkg-1.0.0.tgz",
		}, "tarball-content"),
	}

	ctx, w := newCtx("GET", "@scope/pkg/-/pkg-1.0.0.tgz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after QueryArtifacts retry, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.queryCalls) != 1 {
		t.Fatalf("expected one QueryArtifacts call, got %d", len(rt.queryCalls))
	}
	if got := rt.queryCalls[0].RemotePath; got != "@scope/pkg" {
		t.Fatalf("RemotePath = %q", got)
	}
	if rt.getCalls != 2 {
		t.Fatalf("expected two GetArtifact calls, got %d", rt.getCalls)
	}
}

type npmQueryThenGetRuntime struct {
	artifact   *runtime.Artifact
	getCalls   int
	queryCalls []runtime.ArtifactQuery
	queried    bool
}

func (r *npmQueryThenGetRuntime) GetArtifact(ctx context.Context, key runtime.ArtifactKey) (*runtime.Artifact, error) {
	r.getCalls++
	if !r.queried {
		return nil, runtime.ErrNotFound
	}
	return r.artifact, nil
}

func (r *npmQueryThenGetRuntime) QueryArtifacts(ctx context.Context, query runtime.ArtifactQuery) ([]*runtime.Artifact, error) {
	r.queryCalls = append(r.queryCalls, query)
	r.queried = true
	return []*runtime.Artifact{r.artifact}, nil
}

func (r *npmQueryThenGetRuntime) RenderProjection(ctx context.Context, query runtime.ProjectionQuery) (*runtime.ProjectionResult, error) {
	return nil, runtime.ErrNotFound
}

func (r *npmQueryThenGetRuntime) BeginUpload(ctx context.Context, req runtime.UploadRequest) (runtime.UploadSession, error) {
	return nil, runtime.ErrReadOnly
}

func (r *npmQueryThenGetRuntime) DeleteArtifact(ctx context.Context, key runtime.ArtifactKey) error {
	return runtime.ErrReadOnly
}

func TestHandle_PackageNotFound(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{}}

	ctx, w := newCtx("GET", "nonexistent-pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandle_EmptyVersions(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("npm", "metadata", map[string]string{
		"name": "pkg-no-versions",
	}, "")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "pkg-no-versions", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandle_NpmInternal_NotFound(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "-/npm/unknown/endpoint", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if w.Body.String() != "Not found\n" {
		t.Errorf("expected 'Not found\\n', got %q", w.Body.String())
	}
}

func TestHandle_MethodNotAllowed(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("POST", "express", nil)
	err := p.Handle(ctx, rt)
	if err == nil || err.Error() != "method not allowed" {
		t.Fatalf("expected 'method not allowed' error, got %v", err)
	}
}

func TestFetchRemote_EmptyPackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":     "empty-pkg",
			"versions": nil,
		})
	}))
	defer srv.Close()

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "empty-pkg")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if arts != nil {
		t.Fatalf("expected nil artifacts for empty package, got %d", len(arts))
	}
}

func TestFetchRemote_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "missing-pkg")
	if err == nil {
		t.Fatalf("expected error for non-200 response, got nil")
	}
	if arts != nil {
		t.Fatalf("expected nil artifacts on error, got %d", len(arts))
	}
	if !errors.Is(err, runtime.ErrNotFound) {
		t.Errorf("expected ErrNotFound for upstream 404, got %v", err)
	}
}

func TestParseNpmMetadata(t *testing.T) {
	registryJSON := `{
		"name": "test-pkg",
		"description": "top-level description",
		"homepage": "https://example.com",
		"license": "MIT",
		"time": {
			"1.0.0": "2024-01-15T10:00:00.000Z",
			"2.0.0": "2024-06-20T15:30:00.000Z"
		},
		"versions": {
			"1.0.0": {
				"name": "test-pkg",
				"version": "1.0.0",
				"description": "version-specific description",
				"license": "Apache-2.0"
			},
			"2.0.0": {
				"name": "test-pkg",
				"version": "2.0.0",
				"license": {
					"type": "BSD-3-Clause",
					"url": "https://opensource.org/licenses/BSD-3-Clause"
				},
				"homepage": "https://v2.example.com"
			}
		}
	}`

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.parseNpmMetadata("test-pkg", strings.NewReader(registryJSON))
	if err != nil {
		t.Fatalf("parseNpmMetadata failed: %v", err)
	}
	if len(arts) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(arts))
	}

	artMap := make(map[string]*runtime.Artifact)
	for _, a := range arts {
		if a.Kind != "version" {
			continue
		}
		artMap[a.Version] = a
	}

	v1 := artMap["1.0.0"]
	if v1 == nil {
		t.Fatal("version 1.0.0 not found")
	}
	if v1.Attributes["license"] != "Apache-2.0" {
		t.Errorf("v1 attributes license: expected 'Apache-2.0', got %q", v1.Attributes["license"])
	}
	if v1.Attributes["description"] != "version-specific description" {
		t.Errorf("v1 description: expected 'version-specific description', got %q", v1.Attributes["description"])
	}
	if v1.Attributes["homepage"] != "https://example.com" {
		t.Errorf("v1 homepage: expected 'https://example.com', got %q", v1.Attributes["homepage"])
	}
	if v1.Attributes["published_at"] != "2024-01-15T10:00:00.000Z" {
		t.Errorf("v1 published_at: expected '2024-01-15T10:00:00.000Z', got %q", v1.Attributes["published_at"])
	}

	v2 := artMap["2.0.0"]
	if v2 == nil {
		t.Fatal("version 2.0.0 not found")
	}
	if v2.Attributes["license"] != "BSD-3-Clause" {
		t.Errorf("v2 license: expected 'BSD-3-Clause' (from object), got %q", v2.Attributes["license"])
	}
	if v2.Attributes["description"] != "top-level description" {
		t.Errorf("v2 description: expected fallback to top-level, got %q", v2.Attributes["description"])
	}
	if v2.Attributes["homepage"] != "https://v2.example.com" {
		t.Errorf("v2 homepage: expected version-specific 'https://v2.example.com', got %q", v2.Attributes["homepage"])
	}
	if v2.Attributes["published_at"] != "2024-06-20T15:30:00.000Z" {
		t.Errorf("v2 published_at: expected '2024-06-20T15:30:00.000Z', got %q", v2.Attributes["published_at"])
	}
}

func TestIsValidSPDXLicense(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		// 合法 SPDX 标识符
		{"MIT", true},
		{"Apache-2.0", true},
		{"BSD-3-Clause", true},
		{"GPL-3.0-or-later", true},
		{"ISC", true},
		{"0BSD", true},
		{"Unlicense", true},
		{"MPL-2.0", true},
		{"LGPL-2.1+", true},
		// SPDX 表达式
		{"MIT OR Apache-2.0", true},
		{"GPL-3.0 WITH GCC-exception-3.1", true},
		{"(MIT OR Apache-2.0) AND BSD-3-Clause", true},
		// 非 SPDX 值
		{"SEE LICENSE IN README.md", false},
		{"See License in LICENSE", false},
		{"UNLICENSED", false},
		{"none", false},
		{"N/A", false},
		{"unknown", false},
		{"not specified", false},
		// 文件路径和 URL
		{"/path/to/license", false},
		{"./LICENSE", false},
		{"http://example.com/license", false},
		{"https://example.com/license", false},
		// 包含非法字符
		{"MIT & Apache", false},
		{"License: MIT", false},
		// 空值
		{"", false},
		{"  ", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidSPDXLicense(tt.input)
			if got != tt.valid {
				t.Errorf("isValidSPDXLicense(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestExtractLicenseFiltersNonSPDX(t *testing.T) {
	tests := []struct {
		name     string
		obj      map[string]interface{}
		expected string
	}{
		{
			name:     "valid SPDX string",
			obj:      map[string]interface{}{"license": "MIT"},
			expected: "MIT",
		},
		{
			name:     "valid SPDX from object",
			obj:      map[string]interface{}{"license": map[string]interface{}{"type": "Apache-2.0", "url": "https://opensource.org/licenses/Apache-2.0"}},
			expected: "Apache-2.0",
		},
		{
			name:     "SEE LICENSE IN README.md filtered",
			obj:      map[string]interface{}{"license": "SEE LICENSE IN README.md"},
			expected: "",
		},
		{
			name:     "URL filtered",
			obj:      map[string]interface{}{"license": "https://opensource.org/licenses/MIT"},
			expected: "",
		},
		{
			name:     "none filtered",
			obj:      map[string]interface{}{"license": "none"},
			expected: "",
		},
		{
			name:     "nil license",
			obj:      map[string]interface{}{"license": nil},
			expected: "",
		},
		{
			name:     "no license field",
			obj:      map[string]interface{}{},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLicense(tt.obj)
			if got != tt.expected {
				t.Errorf("extractLicense() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDistTags_Prerelease(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// handlePackageGet 用 RemotePath=packageName 查询，artifact 需带匹配的 remote_path
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0-alpha.1", "remote_path": "prerelease-pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0-beta.1", "remote_path": "prerelease-pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0-rc.1", "remote_path": "prerelease-pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0", "remote_path": "prerelease-pkg"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v0.9.0", "remote_path": "prerelease-pkg"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "prerelease-pkg", nil)
	p.Handle(ctx, rt)

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	distTags := result["dist-tags"].(map[string]interface{})
	if distTags["latest"] != "v1.0.0" {
		t.Errorf("expected latest 'v1.0.0', got %v", distTags["latest"])
	}

	versions := result["versions"].(map[string]interface{})
	if len(versions) != 5 {
		t.Errorf("expected 5 versions, got %d", len(versions))
	}
	expectedVersions := []string{"v1.0.0-alpha.1", "v1.0.0-beta.1", "v1.0.0-rc.1", "v1.0.0", "v0.9.0"}
	for _, v := range expectedVersions {
		if _, ok := versions[v]; !ok {
			t.Errorf("expected version %q not found", v)
		}
	}
}

// TestParseNpmMetadata_ExtractsBinAndComplexFields 验证 parseNpmMetadata
// 从上游 npm registry JSON 中提取 bin/main/scripts/dependencies 等关键字段。
func TestParseNpmMetadata_ExtractsBinAndComplexFields(t *testing.T) {
	registryJSON := `{
		"name": "cli-tool",
		"description": "A CLI tool",
		"versions": {
			"1.0.0": {
				"name": "cli-tool",
				"version": "1.0.0",
				"description": "A CLI tool",
				"main": "lib/index.js",
				"module": "es/index.js",
				"type": "module",
				"bin": {
					"cli-tool": "./bin/cli.js",
					"cli-helper": "./bin/helper.js"
				},
				"scripts": {
					"build": "tsc",
					"test": "jest"
				},
				"dependencies": {
					"lodash": "^4.17.21",
					"express": "^4.18.0"
				},
				"devDependencies": {
					"typescript": "^5.0.0"
				},
				"peerDependencies": {
					"react": ">=18.0.0"
				},
				"optionalDependencies": {
					"fsevents": "^2.3.0"
				},
				"engines": {
					"node": ">=16.0.0"
				},
				"os": ["linux", "darwin"],
				"cpu": ["x64", "arm64"],
				"directories": {
					"bin": "./bin",
					"lib": "./lib"
				},
				"man": ["./man/cli-tool.1"],
				"repository": {
					"type": "git",
					"url": "https://github.com/example/cli-tool.git"
				},
				"keywords": ["cli", "tool"],
				"author": "John Doe",
				"contributors": [{"name": "Jane"}],
				"license": "MIT",
				"homepage": "https://example.com",
				"dist": {
					"tarball": "https://registry.npmjs.org/cli-tool/-/cli-tool-1.0.0.tgz",
					"shasum": "abc123def456",
					"integrity": "sha512-xyz789",
					"unpackedSize": 12345
				}
			}
		}
	}`

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.parseNpmMetadata("cli-tool", strings.NewReader(registryJSON))
	if err != nil {
		t.Fatalf("parseNpmMetadata failed: %v", err)
	}

	// 找到 version artifact
	var versionArt *runtime.Artifact
	for _, a := range arts {
		if a.Kind == "version" && a.Version == "1.0.0" {
			versionArt = a
			break
		}
	}
	if versionArt == nil {
		t.Fatal("version 1.0.0 artifact not found")
	}

	// 验证字符串字段
	assertStrAttr := func(key, want string) {
		if got := versionArt.Attributes[key]; got != want {
			t.Errorf("Attributes[%q] = %q, want %q", key, got, want)
		}
	}
	assertStrAttr("description", "A CLI tool")
	assertStrAttr("main", "lib/index.js")
	assertStrAttr("module", "es/index.js")
	assertStrAttr("type", "module")
	assertStrAttr("license", "MIT")
	assertStrAttr("homepage", "https://example.com")
	assertStrAttr("shasum", "abc123def456")
	assertStrAttr("integrity", "sha512-xyz789")

	// 验证 JSON 序列化的复合字段
	assertJSONAttr := func(key string, expected interface{}) {
		got, ok := versionArt.Attributes[key]
		if !ok {
			t.Errorf("Attributes[%q] not found", key)
			return
		}
		var gotVal interface{}
		if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
			t.Errorf("Attributes[%q] = %q, failed to unmarshal: %v", key, got, err)
			return
		}
		gotJSON, _ := json.Marshal(gotVal)
		wantJSON, _ := json.Marshal(expected)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("Attributes[%q] = %s, want %s", key, gotJSON, wantJSON)
		}
	}
	assertJSONAttr("bin", map[string]interface{}{"cli-tool": "./bin/cli.js", "cli-helper": "./bin/helper.js"})
	assertJSONAttr("scripts", map[string]interface{}{"build": "tsc", "test": "jest"})
	assertJSONAttr("dependencies", map[string]interface{}{"lodash": "^4.17.21", "express": "^4.18.0"})
	assertJSONAttr("devDependencies", map[string]interface{}{"typescript": "^5.0.0"})
	assertJSONAttr("peerDependencies", map[string]interface{}{"react": ">=18.0.0"})
	assertJSONAttr("optionalDependencies", map[string]interface{}{"fsevents": "^2.3.0"})
	assertJSONAttr("engines", map[string]interface{}{"node": ">=16.0.0"})
	assertJSONAttr("os", []interface{}{"linux", "darwin"})
	assertJSONAttr("cpu", []interface{}{"x64", "arm64"})
	assertJSONAttr("directories", map[string]interface{}{"bin": "./bin", "lib": "./lib"})
	assertJSONAttr("man", []interface{}{"./man/cli-tool.1"})
	assertJSONAttr("keywords", []interface{}{"cli", "tool"})
}

// TestParseNpmMetadata_BinAsString 验证 bin 字段为字符串时的提取。
// npm 规范允许 bin 为字符串（如 "bin": "./bin/cli.js"）。
func TestParseNpmMetadata_BinAsString(t *testing.T) {
	registryJSON := `{
		"name": "simple-cli",
		"versions": {
			"1.0.0": {
				"name": "simple-cli",
				"version": "1.0.0",
				"bin": "./bin/cli.js"
			}
		}
	}`

	p := NewNpmPlugin(http.DefaultClient)
	arts, err := p.parseNpmMetadata("simple-cli", strings.NewReader(registryJSON))
	if err != nil {
		t.Fatalf("parseNpmMetadata failed: %v", err)
	}

	var versionArt *runtime.Artifact
	for _, a := range arts {
		if a.Kind == "version" && a.Version == "1.0.0" {
			versionArt = a
			break
		}
	}
	if versionArt == nil {
		t.Fatal("version 1.0.0 artifact not found")
	}

	// bin 为字符串时，JSON 序列化后是 "\"./bin/cli.js\""
	binVal := versionArt.Attributes["bin"]
	if binVal == "" {
		t.Fatal("Attributes[\"bin\"] is empty")
	}
	var decoded interface{}
	if err := json.Unmarshal([]byte(binVal), &decoded); err != nil {
		t.Fatalf("failed to unmarshal bin: %v", err)
	}
	if decoded != "./bin/cli.js" {
		t.Errorf("bin = %v, want \"./bin/cli.js\"", decoded)
	}
}

// TestHandlePackageGet_RestoresBinField 验证 handlePackageGet
// 从 artifact Attributes 还原 bin 等字段到版本元数据响应中。
func TestHandlePackageGet_RestoresBinField(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "cli-tool",
			Version:    "1.0.0",
			RemotePath: "cli-tool",
			Attributes: map[string]string{
				"description":  "A CLI tool",
				"main":         "lib/index.js",
				"bin":          `{"cli-tool":"./bin/cli.js"}`,
				"scripts":      `{"build":"tsc"}`,
				"dependencies": `{"lodash":"^4.17.21"}`,
				"shasum":       "abc123",
				"integrity":    "sha512-xyz",
			},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "cli-tool", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	versions := result["versions"].(map[string]interface{})
	v100 := versions["1.0.0"].(map[string]interface{})
	if v100["name"] != "cli-tool" {
		t.Errorf("name = %v, want cli-tool", v100["name"])
	}
	if v100["version"] != "1.0.0" {
		t.Errorf("version = %v, want 1.0.0", v100["version"])
	}

	// 验证字符串字段还原
	if v100["description"] != "A CLI tool" {
		t.Errorf("description = %v, want 'A CLI tool'", v100["description"])
	}
	if v100["main"] != "lib/index.js" {
		t.Errorf("main = %v, want 'lib/index.js'", v100["main"])
	}

	// 验证 bin 字段还原（对象形式）
	bin, ok := v100["bin"].(map[string]interface{})
	if !ok {
		t.Fatalf("bin = %v, want map[string]interface{}", v100["bin"])
	}
	if bin["cli-tool"] != "./bin/cli.js" {
		t.Errorf("bin[\"cli-tool\"] = %v, want \"./bin/cli.js\"", bin["cli-tool"])
	}

	// 验证 scripts 字段还原
	scripts, ok := v100["scripts"].(map[string]interface{})
	if !ok {
		t.Fatalf("scripts = %v, want map[string]interface{}", v100["scripts"])
	}
	if scripts["build"] != "tsc" {
		t.Errorf("scripts[\"build\"] = %v, want \"tsc\"", scripts["build"])
	}

	// 验证 dependencies 字段还原
	deps, ok := v100["dependencies"].(map[string]interface{})
	if !ok {
		t.Fatalf("dependencies = %v, want map[string]interface{}", v100["dependencies"])
	}
	if deps["lodash"] != "^4.17.21" {
		t.Errorf("dependencies[\"lodash\"] = %v, want \"^4.17.21\"", deps["lodash"])
	}

	// 验证 dist 子字段还原
	dist, ok := v100["dist"].(map[string]interface{})
	if !ok {
		t.Fatalf("dist = %v, want map[string]interface{}", v100["dist"])
	}
	if dist["shasum"] != "abc123" {
		t.Errorf("dist[\"shasum\"] = %v, want \"abc123\"", dist["shasum"])
	}
	if dist["integrity"] != "sha512-xyz" {
		t.Errorf("dist[\"integrity\"] = %v, want \"sha512-xyz\"", dist["integrity"])
	}
	// tarball URL 必须存在
	if _, hasTarball := dist["tarball"]; !hasTarball {
		t.Error("dist[\"tarball\"] is missing")
	}
}

// TestHandlePackageGet_BinStringFallback 验证 bin 为字符串时
// 在响应中正确还原为字符串（而非对象）。
func TestHandlePackageGet_BinStringFallback(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "simple-cli",
			Version:    "1.0.0",
			RemotePath: "simple-cli",
			Attributes: map[string]string{
				"bin": `"./bin/cli.js"`,
			},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple-cli", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	versions := result["versions"].(map[string]interface{})
	v100 := versions["1.0.0"].(map[string]interface{})

	// bin 为字符串时，还原后也应该是字符串
	bin, ok := v100["bin"].(string)
	if !ok {
		t.Fatalf("bin = %v (type %T), want string", v100["bin"], v100["bin"])
	}
	if bin != "./bin/cli.js" {
		t.Errorf("bin = %q, want \"./bin/cli.js\"", bin)
	}
}

// TestHandlePackageGet_TimeField 验证 handlePackageGet
// 从 Attributes 还原 time 字段到响应中。
func TestHandlePackageGet_TimeField(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "timed-pkg",
			Version:    "1.0.0",
			RemotePath: "timed-pkg",
			Attributes: map[string]string{
				"published_at": "2024-01-15T10:00:00.000Z",
			},
		}),
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "timed-pkg",
			Version:    "2.0.0",
			RemotePath: "timed-pkg",
			Attributes: map[string]string{
				"published_at": "2024-06-20T15:30:00.000Z",
			},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "timed-pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)

	timeMap, ok := result["time"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing 'time' field")
	}
	if timeMap["1.0.0"] != "2024-01-15T10:00:00.000Z" {
		t.Errorf("time[\"1.0.0\"] = %v, want 2024-01-15T10:00:00.000Z", timeMap["1.0.0"])
	}
	if timeMap["2.0.0"] != "2024-06-20T15:30:00.000Z" {
		t.Errorf("time[\"2.0.0\"] = %v, want 2024-06-20T15:30:00.000Z", timeMap["2.0.0"])
	}
}

// TestHandlePackageGet_PrefersAttributesOverTarball 验证
// handlePackageGet 优先使用带 Attributes 的 artifact 构建版本元数据，
// 而非使用没有 Attributes 的 tarball artifact。
func TestHandlePackageGet_PrefersAttributesOverTarball(t *testing.T) {
	p := NewNpmPlugin(http.DefaultClient)
	// tarball artifact 没有 Attributes，先出现
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "tarball", map[string]string{
			"name": "cli-tool", "version": "1.0.0",
			"path": "cli-tool/-", "filename": "cli-tool-1.0.0.tgz",
		}, ""),
		// version artifact 有 Attributes，后出现
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "npm",
			Kind:       runtime.KindVersion,
			Name:       "cli-tool",
			Version:    "1.0.0",
			RemotePath: "cli-tool",
			Attributes: map[string]string{
				"bin":  `{"cli-tool":"./bin/cli.js"}`,
				"main": "lib/index.js",
			},
		}),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "cli-tool", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	versions := result["versions"].(map[string]interface{})
	v100 := versions["1.0.0"].(map[string]interface{})

	// 即使 tarball 先出现，version artifact 的 Attributes 应该被使用
	if _, ok := v100["bin"]; !ok {
		t.Error("bin field missing from version metadata - tarball may have overridden version artifact")
	}
	if v100["main"] != "lib/index.js" {
		t.Errorf("main = %v, want 'lib/index.js'", v100["main"])
	}
}

// TestExtractNpmVersionAttributes 验证 Hosted 上传场景中
// extractNpmVersionAttributes 正确提取关键字段到 Attributes。
func TestExtractNpmVersionAttributes(t *testing.T) {
	npmMeta := map[string]interface{}{
		"name":        "hosted-pkg",
		"version":     "1.0.0",
		"description": "A hosted package",
		"main":        "index.js",
		"bin":         "./bin/run.js",
		"scripts": map[string]interface{}{
			"start": "node index.js",
		},
		"dependencies": map[string]interface{}{
			"lodash": "^4.17.21",
		},
		"license":  "MIT",
		"homepage": "https://example.com",
		"engines": map[string]interface{}{
			"node": ">=16",
		},
		"dist": map[string]interface{}{
			"shasum":    "deadbeef",
			"integrity": "sha512-abc",
		},
	}

	attrs := extractNpmVersionAttributes(npmMeta)

	if attrs["description"] != "A hosted package" {
		t.Errorf("description = %q, want 'A hosted package'", attrs["description"])
	}
	if attrs["main"] != "index.js" {
		t.Errorf("main = %q, want 'index.js'", attrs["main"])
	}
	if attrs["license"] != "MIT" {
		t.Errorf("license = %q, want 'MIT'", attrs["license"])
	}
	if attrs["shasum"] != "deadbeef" {
		t.Errorf("shasum = %q, want 'deadbeef'", attrs["shasum"])
	}
	if attrs["integrity"] != "sha512-abc" {
		t.Errorf("integrity = %q, want 'sha512-abc'", attrs["integrity"])
	}

	// 验证 JSON 序列化的复合字段
	assertJSONAttr := func(key string, expected interface{}) {
		got, ok := attrs[key]
		if !ok {
			t.Errorf("attrs[%q] not found", key)
			return
		}
		var gotVal interface{}
		if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
			t.Errorf("attrs[%q] = %q, failed to unmarshal: %v", key, got, err)
			return
		}
		gotJSON, _ := json.Marshal(gotVal)
		wantJSON, _ := json.Marshal(expected)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("attrs[%q] = %s, want %s", key, gotJSON, wantJSON)
		}
	}
	assertJSONAttr("bin", "./bin/run.js")
	assertJSONAttr("scripts", map[string]interface{}{"start": "node index.js"})
	assertJSONAttr("dependencies", map[string]interface{}{"lodash": "^4.17.21"})
	assertJSONAttr("engines", map[string]interface{}{"node": ">=16"})
}

// TestRestoreJSONField 验证 restoreJSONField 辅助函数的正确性。
func TestRestoreJSONField(t *testing.T) {
	t.Run("object value", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"bin": `{"cli":"./bin/cli.js"}`}, "bin", target)
		bin, ok := target["bin"].(map[string]interface{})
		if !ok {
			t.Fatalf("bin = %T, want map[string]interface{}", target["bin"])
		}
		if bin["cli"] != "./bin/cli.js" {
			t.Errorf("bin[\"cli\"] = %v, want \"./bin/cli.js\"", bin["cli"])
		}
	})

	t.Run("string value", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"bin": `"./bin/run.js"`}, "bin", target)
		bin, ok := target["bin"].(string)
		if !ok {
			t.Fatalf("bin = %T, want string", target["bin"])
		}
		if bin != "./bin/run.js" {
			t.Errorf("bin = %q, want \"./bin/run.js\"", bin)
		}
	})

	t.Run("array value", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"os": `["linux","darwin"]`}, "os", target)
		os, ok := target["os"].([]interface{})
		if !ok {
			t.Fatalf("os = %T, want []interface{}", target["os"])
		}
		if os[0] != "linux" || os[1] != "darwin" {
			t.Errorf("os = %v, want [linux darwin]", os)
		}
	})

	t.Run("targetKey override", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"dist_signatures": `[{}]`}, "dist_signatures", target, "signatures")
		if _, ok := target["signatures"]; !ok {
			t.Error("targetKey override failed: 'signatures' key not found")
		}
		if _, ok := target["dist_signatures"]; ok {
			t.Error("original key 'dist_signatures' should not exist when targetKey is provided")
		}
	})

	t.Run("empty value skipped", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{"bin": ""}, "bin", target)
		if _, ok := target["bin"]; ok {
			t.Error("empty value should be skipped")
		}
	})

	t.Run("missing key skipped", func(t *testing.T) {
		target := map[string]interface{}{}
		restoreJSONField(map[string]string{}, "bin", target)
		if _, ok := target["bin"]; ok {
			t.Error("missing key should be skipped")
		}
	})
}
