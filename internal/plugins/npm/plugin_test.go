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
	if len(arts) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(arts))
	}
	var versions, tarballs int
	for _, a := range arts {
		switch a.Kind {
		case "version":
			versions++
		case "tarball":
			tarballs++
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

	p := NewNpmPlugin()
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

func TestHandle_TarballDownload_ScopedPackage(t *testing.T) {
	p := NewNpmPlugin()
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
	p := NewNpmPlugin()
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
	p := NewNpmPlugin()
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
	p := NewNpmPlugin()
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
	p := NewNpmPlugin()
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
	p := NewNpmPlugin()
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
	p := NewNpmPlugin()
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

	p := NewNpmPlugin()
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

	p := NewNpmPlugin()
	arts, err := p.FetchRemote(context.Background(), srv.URL, "missing-pkg")
	if err == nil {
		t.Fatalf("expected error for non-200 response, got nil")
	}
	if arts != nil {
		t.Fatalf("expected nil artifacts on error, got %d", len(arts))
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("expected error to contain 'status 404', got %v", err)
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

	p := NewNpmPlugin()
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
		artMap[a.Coordinates["version"]] = a
	}

	v1 := artMap["1.0.0"]
	if v1 == nil {
		t.Fatal("version 1.0.0 not found")
	}
	if v1.Properties["license"] != "Apache-2.0" {
		t.Errorf("v1 license: expected 'Apache-2.0', got %q", v1.Properties["license"])
	}
	if v1.Properties["description"] != "version-specific description" {
		t.Errorf("v1 description: expected 'version-specific description', got %q", v1.Properties["description"])
	}
	if v1.Properties["homepage"] != "https://example.com" {
		t.Errorf("v1 homepage: expected 'https://example.com', got %q", v1.Properties["homepage"])
	}
	if v1.Properties["published_at"] != "2024-01-15T10:00:00.000Z" {
		t.Errorf("v1 published_at: expected '2024-01-15T10:00:00.000Z', got %q", v1.Properties["published_at"])
	}

	v2 := artMap["2.0.0"]
	if v2 == nil {
		t.Fatal("version 2.0.0 not found")
	}
	if v2.Properties["license"] != "BSD-3-Clause" {
		t.Errorf("v2 license: expected 'BSD-3-Clause' (from object), got %q", v2.Properties["license"])
	}
	if v2.Properties["description"] != "top-level description" {
		t.Errorf("v2 description: expected fallback to top-level, got %q", v2.Properties["description"])
	}
	if v2.Properties["homepage"] != "https://v2.example.com" {
		t.Errorf("v2 homepage: expected version-specific 'https://v2.example.com', got %q", v2.Properties["homepage"])
	}
	if v2.Properties["published_at"] != "2024-06-20T15:30:00.000Z" {
		t.Errorf("v2 published_at: expected '2024-06-20T15:30:00.000Z', got %q", v2.Properties["published_at"])
	}
}

func TestDistTags_Prerelease(t *testing.T) {
	p := NewNpmPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0-alpha.1"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0-beta.1"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0-rc.1"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v1.0.0"}, ""),
		testhelper.NewArtifact("npm", "version", map[string]string{"name": "prerelease-pkg", "version": "v0.9.0"}, ""),
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
