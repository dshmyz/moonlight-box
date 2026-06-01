package gomod

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
)

type GoPlugin struct {
	httpClient *http.Client
}

func NewGoPlugin() *GoPlugin {
	return &GoPlugin{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetHTTPClient allows injecting a shared HTTP client (with DNS mapping, TLS config, etc.)
func (p *GoPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// FetchRemote implements the RemoteFetcher interface.
// Runtime calls this when local cache is empty; Plugin handles remote Go protocol interaction.
func (p *GoPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, errors.New("go: empty module path")
	}

	// Determine the remote fetch strategy based on the path suffix.
	if strings.HasSuffix(path, "/@v/list") {
		return p.fetchVersionList(ctx, remoteURL, path)
	}
	if strings.HasSuffix(path, "/@latest") {
		return p.fetchLatest(ctx, remoteURL, path)
	}
	// For other paths (e.g. /@v/*.info, *.mod, *.zip), return a basic artifact indicating the resource exists.
	modulePath, filename := p.splitModulePath(path)
	return []*runtime.Artifact{
		{
			Format: "go",
			Kind:   "module-file",
			Coordinates: map[string]string{
				"module":   modulePath,
				"path":     path,
				"filename": filename,
			},
		},
	}, nil
}

// fetchVersionList fetches the @v/list endpoint from a Go module proxy
// and parses the line-separated version list.
func (p *GoPlugin) fetchVersionList(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	modulePath := strings.TrimSuffix(path, "/@v/list")
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + encodeGoPath(path)

	logrus.WithFields(logrus.Fields{
		"remoteURL":  remoteURL,
		"path":       path,
		"modulePath": modulePath,
		"fullURL":    fullURL,
	}).Debug("go: fetchVersionList called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).Error("go: create request for version list failed")
		return nil, fmt.Errorf("go: create request for version list: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL": fullURL,
			"error":   err.Error(),
		}).Error("go: fetch version list HTTP request failed")
		return nil, fmt.Errorf("go: fetch version list from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logrus.WithFields(logrus.Fields{
			"fullURL":    fullURL,
			"statusCode": resp.StatusCode,
		}).Error("go: fetch version list returned non-200 status")
		return nil, fmt.Errorf("go: fetch version list from %s: status %d", fullURL, resp.StatusCode)
	}

	var artifacts []*runtime.Artifact
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		version := strings.TrimSpace(scanner.Text())
		if version == "" {
			continue
		}
		artifacts = append(artifacts, &runtime.Artifact{
			Format: "go",
			Kind:   "version",
			Coordinates: map[string]string{
				"module":  modulePath,
				"version": version,
			},
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("go: scan version list: %w", err)
	}
	logrus.WithFields(logrus.Fields{
		"modulePath":   modulePath,
		"versionCount": len(artifacts),
	}).Debug("go: fetchVersionList success")

	const maxInfoFetches = 10
	infoStart := len(artifacts) - maxInfoFetches
	if infoStart < 0 {
		infoStart = 0
	}

	var wg sync.WaitGroup
	for _, a := range artifacts[infoStart:] {
		wg.Add(1)
		go func(art *runtime.Artifact) {
			defer wg.Done()
			info, err := p.fetchVersionInfo(ctx, remoteURL, art.Coordinates["module"], art.Coordinates["version"])
			if err == nil && info.Time != "" {
				art.Properties = map[string]string{"published_at": info.Time}
			}
		}(a)
	}
	wg.Wait()

	return artifacts, nil
}

func (p *GoPlugin) fetchVersionInfo(ctx context.Context, remoteURL, modulePath, version string) (struct {
	Version string
	Time    string
}, error) {
	infoPath := modulePath + "/@v/" + version + ".info"
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + encodeGoPath(infoPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return struct {
			Version string
			Time    string
		}{}, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return struct {
			Version string
			Time    string
		}{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return struct {
			Version string
			Time    string
		}{}, fmt.Errorf("go: fetch version info from %s: status %d", fullURL, resp.StatusCode)
	}

	var result struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return struct {
			Version string
			Time    string
		}{}, err
	}
	return struct {
		Version string
		Time    string
	}{Version: result.Version, Time: result.Time}, nil
}

// fetchLatest fetches the @latest endpoint from a Go module proxy
// and parses the JSON response.
func (p *GoPlugin) fetchLatest(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	modulePath := strings.TrimSuffix(path, "/@latest")
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + encodeGoPath(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("go: create request for @latest: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("go: fetch @latest from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("go: fetch @latest from %s: status %d", fullURL, resp.StatusCode)
	}

	var result struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("go: decode @latest response: %w", err)
	}
	if result.Version == "" {
		return nil, nil
	}
	return []*runtime.Artifact{
		{
			Format: "go",
			Kind:   "version",
			Coordinates: map[string]string{
				"module":  modulePath,
				"version": result.Version,
			},
			Properties: map[string]string{
				"time": result.Time,
			},
		},
	}, nil
}

// splitModulePath splits a Go module path like "github.com/foo/bar/@v/v1.0.0.zip"
// into the module path and the filename portion.
func (p *GoPlugin) splitModulePath(path string) (modulePath, filename string) {
	if idx := strings.Index(path, "/@v/"); idx >= 0 {
		return path[:idx], path[idx+4:]
	}
	if idx := strings.Index(path, "/@latest"); idx >= 0 {
		return path[:idx], "@latest"
	}
	return path, ""
}

// encodeGoPath encodes uppercase letters per Go module proxy spec:
// A → !a, B → !b, ..., Z → !z. Required for fetching from upstream proxies
// like proxy.golang.org when module paths contain uppercase letters.
func encodeGoPath(path string) string {
	var b strings.Builder
	b.Grow(len(path) + 8)
	for _, r := range path {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			b.WriteByte(byte(r) + 32) // to lowercase
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (p *GoPlugin) Name() string {
	return "go"
}

func (p *GoPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "/@latest") {
		return p.handleLatest(ctx, repoRuntime, path)
	}

	if strings.HasSuffix(path, "/@v/list") {
		return p.handleVersionList(ctx, repoRuntime, path)
	}

	if strings.Contains(path, "/@v/") {
		return p.handleModuleDownload(ctx, repoRuntime, path)
	}

	return errors.New("invalid go module path")
}

func (p *GoPlugin) handleLatest(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	modulePath := strings.TrimSuffix(path, "/@latest")

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "go",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
		Coordinates: map[string]string{
			"module": modulePath,
		},
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	if len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	latest := artifacts[0]
	for _, a := range artifacts {
		if a.CreatedAt.After(latest.CreatedAt) {
			latest = a
		}
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.Writer, `{"Version":"%s"}`, latest.Coordinates["version"])
	return nil
}

func (p *GoPlugin) handleVersionList(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	modulePath := strings.TrimSuffix(path, "/@v/list")

	logrus.WithFields(logrus.Fields{
		"repository":   ctx.Repository.Name,
		"repositoryID": ctx.Repository.ID,
		"path":         path,
		"modulePath":   modulePath,
		"repoType":     ctx.Repository.Type,
	}).Debug("go: handleVersionList called")

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "go",
		RemotePath:   path,
		Coordinates: map[string]string{
			"module": modulePath,
		},
	})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"modulePath": modulePath,
			"error":      err.Error(),
		}).Error("go: QueryArtifacts failed in handleVersionList")
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"modulePath":    modulePath,
		"artifactCount": len(artifacts),
	}).Debug("go: handleVersionList got artifacts")

	var sb strings.Builder
	seen := make(map[string]bool)
	for _, artifact := range artifacts {
		version := artifact.Coordinates["version"]
		if version != "" && !seen[version] {
			seen[version] = true
			sb.WriteString(version + "\n")
		}
	}

	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write([]byte(sb.String()))
	return nil
}

