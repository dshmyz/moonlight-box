package http

import (
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/gin-gonic/gin"
)

type BackupConfigHandler struct {
	configSvc    *service.SystemConfigService
	schedulerSvc *service.SchedulerService
}

func NewBackupConfigHandler(configSvc *service.SystemConfigService, schedulerSvc *service.SchedulerService) *BackupConfigHandler {
	return &BackupConfigHandler{
		configSvc:    configSvc,
		schedulerSvc: schedulerSvc,
	}
}

type BackupConfigResponse struct {
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval"`
	Time     string `json:"time"`
}

func (h *BackupConfigHandler) GetConfig(c *gin.Context) {
	enabled := true
	interval := "24h"
	timeStr := "02:00"

	if config, err := h.configSvc.Get("backup.enabled"); err == nil {
		enabled = config.Value == "true" || config.Value == "1"
	}
	if config, err := h.configSvc.Get("backup.interval"); err == nil {
		interval = config.Value
	}
	if config, err := h.configSvc.Get("backup.time"); err == nil {
		timeStr = config.Value
	}

	response.Success(c, BackupConfigResponse{
		Enabled:  enabled,
		Interval: interval,
		Time:     timeStr,
	})
}

type UpdateBackupConfigRequest struct {
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval" binding:"required"`
	Time     string `json:"time" binding:"required"`
}

func (h *BackupConfigHandler) UpdateConfig(c *gin.Context) {
	var req UpdateBackupConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	userID := c.GetUint("userID")

	if err := h.configSvc.Set("backup.enabled", boolToString(req.Enabled), "boolean", "backup", "Enable or disable scheduled backup", false, userID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if err := h.configSvc.Set("backup.interval", req.Interval, "string", "backup", "Backup interval (e.g., 24h, 12h, 1h)", false, userID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if err := h.configSvc.Set("backup.time", req.Time, "string", "backup", "Backup time (HH:MM format)", false, userID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if err := h.schedulerSvc.UpdateBackupSchedule(); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, BackupConfigResponse{
		Enabled:  req.Enabled,
		Interval: req.Interval,
		Time:     req.Time,
	})
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
