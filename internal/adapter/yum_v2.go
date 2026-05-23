package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/moonlight-box/registry/internal/types"
)

type YumPlugin struct{}

func NewYumPlugin() *YumPlugin {
	return &YumPlugin{}
}

func (p *YumPlugin) Name() string {
	return "yum"
}

func (p *YumPlugin) Handle(ctx *types.RequestContext, runtime types.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if p.isRepomdRequest(path) {
		return p.handleRepomd(ctx, runtime, path)
	}

	if p.isPrimaryRequest(path) {
		return p.handlePrimary(ctx, runtime, path)
	}

	if p.isRpmPackageRequest(path) {
		return p.handleRpmPackage(ctx, runtime, path)
	}

	return errors.New("invalid yum path")
}

func (p *YumPlugin) isRepomdRequest(path string) bool {
	return strings.HasSuffix(path, "repodata/repomd.xml")
}

func (p *YumPlugin) isPrimaryRequest(path string) bool {
	return strings.Contains(path, "repodata/") && strings.Contains(path, "primary.xml")
}

func (p *YumPlugin) isRpmPackageRequest(path string) bool {
	return strings.HasSuffix(path, ".rpm")
}

func (p *YumPlugin) handleRepomd(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	key := types.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Coordinates: map[string]string{
			"file": "repomd.xml",
		},
		Filename: "repomd.xml",
	}

	artifact, err := runtime.GetArtifact(context.Background(), key)
	if err != nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	ctx.Writer.Header().Set("Content-Type", "application/xml")
	ctx.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.Writer, "Artifact: %s", artifact.ID)
	return nil
}

func (p *YumPlugin) handlePrimary(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	filename := filepath.Base(path)
	key := types.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Coordinates: map[string]string{
			"file": filename,
		},
		Filename: filename,
	}

	artifact, err := runtime.GetArtifact(context.Background(), key)
	if err != nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	ctx.Writer.Header().Set("Content-Type", "application/xml")
	ctx.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.Writer, "Artifact: %s", artifact.ID)
	return nil
}

func (p *YumPlugin) handleRpmPackage(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	filename := filepath.Base(path)

	key := types.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Coordinates: map[string]string{
			"filename": filename,
		},
		Filename: filename,
	}

	switch ctx.Request.Method {
	case http.MethodGet:
		artifact, err := runtime.GetArtifact(context.Background(), key)
		if err != nil {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		ctx.Writer.Header().Set("Content-Type", "application/x-rpm")
		ctx.Writer.WriteHeader(http.StatusOK)
		fmt.Fprintf(ctx.Writer, "Artifact: %s", artifact.ID)
		return nil
	}
	return errors.New("method not allowed")
}
