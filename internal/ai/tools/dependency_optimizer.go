package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type DependencyOptimizerTool struct{ BaseTool }

func NewDependencyOptimizerTool() *DependencyOptimizerTool { return &DependencyOptimizerTool{} }

func (t *DependencyOptimizerTool) Name() string { return "dependency_optimizer" }

func (t *DependencyOptimizerTool) Description() string {
	return "分析项目依赖关系，提供优化建议"
}

func (t *DependencyOptimizerTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project_name": {"type": "string"},
			"package_type": {"type": "string", "enum": ["npm", "maven", "pypi", "go", "generic"]},
			"analysis_scope": {"type": "string", "enum": ["conflicts", "security", "all"], "default": "all"},
			"min_severity": {"type": "string", "enum": ["low", "medium", "high", "critical"], "default": "medium"}
		},
		"required": ["project_name"]
	}`)
}

func (t *DependencyOptimizerTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	projectName, ok := params["project_name"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: project_name")
	}
	packageType, _ := params["package_type"].(string)
	analysisScope, _ := params["analysis_scope"].(string)
	if analysisScope == "" {
		analysisScope = "all"
	}
	minSeverity, _ := params["min_severity"].(string)
	if minSeverity == "" {
		minSeverity = "medium"
	}

	db := t.Context().DB
	if db == nil {
		return "", fmt.Errorf("数据库连接未配置")
	}

	// 查询匹配的 artifacts
	var artifacts []model.Artifact
	q := db.Model(&model.Artifact{}).Where("coordinates LIKE ?", fmt.Sprintf(`%%"name":"%s%%`, projectName))
	if packageType != "" {
		q = q.Where("format = ?", packageType)
	}
	if err := q.Limit(50).Find(&artifacts).Error; err != nil {
		return "", fmt.Errorf("查询项目包失败: %v", err)
	}

	if len(artifacts) == 0 {
		return fmt.Sprintf("⚠️ 未找到与 '%s' 相关的项目包", projectName), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **依赖优化分析报告** - %s\n\n", projectName))
	sb.WriteString(fmt.Sprintf("找到 %d 个相关制品\n", len(artifacts)))

	// 按类型分组统计
	formats := make(map[string]int)
	for _, a := range artifacts {
		formats[a.Format]++
	}
	sb.WriteString("\n📊 格式分布:\n")
	for f, c := range formats {
		sb.WriteString(fmt.Sprintf("   - %s: %d\n", f, c))
	}

	// 安全分析
	if analysisScope == "security" || analysisScope == "all" {
		sb.WriteString(t.analyzeSecurity(db, artifacts, minSeverity))
	}

	// 建议
	sb.WriteString("\n💡 **优化建议**\n")
	sb.WriteString("1️⃣  定期检查包版本更新，及时升级\n")
	sb.WriteString("2️⃣  配置自动安全扫描流水线\n")
	sb.WriteString("3️⃣  清理不再使用的旧版本\n")

	return sb.String(), nil
}

func (t *DependencyOptimizerTool) analyzeSecurity(db *gorm.DB, artifacts []model.Artifact, minSeverity string) string {
	var sb strings.Builder

	severityOrder := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}
	minLevel := severityOrder[minSeverity]

	vulnCount := 0
	for _, a := range artifacts {
		var scan model.ScanResult
		if err := db.Where("component_id = ?", a.ID).
			Preload("Vulnerabilities").
			First(&scan).Error; err != nil {
			continue
		}
		if scan.TotalVulnerabilities > 0 {
			hasRelevant := false
			if scan.CriticalCount > 0 && minLevel <= 4 {
				hasRelevant = true
			}
			if scan.HighCount > 0 && minLevel <= 3 {
				hasRelevant = true
			}
			if scan.MediumCount > 0 && minLevel <= 2 {
				hasRelevant = true
			}
			if hasRelevant {
				vulnCount++
				name := coordStr(a.Coordinates, "name")
				ver := coordStr(a.Coordinates, "version")
				sb.WriteString(fmt.Sprintf("🔒 `%s@%s`: 严重:%d 高危:%d 中危:%d\n",
					name, ver, scan.CriticalCount, scan.HighCount, scan.MediumCount))
			}
		}
	}

	if vulnCount == 0 {
		sb.WriteString("\n✅ **安全扫描**: 未发现高于 " + minSeverity + " 级别的漏洞\n")
	} else {
		sb.WriteString(fmt.Sprintf("\n⚠️ **发现 %d 个存在安全风险的制品**\n", vulnCount))
	}
	return sb.String()
}
