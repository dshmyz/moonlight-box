package pypi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/plugins/testhelper"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"github.com/dshmyz/moonlight-box/internal/util"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	_ = util.InitLogger(&util.LoggerConfig{Level: "error", Format: "console", Output: "stdout"})
}

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

func newHostedPyPIRuntime(t *testing.T) runtime.RepositoryRuntime {
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

func TestHandle_SimpleIndex(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "requests", "package": "requests"}, ""),
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "flask", "package": "flask"}, ""),
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

func TestHandle_SimpleIndexHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "requests", "package": "requests"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("HEAD", "simple/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html, got %q", ct)
	}
}

func TestHandle_SimpleIndexAggregatesHostedPackageNames(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	// 用真实 HostedRuntime 上传两个不同包
	rt := newHostedPyPIRuntime(t)

	// 上传 requests
	req1 := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "pypi",
		Kind:         "package-file",
		Name:         "requests",
		Version:      "2.28.0",
		BlobRefs:     []runtime.BlobRef{{Algorithm: "sha256", Digest: "aaaa", Size: 100}},
	})
	sess1, err := rt.BeginUpload(context.Background(), runtime.UploadRequest{RepositoryID: "1", Format: "pypi", Filename: "requests-2.28.0.tar.gz", Size: 100})
	if err != nil {
		t.Fatalf("upload session failed: %v", err)
	}
	_ = sess1.PutArtifact(context.Background(), req1)
	_ = sess1.Commit(context.Background())

	// 上传 flask
	req2 := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "pypi",
		Kind:         "package-file",
		Name:         "flask",
		Version:      "2.0.0",
		BlobRefs:     []runtime.BlobRef{{Algorithm: "sha256", Digest: "bbbb", Size: 100}},
	})
	sess2, err := rt.BeginUpload(context.Background(), runtime.UploadRequest{RepositoryID: "1", Format: "pypi", Filename: "flask-2.0.0.tar.gz", Size: 100})
	if err != nil {
		t.Fatalf("upload session failed: %v", err)
	}
	_ = sess2.PutArtifact(context.Background(), req2)
	_ = sess2.Commit(context.Background())

	// simple index 应该聚合所有包名
	ctx, w := newCtx("GET", "simple/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "requests") {
		t.Errorf("simple index missing 'requests': %s", body)
	}
	if !strings.Contains(body, "flask") {
		t.Errorf("simple index missing 'flask': %s", body)
	}
}

func TestHandle_SimpleIndexJSON(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "requests", "package": "requests"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/", nil)
	ctx.Request.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	p.Handle(ctx, rt)

	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("expected JSON content type, got %s", ct)
	}
}

func TestHandle_SimpleIndexHonorsExplicitHTMLAccept(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "requests", "package": "requests"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/", nil)
	ctx.Request.Header.Set("Accept", "application/vnd.pypi.simple.v1+html")
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if ct := w.Header().Get("Content-Type"); strings.Contains(ct, "json") {
		t.Fatalf("expected HTML content type, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("expected HTML response, got %s", w.Body.String())
	}
}

func TestHandle_SimpleIndexHonorsAcceptQualityWeights(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "requests", "package": "requests"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/", nil)
	ctx.Request.Header.Set("Accept", "application/vnd.pypi.simple.v1+json;q=0.1, application/vnd.pypi.simple.v1+html;q=0.9")
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if ct := w.Header().Get("Content-Type"); strings.Contains(ct, "json") {
		t.Fatalf("expected HTML content type from higher q weight, got %s", ct)
	}
	if got := w.Header().Get("Vary"); got != "Accept" {
		t.Fatalf("Vary = %q, want Accept", got)
	}
}

func TestHandle_PackageList(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "package-file", map[string]string{
			"name":     "requests",
			"package":  "requests",
			"version":  "2.28.0",
			"filename": "requests-2.28.0.tar.gz",
		}, ""),
	}
	arts[0].Properties = map[string]string{"remote_path": "ab/cd/requests-2.28.0.tar.gz"}
	// NewArtifact 会经 NormalizeArtifactForStore 去掉 RemotePath 尾部斜杠，
	// 这里覆盖为请求路径（含尾部斜杠），与 handlePackageList 查询的 RemotePath 一致，
	// 模拟回源后 Runtime 层存储的 RemotePath。
	arts[0].RemotePath = "simple/requests/"
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

// TestHandle_PackageListFallsBackToNameQueryWhenRemotePathMisses 覆盖协议逻辑下沉到插件后的
// 二次查询回退：本地上传的包文件其 RemotePath 形如 packages/<hash>/<file>，不等于
// simple/<name>/，故首查（按 RemotePath 精确匹配）为空，插件须再发一次清空 RemotePath、
// 按 Name 聚合的查询。断言共 2 次查询、第二次 RemotePath 为空，且渲染出文件名。
func TestHandle_PackageListFallsBackToNameQueryWhenRemotePathMisses(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "package-file", map[string]string{
			"name":     "requests",
			"package":  "requests",
			"version":  "2.28.0",
			"filename": "requests-2.28.0.tar.gz",
		}, ""),
	}
	// 存储的 RemotePath 是包文件路径，而非 simple/<name>/，故首查不命中。
	arts[0].RemotePath = "packages/ab/cd/requests-2.28.0.tar.gz"
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.QueryCalls) != 2 {
		t.Fatalf("expected 2 query calls (path then name fallback), got %d", len(rt.QueryCalls))
	}
	if got := rt.QueryCalls[0].RemotePath; got != "simple/requests/" {
		t.Fatalf("first query RemotePath = %q, want simple/requests/", got)
	}
	if got := rt.QueryCalls[1].RemotePath; got != "" {
		t.Fatalf("fallback query RemotePath = %q, want empty (Name-only aggregation)", got)
	}
	if rt.QueryCalls[1].Name != "requests" {
		t.Fatalf("fallback query Name = %q, want requests", rt.QueryCalls[1].Name)
	}
	if !strings.Contains(w.Body.String(), "requests-2.28.0.tar.gz") {
		t.Errorf("expected filename in HTML, got: %s", w.Body.String())
	}
}

func TestHandle_PackageListRedirectsToCanonicalSlashURL(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "simple/My_Pkg", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != "/repository/pypi-test/simple/my-pkg/" {
		t.Fatalf("Location = %q, want /repository/pypi-test/simple/my-pkg/", got)
	}
	if len(rt.QueryCalls) != 0 {
		t.Fatalf("expected redirect before querying runtime, got %d queries", len(rt.QueryCalls))
	}
}

func TestHandle_PackageListRedirectsNormalizedNameWithExistingSlash(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "simple/My_Pkg/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != "/repository/pypi-test/simple/my-pkg/" {
		t.Fatalf("Location = %q, want /repository/pypi-test/simple/my-pkg/", got)
	}
	if len(rt.QueryCalls) != 0 {
		t.Fatalf("expected redirect before querying runtime, got %d queries", len(rt.QueryCalls))
	}
}

func TestHandle_PackageListHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("pypi", "package-file", map[string]string{
		"name":        "requests",
		"package":     "requests",
		"version":     "2.28.0",
		"filename":    "requests-2.28.0.tar.gz",
		"remote_path": "simple/requests/",
	}, "")
	art.RemotePath = "simple/requests/"
	art.Properties = map[string]string{"remote_path": "packages/ab/cd/requests-2.28.0.tar.gz"}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "simple/requests/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html, got %q", ct)
	}
}

