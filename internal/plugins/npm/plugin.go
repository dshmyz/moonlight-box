package npm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/moonlight-box/registry/internal/core/runtime"
)

type NpmPlugin struct{}

func NewNpmPlugin() *NpmPlugin {
	return &NpmPlugin{}
}

func (p *NpmPlugin) Name() string {
	return "npm"
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

	artifacts, err := repoRuntime.QueryArtifacts(context.Background(), runtime.ArtifactQuery{
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
	http.Error(ctx.Writer, "Not implemented", http.StatusNotImplemented)
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

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Coordinates: map[string]string{
			"name": packageName,
		},
		Filename: filename,
	}

	artifact, err := repoRuntime.GetArtifact(context.Background(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
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

func (p *NpmPlugin) handlePackageGet(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	artifacts, err := repoRuntime.QueryArtifacts(context.Background(), runtime.ArtifactQuery{
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

func (p *NpmPlugin) handlePackagePut(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, packageName string) error {
	content, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}
	_ = content

	artifact := &runtime.Artifact{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
		Coordinates: map[string]string{
			"name": packageName,
		},
	}

	session, err := repoRuntime.BeginUpload(context.Background(), runtime.UploadRequest{
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
