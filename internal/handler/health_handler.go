package handler

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/response"
)

// HealthCheckHandler 健康检查API处理器
type HealthCheckHandler struct {
	healthCheckSvc *proxy.HealthCheckService
}

// NewHealthCheckHandler 创建健康检查处理器
func NewHealthCheckHandler(healthCheckSvc *proxy.HealthCheckService) *HealthCheckHandler {
	return &HealthCheckHandler{
		healthCheckSvc: healthCheckSvc,
	}
}

// GetHealthStatus 获取指定仓库的健康状态
// GET /api/v1/health/repos/:id
func (h *HealthCheckHandler) GetHealthStatus(c *gin.Context) {
	repoIDStr := c.Param("id")
	repoID, err := strconv.ParseUint(repoIDStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid repository ID", "")
		return
	}

	status := h.healthCheckSvc.GetHealthStatus(uint(repoID))
	if status == nil {
		response.NotFound(c, "health status not found")
		return
	}

	cb := h.healthCheckSvc.GetCircuitBreaker(uint(repoID))
	var cbStats interface{}
	if cb != nil {
		cbStats = cb.GetStats()
	}

	response.Success(c, gin.H{
		"health_status":   status,
		"circuit_breaker": cbStats,
	})
}

// GetAllHealthStatuses 获取所有仓库的健康状态
// GET /api/v1/health/repos
func (h *HealthCheckHandler) GetAllHealthStatuses(c *gin.Context) {
	statuses := h.healthCheckSvc.GetAllHealthStatuses()

	result := make([]gin.H, 0, len(statuses))
	for repoID, status := range statuses {
		cb := h.healthCheckSvc.GetCircuitBreaker(repoID)
		var cbStats interface{}
		if cb != nil {
			cbStats = cb.GetStats()
		}

		result = append(result, gin.H{
			"repo_id":         repoID,
			"health_status":   status,
			"circuit_breaker": cbStats,
		})
	}

	response.Success(c, gin.H{
		"total": len(result),
		"items": result,
	})
}

// ResetCircuitBreaker 重置指定仓库的断路器
// POST /api/v1/health/repos/:id/reset
func (h *HealthCheckHandler) ResetCircuitBreaker(c *gin.Context) {
	repoIDStr := c.Param("id")
	repoID, err := strconv.ParseUint(repoIDStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid repository ID", "")
		return
	}

	h.healthCheckSvc.ResetCircuitBreaker(uint(repoID))

	slog.Info("circuit breaker reset via API", "repo_id", repoID)
	response.Success(c, gin.H{
		"message": "circuit breaker reset successfully",
		"repo_id": repoID,
	})
}
