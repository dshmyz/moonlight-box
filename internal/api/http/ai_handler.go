package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dshmyz/moonlight-box/internal/ai"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

type AIHandler struct {
	aiService *ai.AIService
}

func NewAIHandler(aiService *ai.AIService) *AIHandler {
	return &AIHandler{
		aiService: aiService,
	}
}

type AIChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message" binding:"required"`
}

func (h *AIHandler) Chat(c *gin.Context) {
	var req AIChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}

	userID := c.GetUint("userID")

	resp, err := h.aiService.Chat(c.Request.Context(), userID, req.SessionID, req.Message)
	if err != nil {
		logrus.WithError(err).Warn("AI chat request failed")
		response.InternalError(c, "AI service temporarily unavailable")
		return
	}

	response.Success(c, resp)
}

func (h *AIHandler) ListTools(c *gin.Context) {
	tools := h.aiService.ListTools()
	response.Success(c, tools)
}

func (h *AIHandler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		response.BadRequest(c, "session_id is required", "")
		return
	}

	h.aiService.DeleteSession(sessionID)
	response.Success(c, gin.H{"message": "session deleted"})
}

func (h *AIHandler) GetRateLimitStatus(c *gin.Context) {
	userID := c.GetUint("userID")
	status := h.aiService.GetRateLimitStatus(userID)
	response.Success(c, status)
}

func (h *AIHandler) GetStats(c *gin.Context) {
	stats := h.aiService.GetStats()
	response.Success(c, stats)
}

func (h *AIHandler) GetCacheStats(c *gin.Context) {
	stats := h.aiService.GetCacheStats()
	response.Success(c, stats)
}

func (h *AIHandler) GetAuditLogs(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	success := parseOptionalBool(c.Query("success"))

	filter := ai.AuditFilter{
		ToolName: c.Query("tool"),
		Username: c.Query("username"),
		Success:  success,
		Limit:    limit,
		Offset:   offset,
	}
	if st := c.Query("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			filter.StartTime = &t
		}
	}
	if et := c.Query("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			filter.EndTime = &t
		}
	}

	auditLogs, total, err := h.aiService.QueryAuditLogs(filter)
	if err != nil {
		logrus.WithError(err).Warn("AI audit query failed")
		response.InternalError(c, "failed to query audit logs")
		return
	}
	response.SuccessWithPagination(c, auditLogs, offset/limit+1, limit, total)
}

func (h *AIHandler) ExportAuditLogs(c *gin.Context) {
	limit := 10000
	format := c.DefaultQuery("format", "json")
	success := parseOptionalBool(c.Query("success"))

	filter := ai.AuditFilter{
		ToolName: c.Query("tool"),
		Username: c.Query("username"),
		Success:  success,
		Limit:    limit,
	}
	if st := c.Query("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			filter.StartTime = &t
		}
	}
	if et := c.Query("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			filter.EndTime = &t
		}
	}

	data, err := h.aiService.ExportAuditLogs(filter, format)
	if err != nil {
		logrus.WithError(err).Warn("AI audit export failed")
		response.InternalError(c, "failed to export audit logs")
		return
	}

	switch format {
	case "csv":
		c.Header("Content-Disposition", `attachment; filename="ai-audit-logs.csv"`)
		c.Data(http.StatusOK, "text/csv", data)
	default:
		c.Header("Content-Disposition", `attachment; filename="ai-audit-logs.json"`)
		c.Data(http.StatusOK, "application/json", data)
	}
}

// VerifyAuditChain 校验 AI 审计日志哈希链（防篡改）。
// earliest_id 可选：指定校验起始日志 ID（默认 0 = 从链头校验全部 AI 日志）。
func (h *AIHandler) VerifyAuditChain(c *gin.Context) {
	var earliestID uint64
	if v := c.Query("earliest_id"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			response.BadRequest(c, "invalid earliest_id", "")
			return
		}
		earliestID = n
	}
	tampered, err := h.aiService.VerifyAuditChain(uint(earliestID))
	if err != nil {
		logrus.WithError(err).Warn("AI audit chain verification failed")
		response.InternalError(c, "failed to verify audit chain")
		return
	}
	if tampered == nil {
		tampered = []uint{}
	}
	response.Success(c, gin.H{
		"intact":   len(tampered) == 0,
		"tampered": tampered,
	})
}

