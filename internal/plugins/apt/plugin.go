package apt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/moonlight-box/registry/internal/core/runtime"
)

type AptPlugin struct{}

func NewAptPlugin() *AptPlugin {
	return &AptPlugin{}
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

	return errors.New("invalid apt path")
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

	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.Writer, "Artifact: %s", artifact.ID)
	return nil
}

func (p *AptPlugin) handlePackages(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
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

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.Writer, "Artifact: %s", artifact.ID)
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
		ctx.Writer.Header().Set("Content-Type", "application/vnd.debian.binary-package")
		ctx.Writer.WriteHeader(http.StatusOK)
		fmt.Fprintf(ctx.Writer, "Artifact: %s", artifact.ID)
		return nil
	}
	return errors.New("method not allowed")
}
