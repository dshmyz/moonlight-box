package maven

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moonlight-box/registry/internal/core/runtime"
)

type MavenPlugin struct{}

func NewMavenPlugin() *MavenPlugin {
	return &MavenPlugin{}
}

func (p *MavenPlugin) Name() string {
	return "maven"
}

func (p *MavenPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "maven-metadata.xml") && ctx.Request.Method == http.MethodGet {
		return p.handleMetadata(ctx, repoRuntime, path)
	}

	key, err := p.parseMavenPath(path)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}
	key.RepositoryID = ctx.Repository.ID

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

type mavenMetadata struct {
	XMLName    xml.Name           `xml:"metadata"`
	Xmlns      string             `xml:"xmlns,attr,omitempty"`
	Model      string             `xml:"modelVersion"`
	GroupID    string             `xml:"groupId"`
	ArtifactID string             `xml:"artifactId"`
	Version    string             `xml:"version,omitempty"`
	Versioning mavenVersioningXML `xml:"versioning"`
}

type mavenVersioningXML struct {
	Latest           string                `xml:"latest,omitempty"`
	Release          string                `xml:"release,omitempty"`
	Versions         mavenVersionsXML      `xml:"versions"`
	LastU            string                `xml:"lastUpdated"`
	Snapshot         *mavenSnapshotXML     `xml:"snapshot,omitempty"`
	SnapshotVersions *mavenSnapshotVersXML `xml:"snapshotVersions,omitempty"`
}

type mavenVersionsXML struct {
	Items []string `xml:"version"`
}

type mavenSnapshotXML struct {
	Timestamp   string `xml:"timestamp,omitempty"`
	BuildNumber string `xml:"buildNumber,omitempty"`
}

type mavenSnapshotVersXML struct {
	Items []mavenSnapshotVersionXML `xml:"snapshotVersion"`
}

type mavenSnapshotVersionXML struct {
	Extension  string `xml:"extension"`
	Classifier string `xml:"classifier,omitempty"`
	Value      string `xml:"value"`
	Updated    string `xml:"updated"`
}

