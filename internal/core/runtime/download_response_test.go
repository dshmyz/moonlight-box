package runtime

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
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

func TestServeArtifactContent_RangeUsesSeekWhenAvailable(t *testing.T) {
	content := &trackingReadSeeker{Reader: strings.NewReader("hello world")}
	artifact := NewArtifact(ArtifactSpec{
		Format:    "generic",
		Kind:      KindFile,
		Name:      "readme.txt",
		Filename:  "readme.txt",
		SizeBytes: 11,
		Content:   content,
	})
	req := httptest.NewRequest(http.MethodGet, "/repository/test/readme.txt", nil)
	req.Header.Set("Range", "bytes=6-10")
	w := httptest.NewRecorder()

	if err := ServeArtifactContent(w, req, artifact, "readme.txt", "text/plain", "inline"); err != nil {
		t.Fatalf("ServeArtifactContent failed: %v", err)
	}

	if content.seekCalls != 1 {
		t.Fatalf("expected one seek call, got %d", content.seekCalls)
	}
	if content.bytesRead != 5 {
		t.Fatalf("expected to read only 5 bytes, read %d", content.bytesRead)
	}
	if got := w.Body.String(); got != "world" {
		t.Fatalf("expected body %q, got %q", "world", got)
	}
}

func TestServeArtifactContent_LargeNonSeekableRangeDoesNotReadWholeBody(t *testing.T) {
	const largeNonSeekableSize = 8*1024*1024 + 1024
	content := &countingLargeReader{remaining: largeNonSeekableSize}
	artifact := NewArtifact(ArtifactSpec{
		Format:    "generic",
		Kind:      KindFile,
		Name:      "large.bin",
		Filename:  "large.bin",
		SizeBytes: largeNonSeekableSize,
		Content:   content,
	})
	req := httptest.NewRequest(http.MethodGet, "/repository/test/large.bin", nil)
	req.Header.Set("Range", "bytes=0-4")
	w := httptest.NewRecorder()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err := ServeArtifactContent(w, req, artifact, "large.bin", "application/octet-stream", "inline")
	runtime.ReadMemStats(&after)

	if err != nil {
		t.Fatalf("ServeArtifactContent failed: %v", err)
	}
	if w.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416 for large non-seekable range, got %d", w.Code)
	}
	if content.bytesRead != 0 {
		t.Fatalf("expected no body reads for large non-seekable range, read %d bytes", content.bytesRead)
	}
	if growth := after.TotalAlloc - before.TotalAlloc; growth > 1024*1024 {
		t.Fatalf("expected no large allocation for range fallback, allocated %d bytes", growth)
	}
}

type trackingReadSeeker struct {
	*strings.Reader
	seekCalls int
	bytesRead int
}

func (r *trackingReadSeeker) Seek(offset int64, whence int) (int64, error) {
	r.seekCalls++
	return r.Reader.Seek(offset, whence)
}

func (r *trackingReadSeeker) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytesRead += n
	return n, err
}

func (r *trackingReadSeeker) Close() error { return nil }

type countingLargeReader struct {
	remaining int64
	bytesRead int64
}

func (r *countingLargeReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = 'x'
	}
	n := len(p)
	r.remaining -= int64(n)
	r.bytesRead += int64(n)
	return n, nil
}

func (r *countingLargeReader) Close() error { return nil }

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
