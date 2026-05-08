package handler

import (
	"github.com/moonlight-box/registry/internal/response"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	apperr "github.com/moonlight-box/registry/internal/errors"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/service"
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

// fillRepositoryURL 为仓库填充访问URL
func fillRepositoryURL(repo *model.Repository, scheme string, host string) {
	if repo == nil || repo.Name == "" {
		return
	}
	repo.URL = fmt.Sprintf("%s://%s/repo/%s/", scheme, host, repo.Name)
}

// fillRepositoryURLs 为仓库列表填充访问URL
func fillRepositoryURLs(repos []model.Repository, scheme string, host string) {
	for i := range repos {
		fillRepositoryURL(&repos[i], scheme, host)
	}
}

// getSchemeAndHost 从请求中获取协议和主机
func getSchemeAndHost(c *gin.Context) (string, string) {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme, c.Request.Host
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
		response.InternalError(c, "Failed to list repositories")
		return
	}

	scheme, host := getSchemeAndHost(c)
	fillRepositoryURLs(repos, scheme, host)

	for i := range repos {
		repos[i].AuthConfig = repos[i].MaskAuthConfig()
	}

	response.Success(c, repos)
}

// Get 根据名称获取仓库详情
func (h *RepositoryHandler) Get(c *gin.Context) {
	name := c.Param("name")
	repo, err := h.svc.Get(name)
	if err != nil {
		response.NotFound(c, "Repository not found")
		return
	}

	scheme, host := getSchemeAndHost(c)
	fillRepositoryURL(repo, scheme, host)

	repo.AuthConfig = repo.MaskAuthConfig()

	response.Success(c, repo)
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
		StorageBackendID   *uint    `json:"storage_backend_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
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
		StorageBackendID:   req.StorageBackendID,
	}

	if err := h.svc.Create(&repo, req.Members); err != nil {
		if apperr.IsDuplicate(err) {
			response.Conflict(c, "仓库名称已存在")
			return
		}
		response.InternalError(c, "Failed to create repository")
		return
	}

	response.Created(c, repo)
}

// Update 更新仓库信息
func (h *RepositoryHandler) Update(c *gin.Context) {
	name := c.Param("name")
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}

	if err := h.svc.Update(name, updates); err != nil {
		response.InternalError(c, "Failed to update repository")
		return
	}

	response.Success(c, gin.H{"message": "Repository updated"})
}

// Delete 删除仓库
func (h *RepositoryHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.svc.Delete(name); err != nil {
		response.InternalError(c, "Failed to delete repository")
		return
	}

	response.Success(c, gin.H{"message": "Repository deleted"})
}

// GetMembers 获取虚拟仓库的成员列表
func (h *RepositoryHandler) GetMembers(c *gin.Context) {
	name := c.Param("name")
	members, err := h.svc.GetMembers(name)
	if err != nil {
		response.InternalError(c, "Failed to get members")
		return
	}

	response.Success(c, members)
}

// AddMember 向虚拟仓库添加成员
func (h *RepositoryHandler) AddMember(c *gin.Context) {
	name := c.Param("name")
	var req struct {
		MemberName string `json:"member_name" binding:"required"`
		Priority   int    `json:"priority"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}

	if err := h.svc.AddMember(name, req.MemberName, req.Priority); err != nil {
		response.InternalError(c, "Failed to add member")
		return
	}

	response.Success(c, gin.H{"message": "Member added"})
}

// RemoveMember 从虚拟仓库移除成员
func (h *RepositoryHandler) RemoveMember(c *gin.Context) {
	name := c.Param("name")
	memberName := c.Param("memberName")

	if err := h.svc.RemoveMember(name, memberName); err != nil {
		response.InternalError(c, "Failed to remove member")
		return
	}

	response.Success(c, gin.H{"message": "Member removed"})
}

// TriggerMetadataSync 手动触发元数据同步
func (h *RepositoryHandler) TriggerMetadataSync(c *gin.Context) {
	name := c.Param("name")

	repo, err := h.svc.Get(name)
	if err != nil {
		response.NotFound(c, "Repository not found")
		return
	}

	if repo.Type != model.RepoTypeProxy {
		response.BadRequest(c, "Only proxy repository supports metadata sync", "")
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	uid, ok := userID.(uint)
	if !ok {
		response.InternalError(c, "Invalid user ID type")
		return
	}

	task, err := h.metadataSyncSvc.TriggerManualSync(repo.ID, uid)
	if err != nil {
		if apperr.IsDuplicate(err) {
			response.Conflict(c, "A sync task is already running")
			return
		}
		response.InternalError(c, "Failed to trigger metadata sync")
		return
	}

	response.Success(c, task)
}

// GetSyncHistory 获取同步历史
func (h *RepositoryHandler) GetSyncHistory(c *gin.Context) {
	repoIDStr := c.Param("id")
	repoID, err := strconv.ParseUint(repoIDStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "Invalid repository ID", "")
		return
	}

	limitStr := c.DefaultQuery("limit", "20")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 20
	}

	tasks, err := h.metadataSyncSvc.GetRepositorySyncHistory(uint(repoID), limit)
	if err != nil {
		response.InternalError(c, "Failed to get sync history")
		return
	}

	response.Success(c, tasks)
}

// GetSyncTaskStatus 获取同步任务状态
func (h *RepositoryHandler) GetSyncTaskStatus(c *gin.Context) {
	taskIDStr := c.Param("taskId")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "Invalid task ID", "")
		return
	}

	task, err := h.metadataSyncSvc.GetTaskStatus(uint(taskID))
	if err != nil {
		response.NotFound(c, "Task not found")
		return
	}

	response.Success(c, task)
}

// CancelSyncTask 取消同步任务
func (h *RepositoryHandler) CancelSyncTask(c *gin.Context) {
	taskIDStr := c.Param("taskId")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "Invalid task ID", "")
		return
	}

	if err := h.metadataSyncSvc.CancelTask(uint(taskID)); err != nil {
		response.InternalError(c, "Failed to cancel task")
		return
	}

	response.Success(c, gin.H{"message": "Task cancelled"})
}

// UpdateMetadataSyncConfig 更新元数据同步配置
func (h *RepositoryHandler) UpdateMetadataSyncConfig(c *gin.Context) {
	name := c.Param("name")

	var req struct {
		Enabled  bool `json:"enabled"`
		Interval int  `json:"interval"` // 秒
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", "")
		return
	}

	// 获取仓库信息
	repo, err := h.svc.Get(name)
	if err != nil {
		response.NotFound(c, "Repository not found")
		return
	}

	// 更新仓库配置
	updates := map[string]interface{}{
		"metadata_sync_enabled":  req.Enabled,
		"metadata_sync_interval": req.Interval,
	}

	if err := h.svc.Update(repo.Name, updates); err != nil {
		response.InternalError(c, "Failed to update metadata sync config")
		return
	}

	// 更新调度任务
	if h.schedulerSvc != nil {
		if req.Enabled {
			interval := time.Duration(req.Interval) * time.Second
			if interval <= 0 {
				interval = time.Hour
			}
			if err := h.schedulerSvc.ScheduleMetadataSync(repo.ID, interval); err != nil {
				response.InternalError(c, "Failed to schedule metadata sync")
				return
			}
		} else {
			if err := h.schedulerSvc.RemoveMetadataSync(repo.ID); err != nil {
				// 任务可能不存在，忽略错误
			}
		}
	}

	response.Success(c, gin.H{"message": "Metadata sync config updated"})
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
