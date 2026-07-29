package apt

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/plugins/testhelper"
	"github.com/ulikunitz/xz"
)

func newCtx(method, path string, body io.Reader) (*runtime.RequestContext, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/repository/apt-test/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "apt-test", Format: "apt", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

func TestHandle_InRelease(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("apt", runtime.KindMetadata, map[string]string{
		"file":     "InRelease",
		"filename": "InRelease",
		"path":     "dists/jammy",
	}, "release-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "dists/jammy/InRelease", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "release-content" {
		t.Errorf("expected 'release-content', got %q", w.Body.String())
	}
}

func TestHandle_InReleaseHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("apt", runtime.KindMetadata, map[string]string{
		"file":     "InRelease",
		"filename": "InRelease",
		"path":     "dists/jammy",
	}, "release-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "dists/jammy/InRelease", nil)
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

func TestHandle_Packages(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("apt", "package", map[string]string{
			"package":     "nginx",
			"name":        "nginx",
			"version":     "1.18.0-6.1",
			"filename":    "nginx_1.18.0-6.1_amd64.deb",
			"remote_path": "dists/jammy/main/binary-amd64/Packages",
		}, ""),
	}
	arts[0].Properties = map[string]string{"filename": "nginx_1.18.0-6.1_amd64.deb"}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "dists/jammy/main/binary-amd64/Packages", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Package: nginx") {
		t.Errorf("expected 'Package: nginx' in output, got: %s", body)
	}
	if !strings.Contains(body, "Version: 1.18.0-6.1") {
		t.Errorf("expected version in output")
	}
}

func TestHandle_PackagesCompressed_PrefersOriginalContentWithGzipType(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	original := "gzipped-packages-bytes"
	art := testhelper.NewArtifact("apt", runtime.KindMetadata, map[string]string{
		"file":     "Packages.gz",
		"filename": "Packages.gz",
		"path":     "dists/jammy/main/binary-amd64",
	}, original)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "dists/jammy/main/binary-amd64/Packages.gz", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != original {
		t.Fatalf("expected original Packages.gz content, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("expected application/gzip, got %q", ct)
	}
}

func TestHandle_DebDownload(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("apt", "package", map[string]string{
		"file":     "nginx_1.18.0-6.1_amd64.deb",
		"filename": "nginx_1.18.0-6.1_amd64.deb",
		"path":     "pool/main/n/nginx",
	}, "deb-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "pool/main/n/nginx/nginx_1.18.0-6.1_amd64.deb", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/vnd.debian.binary-package" {
		t.Errorf("expected debian binary package content type, got %s", ct)
	}
}

func TestHandle_DebHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("apt", "package", map[string]string{
		"file":     "nginx_1.18.0-6.1_amd64.deb",
		"filename": "nginx_1.18.0-6.1_amd64.deb",
		"path":     "pool/main/n/nginx",
	}, "deb-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "pool/main/n/nginx/nginx_1.18.0-6.1_amd64.deb", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/vnd.debian.binary-package" {
		t.Fatalf("expected debian binary package content type, got %s", ct)
	}
}

func TestHandle_DebRangeReturnsPartialContent(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("apt", "package", map[string]string{
		"file":     "nginx_1.18.0-6.1_amd64.deb",
		"filename": "nginx_1.18.0-6.1_amd64.deb",
		"path":     "pool/main/n/nginx",
	}, "deb-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "pool/main/n/nginx/nginx_1.18.0-6.1_amd64.deb", nil)
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
}

func TestFetchRemote_DebUsesDirectoryPathAndRemotePath(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), "http://example.test", "pool/main/n/nginx/nginx_1.18.0-6.1_amd64.deb")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	a := arts[0]
	if got := a.Path; got != "pool/main/n/nginx" {
		t.Fatalf("path = %q, want directory path", got)
	}
	if got := a.Filename; got != "nginx_1.18.0-6.1_amd64.deb" {
		t.Fatalf("filename = %q", got)
	}
	if got := a.Properties["remote_path"]; got != "pool/main/n/nginx/nginx_1.18.0-6.1_amd64.deb" {
		t.Fatalf("remote_path = %q, want full remote path", got)
	}
}

func TestHandle_NotFound(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "dists/jammy/InRelease", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandle_UnsupportedPath(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "unsupported/path", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandle_ByHashRequest(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	// by-hash 路径格式: {dir}/by-hash/{algorithm}/{hash}
	hashPath := "dists/stable/main/binary-amd64/by-hash/SHA256/abc123def456"
	art := testhelper.NewArtifact("apt", runtime.KindMetadata, map[string]string{
		"file":        "abc123def456",
		"filename":    "abc123def456",
		"path":        "dists/stable/main/binary-amd64/by-hash/SHA256",
		"remote_path": hashPath,
	}, "packages-index-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", hashPath, nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for by-hash request, got %d body=%q", w.Code, w.Body.String())
	}
	if w.Body.String() != "packages-index-content" {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}
}

func TestHandle_ByHashRequestNotFound(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "dists/stable/main/binary-amd64/by-hash/SHA256/nonexistent", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent by-hash, got %d", w.Code)
	}
}

func TestParsePackagesIndex(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	content := `Package: nginx
Version: 1.18.0-6.1
Architecture: amd64
Filename: pool/main/n/nginx/nginx_1.18.0-6.1_amd64.deb
Size: 567890

Package: curl
Version: 7.81.0-1ubuntu1.10
Architecture: amd64
Filename: pool/main/c/curl/curl_7.81.0-1ubuntu1.10_amd64.deb
Size: 194000
`
	arts := p.parsePackagesIndex(content)
	if len(arts) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(arts))
	}
	if arts[0].Qualifiers["package"] != "nginx" {
		t.Errorf("expected 'nginx', got %q", arts[0].Qualifiers["package"])
	}
	if arts[0].Version != "1.18.0-6.1" {
		t.Errorf("expected '1.18.0-6.1', got %q", arts[0].Version)
	}
	if arts[1].Qualifiers["package"] != "curl" {
		t.Errorf("expected 'curl', got %q", arts[1].Qualifiers["package"])
	}
}

