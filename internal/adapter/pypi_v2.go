package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/moonlight-box/registry/internal/types"
)

type PyPIPlugin struct{}

func NewPyPIPlugin() *PyPIPlugin {
	return &PyPIPlugin{}
}

func (p *PyPIPlugin) Name() string {
	return "pypi"
}

func (p *PyPIPlugin) Handle(ctx *types.RequestContext, runtime types.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if err := validatePyPIPath(path); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}

	if p.isSimpleIndexRequest(path) {
		return p.handleSimpleIndex(ctx, runtime, path)
	}

	if p.isPackageListRequest(path) {
		return p.handlePackageList(ctx, runtime, path)
	}

	if p.isPackagesPath(path) {
		return p.handlePackagesDownload(ctx, runtime, path)
	}

	if p.isJsonAPIRequest(path) {
		return p.handleJsonAPI(ctx, runtime, path)
	}

	return errors.New("invalid pypi path")
}

func (p *PyPIPlugin) isSimpleIndexRequest(path string) bool {
	return path == "simple" || path == "simple/"
}

func (p *PyPIPlugin) isPackageListRequest(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts) == 2 && parts[0] == "simple"
}

func (p *PyPIPlugin) isPackagesPath(path string) bool {
	return strings.HasPrefix(path, "packages/")
}

func (p *PyPIPlugin) isJsonAPIRequest(path string) bool {
	return strings.HasPrefix(path, "pypi/") && strings.HasSuffix(path, "/json")
}

func (p *PyPIPlugin) handleSimpleIndex(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	artifacts, err := runtime.QueryArtifacts(context.Background(), types.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	accept := ctx.Request.Header.Get("Accept")
	if strings.Contains(accept, "application/vnd.pypi.simple") || strings.Contains(accept, "application/json") {
		return p.writeSimpleIndexJSON(ctx, artifacts)
	}
	return p.writeSimpleIndexHTML(ctx, artifacts)
}

func (p *PyPIPlugin) writeSimpleIndexHTML(ctx *types.RequestContext, artifacts []*types.Artifact) error {
	seen := make(map[string]bool)
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html><head><title>Simple Index</title></head><body>\n")
	for _, artifact := range artifacts {
		name := artifact.Coordinates["package"]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		sb.WriteString(`<a href="`)
		sb.WriteString(name)
		sb.WriteString(`/">`)
		sb.WriteString(name)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	html := sb.String()
	ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write([]byte(html))
	return nil
}

func (p *PyPIPlugin) writeSimpleIndexJSON(ctx *types.RequestContext, artifacts []*types.Artifact) error {
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

func (p *PyPIPlugin) handlePackageList(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	packageName := normalizePackageName(parts[1])

	artifacts, err := runtime.QueryArtifacts(context.Background(), types.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	accept := ctx.Request.Header.Get("Accept")
	if strings.Contains(accept, "application/vnd.pypi.simple") || strings.Contains(accept, "application/json") {
		return p.writePackageFilesJSON(ctx, packageName, artifacts)
	}
	return p.writePackageFilesHTML(ctx, packageName, artifacts)
}

func (p *PyPIPlugin) writePackageFilesHTML(ctx *types.RequestContext, packageName string, artifacts []*types.Artifact) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<!DOCTYPE html>\n<html><head><title>Links for %s</title></head><body>\n<h1>Links for %s</h1>\n", packageName, packageName))

	for _, artifact := range artifacts {
		if artifact.Coordinates["package"] != packageName {
			continue
		}
		filename := artifact.Coordinates["filename"]
		if filename == "" {
			continue
		}
		sb.WriteString(`<a href="../../packages/`)
		sb.WriteString(filename)
		sb.WriteString(`">`)
		sb.WriteString(filename)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	html := sb.String()
	ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write([]byte(html))
	return nil
}

func (p *PyPIPlugin) writePackageFilesJSON(ctx *types.RequestContext, packageName string, artifacts []*types.Artifact) error {
	files := make([]map[string]interface{}, 0)
	for _, artifact := range artifacts {
		if artifact.Coordinates["package"] != packageName {
			continue
		}
		filename := artifact.Coordinates["filename"]
		if filename == "" {
			continue
		}

		file := map[string]interface{}{
			"url":      "../../packages/" + filename,
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

func (p *PyPIPlugin) handlePackagesDownload(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	filename := filepath.Base(path)
	filename = strings.TrimPrefix(filename, "packages/")

	if strings.HasSuffix(filename, ".sha256") {
		return p.handleChecksumRequest(ctx, runtime, filename)
	}

	key := types.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Coordinates: map[string]string{
			"filename": filename,
		},
		Filename: filename,
	}

	switch ctx.Request.Method {
	case http.MethodGet:
		return p.handleDownload(ctx, runtime, key)
	case http.MethodPut:
		return p.handleUpload(ctx, runtime, key)
	}
	return errors.New("method not allowed")
}

func (p *PyPIPlugin) handleChecksumRequest(ctx *types.RequestContext, runtime types.RepositoryRuntime, filename string) error {
	actualFilename := strings.TrimSuffix(filename, ".sha256")

	key := types.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Coordinates: map[string]string{
			"filename": actualFilename,
		},
		Filename: actualFilename,
	}

	artifact, err := runtime.GetArtifact(context.Background(), key)
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

func (p *PyPIPlugin) handleJsonAPI(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
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

	artifacts, err := runtime.QueryArtifacts(context.Background(), types.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
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
		if filename == "" {
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
			"name": packageName,
		},
		"releases": releases,
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func (p *PyPIPlugin) handleDownload(ctx *types.RequestContext, runtime types.RepositoryRuntime, key types.ArtifactKey) error {
	artifact, err := runtime.GetArtifact(context.Background(), key)
	if err != nil {
		if errors.Is(err, types.ErrNotFound) {
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

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, key.Filename))
	ctx.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.Writer, "Artifact: %s, Blobs: %d", artifact.ID, len(artifact.BlobRefs))
	return nil
}

func (p *PyPIPlugin) handleUpload(ctx *types.RequestContext, runtime types.RepositoryRuntime, key types.ArtifactKey) error {
	content, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}

	_ = content

	packageName := p.extractPackageNameFromFilename(key.Filename)
	version := p.extractVersionFromFilename(key.Filename)

	artifact := &types.Artifact{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Coordinates: map[string]string{
			"package":  packageName,
			"version":  version,
			"filename": key.Filename,
		},
	}

	session, err := runtime.BeginUpload(context.Background(), types.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "pypi",
		Filename:     key.Filename,
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	if err := session.PutArtifact(context.Background(), artifact); err != nil {
		session.Abort(context.Background())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	if err := session.Commit(context.Background()); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	ctx.Writer.WriteHeader(http.StatusOK)
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
	wheelPattern := regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*-[A-Za-z0-9_.-]+-py[0-9]+-[a-z]+-[a-z0-9_]+(-[A-Za-z0-9_]+)?\.whl$`)
	return wheelPattern.MatchString(filename)
}

func isValidSdistFilename(filename string) bool {
	sdistPattern := regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*-[A-Za-z0-9_.-]+\.(tar\.gz|tar\.bz2|zip)$`)
	return sdistPattern.MatchString(filename)
}

func isValidPyPIFilename(filename string) bool {
	return isValidWheelFilename(filename) || isValidSdistFilename(filename)
}