func TestHandle_PackageListSortsByPEP440Version(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "package-file", map[string]string{"name": "demo", "package": "demo", "version": "1.0.0.post1", "filename": "demo-1.0.0.post1.tar.gz"}, ""),
		testhelper.NewArtifact("pypi", "package-file", map[string]string{"name": "demo", "package": "demo", "version": "1.0.0a1", "filename": "demo-1.0.0a1.tar.gz"}, ""),
		testhelper.NewArtifact("pypi", "package-file", map[string]string{"name": "demo", "package": "demo", "version": "1.0.0", "filename": "demo-1.0.0.tar.gz"}, ""),
	}
	for _, a := range arts {
		a.Properties = map[string]string{"remote_path": "packages/" + a.Filename}
		// NewArtifact 会经 NormalizeArtifactForStore 去掉 RemotePath 尾部斜杠，
		// 这里覆盖为请求路径（含尾部斜杠），与 handlePackageList 查询的 RemotePath 一致，
		// 模拟回源后 Runtime 层存储的 RemotePath。
		a.RemotePath = "simple/demo/"
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/demo/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	body := w.Body.String()
	idxPre := strings.Index(body, "demo-1.0.0a1.tar.gz")
	idxFinal := strings.Index(body, "demo-1.0.0.tar.gz")
	idxPost := strings.Index(body, "demo-1.0.0.post1.tar.gz")
	if !(idxPre >= 0 && idxFinal >= 0 && idxPost >= 0 && idxPre < idxFinal && idxFinal < idxPost) {
		t.Fatalf("expected PEP 440 order pre < final < post, got body: %s", body)
	}
}

