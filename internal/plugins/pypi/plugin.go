package pypi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

type PyPIPlugin struct {
	httpClient *http.Client // 统一 HTTP 客户端
}

func NewPyPIPlugin() *PyPIPlugin {
	return &PyPIPlugin{
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// SetHTTPClient 允许注入统一的 HTTP 客户端
func (p *PyPIPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

func (p *PyPIPlugin) Name() string {
	return "pypi"
}

// FetchRemote 实现 RemoteFetcher 接口。
// Runtime 在本地缓存为空时回调此方法，Plugin 负责远端协议交互。
func (p *PyPIPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + path
	resp, err := p.httpGet(ctx, fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote returned status %d for %s", resp.StatusCode, fullURL)
	}

	if p.isSimpleIndexRequest(path) {
		return p.parseSimpleIndex(resp.Body)
	}
	if p.isPackageListRequest(path) {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		return p.parsePackageList(normalizePackageName(parts[1]), resp.Body)
	}
	return nil, fmt.Errorf("unsupported remote path: %s", path)
}

func (p *PyPIPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if err := validatePyPIPath(path); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}

	if p.isSimpleIndexRequest(path) {
		return p.handleSimpleIndex(ctx, repoRuntime, path)
	}

	if p.isPackageListRequest(path) {
		return p.handlePackageList(ctx, repoRuntime, path)
	}

	if p.isPackagesPath(path) {
		return p.handlePackagesDownload(ctx, repoRuntime, path)
	}

	if p.isJsonAPIRequest(path) {
		return p.handleJsonAPI(ctx, repoRuntime, path)
	}

	return errors.New("invalid pypi path")
}

func (p *PyPIPlugin) isSimpleIndexRequest(path string) bool {
	return path == "simple" || path == "simple/"
}

func (p *PyPIPlugin) isPackageListRequest(path string) bool {
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	// PEP 503: simple/{pkg}/ 或 simple/{pkg}
	return len(parts) == 2 && parts[0] == "simple"
}

func (p *PyPIPlugin) isPackagesPath(path string) bool {
	return strings.HasPrefix(path, "packages/")
}

func (p *PyPIPlugin) isJsonAPIRequest(path string) bool {
	return strings.HasPrefix(path, "pypi/") && strings.HasSuffix(path, "/json")
}

func (p *PyPIPlugin) handleSimpleIndex(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		RemotePath:   path,
	})
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	accept := ctx.Request.Header.Get("Accept")
	if strings.Contains(accept, "application/vnd.pypi.simple") || strings.Contains(accept, "application/json") {
		return p.writeSimpleIndexJSON(ctx, artifacts)
	}
	return p.writeSimpleIndexHTML(ctx, artifacts)
}

func (p *PyPIPlugin) writeSimpleIndexHTML(ctx *runtime.RequestContext, artifacts []*runtime.Artifact) error {
	seen := make(map[string]bool)
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html><head><title>Simple Index</title></head><body>\n")
	for _, artifact := range artifacts {
		name := artifact.Coordinates["package"]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		escaped := html.EscapeString(name)
		sb.WriteString(`<a href="`)
		sb.WriteString(escaped)
		sb.WriteString(`/">`)
		sb.WriteString(escaped)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	output := sb.String()
	ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write([]byte(output))
	return nil
}

func (p *PyPIPlugin) writeSimpleIndexJSON(ctx *runtime.RequestContext, artifacts []*runtime.Artifact) error {
	seen := make(map[string]bool)
	projects := make([]map[string]string, 0)
	for _, artifact := range artifacts {
		name := artifact.Coordinates["package"]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		projects = append(projects, map[string]string{
			"name": name,
			"url":  name + "/",
		})
	}

	data := map[string]interface{}{
		"meta":     map[string]string{"api-version": "1.0"},
		"projects": projects,
	}

	ctx.Writer.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func (p *PyPIPlugin) handlePackageList(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	packageName := normalizePackageName(parts[1])

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Coordinates: map[string]string{
			"package": packageName,
		},
		RemotePath: path,
	})
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	accept := ctx.Request.Header.Get("Accept")
	if strings.Contains(accept, "application/vnd.pypi.simple") || strings.Contains(accept, "application/json") {
		return p.writePackageFilesJSON(ctx, packageName, artifacts)
	}
	return p.writePackageFilesHTML(ctx, packageName, artifacts)
}

