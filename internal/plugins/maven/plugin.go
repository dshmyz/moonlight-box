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
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
)

type MavenPlugin struct {
	httpClient *http.Client
}

func NewMavenPlugin(httpClient *http.Client) *MavenPlugin {
	if httpClient == nil {
		panic("maven: httpClient is required")
	}
	return &MavenPlugin{httpClient: httpClient}
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
		"group":    key.Qualifiers["group"],
		"artifact": key.Qualifiers["artifact"],
		"version":  key.Version,
		"filename": key.Filename,
	}).Debug("maven: FetchRemote returning artifact reference")
	return []*runtime.Artifact{
		runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "maven",
			Kind:       runtime.KindArtifact,
			Namespace:  key.Namespace,
			Name:       key.Name,
			Version:    key.Version,
			Path:       key.Path,
			Filename:   key.Filename,
			RemotePath: key.RemotePath,
			Extension:  key.Extension,
			Qualifiers: key.Qualifiers,
			Properties: map[string]string{
				"filename":  key.Filename,
				"extension": key.Extension,
			},
		}),
	}, nil
}

// FetchArtifactMetadata retrieves the version POM and exposes its license as a
// protocol-normalized artifact attribute for conditional rule evaluation.
func (p *MavenPlugin) FetchArtifactMetadata(ctx context.Context, remoteURL string, key runtime.ArtifactKey) (*runtime.ArtifactMetadata, error) {
	group := key.Qualifiers["group"]
	artifact := key.Qualifiers["artifact"]
	if group == "" || artifact == "" {
		parts := strings.SplitN(key.Name, ":", 2)
		if len(parts) == 2 {
			group, artifact = parts[0], parts[1]
		}
	}
	if group == "" || artifact == "" || key.Version == "" {
		return nil, runtime.ErrMetadataUnavailable
	}
	pomPath := strings.ReplaceAll(group, ".", "/") + "/" + artifact + "/" + key.Version + "/" + artifact + "-" + key.Version + ".pom"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(remoteURL, "/")+"/"+pomPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, runtime.ErrMetadataUnavailable
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	license := parsePOMLicense(body)
	if license == "" {
		return nil, runtime.ErrMetadataUnavailable
	}
	return &runtime.ArtifactMetadata{Attributes: map[string]string{"license": license}}, nil
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
		if resp.StatusCode == http.StatusNotFound {
			return nil, runtime.ErrNotFound
		}
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
		if t, err := parseMavenLastUpdated(meta.Versioning.LastU); err == nil {
			publishedAt = t.Format(time.RFC3339)
		}
	}

	for _, v := range versions {
		qualifiers := map[string]string{"group": group, "artifact": artifact}
		if version != "" {
			qualifiers["base_version"] = version
		}
		props := map[string]string{
			"latest":  meta.Versioning.Latest,
			"release": meta.Versioning.Release,
		}
		if publishedAt != "" {
			props["published_at"] = publishedAt
		}
		artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
			Format:     "maven",
			Kind:       runtime.KindVersion,
			Namespace:  group,
			Name:       group + ":" + artifact,
			Version:    v,
			Qualifiers: qualifiers,
			Attributes: props,
			Properties: props,
		}))
	}

	if version != "" && meta.Versioning.SnapshotVersions != nil {
		basePath := strings.TrimSuffix(path, "/maven-metadata.xml")
		for _, sv := range meta.Versioning.SnapshotVersions.Items {
			filename := mavenSnapshotFilename(artifact, sv)
			if filename == "" {
				continue
			}
			qualifiers := map[string]string{
				"group":        group,
				"artifact":     artifact,
				"base_version": version,
				"classifier":   sv.Classifier,
			}
			props := map[string]string{
				"filename":  filename,
				"extension": sv.Extension,
				"updated":   sv.Updated,
			}
			artifacts = append(artifacts, runtime.NewArtifact(runtime.ArtifactSpec{
				Format:     "maven",
				Kind:       runtime.KindArtifact,
				Namespace:  group,
				Name:       group + ":" + artifact,
				Version:    version,
				Path:       basePath,
				Filename:   filename,
				RemotePath: basePath + "/" + filename,
				Extension:  "." + strings.TrimPrefix(sv.Extension, "."),
				Qualifiers: qualifiers,
				Properties: props,
			}))
		}
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

func mavenSnapshotFilename(artifact string, sv mavenSnapshotVersionXML) string {
	ext := strings.TrimPrefix(sv.Extension, ".")
	if artifact == "" || sv.Value == "" || ext == "" {
		return ""
	}
	name := artifact + "-" + sv.Value
	if sv.Classifier != "" {
		name += "-" + sv.Classifier
	}
	return name + "." + ext
}

func parseMavenLastUpdated(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("empty lastUpdated")
	}
	layouts := []string{"20060102150405", "200601021504"}
	var lastErr error
	for _, layout := range layouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func parsePOMLicense(body []byte) string {
	var pom pomProject
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&pom); err != nil {
		return ""
	}
	for _, license := range pom.Licenses {
		name := strings.TrimSpace(license.Name)
		if name != "" {
			return name
		}
	}
	return ""
}

