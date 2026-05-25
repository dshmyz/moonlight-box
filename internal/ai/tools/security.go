package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
)

// SecurityTool 安全分析工具
type SecurityTool struct {
	BaseTool
}

// NewSecurityTool 创建安全分析工具
func NewSecurityTool() *SecurityTool {
	return &SecurityTool{}
}

// Name 返回工具名称
func (t *SecurityTool) Name() string {
	return "security_analysis"
}

// Description 返回工具描述
func (t *SecurityTool) Description() string {
	return "分析包的安全问题、漏洞详情、修复建议"
}

// Parameters 返回工具参数的 JSON Schema
func (t *SecurityTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"analysis_type": {
				"type": "string",
				"description": "分析类型",
				"enum": ["package_scan", "vulnerability_detail", "security_report", "fix_recommendations"]
			},
			"package_name": {
				"type": "string",
				"description": "包名称"
			},
			"cve_id": {
				"type": "string",
				"description": "CVE编号"
			},
			"severity": {
				"type": "string",
				"description": "严重级别 (critical, high, medium, low)",
				"enum": ["critical", "high", "medium", "low"]
			}
		},
		"required": ["analysis_type"]
	}`)
}

// Execute 执行工具并返回结果
func (t *SecurityTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	analysisType, ok := params["analysis_type"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: analysis_type")
	}

	packageName, _ := params["package_name"].(string)
	cveID, _ := params["cve_id"].(string)
	severity, _ := params["severity"].(string)

	db := t.Context().DB
	if db == nil {
		return "", fmt.Errorf("数据库连接未配置")
	}

	switch analysisType {
	case "package_scan":
		return t.analyzePackageScan(packageName)
	case "vulnerability_detail":
		return t.getVulnerabilityDetail(cveID)
	case "security_report":
		return t.generateSecurityReport(severity)
	case "fix_recommendations":
		return t.getFixRecommendations(packageName, severity)
	default:
		return "", fmt.Errorf("不支持的分析类型: %s", analysisType)
	}
}

// analyzePackageScan 分析包的安全扫描结果
func (t *SecurityTool) analyzePackageScan(packageName string) (string, error) {
	db := t.Context().DB

	if packageName == "" {
		return "", fmt.Errorf("缺少包名称参数")
	}

	// 查询 artifacts 表中包名匹配的记录（从 coordinates JSONB 提取）
	var artifacts []model.Artifact
	namePattern := fmt.Sprintf(`%%"name":%%"%s"%%`, packageName)
	if err := db.Where("coordinates LIKE ?", namePattern).
		Order("created_at DESC").
		Find(&artifacts).Error; err != nil {
		return "", fmt.Errorf("查询版本信息失败: %v", err)
	}

	if len(artifacts) == 0 {
		return fmt.Sprintf("🔒 未找到包 **%s** 的相关记录", packageName), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔒 **%s** 安全扫描报告\n\n", packageName))

	totalVulns := 0
	criticalCount := 0
	highCount := 0
	mediumCount := 0
	lowCount := 0

	// 按 version 去重聚合
	type versionAgg struct {
		id      uint
		version string
	}
	seen := make(map[string]bool)
	var uniqueVersions []versionAgg

	for _, a := range artifacts {
		ver := coordinateStr(a.Coordinates, "version")
		if ver == "" {
			continue
		}
		if seen[ver] {
			continue
		}
		seen[ver] = true
		uniqueVersions = append(uniqueVersions, versionAgg{id: a.ID, version: ver})
	}

	for _, v := range uniqueVersions {
		var scanResult model.ScanResult
		if err := db.Where("component_id = ?", v.id).
			Preload("Vulnerabilities").
			First(&scanResult).Error; err != nil {
			continue
		}

		if scanResult.ScanStatus != model.ScanStatusCompleted {
			continue
		}

		totalVulns += scanResult.TotalVulnerabilities
		criticalCount += scanResult.CriticalCount
		highCount += scanResult.HighCount
		mediumCount += scanResult.MediumCount
		lowCount += scanResult.LowCount

		if scanResult.TotalVulnerabilities > 0 {
			sb.WriteString(fmt.Sprintf("📦 版本 **%s**:\n", v.version))
			sb.WriteString(fmt.Sprintf("   扫描时间: %s\n", scanResult.ScannedAt.Format("2006-01-02 15:04:05")))
			sb.WriteString(fmt.Sprintf("   漏洞数: %d (严重: %d, 高危: %d, 中危: %d, 低危: %d)\n\n",
				scanResult.TotalVulnerabilities, scanResult.CriticalCount,
				scanResult.HighCount, scanResult.MediumCount, scanResult.LowCount))

			// 显示前5个漏洞
			for i, vuln := range scanResult.Vulnerabilities {
				if i >= 5 {
					sb.WriteString(fmt.Sprintf("   ... 还有 %d 个漏洞\n\n", len(scanResult.Vulnerabilities)-5))
					break
				}
				sb.WriteString(fmt.Sprintf("   %d. **%s** [%s] (CVSS: %.1f)\n",
					i+1, vuln.CVEID, vuln.Severity, vuln.CVSSScore))
				sb.WriteString(fmt.Sprintf("      %s\n", vuln.Title))
				if vuln.FixedVersion != "" {
					sb.WriteString(fmt.Sprintf("      修复版本: %s\n", vuln.FixedVersion))
				}
			}
			sb.WriteString("\n")
		}
	}

	// 汇总统计
	sb.WriteString("📊 汇总统计:\n")
	sb.WriteString(fmt.Sprintf("   扫描版本数: %d\n", len(uniqueVersions)))
	sb.WriteString(fmt.Sprintf("   总漏洞数: %d\n", totalVulns))
	if totalVulns > 0 {
		sb.WriteString(fmt.Sprintf("   严重: %d, 高危: %d, 中危: %d, 低危: %d\n",
			criticalCount, highCount, mediumCount, lowCount))

		// 风险评估
		if criticalCount > 0 {
			sb.WriteString("\n⚠️  风险评估: **极高风险** - 发现严重漏洞，建议立即修复\n")
		} else if highCount > 0 {
			sb.WriteString("\n⚠️  风险评估: **高风险** - 存在高危漏洞，建议尽快修复\n")
		} else if mediumCount > 0 {
			sb.WriteString("\n⚠️  风险评估: **中等风险** - 存在中危漏洞，建议安排修复\n")
		} else {
			sb.WriteString("\n✅ 风险评估: **低风险** - 仅存在低危漏洞\n")
		}
	} else {
		sb.WriteString("\n✅ 未发现已知漏洞\n")
	}

	return sb.String(), nil
}

// getVulnerabilityDetail 获取漏洞详情
func (t *SecurityTool) getVulnerabilityDetail(cveID string) (string, error) {
	db := t.Context().DB

	if cveID == "" {
		return "", fmt.Errorf("缺少CVE编号参数")
	}

	var vulnerabilities []model.Vulnerability
	if err := db.Where("cve_id = ?", cveID).
		Preload("ScanResult").
		Find(&vulnerabilities).Error; err != nil {
		return "", fmt.Errorf("查询漏洞详情失败: %v", err)
	}

	if len(vulnerabilities) == 0 {
		return fmt.Sprintf("🔍 未找到CVE: %s", cveID), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **%s** 漏洞详情\n\n", cveID))

	vuln := vulnerabilities[0]
	sb.WriteString(fmt.Sprintf("📊 严重级别: **%s** (CVSS: %.1f)\n", vuln.Severity, vuln.CVSSScore))
	sb.WriteString(fmt.Sprintf("📝 标题: %s\n\n", vuln.Title))

	if vuln.Description != "" {
		sb.WriteString("📄 描述:\n")
		sb.WriteString(fmt.Sprintf("   %s\n\n", vuln.Description))
	}

	if vuln.DependencyName != "" {
		sb.WriteString("📦 受影响组件:\n")
		sb.WriteString(fmt.Sprintf("   依赖名称: %s\n", vuln.DependencyName))
		sb.WriteString(fmt.Sprintf("   当前版本: %s\n", vuln.CurrentVersion))
		if vuln.FixedVersion != "" {
			sb.WriteString(fmt.Sprintf("   修复版本: %s\n", vuln.FixedVersion))
		}
		sb.WriteString(fmt.Sprintf("   直接依赖: %v\n\n", vuln.IsDirectDep))
	}

	if vuln.References != "" {
		sb.WriteString("🔗 参考链接:\n")
		refs := strings.Split(vuln.References, "\n")
		for i, ref := range refs {
			if i >= 5 {
				sb.WriteString(fmt.Sprintf("   ... 还有 %d 个链接\n", len(refs)-5))
				break
			}
			sb.WriteString(fmt.Sprintf("   %d. %s\n", i+1, strings.TrimSpace(ref)))
		}
		sb.WriteString("\n")
	}

	// 修复建议
	sb.WriteString("💡 修复建议:\n")
	if vuln.FixedVersion != "" {
		sb.WriteString(fmt.Sprintf("   1. 升级到版本 %s 或更高版本\n", vuln.FixedVersion))
	} else {
		sb.WriteString("   1. 目前没有可用的修复版本，建议关注官方更新\n")
	}
	sb.WriteString("   2. 评估漏洞对您应用的实际影响\n")
	sb.WriteString("   3. 考虑使用替代方案或添加缓解措施\n")

	return sb.String(), nil
}

// generateSecurityReport 生成安全报告
func (t *SecurityTool) generateSecurityReport(severity string) (string, error) {
	db := t.Context().DB

	query := db.Model(&model.Vulnerability{}).
		Preload("ScanResult")

	if severity != "" {
		query = query.Where("severity = ?", model.VulnerabilitySeverity(severity))
	}

	var vulnerabilities []model.Vulnerability
	if err := query.Order("cvss_score DESC").Limit(50).Find(&vulnerabilities).Error; err != nil {
		return "", fmt.Errorf("生成安全报告失败: %v", err)
	}

	if len(vulnerabilities) == 0 {
		return "📊 暂无安全漏洞数据", nil
	}

	var sb strings.Builder
	if severity != "" {
		sb.WriteString(fmt.Sprintf("🔒 安全报告 - %s 级别漏洞\n\n", strings.Title(severity)))
	} else {
		sb.WriteString("🔒 安全报告 - 所有漏洞\n\n")
	}

	// 按严重级别分组统计
	severityCount := make(map[string]int)
	for _, vuln := range vulnerabilities {
		severityCount[string(vuln.Severity)]++
	}

	sb.WriteString("📊 统计信息:\n")
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		if count, ok := severityCount[sev]; ok {
			emoji := t.getSeverityEmoji(model.VulnerabilitySeverity(sev))
			sb.WriteString(fmt.Sprintf("   %s %s: %d 个\n", emoji, strings.Title(sev), count))
		}
	}
	sb.WriteString("\n")

	// 显示漏洞列表
	sb.WriteString("📋 漏洞列表:\n\n")
	for i, vuln := range vulnerabilities {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("   ... 还有 %d 个漏洞\n", len(vulnerabilities)-20))
			break
		}

		emoji := t.getSeverityEmoji(vuln.Severity)
		sb.WriteString(fmt.Sprintf("%d. %s **%s** [%s] (CVSS: %.1f)\n",
			i+1, emoji, vuln.CVEID, vuln.Severity, vuln.CVSSScore))
		sb.WriteString(fmt.Sprintf("   依赖: %s@%s\n", vuln.DependencyName, vuln.CurrentVersion))
		if vuln.FixedVersion != "" {
			sb.WriteString(fmt.Sprintf("   修复: 升级到 %s\n", vuln.FixedVersion))
		}
		sb.WriteString(fmt.Sprintf("   %s\n\n", vuln.Title))
	}

	return sb.String(), nil
}

// getFixRecommendations 获取修复建议
func (t *SecurityTool) getFixRecommendations(packageName string, severity string) (string, error) {
	db := t.Context().DB

	query := db.Model(&model.Vulnerability{}).
		Where("fixed_version != ?", "").
		Order("cvss_score DESC")

	if packageName != "" {
		query = query.Where("dependency_name LIKE ?", "%"+packageName+"%")
	}

	if severity != "" {
		query = query.Where("severity = ?", model.VulnerabilitySeverity(severity))
	}

	var vulnerabilities []model.Vulnerability
	if err := query.Limit(30).Find(&vulnerabilities).Error; err != nil {
		return "", fmt.Errorf("获取修复建议失败: %v", err)
	}

	if len(vulnerabilities) == 0 {
		return "✅ 未找到需要修复的漏洞", nil
	}

	var sb strings.Builder
	sb.WriteString("💡 修复建议\n\n")

	// 按依赖分组
	deps := make(map[string][]model.Vulnerability)
	for _, vuln := range vulnerabilities {
		deps[vuln.DependencyName] = append(deps[vuln.DependencyName], vuln)
	}

	sb.WriteString("📦 按依赖分组:\n\n")
	i := 1
	for depName, vulns := range deps {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i, depName))

		// 找出最高修复版本
		latestFix := ""
		for _, vuln := range vulns {
			if vuln.FixedVersion != "" {
				if latestFix == "" || vuln.FixedVersion > latestFix {
					latestFix = vuln.FixedVersion
				}
			}
		}

		if latestFix != "" {
			sb.WriteString(fmt.Sprintf("   建议升级到: %s\n", latestFix))
		}

		sb.WriteString(fmt.Sprintf("   涉及漏洞: %d 个\n", len(vulns)))
		for j, vuln := range vulns {
			if j >= 3 {
				sb.WriteString(fmt.Sprintf("   ... 还有 %d 个漏洞\n", len(vulns)-3))
				break
			}
			emoji := t.getSeverityEmoji(vuln.Severity)
			sb.WriteString(fmt.Sprintf("   - %s %s (CVSS: %.1f)\n", emoji, vuln.CVEID, vuln.CVSSScore))
		}
		sb.WriteString("\n")
		i++
		if i > 10 {
			sb.WriteString(fmt.Sprintf("   ... 还有 %d 个依赖\n", len(deps)-10))
			break
		}
	}

	sb.WriteString("📝 修复步骤:\n")
	sb.WriteString("   1. 检查依赖的兼容性要求\n")
	sb.WriteString("   2. 在测试环境中验证修复版本\n")
	sb.WriteString("   3. 运行完整的测试套件\n")
	sb.WriteString("   4. 部署到生产环境\n")
	sb.WriteString("   5. 重新扫描确认漏洞已修复\n")

	return sb.String(), nil
}

// getSeverityEmoji 获取严重级别对应的emoji
func (t *SecurityTool) getSeverityEmoji(severity model.VulnerabilitySeverity) string {
	switch severity {
	case model.SeverityCritical:
		return "🔴"
	case model.SeverityHigh:
		return "🟠"
	case model.SeverityMedium:
		return "🟡"
	case model.SeverityLow:
		return "🟢"
	default:
		return "⚪"
	}
}

func coordinateStr(coords model.JSONB, key string) string {
	if coords == nil {
		return ""
	}
	v, ok := coords[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}