func (p *PyPIPlugin) writePackageFilesHTML(ctx *runtime.RequestContext, packageName string, artifacts []*runtime.Artifact) error {
	var sb strings.Builder
	escapedPkg := html.EscapeString(packageName)
	sb.WriteString(fmt.Sprintf("<!DOCTYPE html>\n<html><head><title>Links for %s</title></head><body>\n<h1>Links for %s</h1>\n", escapedPkg, escapedPkg))

	for _, artifact := range artifacts {
		if artifact.Coordinates["package"] != packageName {
			continue
		}
		filename := artifact.Coordinates["filename"]
		remotePath := artifact.Properties["remote_path"]
		if filename == "" || remotePath == "" {
			continue
		}
		sb.WriteString(`<a href="../../packages/`)
		sb.WriteString(html.EscapeString(remotePath))
		sb.WriteString(`">`)
		sb.WriteString(html.EscapeString(filename))
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	output := sb.String()
	ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write([]byte(output))
	return nil
}

func (p *PyPIPlugin) writePackageFilesJSON(ctx *runtime.RequestContext, packageName string, artifacts []*runtime.Artifact) error {
	files := make([]map[string]interface{}, 0)
	for _, artifact := range artifacts {
		if artifact.Coordinates["package"] != packageName {
			continue
		}
		filename := artifact.Coordinates["filename"]
		remotePath := artifact.Properties["remote_path"]
		if filename == "" || remotePath == "" {
			continue
		}

		file := map[string]interface{}{
			"url":      "../../packages/" + remotePath,
			"filename": filename,
		}

		hashes := make(map[string]string)
		for _, blobRef := range artifact.BlobRefs {
			if blobRef.Algorithm == "sha256" {
				hashes["sha256"] = blobRef.Digest
			}
		}
		if len(hashes) > 0 {
			file["hashes"] = hashes
		}

		files = append(files, file)
	}

	data := map[string]interface{}{
		"meta":  map[string]string{"api-version": "1.0"},
		"files": files,
	}

	ctx.Writer.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func (p *PyPIPlugin) handlePackagesDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	filename := filepath.Base(path)
	dir := filepath.Dir(path) // e.g. "packages/62/35/0230421b8c4efad6624518028163329ad0c2df9e58e6b3bee013427bf8f6"

	if strings.HasSuffix(filename, ".sha256") {
		return p.handleChecksumRequest(ctx, repoRuntime, filename)
	}

	packageName := p.extractPackageNameFromFilename(filename)
	version := p.extractVersionFromFilename(filename)

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Coordinates: map[string]string{
			"package":  packageName,
			"version":  version,
			"filename": filename,
			"path":     dir,
		},
		Filename: filename,
	}

	switch ctx.Request.Method {
	case http.MethodGet:
		artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err == nil {
			defer artifact.Content.Close()
			ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
			ctx.Writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, runtime.SanitizeFilename(key.Filename)))
			ctx.Writer.WriteHeader(http.StatusOK)
			io.Copy(ctx.Writer, artifact.Content)
			return nil
		}
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	case http.MethodPut:
		return p.handleUpload(ctx, repoRuntime, key)
	}
	return errors.New("method not allowed")
}

func (p *PyPIPlugin) handleChecksumRequest(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, filename string) error {
	actualFilename := strings.TrimSuffix(filename, ".sha256")

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Coordinates: map[string]string{
			"filename": actualFilename,
		},
		Filename: actualFilename,
	}

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	if len(artifact.BlobRefs) == 0 {
		http.Error(ctx.Writer, "No blob", http.StatusNotFound)
		return nil
	}

	sha256Digest := artifact.BlobRefs[0].Digest
	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write([]byte(sha256Digest + "\n"))
	return nil
}

func (p *PyPIPlugin) handleJsonAPI(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	path = strings.TrimPrefix(path, "pypi/")
	path = strings.TrimSuffix(path, "/json")
	parts := strings.Split(path, "/")

	packageName := normalizePackageName(parts[0])
	var version string
	if len(parts) > 1 {
		version = parts[1]
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	releases := make(map[string][]map[string]interface{})
	for _, artifact := range artifacts {
		if artifact.Coordinates["package"] != packageName {
			continue
		}
		v := artifact.Coordinates["version"]
		if version != "" && v != version {
			continue
		}

		filename := artifact.Coordinates["filename"]
		remotePath := artifact.Properties["remote_path"]
		if filename == "" || remotePath == "" {
			continue
		}

		file := map[string]interface{}{
			"filename": filename,
			"url":      "../../packages/" + filename,
		}

		if len(artifact.BlobRefs) > 0 {
			file["digest"] = map[string]string{
				"sha256": artifact.BlobRefs[0].Digest,
			}
			file["size"] = artifact.BlobRefs[0].Size
		}

		releases[v] = append(releases[v], file)
	}

	data := map[string]interface{}{
		"info": map[string]interface{}{
			"name":    packageName,
			"version": version,
		},
		"releases": releases,
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func (p *PyPIPlugin) handleDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}

	if len(artifact.BlobRefs) == 0 {
		http.Error(ctx.Writer, "No blob available", http.StatusNotFound)
		return nil
	}
	if artifact.Content == nil {
		http.Error(ctx.Writer, "No blob available", http.StatusNotFound)
		return nil
	}
	defer artifact.Content.Close()

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, runtime.SanitizeFilename(key.Filename)))
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		return err
	}
	return nil
}

