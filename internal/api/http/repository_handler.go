package http

import (
	"fmt"
	"strconv"
	"strings"

	apperr "github.com/dshmyz/moonlight-box/internal/errors"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/gin-gonic/gin"
)

// RepositoryHandler 仓库管理处理器
type RepositoryHandler struct {
	svc *service.RepositoryService
}

// NewRepositoryHandler 创建仓库管理处理器实例
func NewRepositoryHandler(svc *service.RepositoryService) *RepositoryHandler {
	return &RepositoryHandler{
		svc: svc,
	}
}

// fillRepositoryURL 为仓库填充访问URL
func fillRepositoryURL(repo *model.Repository, scheme string, host string, prefix string) {
	if repo == nil || repo.Name == "" {
		return
	}
	repo.URL = fmt.Sprintf("%s://%s%s/repository/%s/", scheme, host, prefix, repo.Name)
}

func fillRepositoryListURLs(repos []service.RepositoryListView, scheme string, host string, prefix string) {
	for i := range repos {
		fillRepositoryURL(&repos[i].Repository, scheme, host, prefix)
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

// parsePagination 从请求中解析分页参数
// 只有客户端显式传了 page 或 page_size 时才启用分页（默认 page=1, pageSize=20, maxPageSize=100）
// 否则返回 0,0 表示不分页，兼容旧前端
func parsePagination(c *gin.Context) (page, pageSize int) {
	pageStr := c.Query("page")
	sizeStr := c.Query("page_size")
	if pageStr == "" && sizeStr == "" {
		return 0, 0
	}
	page, _ = strconv.Atoi(pageStr)
	pageSize, _ = strconv.Atoi(sizeStr)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return
}

// getForwardedPrefix 从请求头中获取反向代理路径前缀
func getForwardedPrefix(c *gin.Context) string {
	prefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	if prefix == "" {
		prefix = strings.TrimRight(c.GetHeader("X-Script-Name"), "/")
	}
	return prefix
}

// List 列出仓库，支持按 package_type、type、keyword 过滤和分页
func (h *RepositoryHandler) List(c *gin.Context) {
	filter := make(map[string]interface{})
	if pkgType := c.Query("package_type"); pkgType != "" {
		filter["package_type"] = pkgType
	}
	if repoType := c.Query("type"); repoType != "" {
		filter["type"] = repoType
	}
	if keyword := c.Query("keyword"); keyword != "" {
		filter["keyword"] = keyword
	}

	page, pageSize := parsePagination(c)

	repos, total, err := h.svc.ListWithHealthContext(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.InternalError(c, "Failed to list repositories")
		return
	}

	scheme, host := getSchemeAndHost(c)
	prefix := getForwardedPrefix(c)
	fillRepositoryListURLs(repos, scheme, host, prefix)

	for i := range repos {
		repos[i].Config = repos[i].SanitizedConfig()
	}

	if page > 0 && pageSize > 0 {
		response.SuccessWithPagination(c, repos, page, pageSize, total)
	} else {
		response.Success(c, repos)
	}
}

// Get 根据名称获取仓库详情
func (h *RepositoryHandler) Get(c *gin.Context) {
	name := c.Param("name")
	repo, err := h.svc.GetContext(c.Request.Context(), name)
	if err != nil {
		response.NotFound(c, "Repository not found")
		return
	}

	scheme, host := getSchemeAndHost(c)
	prefix := getForwardedPrefix(c)
	fillRepositoryURL(repo, scheme, host, prefix)

	repo.Config = repo.SanitizedConfig()

	response.Success(c, repo)
}

// Create 创建新仓库
func (h *RepositoryHandler) Create(c *gin.Context) {
	var req struct {
		Name             string                  `json:"name" binding:"required"`
		DisplayName      string                  `json:"display_name"`
		Description      string                  `json:"description"`
		Type             string                  `json:"type" binding:"required"`
		PackageType      string                  `json:"package_type" binding:"required"`
		Config           *model.RepositoryConfig `json:"config"`
		Members          []string                `json:"members"`
		StorageBackendID *uint                   `json:"storage_backend_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}

	repo := model.Repository{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Type: func() model.RepositoryType {
			// 规范化：group 等同于 virtual（Nexus 兼容）
			if req.Type == "group" {
				return model.RepoTypeVirtual
			}
			return model.RepositoryType(req.Type)
		}(),
		PackageType:      req.PackageType,
		Config:           req.Config,
		Enabled:          true,
		StorageBackendID: req.StorageBackendID,
	}

	if err := h.svc.Create(&repo, req.Members); err != nil {
		if apperr.IsDuplicate(err) {
			response.Conflict(c, "仓库名称已存在")
			return
		}
		response.ErrorResponse(c, 400, err.Error())
		return
	}

	response.Created(c, repo)
}

// Update 更新仓库信息
func (h *RepositoryHandler) Update(c *gin.Context) {
	name := c.Param("name")
	var req model.UpdateRepositoryParams
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}

	if err := h.svc.Update(name, &req); err != nil {
		response.ErrorResponse(c, 400, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Repository updated"})
}

// Delete 删除仓库
func (h *RepositoryHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.svc.Delete(name); err != nil {
		response.WriteAppError(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Repository deleted"})
}

// GetMembers 获取虚拟仓库的成员列表
func (h *RepositoryHandler) GetMembers(c *gin.Context) {
	name := c.Param("name")
	members, err := h.svc.GetMembersContext(c.Request.Context(), name)
	if err != nil {
		response.WriteAppError(c, err)
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
		response.WriteAppError(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Member added"})
}

// RemoveMember 从虚拟仓库移除成员
func (h *RepositoryHandler) RemoveMember(c *gin.Context) {
	name := c.Param("name")
	memberName := c.Param("memberName")

	if err := h.svc.RemoveMember(name, memberName); err != nil {
		response.WriteAppError(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Member removed"})
}


