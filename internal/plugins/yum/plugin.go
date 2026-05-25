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

	"github.com/moonlight-box/registry/internal/core/cache"
	"github.com/moonlight-box/registry/internal/core/runtime"
)

type YumPlugin struct {
	cache *cache.MemoryCache
}

func NewYumPlugin() *YumPlugin {
	return &YumPlugin{cache: cache.NewMemoryCache()}
}

func (p *YumPlugin) Name() string {
	return "yum"
}

func (p *YumPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if p.isRepomdRequest(path) {
		return p.handleRepomd(ctx, repoRuntime, path)
	}

	if p.isPrimaryRequest(path) {
		return p.handlePrimary(ctx, repoRuntime, path)
	}

	if p.isRpmPackageRequest(path) {
		return p.handleRpmPackage(ctx, repoRuntime, path)
	}

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

	if artifact, err := repoRuntime.GetArtifact(context.Background(), key); err == nil && artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.Writer.Header().Set("Content-Type", "application/xml")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+key.Filename+"\"")
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

	artifacts, err := repoRuntime.QueryArtifacts(context.Background(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
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

	if artifact, err := repoRuntime.GetArtifact(context.Background(), key); err == nil && artifact.Content != nil {
		defer artifact.Content.Close()
		ctx.Writer.Header().Set("Content-Type", "application/xml")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+key.Filename+"\"")
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

	artifacts, err := repoRuntime.QueryArtifacts(context.Background(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "yum",
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
		artifact, err := repoRuntime.GetArtifact(context.Background(), key)
		if err != nil {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		if artifact.Content == nil {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return nil
		}
		defer artifact.Content.Close()
		ctx.Writer.Header().Set("Content-Type", "application/x-rpm")
		ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+key.Filename+"\"")
		ctx.Writer.WriteHeader(http.StatusOK)
		if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
			return err
		}
		return nil
	}
	return errors.New("method not allowed")
}
