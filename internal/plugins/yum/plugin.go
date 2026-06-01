package yum

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/cache"
	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
)

type YumPlugin struct {
	cache      *cache.MemoryCache
	httpClient *http.Client
}

func NewYumPlugin() *YumPlugin {
	return &YumPlugin{
		cache:      cache.NewMemoryCache(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetHTTPClient allows injecting a shared HTTP client (with DNS mapping, TLS config, etc.)
func (p *YumPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// FetchRemote implements the RemoteFetcher interface.
// Runtime calls this when local cache is empty; Plugin handles remote YUM protocol interaction.
// It fetches repomd.xml or other repository metadata from the remote repository.
func (p *YumPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, errors.New("yum: empty path")
	}

	logrus.WithFields(logrus.Fields{
		"remoteURL": remoteURL,
		"path":      path,
	}).Debug("yum: FetchRemote called")

	// For repomd.xml requests, fetch and parse the XML from remote.
	if strings.HasSuffix(path, "repodata/repomd.xml") {
		return p.fetchRepomd(ctx, remoteURL, path)
	}

	// For other paths (RPM packages, primary.xml, etc.), return a basic artifact indicating the remote resource exists.
	filename := filepath.Base(path)
	logrus.WithFields(logrus.Fields{
		"path":     path,
		"filename": filename,
	}).Debug("yum: FetchRemote returning file reference")
	return []*runtime.Artifact{
		{
			Format: "yum",
			Kind:   "file",
			Coordinates: map[string]string{
				"file":     filename,
				"filename": filename,
				"path":     path,
			},
			Properties: map[string]string{
				"filename": filename,
			},
		},
	}, nil
}

// fetchRepomd fetches repomd.xml from the remote repository and parses data references.
func (p *YumPlugin) fetchRepomd(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	start := time.Now()
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + path

	logrus.WithFields(logrus.Fields{
		"remoteURL": remoteURL,
		"path":      path,
		"fullURL":   fullURL,
	}).Debug("yum: fetchRepomd called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).WithField("fullURL", fullURL).Error("yum: create request for repomd failed")
		return nil, fmt.Errorf("yum: create request for repomd: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("yum: fetch repomd HTTP request failed")
		return nil, fmt.Errorf("yum: fetch repomd from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logrus.WithFields(logrus.Fields{
			"fullURL":    fullURL,
			"statusCode": resp.StatusCode,
			"duration":   time.Since(start).Seconds(),
		}).Error("yum: fetch repomd returned non-200 status")
		return nil, fmt.Errorf("yum: fetch repomd from %s: status %d", fullURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("yum: read repomd body failed")
		return nil, fmt.Errorf("yum: read repomd body: %w", err)
	}

	type repomdData struct {
		Type     string `xml:"type,attr"`
		Location struct {
			Href string `xml:"href,attr"`
		} `xml:"location"`
	}
	type repomdXML struct {
		XMLName xml.Name     `xml:"repomd"`
		Data    []repomdData `xml:"data"`
	}

	var repomd repomdXML
	if err := xml.Unmarshal(body, &repomd); err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("yum: unmarshal repomd XML failed")
		return nil, fmt.Errorf("yum: unmarshal repomd XML: %w", err)
	}

	var artifacts []*runtime.Artifact
	// Add the repomd.xml itself as an artifact.
	artifacts = append(artifacts, &runtime.Artifact{
		Format: "yum",
		Kind:   "metadata",
		Coordinates: map[string]string{
			"file": "repomd.xml",
		},
		Properties: map[string]string{
			"filename": "repomd.xml",
		},
	})
	// Add each data reference as an artifact.
	for _, d := range repomd.Data {
		href := d.Location.Href
		if href == "" {
			continue
		}
		artifacts = append(artifacts, &runtime.Artifact{
			Format: "yum",
			Kind:   "metadata-ref",
			Coordinates: map[string]string{
				"file": filepath.Base(href),
				"type": d.Type,
				"href": href,
			},
			Properties: map[string]string{
				"filename": filepath.Base(href),
			},
		})
	}

	logrus.WithFields(logrus.Fields{
		"fullURL":       fullURL,
		"artifactCount": len(artifacts),
		"duration":      time.Since(start).Seconds(),
	}).Debug("yum: fetchRepomd success")
	return artifacts, nil
}

func (p *YumPlugin) Name() string {
	return "yum"
}

func (p *YumPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	logrus.WithFields(logrus.Fields{
		"originalPath":   ctx.RepositoryPath,
		"trimmedPath":    path,
		"isRepomd":       p.isRepomdRequest(path),
		"isPrimary":      p.isPrimaryRequest(path),
		"isRpmPackage":   p.isRpmPackageRequest(path),
		"repositoryName": ctx.Repository.Name,
	}).Debug("yum: Handle called")

	if p.isRepomdRequest(path) {
		return p.handleRepomd(ctx, repoRuntime, path)
	}

	if p.isPrimaryRequest(path) {
		return p.handlePrimary(ctx, repoRuntime, path)
	}

	if p.isRpmPackageRequest(path) {
		return p.handleRpmPackage(ctx, repoRuntime, path)
	}

	logrus.WithFields(logrus.Fields{
		"path":           path,
		"repositoryName": ctx.Repository.Name,
	}).Warn("yum: path does not match any known pattern, returning 404")
	http.Error(ctx.Writer, "Not found", http.StatusNotFound)
	return nil
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

func (p *YumPlugin) handleRepomd(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Coordinates: map[string]string{
			"file": "repomd.xml",
		},
		Filename: "repomd.xml",
	}

	if artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key); err == nil && artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		ctx.Writer.Header().Set("Content-Type", "application/xml")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			return err
		}
		return nil
	}

	cacheKey := "yum:repomd:" + ctx.Repository.ID + ":" + path
	if p.cache != nil {
		if v, ok := p.cache.Get(cacheKey); ok {
			if b, ok := v.([]byte); ok && len(b) > 0 {
				ctx.Writer.Header().Set("Content-Type", "application/xml")
				ctx.Writer.WriteHeader(http.StatusOK)
				_, _ = ctx.Writer.Write(b)
				return nil
			}
		}
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
	})
	if err != nil || len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	type data struct {
		Type     string `xml:"type,attr"`
		Location struct {
			Href string `xml:"href,attr"`
		} `xml:"location"`
	}
	type repomd struct {
		XMLName xml.Name `xml:"repomd"`
		Xmlns   string   `xml:"xmlns,attr"`
		Data    []data   `xml:"data"`
	}
	out := repomd{Xmlns: "http://linux.duke.edu/metadata/repo"}
	for _, a := range artifacts {
		f := a.Coordinates["file"]
		if f == "" {
			continue
		}
		d := data{Type: "primary"}
		d.Location.Href = f
		out.Data = append(out.Data, d)
	}
	if len(out.Data) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	body, _ := xml.MarshalIndent(out, "", "  ")
	finalBody := append([]byte(xml.Header), body...)
	if p.cache != nil {
		p.cache.Set(cacheKey, finalBody, 5*time.Minute)
	}
	ctx.Writer.Header().Set("Content-Type", "application/xml")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write(finalBody)
	return nil
}

