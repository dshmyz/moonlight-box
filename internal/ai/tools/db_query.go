package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/model"
)

type DBQueryTool struct{ BaseTool }

func NewDBQueryTool() *DBQueryTool { return &DBQueryTool{} }

func (t *DBQueryTool) Name() string { return "db_query" }

func (t *DBQueryTool) Description() string {
	return "查询仓库数据库（包信息、下载统计、安全扫描等）"
}

func (t *DBQueryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query_type": {"type": "string", "enum": ["package_stats", "download_stats", "security_summary", "repository_stats", "recent_packages"]},
			"package_name": {"type": "string"},
			"package_type": {"type": "string"},
			"time_range": {"type": "string"},
			"limit": {"type": "integer", "default": 20}
		},
		"required": ["query_type"]
	}`)
}

func (t *DBQueryTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	queryType, _ := params["query_type"].(string)
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
		return t.querySecuritySummary(packageName, limit)
	case "repository_stats":
		return t.queryRepositoryStats()
	case "recent_packages":
		return t.queryRecentPackages(limit)
	default:
		return "", fmt.Errorf("不支持的查询类型: %s", queryType)
	}
}

func (t *DBQueryTool) queryPackageStats(packageName, packageType string, limit int) (string, error) {
	db := t.Context().DB
	var artifacts []model.Artifact
	q := db.Model(&model.Artifact{})
	if packageName != "" {
		q = q.Where("name = ?", packageName)
	}
	if packageType != "" {
		q = q.Where("format = ?", packageType)
	}
	if err := q.Limit(limit).Find(&artifacts).Error; err != nil {
		return "", fmt.Errorf("查询包信息失败: %v", err)
	}
	if len(artifacts) == 0 {
		return "📦 未找到匹配的包", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 找到 %d 个包:\n\n", len(artifacts)))
	for i, a := range artifacts {
		name := a.Name
		version := a.Version
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s@%s)\n", i+1, name, a.Format, version))
		sb.WriteString(fmt.Sprintf("   - ID: %d, Kind: %s\n", a.ID, a.Kind))
		sb.WriteString(fmt.Sprintf("   - Created: %s\n\n", a.CreatedAt.Format("2006-01-02 15:04:05")))
	}
	return sb.String(), nil
}

func (t *DBQueryTool) queryDownloadStats(timeRange string, limit int) (string, error) {
	db := t.Context().DB
	var artifacts []model.Artifact
	if err := db.Order("created_at DESC").Limit(limit).Find(&artifacts).Error; err != nil {
		return "", fmt.Errorf("查询下载统计失败: %v", err)
	}
	if len(artifacts) == 0 {
		return "📊 暂无统计数据", nil
	}
	d := timeRange
	if d == "" {
		d = "7d"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 最近制品 (显示 %s):\n\n", d))
	for i, a := range artifacts {
		name := a.Name
		version := a.Version
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s@%s) - %s\n", i+1, name, a.Format, version, a.CreatedAt.Format("2006-01-02")))
	}
	return sb.String(), nil
}

func (t *DBQueryTool) querySecuritySummary(packageName string, limit int) (string, error) {
	db := t.Context().DB
	var results []model.ScanResult
	q := db.Model(&model.ScanResult{}).Preload("Vulnerabilities").Where("scan_status = ?", model.ScanStatusCompleted)
	if packageName != "" {
		q = q.Where("package_name LIKE ?", "%"+packageName+"%")
	}
	if err := q.Order("scanned_at DESC").Limit(limit).Find(&results).Error; err != nil {
		return "", fmt.Errorf("查询安全摘要失败: %v", err)
	}
	if len(results) == 0 {
		return "🔒 暂无安全扫描数据", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔒 安全扫描摘要 (共 %d 条记录):\n\n", len(results)))
	tc, th, tm, tl := 0, 0, 0, 0
	for _, r := range results {
		tc += r.CriticalCount
		th += r.HighCount
		tm += r.MediumCount
		tl += r.LowCount
	}
	sb.WriteString(fmt.Sprintf("严重: %d, 高危: %d, 中危: %d, 低危: %d\n", tc, th, tm, tl))
	return sb.String(), nil
}

func (t *DBQueryTool) queryRepositoryStats() (string, error) {
	db := t.Context().DB
	var totalArtifacts int64
	db.Model(&model.Artifact{}).Count(&totalArtifacts)
	var totalRepos int64
	db.Model(&model.Repository{}).Count(&totalRepos)
	var formats []struct {
		Format string
		Count  int64
	}
	db.Model(&model.Artifact{}).Select("format, count(*) as count").Group("format").Find(&formats)

	var sb strings.Builder
	sb.WriteString("📊 仓库统计信息:\n\n")
	sb.WriteString(fmt.Sprintf("📦 总制品数: %d\n", totalArtifacts))
	sb.WriteString(fmt.Sprintf("📁 总仓库数: %d\n\n", totalRepos))
	sb.WriteString("📈 按包类型统计:\n")
	for _, f := range formats {
		sb.WriteString(fmt.Sprintf("   - %s: %d 个\n", f.Format, f.Count))
	}
	return sb.String(), nil
}

func (t *DBQueryTool) queryRecentPackages(limit int) (string, error) {
	db := t.Context().DB
	var artifacts []model.Artifact
	if err := db.Order("created_at DESC").Limit(limit).Find(&artifacts).Error; err != nil {
		return "", fmt.Errorf("查询最近的包失败: %v", err)
	}
	if len(artifacts) == 0 {
		return "📦 暂无包数据", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 最近添加的 %d 个包:\n\n", len(artifacts)))
	for i, a := range artifacts {
		name := a.Name
		version := a.Version
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s@%s)\n", i+1, name, a.Format, version))
		sb.WriteString(fmt.Sprintf("   - 创建时间: %s\n\n", a.CreatedAt.Format("2006-01-02 15:04:05")))
	}
	return sb.String(), nil
}
