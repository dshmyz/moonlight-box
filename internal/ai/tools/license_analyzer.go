package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

// LicenseCategory 许可证合规分类。
type LicenseCategory string

const (
	LicensePermissive LicenseCategory = "permissive" // 宽松许可（可安全使用）
	LicenseCopyleft   LicenseCategory = "copyleft"   // 传染性许可（GPL/AGPL 等，需合规审查）
	LicenseRestricted LicenseCategory = "restricted" // 限制性/商业许可
	LicenseUnknown    LicenseCategory = "unknown"    // 未知或缺失
)

// licenseRule 内置许可证分类规则（前缀匹配，大小写不敏感）。
var licenseRules = []struct {
	category LicenseCategory
	patterns []string
}{
	{LicensePermissive, []string{"mit", "apache", "bsd", "isc", "unlicense", "unlicense", "cc0", "zlib", "python-2.0", "mpl-2.0", "mozilla"}},
	{LicenseCopyleft, []string{"gpl", "agpl", "lgpl", "epl", "cc-by-sa", "sspl"}},
	{LicenseRestricted, []string{"commercial", "proprietary", "fair use", "no license", "all rights reserved"}},
}

// LicenseAnalyzerTool 许可合规分析。
//
// 按 license 字段对仓库内包分类（permissive/copyleft/restricted/unknown），
// 识别 copyleft 与未知许可包，结合已有阻断规则给出废弃/替代建议。只读分析。
type LicenseAnalyzerTool struct {
	BaseTool
}

func NewLicenseAnalyzerTool() *LicenseAnalyzerTool {
	return &LicenseAnalyzerTool{}
}

func (t *LicenseAnalyzerTool) Name() string {
	return "license_analyzer"
}

func (t *LicenseAnalyzerTool) Description() string {
	return "许可合规分析：按 license 对仓库包分类，识别 copyleft/未知许可风险，" +
		"结合阻断规则给出废弃或替代建议。只读分析。"
}