func (p *YumPlugin) handlePrimary(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	filename := filepath.Base(path)
	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Coordinates: map[string]string{
			"file": filename,
		},
		Filename: filename,
	}

	if artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key); err == nil && artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		ctx.Writer.Header().Set("Content-Type", "application/xml")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			return err
		}
		return nil
	}

	cacheKey := "yum:primary:" + ctx.Repository.ID + ":" + path
	if p.cache != nil {
		if v, ok := p.cache.Get(cacheKey); ok {
			if b, ok := v.([]byte); ok && len(b) > 0 {
				ctx.Writer.Header().Set("Content-Type", "application/xml")
				ctx.Writer.WriteHeader(http.StatusOK)
				_, _ = ctx.Writer.Write(b)
				return nil
			}
		}
	}

	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
	})
	if err != nil || len(artifacts) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<metadata xmlns="http://linux.duke.edu/metadata/common">` + "\n")
	for _, a := range artifacts {
		name := a.Coordinates["name"]
		ver := a.Coordinates["version"]
		if name == "" || ver == "" {
			continue
		}
		fmt.Fprintf(&b, "  <package type=\"rpm\"><name>%s</name><version ver=\"%s\"/></package>\n", name, ver)
	}
	b.WriteString(`</metadata>`)
	body := []byte(b.String())
	if p.cache != nil {
		p.cache.Set(cacheKey, body, 5*time.Minute)
	}
	ctx.Writer.Header().Set("Content-Type", "application/xml")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write(body)
	return nil
}

func (p *YumPlugin) handleRpmPackage(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	filename := filepath.Base(path)

	key := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
		Coordinates: map[string]string{
			"filename": filename,
		},
		Filename: filename,
	}

	switch ctx.Request.Method {
	case http.MethodGet:
		artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
		if err != nil {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		if artifact.Content == nil {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		defer artifact.Content.Close()
		ctx.FromCache = artifact.FromCache
		ctx.RemoteURL = artifact.RemoteURL
		ctx.SizeBytes = artifact.SizeBytes
		ctx.Writer.Header().Set("Content-Type", "application/x-rpm")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			return err
		}
		return nil
	}
	return errors.New("method not allowed")
}
