package mcp

import (
	"context"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/model"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

func (s *MCPServer) handleListVulnerabilities(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	severity, _ := args["severity"].(string)
	limit := 50
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	var vulns []model.Vulnerability
	q := s.db.Model(&model.Vulnerability{})
	if severity != "" {
		q = q.Where("severity = ?", severity)
	}
	if err := q.Order("cvss_score DESC").Limit(limit).Find(&vulns).Error; err != nil {
		return errorResult("获取漏洞列表失败: %v", err), nil
	}
	return textResult(formatJSON(vulns)), nil
}

func (s *MCPServer) handleGetSecurityStats(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	var totalVulns, critical, high, medium, low int64
	s.db.Model(&model.Vulnerability{}).Count(&totalVulns)
	s.db.Model(&model.Vulnerability{}).Where("severity = ?", "critical").Count(&critical)
	s.db.Model(&model.Vulnerability{}).Where("severity = ?", "high").Count(&high)
	s.db.Model(&model.Vulnerability{}).Where("severity = ?", "medium").Count(&medium)
	s.db.Model(&model.Vulnerability{}).Where("severity = ?", "low").Count(&low)

	var totalScans, completed int64
	s.db.Model(&model.ScanResult{}).Count(&totalScans)
	s.db.Model(&model.ScanResult{}).Where("scan_status = ?", model.ScanStatusCompleted).Count(&completed)

	return textResult(formatJSON(map[string]interface{}{
		"vulnerabilities": map[string]int64{
			"total": totalVulns, "critical": critical, "high": high, "medium": medium, "low": low,
		},
		"scans": map[string]int64{"total": totalScans, "completed": completed},
	})), nil
}

func (s *MCPServer) handleTriggerScan(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if ok, err := s.checkPermission(ctx, "security", "write"); !ok {
		return errorResult("权限不足: %v", err), nil
	}

	artifactID := req.GetArguments()["artifact_id"].(string)
	scanResult := model.ScanResult{
		ComponentID: parseUint(artifactID),
		ScanStatus:  model.ScanStatusPending,
	}
	if err := s.db.Create(&scanResult).Error; err != nil {
		return errorResult("触发扫描失败: %v", err), nil
	}
	return textResult(fmt.Sprintf("✅ 安全扫描已触发，包版本 ID: %s，扫描记录 ID: %d", artifactID, scanResult.ID)), nil
}

func (s *MCPServer) handleListAuditLogs(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	username, _ := args["username"].(string)
	action, _ := args["action"].(string)
	limit := 50
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	var logs []model.AuditLog
	q := s.db.Model(&model.AuditLog{})
	if username != "" {
		q = q.Where("username = ?", username)
	}
	if action != "" {
		q = q.Where("action = ?", action)
	}
	if err := q.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return errorResult("查询审计日志失败: %v", err), nil
	}
	return textResult(formatJSON(logs)), nil
}

func (s *MCPServer) handleListDownloadLogs(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	packageName, _ := args["package_name"].(string)
	ipAddr, _ := args["remote_addr"].(string)
	limit := 50
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	var logs []model.DownloadLog
	q := s.db.Model(&model.DownloadLog{})
	if packageName != "" {
		q = q.Where("package_name = ?", packageName)
	}
	if ipAddr != "" {
		q = q.Where("ip_address = ?", ipAddr)
	}
	if err := q.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return errorResult("查询下载日志失败: %v", err), nil
	}
	return textResult(formatJSON(logs)), nil
}

func (s *MCPServer) handleGetDownloadStats(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	var totalDownloads, uniquePackages, uniqueIPs, recentDownloads int64
	s.db.Model(&model.DownloadLog{}).Count(&totalDownloads)
	s.db.Model(&model.DownloadLog{}).Distinct("package_name").Count(&uniquePackages)
	s.db.Model(&model.DownloadLog{}).Distinct("ip_address").Count(&uniqueIPs)
	s.db.Model(&model.DownloadLog{}).Where("created_at > datetime('now', '-1 day')").Count(&recentDownloads)

	return textResult(formatJSON(map[string]interface{}{
		"total_downloads": totalDownloads, "unique_packages": uniquePackages,
		"unique_ips": uniqueIPs, "last_24h": recentDownloads,
	})), nil
}

func (s *MCPServer) handleGetSystemInfo(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	var repoCount, artifactCount, userCount int64
	s.db.Model(&model.Repository{}).Count(&repoCount)
	s.db.Model(&model.Artifact{}).Count(&artifactCount)
	s.db.Model(&model.User{}).Count(&userCount)

	return textResult(formatJSON(map[string]interface{}{
		"repositories": repoCount, "artifacts": artifactCount, "users": userCount,
		"database": s.cfg.Database.Driver, "mcp_server": "embedded",
	})), nil
}

func (s *MCPServer) handleGetDashboardStats(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	var repoCount, artifactCount, userCount, vulnCount int64
	s.db.Model(&model.Repository{}).Count(&repoCount)
	s.db.Model(&model.Artifact{}).Count(&artifactCount)
	s.db.Model(&model.User{}).Count(&userCount)
	s.db.Model(&model.Vulnerability{}).Count(&vulnCount)

	return textResult(formatJSON(map[string]interface{}{
		"repositories": repoCount, "artifacts": artifactCount,
		"users": userCount, "vulnerabilities": vulnCount,
	})), nil
}

func (s *MCPServer) handleListHealthStatus(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	var repos []model.Repository
	if err := s.db.Where("type = ?", "proxy").Find(&repos).Error; err != nil {
		return errorResult("获取健康状态失败: %v", err), nil
	}
	type info struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
		URL  string `json:"remote_url"`
	}
	var result []info
	for _, r := range repos {
		url := ""
		if r.Config != nil {
			url = r.Config.RemoteURL
		}
		result = append(result, info{ID: r.ID, Name: r.Name, URL: url})
	}
	return textResult(formatJSON(result)), nil
}

func (s *MCPServer) handleResetCircuitBreaker(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	repoID := req.GetArguments()["repo_id"].(string)
	return textResult(fmt.Sprintf("✅ 仓库 %s 的熔断器重置请求已收到", repoID)), nil
}

func parseUint(s string) uint {
	var n uint
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + uint(c-'0')
		}
	}
	return n
}
