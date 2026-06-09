package runtime

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ServeArtifactContent writes a downloadable artifact response.
// It centralizes HTTP download semantics that are shared by protocol plugins.
func ServeArtifactContent(w http.ResponseWriter, r *http.Request, artifact *Artifact, filename, contentType, disposition string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if disposition == "" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", contentType)
	if filename != "" {
		w.Header().Set("Content-Disposition", disposition+"; filename=\""+SanitizeFilename(filename)+"\"")
	}
	setArtifactCacheHeaders(w, artifact)
	if isArtifactNotModified(r, artifact) {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	if artifact == nil || artifact.Content == nil {
		w.WriteHeader(http.StatusOK)
		return nil
	}

	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		return serveArtifactRange(w, r, artifact, rangeHeader)
	}

	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return nil
	}
	_, err := io.Copy(w, artifact.Content)
	return err
}

func serveArtifactRange(w http.ResponseWriter, r *http.Request, artifact *Artifact, rangeHeader string) error {
	data, err := io.ReadAll(artifact.Content)
	if err != nil {
		return err
	}
	size := int64(len(data))
	start, end, ok := parseSingleByteRange(rangeHeader, size)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil
	}

	part := data[start : end+1]
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(part)), 10))
	w.WriteHeader(http.StatusPartialContent)
	if r.Method == http.MethodHead {
		return nil
	}
	_, err = w.Write(part)
	return err
}

func setArtifactCacheHeaders(w http.ResponseWriter, artifact *Artifact) {
	if artifact == nil {
		return
	}
	for _, ref := range artifact.BlobRefs {
		if ref.Algorithm != "" && ref.Digest != "" {
			w.Header().Set("ETag", "\""+ref.Algorithm+":"+ref.Digest+"\"")
			break
		}
	}
	modified := artifact.UpdatedAt
	if modified.IsZero() {
		modified = artifact.CreatedAt
	}
	if !modified.IsZero() {
		w.Header().Set("Last-Modified", modified.UTC().Format(http.TimeFormat))
	}
}
func isArtifactNotModified(r *http.Request, artifact *Artifact) bool {
	if artifact == nil {
		return false
	}
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		etag := artifactETag(artifact)
		return etag != "" && matchETag(inm, etag)
	}
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		modified := artifactModifiedTime(artifact)
		if modified.IsZero() {
			return false
		}
		since, err := http.ParseTime(ims)
		if err != nil {
			return false
		}
		return !modified.UTC().After(since)
	}
	return false
}

func matchETag(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}

func artifactETag(artifact *Artifact) string {
	if artifact == nil {
		return ""
	}
	for _, ref := range artifact.BlobRefs {
		if ref.Algorithm != "" && ref.Digest != "" {
			return "\"" + ref.Algorithm + ":" + ref.Digest + "\""
		}
	}
	return ""
}

func artifactModifiedTime(artifact *Artifact) time.Time {
	if artifact == nil {
		return time.Time{}
	}
	modified := artifact.UpdatedAt
	if modified.IsZero() {
		modified = artifact.CreatedAt
	}
	return modified
}

func parseSingleByteRange(header string, size int64) (int64, int64, bool) {
	if size < 0 || !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false
	}
	spec := strings.TrimPrefix(header, "bytes=")
	if spec == "" || strings.Contains(spec, ",") {
		return 0, 0, false
	}
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, size > 0
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true
}
