package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)

type SecurityHandler struct {
	securityScanner *service.SecurityScanner
}

func NewSecurityHandler(securityScanner *service.SecurityScanner) *SecurityHandler {
	return &SecurityHandler{
		securityScanner: securityScanner,
	}
}

func (h *SecurityHandler) GetScanResult(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	result, err := h.securityScanner.GetScanResult(uint(versionID))
	if err != nil {
		response.NotFound(c, "scan result not found")
		return
	}

	response.Success(c, result)
}

func (h *SecurityHandler) TriggerScan(c *gin.Context) {
	versionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid version ID", "version ID must be a positive integer")
		return
	}

	pkgType := c.Query("pkg_type")
	name := c.Query("name")
	version := c.Query("version")

	if pkgType == "" || name == "" || version == "" {
		response.BadRequest(c, "missing required parameters", "pkg_type, name, and version are required")
		return
	}

	h.securityScanner.TriggerScan(c.Request.Context(), uint(versionID), pkgType, name, version)

	response.Success(c, gin.H{
		"message":      "scan triggered",
		"component_id": versionID,
	})
}

func (h *SecurityHandler) ListVulnerabilities(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	severity := c.Query("severity")
	pkgType := c.Query("pkg_type")

	vulns, total, err := h.securityScanner.ListVulnerabilitiesPaginated(page, pageSize, severity, pkgType)
	if err != nil {
		response.InternalError(c, "failed to list vulnerabilities")
		return
	}

	var vulnList []map[string]interface{}
	for _, v := range vulns {
		vulnList = append(vulnList, map[string]interface{}{
			"id":              v.ID,
			"scan_result_id":  v.ScanResultID,
			"cve_id":          v.CVEID,
			"severity":        v.Severity,
			"cvss_score":      v.CVSSScore,
			"dependency_name": v.DependencyName,
			"current_version": v.CurrentVersion,
			"fixed_version":   v.FixedVersion,
			"is_direct_dep":   v.IsDirectDep,
			"title":           v.Title,
			"description":     v.Description,
			"references":      v.References,
			"created_at":      v.CreatedAt,
		})
	}

	response.SuccessWithPagination(c, vulnList, page, pageSize, total)
}

func (h *SecurityHandler) ListScanResults(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	pkgType := c.Query("pkg_type")

	results, total, err := h.securityScanner.ListScanResults(page, pageSize, status, pkgType)
	if err != nil {
		response.InternalError(c, "failed to list scan results")
		return
	}

	response.SuccessWithPagination(c, results, page, pageSize, total)
}

func (h *SecurityHandler) GetSecurityStats(c *gin.Context) {
	total, critical, high, medium, low, err := h.securityScanner.GetSecurityStats()
	if err != nil {
		response.InternalError(c, "failed to get security stats")
		return
	}

	response.Success(c, gin.H{
		"total_scans": total,
		"critical":    critical,
		"high":        high,
		"medium":      medium,
		"low":         low,
		"total_vulns": critical + high + medium + low,
	})
}

func (h *SecurityHandler) GetDashboard(c *gin.Context) {
	totalScans, critical, high, medium, low, err := h.securityScanner.GetSecurityStats()
	if err != nil {
		response.InternalError(c, "failed to get dashboard data")
		return
	}

	results, _, _ := h.securityScanner.ListScanResults(1, 10, "", "")

	// 批量查询所有 scan result 的 vulnerabilities，避免 N+1 查询
	recentVulns := make([]map[string]interface{}, 0)
	if len(results) > 0 {
		scanResultIDs := make([]uint, 0, len(results))
		for _, r := range results {
			scanResultIDs = append(scanResultIDs, r.ID)
		}

		allVulns, err := h.securityScanner.ListVulnerabilitiesByScanResultIDs(scanResultIDs)
		if err == nil {
			for _, v := range allVulns {
				recentVulns = append(recentVulns, map[string]interface{}{
					"cve_id":          v.CVEID,
					"severity":        v.Severity,
					"cvss_score":      v.CVSSScore,
					"title":           v.Title,
					"dependency_name": v.DependencyName,
					"fixed_version":   v.FixedVersion,
					"scan_result_id":  v.ScanResultID,
				})
				if len(recentVulns) >= 20 {
					break
				}
			}
		}
	}

	response.Success(c, gin.H{
		"stats": gin.H{
			"total_scans": totalScans,
			"critical":    critical,
			"high":        high,
			"medium":      medium,
			"low":         low,
			"total_vulns": critical + high + medium + low,
		},
		"recent_vulnerabilities": recentVulns,
	})
}

func (h *SecurityHandler) TriggerFullScan(c *gin.Context) {
	h.securityScanner.ScanAllPackages(c.Request.Context())
	response.Success(c, gin.H{"message": "full scan started"})
}

func (h *SecurityHandler) BlockByCVE(c *gin.Context) {
	cveID := c.Param("cve")
	if cveID == "" {
		response.BadRequest(c, "missing CVE ID", "CVE ID is required")
		return
	}

	err := h.securityScanner.BlockByVulnerability(c.Request.Context(), cveID)
	if err != nil {
		response.InternalError(c, "failed to create block rule")
		return
	}

	response.Success(c, gin.H{"message": "block rule created", "cve": cveID})
}

type securityHandlerKey struct{}

func WithSecurityHandler(handler *SecurityHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("securityHandler", handler)
		c.Next()
	}
}

func GetSecurityHandler(c *gin.Context) *SecurityHandler {
	if val, exists := c.Get("securityHandler"); exists {
		return val.(*SecurityHandler)
	}
	return nil
}

var _ context.Context

var _ = http.StatusOK
