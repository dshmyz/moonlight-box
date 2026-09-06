package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)

// RiskAssessmentHandler 风险组件研判：上传风险清单 Excel → 匹配制品与依赖 → 落库回看。
type RiskAssessmentHandler struct {
	service *service.RiskAssessmentService
}

func NewRiskAssessmentHandler(riskSvc *service.RiskAssessmentService) *RiskAssessmentHandler {
	return &RiskAssessmentHandler{service: riskSvc}
}

// UploadAssessment 上传 Excel/CSV 风险组件清单，执行研判并保存。
func (h *RiskAssessmentHandler) UploadAssessment(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file", "请上传 Excel/CSV 文件（字段名 file）")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		response.InternalError(c, "failed to open uploaded file")
		return
	}
	defer file.Close()

	components, err := h.service.ParseComponents(file, fileHeader.Filename)
	if err != nil {
		response.BadRequest(c, "parse failed", err.Error())
		return
	}

	createdBy := c.GetUint("userID")
	assessment, err := h.service.Analyze(c.Request.Context(), fileHeader.Filename, createdBy, components)
	if err != nil {
		response.InternalError(c, "failed to run risk assessment")
		return
	}
	h.service.EnrichSuggestions(c.Request.Context(), assessment)

	response.Success(c, assessment)
}

// AnalyzeComponents 直接提交组件列表执行研判（供 AI 解析结果或手动录入使用）。
func (h *RiskAssessmentHandler) AnalyzeComponents(c *gin.Context) {
	var req struct {
		FileName   string                      `json:"file_name"`
		Components []service.RiskComponentInput `json:"components" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}
	createdBy := c.GetUint("userID")
	assessment, err := h.service.Analyze(c.Request.Context(), req.FileName, createdBy, req.Components)
	if err != nil {
		response.InternalError(c, "failed to run risk assessment")
		return
	}
	h.service.EnrichSuggestions(c.Request.Context(), assessment)
	response.Success(c, assessment)
}

// AIParse 用 AI 兜底解析非标准格式的风险组件文档。
func (h *RiskAssessmentHandler) AIParse(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file", "请上传文件（字段名 file）")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.InternalError(c, "failed to open uploaded file")
		return
	}
	defer file.Close()

	components, err := h.service.AIParse(c.Request.Context(), file, fileHeader.Filename)
	if err != nil {
		response.BadRequest(c, "AI parse failed", err.Error())
		return
	}
	response.Success(c, gin.H{
		"file_name":  fileHeader.Filename,
		"components": components,
		"count":      len(components),
	})
}

// CreateBlockRules 为研判命中的组件一键生成阻断规则。
// item_ids 为空表示对全部命中项生成；但 body 必须为合法 JSON，避免字段名笔误被静默当作空列表全量阻断。
func (h *RiskAssessmentHandler) CreateBlockRules(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req struct {
		ItemIDs []uint `json:"item_ids"`
	}
	// DisallowUnknownFields：字段名笔误（如 itemIds）直接报错，避免被当作空列表对全部命中项生成阻断规则
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	count, err := h.service.CreateBlockRules(c.Request.Context(), id, req.ItemIDs)
	if err != nil {
		response.InternalError(c, "failed to create block rules")
		return
	}
	response.Success(c, gin.H{"message": "block rules created", "count": count})
}

// AIReport 生成研判结果的 AI 总结报告与处置建议。
func (h *RiskAssessmentHandler) AIReport(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	report, err := h.service.AIReport(c.Request.Context(), id)
	if err != nil {
		response.BadRequest(c, "AI report failed", err.Error())
		return
	}
	response.Success(c, gin.H{"report": report})
}

// Dispose 快速处置单个明细：移除制品（含自动阻断）/ 仅阻断 / 忽略 / 重置。
func (h *RiskAssessmentHandler) Dispose(c *gin.Context) {
	var req struct {
		ItemID uint   `json:"item_id" binding:"required"`
		Action string `json:"action" binding:"required,oneof=remove block ignore reset"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	item, err := h.service.DisposeItem(c.Request.Context(), service.DisposeInput{
		ItemID: req.ItemID,
		Action: req.Action,
		Note:   req.Note,
		UserID: c.GetUint("userID"),
	})
	if err != nil {
		response.BadRequest(c, "dispose failed", err.Error())
		return
	}
	response.Success(c, item)
}

// DisposeStats 处置进度统计。
func (h *RiskAssessmentHandler) DisposeStats(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	stats, err := h.service.AssessmentDisposeStats(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, "failed to load dispose stats")
		return
	}
	response.Success(c, stats)
}

// DownloadTemplate 下载研判模板 xlsx。
func (h *RiskAssessmentHandler) DownloadTemplate(c *gin.Context) {
	data, err := h.service.TemplateBytes()
	if err != nil {
		response.InternalError(c, "failed to generate template")
		return
	}
	filename := "risk-assessment-template.xlsx"
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// ListAssessments 分页列出研判历史。
func (h *RiskAssessmentHandler) ListAssessments(c *gin.Context) {
	page, pageSize := parsePagination(c)
	list, total, err := h.service.ListAssessments(c.Request.Context(), page, pageSize)
	if err != nil {
		response.InternalError(c, "failed to list risk assessments")
		return
	}
	response.SuccessWithPagination(c, list, page, pageSize, total)
}

// GetAssessment 获取研判记录详情（含明细）。
func (h *RiskAssessmentHandler) GetAssessment(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	assessment, err := h.service.GetAssessment(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "risk assessment not found")
		return
	}
	response.Success(c, assessment)
}

// ExportAssessment 导出研判结果为 xlsx。
func (h *RiskAssessmentHandler) ExportAssessment(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	data, err := h.service.ExportAssessment(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "risk assessment not found")
		return
	}

	filename := fmt.Sprintf("risk-assessment-%d-%s.xlsx", id, time.Now().Format("20060102"))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// DeleteAssessment 删除研判记录。
func (h *RiskAssessmentHandler) DeleteAssessment(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteAssessment(c.Request.Context(), id); err != nil {
		response.InternalError(c, "failed to delete risk assessment")
		return
	}
	response.Success(c, gin.H{"message": "risk assessment deleted", "id": id})
}

func (h *RiskAssessmentHandler) parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id", "id must be a positive integer")
		return 0, false
	}
	return uint(id), true
}