func (p *MavenPlugin) Name() string {
	return "maven"
}

func (p *MavenPlugin) NormalizeAsset(ctx context.Context, input runtime.NormalizeInput) (*runtime.Artifact, error) {
	path := strings.Trim(input.RemotePath, "/")
	var key runtime.ArtifactKey
	var err error
	if strings.HasSuffix(path, "/maven-metadata.xml") {
		key, err = p.parseMavenMetadataPath(path)
	} else {
		key, err = p.parseMavenPath(path)
	}
	if err != nil {
		return nil, err
	}
	return runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: input.RepositoryID,
		Format:       key.Format,
		Kind:         key.Kind,
		Namespace:    key.Namespace,
		Name:         key.Name,
		Version:      key.Version,
		Path:         key.Path,
		Filename:     key.Filename,
		RemotePath:   key.RemotePath,
		Extension:    key.Extension,
		ContentType:  input.ContentType,
		SizeBytes:    input.SizeBytes,
		Checksums:    input.Checksums,
		Qualifiers:   key.Qualifiers,
		Attributes:   input.Attributes,
		BlobRefs:     input.BlobRefs,
	}), nil
}

func (p *MavenPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "maven-metadata.xml") {
		if ctx.Request.Method == http.MethodGet || ctx.Request.Method == http.MethodHead {
			return p.handleMetadata(ctx, repoRuntime, path)
		}
		if ctx.Request.Method == http.MethodPut {
			key, err := p.parseMavenMetadataPath(path)
			if err != nil {
				http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
				return nil
			}
			key.RepositoryID = ctx.Repository.ID
			ctx.PackageName = key.Name
			ctx.Version = key.Version
			ctx.Filename = key.Filename
			return p.handleUpload(ctx, repoRuntime, key)
		}
	}

	if metaPath, originalFile, algo, ok := parseMavenMetadataChecksum(path); ok {
		metaKey, err := p.parseMavenMetadataPath(metaPath)
		if err != nil {
			http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
			return nil
		}
		metaKey.RepositoryID = ctx.Repository.ID
		if ctx.Request.Method == http.MethodPut {
			checksumKey := metaKey
			checksumKey.Kind = runtime.KindChecksum
			checksumKey.Filename = filepath.Base(path)
			checksumKey.Extension = filepath.Ext(checksumKey.Filename)
			checksumKey.RemotePath = strings.Trim(path, "/")
			checksumKey.Path = strings.TrimSuffix(checksumKey.RemotePath, "/"+checksumKey.Filename)
			ctx.PackageName = checksumKey.Name
			ctx.Version = checksumKey.Version
			ctx.Filename = checksumKey.Filename
			return p.handleChecksumUpload(ctx, repoRuntime, checksumKey, originalFile, algo)
		}
		return p.handleChecksumDownload(ctx, repoRuntime, metaKey, originalFile, algo)
	}

	key, err := p.parseMavenPath(path)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}
	key.RepositoryID = ctx.Repository.ID

	ctx.PackageName = key.Name
	ctx.Version = key.Version
	ctx.Filename = key.Filename

	switch ctx.Request.Method {
	case http.MethodGet, http.MethodHead:
		// 拦截 checksum 文件请求（.sha1/.md5/.sha256），动态计算并返回
		if originalFile, algo, ok := parseChecksumRequest(key.Filename); ok {
			return p.handleChecksumDownload(ctx, repoRuntime, key, originalFile, algo)
		}
		return p.handleDownload(ctx, repoRuntime, key)
	case http.MethodPut:
		if originalFile, algo, ok := parseChecksumRequest(key.Filename); ok {
			key.Kind = runtime.KindChecksum
			return p.handleChecksumUpload(ctx, repoRuntime, key, originalFile, algo)
		}
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

type snapshotFileInfo struct {
	ext        string
	classifier string
	timestamp  string
	buildNum   string
}

// parseSnapshotFileInfo 从 SNAPSHOT 文件名中解析 timestamp、buildNumber、extension、classifier。
// 文件名格式: {artifactId}-{baseVersion}-{timestamp}-{buildNumber}[-classifier].{ext}
// 例如: my-lib-1.0-20260603.120000-1.jar / my-lib-1.0-20260603.120000-1-sources.jar
// artifact 为 artifactId，version 为完整 SNAPSHOT 版本（如 1.0-SNAPSHOT）。
// 解析失败（文件名不符合 SNAPSHOT 时间戳格式）返回 ok=false。
func parseSnapshotFileInfo(artifact, version, filename string) (info snapshotFileInfo, ok bool) {
	if filename == "" || strings.Contains(filename, "maven-metadata") {
		return snapshotFileInfo{}, false
	}
	baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
	prefix := artifact + "-" + baseVersion + "-"
	if !strings.HasPrefix(filename, prefix) {
		return snapshotFileInfo{}, false
	}
	rest := strings.TrimPrefix(filename, prefix)
	// rest: 20260603.033633-3.jar 或 20260603.033633-3-sources.jar
	parts := strings.SplitN(rest, "-", 3)
	if len(parts) < 2 {
		return snapshotFileInfo{}, false
	}
	ts := parts[0]       // 20260603.033633
	buildNum := parts[1] // 3 或 3.jar
	// 去掉扩展名
	if idx := strings.Index(buildNum, "."); idx >= 0 {
		buildNum = buildNum[:idx]
	}
	if ts == "" || buildNum == "" {
		return snapshotFileInfo{}, false
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
	return snapshotFileInfo{
		ext:        strings.TrimPrefix(ext, "."),
		classifier: classifier,
		timestamp:  ts,
		buildNum:   buildNum,
	}, true
}

// buildSnapshotMetadata 构建 SNAPSHOT metadata 的 <snapshot> 和 <snapshotVersions> 部分。
// 从 artifacts 文件名中解析时间戳，选出最新一组（同 timestamp+buildNumber）产物，
// 生成与路径A一致的 metadata 结构。lastUpdated 由调用方传入（14 位 YYYYMMDDHHmmss）。
// 返回 (snapshotBlock, snapshotVersionsBlock)。若无法解析出任何有效时间戳，两者均为 nil。
func buildSnapshotMetadata(artifact, version string, artifacts []*runtime.Artifact, lastUpdated string) (*mavenSnapshotXML, *mavenSnapshotVersXML) {
	var latestTimestamp, latestBuildNum string
	var snapshotFiles []snapshotFileInfo

	for _, a := range artifacts {
		info, ok := parseSnapshotFileInfo(artifact, version, a.Filename)
		if !ok {
			continue
		}
		if compareMavenSnapshotBuild(info.timestamp, info.buildNum, latestTimestamp, latestBuildNum) > 0 {
			latestTimestamp = info.timestamp
			latestBuildNum = info.buildNum
		}
		snapshotFiles = append(snapshotFiles, info)
	}

	if latestTimestamp == "" || latestBuildNum == "" {
		return nil, nil
	}

	baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
	value := baseVersion + "-" + latestTimestamp + "-" + latestBuildNum

	snapBlock := &mavenSnapshotXML{
		Timestamp:   latestTimestamp,
		BuildNumber: latestBuildNum,
	}

	// 生成 snapshotVersions：只保留最新一组（同 timestamp+buildNumber），同 ext+classifier 去重
	snapshotItems := make([]mavenSnapshotVersionXML, 0)
	seenItems := make(map[string]struct{})
	for _, info := range snapshotFiles {
		if info.timestamp != latestTimestamp || info.buildNum != latestBuildNum {
			continue
		}
		itemKey := info.ext + "/" + info.classifier
		if _, seen := seenItems[itemKey]; seen {
			continue
		}
		seenItems[itemKey] = struct{}{}
		snapshotItems = append(snapshotItems, mavenSnapshotVersionXML{
			Extension:  info.ext,
			Classifier: info.classifier,
			Value:      value,
			Updated:    lastUpdated,
		})
	}
	if len(snapshotItems) == 0 {
		return snapBlock, nil
	}
	sort.Slice(snapshotItems, func(i, j int) bool {
		if snapshotItems[i].Extension != snapshotItems[j].Extension {
			return snapshotItems[i].Extension < snapshotItems[j].Extension
		}
		return snapshotItems[i].Classifier < snapshotItems[j].Classifier
	})
	return snapBlock, &mavenSnapshotVersXML{Items: snapshotItems}
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

	// 注意：不在此处提前调用 GetArtifact 短路返回。
	// GetArtifact 会将 metadata.xml 作为普通文件缓存（kind=artifact），但不解析
	// version 列表，导致 packages 表的 version_count=0，搜索不到该包。
	// 统一走下方的 QueryArtifacts 路径，由 FetchRemote 解析 XML 并写入 version
	// artifacts，确保搜索数据一致性。若 QueryArtifacts 未返回 version 记录，
	// 下方 !hasVersionArtifacts 分支仍有 GetArtifact 作为 fallback。
	metaKey := runtime.ArtifactKey{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
		Kind:         runtime.KindMetadata,
		Namespace:    group,
		Name:         group + ":" + artifact,
		Version:      version,
		Path:         strings.TrimSuffix(strings.Trim(path, "/"), "/maven-metadata.xml"),
		Filename:     "maven-metadata.xml",
		RemotePath:   strings.Trim(path, "/"),
		Qualifiers: map[string]string{
			"group":    group,
			"artifact": artifact,
		},
	}

	query := runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
		Namespace:    group,
		Name:         group + ":" + artifact,
		Version:      version,
		RemotePath:   path, // 必须带 RemotePath，供 FetchRemote 回源使用
		Qualifiers: map[string]string{
			"group":    group,
			"artifact": artifact,
		},
	}
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), query)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			// 继续走下方的 hasVersionArtifacts 逻辑，尝试 GetArtifact 或返回 404
			artifacts = nil
		} else {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return err
		}
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

	if hasVersionArtifacts {
		// QueryArtifacts above keeps the derived version index fresh. If the
		// runtime also has the original metadata document, serve it verbatim so
		// proxy responses preserve upstream Maven semantics.
		if metaArtifact, metaErr := repoRuntime.GetArtifact(ctx.Request.Context(), metaKey); metaErr == nil && metaArtifact.Content != nil {
			defer metaArtifact.Content.Close()
			ctx.FromCache = metaArtifact.FromCache
			ctx.RemoteURL = metaArtifact.RemoteURL
			ctx.SizeBytes = metaArtifact.SizeBytes
			if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, metaArtifact, "", "application/xml", "inline"); err != nil {
				logrus.WithError(err).Warn("failed to write maven metadata content to client")
			}
			return nil
		}
	}

	if !hasVersionArtifacts {
		// For SNAPSHOT version metadata in local repos: aggregate from uploaded artifacts
		// to generate dynamic metadata with correct timestamp and buildNumber.
		if version != "" && strings.Contains(version, "-SNAPSHOT") {
			// 查询所有已上传的 SNAPSHOT artifacts。该查询只用于 hosted/local
			// 动态 metadata 聚合，不作为 proxy 回源入口。
			snapQuery := runtime.ArtifactQuery{
				RepositoryID: ctx.Repository.ID,
				Format:       "maven",
				Namespace:    group,
				Name:         group + ":" + artifact,
				Version:      version,
				Qualifiers: map[string]string{
					"group":    group,
					"artifact": artifact,
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
				snapBlock, snapVersionsBlock := buildSnapshotMetadata(artifact, version, snapArtifacts, "")
				if snapBlock != nil {
					// lastUpdated 格式: YYYYMMDDHHmmss（14 位），由 timestamp 去掉点号得到
					lastUpdated := strings.ReplaceAll(snapBlock.Timestamp, ".", "")
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
							Snapshot: snapBlock,
						},
					}
					meta.Versioning.SnapshotVersions = snapVersionsBlock

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

		if !hasVersionArtifacts && version == "" {
			// Hosted/local fallback: aggregate uploaded artifacts to render
			// artifact-level metadata when no fetched version metadata exists.
			artifactArts, qErr := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
				RepositoryID: ctx.Repository.ID,
				Format:       "maven",
				Namespace:    group,
				Name:         group + ":" + artifact,
				Qualifiers: map[string]string{
					"group":    group,
					"artifact": artifact,
				},
			})
			if qErr == nil && len(artifactArts) > 0 {
				versionSet := map[string]struct{}{}
				var lastTime time.Time
				for _, a := range artifactArts {
					v := a.Version
					if v == "" {
						continue
					}
					versionSet[v] = struct{}{}
					if a.CreatedAt.After(lastTime) {
						lastTime = a.CreatedAt
					}
				}
				versions := make([]string, 0, len(versionSet))
				for v := range versionSet {
					versions = append(versions, v)
				}
				sortMavenVersions(versions)
				if len(versions) > 0 {
					lastUpdated := lastTime.UTC().Format("20060102150405")
					meta := mavenMetadata{
						Model:      "1.1.0",
						GroupID:    group,
						ArtifactID: artifact,
						Version:    version,
						Versioning: mavenVersioningXML{
							Latest:   versions[len(versions)-1],
							Release:  versions[len(versions)-1],
							Versions: mavenVersionsXML{Items: versions},
							LastU:    lastUpdated,
						},
					}
					body, _ := xml.MarshalIndent(meta, "", "  ")
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
		if metaArtifact, metaErr := repoRuntime.GetArtifact(ctx.Request.Context(), metaKey); metaErr == nil && metaArtifact.Content != nil {
			defer metaArtifact.Content.Close()
			ctx.FromCache = metaArtifact.FromCache
			ctx.RemoteURL = metaArtifact.RemoteURL
			ctx.SizeBytes = metaArtifact.SizeBytes
			if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, metaArtifact, "", "application/xml", "inline"); err != nil {
				logrus.WithError(err).Warn("failed to write maven metadata content to client")
			}
			return nil
		}
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}

	versionSet := map[string]struct{}{}
	for _, a := range artifacts {
		v := a.Version
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
	sortMavenVersions(versions)
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
		// 从 artifact 文件名中解析真实的 timestamp 和 buildNumber，
		// 生成与 version 级 metadata 一致的 <snapshot> 和 <snapshotVersions>。
		snapBlock, snapVersionsBlock := buildSnapshotMetadata(artifact, version, artifacts, lastUpdated)
		if snapBlock != nil {
			meta.Versioning.Snapshot = snapBlock
			meta.Versioning.SnapshotVersions = snapVersionsBlock
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

func sortMavenVersions(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		return compareMavenVersion(versions[i], versions[j]) < 0
	})
}

func compareMavenSnapshotBuild(tsA, buildA, tsB, buildB string) int {
	if tsA != tsB {
		if tsA < tsB {
			return -1
		}
		return 1
	}
	numA, errA := strconv.Atoi(buildA)
	numB, errB := strconv.Atoi(buildB)
	if errA == nil && errB == nil && numA != numB {
		if numA < numB {
			return -1
		}
		return 1
	}
	if buildA < buildB {
		return -1
	}
	if buildA > buildB {
		return 1
	}
	return 0
}

func compareMavenVersion(a, b string) int {
	ta := splitMavenVersionTokens(a)
	tb := splitMavenVersionTokens(b)
	max := len(ta)
	if len(tb) > max {
		max = len(tb)
	}
	for i := 0; i < max; i++ {
		if i >= len(ta) {
			if tb[i].num && tb[i].value == "0" {
				continue
			}
			return compareMavenToken(mavenVersionToken{value: "", num: false}, tb[i])
		}
		if i >= len(tb) {
			if ta[i].num && ta[i].value == "0" {
				continue
			}
			return compareMavenToken(ta[i], mavenVersionToken{value: "", num: false})
		}
		if cmp := compareMavenToken(ta[i], tb[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func compareMavenToken(a, b mavenVersionToken) int {
	if a.num && b.num {
		return compareNumericString(a.value, b.value)
	}
	if a.num != b.num {
		if a.num {
			return 1
		}
		return -1
	}
	if cmp := compareMavenQualifier(a.value, b.value); cmp != 0 {
		return cmp
	}
	if a.value < b.value {
		return -1
	}
	if a.value > b.value {
		return 1
	}
	return 0
}

func compareMavenQualifier(a, b string) int {
	ra, oka := mavenQualifierRank(a)
	rb, okb := mavenQualifierRank(b)
	if oka || okb {
		if !oka {
			ra = 8
		}
		if !okb {
			rb = 8
		}
		if ra < rb {
			return -1
		}
		if ra > rb {
			return 1
		}
		return 0
	}
	return 0
}

func mavenQualifierRank(q string) (int, bool) {
	switch q {
	case "alpha", "a":
		return 1, true
	case "beta", "b":
		return 2, true
	case "milestone", "m":
		return 3, true
	case "rc", "cr":
		return 4, true
	case "snapshot":
		return 5, true
	case "", "ga", "final", "release":
		return 6, true
	case "sp":
		return 7, true
	default:
		return 0, false
	}
}

type mavenVersionToken struct {
	value string
	num   bool
}

func splitMavenVersionTokens(version string) []mavenVersionToken {
	var tokens []mavenVersionToken
	var b strings.Builder
	inNum := false
	hasToken := false
	flush := func() {
		if !hasToken {
			return
		}
		tokens = append(tokens, mavenVersionToken{value: b.String(), num: inNum})
		b.Reset()
		hasToken = false
	}
	for _, r := range strings.ToLower(version) {
		isNum := r >= '0' && r <= '9'
		if r == '.' || r == '-' || r == '_' {
			flush()
			continue
		}
		if hasToken && isNum != inNum {
			flush()
		}
		inNum = isNum
		hasToken = true
		b.WriteRune(r)
	}
	flush()
	return tokens
}

func compareNumericString(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if a == "" {
		a = "0"
	}
	if b == "" {
		b = "0"
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
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

func parseMavenMetadataChecksum(path string) (metadataPath, originalFile string, algo checksumAlgo, ok bool) {
	originalFile, algo, ok = parseChecksumRequest(filepath.Base(path))
	if !ok || originalFile != "maven-metadata.xml" {
		return "", "", "", false
	}
	metadataPath = strings.TrimSuffix(path, "."+string(algo))
	return metadataPath, originalFile, algo, true
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
		Format:     "maven",
		Kind:       runtime.KindArtifact,
		Namespace:  group,
		Name:       group + ":" + artifact,
		Version:    version,
		Path:       strings.TrimSuffix(path, "/"+filename),
		Filename:   filename,
		RemotePath: path,
		Qualifiers: map[string]string{
			"group":      group,
			"artifact":   artifact,
			"classifier": classifier,
		},
		Extension: ext,
	}, nil
}

func (p *MavenPlugin) parseMavenMetadataPath(path string) (runtime.ArtifactKey, error) {
	clean := strings.Trim(path, "/")
	if !strings.HasSuffix(clean, "/maven-metadata.xml") {
		return runtime.ArtifactKey{}, errors.New("invalid maven metadata path")
	}
	base := strings.TrimSuffix(clean, "/maven-metadata.xml")
	parts := strings.Split(base, "/")
	if len(parts) < 2 {
		return runtime.ArtifactKey{}, errors.New("invalid maven metadata path")
	}

	version := ""
	artifact := parts[len(parts)-1]
	groupParts := parts[:len(parts)-1]
	if len(parts) >= 3 && strings.Contains(parts[len(parts)-1], "-SNAPSHOT") {
		version = parts[len(parts)-1]
		artifact = parts[len(parts)-2]
		groupParts = parts[:len(parts)-2]
	}
	if len(groupParts) == 0 || artifact == "" {
		return runtime.ArtifactKey{}, errors.New("invalid maven metadata path")
	}
	group := strings.Join(groupParts, ".")

	return runtime.ArtifactKey{
		Format:     "maven",
		Kind:       runtime.KindMetadata,
		Namespace:  group,
		Name:       group + ":" + artifact,
		Version:    version,
		Path:       base,
		Filename:   "maven-metadata.xml",
		RemotePath: clean,
		Qualifiers: map[string]string{
			"group":    group,
			"artifact": artifact,
		},
		Extension: ".xml",
	}, nil
}

func (p *MavenPlugin) handleChecksumDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey, originalFile string, algo checksumAlgo) error {
	// 构建原始 artifact 的 key（用原始文件名替换 checksum 文件名）
	originalKey := key
	originalKey.Filename = originalFile
	originalKey.Extension = filepath.Ext(originalFile)
	originalKey.RemotePath = strings.TrimSuffix(key.RemotePath, "/"+key.Filename) + "/" + originalFile

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), originalKey)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) && originalFile == "maven-metadata.xml" {
			metaXML := p.buildDynamicMetadata(ctx, repoRuntime, key)
			if metaXML != nil {
				digest, err := computeChecksum(strings.NewReader(string(metaXML)), algo)
				if err == nil {
					ctx.Writer.Header().Set("Content-Type", "text/plain")
					ctx.Writer.WriteHeader(http.StatusOK)
					_, _ = ctx.Writer.Write([]byte(formatMavenChecksum(digest, originalFile)))
					return nil
				}
			}
		}
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return err
		}
		return nil
	}
	if artifact.Content == nil {
		http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		return nil
	}
	defer artifact.Content.Close()

	digest, err := computeChecksum(artifact.Content, algo)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"filename": key.Filename,
			"algo":     string(algo),
		}).Error("maven: compute checksum failed")
		http.Error(ctx.Writer, "internal error", http.StatusInternalServerError)
		return nil
	}

	ctx.FromCache = artifact.FromCache
	ctx.RemoteURL = artifact.RemoteURL
	if len(artifact.BlobRefs) > 0 {
		ctx.SizeBytes = artifact.BlobRefs[0].Size
	}

	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write([]byte(formatMavenChecksum(digest, originalFile)))
	return nil
}