func (p *MavenPlugin) handleMetadata(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, path string) error {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 {
		http.Error(ctx.Writer, "invalid maven metadata path", http.StatusBadRequest)
		return nil
	}
	parts = parts[:len(parts)-1] // drop maven-metadata.xml
	if len(parts) < 2 {
		http.Error(ctx.Writer, "invalid maven metadata path", http.StatusBadRequest)
		return nil
	}

	artifact := parts[len(parts)-1]
	groupParts := parts[:len(parts)-1]
	version := ""
	if len(parts) >= 4 {
		// {groupDirs...}/{artifactId}/{version}/maven-metadata.xml
		version = parts[len(parts)-1]
		artifact = parts[len(parts)-2]
		groupParts = parts[:len(parts)-2]
	} else if len(parts) >= 3 {
		last := parts[len(parts)-1]
		prev := parts[len(parts)-2]
		if prev != "" && last != "" && len(last) > 0 && last[0] >= '0' && last[0] <= '9' {
			version = last
			artifact = prev
			groupParts = parts[:len(parts)-2]
		}
	}
	group := strings.Join(groupParts, ".")

	query := runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
		Coordinates: map[string]string{
			"group":    group,
			"artifact": artifact,
		},
	}
	artifacts, err := repoRuntime.QueryArtifacts(context.Background(), query)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}
	if len(artifacts) == 0 {
		// For proxy repos: try fetching maven-metadata.xml as a cached artifact.
		// GetArtifact on ProxyRuntime fetches from remote and caches locally.
		metaKey := runtime.ArtifactKey{
			RepositoryID: ctx.Repository.ID,
			Format:       "maven",
			Coordinates: map[string]string{
				"group":    group,
				"artifact": artifact,
				"version":  version,
				"path":     strings.TrimSuffix(strings.Trim(path, "/"), "/maven-metadata.xml"),
			},
			Filename: "maven-metadata.xml",
		}
		if metaArtifact, metaErr := repoRuntime.GetArtifact(context.Background(), metaKey); metaErr == nil && metaArtifact.Content != nil {
			defer metaArtifact.Content.Close()
			body, _ := io.ReadAll(metaArtifact.Content)
			ctx.Writer.Header().Set("Content-Type", "application/xml")
			ctx.Writer.WriteHeader(http.StatusOK)
			_, _ = ctx.Writer.Write(body)
			return nil
		}
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	versionSet := map[string]struct{}{}
	for _, a := range artifacts {
		v := a.Coordinates["version"]
		if v == "" {
			continue
		}
		if version != "" && v != version && !strings.HasPrefix(v, version+"-") {
			continue
		}
		versionSet[v] = struct{}{}
	}
	versions := make([]string, 0, len(versionSet))
	for v := range versionSet {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	if len(versions) == 0 {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	latest := versions[len(versions)-1]
	lastTime := artifacts[0].CreatedAt
	for _, a := range artifacts {
		if a.CreatedAt.After(lastTime) {
			lastTime = a.CreatedAt
		}
	}
	lastUpdated := lastTime.UTC().Format("20060102150405")
	meta := mavenMetadata{
		Model:      "1.1.0",
		GroupID:    group,
		ArtifactID: artifact,
		Version:    version,
		Versioning: mavenVersioningXML{
			Latest:   latest,
			Release:  latest,
			Versions: mavenVersionsXML{Items: versions},
			LastU:    lastUpdated,
		},
	}
	if version != "" && strings.Contains(version, "SNAPSHOT") {
		ts := strings.TrimSuffix(lastUpdated, lastUpdated[len(lastUpdated)-2:])
		meta.Versioning.Snapshot = &mavenSnapshotXML{
			Timestamp:   ts,
			BuildNumber: "1",
		}
		snapshotItems := make([]mavenSnapshotVersionXML, 0)
		for _, a := range artifacts {
			v := a.Coordinates["version"]
			if !strings.HasPrefix(v, version) {
				continue
			}
			ext := a.Properties["extension"]
			if ext == "" {
				ext = strings.TrimPrefix(filepath.Ext(a.Properties["filename"]), ".")
			}
			if ext == "" {
				ext = strings.TrimPrefix(filepath.Ext(a.Coordinates["filename"]), ".")
			}
			if ext == "" {
				ext = "jar"
			}
			classifier := a.Properties["classifier"]
			if classifier == "" {
				classifier = a.Coordinates["classifier"]
			}
			value := v
			if value == "" {
				value = version
			}
			snapshotItems = append(snapshotItems, mavenSnapshotVersionXML{
				Extension:  ext,
				Classifier: classifier,
				Value:      value,
				Updated:    lastUpdated,
			})
		}
		if len(snapshotItems) > 0 {
			sort.Slice(snapshotItems, func(i, j int) bool {
				if snapshotItems[i].Extension != snapshotItems[j].Extension {
					return snapshotItems[i].Extension < snapshotItems[j].Extension
				}
				return snapshotItems[i].Classifier < snapshotItems[j].Classifier
			})
			meta.Versioning.SnapshotVersions = &mavenSnapshotVersXML{Items: snapshotItems}
		}
	}

	body, err := xml.MarshalIndent(meta, "", "  ")
	if err != nil {
		http.Error(ctx.Writer, fmt.Sprintf("render metadata failed: %v", err), http.StatusInternalServerError)
		return nil
	}
	ctx.Writer.Header().Set("Content-Type", "application/xml")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write([]byte(xml.Header))
	_, _ = ctx.Writer.Write(body)
	return nil
}

func (p *MavenPlugin) handleDelete(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
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

func (p *MavenPlugin) parseMavenPath(path string) (runtime.ArtifactKey, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 {
		return runtime.ArtifactKey{}, errors.New("invalid maven path")
	}

	filename := parts[len(parts)-1]
	ext := filepath.Ext(filename)

	// Correct approach: path segments directly encode group/artifact/version.
	// Path: {group...}/{artifactId}/{version}/{artifactId}-{version}[-classifier].{ext}
	// So parts[-2] = version, parts[-3] = artifact, parts[:-3] = group path segments.
	version := parts[len(parts)-2]
	artifact := parts[len(parts)-3]
	groupParts := parts[:len(parts)-3]
	group := strings.Join(groupParts, ".")

	return runtime.ArtifactKey{
		Format: "maven",
		Coordinates: map[string]string{
			"group":    group,
			"artifact": artifact,
			"version":  version,
			"path":     strings.TrimSuffix(path, "/"+filename),
		},
		Filename:  filename,
		Extension: ext,
	}, nil
}

func (p *MavenPlugin) handleDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
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

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+key.Filename+"\"")
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		return err
	}
	return nil
}

func (p *MavenPlugin) handleUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	session, err := repoRuntime.BeginUpload(context.Background(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
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
		Format:       "maven",
		Kind:         "artifact",
		Coordinates:  key.Coordinates,
		BlobRefs:     []runtime.BlobRef{blobRef},
		Properties: map[string]string{
			"filename":  key.Filename,
			"extension": key.Extension,
			"group":     key.Coordinates["group"],
			"artifact":  key.Coordinates["artifact"],
			"version":   key.Coordinates["version"],
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
