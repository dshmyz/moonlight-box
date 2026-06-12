package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PackageVersionHandler struct {
	db *gorm.DB
}

func NewPackageVersionHandler(db *gorm.DB) *PackageVersionHandler {
	return &PackageVersionHandler{db: db}
}

type blobInfo struct {
	ArtifactID uint
	Size       int64
	Digest     string
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
	}
	verGroups := make(map[string]*versionGroup)
	var verOrder []string

	for _, a := range artifacts {
		version := a.Version
		if version == "" {
			continue
		}

		vp, ok := verGroups[version]
		if !ok {
			vp = &versionGroup{repoIDs: make(map[uint]bool)}
			verGroups[version] = vp
			verOrder = append(verOrder, version)
		}
		vp.repoIDs[a.RepositoryID] = true
		if a.CreatedAt.After(vp.latestAt) {
			vp.latestAt = a.CreatedAt
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

		// 聚合该版本在所有仓库中的下载计数
		var downloadCount int64
		for repoID := range vp.repoIDs {
			key := fmt.Sprintf("%d|%s", repoID, ver)
			downloadCount += downloadCountMap[key]
		}

		entry := gin.H{
			"version":          ver,
			"name":             vp.name,
			"namespace":        vp.namespace,
			"identity_key":     vp.identityKey,
			"status":           "published",
			"published_at":     publishedAt,
			"size_bytes":       totalSize,
			"checksum_sha256":  sha256,
			"files":            vp.files,
			"files_downloaded": len(vp.blobs) > 0,
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
	switch a.Kind {
	case "version", "metadata", "checksum":
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

	if artifact.Metadata == nil {
		artifact.Metadata = make(model.JSONB)
	}
	artifact.Metadata["status"] = "deprecated"
	artifact.Metadata["deprecation_reason"] = req.Reason
	if err := h.db.Save(&artifact).Error; err != nil {
		response.InternalError(c, err.Error())
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

	if artifact.Metadata == nil {
		artifact.Metadata = make(model.JSONB)
	}
	artifact.Metadata["status"] = "published"
	delete(artifact.Metadata, "deprecation_reason")
	if err := h.db.Save(&artifact).Error; err != nil {
		response.InternalError(c, err.Error())
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

	if artifact.Metadata == nil {
		artifact.Metadata = make(model.JSONB)
	}
	artifact.Metadata["status"] = "yanked"
	artifact.Metadata["yank_reason"] = req.Reason
	if err := h.db.Save(&artifact).Error; err != nil {
		response.InternalError(c, err.Error())
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

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("artifact_id = ?", uint(versionID)).Delete(&model.ArtifactBlob{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Artifact{}, uint(versionID)).Error
	}); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.NoContent(c)
}

func (h *PackageVersionHandler) DeletePackage(c *gin.Context) {
	packageID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid package ID", "package ID must be a positive integer")
		return
	}

	var pkg model.Package
	if err := h.db.First(&pkg, uint(packageID)).Error; err != nil {
		response.NotFound(c, "package not found")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var allIDs []uint
		if err := tx.Model(&model.Artifact{}).
			Where("repository_id = ? AND format = ? AND name = ?",
				pkg.RepositoryID, pkg.Format, pkg.Name).
			Pluck("id", &allIDs).Error; err != nil {
			return err
		}
		if len(allIDs) > 0 {
			if err := tx.Where("artifact_id IN ?", allIDs).Delete(&model.ArtifactBlob{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", allIDs).Delete(&model.Artifact{}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&pkg).Error
	}); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "package deleted",
	})
}

var _ = context.Background
var _ = http.StatusOK
var _ = strings.Index