func (p *MavenPlugin) buildDynamicMetadata(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) []byte {
	group := key.Qualifiers["group"]
	artifact := key.Qualifiers["artifact"]
	if group == "" || artifact == "" {
		return nil
	}
	// Hosted/local fallback only: build metadata from uploaded artifacts after
	// direct metadata lookup failed.
	arts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
		Namespace:    group,
		Name:         group + ":" + artifact,
		Qualifiers: map[string]string{
			"group":    group,
			"artifact": artifact,
		},
	})
	if err != nil || len(arts) == 0 {
		return nil
	}
	versionSet := map[string]struct{}{}
	var lastTime time.Time
	for _, a := range arts {
		if a.Version == "" {
			continue
		}
		versionSet[a.Version] = struct{}{}
		if a.CreatedAt.After(lastTime) {
			lastTime = a.CreatedAt
		}
	}
	versions := make([]string, 0, len(versionSet))
	for v := range versionSet {
		versions = append(versions, v)
	}
	sortMavenVersions(versions)
	if len(versions) == 0 {
		return nil
	}
	lastUpdated := lastTime.UTC().Format("20060102150405")
	meta := mavenMetadata{
		Model:      "1.1.0",
		GroupID:    group,
		ArtifactID: artifact,
		Versioning: mavenVersioningXML{
			Latest:   versions[len(versions)-1],
			Release:  versions[len(versions)-1],
			Versions: mavenVersionsXML{Items: versions},
			LastU:    lastUpdated,
		},
	}
	body, err := xml.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil
	}
	return append([]byte(xml.Header), body...)
}

