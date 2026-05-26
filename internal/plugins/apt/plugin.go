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
	cache *cache.MemoryCache
}

func NewAptPlugin() *AptPlugin {
	return &AptPlugin{cache: cache.NewMemoryCache()}
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

	artifact, err := repoRuntime.GetArtifact(context.Background(), key)
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
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+key.Filename+"\"")
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

	if artifact, err := repoRuntime.GetArtifact(context.Background(), key); err == nil && artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+key.Filename+"\"")
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

	artifacts, err := repoRuntime.QueryArtifacts(context.Background(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "apt",
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
		artifact, err := repoRuntime.GetArtifact(context.Background(), key)
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
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+key.Filename+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			return err
		}
		return nil
	}
	return errors.New("method not allowed")
}
