package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/ai"
	"github.com/moonlight-box/registry/internal/model"
)

// AIHandler AI接口处理器
type AIHandler struct {
	aiService *ai.AIService
}

// NewAIHandler 创建AI处理器
func NewAIHandler(aiService *ai.AIService) *AIHandler {
	return &AIHandler{
		aiService: aiService,
	}
}

// ChatRequest 聊天请求
type ChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message" binding:"required"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	SessionID string            `json:"session_id"`
	Message   string            `json:"message"`
	ToolCalls []ai.ToolCallInfo `json:"tool_calls,omitempty"`
	Usage     *ai.UsageInfo     `json:"usage,omitempty"`
}

// Chat 处理聊天请求
// @Summary AI聊天
// @Description 与AI助手进行对话
// @Tags AI
// @Accept json
// @Produce json
// @Param request body ChatRequest true "聊天请求"
// @Success 200 {object} ChatResponse
// @Failure 400 {object} Response
// @Failure 401 {object} Response
// @Failure 429 {object} Response
// @Failure 500 {object} Response
// @Router /api/v1/ai/chat [post]
func (h *AIHandler) Chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "无效的请求参数", err.Error())
		return
	}

	// 获取当前用户ID
	userID := c.GetUint("userID")
	if userID == 0 {
		Unauthorized(c, "未授权")
		return
	}

	// 调用AI服务
	resp, err := h.aiService.Chat(c.Request.Context(), userID, req.SessionID, req.Message)
	if err != nil {
		// 检查是否是限流错误
		if err.Error() == "请求过于频繁" {
			TooManyRequests(c, err.Error())
			return
		}
		InternalError(c, "AI服务错误: "+err.Error())
		return
	}

	Success(c, ChatResponse{
		SessionID: resp.SessionID,
		Message:   resp.Message,
		ToolCalls: resp.ToolCalls,
		Usage:     resp.Usage,
	})
}

// ListToolsResponse 工具列表响应
type ListToolsResponse struct {
	Tools []ai.ToolInfo `json:"tools"`
}

// ListTools 获取工具列表
// @Summary 获取AI工具列表
// @Description 获取所有可用的AI工具
// @Tags AI
// @Produce json
// @Success 200 {object} ListToolsResponse
// @Router /api/v1/ai/tools [get]
func (h *AIHandler) ListTools(c *gin.Context) {
	tools := h.aiService.ListTools()
	Success(c, ListToolsResponse{
		Tools: tools,
	})
}

// DeleteSession 删除会话
// @Summary 删除AI会话
// @Description 删除指定的AI会话
// @Tags AI
// @Param id path string true "会话ID"
// @Success 204 "删除成功"
// @Failure 401 {object} Response
// @Failure 404 {object} Response
// @Router /api/v1/ai/sessions/{id} [delete]
func (h *AIHandler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		BadRequest(c, "会话ID不能为空", nil)
		return
	}

	// 获取当前用户ID
	userID := c.GetUint("userID")
	if userID == 0 {
		Unauthorized(c, "未授权")
		return
	}

	// 检查会话是否属于当前用户
	session := h.aiService.GetSession(sessionID)
	if session == nil {
		NotFound(c, "会话不存在")
		return
	}

	if session.UserID != userID {
		Forbidden(c, "无权删除此会话")
		return
	}

	// 删除会话
	h.aiService.DeleteSession(sessionID)
	NoContent(c)
}

// GetRateLimitStatus 获取限流状态
// @Summary 获取AI限流状态
// @Description 获取当前用户的AI请求限流状态
// @Tags AI
// @Produce json
// @Success 200 {object} ai.RateLimitStatus
// @Router /api/v1/ai/rate-limit [get]
func (h *AIHandler) GetRateLimitStatus(c *gin.Context) {
	// 获取当前用户ID
	userID := c.GetUint("userID")
	if userID == 0 {
		Unauthorized(c, "未授权")
		return
	}

	status := h.aiService.GetRateLimitStatus(userID)
	Success(c, status)
}

// GetStats 获取服务统计
// @Summary 获取AI服务统计
// @Description 获取AI服务的统计信息
// @Tags AI
// @Produce json
// @Success 200 {object} ai.ServiceStats
// @Router /api/v1/ai/stats [get]
func (h *AIHandler) GetStats(c *gin.Context) {
	stats := h.aiService.GetStats()
	Success(c, stats)
}

// GetCacheStats 获取缓存统计
// @Summary 获取AI缓存统计
// @Description 获取AI响应缓存的统计信息
// @Tags AI
// @Produce json
// @Success 200 {object} ai.CacheStats
// @Router /api/v1/ai/cache/stats [get]
func (h *AIHandler) GetCacheStats(c *gin.Context) {
	stats := h.aiService.GetCacheStats()
	if stats == nil {
		Success(c, gin.H{"enabled": false})
		return
	}
	Success(c, stats)
}

// GetAuditLogsResponse 审计日志响应
type GetAuditLogsResponse struct {
	Logs []ai.AuditEntry `json:"logs"`
}

// GetAuditLogs 获取审计日志
// @Summary 获取AI审计日志
// @Description 获取AI工具调用的审计日志
// @Tags AI
// @Param limit query int false "返回数量限制" default(100)
// @Produce json
// @Success 200 {object} GetAuditLogsResponse
// @Router /api/v1/ai/audit-logs [get]
func (h *AIHandler) GetAuditLogs(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	logs := h.aiService.GetAuditLogs(limit)
	Success(c, GetAuditLogsResponse{
		Logs: logs,
	})
}

// HealthCheck 健康检查
// @Summary AI服务健康检查
// @Description 检查AI服务是否正常
// @Tags AI
// @Produce json
// @Success 200 {object} Response
// @Failure 503 {object} Response
// @Router /api/v1/ai/health [get]
func (h *AIHandler) HealthCheck(c *gin.Context) {
	if err := h.aiService.HealthCheck(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	Success(c, gin.H{
		"status": "healthy",
	})
}

// TooManyRequests 返回429错误
func TooManyRequests(c *gin.Context, message string) {
	c.JSON(http.StatusTooManyRequests, gin.H{
		"code":    429,
		"message": message,
	})
}

// getCurrentUser 从context中获取当前用户信息
func getCurrentUser(c *gin.Context) *model.User {
	userID := c.GetUint("userID")
	if userID == 0 {
		return nil
	}

	username, _ := c.Get("username")
	rolesInterface, _ := c.Get("roles")

	user := &model.User{
		Username: username.(string),
	}
	user.ID = userID

	if roles, ok := rolesInterface.([]string); ok {
		user.Roles = make([]model.Role, len(roles))
		for i, roleName := range roles {
			user.Roles[i] = model.Role{Name: roleName}
		}
	}

	return user
}
