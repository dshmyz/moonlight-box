package http

import (
	"strconv"
	"time"

	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/gin-gonic/gin"
)

// LogCleanupConfigHandler 处理下载日志清理配置的 HTTP 请求。
type LogCleanupConfigHandler struct {
	configSvc     *service.SystemConfigService
	logCleanupSvc *service.LogCleanupService
}

func NewLogCleanupConfigHandler(configSvc *service.SystemConfigService, logCleanupSvc *service.LogCleanupService) *LogCleanupConfigHandler {
	return &LogCleanupConfigHandler{
		configSvc:     configSvc,
		logCleanupSvc: logCleanupSvc,
	}
}

type LogCleanupConfigResponse struct {
	Enabled       bool   `json:"enabled"`
	RetentionDays int    `json:"retention_days"`
	Interval      string `json:"interval"`
}

func (h *LogCleanupConfigHandler) GetConfig(c *gin.Context) {
	enabled := true
	retentionDays := 30
	interval := "24h"

	if config, err := h.configSvc.Get("log_cleanup.enabled"); err == nil {
		enabled = config.Value == "true" || config.Value == "1"
	}
	if config, err := h.configSvc.Get("log_cleanup.retention_days"); err == nil {
		if d, err := strconv.Atoi(config.Value); err == nil && d > 0 {
			retentionDays = d
		}
	}
	if config, err := h.configSvc.Get("log_cleanup.interval"); err == nil {
		// 校验是否为合法 duration
		if _, err := time.ParseDuration(config.Value); err == nil {
			interval = config.Value
		}
	}

	response.Success(c, LogCleanupConfigResponse{
		Enabled:       enabled,
		RetentionDays: retentionDays,
		Interval:      interval,
	})
}

type UpdateLogCleanupConfigRequest struct {
	Enabled       bool   `json:"enabled"`
	RetentionDays int    `json:"retention_days" binding:"required,min=1"`
	Interval      string `json:"interval" binding:"required"`
}

func (h *LogCleanupConfigHandler) UpdateConfig(c *gin.Context) {
	var req UpdateLogCleanupConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	// 校验 interval 格式
	intervalDur, err := time.ParseDuration(req.Interval)
	if err != nil || intervalDur <= 0 {
		response.BadRequest(c, "invalid interval", "interval must be a valid duration (e.g., 24h, 12h, 1h)")
		return
	}

	userID := c.GetUint("userID")

	if err := h.configSvc.Set("log_cleanup.enabled", boolToString(req.Enabled), "boolean", "logging", "启用下载日志自动清理", false, userID); err != nil {
		internalErr(c, err, "handler error")
		return
	}

	if err := h.configSvc.Set("log_cleanup.retention_days", strconv.Itoa(req.RetentionDays), "int", "logging", "下载日志保留天数", false, userID); err != nil {
		internalErr(c, err, "handler error")
		return
	}

	if err := h.configSvc.Set("log_cleanup.interval", req.Interval, "string", "logging", "清理执行间隔", false, userID); err != nil {
		internalErr(c, err, "handler error")
		return
	}

	// 热更新清理计划
	h.logCleanupSvc.UpdateSchedule()

	response.Success(c, LogCleanupConfigResponse{
		Enabled:       req.Enabled,
		RetentionDays: req.RetentionDays,
		Interval:      req.Interval,
	})
}

// CleanupNow 立即执行一次日志清理。
func (h *LogCleanupConfigHandler) CleanupNow(c *gin.Context) {
	if err := h.logCleanupSvc.CleanupNow(); err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.Success(c, gin.H{
		"message": "cleanup completed",
	})
}
