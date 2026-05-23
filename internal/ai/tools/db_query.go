package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
)

// DBQueryTool 数据库查询工具
type DBQueryTool struct {
	BaseTool
}

// NewDBQueryTool 创建数据库查询工具
func NewDBQueryTool() *DBQueryTool {
	return &DBQueryTool{}
}

// Name 返回工具名称
func (t *DBQueryTool) Name() string {
	return "db_query"
}

// Description 返回工具描述
func (t *DBQueryTool) Description() string {
	return "查询仓库数据库（包信息、下载统计、安全扫描等）"
}

// Parameters 返回工具参数的 JSON Schema
func (t *DBQueryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query_type": {
				"type": "string",
				"description": "查询类型",
				"enum": ["package_stats", "download_stats", "security_summary", "repository_stats", "recent_packages"]
			},
			"package_name": {
				"type": "string",
				"description": "包名称"
			},
			"package_type": {
				"type": "string",
				"description": "包类型 (npm, maven, pypi, go, nuget, yum, apt, generic)"
			},
			"time_range": {
				"type": "string",
				"description": "时间范围 (1h, 24h, 7d, 30d, 90d)"
			},
			"limit": {
				"type": "integer",
				"description": "返回结果数量限制",
				"default": 20
			}
		},
		"required": ["query_type"]
	}`)
}

// Execute 执行工具并返回结果
func (t *DBQueryTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	queryType, ok := params["query_type"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: query_type")
	}

	packageName, _ := params["package_name"].(string)
	packageType, _ := params["package_type"].(string)
	timeRange, _ := params["time_range"].(string)
	limit := 20
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	db := t.Context().DB
	if db == nil {
		return "", fmt.Errorf("数据库连接未配置")
	}

	switch queryType {
	case "package_stats":
		return t.queryPackageStats(packageName, packageType, limit)
	case "download_stats":
		return t.queryDownloadStats(timeRange, limit)
	case "security_summary":
		return t.querySecuritySummary(packageName, packageType, limit)
	case "repository_stats":
		return t.queryRepositoryStats(limit)
	case "recent_packages":
		return t.queryRecentPackages(limit)
	default:
		return "", fmt.Errorf("不支持的查询类型: %s", queryType)
	}
}

// queryPackageStats 查询包统计信息
func (t *DBQueryTool) queryPackageStats(packageName, packageType string, limit int) (string, error) {
	db := t.Context().DB

	var packages []model.Component
	query := db.Model(&model.Component{})

	if packageName != "" {
		query = query.Where("name LIKE ?", "%"+packageName+"%")
	}
	if packageType != "" {
		query = query.Where("format = ?", packageType)
	}

	if err := query.Limit(limit).Find(&packages).Error; err != nil {
		return "", fmt.Errorf("查询包信息失败: %v", err)
	}

	if len(packages) == 0 {
		return "📦 未找到匹配的包", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 找到 %d 个包:\n\n", len(packages)))
	for i, pkg := range packages {
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s@%s)\n", i+1, pkg.Name, pkg.Format, pkg.Version))
		sb.WriteString(fmt.Sprintf("   - 显示名称: %s\n", pkg.DisplayName))
		sb.WriteString(fmt.Sprintf("   - 描述: %s\n", pkg.Description))
		sb.WriteString(fmt.Sprintf("   - 下载次数: %d\n", pkg.DownloadCount))
		if pkg.License != "" {
			sb.WriteString(fmt.Sprintf("   - 许可证: %s\n", pkg.License))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// queryDownloadStats 查询下载统计
func (t *DBQueryTool) queryDownloadStats(timeRange string, limit int) (string, error) {
	db := t.Context().DB

	// 注意: 当前数据模型不支持按时间过滤下载统计
	// 这里显示的是总下载量，时间范围仅用于显示

	var packages []model.Component
	if err := db.Where("download_count > 0").
		Order("download_count DESC").
		Limit(limit).
		Find(&packages).Error; err != nil {
		return "", fmt.Errorf("查询下载统计失败: %v", err)
	}

	if len(packages) == 0 {
		return "📊 暂无下载统计数据", nil
	}

	timeRangeDisplay := timeRange
	if timeRangeDisplay == "" {
		timeRangeDisplay = "7d"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 下载统计 (最近 %s):\n\n", timeRangeDisplay))
	for i, pkg := range packages {
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s@%s)\n", i+1, pkg.Name, pkg.Format, pkg.Version))
		sb.WriteString(fmt.Sprintf("   - 总下载次数: %d\n", pkg.DownloadCount))
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// querySecuritySummary 查询安全摘要
func (t *DBQueryTool) querySecuritySummary(packageName, packageType string, limit int) (string, error) {
	db := t.Context().DB

	var results []model.ScanResult
	query := db.Model(&model.ScanResult{}).
		Preload("Vulnerabilities").
		Where("scan_status = ?", model.ScanStatusCompleted)

	if packageName != "" {
		query = query.Joins("JOIN components ON components.id = scan_results.component_id").
			Where("components.name LIKE ?", "%"+packageName+"%")
	}

	if err := query.Order("scanned_at DESC").Limit(limit).Find(&results).Error; err != nil {
		return "", fmt.Errorf("查询安全摘要失败: %v", err)
	}

	if len(results) == 0 {
		return "🔒 暂无安全扫描数据", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔒 安全扫描摘要 (共 %d 条记录):\n\n", len(results)))

	totalCritical := 0
	totalHigh := 0
	totalMedium := 0
	totalLow := 0

	for _, result := range results {
		totalCritical += result.CriticalCount
		totalHigh += result.HighCount
		totalMedium += result.MediumCount
		totalLow += result.LowCount

		sb.WriteString(fmt.Sprintf("📦 组件ID %d:\n", result.ComponentID))
		sb.WriteString(fmt.Sprintf("   - 扫描时间: %s\n", result.ScannedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("   - 总漏洞数: %d\n", result.TotalVulnerabilities))
		sb.WriteString(fmt.Sprintf("   - 严重: %d, 高危: %d, 中危: %d, 低危: %d\n",
			result.CriticalCount, result.HighCount, result.MediumCount, result.LowCount))
		sb.WriteString("\n")
	}

	sb.WriteString("📈 汇总统计:\n")
	sb.WriteString(fmt.Sprintf("   - 总严重漏洞: %d\n", totalCritical))
	sb.WriteString(fmt.Sprintf("   - 总高危漏洞: %d\n", totalHigh))
	sb.WriteString(fmt.Sprintf("   - 总中危漏洞: %d\n", totalMedium))
	sb.WriteString(fmt.Sprintf("   - 总低危漏洞: %d\n", totalLow))

	return sb.String(), nil
}

// queryRepositoryStats 查询仓库统计
func (t *DBQueryTool) queryRepositoryStats(limit int) (string, error) {
	db := t.Context().DB

	var stats struct {
		TotalPackages  int64
		TotalVersions  int64
		TotalDownloads int64
		PackagesByType []struct {
			Type  string
			Count int64
		}
	}

	// 统计总数
	db.Model(&model.Component{}).Distinct("repository_id, format, namespace, name").Count(&stats.TotalPackages)
	db.Model(&model.Component{}).Count(&stats.TotalVersions)
	db.Model(&model.Component{}).Select("COALESCE(SUM(download_count),0)").Scan(&stats.TotalDownloads)

	db.Model(&model.Component{}).
		Select("format as type, count(*) as count").
		Group("format").
		Find(&stats.PackagesByType)

	var sb strings.Builder
	sb.WriteString("📊 仓库统计信息:\n\n")
	sb.WriteString(fmt.Sprintf("📦 总包数: %d\n", stats.TotalPackages))
	sb.WriteString(fmt.Sprintf("📋 总版本数: %d\n", stats.TotalVersions))
	sb.WriteString(fmt.Sprintf("⬇️  总下载次数: %d\n\n", stats.TotalDownloads))

	sb.WriteString("📈 按包类型统计:\n")
	for _, item := range stats.PackagesByType {
		sb.WriteString(fmt.Sprintf("   - %s: %d 个包\n", item.Type, item.Count))
	}

	return sb.String(), nil
}

// queryRecentPackages 查询最近的包
func (t *DBQueryTool) queryRecentPackages(limit int) (string, error) {
	db := t.Context().DB

	var packages []model.Component
	if err := db.Order("created_at DESC").
		Limit(limit).
		Find(&packages).Error; err != nil {
		return "", fmt.Errorf("查询最近的包失败: %v", err)
	}

	if len(packages) == 0 {
		return "📦 暂无包数据", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 最近添加的 %d 个包:\n\n", len(packages)))
	for i, pkg := range packages {
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s@%s)\n", i+1, pkg.Name, pkg.Format, pkg.Version))
		sb.WriteString(fmt.Sprintf("   - 创建时间: %s\n", pkg.CreatedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("   - 版本: %s\n", pkg.Version))
		sb.WriteString(fmt.Sprintf("   - 描述: %s\n\n", pkg.Description))
	}

	return sb.String(), nil
}
