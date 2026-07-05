package http

import (
	"strconv"

	"github.com/dshmyz/moonlight-box/internal/response"

	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/gin-gonic/gin"
)

// PackageSearchHandler 包搜索处理器
type PackageSearchHandler struct {
	svc *service.PackageSearchService
}

// NewPackageSearchHandler 创建包搜索处理器实例
func NewPackageSearchHandler(svc *service.PackageSearchService) *PackageSearchHandler {
	return &PackageSearchHandler{svc: svc}
}

// Search 处理包搜索请求
func (h *PackageSearchHandler) Search(c *gin.Context) {
	query := c.Query("q")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	req := &service.SearchRequest{
		Query:      query,
		Type:       c.Query("type"),
		Name:       c.Query("name"),
		Version:    c.Query("version"),
		Repository: c.Query("repository"),
		Sort:       c.DefaultQuery("sort", "updated_at"),
		Page:       page,
		PageSize:   pageSize,
	}

	result, err := h.svc.Search(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// List 处理关键字查询请求，一次返回包信息 + 完整版本列表。
// GET /api/v1/packages/list?q=关键字&type=npm&repository=npm-proxy&version=4.17.*&files_downloaded=true&page=1&page_size=20
//
// files_downloaded 参数：
//   - "true"（默认）：只返回已下载到本地存储的版本
//   - "false"：只返回未下载到本地存储的版本
//   - "all"：不过滤，返回所有版本
func (h *PackageSearchHandler) List(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		response.BadRequest(c, "missing query", "please provide query via 'q' query parameter")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// files_downloaded 参数：默认 true，"all" 表示不过滤
	var filesDownloaded *bool
	switch c.DefaultQuery("files_downloaded", "true") {
	case "true", "1", "":
		t := true
		filesDownloaded = &t
	case "false", "0":
		f := false
		filesDownloaded = &f
	case "all", "any":
		// nil 表示不过滤
	}

	req := &service.ListRequest{
		Query:           query,
		Type:            c.Query("type"),
		Repository:      c.Query("repository"),
		Version:         c.Query("version"),
		FilesDownloaded: filesDownloaded,
		Page:            page,
		PageSize:        pageSize,
	}

	result, err := h.svc.List(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}