func (p *PyPIPlugin) handleUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	packageName := p.extractPackageNameFromFilename(key.Filename)
	version := p.extractVersionFromFilename(key.Filename)

	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Filename:     key.Filename,
		Size:         ctx.Request.ContentLength,
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	blobRef, err := session.PutBlob(ctx.Request.Context(), ctx.Request.Body)
	if err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	artifact := &runtime.Artifact{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Kind:         "package",
		Coordinates: map[string]string{
			"name":     packageName,
			"package":  packageName,
			"version":  version,
			"filename": key.Filename,
		},
		BlobRefs: []runtime.BlobRef{blobRef},
		Properties: map[string]string{
			"filename": key.Filename,
		},
	}

	if err := session.PutArtifact(ctx.Request.Context(), artifact); err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	if err := session.Commit(ctx.Request.Context()); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	ctx.Writer.WriteHeader(http.StatusCreated)
	return nil
}

func (p *PyPIPlugin) extractPackageNameFromFilename(filename string) string {
	if strings.HasSuffix(filename, ".whl") {
		parts := strings.Split(filename, "-")
		if len(parts) >= 1 {
			return normalizePackageName(parts[0])
		}
	}

	base := filename
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".zip"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}

	parts := strings.Split(base, "-")
	if len(parts) >= 1 {
		return normalizePackageName(parts[0])
	}
	return ""
}

func (p *PyPIPlugin) extractVersionFromFilename(filename string) string {
	if strings.HasSuffix(filename, ".whl") {
		parts := strings.Split(filename, "-")
		if len(parts) >= 2 {
			return parts[1]
		}
	}

	base := filename
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".zip"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}

	parts := strings.Split(base, "-")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

func validatePyPIPath(path string) error {
	if path == "" {
		return nil
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid pypi path: path traversal not allowed")
	}
	dangerousPatterns := []string{"~", "$", "`", "|", ";", "&", "(", ")", "<", ">", "\n", "\r", "\x00"}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(path, pattern) {
			return fmt.Errorf("invalid pypi path: dangerous character not allowed")
		}
	}
	return nil
}

func normalizePackageName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func isValidWheelFilename(filename string) bool {
	// PEP 427: {distribution}-{version}(-{build})?-{python}-{abi}-{platform}.whl
	// 标签可含字母数字及 . _ 等，放宽校验避免误杀合法文件名
	if !strings.HasSuffix(filename, ".whl") {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(filename, ".whl"), "-")
	return len(parts) >= 5 // name, version, python-tag, abi-tag, platform-tag
}

func isValidSdistFilename(filename string) bool {
	// PEP 625: {name}-{version}.tar.gz / .tar.bz2 / .zip
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".zip"} {
		if strings.HasSuffix(filename, ext) {
			return strings.Count(filename, "-") >= 1
		}
	}
	return false
}

func isValidPyPIFilename(filename string) bool {
	return isValidWheelFilename(filename) || isValidSdistFilename(filename)
}

func (p *PyPIPlugin) httpGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Moonlight-Registry/1.0")
	return p.httpClient.Do(req)
}

// parseSimpleIndex 解析 PyPI Simple Index 页面，提取包名列表
func (p *PyPIPlugin) parseSimpleIndex(body io.Reader) ([]*runtime.Artifact, error) {
	htmlBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	html := string(htmlBytes)
	re := regexp.MustCompile(`<a href="([^"]+)/">([^<]+)</a>`)
	matches := re.FindAllStringSubmatch(html, -1)
	seen := make(map[string]bool)
	var artifacts []*runtime.Artifact
	for _, m := range matches {
		pkgName := normalizePackageName(m[2])
		if pkgName == "" || seen[pkgName] {
			continue
		}
		seen[pkgName] = true
		artifacts = append(artifacts, &runtime.Artifact{
			Format: "pypi",
			Kind:   "package-index",
			Coordinates: map[string]string{
				"name":    pkgName,
				"package": pkgName,
			},
		})
	}
	return artifacts, nil
}

// parsePackageList 解析 PyPI 包版本列表页面，提取文件列表
func (p *PyPIPlugin) parsePackageList(packageName string, body io.Reader) ([]*runtime.Artifact, error) {
	htmlBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	html := string(htmlBytes)
	re := regexp.MustCompile(`<a href="[^"]*/packages/(([^"#]+))(?:#[^"]*)?[^>]*>([^<]+)</a>`)
	matches := re.FindAllStringSubmatch(html, -1)
	var artifacts []*runtime.Artifact
	for _, m := range matches {
		fullPath := m[1]                    // e.g. "62/35/.../requests-0.10.0.tar.gz"
		filename := filepath.Base(fullPath) // e.g. "requests-0.10.0.tar.gz"
		if !isValidPyPIFilename(filename) {
			continue
		}
		version := p.extractVersionFromFilename(filename)
		dir := filepath.Dir(fullPath) // e.g. "62/35/0230421b8c4efad6624518028163329ad0c2df9e58e6b3bee013427bf8f6"
		artifacts = append(artifacts, &runtime.Artifact{
			Format: "pypi",
			Kind:   "package-file",
			Coordinates: map[string]string{
				"name":     packageName,
				"package":  packageName,
				"version":  version,
				"filename": filename,
				"path":     "packages/" + dir,
			},
			Properties: map[string]string{
				"remote_path": fullPath,
			},
		})
	}
	return artifacts, nil
}
