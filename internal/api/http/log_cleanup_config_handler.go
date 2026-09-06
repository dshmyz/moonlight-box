package http

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/util"
)

// scheduleDisplay 把任务的调度表达式转成可读字符串：@every <间隔> 显示为 <间隔>，cron 表达式原样显示。
func scheduleDisplay(spec string, fallback time.Duration) string {
	if strings.HasPrefix(spec, "@every ") {
		return strings.TrimPrefix(spec, "@every ")
	}
	if spec == "" {
		return util.CompactDuration(fallback)
	}
	return spec
}

// LogCleanupConfigHandler 处理下载日志清理配置的 HTTP 请求。
type LogCleanupConfigHandler struct {
	configSvc     *service.SystemConfigService
	taskScheduler *service.TaskScheduler
}

func NewLogCleanupConfigHandler(configSvc *service.SystemConfigService, taskScheduler *service.TaskScheduler) *LogCleanupConfigHandler {
	return &LogCleanupConfigHandler{
		configSvc:     configSvc,
		taskScheduler: taskScheduler,
	}
}

type LogCleanupConfigResponse struct {
	Enabled       bool   `json:"enabled"`
	RetentionDays int    `json:"retention_days"`
	Interval      string `json:"interval"`
}

// configTask 返回 log_cleanup 任务的 ConfigurableTask 视图（无则返回 nil）。
func (h *LogCleanupConfigHandler) configTask() service.ConfigurableTask {
	for _, t := range h.taskScheduler.GetTasks() {
		if t.Name() == "log_cleanup" {
			if ct, ok := t.(service.ConfigurableTask); ok {
				return ct
			}
		}
	}
	return nil
}

func (h *LogCleanupConfigHandler) GetConfig(c *gin.Context) {
	enabled := true
	retentionDays := 30
	interval := scheduleDisplay(h.taskScheduler.ScheduleForName("log_cleanup"), h.taskScheduler.GetInterval())

	// 配置值统一从任务自身的 ConfigFields 读取，避免与调度页维护的两套拷贝
	if ct := h.configTask(); ct != nil {
		for _, f := range ct.ConfigFields() {
			switch f.Key {
			case "enabled":
				if v, ok := f.Value.(bool); ok {
					enabled = v
				}
			case "retention_days":
				if v, ok := f.Value.(int); ok {
					retentionDays = v
				}
			}
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

	// 兼容两种来源：旧页发送纯 duration（如 "24h"），统一调度页可能已存 cron/@every。
	// 纯 duration 转成 @every 间隔；其余按 cron/@every 原样校验写入。
	spec := strings.TrimSpace(req.Interval)
	if d, err := time.ParseDuration(spec); err == nil && d > 0 {
		spec = "@every " + spec
	} else if err := service.ValidateSchedule(spec); err != nil {
		response.BadRequest(c, "invalid interval",
			"支持 duration（如 24h）、@every 间隔或 cron 表达式（如 0 3 * * *）")
		return
	}

	userID := c.GetUint("userID")

	// 配置写入统一走任务自身的 UpdateConfig，与调度页共用同一套校验/持久化
	if ct := h.configTask(); ct != nil {
		if err := ct.UpdateConfig(map[string]any{
			"enabled":        req.Enabled,
			"retention_days": float64(req.RetentionDays),
		}, userID); err != nil {
			response.BadRequest(c, "invalid config", err.Error())
			return
		}
	}

	if err := h.configSvc.Set(service.ScheduleConfigKey("log_cleanup"), spec, "string", "scheduler", "下载日志清理调度（cron 或 @every 间隔）", false, userID); err != nil {
		internalErr(c, err, "handler error")
		return
	}

	// 热更新任务调度
	h.taskScheduler.ReloadAll()

	response.Success(c, LogCleanupConfigResponse{
		Enabled:       req.Enabled,
		RetentionDays: req.RetentionDays,
		Interval:      req.Interval,
	})
}

// CleanupNow 立即执行一次日志清理。
func (h *LogCleanupConfigHandler) CleanupNow(c *gin.Context) {
	// 任务被禁用时 RunNow 会静默跳过，这里明确提示，避免"点了没反应"
	if ct := h.configTask(); ct != nil {
		for _, f := range ct.ConfigFields() {
			if f.Key == "enabled" {
				if v, ok := f.Value.(bool); ok && !v {
					response.Success(c, gin.H{"message": "日志清理已禁用，本次未执行"})
					return
				}
			}
		}
	}

	if _, err := h.taskScheduler.RunNow("log_cleanup"); err != nil {
		internalErr(c, err, "handler error")
		return
	}

	response.Success(c, gin.H{
		"message": "cleanup completed",
	})
}
