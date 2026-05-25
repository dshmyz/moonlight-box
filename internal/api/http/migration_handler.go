package http

import (
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/migration"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/gin-gonic/gin"
)

type MigrationHandler struct {
	service            *migration.MigrationService
	worker             migration.MigrationWorkerInterface
	repoRepo           *repository.RepositoryRepository
	storageBackendRepo *repository.StorageBackendRepository
}

func NewMigrationHandler(service *migration.MigrationService, worker migration.MigrationWorkerInterface, repoRepo *repository.RepositoryRepository, storageBackendRepo *repository.StorageBackendRepository) *MigrationHandler {
	return &MigrationHandler{
		service:            service,
		worker:             worker,
		repoRepo:           repoRepo,
		storageBackendRepo: storageBackendRepo,
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
	URL                string   `json:"url" binding:"required"`
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	SelectedRepos      []string `json:"selected_repos" binding:"required"`
	TargetRepositoryID uint     `json:"target_repository_id"`
	TargetRepository   string   `json:"target_repository"`
	WorkerCount        int      `json:"worker_count"`
	MaxRetries         int      `json:"max_retries"`
	BatchSize          int      `json:"batch_size"`
	SyncRepos          bool     `json:"sync_repos"`
}

func (h *MigrationHandler) CreateMigration(c *gin.Context) {
	var req CreateMigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	task, err := h.service.CreateTask(req.URL, req.Username, req.Password, req.SelectedRepos, req.TargetRepositoryID, req.TargetRepository, req.WorkerCount, req.MaxRetries, req.BatchSize)
	if err != nil {
		response.InternalError(c, "创建迁移任务失败: "+err.Error())
		return
	}

	// 同步 Nexus 仓库配置
	if req.SyncRepos && h.repoRepo != nil {
		client := migration.NewNexusClient(req.URL, req.Username, req.Password)
		synced, err := h.syncNexusRepos(c, client, req.SelectedRepos)
		if err != nil {
			h.service.AddLog(task.ID, "同步仓库配置失败: "+err.Error())
		} else {
			h.service.AddLog(task.ID, fmt.Sprintf("成功同步 %d 个仓库配置", synced))
		}
	}

	// 移除立即执行的 goroutine
	// 任务已经在 CreateTask 中自动入队

	response.Success(c, task)
}

func (h *MigrationHandler) SyncNexusRepos(c *gin.Context) {
	var req struct {
		URL      string   `json:"url" binding:"required"`
		Username string   `json:"username"`
		Password string   `json:"password"`
		Repos    []string `json:"repos"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	if h.repoRepo == nil {
		response.InternalError(c, "仓库服务不可用")
		return
	}

	client := migration.NewNexusClient(req.URL, req.Username, req.Password)
	synced, err := h.syncNexusRepos(c, client, req.Repos)
	if err != nil {
		response.InternalError(c, "同步仓库配置失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"synced": synced, "message": fmt.Sprintf("成功同步 %d 个仓库配置", synced)})
}

type SyncConfigOnlyRequest struct {
	URL           string   `json:"url" binding:"required"`
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	SelectedRepos []string `json:"selected_repos"`
	Repos         []string `json:"repos"`
}

func (h *MigrationHandler) SyncConfigOnly(c *gin.Context) {
	var req SyncConfigOnlyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	selectedRepos := req.SelectedRepos
	// 向后兼容前端历史字段 `repos`
	if len(selectedRepos) == 0 && len(req.Repos) > 0 {
		selectedRepos = req.Repos
	}

	task, err := h.service.CreateSyncConfigTask(req.URL, req.Username, req.Password, selectedRepos)
	if err != nil {
		response.InternalError(c, "创建同步配置任务失败: "+err.Error())
		return
	}

	response.Success(c, task)
}

func (h *MigrationHandler) syncNexusRepos(c *gin.Context, client *migration.NexusClient, repoNames []string) (int, error) {
	nexusRepos, err := client.ListRepositories(c.Request.Context())
	if err != nil {
		return 0, err
	}

	// 查找默认存储后端
	var defaultBackendID *uint
	if h.storageBackendRepo != nil {
		if defaultBackend, err := h.storageBackendRepo.FindDefault(); err == nil {
			defaultBackendID = &defaultBackend.ID
		}
	}

	synced := 0
	for _, nr := range nexusRepos {
		if len(repoNames) > 0 {
			found := false
			for _, name := range repoNames {
				if nr.Name == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if nr.Format == "" || nr.Type == "" {
			continue
		}

		repoType := h.mapRepoType(nr.Type)
		if !h.repoExists(nr.Name) {
			repo := &model.Repository{
				Name:             nr.Name,
				Type:             model.RepositoryType(repoType),
				PackageType:      util.NormalizePackageType(nr.Format),
				Enabled:          true,
				StorageBackendID: defaultBackendID,
			}

			// 代理仓库需要设置远程地址
			if nr.Type == "proxy" {
				// 获取仓库详细配置以获取正确的远程URL
				detail, err := client.GetRepositoryDetail(c.Request.Context(), nr.Name)
				remoteURL := ""
				if err == nil && detail != nil && detail.Proxy != nil && detail.Proxy.RemoteURL != "" {
					remoteURL = detail.Proxy.RemoteURL
				} else if nr.URL != "" {
					// 如果获取详情失败，使用列表API返回的URL作为备用
					remoteURL = nr.URL
				}
				repo.Config = &model.RepositoryConfig{
					RemoteURL:       remoteURL,
					CacheEnabled:    true,
					CacheTTLSeconds: 86400,
				}
			}

			// 虚拟仓不需要存储后端
			if nr.Type == "group" {
				repo.StorageBackendID = nil
			}

			if err := h.repoRepo.Create(repo); err != nil {
				continue
			}
			synced++
		}
	}
	return synced, nil
}

func (h *MigrationHandler) mapRepoType(nexusType string) string {
	switch nexusType {
	case "proxy":
		return "proxy"
	case "hosted":
		return "local"
	case "group":
		return "virtual"
	default:
		return "local"
	}
}

func (h *MigrationHandler) repoExists(name string) bool {
	_, err := h.repoRepo.FindByName(name)
	return err == nil
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
		"task":            task,
		"processed_items": task.ProcessedItems,
		"failed_items":    task.FailedItems,
		"total_items":     task.TotalItems,
		"logs":            []string{},
	}

	if progress != nil {
		resp["processed_items"] = progress.Processed
		resp["failed_items"] = progress.Failed
		resp["total_items"] = progress.Total
		resp["logs"] = progress.Logs
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

func (h *MigrationHandler) RetryFailedMigration(c *gin.Context) {
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

	if task.Status == model.MigrationRunning {
		response.BadRequest(c, "任务正在运行中，无法重试", "")
		return
	}

	h.service.RetryFailedTask(task)

	response.Success(c, gin.H{"message": "重试任务已加入队列"})
}

func (h *MigrationHandler) StartMigration(c *gin.Context) {
	id := c.Param("id")

	taskID, err := parseUint(id)
	if err != nil {
		response.BadRequest(c, "无效的任务 ID", "")
		return
	}

	if err := h.service.StartTask(taskID); err != nil {
		response.BadRequest(c, err.Error(), "")
		return
	}

	response.Success(c, gin.H{"message": "任务已加入队列"})
}

func (h *MigrationHandler) ListMigrations(c *gin.Context) {
	tasks, err := h.service.ListTasks()
	if err != nil {
		response.InternalError(c, "获取迁移历史失败: "+err.Error())
		return
	}

	response.Success(c, tasks)
}

func (h *MigrationHandler) ListMigrationItems(c *gin.Context) {
	id := c.Param("id")

	taskID, err := parseUint(id)
	if err != nil {
		response.BadRequest(c, "无效的任务 ID", "")
		return
	}

	page := 1
	pageSize := 50

	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	if ps := c.Query("page_size"); ps != "" {
		fmt.Sscanf(ps, "%d", &pageSize)
	}
	if pageSize > 100 {
		pageSize = 100
	}

	items, total, err := h.service.ListItems(taskID, page, pageSize)
	if err != nil {
		response.InternalError(c, "获取迁移项列表失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"list":      items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *MigrationHandler) GetQueueStatus(c *gin.Context) {
	status := h.service.GetQueueStatus()
	response.Success(c, status)
}

// 用户迁移相关API

type SyncUsersRequest struct {
	URL      string `json:"url" binding:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *MigrationHandler) SyncUsersFromNexus(c *gin.Context) {
	var req SyncUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	task, err := h.service.CreateUserMigrationTask(req.URL, req.Username, req.Password)
	if err != nil {
		response.InternalError(c, "创建用户迁移任务失败: "+err.Error())
		return
	}

	response.Success(c, task)
}

func (h *MigrationHandler) ListNexusUsers(c *gin.Context) {
	var req struct {
		URL      string `json:"url" binding:"required"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	client := migration.NewNexusClient(req.URL, req.Username, req.Password)
	users, err := client.ListUsers(c.Request.Context())
	if err != nil {
		response.BadRequest(c, "获取用户列表失败", err.Error())
		return
	}

	response.Success(c, users)
}

func (h *MigrationHandler) ListNexusRoles(c *gin.Context) {
	var req struct {
		URL      string `json:"url" binding:"required"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	client := migration.NewNexusClient(req.URL, req.Username, req.Password)
	roles, err := client.ListRoles(c.Request.Context())
	if err != nil {
		response.BadRequest(c, "获取角色列表失败", err.Error())
		return
	}

	response.Success(c, roles)
}

func parseUint(s string) (uint, error) {
	var v uint
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