func (p *GoPlugin) handleModuleDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	parts := strings.Split(path, "/@v/")
	if len(parts) != 2 {
		http.Error(ctx.Writer, "Invalid path", http.StatusBadRequest)
		return nil
	}

	modulePath := parts[0]
	filename := parts[1]

	// Determine file type and clean version.
	var fileType string
	cleanVersion := filename
	switch {
	case strings.HasSuffix(filename, ".info"):
		fileType = "info"
		cleanVersion = strings.TrimSuffix(filename, ".info")
	case strings.HasSuffix(filename, ".mod"):
		fileType = "mod"
		cleanVersion = strings.TrimSuffix(filename, ".mod")
	case strings.HasSuffix(filename, ".zip"):
		fileType = "zip"
		cleanVersion = strings.TrimSuffix(filename, ".zip")
	}

	// For .info, generate JSON dynamically so the version is always a valid semver.
	if fileType == "info" {
		ctx.Writer.Header().Set("Content-Type", "application/json")
		ctx.Writer.WriteHeader(http.StatusOK)
		fmt.Fprintf(ctx.Writer, `{"Version":"%s","Time":"%s"}`, cleanVersion, time.Now().UTC().Format(time.RFC3339))
		return nil
	}

	// For .mod and .zip, fetch and stream the stored blob.
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "go",
		Coordinates: map[string]string{
			"name":     modulePath,
			"module":   modulePath,
			"version":  cleanVersion,
			"path":     modulePath + "/@v",
			"ext":      fileType,
			"filename": filename,
		},
		Filename: filename,
	}

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}
	defer artifact.Content.Close()

	ctx.FromCache = artifact.FromCache
	ctx.RemoteURL = artifact.RemoteURL
	ctx.SizeBytes = artifact.SizeBytes

	switch fileType {
	case "mod":
		ctx.Writer.Header().Set("Content-Type", "text/plain")
	case "zip":
		ctx.Writer.Header().Set("Content-Type", "application/zip")
	}
	ctx.Writer.WriteHeader(http.StatusOK)
	io.Copy(ctx.Writer, artifact.Content)
	return nil
}
