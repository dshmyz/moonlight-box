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

type GenericPlugin struct{}

func NewGenericPlugin() *GenericPlugin {
	return &GenericPlugin{}
}

func (p *GenericPlugin) Name() string {
	return "generic"
}

func (p *GenericPlugin) Handle(ctx *types.RequestContext, runtime types.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if path == "" {
		return errors.New("empty path")
	}

	filename := filepath.Base(path)
	dir := filepath.Dir(path)
	if dir == "." {
		dir = ""
	}

	key := types.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "generic",
		Coordinates: map[string]string{
			"path": dir,
		},
		Filename:  filename,
		Extension: filepath.Ext(filename),
	}

	switch ctx.Request.Method {
	case http.MethodGet:
		return p.handleDownload(ctx, runtime, key)
	case http.MethodPut:
		return p.handleUpload(ctx, runtime, key)
	case http.MethodDelete:
		return p.handleDelete(ctx, runtime, key)
	}
	return errors.New("method not allowed")
}

func (p *GenericPlugin) handleDownload(ctx *types.RequestContext, runtime types.RepositoryRuntime, key types.ArtifactKey) error {
	artifact, err := runtime.GetArtifact(context.Background(), key)
	if err != nil {
		if errors.Is(err, types.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}

	contentType := "application/octet-stream"
	ext := strings.ToLower(key.Extension)
	switch ext {
	case ".txt":
		contentType = "text/plain"
	case ".json":
		contentType = "application/json"
	case ".xml":
		contentType = "application/xml"
	case ".zip":
		contentType = "application/zip"
	}

	ctx.Writer.Header().Set("Content-Type", contentType)
	ctx.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(ctx.Writer, "Artifact: %s", artifact.ID)
	return nil
}

func (p *GenericPlugin) handleUpload(ctx *types.RequestContext, runtime types.RepositoryRuntime, key types.ArtifactKey) error {
	session, err := runtime.BeginUpload(context.Background(), types.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "generic",
		Filename:     key.Filename,
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	artifact := &types.Artifact{
		RepositoryID: ctx.Repository.ID,
		Format:       "generic",
		Coordinates:  key.Coordinates,
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

func (p *GenericPlugin) handleDelete(ctx *types.RequestContext, runtime types.RepositoryRuntime, key types.ArtifactKey) error {
	http.Error(ctx.Writer, "Delete not implemented", http.StatusNotImplemented)
	return nil
}
