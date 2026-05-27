package npm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"golang.org/x/mod/semver"
)

type NpmPlugin struct {
	httpClient *http.Client // 统一 HTTP 客户端，可注入
}

func NewNpmPlugin() *NpmPlugin {
	return &NpmPlugin{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetHTTPClient 允许注入统一的 HTTP 客户端（含 DNS 映射、TLS 配置等）
func (p *NpmPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

func (p *NpmPlugin) Name() string {
	return "npm"
}

// FetchRemote 实现 RemoteFetcher 接口——Runtime 回调, 负责远端 npm registry 协议交互
func (p *NpmPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	packageName := strings.TrimPrefix(path, "/")
	if packageName == "" {
		return nil, errors.New("npm: empty package path")
	}
	// scoped package 需要 URL 编码: @scope/pkg → %40scope%2Fpkg
	encodedName := url.PathEscape(packageName)
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + encodedName
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return p.parseNpmMetadata(packageName, resp.Body)
}

// parseNpmMetadata 解析 npm registry JSON, 提取版本列表为 artifact
func (p *NpmPlugin) parseNpmMetadata(packageName string, body io.Reader) ([]*runtime.Artifact, error) {
	var raw map[string]interface{}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}
	versions, _ := raw["versions"].(map[string]interface{})
	if versions == nil {
		return nil, nil
	}
	var artifacts []*runtime.Artifact
	for version := range versions {
		artifacts = append(artifacts, &runtime.Artifact{
			Format: "npm",
			Kind:   "version",
			Coordinates: map[string]string{
				"name":    packageName,
				"version": version,
			},
		})
	}
	return artifacts, nil
}

func (p *NpmPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "/-all") || path == "-/all" {
		return p.handleAllPackages(ctx, repoRuntime)
	}

	if strings.HasPrefix(path, "-/npm/") {
		return p.handleNpmInternal(ctx, repoRuntime, path)
	}

	if strings.Contains(path, "/-/") {
		return p.handleTarballDownload(ctx, repoRuntime, path)
	}

	return p.handlePackage(ctx, repoRuntime, path)
}

func (p *NpmPlugin) handleAllPackages(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	packages := make(map[string]interface{})
	for _, artifact := range artifacts {
		name := artifact.Coordinates["name"]
		if name == "" {
			continue
		}
		if _, ok := packages[name]; !ok {
			packages[name] = map[string]interface{}{
				"name": name,
			}
		}
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(packages)
	return nil
}

func (p *NpmPlugin) handleNpmInternal(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodPost {
		return errors.New("method not allowed")
	}

	normalized := strings.TrimPrefix(path, "-/npm/")

	// npm registry heartbeat endpoint used by npm CLI.
	if normalized == "ping" {
		ctx.Writer.Header().Set("Content-Type", "application/json")
		ctx.Writer.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(ctx.Writer).Encode(map[string]interface{}{
			"ok":   true,
			"pong": time.Now().UTC().Format(time.RFC3339),
		})
		return nil
	}

	// npm audit endpoints: return empty advisory set to keep client flow working.
	if normalized == "v1/security/advisories/bulk" || normalized == "v1/security/audits/quick" {
		_, _ = io.Copy(io.Discard, ctx.Request.Body)
		ctx.Writer.Header().Set("Content-Type", "application/json")
		ctx.Writer.WriteHeader(http.StatusOK)
		_, _ = ctx.Writer.Write([]byte(`{}`))
		return nil
	}

	http.Error(ctx.Writer, "Not found", http.StatusNotFound)
	return nil
}

func (p *NpmPlugin) handleTarballDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	parts := strings.Split(path, "/-/")
	if len(parts) != 2 {
		http.Error(ctx.Writer, "Invalid path", http.StatusBadRequest)
		return nil
	}

	packageName := parts[0]
	filename := parts[1]
	version := strings.TrimSuffix(strings.TrimPrefix(filename, packageName+"-"), ".tgz")

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Coordinates: map[string]string{
			"name":    packageName,
			"version": version,
			"path":    packageName + "/-",
		},
		Filename: filename,
	}

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrBlocked) {
			return err
		}
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}
	if artifact.Content == nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	defer artifact.Content.Close()

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		return err
	}
	return nil
}

func (p *NpmPlugin) handlePackage(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	packageName := strings.TrimSuffix(path, "/")

	switch ctx.Request.Method {
	case http.MethodGet:
		return p.handlePackageGet(ctx, repoRuntime, packageName)
	case http.MethodPut:
		return p.handlePackagePut(ctx, repoRuntime, packageName)
	}
	return errors.New("method not allowed")
}

func (p *NpmPlugin) handleTarballDelete(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	parts := strings.Split(path, "/-/")
	if len(parts) != 2 {
		http.Error(ctx.Writer, "Invalid path", http.StatusBadRequest)
		return nil
	}
	packageName := parts[0]
	filename := parts[1]
	version := strings.TrimSuffix(strings.TrimPrefix(filename, packageName+"-"), ".tgz")
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Coordinates: map[string]string{
			"name":    packageName,
			"version": version,
			"path":    packageName + "/-",
		},
		Filename: filename,
	}
	return deleteArtifact(ctx, repoRuntime, key)
}

