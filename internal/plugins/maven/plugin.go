package maven

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
)

// mavenVersionPattern 匹配 Maven 版本号：数字开头或包含 SNAPSHOT
var mavenVersionPattern = regexp.MustCompile(`^\d|^SNAPSHOT`)

type MavenPlugin struct {
	httpClient *http.Client
}

func NewMavenPlugin() *MavenPlugin {
	return &MavenPlugin{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetHTTPClient allows injecting a shared HTTP client (with DNS mapping, TLS config, etc.)
func (p *MavenPlugin) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// FetchRemote implements the RemoteFetcher interface.
// Runtime calls this when local cache is empty; Plugin handles remote Maven protocol interaction.
// It fetches maven-metadata.xml from the remote repository and parses versions from it.
func (p *MavenPlugin) FetchRemote(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, errors.New("maven: empty path")
	}

	logrus.WithFields(logrus.Fields{
		"remoteURL": remoteURL,
		"path":      path,
	}).Debug("maven: FetchRemote called")

	// For maven-metadata.xml requests, fetch and parse the XML from remote.
	if strings.HasSuffix(path, "maven-metadata.xml") {
		return p.fetchMetadata(ctx, remoteURL, path)
	}

	// For other paths (artifact downloads), return a basic artifact indicating the remote resource exists.
	key, err := p.parseMavenPath(path)
	if err != nil {
		logrus.WithError(err).WithField("path", path).Error("maven: parse remote path failed")
		return nil, fmt.Errorf("maven: parse remote path: %w", err)
	}
	logrus.WithFields(logrus.Fields{
		"path":     path,
		"group":    key.Coordinates["group"],
		"artifact": key.Coordinates["artifact"],
		"version":  key.Coordinates["version"],
		"filename": key.Filename,
	}).Debug("maven: FetchRemote returning artifact reference")
	return []*runtime.Artifact{
		{
			Format:      "maven",
			Kind:        "artifact",
			Coordinates: key.Coordinates,
			Properties: map[string]string{
				"filename":  key.Filename,
				"extension": key.Extension,
			},
		},
	}, nil
}

// fetchMetadata fetches maven-metadata.xml from the remote repository and extracts versions.
func (p *MavenPlugin) fetchMetadata(ctx context.Context, remoteURL, path string) ([]*runtime.Artifact, error) {
	start := time.Now()
	fullURL := strings.TrimRight(remoteURL, "/") + "/" + path

	logrus.WithFields(logrus.Fields{
		"remoteURL": remoteURL,
		"path":      path,
		"fullURL":   fullURL,
	}).Debug("maven: fetchMetadata called")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		logrus.WithError(err).WithField("fullURL", fullURL).Error("maven: create request for metadata failed")
		return nil, fmt.Errorf("maven: create request for metadata: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("maven: fetch metadata HTTP request failed")
		return nil, fmt.Errorf("maven: fetch metadata from %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logrus.WithFields(logrus.Fields{
			"fullURL":    fullURL,
			"statusCode": resp.StatusCode,
			"duration":   time.Since(start).Seconds(),
		}).Error("maven: fetch metadata returned non-200 status")
		return nil, fmt.Errorf("maven: fetch metadata from %s: status %d", fullURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("maven: read metadata body failed")
		return nil, fmt.Errorf("maven: read metadata body: %w", err)
	}

	var meta mavenMetadata
	if err := xml.Unmarshal(body, &meta); err != nil {
		logrus.WithFields(logrus.Fields{
			"fullURL":  fullURL,
			"duration": time.Since(start).Seconds(),
			"error":    err.Error(),
		}).Error("maven: unmarshal metadata XML failed")
		return nil, fmt.Errorf("maven: unmarshal metadata XML: %w", err)
	}

	// Parse the path to extract group and artifact coordinates.
	parts := strings.Split(strings.Trim(strings.TrimSuffix(path, "/maven-metadata.xml"), "/"), "/")
	if len(parts) < 2 {
		logrus.WithField("path", path).Error("maven: invalid metadata path")
		return nil, fmt.Errorf("maven: invalid metadata path: %s", path)
	}

	artifact := parts[len(parts)-1]
	groupParts := parts[:len(parts)-1]
	version := ""
	// 最后一段是版本号当且仅当它匹配版本模式（数字开头或 SNAPSHOT）
	if len(parts) >= 3 && mavenVersionPattern.MatchString(parts[len(parts)-1]) {
		version = parts[len(parts)-1]
		artifact = parts[len(parts)-2]
		groupParts = parts[:len(parts)-2]
	}
	group := strings.Join(groupParts, ".")
	if meta.GroupID != "" {
		group = meta.GroupID
	}
	if meta.ArtifactID != "" {
		artifact = meta.ArtifactID
	}

	var artifacts []*runtime.Artifact
	versions := meta.Versioning.Versions.Items
	if len(versions) == 0 && meta.Version != "" {
		versions = []string{meta.Version}
	}

	var publishedAt string
	if meta.Versioning.LastU != "" {
		if t, err := time.Parse("200601021504", meta.Versioning.LastU); err == nil {
			publishedAt = t.Format(time.RFC3339)
		}
	}

	for _, v := range versions {
		coords := map[string]string{
			"group":    group,
			"artifact": artifact,
			"version":  v,
		}
		if version != "" {
			coords["base_version"] = version
		}
		props := map[string]string{
			"latest":  meta.Versioning.Latest,
			"release": meta.Versioning.Release,
		}
		if publishedAt != "" {
			props["published_at"] = publishedAt
		}
		artifacts = append(artifacts, &runtime.Artifact{
			Format:      "maven",
			Kind:        "version",
			Coordinates: coords,
			Properties:  props,
		})
	}

	logrus.WithFields(logrus.Fields{
		"fullURL":      fullURL,
		"group":        group,
		"artifact":     artifact,
		"versionCount": len(artifacts),
		"duration":     time.Since(start).Seconds(),
	}).Debug("maven: fetchMetadata success")
	return artifacts, nil
}

func (p *MavenPlugin) fetchLicenseFromPOM(ctx context.Context, remoteURL, group, artifact, version string) string {
	groupPath := strings.ReplaceAll(group, ".", "/")
	pomURL := strings.TrimRight(remoteURL, "/") + "/" + groupPath + "/" + artifact + "/" + version + "/" + artifact + "-" + version + ".pom"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pomURL, nil)
	if err != nil {
		return ""
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var pom pomProject
	if err := xml.NewDecoder(resp.Body).Decode(&pom); err != nil {
		return ""
	}
	if len(pom.Licenses) > 0 {
		return pom.Licenses[0].Name
	}
	return ""
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

type pomProject struct {
	Licenses []pomLicense `xml:"licenses>license"`
}

type pomLicense struct {
	Name string `xml:"name"`
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
	// 最后一段是版本号当且仅当它匹配版本模式（数字开头或 SNAPSHOT）
	if len(parts) >= 3 && mavenVersionPattern.MatchString(parts[len(parts)-1]) {
		version = parts[len(parts)-1]
		artifact = parts[len(parts)-2]
		groupParts = parts[:len(parts)-2]
	}
	group := strings.Join(groupParts, ".")

	query := runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
		Coordinates: map[string]string{
			"group":    group,
			"artifact": artifact,
		},
	}
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), query)
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
				"filename": "maven-metadata.xml",
			},
			Filename: "maven-metadata.xml",
		}
		if metaArtifact, metaErr := repoRuntime.GetArtifact(ctx.Request.Context(), metaKey); metaErr == nil && metaArtifact.Content != nil {
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
			"name":     group + ":" + artifact,
			"group":    group,
			"artifact": artifact,
			"version":  version,
			"filename": filename,
			"path":     strings.TrimSuffix(path, "/"+filename),
		},
		Filename:  filename,
		Extension: ext,
	}, nil
}

func (p *MavenPlugin) handleDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
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

	ctx.FromCache = artifact.FromCache
	ctx.RemoteURL = artifact.RemoteURL
	ctx.SizeBytes = artifact.SizeBytes

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.Header().Set("Content-Disposition", "inline; filename=\""+runtime.SanitizeFilename(key.Filename)+"\"")
	ctx.Writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
		return err
	}
	return nil
}

func (p *MavenPlugin) handleUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	session, err := repoRuntime.BeginUpload(ctx.Request.Context(), runtime.UploadRequest{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
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

	key.Coordinates["name"] = key.Coordinates["group"] + ":" + key.Coordinates["artifact"]

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
