package http

import (
	"github.com/moonlight-box/registry/internal/response"
	"fmt"
	"net/url"
	"strconv"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

type BlockRuleHandler struct {
	svc       *service.BlockRuleService
	auditSvc  *service.AuditService
	auditRepo *repository.AuditRepository
}

func NewBlockRuleHandler(svc *service.BlockRuleService, auditSvc *service.AuditService, auditRepo *repository.AuditRepository) *BlockRuleHandler {
	return &BlockRuleHandler{svc: svc, auditSvc: auditSvc, auditRepo: auditRepo}
}

func (h *BlockRuleHandler) List(c *gin.Context) {
	filter := make(map[string]interface{})
	if pkgName := c.Query("package_name"); pkgName != "" {
		filter["package_name"] = "%" + pkgName + "%"
	}
	if pkgType := c.Query("package_type"); pkgType != "" {
		filter["package_type"] = pkgType
	}
	if enabled := c.Query("enabled"); enabled != "" {
		filter["enabled"] = enabled == "true"
	}

	if p := c.Query("page"); p != "" {
		page, _ := strconv.Atoi(p)
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if page < 1 {
			page = 1
		}
		rules, total, err := h.svc.ListWithPage(page, pageSize, filter)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		response.SuccessWithPagination(c, rules, page, pageSize, total)
	} else {
		rules, err := h.svc.List(filter)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		response.Success(c, rules)
	}
}

func (h *BlockRuleHandler) Create(c *gin.Context) {
	var req struct {
		PackageName string `json:"package_name" binding:"required"`
		Version     string `json:"version" binding:"required"`
		MatchType   string `json:"match_type" binding:"required"`
		PackageType string `json:"package_type" binding:"required"`
		Reason      string `json:"reason"`
		Enabled     *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}

	if req.MatchType != string(model.BlockMatchExact) && req.MatchType != string(model.BlockMatchWildcard) {
		response.BadRequest(c, "Invalid match_type", "must be 'exact' or 'wildcard'")
		return
	}

	userID, exists := c.Get("userID")
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rule := &model.BlockRule{
		PackageName: req.PackageName,
		Version:     req.Version,
		MatchType:   model.BlockMatchType(req.MatchType),
		PackageType: req.PackageType,
		Reason:      req.Reason,
		Enabled:     enabled,
	}
	if exists {
		uid := userID.(uint)
		rule.CreatedBy = &uid
	}

	if err := h.svc.Create(rule); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, rule)
}

func (h *BlockRuleHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "Invalid id", err.Error())
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}

	if err := h.svc.Update(uint(id), updates); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Block rule updated"})
}

func (h *BlockRuleHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "Invalid id", err.Error())
		return
	}

	if err := h.svc.Delete(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Block rule deleted"})
}

func (h *BlockRuleHandler) ListBlockLogs(c *gin.Context) {
	page := 1
	pageSize := 20
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(c.Query("page_size")); err == nil && ps > 0 && ps <= 100 {
		pageSize = ps
	}

	logs, total, err := h.auditSvc.List(page, pageSize, nil, string(model.ActionBlock))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithPagination(c, logs, page, pageSize, total)
}

func (h *BlockRuleHandler) BatchImport(c *gin.Context) {
	var req struct {
		Rules []struct {
			PackageName string `json:"package_name" binding:"required"`
			Version     string `json:"version" binding:"required"`
			MatchType   string `json:"match_type"`
			PackageType string `json:"package_type" binding:"required"`
			Reason      string `json:"reason"`
			Enabled     *bool  `json:"enabled"`
		} `json:"rules" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", err.Error())
		return
	}

	userID, exists := c.Get("userID")
	var rules []*model.BlockRule
	for _, r := range req.Rules {
		enabled := true
		if r.Enabled != nil {
			enabled = *r.Enabled
		}
		matchType := model.BlockMatchExact
		if r.MatchType != "" {
			matchType = model.BlockMatchType(r.MatchType)
		}
		rule := &model.BlockRule{
			PackageName: r.PackageName,
			Version:     r.Version,
			MatchType:   matchType,
			PackageType: r.PackageType,
			Reason:      r.Reason,
			Enabled:     enabled,
		}
		if exists {
			uid := userID.(uint)
			rule.CreatedBy = &uid
		}
		rules = append(rules, rule)
	}

	success, failed, err := h.svc.BatchCreate(rules)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"success": success,
		"failed":  failed,
		"total":   len(req.Rules),
	})
}

func (h *BlockRuleHandler) DownloadTemplate(c *gin.Context) {
	csvContent := "\xEF\xBB\xBF包名\t版本\t包类型\t匹配类型\t阻断原因\nlodash\t4.17.20\tnpm\texact\t存在安全漏洞\nfastjson\t1.*\tmaven\twildcard\t严重漏洞\n"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", url.QueryEscape("block_rule_template.csv")))
	c.Header("Content-Transfer-Encoding", "binary")
	c.String(200, csvContent)
}

func (h *BlockRuleHandler) GetBlockStats(c *gin.Context) {
	hours := 24
	if hStr := c.Query("hours"); hStr != "" {
		if hVal, err := strconv.Atoi(hStr); err == nil && hVal > 0 && hVal <= 720 {
			hours = hVal
		}
	}

	stats, err := h.auditRepo.GetBlockStats(hours)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, stats)
}
