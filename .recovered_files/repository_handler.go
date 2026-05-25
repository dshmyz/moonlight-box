package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/gin-gonic/gin"
)

// RepositoryHandler 仓库管理处理器
type RepositoryHandler struct {
	svc             *service.RepositoryService
	metadataSyncSvc *service.MetadataSyncService
	schedulerSvc    *service.SchedulerService
}

// NewRepositoryHandler 创建仓库管理处理器实例
func NewRepositoryHandler(svc *service.RepositoryService, metadataSyncSvc *service.MetadataSyncService, schedulerSvc *service.SchedulerService) *RepositoryHandler {
	return &RepositoryHandler{
		svc:             svc,
		metadataSyncSvc: metadataSyncSvc,
		schedulerSvc:    schedulerSvc,
	}
}

// Service 返回 RepositoryService 实例
func (h *RepositoryHandler) Service() *service.RepositoryService {
	return h.svc
}

// List 列出仓库，支持按 package_type 和 type 过滤
func (h *RepositoryHandler) List(c *gin.Context) {
	filter := make(map[string]interface{})
	if pkgType := c.Query("package_type"); pkgType != "" {
		filter["package_type"] = pkgType
	}
	if repoType := c.Query("type"); repoType != "" {
		filter["type"] = repoType
	}

	repos, err := h.svc.List(filter)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, repos)
}

// Get 根据名称获取仓库详情
func (h *RepositoryHandler) Get(c *gin.Context) {
	name := c.Param("name")
	repo, err := h.svc.Get(name)
	if err != nil {
		NotFound(c, "Repository not found")
		return
	}

	Success(c, repo)
}

// Create 创建新仓库
func (h *RepositoryHandler) Create(c *gin.Context) {
	var req struct {
		Name               string   `json:"name" binding:"required"`
		DisplayName        string   `json:"display_name"`
		Description        string   `json:"description"`
		Type               string   `json:"type" binding:"required"`
		PackageType        string   `json:"package_type" binding:"required"`
		RemoteURL          string   `json:"remote_url"`
		AuthType           string   `json:"auth_type"`
		AuthConfig         string   `json:"auth_config"`
		ProxyPriority      int      `json:"proxy_priority"`
		TimeoutSeconds     int      `json:"timeout_seconds"`
		MaxRedirects       int      `json:"max_redirects"`
		InsecureSkipVerify bool     `json:"insecure_skip_verify"`
		FailureCacheRules  string   `json:"failure_cache_rules"`
		Members            []string `json:"members"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "Invalid request", err.Error())
		return
	}

	repo := model.Repository{
		Name:               req.Name,
		DisplayName:        req.DisplayName,
		Description:        req.Description,
		Type:               model.RepositoryType(req.Type),
		PackageType:        req.PackageType,
		RemoteURL:          req.RemoteURL,
		AuthType:           req.AuthType,
		AuthConfig:         req.AuthConfig,
		ProxyPriority:      req.ProxyPriority,
		TimeoutSeconds:     req.TimeoutSeconds,
		MaxRedirects:       req.MaxRedirects,
		InsecureSkipVerify: req.InsecureSkipVerify,
		FailureCacheRules:  req.FailureCacheRules,
		Enabled:            true,
	}

	if err := h.svc.Create(&repo, req.Members); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "duplicate key") {
			Conflict(c, "仓库名称已存在")
			return
		}
		InternalError(c, err.Error())
		return
	}

	Created(c, repo)
}

// Update 更新仓库信息
func (h *RepositoryHandler) Update(c *gin.Context) {
	name := c.Param("name")
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		BadRequest(c, "Invalid request", err.Error())
		return
	}

	if err := h.svc.Update(name, updates); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Repository updated"})
}

// Delete 删除仓库
func (h *RepositoryHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.svc.Delete(name); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Repository deleted"})
}

// GetMembers 获取虚拟仓库的成员列表
func (h *RepositoryHandler) GetMembers(c *gin.Context) {
	name := c.Param("name")
	members, err := h.svc.GetMembers(name)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, members)
}

// AddMember 向虚拟仓库添加成员
func (h *RepositoryHandler) AddMember(c *gin.Context) {
	name := c.Param("name")
	var req struct {
		MemberName string `json:"member_name" binding:"required"`
		Priority   int    `json:"priority"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "Invalid request", err.Error())
		return
	}

	if err := h.svc.AddMember(name, req.MemberName, req.Priority); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Member added"})
}

// RemoveMember 从虚拟仓库移除成员
func (h *RepositoryHandler) RemoveMember(c *gin.Context) {
	name := c.Param("name")
	memberName := c.Param("memberName")

	if err := h.svc.RemoveMember(name, memberName); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Member removed"})
}

// TriggerMetadataSync 手动触发元数据同步
func (h *RepositoryHandler) TriggerMetadataSync(c *gin.Context) {
	repoIDStr := c.Param("id")
	repoID, err := strconv.ParseUint(repoIDStr, 10, 32)
	if err != nil {
		BadRequest(c, "Invalid repository ID", err.Error())
		return
	}

	// 获取用户ID
	userID, exists := c.Get("userID")
	if !exists {
		Unauthorized(c, "User not authenticated")
		return
	}

	task, err := h.metadataSyncSvc.TriggerManualSync(uint(repoID), userID.(uint))
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, task)
}

