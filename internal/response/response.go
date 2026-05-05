// Package response 提供统一的 HTTP 响应工具函数。
// 此包不依赖任何内部业务包，用于打破 handler 与 adapter 之间的循环依赖。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 标准 API 响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PaginatedData 分页数据结构
type PaginatedData struct {
	Items      interface{} `json:"items"`
	Pagination Pagination  `json:"pagination"`
}

// Pagination 分页信息
type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// Success 返回成功响应（200 OK）
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    data,
	})
}

// SuccessWithPagination 返回分页成功响应
func SuccessWithPagination(c *gin.Context, items interface{}, page, pageSize int, total int64) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: PaginatedData{
			Items: items,
			Pagination: Pagination{
				Page:       page,
				PageSize:   pageSize,
				Total:      total,
				TotalPages: totalPages,
			},
		},
	})
}

// Created 返回创建成功响应（201 Created）
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code:    http.StatusCreated,
		Message: "created",
		Data:    data,
	})
}

// NoContent 返回无内容响应（204 No Content）
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// BadRequest 返回请求错误响应（400 Bad Request）
func BadRequest(c *gin.Context, message string, errors interface{}) {
	c.JSON(http.StatusBadRequest, Response{
		Code:    http.StatusBadRequest,
		Message: message,
		Data:    errors,
	})
}

// Unauthorized 返回未授权响应（401 Unauthorized）
func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    http.StatusUnauthorized,
		Message: message,
	})
}

// Forbidden 返回禁止访问响应（403 Forbidden）
func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Message: message,
	})
}

// NotFound 返回未找到响应（404 Not Found）
func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Response{
		Code:    http.StatusNotFound,
		Message: message,
	})
}

// Conflict 返回资源冲突响应（409 Conflict）
func Conflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, Response{
		Code:    http.StatusConflict,
		Message: message,
	})
}

// InternalError 返回内部错误响应（500 Internal Server Error）
func InternalError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, Response{
		Code:    http.StatusInternalServerError,
		Message: message,
	})
}

// ErrorResponse 返回自定义状态码的错误响应
func ErrorResponse(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, Response{
		Code:    statusCode,
		Message: message,
	})
}
