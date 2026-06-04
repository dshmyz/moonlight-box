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

	// 按统一 name 键匹配（所有协议已在 plugin 层标准化）
	db = db.Where("coordinates LIKE ?", fmt.Sprintf(`%%"name":"%s%%`, pkgName))

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

	// 按版本号分组，聚合文件列表
	type versionGroup struct {
		latestAt    time.Time
		publishedAt string
		license     string
		triggerIP   string
		files       []gin.H
		blobs       []blobInfo
	}
	verGroups := make(map[string]*versionGroup)
	var verOrder []string

	for _, a := range artifacts {
		version := coordinateStr(a.Coordinates, "version")
		if version == "" {
			continue
		}

		vp, ok := verGroups[version]
		if !ok {
			vp = &versionGroup{}
			verGroups[version] = vp
			verOrder = append(verOrder, version)
		}
		if a.CreatedAt.After(vp.latestAt) {
			vp.latestAt = a.CreatedAt
		}
		if vp.publishedAt == "" && a.Metadata != nil {
			if pa, ok := a.Metadata["published_at"]; ok {
				if s, ok := pa.(string); ok && s != "" {
					vp.publishedAt = s
				}
			}
		}
		if vp.license == "" && a.Metadata != nil {
			if lic, ok := a.Metadata["license"]; ok {
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

		filename := coordinateStr(a.Coordinates, "filename")
		if filename == "" {
			filename = coordinateStr(a.Coordinates, "file")
		}
		if filename == "" {
			if v, ok := a.Coordinates["artifact"]; ok {
				if s, ok2 := v.(string); ok2 {
					filename = s
				}
			}
		}

		fileType := classifyFileType(a.Format, filename, a.Coordinates)

		blobs := blobMap[a.ID]
		for _, b := range blobs {
			vp.blobs = append(vp.blobs, b)
		}

		repoName := repoNameMap[a.RepositoryID]
		downloadURL := buildDownloadURL(repoName, a.Metadata)

		if isDownloadableArtifact(a, filename, downloadURL, blobs) {
			vp.files = append(vp.files, gin.H{
				"filename":     filename,
				"file_type":    fileType,
				"size_bytes":   sumBlobSizes(blobs),
				"download_url": downloadURL,
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
		sha256 := ""
		if len(vp.blobs) > 0 && vp.blobs[0].Digest != "" {
			sha256 = vp.blobs[0].Digest
		}

		publishedAt := vp.latestAt
		if vp.publishedAt != "" {
			if t, err := time.Parse(time.RFC3339, vp.publishedAt); err == nil {
				publishedAt = t
			}
		}

		entry := gin.H{
			"version":          ver,
			"status":           "published",
			"published_at":     publishedAt,
			"size_bytes":       totalSize,
			"checksum_sha256":  sha256,
			"files":            vp.files,
			"files_downloaded": len(vp.blobs) > 0,
			"download_count":   0,
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

func classifyFileType(format, filename string, coords model.JSONB) string {
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

func buildDownloadURL(repoName string, metadata model.JSONB) string {
	downloadPath := coordinateStr(metadata, "download_path")
	if downloadPath == "" {
		return ""
	}
	return "/repository/" + repoName + "/" + downloadPath
}

func isDownloadableArtifact(a model.Artifact, filename, downloadURL string, blobs []blobInfo) bool {
	if filename == "" && downloadURL == "" && len(blobs) == 0 {
		return false
	}
	switch a.Kind {
	case "version", "metadata", "package-index", "metadata-ref", "release":
		return false
	}
	if filename != "" || downloadURL != "" || len(blobs) > 0 {
		return true
	}
	return false
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
		"version": coordinateStr(artifact.Coordinates, "version"),
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
		"version": coordinateStr(artifact.Coordinates, "version"),
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
		"version": coordinateStr(artifact.Coordinates, "version"),
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

	var artifact model.Artifact
	if err := h.db.First(&artifact, uint(packageID)).Error; err != nil {
		response.NotFound(c, "package not found")
		return
	}

	name := coordinateStr(artifact.Coordinates, "name")

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var allIDs []uint
		if err := tx.Model(&model.Artifact{}).
			Where("repository_id = ? AND format = ? AND coordinates LIKE ?",
				artifact.RepositoryID, artifact.Format, fmt.Sprintf(`%%"name":"%s%%`, name)).
			Pluck("id", &allIDs).Error; err != nil {
			return err
		}
		if len(allIDs) == 0 {
			allIDs = []uint{uint(packageID)}
		}
		if err := tx.Where("artifact_id IN ?", allIDs).Delete(&model.ArtifactBlob{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", allIDs).Delete(&model.Artifact{}).Error
	}); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "package deleted",
	})
}

func coordinateStr(coords model.JSONB, key string) string {
	if coords == nil {
		return ""
	}
	v, ok := coords[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

var _ = context.Background
var _ = http.StatusOK
var _ = strings.Index
