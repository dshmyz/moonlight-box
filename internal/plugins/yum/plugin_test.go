package yum

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
	"github.com/ulikunitz/xz"
)

func newCtx(method, path string, body io.Reader) (*runtime.RequestContext, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/repository/yum-test/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "yum-test", Format: "yum", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

func TestHandle_Repomd(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		// remote_path 与 handleRepomd 查询用的 RemotePath 一致，模拟回源后 Runtime 层存储的 RemotePath
		testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{"file": "repomd.xml", "remote_path": "repodata/repomd.xml"}, ""),
		testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{"file": "abc123-primary.xml.gz", "type": "primary", "href": "repodata/abc123-primary.xml.gz", "remote_path": "repodata/repomd.xml"}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "repodata/repomd.xml", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/xml" {
		t.Errorf("expected application/xml, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "repomd") {
		t.Errorf("expected repomd in XML output, got: %s", body)
	}
}

func TestHandle_RepomdDynamicHasRequiredFields(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	// remote_path 与 handleRepomd 查询用的 RemotePath 一致，模拟回源后 Runtime 层存储的 RemotePath
	art := testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{
		"file": "abc123-primary.xml.gz", "type": "primary", "href": "repodata/abc123-primary.xml.gz",
		"remote_path": "repodata/repomd.xml",
	}, "")
	art.SizeBytes = 12345
	art.BlobRefs = []runtime.BlobRef{{Algorithm: "sha256", Digest: "abc123hash", Size: 12345}}

	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "repodata/repomd.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`<checksum`,
		`<open-checksum`,
		`<timestamp`,
		`<size`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("repomd missing %s: %s", want, body)
		}
	}
}

func TestHandle_PrimaryDynamicHasRequiredFields(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("yum", runtime.KindFile, map[string]string{
		"name":    "nginx",
		"version": "1.20.1",
		"file":    "primary.xml",
		"path":    "repodata",
	}, "")
	art.Attributes = map[string]string{"arch": "x86_64", "release": "1.el8", "epoch": "1", "summary": "A web server", "license": "BSD"}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "repodata/primary.xml", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`<name>nginx</name>`,
		`<arch>x86_64</arch>`,
		`<version`,
		`ver="1.20.1"`,
		`<location`,
		`href=`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("primary missing %s: %s", want, body)
		}
	}
}

func TestHandle_PrimaryCompressedXzRequiresOriginalContent(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	art := runtime.NewArtifact(runtime.ArtifactSpec{
		Format:     "yum",
		Kind:       runtime.KindMetadata,
		Name:       "abc123-primary.xml.xz",
		Path:       "repodata",
		Filename:   "abc123-primary.xml.xz",
		RemotePath: "repodata/abc123-primary.xml.xz",
	})
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "repodata/abc123-primary.xml.xz", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected compressed primary without original content to 404, got %d body=%q", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "<metadata") {
		t.Fatalf("compressed primary must not be dynamically rendered as plain XML: %s", w.Body.String())
	}
}

func TestHandle_RepomdHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{
		"file":     "repomd.xml",
		"filename": "repomd.xml",
		"path":     "repodata",
	}, `<repomd/>`)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "repodata/repomd.xml", nil)
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