func (t *LicenseAnalyzerTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"analysis_type": {
				"type": "string",
				"description": "分析类型",
				"enum": ["overview", "by_license", "risky", "unknown", "blocked"]
			},
			"package_type": {
				"type": "string",
				"description": "限定包类型（npm/maven/pypi/go/yum/apt/generic），可空"
			},
			"limit": {
				"type": "integer",
				"description": "返回条目上限，默认 30",
				"default": 30
			}
		},
		"required": ["analysis_type"]
	}`)
}

func (t *LicenseAnalyzerTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	analysisType, ok := params["analysis_type"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: analysis_type")
	}
	packageType, _ := params["package_type"].(string)
	limit := 30
	if l, ok := params["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}

	toolCtx := t.Context()
	if toolCtx == nil || toolCtx.DB == nil {
		return "", fmt.Errorf("工具上下文未配置 DB")
	}
	db := toolCtx.DB

	var result string
	var err error
	switch analysisType {
	case "overview":
		result, err = t.analyzeOverview(db, packageType)
	case "by_license":
		result, err = t.analyzeByLicense(db, packageType, limit)
	case "risky":
		result, err = t.analyzeRisky(db, packageType, limit)
	case "unknown":
		result, err = t.analyzeUnknown(db, packageType, limit)
	case "blocked":
		result, err = t.analyzeBlockedLicenses(db, packageType, limit)
	default:
		return "", fmt.Errorf("不支持的分析类型: %s", analysisType)
	}
	if err != nil {
		return "", err
	}
	return result, nil
}

// licenseRow 聚合查询行。
type licenseRow struct {
	License string
	Count   int64
}

// classifyLicense 分类许可证。
func classifyLicense(license string) LicenseCategory {
	l := strings.ToLower(strings.TrimSpace(license))
	if l == "" {
		return LicenseUnknown
	}
	for _, rule := range licenseRules {
		for _, p := range rule.patterns {
			if strings.Contains(l, p) {
				return rule.category
			}
		}
	}
	// 有值但无法识别
	return LicenseUnknown
}

// queryLicenseAgg 查询许可证分布（按分类聚合）。
func queryLicenseAgg(db *gorm.DB, packageType string) (map[LicenseCategory]int64, []licenseRow, error) {
	query := db.Model(&model.Package{}).Select("license, COUNT(*) as count")
	if packageType != "" {
		query = query.Where("format = ?", packageType)
	}
	var rows []licenseRow
	if err := query.Group("license").Order("count DESC").Limit(500).Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	agg := map[LicenseCategory]int64{}
	for _, r := range rows {
		agg[classifyLicense(r.License)] += r.Count
	}
	return agg, rows, nil
}

func (t *LicenseAnalyzerTool) analyzeOverview(db *gorm.DB, packageType string) (string, error) {
	agg, rows, err := queryLicenseAgg(db, packageType)
	if err != nil {
		return "", fmt.Errorf("查询许可证分布失败: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("📜 许可合规概览\n\n")
	if packageType != "" {
		sb.WriteString(fmt.Sprintf("包类型: **%s**\n\n", packageType))
	}

	var total int64
	for _, v := range agg {
		total += v
	}
	sb.WriteString(fmt.Sprintf("🔢 已统计包数: **%d**\n\n", total))
	sb.WriteString(fmt.Sprintf("✅ 宽松许可: %d\n", agg[LicensePermissive]))
	sb.WriteString(fmt.Sprintf("⚠️ Copyleft许可: %d\n", agg[LicenseCopyleft]))
	sb.WriteString(fmt.Sprintf("🔒 限制性许可: %d\n", agg[LicenseRestricted]))
	sb.WriteString(fmt.Sprintf("❓ 未知/缺失: %d\n\n", agg[LicenseUnknown]))

	if agg[LicenseCopyleft] > 0 || agg[LicenseRestricted] > 0 {
		sb.WriteString("⚠️ 发现 copyleft/限制性许可包，建议法务或合规团队评估后再决定是否继续使用。\n")
	}
	if agg[LicenseUnknown] > 0 {
		sb.WriteString("⚠️ 存在未知许可包，建议补充 license 元数据（hosted 包可在发布时声明）。\n")
	}
	if agg[LicenseUnknown] == 0 && agg[LicenseCopyleft] == 0 && agg[LicenseRestricted] == 0 {
		sb.WriteString("✅ 全部包为宽松许可，合规状态良好。\n")
	}

	// Top 许可证
	if len(rows) > 0 {
		sb.WriteString("\n📊 Top 许可证:\n\n")
		for i, r := range rows {
			if i >= 10 {
				break
			}
			category := classifyLicense(r.License)
			icon := map[LicenseCategory]string{
				LicensePermissive: "✅", LicenseCopyleft: "⚠️",
				LicenseRestricted: "🔒", LicenseUnknown: "❓",
			}[category]
			display := r.License
			if display == "" {
				display = "(未声明)"
			}
			sb.WriteString(fmt.Sprintf("%s %s: %d 个包\n", icon, display, r.Count))
		}
	}
	return sb.String(), nil
}

func (t *LicenseAnalyzerTool) analyzeByLicense(db *gorm.DB, packageType string, limit int) (string, error) {
	query := db.Model(&model.Package{}).Select("license, COUNT(*) as count")
	if packageType != "" {
		query = query.Where("format = ?", packageType)
	}
	var rows []licenseRow
	if err := query.Group("license").Order("count DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("📋 按许可证统计\n\n")
	if len(rows) == 0 {
		sb.WriteString("📭 无数据\n")
		return sb.String(), nil
	}
	for _, r := range rows {
		cat := classifyLicense(r.License)
		icon := map[LicenseCategory]string{
			LicensePermissive: "✅", LicenseCopyleft: "⚠️",
			LicenseRestricted: "🔒", LicenseUnknown: "❓",
		}[cat]
		display := r.License
		if display == "" {
			display = "(未声明)"
		}
		sb.WriteString(fmt.Sprintf("%s **%s** (%s): %d 个包\n", icon, display, cat, r.Count))
	}
	return sb.String(), nil
}

// analyzeRisky 列出 copyleft/限制性许可的包。
func (t *LicenseAnalyzerTool) analyzeRisky(db *gorm.DB, packageType string, limit int) (string, error) {
	rows, err := listPackagesByCategory(db, packageType, limit, LicenseCopyleft, LicenseRestricted)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("⚠️ Copyleft / 限制性许可包\n\n")
	if len(rows) == 0 {
		sb.WriteString("✅ 未发现 copyleft/限制性许可包\n")
		return sb.String(), nil
	}
	for i, r := range rows {
		cat := classifyLicense(r.License)
		icon := map[LicenseCategory]string{LicenseCopyleft: "⚠️", LicenseRestricted: "🔒"}[cat]
		display := r.License
		if display == "" {
			display = "(未声明)"
		}
		sb.WriteString(fmt.Sprintf("%d. %s **%s** (%s) - %s\n", i+1, icon, r.PackageName, r.PackageType, display))
	}
	sb.WriteString("\n💡 处置建议: 评估使用场景后，可:\n")
	sb.WriteString("   - 对高风险包设置阻断规则（license 类型）\n")
	sb.WriteString("   - 在管理后台废弃相关版本\n")
	sb.WriteString("   - 寻找宽松许可的替代实现")
	return sb.String(), nil
}

// analyzeUnknown 列出未知/缺失许可的包。
func (t *LicenseAnalyzerTool) analyzeUnknown(db *gorm.DB, packageType string, limit int) (string, error) {
	rows, err := listPackagesByCategory(db, packageType, limit, LicenseUnknown)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("❓ 未知/缺失许可的包\n\n")
	if len(rows) == 0 {
		sb.WriteString("✅ 无未知许可包\n")
		return sb.String(), nil
	}
	for i, r := range rows {
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s) - %s\n", i+1, r.PackageName, r.PackageType, r.License))
	}
	sb.WriteString("\n💡 建议: 联系包维护者补充许可声明，或评估替换为许可明确的包。")
	return sb.String(), nil
}

// analyzeBlockedLicenses 结合阻断规则，查看已按 license 阻断的规则及命中包数。
func (t *LicenseAnalyzerTool) analyzeBlockedLicenses(db *gorm.DB, packageType string, limit int) (string, error) {
	var rules []model.BlockRule
	query := db.Where("enabled = ? AND condition_type = ?", true, model.ConditionTypeLicense)
	if err := query.Find(&rules).Error; err != nil {
		return "", fmt.Errorf("查询阻断规则失败: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("🚫 License 类型阻断规则\n\n")
	if len(rules) == 0 {
		sb.WriteString("📭 当前没有 license 类型的启用规则。\n")
		sb.WriteString("💡 可通过管理后台创建 license 阻断规则（condition_type=license）。")
		return sb.String(), nil
	}
	for i, r := range rules {
		pattern := r.PackageName
		if pattern == "*" || pattern == "" {
			pattern = "(全部包)"
		}
		sb.WriteString(fmt.Sprintf("%d. 规则#%d: %s 的 license %s %s %s - %s\n",
			i+1, r.ID, pattern, r.ConditionOp, r.ConditionValue, r.MatchType, r.Reason))
		if i >= limit {
			break
		}
	}
	return sb.String(), nil
}

// pkgLicenseRow 包级别查询行。
type pkgLicenseRow struct {
	PackageName string
	PackageType string
	License     string
}

// listPackagesByCategory 查询属于指定分类的包列表。
func listPackagesByCategory(db *gorm.DB, packageType string, limit int, categories ...LicenseCategory) ([]pkgLicenseRow, error) {
	var packages []model.Package
	query := db.Model(&model.Package{})
	if packageType != "" {
		query = query.Where("format = ?", packageType)
	}
	// 先取全量 license 再在内存分类（license 值有限，500 行内可控）
	if err := query.Limit(5000).Find(&packages).Error; err != nil {
		return nil, fmt.Errorf("查询包失败: %w", err)
	}
	catSet := make(map[LicenseCategory]bool)
	for _, c := range categories {
		catSet[c] = true
	}
	var rows []pkgLicenseRow
	for _, p := range packages {
		license := strings.TrimSpace(p.License)
		if catSet[classifyLicense(license)] {
			rows = append(rows, pkgLicenseRow{PackageName: p.Name, PackageType: p.Format, License: license})
			if len(rows) >= limit {
				break
			}
		}
	}
	return rows, nil
}
