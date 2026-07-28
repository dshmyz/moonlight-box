package http

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)

type VulnRuleHandler struct {
	vulnRuleService *service.VulnRuleService
}

func NewVulnRuleHandler(vulnRuleService *service.VulnRuleService) *VulnRuleHandler {
	return &VulnRuleHandler{
		vulnRuleService: vulnRuleService,
	}
}

func (h *VulnRuleHandler) ListRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	source := c.Query("source")
	severity := c.Query("severity")
	pkgType := c.Query("pkg_type")
	keyword := c.Query("keyword")

	rules, total, err := h.vulnRuleService.ListRules(page, pageSize, source, severity, pkgType, keyword)
	if err != nil {
		response.InternalError(c, "failed to list rules")
		return
	}

	response.SuccessWithPagination(c, rules, page, pageSize, total)
}

func (h *VulnRuleHandler) GetRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id", err.Error())
		return
	}
	rule, err := h.vulnRuleService.GetRule(uint(id))
	if err != nil {
		response.NotFound(c, "rule not found")
		return
	}
	response.Success(c, rule)
}

func (h *VulnRuleHandler) CreateRule(c *gin.Context) {
	var rule model.VulnRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}
	if err := h.vulnRuleService.CreateRule(&rule); err != nil {
		response.InternalError(c, "failed to create rule")
		return
	}
	response.Success(c, rule)
}

func (h *VulnRuleHandler) UpdateRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id", err.Error())
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}
	if err := h.vulnRuleService.UpdateRule(uint(id), updates); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "rule updated"})
}

func (h *VulnRuleHandler) DeleteRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id", err.Error())
		return
	}
	if err := h.vulnRuleService.DeleteRule(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "rule deleted"})
}

func (h *VulnRuleHandler) ImportRules(c *gin.Context) {
	var rules []model.VulnRule
	if err := c.ShouldBindJSON(&rules); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}
	count, err := h.vulnRuleService.ImportRules(rules)
	if err != nil {
		response.InternalError(c, "failed to import rules")
		return
	}
	response.Success(c, gin.H{"message": "rules imported", "count": count})
}

func (h *VulnRuleHandler) ListDataSources(c *gin.Context) {
	sources, err := h.vulnRuleService.ListDataSources()
	if err != nil {
		response.InternalError(c, "failed to list data sources")
		return
	}
	response.Success(c, sources)
}

func (h *VulnRuleHandler) CreateDataSource(c *gin.Context) {
	var ds model.VulnDataSource
	if err := c.ShouldBindJSON(&ds); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}
	if err := h.vulnRuleService.CreateDataSource(&ds); err != nil {
		response.InternalError(c, "failed to create data source")
		return
	}
	response.Success(c, ds)
}

func (h *VulnRuleHandler) UpdateDataSource(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id", err.Error())
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}
	if err := h.vulnRuleService.UpdateDataSource(uint(id), updates); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "data source updated"})
}

func (h *VulnRuleHandler) DeleteDataSource(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id", err.Error())
		return
	}
	if err := h.vulnRuleService.DeleteDataSource(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "data source deleted"})
}

func (h *VulnRuleHandler) SyncDataSource(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid id", err.Error())
		return
	}
	if err := h.vulnRuleService.SyncDataSource(c.Request.Context(), uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "sync completed"})
}

func (h *VulnRuleHandler) SyncAllDataSources(c *gin.Context) {
	if err := h.vulnRuleService.SyncAllDataSources(c.Request.Context()); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "sync completed"})
}

func (h *VulnRuleHandler) TestDataSource(c *gin.Context) {
	var ds model.VulnDataSource
	if err := c.ShouldBindJSON(&ds); err != nil {
		response.BadRequest(c, "invalid request", err.Error())
		return
	}

	// SSRF 防护：只允许 http/https，禁止内网地址
	if err := validateExternalURL(ds.URL); err != nil {
		response.BadRequest(c, "invalid URL", err.Error())
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", ds.URL, nil)
	if err != nil {
		response.BadRequest(c, "invalid URL", err.Error())
		return
	}

	client := newVulnDataSourceHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		response.BadRequest(c, "connection failed", err.Error())
		return
	}
	defer resp.Body.Close()

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		response.BadRequest(c, "invalid JSON response", err.Error())
		return
	}

	response.Success(c, gin.H{
		"status":     resp.StatusCode,
		"reachable":  true,
		"valid_json": true,
	})
}

func newVulnDataSourceHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// validateExternalURL 检查 URL 是否为合法的外部 http/https 地址，防止 SSRF
func validateExternalURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return &url.Error{Op: "validate", URL: rawURL, Err: net.InvalidAddrError("only http/https allowed")}
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return &url.Error{Op: "validate", URL: rawURL, Err: net.InvalidAddrError("localhost not allowed")}
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return &url.Error{Op: "validate", URL: rawURL, Err: net.InvalidAddrError("private/internal address not allowed")}
		}
	}
	return nil
}
