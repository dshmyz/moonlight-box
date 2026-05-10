package handler

import (
	"github.com/moonlight-box/registry/internal/response"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/service"
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
		Scope:      c.DefaultQuery("scope", "name"),
		Sort:       c.DefaultQuery("sort", "downloads"),
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
