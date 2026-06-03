// Package maven implements the Maven repository protocol plugin.
//
// # Maven 仓库协议要点
//
// ## 路径结构
//   - groupId/artifactId/version/artifactId-version[-classifier].ext
//   - 例如: com/example/my-lib/1.0.0/my-lib-1.0.0.jar
//   - SNAPSHOT: com/example/my-lib/1.0-SNAPSHOT/my-lib-1.0-20260603.120000-1.jar
//
// ## 元数据文件 (maven-metadata.xml)
//
// ### Release 版本格式
//
//	<metadata>
//	  <groupId>com.example</groupId>
//	  <artifactId>my-lib</artifactId>
//	  <versioning>
//	    <latest>1.0.0</latest>
//	    <release>1.0.0</release>
//	    <versions>
//	      <version>1.0.0</version>
//	    </versions>
//	    <lastUpdated>20260603120000</lastUpdated>
//	  </versioning>
//	</metadata>
//
// ### SNAPSHOT 版本格式
//
//	<metadata>
//	  <groupId>com.example</groupId>
//	  <artifactId>my-lib</artifactId>
//	  <version>1.0-SNAPSHOT</version>
//	  <versioning>
//	    <snapshot>
//	      <timestamp>20260603.120000</timestamp>
//	      <buildNumber>1</buildNumber>
//	    </snapshot>
//	    <lastUpdated>20260603120000</lastUpdated>
//	    <snapshotVersions>
//	      <snapshotVersion>
//	        <extension>jar</extension>
//	        <value>1.0-20260603.120000-1</value>
//	        <updated>20260603120000</updated>
//	      </snapshotVersion>
//	    </snapshotVersions>
//	  </versioning>
//	</metadata>
//
// ### 时间格式规范（重要）
//
//	| 字段        | 格式              | 示例              | 位数 |
//	|-------------|-------------------|-------------------|------|
//	| timestamp   | YYYYMMDD.HHmmss   | 20260603.120000   | 15位 |
//	| lastUpdated | YYYYMMDDHHmmss    | 20260603120000    | 14位 |
//	| updated     | YYYYMMDDHHmmss    | 20260603120000    | 14位 |
//
// 转换公式: lastUpdated = strings.ReplaceAll(timestamp, ".", "")
//
// **注意**: lastUpdated 不能添加额外后缀（如 "00"），否则 Maven 客户端可能解析异常。
//
// ## 关键实现点
//   - SNAPSHOT 版本过滤: 使用 baseVersion 前缀匹配（去掉 -SNAPSHOT 后缀）
//   - Classifier 提取: 从文件名中解析 sources、javadoc 等 classifier
//   - Timestamp 格式: YYYYMMDD.HHmmss（日期和时间用点号分隔）
//   - LastUpdated 格式: YYYYMMDDHHmmss（直接拼接，无分隔符，14 位）
//   - metaKey 必须包含 name 字段（group:artifact）以匹配上传时的坐标
//   - Local 仓库 SNAPSHOT metadata: 需查询已上传 artifacts 聚合生成最新时间戳
//
// ## 参考规范
//   - https://maven.apache.org/repository-layout.html
//   - https://maven.apache.org/ref/3.9.6/maven-repository-metadata/repository-metadata.html
//   - https://developer.aliyun.com/article/136028 (SNAPSHOT 机制详解)
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
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
)

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

	// Maven 坐标必须以 XML metadata 为准，不依赖路径推断。
	// 路径只用来辅助判断请求语义（artifact 级 or version 级 metadata）。
	if meta.GroupID == "" || meta.ArtifactID == "" {
		return nil, fmt.Errorf("maven: metadata XML missing groupId or artifactId")
	}
	group := meta.GroupID
	artifact := meta.ArtifactID

	// 从路径判断是否是 SNAPSHOT 版本的 metadata 请求
	// 路径格式: {groupId}/{artifactId}/maven-metadata.xml      (artifact 级)
	//        或 {groupId}/{artifactId}/{version}/maven-metadata.xml (version 级, 仅 SNAPSHOT)
	parts := strings.Split(strings.Trim(strings.TrimSuffix(path, "/maven-metadata.xml"), "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("maven: invalid metadata path: %s", path)
	}
	version := ""
	if len(parts) >= 3 && strings.Contains(parts[len(parts)-1], "-SNAPSHOT") {
		version = parts[len(parts)-1]
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

	ctx.PackageName = key.Coordinates["name"]
	ctx.Version = key.Coordinates["version"]
	ctx.Filename = key.Filename

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

	// 路径推断：artifact 级或 version 级 metadata
	// artifact 级: {groupId}/{artifactId}/maven-metadata.xml
	// version 级: {groupId}/{artifactId}/{version}/maven-metadata.xml (仅 SNAPSHOT)
	artifact := parts[len(parts)-1]
	groupParts := parts[:len(parts)-1]
	version := ""
	if len(parts) >= 3 && strings.Contains(parts[len(parts)-1], "-SNAPSHOT") {
		version = parts[len(parts)-1]
		artifact = parts[len(parts)-2]
		groupParts = parts[:len(parts)-2]
	}
	group := strings.Join(groupParts, ".")

	ctx.PackageName = group + ":" + artifact

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

	// 只有当缓存中存在 metadata 回源产生的 version 记录时，才用聚合方式生成 metadata。
	// 如果缓存中只有 GetArtifact 产生的 artifact 记录（单文件下载缓存），版本列表不完整，
	// 应该回源获取原始 maven-metadata.xml。
	hasVersionArtifacts := false
	for _, a := range artifacts {
		if a.Kind == "version" {
			hasVersionArtifacts = true
			break
		}
	}

	if !hasVersionArtifacts {
		// For SNAPSHOT version metadata in local repos: aggregate from uploaded artifacts
		// to generate dynamic metadata with correct timestamp and buildNumber.
		if version != "" && strings.Contains(version, "-SNAPSHOT") {
			// 查询所有已上传的 SNAPSHOT artifacts
			snapQuery := runtime.ArtifactQuery{
				RepositoryID: ctx.Repository.ID,
				Format:       "maven",
				Coordinates: map[string]string{
					"group":    group,
					"artifact": artifact,
					"version":  version,
				},
			}
			snapArtifacts, snapErr := repoRuntime.QueryArtifacts(ctx.Request.Context(), snapQuery)
			if snapErr != nil {
				logrus.WithError(snapErr).WithFields(logrus.Fields{
					"group":    group,
					"artifact": artifact,
					"version":  version,
				}).Warn("maven: query snapshot artifacts failed, fallback to GetArtifact")
			}
			if snapErr == nil && len(snapArtifacts) > 0 {
				// 从文件名中提取 timestamp 和 buildNumber
				// 格式: artifactId-version-timestamp-buildNumber.ext
				baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
				var latestTimestamp, latestBuildNum string
				var latestTime time.Time
				var snapshotFiles []struct{ ext, classifier string }

				for _, a := range snapArtifacts {
					filename := a.Coordinates["filename"]
					if filename == "" || strings.Contains(filename, "maven-metadata") {
						continue
					}
					// 解析文件名: snapshot-lib-1.0-20260603.033633-3.jar
					prefix := artifact + "-" + baseVersion + "-"
					if !strings.HasPrefix(filename, prefix) {
						continue
					}
					rest := strings.TrimPrefix(filename, prefix)
					// rest: 20260603.033633-3.jar 或 20260603.033633-3-sources.jar
					parts := strings.SplitN(rest, "-", 3)
					if len(parts) < 2 {
						continue
					}
					ts := parts[0]       // 20260603.033633
					buildNum := parts[1] // 3 或 3.jar
					// 去掉扩展名
					if idx := strings.Index(buildNum, "."); idx >= 0 {
						buildNum = buildNum[:idx]
					}

					// 比较时间，找出最新的
					if a.UpdatedAt.After(latestTime) {
						latestTime = a.UpdatedAt
						latestTimestamp = ts
						latestBuildNum = buildNum
					}

					// 收集文件类型（jar, pom, sources, javadoc 等）
					ext := filepath.Ext(filename)
					base := strings.TrimSuffix(filename, ext)
					classifier := ""
					// 检查是否有 classifier: artifactId-version-timestamp-buildNum-classifier
					classifierPrefix := prefix + ts + "-" + buildNum + "-"
					if strings.HasPrefix(base, classifierPrefix) {
						classifier = strings.TrimPrefix(base, classifierPrefix)
					}
					snapshotFiles = append(snapshotFiles, struct{ ext, classifier string }{
						ext:        strings.TrimPrefix(ext, "."),
						classifier: classifier,
					})
				}

				if latestTimestamp != "" && latestBuildNum != "" {
					// 生成动态 SNAPSHOT metadata
					// lastUpdated 格式: YYYYMMDDHHmmss（14 位）
					lastUpdated := strings.ReplaceAll(latestTimestamp, ".", "")
					meta := mavenMetadata{
						Model:      "1.1.0",
						GroupID:    group,
						ArtifactID: artifact,
						Version:    version,
						Versioning: mavenVersioningXML{
							Latest:   version,
							Release:  "",
							Versions: mavenVersionsXML{Items: []string{version}},
							LastU:    lastUpdated,
							Snapshot: &mavenSnapshotXML{
								Timestamp:   latestTimestamp,
								BuildNumber: latestBuildNum,
							},
						},
					}

					// 生成 snapshotVersions
					snapshotItems := make([]mavenSnapshotVersionXML, 0)
					for _, info := range snapshotFiles {
						snapshotItems = append(snapshotItems, mavenSnapshotVersionXML{
							Extension:  info.ext,
							Classifier: info.classifier,
							Value:      baseVersion + "-" + latestTimestamp + "-" + latestBuildNum,
							Updated:    lastUpdated,
						})
						// 同时添加 SNAPSHOT 版本引用
						snapshotItems = append(snapshotItems, mavenSnapshotVersionXML{
							Extension:  info.ext,
							Classifier: info.classifier,
							Value:      version,
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
			}
		}

		// For proxy repos: try fetching maven-metadata.xml as a cached artifact.
		// GetArtifact on ProxyRuntime fetches from remote and caches locally.
		metaKey := runtime.ArtifactKey{
			RepositoryID: ctx.Repository.ID,
			Format:       "maven",
			Coordinates: map[string]string{
				"name":     group + ":" + artifact,
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
		if version != "" && strings.Contains(version, "-SNAPSHOT") {
			baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
			if v != version && !strings.HasPrefix(v, baseVersion+"-") {
				continue
			}
		} else if version != "" && v != version {
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
		var ts string
		if len(lastUpdated) >= 14 {
			ts = lastUpdated[:8] + "." + lastUpdated[8:14]
		} else {
			ts = lastUpdated
		}
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

	// Extract classifier from filename: {artifactId}-{version}[-classifier].{ext}
	// SNAPSHOT 版本的文件名中版本可能是时间戳格式（如 1.0-20230615.120000-1），
	// 需要同时尝试 SNAPSHOT 版本和时间戳版本的前缀
	baseName := strings.TrimSuffix(filename, ext)
	var classifier string
	classifierPrefix := artifact + "-" + version + "-"
	if strings.HasPrefix(baseName, classifierPrefix) {
		classifier = strings.TrimPrefix(baseName, classifierPrefix)
	} else if strings.Contains(version, "-SNAPSHOT") {
		// SNAPSHOT 版本：文件名中可能是时间戳格式，尝试匹配 artifactId- 前缀后提取
		// 文件名格式: {artifactId}-{baseVersion}-{timestamp}-{buildNumber}[-classifier].{ext}
		baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
		tsPrefix := artifact + "-" + baseVersion + "-"
		if strings.HasPrefix(baseName, tsPrefix) {
			rest := strings.TrimPrefix(baseName, tsPrefix)
			// rest 格式: {timestamp}-{buildNumber}[-classifier]
			// 跳过 timestamp-buildNumber 部分，提取 classifier
			// timestamp 格式: YYYYMMDD.HHmmss，buildNumber 是数字
			parts := strings.SplitN(rest, "-", 3)
			if len(parts) >= 3 {
				classifier = parts[2]
			}
		}
	}

	return runtime.ArtifactKey{
		Format: "maven",
		Coordinates: map[string]string{
			"name":       group + ":" + artifact,
			"group":      group,
			"artifact":   artifact,
			"version":    version,
			"filename":   filename,
			"path":       strings.TrimSuffix(path, "/"+filename),
			"classifier": classifier,
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
		logrus.WithError(err).Warn("failed to write artifact content to client")
		return nil
	}
	return nil
}

func (p *MavenPlugin) handleUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	// 检查文件是否已存在，用于决定返回 201 还是 200
	key.Coordinates["name"] = key.Coordinates["group"] + ":" + key.Coordinates["artifact"]
	existingArtifact, _ := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	isUpdate := existingArtifact != nil

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

	// 创建返回 201，更新返回 200
	if isUpdate {
		ctx.Writer.WriteHeader(http.StatusOK)
	} else {
		ctx.Writer.WriteHeader(http.StatusCreated)
	}
	return nil
}
