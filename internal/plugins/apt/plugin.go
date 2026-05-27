package apt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/cache"
	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

type AptPlugin struct {
	cache      *cache.MemoryCache
	httpClient *http.Client
}

func NewAptPlugin() *AptPlugin {
	return &AptPlugin{
		cache:      cache.NewMemoryCache(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetHTTPClient allows injecting a shared HTTP client (with DNS mapping, TLS config, etc.)
func (p *AptPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// FetchRemote implements the RemoteFetcher interface.
// Runtime calls this when local cache is empty; Plugin handles remote APT protocol interaction.
// It fetches Packages index or InRelease/Release files from the remote repository.
func (p *AptPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, errors.New("apt: empty path")
	}

	// For Packages index requests, fetch and parse the Packages file.
	if p.isPackagesRequest(path) {
		return p.fetchPackagesIndex(ctx, remoteURL, path)
	}

	// For InRelease/Release requests, return a basic artifact indicating the remote resource exists.
	if p.isInReleaseRequest(path) {
		filename := filepath.Base(path)
		return []*runtime.Artifact{
			{
				Format: "apt",
				Kind:   "release",
				Coordinates: map[string]string{
					"file": filename,
				},
				Properties: map[string]string{
					"filename": filename,
				},
			},
		}, nil
	}

	// For .deb package requests, return a basic artifact indicating the remote resource exists.
	filename := filepath.Base(path)
	return []*runtime.Artifact{
		{
			Format: "apt",
			Kind:   "package",
			Coordinates: map[string]string{
				"filename": filename,
			},
			Properties: map[string]string{
				"filename": filename,
			},
		},
	}, nil
}

// fetchPackagesIndex fetches a Packages index file from the remote repository
// and parses it to extract package entries.
func (p *AptPlugin) fetchPackagesIndex(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("apt: create request for packages index: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apt: fetch packages index from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apt: fetch packages index from %s: status %d", fullURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("apt: read packages index body: %w", err)
	}

	return p.parsePackagesIndex(string(body)), nil
}

// parsePackagesIndex parses a Debian Packages index file and extracts package entries.
func (p *AptPlugin) parsePackagesIndex(content string) []*runtime.Artifact {
	var artifacts []*runtime.Artifact
	var currentPkg map[string]string

	finishPkg := func() {
		if currentPkg == nil {
			return
		}
		pkgName := currentPkg["Package"]
		version := currentPkg["Version"]
		filename := currentPkg["Filename"]
		if pkgName != "" && version != "" {
			artifact := &runtime.Artifact{
				Format: "apt",
				Kind:   "package",
				Coordinates: map[string]string{
					"package":  pkgName,
					"name":     pkgName,
					"version":  version,
					"filename": filepath.Base(filename),
				},
				Properties: map[string]string{
					"filename":    filepath.Base(filename),
					"remote_path": filename,
				},
			}
			artifacts = append(artifacts, artifact)
		}
		currentPkg = nil
	}

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			finishPkg()
			continue
		}
		if strings.Contains(line, ": ") {
			parts := strings.SplitN(line, ": ", 2)
			key := parts[0]
			value := parts[1]
			if currentPkg == nil {
				currentPkg = make(map[string]string)
			}
			currentPkg[key] = value
		}
	}
	finishPkg()
	return artifacts
}

func (p *AptPlugin) Name() string {
	return "apt"
}

func (p *AptPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if p.isInReleaseRequest(path) {
		return p.handleInRelease(ctx, repoRuntime, path)
	}

	if p.isPackagesRequest(path) {
		return p.handlePackages(ctx, repoRuntime, path)
	}

	if p.isDebPackageRequest(path) {
		return p.handleDebPackage(ctx, repoRuntime, path)
	}

	http.Error(ctx.Writer, "Not found", http.StatusNotFound)
	return nil
}

func (p *AptPlugin) isInReleaseRequest(path string) bool {
	return strings.HasSuffix(path, "InRelease") || strings.HasSuffix(path, "Release") || strings.HasSuffix(path, "Release.gpg")
}

func (p *AptPlugin) isPackagesRequest(path string) bool {
	return strings.Contains(path, "Packages")
}

func (p *AptPlugin) isDebPackageRequest(path string) bool {
	return strings.HasSuffix(path, ".deb")
}

func (p *AptPlugin) handleInRelease(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	filename := filepath.Base(path)
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		Coordinates: map[string]string{
			"file": filename,
		},
		Filename: filename,
	}

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	if artifact.Content == nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	defer artifact.Content.Close()

	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		return err
	}
	return nil
}

func (p *AptPlugin) handlePackages(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	// Try serve stored Packages file first.
	filename := filepath.Base(path)
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		Coordinates: map[string]string{
			"file": filename,
		},
		Filename: filename,
	}

	if artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key); err == nil && artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			return err
		}
		return nil
	}

	// Fallback: render lightweight dynamic Packages index from artifact graph.
	cacheKey := "apt:packages:" + ctx.Repository.ID + ":" + path
	if p.cache != nil {
		if v, ok := p.cache.Get(cacheKey); ok {
			if b, ok := v.([]byte); ok && len(b) > 0 {
				ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
				ctx.Writer.WriteHeader(http.StatusOK)
				_, _ = ctx.Writer.Write(b)
				return nil
			}
		}
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}
	var b strings.Builder
	for _, a := range artifacts {
		name := a.Coordinates["package"]
		if name == "" {
			name = a.Coordinates["name"]
		}
		version := a.Coordinates["version"]
		file := a.Coordinates["filename"]
		if file == "" {
			file = a.Properties["filename"]
		}
		if name == "" || version == "" || file == "" {
			continue
		}
		fmt.Fprintf(&b, "Package: %s\n", name)
		fmt.Fprintf(&b, "Version: %s\n", version)
		fmt.Fprintf(&b, "Filename: %s\n", file)
		if len(a.BlobRefs) > 0 {
			fmt.Fprintf(&b, "Size: %d\n", a.BlobRefs[0].Size)
			if a.BlobRefs[0].Algorithm == "sha256" && a.BlobRefs[0].Digest != "" {
				fmt.Fprintf(&b, "SHA256: %s\n", a.BlobRefs[0].Digest)
			}
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	body := []byte(b.String())
	if p.cache != nil {
		p.cache.Set(cacheKey, body, 5*time.Minute)
	}
	ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write(body)
	return nil
}

func (p *AptPlugin) handleDebPackage(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	filename := filepath.Base(path)

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
		Coordinates: map[string]string{
			"filename": filename,
		},
		Filename: filename,
	}

	switch ctx.Request.Method {
	case http.MethodGet:
		artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err != nil {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		if artifact.Content == nil {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		defer artifact.Content.Close()
		ctx.Writer.Header().Set("Content-Type", "application/vnd.debian.binary-package")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			return err
		}
		return nil
	}
	return errors.New("method not allowed")
}