func TestHandle_PackageDownload(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("pypi", "package", map[string]string{
		"name":     "requests",
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

func TestHandle_PackageDownloadHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("pypi", "package-file", map[string]string{
		"name":     "requests",
		"package":  "requests",
		"version":  "2.28.0",
		"filename": "requests-2.28.0.tar.gz",
		"path":     "packages/ab/cd",
	}, "package-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "packages/ab/cd/requests-2.28.0.tar.gz", nil)
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

func TestHandle_PackageDownloadRangeReturnsPartialContent(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("pypi", "package-file", map[string]string{
		"name":     "requests",
		"package":  "requests",
		"version":  "2.28.0",
		"filename": "requests-2.28.0.tar.gz",
		"path":     "packages/ab/cd",
	}, "package-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "packages/ab/cd/requests-2.28.0.tar.gz", nil)
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

func TestHandle_PackageDownloadMissQueriesRemotePathBeforeRetry(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &queryThenGetRuntime{
		artifact: testhelper.NewArtifact("pypi", "package-file", map[string]string{
			"name":     "requests",
			"package":  "requests",
			"version":  "2.28.0",
			"filename": "requests-2.28.0.tar.gz",
			"path":     "packages/ab/cd",
		}, "package-content"),
	}

	ctx, w := newCtx("GET", "packages/ab/cd/requests-2.28.0.tar.gz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after QueryArtifacts retry, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.queryCalls) != 1 {
		t.Fatalf("expected one QueryArtifacts call, got %d", len(rt.queryCalls))
	}
	if got := rt.queryCalls[0].RemotePath; got != "packages/ab/cd/requests-2.28.0.tar.gz" {
		t.Fatalf("RemotePath = %q", got)
	}
	if rt.getCalls != 2 {
		t.Fatalf("expected two GetArtifact calls, got %d", rt.getCalls)
	}
}

type queryThenGetRuntime struct {
	artifact   *runtime.Artifact
	getCalls   int
	queryCalls []runtime.ArtifactQuery
	queried    bool
}

func (r *queryThenGetRuntime) GetArtifact(ctx context.Context, key runtime.ArtifactKey) (*runtime.Artifact, error) {
	r.getCalls++
	if !r.queried {
		return nil, runtime.ErrNotFound
	}
	return r.artifact, nil
}

func (r *queryThenGetRuntime) QueryArtifacts(ctx context.Context, query runtime.ArtifactQuery) ([]*runtime.Artifact, error) {
	r.queryCalls = append(r.queryCalls, query)
	r.queried = true
	return []*runtime.Artifact{r.artifact}, nil
}

func (r *queryThenGetRuntime) RenderProjection(ctx context.Context, query runtime.ProjectionQuery) (*runtime.ProjectionResult, error) {
	return nil, runtime.ErrNotFound
}

func (r *queryThenGetRuntime) OpenRemote(context.Context, runtime.RemoteOpenRequest) (*runtime.RemoteResponse, error) {
	return nil, runtime.ErrRemoteUnsupported
}

func (r *queryThenGetRuntime) BeginUpload(ctx context.Context, req runtime.UploadRequest) (runtime.UploadSession, error) {
	return nil, errors.New("not implemented")
}

func (r *queryThenGetRuntime) DeleteArtifact(ctx context.Context, key runtime.ArtifactKey) error {
	return runtime.ErrReadOnly
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

func TestComparePEP440Versions(t *testing.T) {
	tests := []struct {
		name string
		less string
		more string
	}{
		{"release segment", "1.9.0", "1.10.0"},
		{"pre release before final", "1.0.0a1", "1.0.0"},
		{"dev release before final", "1.0.0.dev1", "1.0.0"},
		{"post release after final", "1.0.0", "1.0.0.post1"},
		{"dev release number", "1.0.0.dev1", "1.0.0.dev2"},
		{"epoch release", "999.0", "1!1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if comparePEP440Versions(tt.less, tt.more) >= 0 {
				t.Fatalf("expected %q < %q", tt.less, tt.more)
			}
			if comparePEP440Versions(tt.more, tt.less) <= 0 {
				t.Fatalf("expected %q > %q", tt.more, tt.less)
			}
		})
	}
}

func TestExtractPackageNameFromFilename_HyphenatedName(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	tests := []struct {
		filename, want string
	}{
		{"my-package-1.2.3-py3-none-any.whl", "my-package"},
		{"requests-2.28.0-py3-none-any.whl", "requests"},
		{"Django-4.2.0-py3-none-any.whl", "django"},
		{"my-package-1.2.3-1-py3-none-any.whl", "my-package"},
		{"my-package-1.2.3.tar.gz", "my-package"},
		{"requests-2.28.0.tar.bz2", "requests"},
		{"my_package-1.2.3.zip", "my-package"},
	}
	for _, tt := range tests {
		got := p.extractPackageNameFromFilename(tt.filename)
		if got != tt.want {
			t.Errorf("extractPackageNameFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestExtractVersionFromFilename_HyphenatedName(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	tests := []struct {
		filename, want string
	}{
		{"my-package-1.2.3-py3-none-any.whl", "1.2.3"},
		{"requests-2.28.0-py3-none-any.whl", "2.28.0"},
		{"my-package-1.2.3-1-py3-none-any.whl", "1.2.3"},
		{"my-package-1.2.3.tar.gz", "1.2.3"},
		{"requests-2.28.0.tar.bz2", "2.28.0"},
		{"my_package-1.2.3.zip", "1.2.3"},
	}
	for _, tt := range tests {
		got := p.extractVersionFromFilename(tt.filename)
		if got != tt.want {
			t.Errorf("extractVersionFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestExtractVersionFromFilename(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
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
	p := NewPyPIPlugin(http.DefaultClient)
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

func TestHandle_JsonAPIQueriesSupportedSimpleRemotePath(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "pypi/requests/json", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if len(rt.QueryCalls) != 1 {
		t.Fatalf("expected 1 query call, got %d", len(rt.QueryCalls))
	}
	if got := rt.QueryCalls[0].RemotePath; got != "simple/requests/" {
		t.Fatalf("expected RemotePath simple/requests/, got %q", got)
	}
}

func TestHandle_QueryRemotePath(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "simple/requests/", nil)
	p.Handle(ctx, rt)

	// 首查按 RemotePath=simple/requests/ 精确匹配；空结果触发按 Name 聚合的二次回退查询。
	if len(rt.QueryCalls) != 2 {
		t.Fatalf("expected 2 query calls (path then name fallback), got %d", len(rt.QueryCalls))
	}
	if rt.QueryCalls[0].RemotePath != "simple/requests/" {
		t.Errorf("unexpected first RemotePath: %q", rt.QueryCalls[0].RemotePath)
	}
	if rt.QueryCalls[1].RemotePath != "" || rt.QueryCalls[1].Name != "requests" {
		t.Errorf("unexpected fallback query: %#v", rt.QueryCalls[1])
	}
}

func TestHandle_HostedPackageListFallsBackToLocalPackageFiles(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := newHostedPyPIRuntime(t)

	artifact := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "pypi",
		Kind:         "package-file",
		Name:         "requests",
		Version:      "2.28.0",
		Filename:     "requests-2.28.0.tar.gz",
		RemotePath:   "packages/requests/requests-2.28.0.tar.gz",
		BlobRefs:     []runtime.BlobRef{{Algorithm: "sha256", Digest: "aaaa", Size: 100}},
	})
	sess, err := rt.BeginUpload(context.Background(), runtime.UploadRequest{RepositoryID: "1", Format: "pypi", Filename: artifact.Filename, Size: 100})
	if err != nil {
		t.Fatalf("upload session failed: %v", err)
	}
	if err := sess.PutArtifact(context.Background(), artifact); err != nil {
		t.Fatalf("put artifact: %v", err)
	}
	if err := sess.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "requests-2.28.0.tar.gz") {
		t.Fatalf("expected hosted simple page to include uploaded file, got %s", w.Body.String())
	}
}

func TestHandle_GroupHostedPackageListUsesRuntimeProjectionMapping(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := newHostedPyPIRuntime(t)

	artifact := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: "1",
		Format:       "pypi",
		Kind:         "package-file",
		Name:         "requests",
		Version:      "2.28.0",
		Filename:     "requests-2.28.0.tar.gz",
		RemotePath:   "packages/requests/requests-2.28.0.tar.gz",
		BlobRefs:     []runtime.BlobRef{{Algorithm: "sha256", Digest: "bbbb", Size: 100}},
	})
	sess, err := rt.BeginUpload(context.Background(), runtime.UploadRequest{RepositoryID: "1", Format: "pypi", Filename: artifact.Filename, Size: 100})
	if err != nil {
		t.Fatalf("upload session failed: %v", err)
	}
	if err := sess.PutArtifact(context.Background(), artifact); err != nil {
		t.Fatalf("put artifact: %v", err)
	}
	if err := sess.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}

	group := &runtime.GroupRuntime{Members: []runtime.RepositoryNode{rt}}
	ctx, w := newCtx("GET", "simple/requests/", nil)
	if err := p.Handle(ctx, group); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "requests-2.28.0.tar.gz") {
		t.Fatalf("expected group hosted simple page to include uploaded file, got %s", w.Body.String())
	}
}

func TestHandle_SimpleMetadataQueriesPackageListRemotePath(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "packages/ab/cd/requests-2.28.0.tar.gz",
		Attributes: map[string]string{"metadata": "Metadata-Version: 2.1\nName: requests\n"},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "simple/requests/requests-2.28.0.tar.gz.metadata", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.QueryCalls) == 0 {
		t.Fatalf("expected QueryArtifacts call")
	}
	if got := rt.QueryCalls[0].RemotePath; got != "simple/requests/" {
		t.Fatalf("RemotePath = %q, want simple/requests/", got)
	}
	if got := rt.QueryCalls[0].Filename; got != "requests-2.28.0.tar.gz" {
		t.Fatalf("Filename = %q, want requests-2.28.0.tar.gz", got)
	}
}

func TestFetchRemote_SimpleIndexReturnsMetadataUnsupported(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><a href="requests/">requests</a></body></html>`))
	}))
	defer srv.Close()

	p := NewPyPIPlugin(http.DefaultClient)
	_, err := p.FetchRemote(context.Background(), srv.URL, "simple/")
	if !errors.Is(err, runtime.ErrMetadataUnsupported) {
		t.Fatalf("FetchRemote error = %v, want ErrMetadataUnsupported", err)
	}
	if requestCount != 0 {
		t.Fatalf("upstream requests = %d, want 0", requestCount)
	}
}

type simpleIndexRemoteRuntime struct {
	*testhelper.MockRuntime
	response  *runtime.RemoteResponse
	err       error
	openCalls []runtime.RemoteOpenRequest
}

func (r *simpleIndexRemoteRuntime) OpenRemote(_ context.Context, request runtime.RemoteOpenRequest) (*runtime.RemoteResponse, error) {
	r.openCalls = append(r.openCalls, request)
	return r.response, r.err
}

type streamingOnlyBody struct {
	chunk       []byte
	chunkCount  int
	writeToUsed bool
	closed      bool
}

func (b *streamingOnlyBody) Read([]byte) (int, error) {
	return 0, errors.New("unexpected Read: remote response was buffered instead of streamed")
}

func (b *streamingOnlyBody) WriteTo(w io.Writer) (int64, error) {
	b.writeToUsed = true
	var total int64
	for range b.chunkCount {
		n, err := w.Write(b.chunk)
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (b *streamingOnlyBody) Close() error {
	b.closed = true
	return nil
}

type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}

func assertVaryTokens(t *testing.T, header http.Header, expected ...string) {
	t.Helper()
	counts := make(map[string]int)
	for _, value := range header.Values("Vary") {
		for _, token := range strings.Split(value, ",") {
			token = strings.ToLower(strings.TrimSpace(token))
			if token != "" {
				counts[token]++
			}
		}
	}
	for _, token := range expected {
		if counts[strings.ToLower(token)] != 1 {
			t.Errorf("Vary token %q count = %d, want 1; header=%v", token, counts[strings.ToLower(token)], header.Values("Vary"))
		}
	}
}

func TestHandle_SimpleIndexRemoteGETStreamsResponse(t *testing.T) {
	chunk := bytes.Repeat([]byte("stream-me-"), 4096)
	body := &streamingOnlyBody{chunk: chunk, chunkCount: 64}
	rt := &simpleIndexRemoteRuntime{
		MockRuntime: &testhelper.MockRuntime{},
		response: &runtime.RemoteResponse{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Type":  {"text/html; charset=utf-8"},
				"ETag":          {`"root-v1"`},
				"Cache-Control": {"public, max-age=120"},
				"Vary":          {"Origin, Accept-Encoding"},
			},
			Body: body,
		},
	}
	ctx, w := newCtx(http.MethodGet, "simple/", nil)
	ctx.Request.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")

	if err := NewPyPIPlugin(http.DefaultClient).Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusPartialContent)
	}
	if got := w.Header().Get("ETag"); got != `"root-v1"` {
		t.Errorf("ETag = %q, want %q", got, `"root-v1"`)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=120" {
		t.Errorf("Cache-Control = %q, want public, max-age=120", got)
	}
	assertVaryTokens(t, w.Header(), "Origin", "Accept-Encoding", "Accept")
	if !body.writeToUsed {
		t.Error("remote body WriterTo was not used; response was not streamed with io.Copy")
	}
	if !body.closed {
		t.Error("remote body was not closed")
	}
	if got, want := w.Body.Len(), len(chunk)*body.chunkCount; got != want {
		t.Fatalf("streamed body size = %d, want %d", got, want)
	}
	if len(rt.openCalls) != 1 {
		t.Fatalf("OpenRemote calls = %d, want 1", len(rt.openCalls))
	}
	if got := rt.openCalls[0].Path; got != "simple/" {
		t.Errorf("OpenRemote Path = %q, want simple/", got)
	}
	if got := rt.openCalls[0].Method; got != http.MethodGet {
		t.Errorf("OpenRemote Method = %q, want GET", got)
	}
	if got := rt.openCalls[0].Headers.Get("Accept"); got != "application/vnd.pypi.simple.v1+json" {
		t.Errorf("OpenRemote Accept = %q", got)
	}
	if len(rt.QueryCalls) != 0 {
		t.Errorf("QueryArtifacts calls = %d, want 0", len(rt.QueryCalls))
	}
}

func TestHandle_SimpleIndexCanonicalizesRemotePath(t *testing.T) {
	rt := &simpleIndexRemoteRuntime{
		MockRuntime: &testhelper.MockRuntime{},
		response: &runtime.RemoteResponse{
			StatusCode: http.StatusNoContent,
		},
	}
	ctx, _ := newCtx(http.MethodGet, "simple", nil)

	if err := NewPyPIPlugin(http.DefaultClient).Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if len(rt.openCalls) != 1 {
		t.Fatalf("OpenRemote calls = %d, want 1", len(rt.openCalls))
	}
	if got := rt.openCalls[0].Path; got != "simple/" {
		t.Errorf("OpenRemote Path = %q, want simple/", got)
	}
}

func TestHandle_SimpleIndexRemoteHEADForwardsStatusAndHeadersWithoutBody(t *testing.T) {
	body := &closeTrackingBody{Reader: strings.NewReader("must not be written")}
	rt := &simpleIndexRemoteRuntime{
		MockRuntime: &testhelper.MockRuntime{},
		response: &runtime.RemoteResponse{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Length": {"19"},
				"ETag":           {`"head-v1"`},
				"Cache-Control":  {"private, max-age=30"},
				"Vary":           {"Origin, aCcEpT"},
			},
			Body: body,
		},
	}
	ctx, w := newCtx(http.MethodHead, "simple/", nil)

	if err := NewPyPIPlugin(http.DefaultClient).Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusPartialContent)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("HEAD body = %q, want empty", w.Body.String())
	}
	if got := w.Header().Get("ETag"); got != `"head-v1"` {
		t.Errorf("ETag = %q, want %q", got, `"head-v1"`)
	}
	if got := w.Header().Get("Cache-Control"); got != "private, max-age=30" {
		t.Errorf("Cache-Control = %q, want private, max-age=30", got)
	}
	assertVaryTokens(t, w.Header(), "Origin", "Accept")
	if !body.closed {
		t.Error("remote body was not closed")
	}
	if len(rt.openCalls) != 1 || rt.openCalls[0].Method != http.MethodHead {
		t.Fatalf("OpenRemote calls = %#v, want one HEAD", rt.openCalls)
	}
	if len(rt.QueryCalls) != 0 {
		t.Errorf("QueryArtifacts calls = %d, want 0", len(rt.QueryCalls))
	}
}

func TestHandle_SimpleIndexRemoteForwardsErrorStatuses(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, status := range []int{http.StatusNotFound, http.StatusServiceUnavailable} {
			t.Run(fmt.Sprintf("%s_%d", method, status), func(t *testing.T) {
				body := &closeTrackingBody{Reader: strings.NewReader(http.StatusText(status))}
				rt := &simpleIndexRemoteRuntime{
					MockRuntime: &testhelper.MockRuntime{},
					response: &runtime.RemoteResponse{
						StatusCode: status,
						Header:     http.Header{"Cache-Control": {"no-cache"}},
						Body:       body,
					},
				}
				ctx, w := newCtx(method, "simple/", nil)

				if err := NewPyPIPlugin(http.DefaultClient).Handle(ctx, rt); err != nil {
					t.Fatalf("Handle failed: %v", err)
				}
				if w.Code != status {
					t.Fatalf("status = %d, want %d", w.Code, status)
				}
				if method == http.MethodGet && w.Body.String() != http.StatusText(status) {
					t.Errorf("GET body = %q, want %q", w.Body.String(), http.StatusText(status))
				}
				if method == http.MethodHead && w.Body.Len() != 0 {
					t.Errorf("HEAD body = %q, want empty", w.Body.String())
				}
				if got := w.Header().Get("Cache-Control"); got != "no-cache" {
					t.Errorf("Cache-Control = %q, want no-cache", got)
				}
				assertVaryTokens(t, w.Header(), "Accept")
				if !body.closed {
					t.Error("remote body was not closed")
				}
			})
		}
	}
}

func TestHandle_SimpleIndexRemoteUnsupportedFallsBackToHostedRenderer(t *testing.T) {
	artifacts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "requests", "package": "requests"}, ""),
	}
	rt := &simpleIndexRemoteRuntime{
		MockRuntime: &testhelper.MockRuntime{Artifacts: artifacts},
		err:         runtime.ErrRemoteUnsupported,
	}
	ctx, w := newCtx(http.MethodGet, "simple/", nil)

	if err := NewPyPIPlugin(http.DefaultClient).Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "requests") {
		t.Fatalf("hosted index body = %q, want requests", w.Body.String())
	}
	assertVaryTokens(t, w.Header(), "Accept")
	if len(rt.QueryCalls) == 0 {
		t.Fatal("hosted fallback did not call QueryArtifacts")
	}
}

func TestHandle_SimpleIndexUpstreamUnavailableEscapesToRouter(t *testing.T) {
	rt := &simpleIndexRemoteRuntime{
		MockRuntime: &testhelper.MockRuntime{},
		err:         fmt.Errorf("open root index: %w", runtime.ErrUpstreamUnavailable),
	}
	ctx, w := newCtx(http.MethodGet, "simple/", nil)

	err := NewPyPIPlugin(http.DefaultClient).Handle(ctx, rt)
	if !errors.Is(err, runtime.ErrUpstreamUnavailable) {
		t.Fatalf("Handle error = %v, want ErrUpstreamUnavailable", err)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("body = %q, want router to render the error", w.Body.String())
	}
	if len(rt.QueryCalls) != 0 {
		t.Errorf("QueryArtifacts calls = %d, want 0", len(rt.QueryCalls))
	}
}

func TestHandle_HtmlEscaping(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "<script>alert(1)</script>", "package": "<script>alert(1)</script>"}, ""),
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

func TestHandle_SimpleIndex_NormalizedNames(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "my-package", "package": "my-package"}, ""),
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "my-package", "package": "my-package"}, ""),
		testhelper.NewArtifact("pypi", runtime.KindMetadata, map[string]string{"name": "my-package", "package": "my-package"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "my-package") {
		t.Errorf("expected normalized package name 'my-package' in HTML, got: %s", body)
	}
	count := strings.Count(body, "my-package")
	if count != 2 {
		t.Errorf("expected 2 occurrences of 'my-package' (one in href, one in text), got %d", count)
	}
}

func TestHandle_PackageList_JSON(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("pypi", "package-file", map[string]string{
			"name":     "requests",
			"package":  "requests",
			"version":  "2.28.0",
			"filename": "requests-2.28.0.tar.gz",
		}, ""),
	}
	arts[0].Properties = map[string]string{"remote_path": "ab/cd/requests-2.28.0.tar.gz"}
	// NewArtifact 会经 NormalizeArtifactForStore 去掉 RemotePath 尾部斜杠，
	// 这里覆盖为请求路径（含尾部斜杠），与 handlePackageList 查询的 RemotePath 一致，
	// 模拟回源后 Runtime 层存储的 RemotePath。
	arts[0].RemotePath = "simple/requests/"
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	ctx.Request.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "json") {
		t.Errorf("expected JSON content type, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "requests-2.28.0.tar.gz") {
		t.Errorf("expected filename in JSON response, got: %s", body)
	}
	var data struct {
		Files []struct {
			Hashes map[string]string `json:"hashes"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if len(data.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(data.Files))
	}
	if data.Files[0].Hashes == nil {
		t.Fatalf("expected hashes field to be present for uv compatibility")
	}
}

func TestHandle_PackageList_JSONIncludesSHA256Hash(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/ab/cd",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "simple/requests/",
		Checksums:  map[string]string{"sha256": "abc123"},
		Properties: map[string]string{"remote_path": "packages/ab/cd/requests-2.28.0.tar.gz"},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	ctx.Request.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var data struct {
		Files []struct {
			Hashes map[string]string `json:"hashes"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if len(data.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(data.Files))
	}
	if got := data.Files[0].Hashes["sha256"]; got != "abc123" {
		t.Fatalf("hashes.sha256 = %q, want abc123", got)
	}
}

func TestHandle_PackageList_JSONIncludesProjectAndInstallerMetadata(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/ab/cd",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "simple/requests/",
		Checksums:  map[string]string{"sha256": "abc123"},
		Attributes: map[string]string{
			"requires_python": ">=3.8",
			"yanked":          "true",
			"yanked_reason":   "bad release",
		},
		Properties: map[string]string{"remote_path": "packages/ab/cd/requests-2.28.0.tar.gz"},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	ctx.Request.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var data struct {
		Name  string `json:"name"`
		Files []struct {
			RequiresPython string `json:"requires-python"`
			Yanked         string `json:"yanked"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if data.Name != "requests" {
		t.Fatalf("name = %q, want requests", data.Name)
	}
	if len(data.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(data.Files))
	}
	if got := data.Files[0].RequiresPython; got != ">=3.8" {
		t.Fatalf("requires-python = %q, want >=3.8", got)
	}
	if got := data.Files[0].Yanked; got != "bad release" {
		t.Fatalf("yanked = %q, want bad release", got)
	}
}

func TestHandle_PackageList_HTMLIncludesInstallerMetadata(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/ab/cd",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "simple/requests/",
		Attributes: map[string]string{
			"requires_python": ">=3.8",
			"yanked":          "true",
			"yanked_reason":   "bad release",
		},
		Properties: map[string]string{"remote_path": "packages/ab/cd/requests-2.28.0.tar.gz"},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, `data-requires-python="&gt;=3.8"`) {
		t.Fatalf("expected data-requires-python attribute, got %s", body)
	}
	if !strings.Contains(body, `data-yanked="bad release"`) {
		t.Fatalf("expected data-yanked attribute, got %s", body)
	}
}

func TestHandle_PackageListUsesStoredChecksumWithoutBlobRef(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/ab/cd",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "simple/requests/", // 与 handlePackageList 查询的 RemotePath 一致，模拟回源后 Runtime 层存储的 RemotePath
		Checksums:  map[string]string{"sha256": "abc123"},
		Properties: map[string]string{"remote_path": "packages/ab/cd/requests-2.28.0.tar.gz"},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "#sha256=abc123") {
		t.Fatalf("expected stored checksum in package link, got: %s", w.Body.String())
	}
}

