package http

import (
	"context"
	"time"

	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/gin-gonic/gin"
)

// rebuildTimeout 全量重建的最长执行时间。重建在事务内逐版本聚合，
// 版本规模较大时可能耗时较长，给出足够宽松的防御性上限。
const rebuildTimeout = 30 * time.Minute

// SystemRebuildHandler 维护类操作：手动重建可投影表（packages / package_versions）。
// 这些表都是 artifacts 的可重建 read model，数据异常时可手动触发全量重建恢复。
type SystemRebuildHandler struct {
	artifactSvc *service.ArtifactService
}

func NewSystemRebuildHandler(artifactSvc *service.ArtifactService) *SystemRebuildHandler {
	return &SystemRebuildHandler{artifactSvc: artifactSvc}
}

// RebuildPackageVersions 从 artifacts 全量重建 package_versions 表。
func (h *SystemRebuildHandler) RebuildPackageVersions(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), rebuildTimeout)
	defer cancel()
	if err := h.artifactSvc.RebuildPackageVersions(ctx); err != nil {
		internalErr(c, err, "rebuild package_versions failed")
		return
	}
	response.Success(c, gin.H{"rebuilt": "package_versions"})
}

// RebuildPackages 从 artifacts 全量重建 packages 表。
func (h *SystemRebuildHandler) RebuildPackages(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), rebuildTimeout)
	defer cancel()
	if err := h.artifactSvc.RebuildPackages(ctx); err != nil {
		internalErr(c, err, "rebuild packages failed")
		return
	}
	response.Success(c, gin.H{"rebuilt": "packages"})
}
