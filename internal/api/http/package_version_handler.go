package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PackageVersionHandler struct {
	db *gorm.DB
}

func NewPackageVersionHandler(db *gorm.DB) *PackageVersionHandler {
	return &PackageVersionHandler{db: db}
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

	versions := make([]gin.H, 0)

	// 1. 优先从 packages + package_versions 表查（web 管理层）
	type pkgRow struct {
		ID uint
	}
	var pkg pkgRow
	pkgQuery := h.db.Table("packages").
		Where("name = ? AND type = ?", pkgName, pkgType)
	if repositoryID > 0 {
		pkgQuery = pkgQuery.Where("repository_id = ?", repositoryID)
	}
	if err := pkgQuery.First(&pkg).Error; err == nil && pkg.ID > 0 {

		type pvRow struct {
			Version   string
			Status    string
			SizeBytes int64  `gorm:"column:size_bytes"`
			CreatedAt string `gorm:"column:published_at"`
		}
		var rows []pvRow
		h.db.Table("package_versions").
			Select("version, COALESCE(status,'published') as status, COALESCE(size_bytes,0) as size_bytes, published_at").
			Where("package_id = ?", pkg.ID).
			Order("published_at DESC").
			Scan(&rows)

		for _, r := range rows {
			versions = append(versions, gin.H{
				"id":         0,
				"version":    r.Version,
				"status":     r.Status,
				"created_at": r.CreatedAt,
				"size_bytes": r.SizeBytes,
			})
		}
	}

	// 2. 从 artifacts 表查（runtime 层），去重
	seenVersions := make(map[string]bool)
	for _, v := range versions {
		if ver, ok := v["version"].(string); ok {
			seenVersions[ver] = true
		}
	}

	db := h.db.WithContext(c.Request.Context()).Model(&model.Artifact{}).
		Where("format = ?", pkgType)

	// 匹配多种坐标 key 模式
	var nameCond string
	if pkgType == "maven" || pkgType == "maven2" {
		nameCond = fmt.Sprintf(`%%"artifact":"%s%%`, pkgName)
	} else {
		nameCond = fmt.Sprintf(`%%"name":"%s%%`, pkgName)
	}
	db = db.Where("coordinates LIKE ?", nameCond)

	if repositoryID > 0 {
		db = db.Where("repository_id = ?", repositoryID)
	}

	var artifacts []model.Artifact
	if err := db.Order("created_at DESC").Find(&artifacts).Error; err != nil {
		// 非致命：已有 packages 数据则忽略
		if len(versions) == 0 {
			response.InternalError(c, err.Error())
			return
		}
	}

	// 批量获取 blob refs
	artifactIDs := make([]uint, len(artifacts))
	for i, a := range artifacts {
		artifactIDs[i] = a.ID
	}
	blobSizeMap := make(map[uint]int64)
	if len(artifactIDs) > 0 {
		type blobRow struct {
			ArtifactID uint
			Size       int64
		}
		var blobRows []blobRow
		h.db.Table("artifact_blobs AS ab").
			Select("ab.artifact_id, SUM(b.size) as size").
			Joins("JOIN blobs b ON b.id = ab.blob_id").
			Where("ab.artifact_id IN ?", artifactIDs).
			Group("ab.artifact_id").
			Scan(&blobRows)
		for _, br := range blobRows {
			blobSizeMap[br.ArtifactID] = br.Size
		}
	}

	for _, a := range artifacts {
		version := coordinateStr(a.Coordinates, "version")
		if version == "" || seenVersions[version] {
			continue
		}
		seenVersions[version] = true

		status := coordinateStr(a.Metadata, "status")
		if status == "" {
			status = "published"
		}

		versions = append(versions, gin.H{
			"id":         a.ID,
			"version":    version,
			"status":     status,
			"created_at": a.CreatedAt,
			"size_bytes": blobSizeMap[a.ID],
		})
	}

	name := pkgName
	if len(artifacts) > 0 {
		if n := coordinateStr(artifacts[0].Coordinates, "name"); n != "" {
			name = n
		}
	}

	response.Success(c, gin.H{
		"package_name": name,
		"type":         pkgType,
		"versions":     versions,
	})
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

	// 找到该 artifact，然后删除同名的所有 artifacts
	var artifact model.Artifact
	if err := h.db.First(&artifact, uint(packageID)).Error; err != nil {
		response.NotFound(c, "package not found")
		return
	}

	name := coordinateStr(artifact.Coordinates, "name")
	if name == "" {
		name = coordinateStr(artifact.Coordinates, "package")
	}

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