func TestHandle_PackageListJSONIncludesSimpleAPI11Metadata(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/ab/cd",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "simple/requests/",
		SizeBytes:  12345,
		Checksums:  map[string]string{"sha256": "abc123"},
		Attributes: map[string]string{
			"published_at":    "2024-01-02T03:04:05Z",
			"requires_python": ">=3.8",
			"metadata":        "Metadata-Version: 2.1",
		},
		Properties: map[string]string{"remote_path": "packages/ab/cd/requests-2.28.0.tar.gz"},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	ctx.Request.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var data struct {
		Meta     map[string]string `json:"meta"`
		Versions []string          `json:"versions"`
		Files    []struct {
			Size         int64             `json:"size"`
			UploadTime   string            `json:"upload-time"`
			CoreMetadata map[string]string `json:"core-metadata"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if got := data.Meta["api-version"]; got != "1.1" {
		t.Fatalf("api-version = %q, want 1.1", got)
	}
	if len(data.Versions) != 1 || data.Versions[0] != "2.28.0" {
		t.Fatalf("versions = %#v, want [2.28.0]", data.Versions)
	}
	if len(data.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(data.Files))
	}
	if got := data.Files[0].Size; got != 12345 {
		t.Fatalf("size = %d, want 12345", got)
	}
	if got := data.Files[0].UploadTime; got != "2024-01-02T03:04:05Z" {
		t.Fatalf("upload-time = %q, want 2024-01-02T03:04:05Z", got)
	}
	if got := data.Files[0].CoreMetadata["sha256"]; got != "abc123" {
		t.Fatalf("core-metadata.sha256 = %q, want abc123", got)
	}
}

func TestHandle_PackageMetadataServesStandardFileURLMetadata(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/ab/cd",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "packages/ab/cd/requests-2.28.0.tar.gz",
		Attributes: map[string]string{"metadata": "Metadata-Version: 2.1\nName: requests\n"},
		Properties: map[string]string{"remote_path": "packages/ab/cd/requests-2.28.0.tar.gz"},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "packages/ab/cd/requests-2.28.0.tar.gz.metadata", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, "Name: requests") {
		t.Fatalf("unexpected metadata body: %q", body)
	}
	if len(rt.QueryCalls) != 1 || rt.QueryCalls[0].RemotePath != "packages/ab/cd/requests-2.28.0.tar.gz" {
		t.Fatalf("expected query by file RemotePath, got %#v", rt.QueryCalls)
	}
}

func TestHandle_LegacyUploadStoresTwineMultipartPackage(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField(":action", "file_upload")
	_ = writer.WriteField("name", "Requests")
	_ = writer.WriteField("version", "2.28.0")
	_ = writer.WriteField("filetype", "sdist")
	_ = writer.WriteField("pyversion", "source")
	content := []byte("package-content")
	sum := sha256.Sum256(content)
	digest := fmt.Sprintf("%x", sum[:])
	_ = writer.WriteField("sha256_digest", digest)
	part, err := writer.CreateFormFile("content", "requests-2.28.0.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(content)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, w := newCtx("POST", "legacy/", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Request.ContentLength = int64(body.Len())
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.UploadCalls) != 1 {
		t.Fatalf("expected one upload session, got %d", len(rt.UploadCalls))
	}
	if len(rt.UploadedArts) != 1 {
		t.Fatalf("expected one uploaded artifact, got %d", len(rt.UploadedArts))
	}
	art := rt.UploadedArts[0]
	if art.Name != "requests" || art.Version != "2.28.0" || art.Filename != "requests-2.28.0.tar.gz" {
		t.Fatalf("unexpected artifact coordinates: name=%q version=%q filename=%q", art.Name, art.Version, art.Filename)
	}
	if art.Kind != runtime.KindArtifact {
		t.Fatalf("artifact kind = %q, want %q", art.Kind, runtime.KindArtifact)
	}
	if !strings.HasPrefix(art.RemotePath, "packages/") {
		t.Fatalf("RemotePath = %q, want packages/...", art.RemotePath)
	}
	if got := art.Checksums["sha256"]; got != digest {
		t.Fatalf("sha256 = %q, want %s", got, digest)
	}
	if got := art.Attributes["filetype"]; got != "sdist" {
		t.Fatalf("filetype = %q, want sdist", got)
	}
}

func TestHandle_LegacyUploadRejectsMismatchedSHA256Digest(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField(":action", "file_upload")
	_ = writer.WriteField("name", "requests")
	_ = writer.WriteField("version", "2.28.0")
	_ = writer.WriteField("sha256_digest", "deadbeef")
	part, err := writer.CreateFormFile("content", "requests-2.28.0.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("package-content"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, w := newCtx("POST", "legacy/", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Request.ContentLength = int64(body.Len())
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", w.Code, w.Body.String())
	}
	if len(rt.UploadCalls) != 0 || len(rt.UploadedArts) != 0 {
		t.Fatalf("expected no upload on digest mismatch, calls=%d artifacts=%d", len(rt.UploadCalls), len(rt.UploadedArts))
	}
}

func TestHandle_PackageList_NotFound(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{QueryErr: runtime.ErrNotFound}

	ctx, w := newCtx("GET", "simple/nonexistent-package/", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent package, got %d", w.Code)
	}
}

func TestHandle_PackageDownload_NotFound(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{GetErr: runtime.ErrNotFound}

	ctx, w := newCtx("GET", "packages/ab/cd/nonexistent-1.0.tar.gz", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent file, got %d", w.Code)
	}
}

func TestHandle_ChecksumUsesStoredChecksumWithoutBlobRef(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := runtime.NewArtifact(runtime.ArtifactSpec{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/ab/cd",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "packages/ab/cd/requests-2.28.0.tar.gz",
		Checksums:  map[string]string{"sha256": "abc123"},
	})
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "packages/ab/cd/requests-2.28.0.tar.gz.sha256", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "abc123\n" {
		t.Fatalf("checksum body = %q, want abc123 newline", got)
	}
}

func TestHandle_ChecksumPrefersExactRemotePath(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	wrongSameFilename := runtime.NewArtifact(runtime.ArtifactSpec{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/aa/bb",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "packages/aa/bb/requests-2.28.0.tar.gz",
		Checksums:  map[string]string{"sha256": "wrong"},
	})
	exactPath := runtime.NewArtifact(runtime.ArtifactSpec{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/ab/cd",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "packages/ab/cd/requests-2.28.0.tar.gz",
		Checksums:  map[string]string{"sha256": "right"},
	})
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{wrongSameFilename, exactPath}}

	ctx, w := newCtx("GET", "packages/ab/cd/requests-2.28.0.tar.gz.sha256", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "right\n" {
		t.Fatalf("checksum body = %q, want exact path digest", got)
	}
}

func TestHandle_PackageDownload_MissingContent(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("pypi", "package", map[string]string{
		"name":     "requests",
		"package":  "requests",
		"version":  "2.28.0",
		"filename": "requests-2.28.0.tar.gz",
		"path":     "packages/ab/cd",
	}, "")
	art.Content = nil
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "packages/ab/cd/requests-2.28.0.tar.gz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing content, got %d", w.Code)
	}
}

func TestHandle_JsonAPI_NotFound(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{}}

	ctx, w := newCtx("GET", "pypi/nonexistent-package/json", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Errorf("JSON API returns 200 with empty data, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "nonexistent-package") {
		t.Errorf("expected package name in response, got: %s", body)
	}
}

func TestHandle_JsonAPI_WithVersion(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art1 := testhelper.NewArtifact("pypi", "package-file", map[string]string{
		"package":  "requests",
		"version":  "2.28.0",
		"filename": "requests-2.28.0.tar.gz",
	}, "")
	art1.Properties = map[string]string{"remote_path": "ab/cd/requests-2.28.0.tar.gz"}

	art2 := testhelper.NewArtifact("pypi", "package-file", map[string]string{
		"package":  "requests",
		"version":  "2.31.0",
		"filename": "requests-2.31.0.tar.gz",
	}, "")
	art2.Properties = map[string]string{"remote_path": "ab/cd/requests-2.31.0.tar.gz"}

	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art1, art2}}

	ctx, w := newCtx("GET", "pypi/requests/2.28.0/json", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "2.31.0") {
		t.Errorf("expected only version 2.28.0 in response, but found 2.31.0: %s", body)
	}
	if !strings.Contains(body, "2.28.0") {
		t.Errorf("expected version 2.28.0 in response, got: %s", body)
	}
}

func TestHandle_JsonAPIUsesDigestsField(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Version:    "2.28.0",
		Path:       "packages/ab/cd",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "simple/requests/",
		Checksums:  map[string]string{"sha256": "abc123"},
		Properties: map[string]string{"remote_path": "packages/ab/cd/requests-2.28.0.tar.gz"},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "pypi/requests/json", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var data struct {
		Releases map[string][]struct {
			Digests map[string]string `json:"digests"`
		} `json:"releases"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if got := data.Releases["2.28.0"][0].Digests["sha256"]; got != "abc123" {
		t.Fatalf("digests.sha256 = %q, want abc123", got)
	}
}

func TestHandle_InvalidPath(t *testing.T) {
	tests := []struct {
		path       string
		wantStatus int
	}{
		{"../etc/passwd", http.StatusBadRequest},
		{"simple/$inject", http.StatusBadRequest},
		{"packages/../../etc/passwd", http.StatusBadRequest},
		{"simple/;rm-rf-/", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			p := NewPyPIPlugin(http.DefaultClient)
			rt := &testhelper.MockRuntime{}

			ctx, w := newCtx("GET", tt.path, nil)
			p.Handle(ctx, rt)

			if w.Code != tt.wantStatus {
				t.Errorf("path %q: expected status %d, got %d", tt.path, tt.wantStatus, w.Code)
			}
		})
	}
}

func TestFetchRemote_PackageInfo(t *testing.T) {
	mockJSONAPIResponse := `{
		"info": {
			"name": "requests",
			"version": "2.28.0",
			"summary": "Python HTTP for Humans.",
			"license": "Apache 2.0",
			"home_page": "https://requests.readthedocs.io"
		},
		"releases": {
			"2.28.0": [
				{
					"filename": "requests-2.28.0-py3-none-any.whl",
					"url": "https://files.pythonhosted.org/packages/requests-2.28.0-py3-none-any.whl",
					"digests": {"sha256": "abc123"},
					"size": 12345,
					"upload_time": "2022-06-01T12:00:00"
				},
				{
					"filename": "requests-2.28.0.tar.gz",
					"url": "https://files.pythonhosted.org/packages/requests-2.28.0.tar.gz",
					"digests": {"sha256": "def456"},
					"size": 23456,
					"upload_time": "2022-06-01T12:00:00"
				}
			],
			"2.31.0": [
				{
					"filename": "requests-2.31.0-py3-none-any.whl",
					"url": "https://files.pythonhosted.org/packages/requests-2.31.0-py3-none-any.whl",
					"digests": {"sha256": "ghi789"},
					"size": 34567,
					"upload_time": "2023-05-01T12:00:00"
				}
			]
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pypi/requests/json" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(mockJSONAPIResponse))
			return
		}
		if r.URL.Path == "/simple/requests/" {
			w.Write([]byte(`<html><body>
<a href="../../packages/ab/cd/requests-2.28.0.tar.gz">requests-2.28.0.tar.gz</a><br>
</body></html>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewPyPIPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "simple/requests/")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) == 0 {
		t.Fatal("expected at least one artifact from JSON API parsing")
	}

	found280 := false
	found2310 := false
	for _, a := range arts {
		if a.Version == "2.28.0" {
			found280 = true
			if a.Attributes["license"] != "Apache 2.0" {
				t.Errorf("expected license 'Apache 2.0', got %q", a.Attributes["license"])
			}
			if a.Attributes["description"] != "Python HTTP for Humans." {
				t.Errorf("expected description 'Python HTTP for Humans.', got %q", a.Attributes["description"])
			}
			if a.Attributes["homepage"] != "https://requests.readthedocs.io" {
				t.Errorf("expected homepage 'https://requests.readthedocs.io', got %q", a.Attributes["homepage"])
			}
		}
		if a.Version == "2.31.0" {
			found2310 = true
		}
	}
	if !found280 {
		t.Error("expected to find artifacts for version 2.28.0")
	}
	if !found2310 {
		t.Error("expected to find artifacts for version 2.31.0")
	}
}

