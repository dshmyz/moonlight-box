package raw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
)

type GenericPlugin struct {
	httpClient *http.Client
}

func NewGenericPlugin() *GenericPlugin {
	return &GenericPlugin{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetHTTPClient allows injecting a shared HTTP client (with DNS mapping, TLS config, etc.)
func (p *GenericPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// FetchRemote implements the RemoteFetcher interface.
// Runtime calls this when local cache is empty; Plugin handles remote generic/raw protocol interaction.
// It performs a simple directory listing or file fetch from the remote repository.
func (p *GenericPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, errors.New("generic: empty path")
	}

	fullURL := strings.TrimRight(remoteURL, "/") + "/" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("generic: create request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("generic: fetch from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("generic: fetch from %s: status %d", fullURL, resp.StatusCode)
	}

	// If the response is HTML (directory listing), attempt to parse links.
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		return p.parseDirectoryListing(path, resp.Body)
	}

	// For non-HTML responses (direct file download), return a single artifact.
	filename := filepath.Base(path)
	dir := filepath.Dir(path)
	if dir == "." {
		dir = ""
	}
	return []*runtime.Artifact{
		{
			Format: "generic",
			Kind:   "file",
			Coordinates: map[string]string{
				"name": filename,
				"path": dir,
			},
			Properties: map[string]string{
				"filename": filename,
			},
		},
	}, nil
}

// parseDirectoryListing attempts to extract file links from an HTML directory listing.
func (p *GenericPlugin) parseDirectoryListing(basePath string, body io.Reader) ([]*runtime.Artifact, error) {
	htmlBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("generic: read directory listing: %w", err)
	}
	html := string(htmlBytes)

	// Match href attributes in anchor tags, a common pattern for directory listings.
	// This handles both auto-index pages (nginx, apache) and simple HTML listings.
	var artifacts []*runtime.Artifact
	seen := make(map[string]bool)
	// Simple regex-free approach: find href="..." patterns in <a> tags.
	for _, segment := range strings.Split(html, `href="`) {
		if len(segment) == 0 {
			continue
		}
		endIdx := strings.Index(segment, `"`)
		if endIdx <= 0 {
			continue
		}
		href := segment[:endIdx]
		// Skip parent directory links, query strings, and absolute URLs.
		if href == "../" || href == "/" || strings.HasPrefix(href, "?") || strings.HasPrefix(href, "http") {
			continue
		}
		name := strings.TrimRight(href, "/")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		isDir := strings.HasSuffix(href, "/")
		kind := "file"
		if isDir {
			kind = "directory"
		}
		dir := filepath.Dir(basePath)
		if dir == "." {
			dir = ""
		}
		artifacts = append(artifacts, &runtime.Artifact{
			Format: "generic",
			Kind:   kind,
			Coordinates: map[string]string{
				"name": name,
				"path": dir,
			},
			Properties: map[string]string{
				"filename": name,
			},
		})
	}
	return artifacts, nil
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
			"name": filename,
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
	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
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
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		return err
	}
	return nil
}

func (p *GenericPlugin) handleUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "generic",
		Filename:     key.Filename,
		Size:         ctx.Request.ContentLength,
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	blobRef, err := session.PutBlob(ctx.Request.Context(), ctx.Request.Body)
	if err != nil {
		session.Abort(ctx.Request.Context())
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

	if err := session.PutArtifact(ctx.Request.Context(), artifact); err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	if err := session.Commit(ctx.Request.Context()); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	ctx.Writer.WriteHeader(http.StatusCreated)
	return nil
}

func (p *GenericPlugin) handleDelete(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	err := repoRuntime.DeleteArtifact(ctx.Request.Context(), key)
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
