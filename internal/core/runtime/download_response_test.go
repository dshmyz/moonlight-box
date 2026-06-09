package runtime

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServeArtifactContent_RangeReturnsPartialContent(t *testing.T) {
	artifact := NewArtifact(ArtifactSpec{
		Format:   "generic",
		Kind:     KindFile,
		Name:     "readme.txt",
		Filename: "readme.txt",
		Content:  io.NopCloser(strings.NewReader("hello world")),
	})
	req := httptest.NewRequest(http.MethodGet, "/repository/test/readme.txt", nil)
	req.Header.Set("Range", "bytes=6-10")
	w := httptest.NewRecorder()

	if err := ServeArtifactContent(w, req, artifact, "readme.txt", "text/plain", "inline"); err != nil {
		t.Fatalf("ServeArtifactContent failed: %v", err)
	}

	if w.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 6-10/11" {
		t.Fatalf("expected Content-Range bytes 6-10/11, got %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "5" {
		t.Fatalf("expected Content-Length 5, got %q", got)
	}
	if got := w.Body.String(); got != "world" {
		t.Fatalf("expected body %q, got %q", "world", got)
	}
}

func TestServeArtifactContent_SetsETagAndLastModified(t *testing.T) {
	updatedAt := time.Date(2026, 6, 9, 10, 11, 12, 0, time.UTC)
	artifact := NewArtifact(ArtifactSpec{
		Format:   "generic",
		Kind:     KindFile,
		Name:     "readme.txt",
		Filename: "readme.txt",
		BlobRefs: []BlobRef{{Algorithm: "sha256", Digest: "abc123", Size: 11}},
		Content:  io.NopCloser(strings.NewReader("hello world")),
	})
	artifact.UpdatedAt = updatedAt
	req := httptest.NewRequest(http.MethodGet, "/repository/test/readme.txt", nil)
	w := httptest.NewRecorder()

	if err := ServeArtifactContent(w, req, artifact, "readme.txt", "text/plain", "inline"); err != nil {
		t.Fatalf("ServeArtifactContent failed: %v", err)
	}

	if got := w.Header().Get("ETag"); got != `"sha256:abc123"` {
		t.Fatalf("expected ETag %q, got %q", `"sha256:abc123"`, got)
	}
	if got := w.Header().Get("Last-Modified"); got != updatedAt.Format(http.TimeFormat) {
		t.Fatalf("expected Last-Modified %q, got %q", updatedAt.Format(http.TimeFormat), got)
	}
}

func TestServeArtifactContent_IfNoneMatchReturnsNotModifiedWithoutBody(t *testing.T) {
	artifact := NewArtifact(ArtifactSpec{
		Format:   "generic",
		Kind:     KindFile,
		Name:     "readme.txt",
		Filename: "readme.txt",
		BlobRefs: []BlobRef{{Algorithm: "sha256", Digest: "abc123", Size: 11}},
		Content:  io.NopCloser(strings.NewReader("hello world")),
	})
	req := httptest.NewRequest(http.MethodGet, "/repository/test/readme.txt", nil)
	req.Header.Set("If-None-Match", `"sha256:abc123"`)
	w := httptest.NewRecorder()

	if err := ServeArtifactContent(w, req, artifact, "readme.txt", "text/plain", "inline"); err != nil {
		t.Fatalf("ServeArtifactContent failed: %v", err)
	}

	if w.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", w.Code)
	}
	if got := w.Body.String(); got != "" {
		t.Fatalf("expected empty body for 304, got %q", got)
	}
}

func TestServeArtifactContent_IfModifiedSinceReturnsNotModifiedWithoutBody(t *testing.T) {
	updatedAt := time.Date(2026, 6, 9, 10, 11, 12, 0, time.UTC)
	artifact := NewArtifact(ArtifactSpec{
		Format:   "generic",
		Kind:     KindFile,
		Name:     "readme.txt",
		Filename: "readme.txt",
		Content:  io.NopCloser(strings.NewReader("hello world")),
	})
	artifact.UpdatedAt = updatedAt
	req := httptest.NewRequest(http.MethodGet, "/repository/test/readme.txt", nil)
	req.Header.Set("If-Modified-Since", updatedAt.Add(time.Minute).Format(http.TimeFormat))
	w := httptest.NewRecorder()

	if err := ServeArtifactContent(w, req, artifact, "readme.txt", "text/plain", "inline"); err != nil {
		t.Fatalf("ServeArtifactContent failed: %v", err)
	}

	if w.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", w.Code)
	}
	if got := w.Body.String(); got != "" {
		t.Fatalf("expected empty body for 304, got %q", got)
	}
}