func TestBuildArtifactsFromJSONAPIPrefersUsableLicense(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := p.buildArtifactsFromJSONAPI("demo", map[string]interface{}{
		"info": map[string]interface{}{
			"license":            "UNKNOWN",
			"license_expression": "MIT",
			"summary":            "Demo package",
			"classifiers": []interface{}{
				"License :: OSI Approved :: Apache Software License",
			},
		},
		"releases": map[string]interface{}{
			"1.0.0": []interface{}{
				map[string]interface{}{
					"filename":    "demo-1.0.0.tar.gz",
					"url":         "https://files.pythonhosted.org/packages/ab/cd/demo-1.0.0.tar.gz",
					"upload_time": "2024-01-01T00:00:00",
				},
			},
		},
	})
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if got := arts[0].Attributes["license"]; got != "MIT" {
		t.Fatalf("license = %q, want MIT", got)
	}
	if _, ok := arts[0].Attributes["remote_path"]; ok {
		t.Fatalf("remote_path should not be stored in attributes: %#v", arts[0].Attributes)
	}
	if got := arts[0].Properties["remote_path"]; got == "" {
		t.Fatalf("remote_path should stay in properties")
	}
}

func TestBuildArtifactsFromJSONAPIPreservesDigestAndSize(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := p.buildArtifactsFromJSONAPI("demo", map[string]interface{}{
		"info": map[string]interface{}{"summary": "Demo package"},
		"releases": map[string]interface{}{
			"1.0.0": []interface{}{
				map[string]interface{}{
					"filename": "demo-1.0.0.tar.gz",
					"url":      "https://files.pythonhosted.org/packages/ab/cd/demo-1.0.0.tar.gz",
					"digests":  map[string]interface{}{"sha256": "abc123"},
					"size":     float64(12345),
				},
			},
		},
	})
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if got := arts[0].Checksums["sha256"]; got != "abc123" {
		t.Fatalf("sha256 = %q, want abc123", got)
	}
	if got := arts[0].SizeBytes; got != 12345 {
		t.Fatalf("SizeBytes = %d, want 12345", got)
	}
}

