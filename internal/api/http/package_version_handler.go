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
	classifiers              map[string]runtime.FileTypeClassifier
	packageVersionTableOnce  sync.Once
	packageVersionTableReady bool
}

func NewPackageVersionHandler(db *gorm.DB) *PackageVersionHandler {
	return NewPackageVersionHandlerWithArtifactService(db, service.NewArtifactService(db))
}

func NewPackageVersionHandlerWithArtifactService(db *gorm.DB, artifactSvc *service.ArtifactService) *PackageVersionHandler {
	return &PackageVersionHandler{db: db, artifactSvc: artifactSvc}
}

func NewPackageVersionHandlerWithClassifiers(db *gorm.DB, artifactSvc *service.ArtifactService, classifiers map[string]runtime.FileTypeClassifier) *PackageVersionHandler {
	return &PackageVersionHandler{db: db, artifactSvc: artifactSvc, classifiers: classifiers}
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

	// 优先走 package_versions 读模型
	if summaries, ok := h.loadPackageVersionSummaries(c.Request.Context(), pkgType, pkgName, repositoryID); ok {
		h.respondVersionsFromArtifacts(c, pkgType, pkgName, repositoryID, summaries)
		return
	}

	// 回退：package_versions 表不存在或无数据时，从 artifacts 聚合
	h.respondVersionsFromArtifacts(c, pkgType, pkgName, repositoryID, nil)
}

