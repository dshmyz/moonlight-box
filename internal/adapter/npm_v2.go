package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/moonlight-box/registry/internal/types"
)

type NpmPlugin struct{}

func NewNpmPlugin() *NpmPlugin {
	return &NpmPlugin{}
}

func (p *NpmPlugin) Name() string {
	return "npm"
}

func (p *NpmPlugin) Handle(ctx *types.RequestContext, runtime types.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "/-all") || path == "-/all" {
		return p.handleAllPackages(ctx, runtime)
	}

	if strings.HasPrefix(path, "-/npm/") {
		return p.handleNpmInternal(ctx, runtime, path)
	}

	if strings.Contains(path, "/-/") {
		return p.handleTarballDownload(ctx, runtime, path)
	}

	return p.handlePackage(ctx, runtime, path)
}

func (p *NpmPlugin) handleAllPackages(ctx *types.RequestContext, runtime types.RepositoryRuntime) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	artifacts, err := runtime.QueryArtifacts(context.Background(), types.ArtifactQuery{
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

func (p *NpmPlugin) handleNpmInternal(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	http.Error(ctx.Writer, "Not implemented", http.StatusNotImplemented)
	return nil
}

func (p *NpmPlugin) handleTarballDownload(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	parts := strings.Split(path, "/-/")
	if len(parts) != 2 {
		http.Error(ctx.Writer, "Invalid path", http.StatusBadRequest)
		return nil
	}

	packageName := parts[0]
	filename := parts[1]

	key := types.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Coordinates: map[string]string{
			"name": packageName,
		},
		Filename: filename,
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

func (p *NpmPlugin) handlePackage(ctx *types.RequestContext, runtime types.RepositoryRuntime, path string) error {
	packageName := strings.TrimSuffix(path, "/")

	switch ctx.Request.Method {
	case http.MethodGet:
		return p.handlePackageGet(ctx, runtime, packageName)
	case http.MethodPut:
		return p.handlePackagePut(ctx, runtime, packageName)
	}
	return errors.New("method not allowed")
}

func (p *NpmPlugin) handlePackageGet(ctx *types.RequestContext, runtime types.RepositoryRuntime, packageName string) error {
	artifacts, err := runtime.QueryArtifacts(context.Background(), types.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Coordinates: map[string]string{
			"name": packageName,
		},
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	versions := make(map[string]interface{})
	for _, artifact := range artifacts {
		version := artifact.Coordinates["version"]
		if version == "" {
			continue
		}
		versions[version] = map[string]interface{}{
			"id": artifact.ID,
		}
	}

	data := map[string]interface{}{
		"name":     packageName,
		"versions": versions,
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(data)
	return nil
}

func (p *NpmPlugin) handlePackagePut(ctx *types.RequestContext, runtime types.RepositoryRuntime, packageName string) error {
	content, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}
	_ = content

	artifact := &types.Artifact{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Coordinates: map[string]string{
			"name": packageName,
		},
	}

	session, err := runtime.BeginUpload(context.Background(), types.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Filename:     packageName,
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

	ctx.Writer.WriteHeader(http.StatusCreated)
	return nil
}
