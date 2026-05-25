package gomod

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

type GoPlugin struct{}

func NewGoPlugin() *GoPlugin {
	return &GoPlugin{}
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

	artifacts, err := repoRuntime.QueryArtifacts(context.Background(), runtime.ArtifactQuery{
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

func (p *GoPlugin) handleVersionList(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	modulePath := strings.TrimSuffix(path, "/@v/list")

	artifacts, err := repoRuntime.QueryArtifacts(context.Background(), runtime.ArtifactQuery{
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
			"name":    modulePath,
			"module":  modulePath,
			"version": cleanVersion,
			"path":    modulePath + "/@v",
			"ext":     fileType,
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
	defer artifact.Content.Close()

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
