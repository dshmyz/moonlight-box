package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	mavenplugin "github.com/dshmyz/moonlight-box/internal/plugins/maven"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PackageVersionHandler struct {
	db                       *gorm.DB
	artifactSvc              *service.ArtifactService
	packageVersionTableOnce  sync.Once
	packageVersionTableReady bool
}

func NewPackageVersionHandler(db *gorm.DB) *PackageVersionHandler {
	return NewPackageVersionHandlerWithArtifactService(db, service.NewArtifactService(db))
}

func NewPackageVersionHandlerWithArtifactService(db *gorm.DB, artifactSvc *service.ArtifactService) *PackageVersionHandler {
	return &PackageVersionHandler{db: db, artifactSvc: artifactSvc}
}

type blobInfo struct {
	ArtifactID uint
	Size       int64
	Digest     string
}

type packageVersionCoordinateRequest struct {
	RepositoryID uint   `json:"repository_id"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Reason       string `json:"reason"`
}

func (h *PackageVersionHandler) ListVersions(c *gin.Context) {
	pkgType := c.Param("type")
	pkgName := c.Query("name")

	if pkgName == "" {
		response.BadRequest(c, "missing package name", "please provide package name via 'name' query parameter")
		return
	}

	var repositoryID uint
	if repoIDParam := c.Query("repository_id"); repoIDParam != "" {
		repoID, err := strconv.ParseUint(repoIDParam, 10, 32)
		if err != nil {
			response.BadRequest(c, "invalid repository ID", "repository_id must be a positive integer")
			return
		}
		repositoryID = uint(repoID)
	}

	db := h.db.WithContext(c.Request.Context()).Model(&model.Artifact{}).
		Where("format = ?", pkgType)

	db = db.Where("name = ?", pkgName)

	if repositoryID > 0 {
		db = db.Where("repository_id = ?", repositoryID)
	}

	versionSummaries, _ := h.loadPackageVersionSummaries(c.Request.Context(), pkgType, pkgName, repositoryID)
	versionSummaryMap := make(map[string]model.PackageVersion, len(versionSummaries))

	var artifacts []model.Artifact
	if err := db.Order("created_at DESC").Find(&artifacts).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 批量加载仓库名称（用于构造下载 URL）
	repoNameMap := make(map[uint]string)
	{
		repoIDs := make(map[uint]bool)
		for _, a := range artifacts {
			repoIDs[a.RepositoryID] = true
		}
		if len(repoIDs) > 0 {
			ids := make([]uint, 0, len(repoIDs))
			for id := range repoIDs {
				ids = append(ids, id)
			}
			var repos []model.Repository
			h.db.Where("id IN ?", ids).Find(&repos)
			for _, r := range repos {
				repoNameMap[r.ID] = r.Name
			}
		}
	}

	// 批量获取 blob refs
	artifactIDs := make([]uint, len(artifacts))
	for i, a := range artifacts {
		artifactIDs[i] = a.ID
	}
	blobMap := make(map[uint][]blobInfo)
	if len(artifactIDs) > 0 {
		var blobRows []struct {
			ArtifactID uint
			Size       int64
			Digest     string
		}
		h.db.Table("artifact_blobs AS ab").
			Select("ab.artifact_id, b.size, b.digest").
			Joins("JOIN blobs b ON b.id = ab.blob_id").
			Where("ab.artifact_id IN ?", artifactIDs).
			Scan(&blobRows)
		for _, br := range blobRows {
			blobMap[br.ArtifactID] = append(blobMap[br.ArtifactID], blobInfo{
				ArtifactID: br.ArtifactID, Size: br.Size, Digest: br.Digest,
			})
		}
	}

	// 批量获取版本级下载计数（从 download_logs 聚合）
	downloadCountMap := make(map[string]int64) // key: "repoID|version"
	if len(artifacts) > 0 {
		repoIDsForCount := make([]uint, 0)
		repoIDSet := make(map[uint]bool)
		for _, a := range artifacts {
			if !repoIDSet[a.RepositoryID] {
				repoIDSet[a.RepositoryID] = true
				repoIDsForCount = append(repoIDsForCount, a.RepositoryID)
			}
		}
		var countRows []struct {
			RepositoryID uint
			Version      string
			Count        int64
		}
		err := h.db.Table("download_logs").
			Select("repository_id, version, COUNT(*) as count").
			Where("repository_id IN ? AND package_type = ? AND package_name = ? AND status IN ?",
				repoIDsForCount, pkgType, pkgName, []string{"success", "cached"}).
			Where("version != ''").
			Group("repository_id, version").
			Scan(&countRows).Error
		if err == nil {
			for _, cr := range countRows {
				key := fmt.Sprintf("%d|%s", cr.RepositoryID, cr.Version)
				downloadCountMap[key] += cr.Count
			}
		}
	}

	// 按版本号分组，聚合文件列表
	type versionGroup struct {
		id          uint
		latestAt    time.Time
		publishedAt string
		license     string
		triggerIP   string
		name        string
		namespace   string
		identityKey string
		attributes  model.JSONB
		qualifiers  model.JSONB
		metadata    model.JSONB
		files       []gin.H
		blobs       []blobInfo
		repoIDs     map[uint]bool // 该版本涉及的仓库 ID 集合
		sizeBytes   int64         // 从 artifact 元数据记录的大小（回源时即有，无需下载 blob）
		sha256      string
		summary     *model.PackageVersion
	}
	verGroups := make(map[string]*versionGroup)
	var verOrder []string
	for _, summary := range versionSummaries {
		s := summary
		versionSummaryMap[summary.Version] = summary
		verGroups[summary.Version] = &versionGroup{
			latestAt:  summary.LatestArtifactAt,
			name:      summary.PackageName,
			namespace: summary.Namespace,
			license:   summary.License,
			repoIDs:   map[uint]bool{summary.RepositoryID: true},
			sizeBytes: summary.SizeBytes,
			sha256:    summary.ChecksumSHA256,
			summary:   &s,
		}
		verOrder = append(verOrder, summary.Version)
	}

	for _, a := range artifacts {
		version := a.Version
		if version == "" {
			continue
		}

		vp, ok := verGroups[version]
		if !ok {
			vp = &versionGroup{repoIDs: make(map[uint]bool)}
			if summary, hasSummary := versionSummaryMap[version]; hasSummary {
				s := summary
				vp.summary = &s
			}
			verGroups[version] = vp
			verOrder = append(verOrder, version)
		}
		vp.repoIDs[a.RepositoryID] = true
		if vp.id == 0 {
			vp.id = a.ID
		}
		if a.CreatedAt.After(vp.latestAt) {
			vp.latestAt = a.CreatedAt
			vp.id = a.ID
		}
		if vp.sizeBytes == 0 && a.SizeBytes > 0 {
			vp.sizeBytes = a.SizeBytes
		}
		if vp.sha256 == "" {
			vp.sha256 = jsonbString(a.Checksums, "sha256")
		}
		if vp.name == "" {
			vp.name = a.Name
		}
		if vp.namespace == "" {
			vp.namespace = a.Namespace
		}
		if vp.identityKey == "" {
			vp.identityKey = a.IdentityKey
		}
		mergeJSONB(vp.attributes, a.Attributes)
		if vp.attributes == nil && len(a.Attributes) > 0 {
			vp.attributes = cloneJSONB(a.Attributes)
		}
		mergeJSONB(vp.qualifiers, a.Qualifiers)
		if vp.qualifiers == nil && len(a.Qualifiers) > 0 {
			vp.qualifiers = cloneJSONB(a.Qualifiers)
		}
		mergeJSONB(vp.metadata, a.Metadata)
		if vp.metadata == nil && len(a.Metadata) > 0 {
			vp.metadata = cloneJSONB(a.Metadata)
		}
		if vp.publishedAt == "" && a.Attributes != nil {
			if pa, ok := a.Attributes["published_at"]; ok {
				if s, ok := pa.(string); ok && s != "" {
					vp.publishedAt = s
				}
			}
		}
		if vp.license == "" && a.Attributes != nil {
			if lic, ok := a.Attributes["license"]; ok {
				if s, ok := lic.(string); ok && s != "" {
					vp.license = s
				}
			}
		}
		if vp.triggerIP == "" && a.Metadata != nil {
			if tip, ok := a.Metadata["trigger_ip"]; ok {
				if s, ok := tip.(string); ok && s != "" {
					vp.triggerIP = s
				}
			}
		}

		filename := a.Filename

		fileType := classifyFileType(a.Format, filename)

		blobs := blobMap[a.ID]
		for _, b := range blobs {
			vp.blobs = append(vp.blobs, b)
		}

		repoName := repoNameMap[a.RepositoryID]
		downloadURL := buildDownloadURL(repoName, a)

		if isDownloadableArtifact(a, filename, downloadURL, blobs) {
			vp.files = append(vp.files, gin.H{
				"id":           a.ID,
				"version_id":   a.ID,
				"filename":     filename,
				"file_type":    fileType,
				"storage_path": firstNonEmptyString(a.RemotePath, a.Path),
				"path":         a.Path,
				"remote_path":  a.RemotePath,
				"size_bytes":   firstNonZeroInt64(sumBlobSizes(blobs), a.SizeBytes),
				"checksum_sha256": firstNonEmptyString(
					firstBlobDigest(blobs),
					jsonbString(a.Checksums, "sha256"),
				),
				"download_url": downloadURL,
				"qualifiers":   a.Qualifiers,
				"attributes":   a.Attributes,
				"metadata":     a.Metadata,
			})
		}
	}

	versions := make([]gin.H, 0, len(verOrder))
	for _, ver := range verOrder {
		vp := verGroups[ver]
		totalSize := int64(0)
		for _, b := range vp.blobs {
			totalSize += b.Size
		}
		// blob 不存在时（未回源下载）回退到 artifact 元数据记录的大小
		totalSize = firstNonZeroInt64(totalSize, vp.sizeBytes)
		sha256 := ""
		if len(vp.blobs) > 0 && vp.blobs[0].Digest != "" {
			sha256 = vp.blobs[0].Digest
		}
		if sha256 == "" {
			sha256 = vp.sha256
		}

		publishedAt := vp.latestAt
		if vp.publishedAt != "" {
			if t, err := time.Parse(time.RFC3339, vp.publishedAt); err == nil {
				publishedAt = t
			}
		}
		// 如果 publishedAt 是零值（没有有效的时间信息），使用当前时间作为默认值
		if publishedAt.IsZero() {
			publishedAt = time.Now()
		}

		// 聚合该版本在所有仓库中的下载计数
		var downloadCount int64
		if vp.summary != nil {
			downloadCount = vp.summary.DownloadCount
		}
		if downloadCount == 0 {
			for repoID := range vp.repoIDs {
				key := fmt.Sprintf("%d|%s", repoID, ver)
				downloadCount += downloadCountMap[key]
			}
		}
		if isMavenPackageType(pkgType) {
			decorateMavenSnapshotDisplayAttributes(ver, mavenArtifactID(vp.name, vp.qualifiers), vp.files)
		}

		status := "published"
		filesDownloaded := len(vp.blobs) > 0
		if vp.summary != nil {
			if vp.summary.Status != "" {
				status = vp.summary.Status
			}
			if vp.summary.PublishedAt != nil {
				publishedAt = *vp.summary.PublishedAt
			} else if !vp.summary.LatestArtifactAt.IsZero() {
				publishedAt = vp.summary.LatestArtifactAt
			}
			if vp.summary.SizeBytes > 0 {
				totalSize = vp.summary.SizeBytes
			}
			if vp.summary.ChecksumSHA256 != "" {
				sha256 = vp.summary.ChecksumSHA256
			}
			if vp.summary.License != "" {
				vp.license = vp.summary.License
			}
			filesDownloaded = vp.summary.FilesDownloaded
		}

		entry := gin.H{
			"id":               vp.id,
			"repository_id":    singleRepositoryID(vp.repoIDs),
			"version":          ver,
			"name":             vp.name,
			"namespace":        vp.namespace,
			"identity_key":     vp.identityKey,
			"status":           status,
			"published_at":     publishedAt,
			"size_bytes":       totalSize,
			"checksum_sha256":  sha256,
			"files":            vp.files,
			"files_downloaded": filesDownloaded,
			"download_count":   downloadCount,
			"attributes":       vp.attributes,
			"qualifiers":       vp.qualifiers,
			"metadata":         vp.metadata,
		}
		if vp.license != "" {
			entry["license"] = vp.license
		}
		if vp.triggerIP != "" {
			entry["trigger_ip"] = vp.triggerIP
		}
		versions = append(versions, entry)
	}

	response.Success(c, gin.H{
		"package_name": pkgName,
		"type":         pkgType,
		"versions":     versions,
	})
}

func (h *PackageVersionHandler) loadPackageVersionSummaries(ctx context.Context, pkgType, pkgName string, repositoryID uint) ([]model.PackageVersion, bool) {
	if !h.hasPackageVersionTable() {
		return nil, false
	}
	db := h.db.WithContext(ctx).Where("format = ? AND package_name = ?", pkgType, pkgName)
	if repositoryID > 0 {
		db = db.Where("repository_id = ?", repositoryID)
	}
	var summaries []model.PackageVersion
	if err := db.Order("latest_artifact_at DESC").Find(&summaries).Error; err != nil || len(summaries) == 0 {
		return nil, false
	}
	return summaries, true
}

func (h *PackageVersionHandler) hasPackageVersionTable() bool {
	h.packageVersionTableOnce.Do(func() {
		h.packageVersionTableReady = h.db.Migrator().HasTable(&model.PackageVersion{})
	})
	return h.packageVersionTableReady
}

func isMavenPackageType(pkgType string) bool {
	switch strings.ToLower(strings.TrimSpace(pkgType)) {
	case "maven", "maven2":
		return true
	default:
		return false
	}
}

func mavenArtifactID(name string, qualifiers model.JSONB) string {
	if artifact := jsonbString(qualifiers, "artifact"); artifact != "" {
		return artifact
	}
	if _, artifact, ok := strings.Cut(name, ":"); ok {
		return artifact
	}
	return name
}

func decorateMavenSnapshotDisplayAttributes(version, artifact string, files []gin.H) {
	if artifact == "" || !strings.HasSuffix(version, "-SNAPSHOT") || len(files) == 0 {
		return
	}

	filenames := make([]string, 0, len(files))
	for _, file := range files {
		filename, _ := file["filename"].(string)
		if filename != "" {
			filenames = append(filenames, filename)
		}
	}
	displays := mavenplugin.CurrentSnapshotFileDisplays(artifact, version, filenames)
	if len(displays) == 0 {
		return
	}

	for _, file := range files {
		filename, _ := file["filename"].(string)
		display, ok := displays[filename]
		if !ok {
			continue
		}
		attrs := cloneResponseAttributes(file["attributes"])
		delete(attrs, "default_visible")
		delete(attrs, "display_group")
		if display.Current {
			attrs["default_visible"] = "true"
			attrs["display_group"] = display.DisplayGroup
		}
		file["attributes"] = attrs
	}
}

func cloneResponseAttributes(value interface{}) map[string]interface{} {
	attrs := make(map[string]interface{})
	switch typed := value.(type) {
	case nil:
	case model.JSONB:
		for k, v := range typed {
			attrs[k] = v
		}
	case gin.H:
		for k, v := range typed {
			attrs[k] = v
		}
	case map[string]interface{}:
		for k, v := range typed {
			attrs[k] = v
		}
	}
	return attrs
}

func singleRepositoryID(repoIDs map[uint]bool) uint {
	if len(repoIDs) != 1 {
		return 0
	}
	for repoID := range repoIDs {
		return repoID
	}
	return 0
}

func classifyFileType(format, filename string) string {
	if strings.HasSuffix(filename, ".pom") {
		return "pom"
	}
	if strings.HasSuffix(filename, ".jar") {
		return "primary"
	}
	if strings.Contains(filename, "-sources") {
		return "sources"
	}
	if strings.Contains(filename, ".whl") || strings.Contains(filename, ".tar.gz") || strings.Contains(filename, ".tar.bz2") {
		return "primary"
	}
	if strings.HasSuffix(filename, ".mod") {
		return "metadata"
	}
	if strings.HasSuffix(filename, ".zip") {
		return "primary"
	}
	return "other"
}

func sumBlobSizes(blobs []blobInfo) int64 {
	var total int64
	for _, b := range blobs {
		total += b.Size
	}
	return total
}

func firstBlobDigest(blobs []blobInfo) string {
	for _, b := range blobs {
		if b.Digest != "" {
			return b.Digest
		}
	}
	return ""
}

func buildDownloadURL(repoName string, artifact model.Artifact) string {
	// 始终构造本地仓库路径，让下载请求经过代理层
	// artifact.DownloadURL 是外部 URL（如 files.pythonhosted.org），
	// 仅给后端 ProxyRuntime 做服务端回源使用，不应暴露给前端（CORS 问题）
	if artifact.RemotePath == "" {
		return ""
	}
	return "/repository/" + repoName + "/" + artifact.RemotePath
}

func isDownloadableArtifact(a model.Artifact, filename, downloadURL string, blobs []blobInfo) bool {
	if filename == "" && downloadURL == "" && len(blobs) == 0 {
		return false
	}
	if a.Kind == runtime.KindVersion || runtime.IsCatalogExcludedKind(a.Kind) || a.Kind == "release" {
		return false
	}
	if filename != "" || downloadURL != "" || len(blobs) > 0 {
		return true
	}
	return false
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func jsonbString(data model.JSONB, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok {
		return ""
	}
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return s
}

func cloneJSONB(src model.JSONB) model.JSONB {
	if len(src) == 0 {
		return nil
	}
	dst := make(model.JSONB, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeJSONB(dst model.JSONB, src model.JSONB) {
	if len(dst) == 0 || len(src) == 0 {
		return
	}
	for k, v := range src {
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
}

func (h *PackageVersionHandler) DeprecateVersion(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Reason = "deprecated"
	}

	var artifact model.Artifact
	if err := h.db.First(&artifact, uint(versionID)).Error; err != nil {
		response.NotFound(c, "version not found")
		return
	}

	if err := h.artifactSvc.UpdatePackageVersionStatus(c.Request.Context(), artifact.RepositoryID, artifact.Format, artifact.Name, artifact.Version, "deprecated", req.Reason); err != nil {
		h.writeVersionOperationError(c, err)
		return
	}

	response.Success(c, gin.H{
		"message": "version deprecated",
		"version": artifact.Version,
		"reason":  req.Reason,
	})
}

func (h *PackageVersionHandler) RestoreVersion(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	var artifact model.Artifact
	if err := h.db.First(&artifact, uint(versionID)).Error; err != nil {
		response.NotFound(c, "version not found")
		return
	}

	if err := h.artifactSvc.UpdatePackageVersionStatus(c.Request.Context(), artifact.RepositoryID, artifact.Format, artifact.Name, artifact.Version, "published", ""); err != nil {
		h.writeVersionOperationError(c, err)
		return
	}

	response.Success(c, gin.H{
		"message": "version restored",
		"version": artifact.Version,
	})
}

func (h *PackageVersionHandler) YankVersion(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Reason = "yanked"
	}

	var artifact model.Artifact
	if err := h.db.First(&artifact, uint(versionID)).Error; err != nil {
		response.NotFound(c, "version not found")
		return
	}

	if err := h.artifactSvc.UpdatePackageVersionStatus(c.Request.Context(), artifact.RepositoryID, artifact.Format, artifact.Name, artifact.Version, "yanked", req.Reason); err != nil {
		h.writeVersionOperationError(c, err)
		return
	}

	response.Success(c, gin.H{
		"message": "version yanked",
		"version": artifact.Version,
		"reason":  req.Reason,
	})
}

func (h *PackageVersionHandler) DeleteVersion(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	var artifact model.Artifact
	if err := h.db.First(&artifact, uint(versionID)).Error; err != nil {
		response.NotFound(c, "version not found")
		return
	}

	if err := h.artifactSvc.DeletePackageVersionByCoordinates(c.Request.Context(), artifact.RepositoryID, artifact.Format, artifact.Name, artifact.Version); err != nil {
		h.writeVersionOperationError(c, err)
		return
	}

	response.NoContent(c)
}

func (h *PackageVersionHandler) DeprecatePackageVersion(c *gin.Context) {
	req, ok := h.bindPackageVersionCoordinate(c)
	if !ok {
		return
	}
	if req.Reason == "" {
		req.Reason = "deprecated"
	}
	if err := h.artifactSvc.UpdatePackageVersionStatus(c.Request.Context(), req.RepositoryID, c.Param("type"), req.Name, req.Version, "deprecated", req.Reason); err != nil {
		h.writeVersionOperationError(c, err)
		return
	}
	response.Success(c, gin.H{
		"message": "version deprecated",
		"version": req.Version,
		"reason":  req.Reason,
	})
}

func (h *PackageVersionHandler) RestorePackageVersion(c *gin.Context) {
	req, ok := h.bindPackageVersionCoordinate(c)
	if !ok {
		return
	}
	if err := h.artifactSvc.UpdatePackageVersionStatus(c.Request.Context(), req.RepositoryID, c.Param("type"), req.Name, req.Version, "published", ""); err != nil {
		h.writeVersionOperationError(c, err)
		return
	}
	response.Success(c, gin.H{
		"message": "version restored",
		"version": req.Version,
	})
}

func (h *PackageVersionHandler) YankPackageVersion(c *gin.Context) {
	req, ok := h.bindPackageVersionCoordinate(c)
	if !ok {
		return
	}
	if req.Reason == "" {
		req.Reason = "yanked"
	}
	if err := h.artifactSvc.UpdatePackageVersionStatus(c.Request.Context(), req.RepositoryID, c.Param("type"), req.Name, req.Version, "yanked", req.Reason); err != nil {
		h.writeVersionOperationError(c, err)
		return
	}
	response.Success(c, gin.H{
		"message": "version yanked",
		"version": req.Version,
		"reason":  req.Reason,
	})
}

func (h *PackageVersionHandler) DeletePackageVersion(c *gin.Context) {
	req, ok := h.bindPackageVersionCoordinate(c)
	if !ok {
		return
	}
	if err := h.artifactSvc.DeletePackageVersionByCoordinates(c.Request.Context(), req.RepositoryID, c.Param("type"), req.Name, req.Version); err != nil {
		h.writeVersionOperationError(c, err)
		return
	}
	response.NoContent(c)
}

func (h *PackageVersionHandler) DeletePackage(c *gin.Context) {
	packageID, err := strconv.ParseUint(packageIDParam(c), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid package ID", "package ID must be a positive integer")
		return
	}

	repositoryID, format, name, err := h.resolvePackageDeleteTarget(c, uint(packageID))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(c, "package not found")
		return
	}
	if err != nil {
		response.BadRequest(c, "invalid package delete request", err.Error())
		return
	}

	if err := h.artifactSvc.DeletePackageByCoordinates(c.Request.Context(), repositoryID, format, name); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "package deleted",
	})
}

func packageIDParam(c *gin.Context) string {
	if id := c.Param("id"); id != "" {
		return id
	}
	return c.Param("type")
}

func (h *PackageVersionHandler) bindPackageVersionCoordinate(c *gin.Context) (packageVersionCoordinateRequest, bool) {
	var req packageVersionCoordinateRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "invalid version request", err.Error())
			return req, false
		}
	}
	if req.Name == "" {
		req.Name = c.Query("name")
	}
	if req.Version == "" {
		req.Version = c.Query("version")
	}
	if req.RepositoryID == 0 {
		if repoIDParam := c.Query("repository_id"); repoIDParam != "" {
			repoID, err := strconv.ParseUint(repoIDParam, 10, 32)
			if err != nil {
				response.BadRequest(c, "invalid repository ID", "repository_id must be a positive integer")
				return req, false
			}
			req.RepositoryID = uint(repoID)
		}
	}
	if req.Name == "" || req.Version == "" {
		response.BadRequest(c, "invalid version request", "name and version are required")
		return req, false
	}
	return req, true
}

func (h *PackageVersionHandler) writeVersionOperationError(c *gin.Context, err error) {
	if errors.Is(err, runtime.ErrNotFound) {
		response.NotFound(c, "version not found")
		return
	}
	response.InternalError(c, err.Error())
}

func (h *PackageVersionHandler) resolvePackageDeleteTarget(c *gin.Context, packageID uint) (uint, string, string, error) {
	if packageID > 0 {
		var pkg model.Package
		if err := h.db.First(&pkg, packageID).Error; err == nil {
			return pkg.RepositoryID, pkg.Format, pkg.Name, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, "", "", err
		}

		var artifact model.Artifact
		if err := h.db.First(&artifact, packageID).Error; err == nil {
			if artifact.Format != "" && artifact.Name != "" {
				return artifact.RepositoryID, artifact.Format, artifact.Name, nil
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, "", "", err
		}
	}

	format := firstNonEmptyString(c.Query("type"), c.Query("format"), c.Query("package_type"))
	name := c.Query("name")
	if format == "" || name == "" {
		return 0, "", "", gorm.ErrRecordNotFound
	}

	var repositoryID uint
	if repoIDParam := c.Query("repository_id"); repoIDParam != "" {
		repoID, err := strconv.ParseUint(repoIDParam, 10, 32)
		if err != nil {
			return 0, "", "", fmt.Errorf("repository_id must be a positive integer")
		}
		repositoryID = uint(repoID)
	}
	return repositoryID, format, name, nil
}

var _ = context.Background
var _ = http.StatusOK
var _ = strings.Index
