package yum

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
	req := httptest.NewRequest(method, "/repository/yum-test/"+path, body)
	return &runtime.RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     &runtime.Repository{ID: "1", Name: "yum-test", Format: "yum", Type: "local"},
		RepositoryPath: "/" + path,
	}, w
}

func TestHandle_Repomd(t *testing.T) {
	p := NewYumPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{"file": "repomd.xml"}, ""),
		testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{"file": "abc123-primary.xml.gz", "type": "primary", "href": "repodata/abc123-primary.xml.gz"}, ""),
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

func TestHandle_Primary(t *testing.T) {
	p := NewYumPlugin()
	arts := []*runtime.Artifact{
		testhelper.NewArtifact("yum", runtime.KindMetadata, map[string]string{
			"name":    "nginx",
			"version": "1.20.1",
			"file":    "abc123-primary.xml.gz",
		}, ""),
	}
	rt := &testhelper.MockRuntime{Artifacts: arts}

	ctx, w := newCtx("GET", "repodata/abc123-primary.xml.gz", nil)
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
	p := NewYumPlugin()
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

func TestHandle_RpmDownload(t *testing.T) {
	p := NewYumPlugin()
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

func TestFetchRemote_RpmUsesDirectoryPathAndRemotePath(t *testing.T) {
	p := NewYumPlugin()
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

func TestHandle_NotFound(t *testing.T) {
	p := NewYumPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "repodata/repomd.xml", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandle_UnsupportedPath(t *testing.T) {
	p := NewYumPlugin()
	rt := &testhelper.MockRuntime{}

	ctx, w := newCtx("GET", "unknown/path", nil)
	p.Handle(ctx, rt)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestIsRepomdRequest(t *testing.T) {
	p := NewYumPlugin()
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
	p := NewYumPlugin()
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

	p := NewYumPlugin()
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

func TestFetchRemote_RpmFile(t *testing.T) {
	p := NewYumPlugin()
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
	p := NewYumPlugin()
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