func TestHandle_Primary(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	arts := []*runtime.Artifact{
		// remote_path 与 handlePrimary 的 GetArtifact key.RemotePath（请求路径）一致
		testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{
			"name":       "nginx",
			"version":    "1.20.1",
			"file":       "primary.xml",
			"remote_path": "repodata/primary.xml",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "repodata/primary.xml", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "nginx") {
		t.Errorf("expected 'nginx' in primary XML, got: %s", body)
	}
	if !strings.Contains(body, "1.20.1") {
		t.Errorf("expected '1.20.1' in primary XML")
	}
}

func TestHandle_PrimaryCompressed_PrefersOriginalContentWithGzipType(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	original := "gzipped-primary-bytes"
	art := testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{
		"file":     "abc123-primary.xml.gz",
		"filename": "abc123-primary.xml.gz",
		"path":     "repodata",
	}, original)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "repodata/abc123-primary.xml.gz", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != original {
		t.Fatalf("expected original primary metadata content, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("expected application/gzip, got %q", ct)
	}
}

func TestHandle_PrimaryCompressedXz_PrefersOriginalContentWithXzType(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	original := "xz-primary-bytes"
	art := testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{
		"file":     "abc123-primary.xml.xz",
		"filename": "abc123-primary.xml.xz",
		"path":     "repodata",
	}, original)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "repodata/abc123-primary.xml.xz", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != original {
		t.Fatalf("expected original primary metadata content, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/x-xz" {
		t.Fatalf("expected application/x-xz, got %q", ct)
	}
}

func TestHandle_RpmDownload(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("yum", "file", map[string]string{
		"file":     "nginx-1.20.1-1.el8.x86_64.rpm",
		"filename": "nginx-1.20.1-1.el8.x86_64.rpm",
		"path":     "Packages",
	}, "rpm-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "Packages/nginx-1.20.1-1.el8.x86_64.rpm", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/x-rpm" {
		t.Errorf("expected application/x-rpm, got %s", ct)
	}
}

func TestHandle_RpmHeadReturnsHeadersWithoutBody(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("yum", "file", map[string]string{
		"file":     "nginx-1.20.1-1.el8.x86_64.rpm",
		"filename": "nginx-1.20.1-1.el8.x86_64.rpm",
		"path":     "Packages",
	}, "rpm-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "Packages/nginx-1.20.1-1.el8.x86_64.rpm", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty HEAD body, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/x-rpm" {
		t.Fatalf("expected application/x-rpm, got %s", ct)
	}
}

func TestHandle_RpmRangeReturnsPartialContent(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	art := testhelper.NewArtifact("yum", "file", map[string]string{
		"file":     "nginx-1.20.1-1.el8.x86_64.rpm",
		"filename": "nginx-1.20.1-1.el8.x86_64.rpm",
		"path":     "Packages",
	}, "rpm-content")
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "Packages/nginx-1.20.1-1.el8.x86_64.rpm", nil)
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

func TestFetchRemote_RpmUsesDirectoryPathAndRemotePath(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), "http://example.test", "Packages/nginx-1.20.1-1.el8.x86_64.rpm")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	a := arts[0]
	if got := a.Path; got != "Packages" {
		t.Fatalf("path = %q, want directory path", got)
	}
	if got := a.Filename; got != "nginx-1.20.1-1.el8.x86_64.rpm" {
		t.Fatalf("filename = %q", got)
	}
	if got := a.Properties["remote_path"]; got != "Packages/nginx-1.20.1-1.el8.x86_64.rpm" {
		t.Fatalf("remote_path = %q, want full remote path", got)
	}
}

func TestFetchRemote_RepodataFileUsesMetadataKind(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), "http://example.test", "repodata/abc123-primary.xml.xz")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if arts[0].Kind != runtime.KindMetadata {
		t.Fatalf("repodata file kind = %q, want %q", arts[0].Kind, runtime.KindMetadata)
	}
	if arts[0].Name != "abc123-primary.xml.xz" {
		t.Fatalf("name = %q", arts[0].Name)
	}
}