func (p *NpmPlugin) handlePackageGet(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Coordinates: map[string]string{
			"name": packageName,
		},
		RemotePath: packageName,
	})
	if err != nil {
		if errors.Is(err, runtime.ErrBlocked) {
			return err
		}
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	hasVersions := false
	for _, a := range artifacts {
		if a.Coordinates["version"] != "" {
			hasVersions = true
			break
		}
	}
	if !hasVersions {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	repoBase := repoBaseURL(ctx.Request, ctx.Repository.Name)

	versions := make(map[string]interface{})
	var versionList []string
	for _, artifact := range artifacts {
		version := artifact.Coordinates["version"]
		if version == "" {
			continue
		}
		if _, exists := versions[version]; exists {
			continue
		}
		tarballName := packageName + "-" + version + ".tgz"
		versions[version] = map[string]interface{}{
			"name":    packageName,
			"version": version,
			"dist": map[string]interface{}{
				"tarball": repoBase + "/" + packageName + "/-/" + tarballName,
			},
		}
		versionList = append(versionList, version)
	}

	// Compute dist-tags.
	distTags := map[string]string{}
	if len(versionList) > 0 {
		sort.Slice(versionList, func(i, j int) bool {
			return semver.Compare(versionList[i], versionList[j]) > 0
		})
		distTags["latest"] = versionList[0]
	}

	data := map[string]interface{}{
		"name":      packageName,
		"dist-tags": distTags,
		"versions":  versions,
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

// repoBaseURL 构造仓库的基础 URL，支持反向代理 (X-Forwarded-* 头)
func repoBaseURL(r *http.Request, repoName string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return fmt.Sprintf("%s://%s/repository/%s", scheme, host, repoName)
}

func (p *NpmPlugin) handlePackageDelete(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Coordinates: map[string]string{
			"name": packageName,
		},
	}
	return deleteArtifact(ctx, repoRuntime, key)
}

func deleteArtifact(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	err := repoRuntime.DeleteArtifact(ctx.Request.Context(), key)
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrNotFound):
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		case errors.Is(err, runtime.ErrReadOnly):
			http.Error(ctx.Writer, "Repository is read only", http.StatusMethodNotAllowed)
		default:
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}
	ctx.Writer.WriteHeader(http.StatusNoContent)
	return nil
}

func (p *NpmPlugin) handlePackagePut(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	var npmMeta map[string]interface{}
	if err := json.NewDecoder(ctx.Request.Body).Decode(&npmMeta); err != nil {
		http.Error(ctx.Writer, "invalid npm metadata: "+err.Error(), http.StatusBadRequest)
		return nil
	}
	version := ""
	if v, ok := npmMeta["version"].(string); ok {
		version = v
	}

	// 提取 _attachments 中的 tarball 数据
	attachments, _ := npmMeta["_attachments"].(map[string]interface{})
	delete(npmMeta, "_attachments") // 存储 metadata 时移除 attachments 数据

	metadataJSON, _ := json.Marshal(npmMeta)
	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Filename:     packageName,
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	// 存储 tarball blob
	for tarballName, att := range attachments {
		attMap, ok := att.(map[string]interface{})
		if !ok {
			continue
		}
		data, _ := attMap["data"].(string)
		if data == "" {
			continue
		}
		tarballBytes, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			session.Abort(ctx.Request.Context())
			http.Error(ctx.Writer, "invalid tarball base64: "+err.Error(), http.StatusBadRequest)
			return nil
		}

		tarballBlob, err := session.PutBlob(ctx.Request.Context(), strings.NewReader(string(tarballBytes)))
		if err != nil {
			session.Abort(ctx.Request.Context())
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
			return nil
		}

		tarballVersion := strings.TrimSuffix(strings.TrimPrefix(tarballName, packageName+"-"), ".tgz")
		tarballArtifact := &runtime.Artifact{
			RepositoryID: ctx.Repository.ID,
			Format:       "npm",
			Kind:         "tarball",
			Coordinates: map[string]string{
				"name":    packageName,
				"version": tarballVersion,
				"path":    packageName + "/-",
			},
			BlobRefs: []runtime.BlobRef{tarballBlob},
			Properties: map[string]string{
				"package":  packageName,
				"version":  tarballVersion,
				"filename": tarballName,
			},
		}
		if err := session.PutArtifact(ctx.Request.Context(), tarballArtifact); err != nil {
			session.Abort(ctx.Request.Context())
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
			return nil
		}
	}

	// 存储 metadata blob
	blob, err := session.PutBlob(ctx.Request.Context(), strings.NewReader(string(metadataJSON)))
	if err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	artifact := &runtime.Artifact{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Kind:         "metadata",
		Coordinates: map[string]string{
			"name":    packageName,
			"version": version,
		},
		BlobRefs: []runtime.BlobRef{blob},
		Properties: map[string]string{
			"package": packageName,
			"version": version,
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
