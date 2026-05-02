package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/service"
)

// DashboardHandler 仪表盘处理器
type DashboardHandler struct {
	svc *service.DashboardService
}

// NewDashboardHandler 创建仪表盘处理器实例
func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// GetStats 获取仪表盘统计数据
func (h *DashboardHandler) GetStats(c *gin.Context) {
	stats, err := h.svc.GetStats(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, stats)
}
