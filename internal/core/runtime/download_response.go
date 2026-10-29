package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxInMemoryRangeFallbackBytes = 8 * 1024 * 1024

// ContentTypeForFilename 按扩展名推导 Content-Type，对齐 Nexus 的映射习惯。
// 不用 mime.TypeByExtension：它依赖宿主机 /etc/mime.types，不同机器结果不同。
func ContentTypeForFilename(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".tgz":
		return "application/x-tgz"
	case ".gz":
		return "application/gzip"
	case ".jar", ".war", ".ear":
		return "application/java-archive"
	case ".zip", ".whl":
		return "application/zip"
	case ".pom", ".xml":
		return "application/xml"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	case ".deb":
		return "application/vnd.debian.binary-package"
	case ".rpm":
		return "application/x-rpm"
	case ".xz":
		return "application/x-xz"
	case ".bz2":
		return "application/x-bzip2"
	default:
		return "application/octet-stream"
	}
}

// ServeArtifactContent writes a downloadable artifact response.
// It centralizes HTTP download semantics that are shared by protocol plugins.
func ServeArtifactContent(w http.ResponseWriter, r *http.Request, artifact *Artifact, filename, contentType, disposition string) error {
	// 调用方没指定类型时按文件名推导：MIME 只在这里决定，插件不再各自硬编码。
	if contentType == "" {
		contentType = ContentTypeForFilename(filename)
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

	// 已知大小就带 Content-Length：客户端据此判断"收全了没有"。没有它，任何
	// 传输中断都只能由格式解码器报"数据不完整"，客户端还会把半截内容缓存下来。
	// 多 blob 制品不声明长度：Content 只包含 BlobRefs[0]，声明总长会误报截断。
	size := int64(0)
	if len(artifact.BlobRefs) <= 1 {
		size = artifactKnownSize(artifact)
	}
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return nil
	}
	if size <= 0 {
		_, err := io.Copy(w, artifact.Content)
		return err
	}
	written, err := io.CopyN(w, artifact.Content, size)
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("artifact content truncated: wrote %d of %d bytes", written, size)
	}
	return err
}

func serveArtifactRange(w http.ResponseWriter, r *http.Request, artifact *Artifact, rangeHeader string) error {
	if seeker, ok := artifact.Content.(io.Seeker); ok {
		size := artifact.SizeBytes
		if size <= 0 && len(artifact.BlobRefs) > 0 {
			size = artifact.BlobRefs[0].Size
		}
		if size > 0 {
			start, end, ok := parseSingleByteRange(rangeHeader, size)
			if !ok {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return nil
			}
			if _, err := seeker.Seek(start, io.SeekStart); err != nil {
				return err
			}
			length := end - start + 1
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
			w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
			w.WriteHeader(http.StatusPartialContent)
			if r.Method == http.MethodHead {
				return nil
			}
			_, err := io.CopyN(w, artifact.Content, length)
			return err
		}
	}

	size := artifactKnownSize(artifact)
	if size > maxInMemoryRangeFallbackBytes {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil
	}

	data, err := readRangeFallbackData(artifact.Content)
	if err != nil {
		return err
	}
	size = int64(len(data))
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

func artifactKnownSize(artifact *Artifact) int64 {
	if artifact == nil {
		return 0
	}
	if artifact.SizeBytes > 0 {
		return artifact.SizeBytes
	}
	for _, ref := range artifact.BlobRefs {
		if ref.Size > 0 {
			return ref.Size
		}
	}
	return 0
}

func readRangeFallbackData(content io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	limited := io.LimitReader(content, maxInMemoryRangeFallbackBytes+1)
	if _, err := buf.ReadFrom(limited); err != nil {
		return nil, err
	}
	if buf.Len() > maxInMemoryRangeFallbackBytes {
		return nil, fmt.Errorf("range fallback content exceeds in-memory limit %d", maxInMemoryRangeFallbackBytes)
	}
	return buf.Bytes(), nil
}

func setArtifactCacheHeaders(w http.ResponseWriter, artifact *Artifact) {
	if artifact == nil {
		return
	}
	if etag := artifactETag(artifact); etag != "" {
		w.Header().Set("ETag", etag)
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
	if artifact.Properties != nil {
		return normalizeETag(artifact.Properties["remote_etag"])
	}
	return ""
}

func normalizeETag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "W/\"") && strings.HasSuffix(value, "\"") {
		return value
	}
	value = strings.Trim(value, `"`)
	if value == "" {
		return ""
	}
	return "\"" + value + "\""
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
