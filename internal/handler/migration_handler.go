package handler

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/migration"
	"github.com/moonlight-box/registry/internal/response"
)

type MigrationHandler struct {
	service *migration.MigrationService
	worker  *migration.MigrationWorker
}

func NewMigrationHandler(service *migration.MigrationService, worker *migration.MigrationWorker) *MigrationHandler {
	return &MigrationHandler{
		service: service,
		worker:  worker,
	}
}

type TestNexusRequest struct {
	URL      string `json:"url" binding:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *MigrationHandler) TestNexusConnection(c *gin.Context) {
	var req TestNexusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	client := migration.NewNexusClient(req.URL, req.Username, req.Password)
	if err := client.TestConnection(c.Request.Context()); err != nil {
		response.BadRequest(c, "连接测试失败", err.Error())
		return
	}

	response.Success(c, gin.H{"message": "连接成功"})
}

type ListNexusReposRequest struct {
	URL      string `json:"url" binding:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *MigrationHandler) ListNexusRepositories(c *gin.Context) {
	var req ListNexusReposRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	client := migration.NewNexusClient(req.URL, req.Username, req.Password)
	repos, err := client.ListRepositories(c.Request.Context())
	if err != nil {
		response.BadRequest(c, "获取仓库列表失败", err.Error())
		return
	}

	response.Success(c, repos)
}

type CreateMigrationRequest struct {
	URL           string   `json:"url" binding:"required"`
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	SelectedRepos []string `json:"selected_repos" binding:"required"`
}

func (h *MigrationHandler) CreateMigration(c *gin.Context) {
	var req CreateMigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	task, err := h.service.CreateTask(req.URL, req.Username, req.Password, req.SelectedRepos)
	if err != nil {
		response.InternalError(c, "创建迁移任务失败: "+err.Error())
		return
	}

	// 启动异步迁移
	go func() {
		if err := h.worker.Execute(c.Request.Context(), task); err != nil {
			h.service.AddLog(task.ID, "迁移执行出错: "+err.Error())
		}
	}()

	response.Success(c, task)
}

func (h *MigrationHandler) GetMigrationStatus(c *gin.Context) {
	id := c.Param("id")

	taskID, err := parseUint(id)
	if err != nil {
		response.BadRequest(c, "无效的任务 ID", "")
		return
	}

	task, err := h.service.GetTask(taskID)
	if err != nil {
		response.NotFound(c, "任务不存在")
		return
	}

	progress := h.service.GetProgress(taskID)
	resp := gin.H{
		"task":             task,
		"processed_items":  0,
		"failed_items":     0,
		"total_items":      0,
	}

	if progress != nil {
		resp["processed_items"] = progress.Processed
		resp["failed_items"] = progress.Failed
		resp["total_items"] = progress.Total
	}

	response.Success(c, resp)
}

func (h *MigrationHandler) CancelMigration(c *gin.Context) {
	id := c.Param("id")

	taskID, err := parseUint(id)
	if err != nil {
		response.BadRequest(c, "无效的任务 ID", "")
		return
	}

	if err := h.service.CancelTask(taskID); err != nil {
		response.InternalError(c, "取消任务失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "任务已取消"})
}

func (h *MigrationHandler) ListMigrations(c *gin.Context) {
	tasks, err := h.service.ListTasks()
	if err != nil {
		response.InternalError(c, "获取迁移历史失败: "+err.Error())
		return
	}

	response.Success(c, tasks)
}

func parseUint(s string) (uint, error) {
	var v uint
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