func parseOptionalBool(s string) *bool {
	switch s {
	case "true", "1":
		v := true
		return &v
	case "false", "0":
		v := false
		return &v
	default:
		return nil
	}
}

// ===== 提示词模板治理 API =====

type CreatePromptRequest struct {
	Name        string `json:"name" binding:"required"`
	Content     string `json:"content" binding:"required"`
	ABGroup     string `json:"ab_group"`
	Weight      int    `json:"weight"`
	Description string `json:"description"`
}

func (h *AIHandler) ListPrompts(c *gin.Context) {
	templates, err := h.aiService.ListPromptTemplates()
	if err != nil {
		logrus.WithError(err).Warn("list prompt templates failed")
		response.InternalError(c, "failed to list prompt templates")
		return
	}
	response.Success(c, templates)
}

func (h *AIHandler) GetPrompt(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid template id", "")
		return
	}
	tpl, err := h.aiService.GetPromptTemplate(uint(id))
	if err != nil {
		response.NotFound(c, "template not found")
		return
	}
	response.Success(c, tpl)
}

func (h *AIHandler) CreatePrompt(c *gin.Context) {
	var req CreatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}
	userID := c.GetUint("userID")
	tpl, err := h.aiService.CreatePromptTemplate(req.Name, req.Content, req.ABGroup, req.Description, req.Weight, userID)
	if err != nil {
		response.BadRequest(c, err.Error(), "")
		return
	}
	response.Success(c, tpl)
}

// respondPromptError 统一映射提示词治理操作的错误：模板不存在 → 404，其余业务/校验错误 → 400。
func respondPromptError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(c, "template not found")
		return
	}
	response.BadRequest(c, err.Error(), "")
}

func (h *AIHandler) ActivatePrompt(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid template id", "")
		return
	}
	userID := c.GetUint("userID")
	tpl, err := h.aiService.ActivatePromptTemplate(uint(id), userID)
	if err != nil {
		respondPromptError(c, err)
		return
	}
	response.Success(c, tpl)
}

func (h *AIHandler) RetirePrompt(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid template id", "")
		return
	}
	userID := c.GetUint("userID")
	if err := h.aiService.RetirePromptTemplate(uint(id), userID); err != nil {
		respondPromptError(c, err)
		return
	}
	response.Success(c, gin.H{"message": "template retired"})
}

func (h *AIHandler) DeletePrompt(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid template id", "")
		return
	}
	userID := c.GetUint("userID")
	if err := h.aiService.DeletePromptTemplate(uint(id), userID); err != nil {
		respondPromptError(c, err)
		return
	}
	response.Success(c, gin.H{"message": "template deleted"})
}

func (h *AIHandler) HealthCheck(c *gin.Context) {
	err := h.aiService.HealthCheck()
	if err != nil {
		logrus.WithError(err).Warn("AI health check failed")
		response.ErrorResponse(c, http.StatusServiceUnavailable, "AI service is not available")
		return
	}
	response.Success(c, gin.H{"status": "healthy"})
}

func (h *AIHandler) StreamChat(c *gin.Context) {
	var req AIChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body", err.Error())
		return
	}

	userID := c.GetUint("userID")

	stream, err := h.aiService.StreamChat(c.Request.Context(), userID, req.SessionID, req.Message)
	if err != nil {
		logrus.WithError(err).Warn("AI stream chat failed")
		response.InternalError(c, "AI service temporarily unavailable")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()

	for chunk := range stream {
		if chunk.Error != nil {
			data, err := json.Marshal(gin.H{"error": chunk.Error.Error()})
			if err != nil {
				logrus.WithError(err).Error("failed to marshal error chunk")
				return
			}
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
			return
		}

		data, err := json.Marshal(gin.H{
			"session_id": chunk.SessionID,
			"content":    chunk.Content,
			"tool_call":  chunk.ToolCall,
			"done":       chunk.Done,
		})
		if err != nil {
			logrus.WithError(err).Error("failed to marshal stream chunk")
			return
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}
}
