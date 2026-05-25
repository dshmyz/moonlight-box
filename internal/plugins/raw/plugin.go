package raw

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/moonlight-box/registry/internal/core/runtime"
)

type GenericPlugin struct{}

func NewGenericPlugin() *GenericPlugin {
	return &GenericPlugin{}
}

func (p *GenericPlugin) Name() string {
	return "generic"
}

func (p *GenericPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
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

	key := runtime.ArtifactKey{
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
		return p.handleDownload(ctx, repoRuntime, key)
	case http.MethodPut:
		return p.handleUpload(ctx, repoRuntime, key)
	case http.MethodDelete:
		return p.handleDelete(ctx, repoRuntime, key)
	}
	return errors.New("method not allowed")
}

func (p *GenericPlugin) handleDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	artifact, err := repoRuntime.GetArtifact(context.Background(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}
	if artifact.Content == nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	defer artifact.Content.Close()

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
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+key.Filename+"\"")
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		return err
	}
	return nil
}

func (p *GenericPlugin) handleUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	session, err := repoRuntime.BeginUpload(context.Background(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "generic",
		Filename:     key.Filename,
		Size:         ctx.Request.ContentLength,
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	blobRef, err := session.PutBlob(context.Background(), ctx.Request.Body)
	if err != nil {
		session.Abort(context.Background())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	artifact := &runtime.Artifact{
		RepositoryID: ctx.Repository.ID,
		Format:       "generic",
		Coordinates:  key.Coordinates,
		Kind:         "file",
		BlobRefs:     []runtime.BlobRef{blobRef},
		Properties: map[string]string{
			"filename": key.Filename,
			"path":     key.Coordinates["path"],
		},
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

func (p *GenericPlugin) handleDelete(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	err := repoRuntime.DeleteArtifact(context.Background(), key)
	if err != nil {
		switch {
		case errors.Is(err, runtime.ErrNotFound):
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		case errors.Is(err, runtime.ErrReadOnly):
			http.Error(ctx.Writer, "Repository is read only", http.StatusMethodNotAllowed)
		default:
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
		return nil
	}
	ctx.Writer.WriteHeader(http.StatusNoContent)
	return nil
}
