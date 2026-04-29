package handler

import (
	"strconv"

	"github.com/moonlight-box/registry/internal/service"
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
		Query:    query,
		Type:     c.Query("type"),
		Scope:    c.DefaultQuery("scope", "name"),
		Sort:     c.DefaultQuery("sort", "downloads"),
		Page:     page,
		PageSize: pageSize,
	}

	result, err := h.svc.Search(c.Request.Context(), req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, result)
}