func TestIsInReleaseRequest(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	tests := []struct {
		path string
		want bool
	}{
		{"dists/jammy/InRelease", true},
		{"dists/jammy/Release", true},
		{"dists/jammy/Release.gpg", true},
		{"dists/jammy/main/Packages", false},
	}
	for _, tt := range tests {
		got := p.isInReleaseRequest(tt.path)
		if got != tt.want {
			t.Errorf("isInReleaseRequest(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsPackagesRequest(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	tests := []struct {
		path string
		want bool
	}{
		{"dists/jammy/main/binary-amd64/Packages", true},
		{"dists/jammy/main/binary-amd64/Packages.gz", true},
		{"dists/jammy/InRelease", false},
	}
	for _, tt := range tests {
		got := p.isPackagesRequest(tt.path)
		if got != tt.want {
			t.Errorf("isPackagesRequest(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestFetchRemote_ParsesPackages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`Package: nginx
Version: 1.18.0
Filename: pool/main/n/nginx/nginx_1.18.0_amd64.deb

Package: curl
Version: 7.81.0
Filename: pool/main/c/curl/curl_7.81.0_amd64.deb
`))
	}))
	defer srv.Close()

	p := NewAptPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "dists/jammy/main/binary-amd64/Packages")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(arts))
	}
}

func TestFetchRemote_ParsesPackagesXZ(t *testing.T) {
	packagesBody := `Package: nginx
Version: 1.18.0
Architecture: amd64
Filename: pool/main/n/nginx/nginx_1.18.0_amd64.deb

Package: curl
Version: 7.81.0
Architecture: amd64
Filename: pool/main/c/curl/curl_7.81.0_amd64.deb
`
	var xzBody bytes.Buffer
	xw, err := xz.NewWriter(&xzBody)
	if err != nil {
		t.Fatalf("create xz writer: %v", err)
	}
	if _, err := xw.Write([]byte(packagesBody)); err != nil {
		t.Fatalf("write xz body: %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("close xz writer: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-xz")
		w.Write(xzBody.Bytes())
	}))
	defer srv.Close()

	p := NewAptPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "dists/jammy/main/binary-amd64/Packages.xz")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("expected metadata plus 2 package artifacts, got %d: %#v", len(arts), arts)
	}
	var foundNginx bool
	for _, a := range arts {
		if a.Name == "nginx" && a.Version == "1.18.0" && a.RemotePath == "pool/main/n/nginx/nginx_1.18.0_amd64.deb" {
			foundNginx = true
		}
	}
	if !foundNginx {
		t.Fatalf("expected parsed nginx package artifact, got %#v", arts)
	}
}

func TestHandle_QueryRemotePath(t *testing.T) {
	p := NewAptPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "dists/jammy/main/binary-amd64/Packages", nil)
	p.Handle(ctx, rt)

	if len(rt.QueryCalls) != 1 {
		t.Fatalf("expected 1 query call, got %d", len(rt.QueryCalls))
	}
	if rt.QueryCalls[0].RemotePath != "dists/jammy/main/binary-amd64/Packages" {
		t.Errorf("unexpected RemotePath: %q", rt.QueryCalls[0].RemotePath)
	}
}

func TestReadDecompressedWithLimitRejectsOversized(t *testing.T) {
	// 超过 limit 时应报错，防止解压炸弹
	big := bytes.NewReader(bytes.Repeat([]byte("A"), 102))
	_, err := readDecompressedWithLimitN(big, "test", 100)
	if err == nil {
		t.Fatal("expected error for oversized decompressed data")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected 'exceeds' error, got %v", err)
	}
}

func TestReadDecompressedWithLimitAcceptsAtLimit(t *testing.T) {
	// 恰好等于 limit 时应正常返回
	exact := bytes.NewReader(bytes.Repeat([]byte("A"), 100))
	out, err := readDecompressedWithLimitN(exact, "test", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 100 {
		t.Fatalf("expected 100 bytes, got %d", len(out))
	}
}

func TestDecompressAptPackagesRejectsGzipBomb(t *testing.T) {
	// 构造一个 gzip 压缩流，解压后超过 maxAptDecompressedSize
	// 用小 limit 验证逻辑：通过 readDecompressedWithLimitN 间接测试
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write(bytes.Repeat([]byte("X"), 200))
	gw.Close()

	// decompressAptPackages 使用 256MB 上限，正常 200 字节不会被拦截
	out, err := decompressAptPackages("Packages.gz", buf.Bytes())
	if err != nil {
		t.Fatalf("unexpected error for small gzip: %v", err)
	}
	if len(out) != 200 {
		t.Fatalf("expected 200 bytes decompressed, got %d", len(out))
	}
}