// respondVersionsFromArtifacts 从 artifacts 表按版本聚合；当 summaries 存在时，
// 优先使用 package_versions 读模型字段，并补齐读模型暂缺的 artifact 版本。
func (h *PackageVersionHandler) respondVersionsFromArtifacts(c *gin.Context, pkgType, pkgName string, repositoryID uint, summaries []model.PackageVersion) {
	db := h.db.WithContext(c.Request.Context()).Model(&model.Artifact{}).
		Where("format = ?", pkgType).
		Where("name = ?", pkgName).
		Where("version != ''").
		Where("(kind IS NULL OR kind NOT IN ?)", []string{"metadata", "checksum", "directory"}).
		Where("(format != 'go' OR kind = 'version' OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))").
		Where("NOT (format = 'yum' AND (remote_path LIKE 'repodata/%' OR remote_path LIKE '%/repodata/%' OR path = 'repodata' OR path LIKE '%/repodata' OR filename = 'repomd.xml'))")

	if repositoryID > 0 {
		db = db.Where("repository_id = ?", repositoryID)
	}
	db = excludeSummarizedArtifactVersions(db, summaries)

	var artifacts []model.Artifact
	if err := db.Order("updated_at DESC").Find(&artifacts).Error; err != nil {
		internalErr(c, err, "handler error")
		return
	}

	repoIDs := make(map[uint]bool)
	for _, a := range artifacts {
		repoIDs[a.RepositoryID] = true
	}
	for _, s := range summaries {
		repoIDs[s.RepositoryID] = true
	}
	downloadCountMap := h.batchDownloadCounts(c.Request.Context(), pkgType, pkgName, repoIDs)

	type versionGroup struct {
		id              uint
		latestAt        time.Time
		publishedAt     string
		publishedAtTime *time.Time
		license         string
		name            string
		namespace       string
		identityKey     string
		status          string
		filesDownloaded bool
		downloadCount   int64
		sizeBytes       int64
		sha256          string
		fileCount       int
		hasSummary      bool
		repoIDs         map[uint]bool
	}
	verGroups := make(map[string]*versionGroup)
	var verOrder []string
	for _, s := range summaries {
		vp, ok := verGroups[s.Version]
		if !ok {
			vp = &versionGroup{repoIDs: make(map[uint]bool)}
			verGroups[s.Version] = vp
			verOrder = append(verOrder, s.Version)
		}
		vp.repoIDs[s.RepositoryID] = true
		if vp.id == 0 {
			vp.id = s.ID
		}
		if s.LatestArtifactAt.After(vp.latestAt) {
			vp.latestAt = s.LatestArtifactAt
		}
		if s.PublishedAt != nil && vp.publishedAtTime == nil {
			publishedAt := *s.PublishedAt
			vp.publishedAtTime = &publishedAt
		}
		if vp.name == "" {
			vp.name = s.PackageName
		}
		if vp.namespace == "" {
			vp.namespace = s.Namespace
		}
		if s.Status != "" {
			vp.status = s.Status
		}
		if s.License != "" {
			vp.license = s.License
		}
		if s.SizeBytes > 0 {
			vp.sizeBytes += s.SizeBytes
		}
		if vp.sha256 == "" {
			vp.sha256 = s.ChecksumSHA256
		}
		vp.fileCount += s.FileCount
		vp.filesDownloaded = vp.filesDownloaded || s.FilesDownloaded
		vp.downloadCount += s.DownloadCount
		vp.hasSummary = true
	}
	for _, a := range artifacts {
		version := a.Version
		vp, ok := verGroups[version]
		if !ok {
			vp = &versionGroup{repoIDs: make(map[uint]bool)}
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
		if !vp.hasSummary && vp.sizeBytes == 0 && a.SizeBytes > 0 {
			vp.sizeBytes = a.SizeBytes
		}
		if !vp.hasSummary && vp.sha256 == "" {
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
		if !vp.hasSummary && vp.license == "" {
			vp.license = jsonbString(a.Attributes, "license")
		}
		if !vp.hasSummary && vp.publishedAt == "" {
			vp.publishedAt = jsonbString(a.Attributes, "published_at")
		}
		if !vp.hasSummary && isDownloadableArtifact(a, a.Filename, "", nil) {
			vp.fileCount++
		}
	}

	versions := make([]gin.H, 0, len(verOrder))

	// 批量加载仓库名
	allRepoIDs := make(map[uint]bool)
	for _, vp := range verGroups {
		for id := range vp.repoIDs {
			allRepoIDs[id] = true
		}
	}
	repoNameMap, groupRepoNameMap := h.batchRepoInfo(c.Request.Context(), allRepoIDs)

	for _, ver := range verOrder {
		vp := verGroups[ver]
		publishedAt := vp.latestAt
		if vp.publishedAtTime != nil && vp.publishedAtTime.After(vp.latestAt) {
			publishedAt = *vp.publishedAtTime
		} else if vp.publishedAt != "" {
			if t, err := time.Parse(time.RFC3339, vp.publishedAt); err == nil {
				publishedAt = t
			}
		}
		if publishedAt.IsZero() {
			publishedAt = time.Now()
		}

		var downloadCount int64
		downloadCount = vp.downloadCount
		if downloadCount == 0 {
			for repoID := range vp.repoIDs {
				downloadCount += downloadCountMap[fmt.Sprintf("%d|%s", repoID, ver)]
			}
		}
		status := vp.status
		if status == "" {
			status = "published"
		}

		entry := gin.H{
			"id":                    vp.id,
			"repository_id":         singleRepositoryID(vp.repoIDs),
			"repository_name":       repositoryNames(vp.repoIDs, repoNameMap),
			"repository_group_name": repositoryGroupNames(vp.repoIDs, groupRepoNameMap),
			"version":               ver,
			"name":                  vp.name,
			"namespace":             vp.namespace,
			"identity_key":          vp.identityKey,
			"status":                status,
			"published_at":          publishedAt,
			"size_bytes":            vp.sizeBytes,
			"checksum_sha256":       vp.sha256,
			"file_count":            vp.fileCount,
			"files_downloaded":      vp.filesDownloaded,
			"download_count":        downloadCount,
		}
		if vp.license != "" {
			entry["license"] = vp.license
		}
		versions = append(versions, entry)
	}

	response.Success(c, gin.H{
		"package_name": pkgName,
		"type":         pkgType,
		"versions":     versions,
	})
}

// batchDownloadCounts 批量查询版本级下载计数。
func (h *PackageVersionHandler) batchDownloadCounts(ctx context.Context, pkgType, pkgName string, repoIDs map[uint]bool) map[string]int64 {
	result := make(map[string]int64)
	if len(repoIDs) == 0 {
		return result
	}
	ids := make([]uint, 0, len(repoIDs))
	for id := range repoIDs {
		ids = append(ids, id)
	}
	var countRows []struct {
		RepositoryID uint
		Version      string
		Count        int64
	}
	err := h.db.WithContext(ctx).Table("download_logs").
		Select("repository_id, version, COUNT(*) as count").
		Where("repository_id IN ? AND package_type = ? AND package_name = ? AND status IN ?",
			ids, pkgType, pkgName, []string{"success", "cached"}).
		Where("version != ''").
		Group("repository_id, version").
		Scan(&countRows).Error
	if err != nil {
		return result
	}
	for _, cr := range countRows {
		result[fmt.Sprintf("%d|%s", cr.RepositoryID, cr.Version)] += cr.Count
	}
	return result
}

// ListVersionFiles 按版本加载文件列表（artifacts + blobs），供前端懒加载。
func (h *PackageVersionHandler) ListVersionFiles(c *gin.Context) {
	pkgType := c.Param("type")
	pkgName := c.Query("name")
	version := c.Query("version")

	if pkgName == "" || version == "" {
		response.BadRequest(c, "missing parameters", "name and version are required")
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

	artifacts, err := findVersionArtifacts(h.db.WithContext(c.Request.Context()), repositoryID, pkgType, pkgName, version)
	if err != nil {
		internalErr(c, err, "handler error")
		return
	}
	if len(artifacts) == 0 {
		response.Success(c, gin.H{"files": []gin.H{}})
		return
	}

	// 批量加载仓库名称
	repoIDs := make(map[uint]bool)
	for _, a := range artifacts {
		repoIDs[a.RepositoryID] = true
	}
	repoNameMap := make(map[uint]string)
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

	files := make([]gin.H, 0, len(artifacts))
	for _, a := range artifacts {
		filename := a.Filename
		fileType := classifyFileType(h.classifiers, a.Format, filename)
		blobs := blobMap[a.ID]
		repoName := repoNameMap[a.RepositoryID]
		downloadURL := buildDownloadURL(repoName, a)

		if !isDownloadableArtifact(a, filename, downloadURL, blobs) {
			continue
		}

		fileEntry := gin.H{
			"id":              a.ID,
			"version_id":      a.ID,
			"filename":        filename,
			"file_type":       fileType,
			"storage_path":    firstNonEmptyString(a.RemotePath, a.Path),
			"path":            a.Path,
			"remote_path":     a.RemotePath,
			"size_bytes":      firstNonZeroInt64(sumBlobSizes(blobs), a.SizeBytes),
			"checksum_sha256": firstNonEmptyString(firstBlobDigest(blobs), jsonbString(a.Checksums, "sha256")),
			"download_url":    downloadURL,
			"qualifiers":      a.Qualifiers,
			"attributes":      a.Attributes,
			"metadata":        a.Metadata,
		}
		files = append(files, fileEntry)
	}

	// Maven SNAPSHOT 文件标记 default_visible
	if isMavenPackageType(pkgType) && len(artifacts) > 0 {
		decorateMavenSnapshotDisplayAttributes(version, mavenArtifactID(pkgName, artifacts[0].Qualifiers), files)
	}

	response.Success(c, gin.H{
		"files": files,
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

func excludeSummarizedArtifactVersions(db *gorm.DB, summaries []model.PackageVersion) *gorm.DB {
	if len(summaries) == 0 {
		return db
	}

	versionsByRepo := make(map[uint][]string)
	seen := make(map[string]bool)
	for _, s := range summaries {
		if s.RepositoryID == 0 || s.Version == "" {
			continue
		}
		key := fmt.Sprintf("%d|%s", s.RepositoryID, s.Version)
		if seen[key] {
			continue
		}
		seen[key] = true
		versionsByRepo[s.RepositoryID] = append(versionsByRepo[s.RepositoryID], s.Version)
	}
	if len(versionsByRepo) == 0 {
		return db
	}

	clauses := make([]string, 0, len(versionsByRepo))
	args := make([]interface{}, 0, len(versionsByRepo)*2)
	for repoID, versions := range versionsByRepo {
		clauses = append(clauses, "(repository_id = ? AND version IN ?)")
		args = append(args, repoID, versions)
	}
	return db.Where("NOT ("+strings.Join(clauses, " OR ")+")", args...)
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

// repositoryNames 返回版本关联的仓库名，多个时用逗号分隔
func repositoryNames(repoIDs map[uint]bool, nameMap map[uint]string) string {
	if len(repoIDs) == 0 {
		return ""
	}
	names := make([]string, 0, len(repoIDs))
	for id := range repoIDs {
		if n, ok := nameMap[id]; ok {
			names = append(names, n)
		}
	}
	return strings.Join(names, ",")
}

// repositoryGroupNames 返回版本关联的 group（virtual）仓库名，多个时用逗号分隔
func repositoryGroupNames(repoIDs map[uint]bool, nameMap map[uint]string) string {
	if len(repoIDs) == 0 {
		return ""
	}
	names := make([]string, 0, len(repoIDs))
	for id := range repoIDs {
		if n, ok := nameMap[id]; ok && n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, ",")
}

// batchRepoInfo 批量查询仓库名称及其所属 group（virtual）仓库名。
func (h *PackageVersionHandler) batchRepoInfo(ctx context.Context, repoIDs map[uint]bool) (nameMap, groupNameMap map[uint]string) {
	nameMap = make(map[uint]string)
	groupNameMap = make(map[uint]string)
	if len(repoIDs) == 0 {
		return
	}
	ids := make([]uint, 0, len(repoIDs))
	for id := range repoIDs {
		ids = append(ids, id)
	}

	type repoRow struct {
		ID   uint   `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	var rows []repoRow
	if err := h.db.WithContext(ctx).Model(&model.Repository{}).Select("id, name").Where("id IN ?", ids).Find(&rows).Error; err == nil {
		for _, r := range rows {
			nameMap[r.ID] = r.Name
		}
	}

	type memberRow struct {
		MemberID    uint   `gorm:"column:member_id"`
		VirtualName string `gorm:"column:name"`
	}
	var memberRows []memberRow
	if err := h.db.WithContext(ctx).
		Table("repository_members").
		Select("repository_members.member_id, repositories.name").
		Joins("JOIN repositories ON repositories.id = repository_members.repository_id").
		Where("repository_members.member_id IN ? AND repositories.type = ?", ids, model.RepoTypeVirtual).
		Find(&memberRows).Error; err == nil {
		for _, r := range memberRows {
			groupNameMap[r.MemberID] = r.VirtualName
		}
	}
	return
}

func classifyFileType(classifiers map[string]runtime.FileTypeClassifier, format, filename string) string {
	if c, ok := classifiers[format]; ok {
		return c.ClassifyFileType(filename)
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

// findVersionArtifacts 查询某版本的所有 artifacts（复用 service 层同名逻辑，
// 但因包隔离在此处独立实现）。
func findVersionArtifacts(db *gorm.DB, repoID uint, format, name, version string) ([]model.Artifact, error) {
	q := db.Model(&model.Artifact{}).
		Where("format = ?", format).
		Where("name = ? AND version = ?", name, version).
		Where("(kind IS NULL OR kind NOT IN ?)", []string{runtime.KindMetadata, runtime.KindChecksum, runtime.KindDirectory}).
		Where("(format != ? OR kind = ? OR EXISTS (SELECT 1 FROM artifact_blobs ab WHERE ab.artifact_id = artifacts.id))", "go", runtime.KindVersion).
		Where("NOT (format = ? AND (remote_path LIKE ? OR remote_path LIKE ? OR path = ? OR path LIKE ? OR filename = ?))",
			"yum", "repodata/%", "%/repodata/%", "repodata", "%/repodata", "repomd.xml").
		Order("updated_at DESC")
	if repoID > 0 {
		q = q.Where("repository_id = ?", repoID)
	}
	var artifacts []model.Artifact
	return artifacts, q.Find(&artifacts).Error
}

func isDownloadableArtifact(a model.Artifact, filename, downloadURL string, blobs []blobInfo) bool {
	if filename == "" && downloadURL == "" && len(blobs) == 0 {
		return false
	}
	if !runtime.IsCountableFileKind(a.Kind) {
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
		internalErr(c, err, "handler error")
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
	internalErr(c, err, "handler error")
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