func TestHandle_NotFound(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "repodata/repomd.xml", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandle_UnsupportedPath(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "unknown/path", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestIsRepomdRequest(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	tests := []struct {
		path string
		want bool
	}{
		{"repodata/repomd.xml", true},
		{"x86_64/repodata/repomd.xml", true},
		{"repodata/primary.xml.gz", false},
		{"Packages/nginx.rpm", false},
	}
	for _, tt := range tests {
		got := p.isRepomdRequest(tt.path)
		if got != tt.want {
			t.Errorf("isRepomdRequest(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsRpmPackageRequest(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	tests := []struct {
		path string
		want bool
	}{
		{"Packages/nginx-1.20.1-1.el8.x86_64.rpm", true},
		{"repodata/repomd.xml", false},
	}
	for _, tt := range tests {
		got := p.isRpmPackageRequest(tt.path)
		if got != tt.want {
			t.Errorf("isRpmPackageRequest(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestFetchRemote_Repomd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <data type="primary">
    <location href="repodata/abc123-primary.xml.gz"/>
  </data>
  <data type="filelists">
    <location href="repodata/def456-filelists.xml.gz"/>
  </data>
</repomd>`))
	}))
	defer srv.Close()

	p := NewYumPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "repodata/repomd.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	// Should return repomd.xml itself + 2 data references
	if len(arts) != 3 {
		t.Fatalf("expected 3 artifacts (repomd + 2 refs), got %d", len(arts))
	}
	if arts[0].Filename != "repomd.xml" {
		t.Errorf("first artifact should be repomd.xml, got %q", arts[0].Filename)
	}
	ref := arts[1]
	if ref.Kind != runtime.KindMetadata {
		t.Fatalf("expected %s, got %q", runtime.KindMetadata, ref.Kind)
	}
	if got := ref.Filename; got != "abc123-primary.xml.gz" {
		t.Fatalf("metadata-ref filename = %q", got)
	}
	if got := ref.Path; got != "repodata" {
		t.Fatalf("metadata-ref path = %q", got)
	}
	if got := ref.Properties["remote_path"]; got != "repodata/abc123-primary.xml.gz" {
		t.Fatalf("metadata-ref remote_path = %q", got)
	}
}

func TestFetchRemote_RepomdParsesPrimaryXMLXZPackages(t *testing.T) {
	primaryBody := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common">
  <package type="rpm">
    <name>nginx</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="1.20.1" rel="1.el8"/>
    <summary>A web server</summary>
    <description>HTTP server</description>
    <location href="Packages/nginx-1.20.1-1.el8.x86_64.rpm"/>
  </package>
</metadata>`
	var xzBody bytes.Buffer
	xw, err := xz.NewWriter(&xzBody)
	if err != nil {
		t.Fatalf("create xz writer: %v", err)
	}
	if _, err := xw.Write([]byte(primaryBody)); err != nil {
		t.Fatalf("write xz body: %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("close xz writer: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repodata/repomd.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <data type="primary">
    <location href="repodata/abc123-primary.xml.xz"/>
  </data>
</repomd>`))
		case "/repodata/abc123-primary.xml.xz":
			w.Header().Set("Content-Type", "application/x-xz")
			w.Write(xzBody.Bytes())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewYumPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), srv.URL, "repodata/repomd.xml")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	var rpm *runtime.Artifact
	for _, a := range arts {
		if a.Name == "nginx" {
			rpm = a
			break
		}
	}
	if rpm == nil {
		t.Fatalf("expected parsed nginx RPM artifact, got %#v", arts)
	}
	if rpm.Kind != runtime.KindFile {
		t.Fatalf("kind = %q, want %q", rpm.Kind, runtime.KindFile)
	}
	if rpm.Version != "1.20.1" {
		t.Fatalf("version = %q, want 1.20.1", rpm.Version)
	}
	if rpm.RemotePath != "Packages/nginx-1.20.1-1.el8.x86_64.rpm" {
		t.Fatalf("remote path = %q", rpm.RemotePath)
	}
}

func TestFetchPrimaryIndexPackagesRejectsOversizedContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Length", "67108865")
		w.Write([]byte("not-a-real-gzip"))
	}))
	defer srv.Close()

	p := NewYumPlugin(http.DefaultClient)
	_, err := p.fetchPrimaryIndexPackages(context.Background(), srv.URL, []repomdData{{
		Type: "primary",
		Location: struct {
			Href string `xml:"href,attr"`
		}{Href: "repodata/primary.xml.gz"},
	}})
	if err == nil {
		t.Fatal("expected oversized primary error")
	}
	if !strings.Contains(err.Error(), "primary body too large") {
		t.Fatalf("error = %v, want primary body too large", err)
	}
}

func TestFetchRemote_RpmFile(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	arts, err := p.FetchRemote(context.Background(), "http://example.com", "Packages/nginx-1.20.1-1.el8.x86_64.rpm")
	if err != nil {
		t.Fatalf("FetchRemote failed: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if arts[0].Filename != "nginx-1.20.1-1.el8.x86_64.rpm" {
		t.Errorf("expected filename, got %q", arts[0].Filename)
	}
}

func TestHandle_QueryRemotePath_Repomd(t *testing.T) {
	p := NewYumPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{}

	ctx, _ := newCtx("GET", "repodata/repomd.xml", nil)
	p.Handle(ctx, rt)

	if len(rt.QueryCalls) < 1 {
		t.Fatal("expected at least 1 query call")
	}
	if rt.QueryCalls[0].RemotePath != "repodata/repomd.xml" {
		t.Errorf("unexpected RemotePath: %q", rt.QueryCalls[0].RemotePath)
	}
}