func TestBuildArtifactsFromJSONAPIPreservesInstallerMetadata(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts := p.buildArtifactsFromJSONAPI("demo", map[string]interface{}{
		"info": map[string]interface{}{"summary": "Demo package"},
		"releases": map[string]interface{}{
			"1.0.0": []interface{}{
				map[string]interface{}{
					"filename":        "demo-1.0.0.tar.gz",
					"url":             "https://files.pythonhosted.org/packages/ab/cd/demo-1.0.0.tar.gz",
					"requires_python": ">=3.8",
					"yanked":          true,
					"yanked_reason":   "bad release",
				},
			},
		},
	})
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if got := arts[0].Attributes["requires_python"]; got != ">=3.8" {
		t.Fatalf("requires_python = %q, want >=3.8", got)
	}
	if got := arts[0].Attributes["yanked"]; got != "true" {
		t.Fatalf("yanked = %q, want true", got)
	}
	if got := arts[0].Attributes["yanked_reason"]; got != "bad release" {
		t.Fatalf("yanked_reason = %q, want bad release", got)
	}
}

func TestFetchRemote_FallbackToSimpleIndex(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == "/pypi/mypackage/json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/simple/mypackage/" {
			w.Write([]byte(`<html><body>
<a href="../../packages/ab/cd/mypackage-1.0.tar.gz">mypackage-1.0.tar.gz</a><br>
<a href="../../packages/ab/cd/mypackage-1.0-py3-none-any.whl">mypackage-1.0-py3-none-any.whl</a><br>
</body></html>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewPyPIPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "simple/mypackage/")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) == 0 {
		t.Fatal("expected artifacts from fallback to simple index")
	}

	var foundTarGz, foundWhl bool
	for _, a := range arts {
		fn := a.Filename
		if fn == "mypackage-1.0.tar.gz" {
			foundTarGz = true
		}
		if fn == "mypackage-1.0-py3-none-any.whl" {
			foundWhl = true
		}
	}
	if !foundTarGz {
		t.Error("expected to find mypackage-1.0.tar.gz from simple index fallback")
	}
	if !foundWhl {
		t.Error("expected to find mypackage-1.0-py3-none-any.whl from simple index fallback")
	}
}

func TestParsePackageListDoesNotStoreRelativeDownloadURL(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts, err := p.parsePackageList("requests", strings.NewReader(`
<html><body>
<a href="../../packages/ab/cd/requests-2.28.0.tar.gz">requests-2.28.0.tar.gz</a><br>
</body></html>`))
	if err != nil {
		t.Fatalf("parsePackageList failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if got := arts[0].Properties["remote_path"]; got != "packages/ab/cd/requests-2.28.0.tar.gz" {
		t.Fatalf("remote_path = %q", got)
	}
	if got := arts[0].Properties["download_url"]; got != "" {
		t.Fatalf("relative link should not be stored as download_url, got %q", got)
	}
}

func TestParsePackageListExtractsSHA256Fragment(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts, err := p.parsePackageList("requests", strings.NewReader(`
<html><body>
<a href="../../packages/ab/cd/requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a><br>
</body></html>`))
	if err != nil {
		t.Fatalf("parsePackageList failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if got := arts[0].Checksums["sha256"]; got != "abc123" {
		t.Fatalf("sha256 = %q, want abc123", got)
	}
}

func TestParsePackageListExtractsInstallerMetadataAttributes(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts, err := p.parsePackageList("requests", strings.NewReader(`
<html><body>
<a href="../../packages/ab/cd/requests-2.28.0.tar.gz#sha256=abc123" data-requires-python="&gt;=3.8" data-yanked="bad release">requests-2.28.0.tar.gz</a><br>
</body></html>`))
	if err != nil {
		t.Fatalf("parsePackageList failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if got := arts[0].Attributes["requires_python"]; got != ">=3.8" {
		t.Fatalf("requires_python = %q, want >=3.8", got)
	}
	if got := arts[0].Attributes["yanked"]; got != "true" {
		t.Fatalf("yanked = %q, want true", got)
	}
	if got := arts[0].Attributes["yanked_reason"]; got != "bad release" {
		t.Fatalf("yanked_reason = %q, want bad release", got)
	}
}

func TestParseSimpleIndexHandlesSingleQuotedLinksAndAttributeOrder(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts, err := p.parseSimpleIndex(strings.NewReader(`
<html><body>
<a data-extra="1" href='Requests/'>Requests</a>
<a href="flask/">Flask</a>
</body></html>`))
	if err != nil {
		t.Fatalf("parseSimpleIndex failed: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(arts))
	}
	if arts[0].Name != "requests" || arts[1].Name != "flask" {
		t.Fatalf("unexpected package names: %q, %q", arts[0].Name, arts[1].Name)
	}
}

func TestParsePackageListHandlesArbitraryRelativeLinksAndEmptyYanked(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	arts, err := p.parsePackageList("requests", strings.NewReader(`
<html><body>
<a data-yanked="" data-requires-python="&gt;=3.8" href='../files/requests-2.28.0.tar.gz#sha256=abc123'>requests-2.28.0.tar.gz</a>
</body></html>`))
	if err != nil {
		t.Fatalf("parsePackageList failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if got := arts[0].RemotePath; got != "files/requests-2.28.0.tar.gz" {
		t.Fatalf("RemotePath = %q, want files/requests-2.28.0.tar.gz", got)
	}
	if got := arts[0].Checksums["sha256"]; got != "abc123" {
		t.Fatalf("sha256 = %q, want abc123", got)
	}
	if got := arts[0].Attributes["requires_python"]; got != ">=3.8" {
		t.Fatalf("requires_python = %q, want >=3.8", got)
	}
	if got := arts[0].Attributes["yanked"]; got != "true" {
		t.Fatalf("yanked = %q, want true", got)
	}
	if got := arts[0].Attributes["yanked_reason"]; got != "" {
		t.Fatalf("yanked_reason = %q, want empty reason", got)
	}
	if got := arts[0].Properties["download_url"]; got != "" {
		t.Fatalf("relative link should not be stored as download_url, got %q", got)
	}
}

func TestFetchRemote_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewPyPIPlugin(http.DefaultClient)
	_, err := p.FetchRemote(context.Background(), srv.URL, "simple")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestExtractPackageFromFilename(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	tests := []struct {
		filename, want string
	}{
		{"requests-2.28.0.tar.gz", "requests"},
		{"requests-2.28.0-py3-none-any.whl", "requests"},
		{"Flask-2.3.2.zip", "flask"},
		{"My_Package-1.0-cp39-cp39-linux_x86_64.whl", "my-package"},
		{"Django-4.2.1.tar.bz2", "django"},
		{"numpy-1.24.0-cp311-cp311-manylinux_2_17_x86_64.manylinux2014_x86_64.whl", "numpy"},
	}
	for _, tt := range tests {
		got := p.extractPackageNameFromFilename(tt.filename)
		if got != tt.want {
			t.Errorf("extractPackageNameFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ErrBlocked 透传：runtime 返回 ErrBlocked 时，plugin.Handle 必须 return ErrBlocked，
// 让 router 统一处理 403 响应 + 审计日志。不能被插件吞成 500/404 后 return nil。
// ---------------------------------------------------------------------------

func TestHandle_PackagesDownloadGetBlockedPropagatesErrBlocked(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{GetErr: runtime.ErrBlocked}

	ctx, _ := newCtx("GET", "packages/62/35/lodash-4.17.21-py3-none-any.whl", nil)
	err := p.Handle(ctx, rt)
	if !errors.Is(err, runtime.ErrBlocked) {
		t.Fatalf("Handle err = %v, want ErrBlocked (must propagate to router for audit log)", err)
	}
}

func TestHandle_SimpleIndexQueryBlockedPropagatesErrBlocked(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{QueryErr: runtime.ErrBlocked}

	ctx, _ := newCtx("GET", "simple/lodash/", nil)
	err := p.Handle(ctx, rt)
	if !errors.Is(err, runtime.ErrBlocked) {
		t.Fatalf("Handle err = %v, want ErrBlocked (must propagate to router for audit log)", err)
	}
}

func TestExtractVersionFromFilename_EdgeCases(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	tests := []struct {
		filename, want string
	}{
		{"requests-2.28.0-py3-none-any.whl", "2.28.0"},
		{"numpy-1.24.0-cp311-cp311-manylinux_2_17_x86_64.whl", "1.24.0"},
		{"package-1.0.0.post1-py3-none-any.whl", "1.0.0.post1"},
		{"package-1.0.0a1-py3-none-any.whl", "1.0.0a1"},
		{"package-1.0.0b2.dev1-py3-none-any.whl", "1.0.0b2.dev1"},
		{"tensorflow-2.13.0-cp311-cp311-manylinux_2_17_x86_64.manylinux2014_x86_64.whl", "2.13.0"},
		{"MyPackage-2.0.1+local.version-py3-none-any.whl", "2.0.1+local.version"},
		{"scipy-1.11.1-cp311-cp311-macosx_11_0_arm64.whl", "1.11.1"},
		{"pandas-2.0.3-pp310-pypy310_pp73-macosx_11_0_arm64.whl", "2.0.3"},
	}
	for _, tt := range tests {
		got := p.extractVersionFromFilename(tt.filename)
		if got != tt.want {
			t.Errorf("extractVersionFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}
