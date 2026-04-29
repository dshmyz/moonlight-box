package handler

import (
	"strings"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/gin-gonic/gin"
)

// RepositoryHandler 仓库管理处理器
type RepositoryHandler struct {
	svc *service.RepositoryService
}

// NewRepositoryHandler 创建仓库管理处理器实例
func NewRepositoryHandler(svc *service.RepositoryService) *RepositoryHandler {
	return &RepositoryHandler{svc: svc}
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
		Name          string   `json:"name" binding:"required"`
		DisplayName   string   `json:"display_name"`
		Description   string   `json:"description"`
		Type          string   `json:"type" binding:"required"`
		PackageType   string   `json:"package_type" binding:"required"`
		RemoteURL     string   `json:"remote_url"`
		AuthType      string   `json:"auth_type"`
		AuthConfig    string   `json:"auth_config"`
		ProxyPriority int      `json:"proxy_priority"`
		TimeoutSeconds     int    `json:"timeout_seconds"`
		MaxRedirects       int    `json:"max_redirects"`
		InsecureSkipVerify bool   `json:"insecure_skip_verify"`
		FailureCacheRules  string `json:"failure_cache_rules"`
		Members       []string `json:"members"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "Invalid request", err.Error())
		return
	}

	repo := model.Repository{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		Type:          model.RepositoryType(req.Type),
		PackageType:   req.PackageType,
		RemoteURL:     req.RemoteURL,
		AuthType:      req.AuthType,
		AuthConfig:    req.AuthConfig,
		ProxyPriority: req.ProxyPriority,
		TimeoutSeconds:     req.TimeoutSeconds,
		MaxRedirects:       req.MaxRedirects,
		InsecureSkipVerify: req.InsecureSkipVerify,
		FailureCacheRules:  req.FailureCacheRules,
		Enabled:       true,
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
