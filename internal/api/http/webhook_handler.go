package http

import (
	"strconv"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	webhookSvc *service.WebhookService
}

func NewWebhookHandler(webhookSvc *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{
		webhookSvc: webhookSvc,
	}
}

type CreateWebhookRequest struct {
	Name        string   `json:"name" binding:"required"`
	URL         string   `json:"url" binding:"required,url"`
	Secret      string   `json:"secret"`
	Events      []string `json:"events" binding:"required"`
	Repository  string   `json:"repository"`
	PackageType string   `json:"package_type"`
}

func (h *WebhookHandler) Create(c *gin.Context) {
	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	userID := c.GetUint("userID")

	webhook := &model.Webhook{
		Name:        req.Name,
		URL:         req.URL,
		Secret:      req.Secret,
		Events:      strings.Join(req.Events, ","),
		Status:      model.WebhookStatusActive,
		Repository:  req.Repository,
		PackageType: req.PackageType,
		CreatedBy:   userID,
	}

	if err := h.webhookSvc.CreateWebhook(webhook); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, webhook)
}

func (h *WebhookHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	webhooks, total, err := h.webhookSvc.ListWebhooks(page, pageSize)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithPagination(c, webhooks, page, pageSize, total)
}

func (h *WebhookHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid webhook id", err.Error())
		return
	}

	webhook, err := h.webhookSvc.GetWebhook(uint(id))
	if err != nil {
		response.NotFound(c, "webhook not found")
		return
	}

	response.Success(c, webhook)
}

func (h *WebhookHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid webhook id", err.Error())
		return
	}

	webhook, err := h.webhookSvc.GetWebhook(uint(id))
	if err != nil {
		response.NotFound(c, "webhook not found")
		return
	}

	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	webhook.Name = req.Name
	webhook.URL = req.URL
	webhook.Secret = req.Secret
	webhook.Events = strings.Join(req.Events, ",")
	webhook.Repository = req.Repository
	webhook.PackageType = req.PackageType

	if err := h.webhookSvc.UpdateWebhook(webhook); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, webhook)
}

func (h *WebhookHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid webhook id", err.Error())
		return
	}

	if err := h.webhookSvc.DeleteWebhook(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.NoContent(c)
}

func (h *WebhookHandler) Test(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid webhook id", err.Error())
		return
	}

	if err := h.webhookSvc.TestWebhook(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "test webhook sent"})
}

func (h *WebhookHandler) ListDeliveries(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid webhook id", err.Error())
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	deliveries, total, err := h.webhookSvc.ListDeliveries(uint(id), page, pageSize)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithPagination(c, deliveries, page, pageSize, total)
}
