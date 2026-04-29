package handler

import (
	"net/http"
	"strconv"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"

	"github.com/gin-gonic/gin"
)

type PackageVersionHandler struct {
	pkgRepo *repository.PackageRepository
}

func NewPackageVersionHandler(pkgRepo *repository.PackageRepository) *PackageVersionHandler {
	return &PackageVersionHandler{pkgRepo: pkgRepo}
}

func (h *PackageVersionHandler) ListVersions(c *gin.Context) {
	pkgType := c.Param("type")
	pkgName := c.Param("name")

	pkg, err := h.pkgRepo.FindByNameAndType(pkgName, model.PackageType(pkgType))
	if err != nil {
		response.NotFound(c, "package not found")
		return
	}

	response.Success(c, gin.H{
		"package_name": pkg.Name,
		"type":         pkg.Type,
		"versions":     pkg.Versions,
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

	ver, pkg, err := findVersionAndPackage(h.pkgRepo, uint(versionID))
	if err != nil {
		response.NotFound(c, "version not found")
		return
	}

	ver.Status = model.StatusDeprecated
	if err := h.pkgRepo.UpdatePackageVersion(ver); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "version deprecated",
		"package": pkg.Name,
		"version": ver.Version,
		"reason":  req.Reason,
	})
}

func (h *PackageVersionHandler) RestoreVersion(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	ver, pkg, err := findVersionAndPackage(h.pkgRepo, uint(versionID))
	if err != nil {
		response.NotFound(c, "version not found")
		return
	}

	ver.Status = model.StatusPublished
	if err := h.pkgRepo.UpdatePackageVersion(ver); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "version restored",
		"package": pkg.Name,
		"version": ver.Version,
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

	ver, pkg, err := findVersionAndPackage(h.pkgRepo, uint(versionID))
	if err != nil {
		response.NotFound(c, "version not found")
		return
	}

	ver.Status = model.StatusYanked
	if err := h.pkgRepo.UpdatePackageVersion(ver); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "version yanked",
		"package": pkg.Name,
		"version": ver.Version,
		"reason":  req.Reason,
	})
}

func (h *PackageVersionHandler) DeleteVersion(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	if err := h.pkgRepo.DeleteVersion(uint(versionID)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.NoContent(c)
}

func findVersionAndPackage(pkgRepo *repository.PackageRepository, versionID uint) (*model.PackageVersion, *model.Package, error) {
	ver, err := pkgRepo.FindVersionByID(versionID)
	if err != nil {
		return nil, nil, err
	}
	return ver, &ver.Package, nil
}

var _ = http.StatusOK
