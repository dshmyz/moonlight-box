package pypi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/plugins/testhelper"
)

func TestIsMetadataRequest(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)

	tests := []struct {
		path     string
		expected bool
	}{
		{"simple/requests/requests-2.28.0.tar.gz.metadata", true},
		{"simple/requests/", false},
		{"packages/ab/cd/requests-2.28.0.tar.gz", false},
		{"simple/requests/requests-2.28.0.tar.gz", false},
	}

	for _, tt := range tests {
		if got := p.isMetadataRequest(tt.path); got != tt.expected {
			t.Errorf("isMetadataRequest(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestHandle_MetadataRequest(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	metadataContent := "Metadata-Version: 2.1\nName: requests\nVersion: 2.28.0\nRequires-Python: >=3.7"
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "packages/ab/cd/requests-2.28.0.tar.gz",
		Attributes: map[string]string{
			"artifact_type": "package-file",
			"metadata":      metadataContent,
		},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "simple/requests/requests-2.28.0.tar.gz.metadata", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("expected Content-Type text/plain, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), "Metadata-Version: 2.1") {
		t.Fatalf("expected metadata content, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Requires-Python: >=3.7") {
		t.Fatalf("expected Requires-Python in metadata, got %s", w.Body.String())
	}
}

func TestHandle_MetadataRequest_HeadMethod(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	metadataContent := "Metadata-Version: 2.1\nName: requests\nVersion: 2.28.0"
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "packages/ab/cd/requests-2.28.0.tar.gz",
		Attributes: map[string]string{
			"metadata": metadataContent,
		},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("HEAD", "simple/requests/requests-2.28.0.tar.gz.metadata", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// HEAD 不应返回 body
	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body for HEAD, got %d bytes", w.Body.Len())
	}
	// 但应包含 Content-Length 头
	if cl := w.Header().Get("Content-Length"); cl == "" {
		t.Fatalf("expected Content-Length header for HEAD")
	}
}

func TestHandle_MetadataRequest_NoMetadataAvailable(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	art := &runtime.Artifact{
		Format:     "pypi",
		Kind:       "package-file",
		Name:       "requests",
		Filename:   "requests-2.28.0.tar.gz",
		RemotePath: "packages/ab/cd/requests-2.28.0.tar.gz",
		Attributes: map[string]string{
			"artifact_type": "package-file",
		},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "simple/requests/requests-2.28.0.tar.gz.metadata", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when metadata not available, got %d", w.Code)
	}
}

func TestHandle_MetadataRequest_PackageNotFound(t *testing.T) {
	p := NewPyPIPlugin(http.DefaultClient)
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{}}

	ctx, w := newCtx("GET", "simple/nonexistent/nonexistent-1.0.0.tar.gz.metadata", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent package, got %d", w.Code)
	}
}

func TestHandle_PackageListIncludesMetadataLink(t *testing.T) {
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
			"artifact_type": "package-file",
			"metadata":      "Metadata-Version: 2.1",
		},
		Properties: map[string]string{"remote_path": "packages/ab/cd/requests-2.28.0.tar.gz"},
	}
	rt := &testhelper.MockRuntime{Artifacts: []*runtime.Artifact{art}}

	ctx, w := newCtx("GET", "simple/requests/", nil)
	if err := p.Handle(ctx, rt); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, `data-dist-info-metadata="true"`) {
		t.Fatalf("expected metadata attribute in HTML, got %s", body)
	}
	if !strings.Contains(body, `requests-2.28.0.tar.gz`) {
		t.Fatalf("expected filename in HTML, got %s", body)
	}
}

func TestHandle_PackageListJSONIncludesMetadata(t *testing.T) {
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
			"artifact_type": "package-file",
			"metadata":      "Metadata-Version: 2.1",
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
		Files []struct {
			DistInfoMetadata map[string]string `json:"dist-info-metadata"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if len(data.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(data.Files))
	}
	if data.Files[0].DistInfoMetadata == nil {
		t.Fatalf("expected dist-info-metadata field to be present")
	}
	if got := data.Files[0].DistInfoMetadata["sha256"]; got != "abc123" {
		t.Fatalf("dist-info-metadata.sha256 = %q, want abc123", got)
	}
}