func (p *MavenPlugin) handleDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	if err != nil {
		if errors.Is(err, runtime.ErrNotFound) {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			// 其他错误（含 ErrBlocked）交给 router 处理
			return err
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

	if err := runtime.ServeArtifactContent(ctx.Writer, ctx.Request, artifact, key.Filename, "application/octet-stream", "inline"); err != nil {
		logrus.WithError(err).Warn("failed to write artifact content to client")
		return nil
	}
	return nil
}

func (p *MavenPlugin) handleUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey) error {
	// 检查文件是否已存在，用于决定返回 201 还是 200
	existingArtifact, _ := repoRuntime.GetArtifact(ctx.Request.Context(), key)
	isUpdate := existingArtifact != nil
	if existingArtifact != nil && existingArtifact.Content != nil {
		_ = existingArtifact.Content.Close()
	}

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

	body := ctx.Request.Body
	attributes := map[string]string{}
	properties := map[string]string{
		"filename":  key.Filename,
		"extension": key.Extension,
		"group":     key.Qualifiers["group"],
		"artifact":  key.Qualifiers["artifact"],
		"version":   key.Version,
	}
	for _, qualifierKey := range []string{"checksum_algorithm", "checksum_for"} {
		if value := key.Qualifiers[qualifierKey]; value != "" {
			properties[qualifierKey] = value
		}
	}
	if strings.EqualFold(key.Extension, ".pom") || strings.HasSuffix(strings.ToLower(key.Filename), ".pom") {
		bodyBytes, readErr := io.ReadAll(ctx.Request.Body)
		if readErr != nil {
			session.Abort(ctx.Request.Context())
			http.Error(ctx.Writer, readErr.Error(), http.StatusInternalServerError)
			return nil
		}
		body = io.NopCloser(bytes.NewReader(bodyBytes))
		if license := parsePOMLicense(bodyBytes); license != "" {
			attributes["license"] = license
			properties["license"] = license
		}
	}

	blobRef, err := session.PutBlob(ctx.Request.Context(), body)
	if err != nil {
		session.Abort(ctx.Request.Context())
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	kind := key.Kind
	if kind == "" {
		kind = runtime.KindArtifact
	}
	artifact := runtime.NewArtifact(runtime.ArtifactSpec{
		RepositoryID: ctx.Repository.ID,
		Format:       "maven",
		Kind:         kind,
		Namespace:    key.Namespace,
		Name:         key.Name,
		Version:      key.Version,
		Path:         key.Path,
		Filename:     key.Filename,
		RemotePath:   key.RemotePath,
		Extension:    key.Extension,
		BlobRefs:     []runtime.BlobRef{blobRef},
		Qualifiers:   key.Qualifiers,
		Attributes:   attributes,
		Properties:   properties,
	})

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

func (p *MavenPlugin) handleChecksumUpload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey, originalFile string, algo checksumAlgo) error {
	if key.Kind == "" {
		key.Kind = runtime.KindChecksum
	}
	if key.Qualifiers == nil {
		key.Qualifiers = map[string]string{}
	}
	key.Qualifiers["checksum_algorithm"] = string(algo)
	key.Qualifiers["checksum_for"] = originalFile
	if key.Extension == "" {
		key.Extension = filepath.Ext(key.Filename)
	}
	return p.handleUpload(ctx, repoRuntime, key)
}
