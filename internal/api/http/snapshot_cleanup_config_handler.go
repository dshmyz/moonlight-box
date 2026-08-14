package http

import (
	"fmt"
	"strconv"
	"time"

	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/gin-gonic/gin"
)

// SnapshotCleanupConfigHandler 处理 Maven SNAPSHOT 清理配置的 HTTP 请求。
type SnapshotCleanupConfigHandler struct {
	configSvc   *service.SystemConfigService
	cleanupSvc  *service.CleanupService
}

func NewSnapshotCleanupConfigHandler(configSvc *service.SystemConfigService, cleanupSvc *service.CleanupService) *SnapshotCleanupConfigHandler {
	return &SnapshotCleanupConfigHandler{
		configSvc:  configSvc,
		cleanupSvc: cleanupSvc,
	}
}

type SnapshotCleanupConfigResponse struct {
	Enabled    bool   `json:"enabled"`
	KeepLast   int    `json:"keep_last"`
	MaxAgeDays int    `json:"max_age_days"`
	Interval   string `json:"interval"`
}

// formatDurationClock 把 time.Duration 规范化为简洁的 Go duration 字符串：
// 24h0m0s 显示为 "24h"，1h30m0s 显示为 "1h30m"，而不是冗长的默认 String()。
func formatDurationClock(d time.Duration) string {
	// 含秒以下精度时，直接用默认 String() 保留精度。
	if d < time.Minute || d%time.Minute != 0 {
		return d.String()
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh%dm", h, m)
	}
}

func (h *SnapshotCleanupConfigHandler) GetConfig(c *gin.Context) {
	enabled := true
	keepLast := 5
	maxAgeDays := 90
	interval := formatDurationClock(h.cleanupSvc.GetInterval())
	if interval == "0s" {
		interval = "24h"
	}

	if config, err := h.configSvc.Get("maven_snapshot_cleanup.enabled"); err == nil {
		enabled = config.Value == "true" || config.Value == "1"
	}
	if config, err := h.configSvc.Get("maven_snapshot_cleanup.keep_last"); err == nil {
		if n, err := strconv.Atoi(config.Value); err == nil && n > 0 {
			keepLast = n
		}
	}
	if config, err := h.configSvc.Get("maven_snapshot_cleanup.max_age_days"); err == nil {
		if n, err := strconv.Atoi(config.Value); err == nil && n > 0 {
			maxAgeDays = n
		}
	}

	response.Success(c, SnapshotCleanupConfigResponse{
		Enabled:    enabled,
		KeepLast:   keepLast,
		MaxAgeDays: maxAgeDays,
		Interval:   interval,
	})
}

type UpdateSnapshotCleanupConfigRequest struct {
	Enabled    bool   `json:"enabled"`
	KeepLast   int    `json:"keep_last" binding:"required,min=1"`
	MaxAgeDays int    `json:"max_age_days" binding:"required,min=1"`
	Interval   string `json:"interval" binding:"required"`
}

func (h *SnapshotCleanupConfigHandler) UpdateConfig(c *gin.Context) {
	var req UpdateSnapshotCleanupConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	intervalDur, err := time.ParseDuration(req.Interval)
	if err != nil || intervalDur <= 0 {
		response.BadRequest(c, "invalid interval", "interval must be a valid duration (e.g., 24h, 12h, 1h)")
		return
	}

	userID := c.GetUint("userID")

	if err := h.configSvc.Set("maven_snapshot_cleanup.enabled", boolToString(req.Enabled), "bool", "maven", "启用 Maven SNAPSHOT 自动清理", false, userID); err != nil {
		internalErr(c, err, "handler error")
		return
	}
	if err := h.configSvc.Set("maven_snapshot_cleanup.keep_last", strconv.Itoa(req.KeepLast), "int", "maven", "每个 SNAPSHOT 版本保留最近构建数", false, userID); err != nil {
		internalErr(c, err, "handler error")
		return
	}
	if err := h.configSvc.Set("maven_snapshot_cleanup.max_age_days", strconv.Itoa(req.MaxAgeDays), "int", "maven", "SNAPSHOT 保留天数", false, userID); err != nil {
		internalErr(c, err, "handler error")
		return
	}
	if err := h.configSvc.Set("cleanup.interval", req.Interval, "string", "maven", "清理任务执行间隔", false, userID); err != nil {
		internalErr(c, err, "handler error")
		return
	}

	// 通知所有清理任务重新加载配置
	h.cleanupSvc.ReloadAll()

	response.Success(c, SnapshotCleanupConfigResponse{
		Enabled:    req.Enabled,
		KeepLast:   req.KeepLast,
		MaxAgeDays: req.MaxAgeDays,
		Interval:   req.Interval,
	})
}

// CleanupNow 立即执行一次 SNAPSHOT 清理。
func (h *SnapshotCleanupConfigHandler) CleanupNow(c *gin.Context) {
	deleted, err := h.cleanupSvc.CleanupNow()
	if err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.Success(c, gin.H{
		"message": "snapshot cleanup completed",
		"deleted": deleted,
	})
}
