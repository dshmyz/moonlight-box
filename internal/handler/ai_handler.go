package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/moonlight-box/registry/internal/ai"
	"github.com/moonlight-box/registry/internal/response"

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
		response.ErrorResponse(c, http.StatusInternalServerError, err.Error())
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
	auditLogs := h.aiService.GetAuditLogs(limit)
	response.Success(c, auditLogs)
}

func (h *AIHandler) HealthCheck(c *gin.Context) {
	err := h.aiService.HealthCheck()
	if err != nil {
		response.ErrorResponse(c, http.StatusServiceUnavailable, err.Error())
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
		response.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()

	for chunk := range stream {
		if chunk.Error != nil {
			data, _ := json.Marshal(gin.H{"error": chunk.Error.Error()})
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
			return
		}

		data, _ := json.Marshal(gin.H{
			"session_id": chunk.SessionID,
			"content":    chunk.Content,
			"tool_call":  chunk.ToolCall,
			"done":       chunk.Done,
		})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}
}
