package http

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
)

// SchedulerHandler 处理定时任务管理：任务列表、调度配置（cron/间隔）、立即运行。
type SchedulerHandler struct {
	configSvc *service.SystemConfigService
	scheduler *service.TaskScheduler
}

func NewSchedulerHandler(configSvc *service.SystemConfigService, scheduler *service.TaskScheduler) *SchedulerHandler {
	return &SchedulerHandler{
		configSvc: configSvc,
		scheduler: scheduler,
	}
}

type schedulerTaskItem struct {
	Name     string                      `json:"name"`
	Schedule string                      `json:"schedule"`
	Custom   bool                        `json:"custom"`
	Config   []service.TaskConfigField   `json:"config,omitempty"`
}

func (h *SchedulerHandler) taskByName(name string) service.ScheduledTask {
	for _, t := range h.scheduler.GetTasks() {
		if t.Name() == name {
			return t
		}
	}
	return nil
}

// List 返回已注册任务及其当前生效的调度与可配置参数。
func (h *SchedulerHandler) List(c *gin.Context) {
	tasks := h.scheduler.GetTasks()
	items := make([]schedulerTaskItem, 0, len(tasks))
	for _, task := range tasks {
		spec, custom := h.scheduler.ScheduleInfo(task)
		item := schedulerTaskItem{
			Name:     task.Name(),
			Schedule: spec,
			Custom:   custom,
		}
		if configurable, ok := task.(service.ConfigurableTask); ok {
			item.Config = configurable.ConfigFields()
		}
		items = append(items, item)
	}
	response.Success(c, gin.H{
		"tasks":    items,
		"interval": h.scheduler.IntervalString(),
	})
}

type UpdateSchedulerTaskRequest struct {
	// Schedule 为 cron 表达式（如 "0 3 * * *"）或 @every 间隔（如 "@every 24h"）。
	// 空串表示清除专属调度，回退到全局间隔。
	Schedule string `json:"schedule"`
	// Config 为任务的可配置参数（key → typed 值），仅对实现 ConfigurableTask 的任务生效。
	Config map[string]any `json:"config"`
}

// Update 更新某任务的调度与配置并热更新。
func (h *SchedulerHandler) Update(c *gin.Context) {
	name := c.Param("name")
	task := h.taskByName(name)
	if task == nil {
		response.NotFound(c, "scheduled task not found")
		return
	}

	var req UpdateSchedulerTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	spec := strings.TrimSpace(req.Schedule)
	if spec != "" {
		if err := service.ValidateSchedule(spec); err != nil {
			response.BadRequest(c, "invalid schedule",
				"支持 cron 表达式（如 0 3 * * *）或正数 @every 间隔（如 @every 24h）")
			return
		}
	}

	userID := c.GetUint("userID")
	if req.Config != nil {
		configurable, ok := task.(service.ConfigurableTask)
		if !ok {
			response.BadRequest(c, "task not configurable", name)
			return
		}
		// 拒绝未知配置字段，避免前端 typo 或字段漂移被静默忽略
		known := make(map[string]bool)
		for _, f := range configurable.ConfigFields() {
			known[f.Key] = true
		}
		for k := range req.Config {
			if !known[k] {
				response.BadRequest(c, "unknown config field", k)
				return
			}
		}
		if err := configurable.UpdateConfig(req.Config, userID); err != nil {
			response.BadRequest(c, "invalid config", err.Error())
			return
		}
	}

	if h.configSvc != nil {
		key := service.ScheduleConfigKey(name)
		var err error
		if spec == "" {
			err = h.configSvc.Delete(key)
		} else {
			err = h.configSvc.Set(key, spec, "string", "scheduler",
				"任务 "+name+" 的调度（cron 或 @every）", false, userID)
		}
		if err != nil {
			internalErr(c, err, "handler error")
			return
		}
	}

	h.scheduler.ReloadAll()
	spec, custom := h.scheduler.ScheduleInfo(task)
	item := schedulerTaskItem{
		Name:     name,
		Schedule: spec,
		Custom:   custom,
	}
	if configurable, ok := task.(service.ConfigurableTask); ok {
		item.Config = configurable.ConfigFields()
	}
	response.Success(c, item)
}

// Run 立即执行指定任务。
func (h *SchedulerHandler) Run(c *gin.Context) {
	name := c.Param("name")
	processed, err := h.scheduler.RunNow(name)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			response.NotFound(c, "scheduled task not found")
			return
		}
		internalErr(c, err, "handler error")
		return
	}
	response.Success(c, gin.H{
		"name":      name,
		"processed": processed,
	})
}

type UpdateSchedulerIntervalRequest struct {
	// Interval 为全局默认调度间隔（如 "24h"）。
	Interval string `json:"interval"`
}

// UpdateInterval 设置全局默认调度间隔（scheduler.interval）并热更新。
func (h *SchedulerHandler) UpdateInterval(c *gin.Context) {
	var req UpdateSchedulerIntervalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}
	d, err := time.ParseDuration(strings.TrimSpace(req.Interval))
	if err != nil || d <= 0 {
		response.BadRequest(c, "invalid interval", "必须是正数 duration（如 24h、12h）")
		return
	}
	if h.configSvc == nil {
		response.InternalError(c, "config service unavailable")
		return
	}
	userID := c.GetUint("userID")
	if err := h.configSvc.Set("scheduler.interval", req.Interval, "string", "scheduler", "全局默认调度间隔", false, userID); err != nil {
		internalErr(c, err, "handler error")
		return
	}
	h.scheduler.ReloadAll()
	response.Success(c, gin.H{"interval": h.scheduler.IntervalString()})
}