// GetSyncHistory 获取同步历史
func (h *RepositoryHandler) GetSyncHistory(c *gin.Context) {
	repoIDStr := c.Param("id")
	repoID, err := strconv.ParseUint(repoIDStr, 10, 32)
	if err != nil {
		BadRequest(c, "Invalid repository ID", err.Error())
		return
	}

	limitStr := c.DefaultQuery("limit", "20")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 20
	}

	tasks, err := h.metadataSyncSvc.GetRepositorySyncHistory(uint(repoID), limit)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, tasks)
}

// GetSyncTaskStatus 获取同步任务状态
func (h *RepositoryHandler) GetSyncTaskStatus(c *gin.Context) {
	taskIDStr := c.Param("taskId")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		BadRequest(c, "Invalid task ID", err.Error())
		return
	}

	task, err := h.metadataSyncSvc.GetTaskStatus(uint(taskID))
	if err != nil {
		NotFound(c, "Task not found")
		return
	}

	Success(c, task)
}

// CancelSyncTask 取消同步任务
func (h *RepositoryHandler) CancelSyncTask(c *gin.Context) {
	taskIDStr := c.Param("taskId")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		BadRequest(c, "Invalid task ID", err.Error())
		return
	}

	if err := h.metadataSyncSvc.CancelTask(uint(taskID)); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Task cancelled"})
}

// UpdateMetadataSyncConfig 更新元数据同步配置
func (h *RepositoryHandler) UpdateMetadataSyncConfig(c *gin.Context) {
	repoIDStr := c.Param("id")
	repoID, err := strconv.ParseUint(repoIDStr, 10, 32)
	if err != nil {
		BadRequest(c, "Invalid repository ID", err.Error())
		return
	}

	var req struct {
		Enabled  bool `json:"enabled"`
		Interval int  `json:"interval"` // 秒
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "Invalid request", err.Error())
		return
	}

	// 获取仓库信息
	repo, err := h.svc.GetByID(uint(repoID))
	if err != nil {
		NotFound(c, "Repository not found")
		return
	}

	// 更新仓库配置
	updates := map[string]interface{}{
		"metadata_sync_enabled":  req.Enabled,
		"metadata_sync_interval": req.Interval,
	}

	if err := h.svc.Update(repo.Name, updates); err != nil {
		InternalError(c, err.Error())
		return
	}

	// 更新调度任务
	if h.schedulerSvc != nil {
		if req.Enabled {
			interval := time.Duration(req.Interval) * time.Second
			if interval <= 0 {
				interval = time.Hour
			}
			if err := h.schedulerSvc.ScheduleMetadataSync(uint(repoID), interval); err != nil {
				InternalError(c, err.Error())
				return
			}
		} else {
			if err := h.schedulerSvc.RemoveMetadataSync(uint(repoID)); err != nil {
				// 任务可能不存在，忽略错误
			}
		}
	}

	Success(c, gin.H{"message": "Metadata sync config updated"})
}

// RegisterRoutes 注册路由
func (h *RepositoryHandler) RegisterRoutes(protected *gin.RouterGroup, roleRepo interface{}, permMw func(resource, action string) gin.HandlerFunc) {
	// 仓库管理
	repos := protected.Group("/repositories")
	repos.Use(permMw("repositories", "read"))
	{
		repos.GET("", h.List)
		repos.GET("/:name", h.Get)
		repos.GET("/:name/members", h.GetMembers)
	}

	reposWrite := protected.Group("/repositories")
	reposWrite.Use(permMw("repositories", "write"))
	{
		reposWrite.POST("", h.Create)
		reposWrite.PUT("/:name", h.Update)
		reposWrite.POST("/:name/members", h.AddMember)

		// 元数据同步相关路由
		reposWrite.POST("/:id/metadata-sync", h.TriggerMetadataSync)
		reposWrite.PUT("/:id/metadata-sync-config", h.UpdateMetadataSyncConfig)
	}

	reposDelete := protected.Group("/repositories")
	reposDelete.Use(permMw("repositories", "delete"))
	{
		reposDelete.DELETE("/:name", h.Delete)
		reposDelete.DELETE("/:name/members/:memberName", h.RemoveMember)
	}

	// 同步历史和任务状态（读取权限）
	syncRead := protected.Group("/repositories")
	syncRead.Use(permMw("repositories", "read"))
	{
		syncRead.GET("/:id/sync-history", h.GetSyncHistory)
		syncRead.GET("/sync-tasks/:taskId", h.GetSyncTaskStatus)
	}

	// 取消任务（写入权限）
	syncWrite := protected.Group("/repositories")
	syncWrite.Use(permMw("repositories", "write"))
	{
		syncWrite.POST("/sync-tasks/:taskId/cancel", h.CancelSyncTask)
	}
}
