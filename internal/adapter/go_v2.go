package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/moonlight-box/registry/internal/types"
)

type GoPlugin struct{}

func NewGoPlugin() *GoPlugin {
	return &GoPlugin{}
}

func (p *GoPlugin) Name() string {
	return "go"
}

func (p *GoPlugin) Handle(ctx *types.RequestContext, runtime types.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "/@latest") {
		return p.handleLatest(ctx, runtime, path)
	}

	if strings.HasSuffix(path, "/@v/list") {
		return p.handleVersionList(ctx, runtime, path)
	}

	if strings.Contains(path, "/@v/") {
		return p.handleModuleDownload(ctx, runtime, path)
	}

	return errors.New("invalid go module path")
}

func (p *GoPlugin) handleLatest(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	modulePath := strings.TrimSuffix(path, "/@latest")

	artifacts, err := runtime.QueryArtifacts(context.Background(), types.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "go",
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

func (p *GoPlugin) handleVersionList(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	modulePath := strings.TrimSuffix(path, "/@v/list")

	artifacts, err := runtime.QueryArtifacts(context.Background(), types.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "go",
		Coordinates: map[string]string{
			"module": modulePath,
		},
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	var sb strings.Builder
	for _, artifact := range artifacts {
		version := artifact.Coordinates["version"]
		if version != "" {
			sb.WriteString(version + "\n")
		}
	}

	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write([]byte(sb.String()))
	return nil
}

func (p *GoPlugin) handleModuleDownload(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	parts := strings.Split(path, "/@v/")
	if len(parts) != 2 {
		http.Error(ctx.Writer, "Invalid path", http.StatusBadRequest)
		return nil
	}

	modulePath := parts[0]
	version := parts[1]

	key := types.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "go",
		Coordinates: map[string]string{
			"module":  modulePath,
			"version": version,
		},
		Filename: version,
	}

	artifact, err := runtime.GetArtifact(context.Background(), key)
	if err != nil {
		if errors.Is(err, types.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.Writer, "Artifact: %s", artifact.ID)
	return nil
}
