package adapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/moonlight-box/registry/internal/types"
)

type MavenPlugin struct{}

func NewMavenPlugin() *MavenPlugin {
	return &MavenPlugin{}
}

func (p *MavenPlugin) Name() string {
	return "maven"
}

func (p *MavenPlugin) Handle(ctx *types.RequestContext, runtime types.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	key, err := p.parseMavenPath(path)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}

	switch ctx.Request.Method {
	case http.MethodGet:
		return p.handleDownload(ctx, runtime, key)
	case http.MethodPut:
		return p.handleUpload(ctx, runtime, key)
	}
	return errors.New("method not allowed")
}

func (p *MavenPlugin) parseMavenPath(path string) (types.ArtifactKey, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 {
		return types.ArtifactKey{}, errors.New("invalid maven path")
	}

	filename := parts[len(parts)-1]
	ext := filepath.Ext(filename)

	var group, artifact, version string
	if strings.Contains(filename, "-") {
		nameParts := strings.Split(strings.TrimSuffix(filename, ext), "-")
		if len(nameParts) >= 3 {
			version = nameParts[len(nameParts)-2]
			artifact = nameParts[len(nameParts)-3]
			groupParts := parts[:len(parts)-3]
			group = strings.Join(groupParts, ".")
		}
	}

	return types.ArtifactKey{
		Format: "maven",
		Coordinates: map[string]string{
			"group":    group,
			"artifact": artifact,
			"version":  version,
		},
		Filename:  filename,
		Extension: ext,
	}, nil
}

func (p *MavenPlugin) handleDownload(ctx *types.RequestContext, runtime types.RepositoryRuntime, key types.ArtifactKey) error {
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

func (p *MavenPlugin) handleUpload(ctx *types.RequestContext, runtime types.RepositoryRuntime, key types.ArtifactKey) error {
	content, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}
	_ = content

	artifact := &types.Artifact{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
		Coordinates:  key.Coordinates,
	}

	session, err := runtime.BeginUpload(context.Background(), types.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
		Filename:     key.Filename,
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
