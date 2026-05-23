package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"

	"github.com/gin-gonic/gin"
)

type PackageVersionHandler struct {
	compRepo *repository.ComponentRepository
}

func NewPackageVersionHandler(compRepo *repository.ComponentRepository) *PackageVersionHandler {
	return &PackageVersionHandler{compRepo: compRepo}
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

	agg, err := h.compRepo.FindByRepoNameAndTypeContext(c.Request.Context(), repositoryID, pkgName, model.PackageType(pkgType))
	if err != nil {
		response.NotFound(c, "package not found")
		return
	}

	versions := make([]gin.H, 0, len(agg.Components))
	for _, comp := range agg.Components {
		size := comp.SizeBytes
		if size == 0 {
			for _, asset := range comp.Assets {
				size += asset.Blob.SizeBytes
			}
		}
		versions = append(versions, gin.H{
			"id":              comp.ID,
			"version":         comp.Version,
			"status":          comp.Status,
			"published_at":    comp.PublishedAt,
			"download_count":  comp.DownloadCount,
			"size_bytes":      size,
			"files_downloaded": comp.FilesDownloaded,
			"assets":          comp.Assets,
		})
	}

	response.Success(c, gin.H{
		"package_name": agg.Name,
		"type":         agg.Format,
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

	comp, err := h.compRepo.FindComponentByIDContext(c.Request.Context(), uint(versionID))
	if err != nil {
		response.NotFound(c, "version not found")
		return
	}

	comp.Status = model.StatusDeprecated
	if err := h.compRepo.UpdateComponentContext(c.Request.Context(), comp); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "version deprecated",
		"package": comp.Name,
		"version": comp.Version,
		"reason":  req.Reason,
	})
}

func (h *PackageVersionHandler) RestoreVersion(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	comp, err := h.compRepo.FindComponentByIDContext(c.Request.Context(), uint(versionID))
	if err != nil {
		response.NotFound(c, "version not found")
		return
	}

	comp.Status = model.StatusPublished
	if err := h.compRepo.UpdateComponentContext(c.Request.Context(), comp); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "version restored",
		"package": comp.Name,
		"version": comp.Version,
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

	comp, err := h.compRepo.FindComponentByIDContext(c.Request.Context(), uint(versionID))
	if err != nil {
		response.NotFound(c, "version not found")
		return
	}

	comp.Status = model.StatusYanked
	if err := h.compRepo.UpdateComponentContext(c.Request.Context(), comp); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "version yanked",
		"package": comp.Name,
		"version": comp.Version,
		"reason":  req.Reason,
	})
}

func (h *PackageVersionHandler) DeleteVersion(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	if err := h.compRepo.DeleteComponentContext(c.Request.Context(), uint(versionID)); err != nil {
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

	if err := h.compRepo.DeleteCatalogEntryContext(c.Request.Context(), uint(packageID)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "package deleted",
	})
}

var _ = context.Background
var _ = http.StatusOK
