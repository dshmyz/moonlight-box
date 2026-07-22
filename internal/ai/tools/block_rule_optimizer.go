package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

// BlockRuleOptimizerTool AI 工具：分析现有阻断规则集，输出优化建议（analyze-only）。
//
// 检测三类问题：
//   - over_broad：wildcard + Version="*" 的规则阻断某包所有版本，建议缩小范围；
//   - stale：规则未匹配到任何已存在的 artifact，可能已过期；
//   - redundant：同包同类型的规则被另一条 wildcard "*" 规则完全覆盖，建议删除。
//
// 工具只读分析，不修改任何规则。
type BlockRuleOptimizerTool struct {
	BaseTool
}

// NewBlockRuleOptimizerTool 创建阻断规则优化分析工具。
func NewBlockRuleOptimizerTool() *BlockRuleOptimizerTool {
	return &BlockRuleOptimizerTool{}
}

func (t *BlockRuleOptimizerTool) Name() string {
	return "block_rule_optimizer"
}

func (t *BlockRuleOptimizerTool) Description() string {
	return "分析现有阻断规则集，输出优化建议（over_broad/stale/redundant）。" +
		"当用户想审查或精简阻断规则时调用此工具。只读分析，不修改任何规则。"
}

func (t *BlockRuleOptimizerTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"operation": {
				"type": "string",
				"description": "操作类型，目前只支持 analyze",
				"enum": ["analyze"]
			}
		},
		"required": ["operation"]
	}`)
}

// optimizationSuggestion 是单条优化建议的 JSON 结构。
type optimizationSuggestion struct {
	Type        string `json:"type"` // over_broad | stale | redundant
	RuleID      uint   `json:"rule_id"`
	PackageName string `json:"package_name"`
	Version     string `json:"version"`
	MatchType   string `json:"match_type"`
	Detail      string `json:"detail"`
	Suggestion  string `json:"suggestion"`
}

// optimizerResponse 是工具返回的完整 JSON 结构。
type optimizerResponse struct {
	TotalRules  int                      `json:"total_rules"`
	Suggestions []optimizationSuggestion `json:"suggestions"`
}

func (t *BlockRuleOptimizerTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	operation, ok := params["operation"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: operation")
	}
	if operation != "analyze" {
		return "", fmt.Errorf("不支持的操作类型: %s（目前只支持 analyze）", operation)
	}

	toolCtx := t.Context()
	if toolCtx == nil || toolCtx.DB == nil {
		return "", fmt.Errorf("工具上下文未配置 DB，无法分析规则")
	}
	db := toolCtx.DB

	// 查询所有启用的规则
	var rules []model.BlockRule
	if err := db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return "", fmt.Errorf("查询阻断规则失败: %w", err)
	}

	var suggestions []optimizationSuggestion
	for i := range rules {
		rule := &rules[i]

		// over_broad: wildcard + Version="*" + 非全局包名
		if rule.MatchType == model.BlockMatchWildcard && rule.Version == "*" && rule.PackageName != "*" {
			suggestions = append(suggestions, optimizationSuggestion{
				Type:        "over_broad",
				RuleID:      rule.ID,
				PackageName: rule.PackageName,
				Version:     rule.Version,
				MatchType:   string(rule.MatchType),
				Detail:      fmt.Sprintf("规则阻断 %s 的所有版本（Version=*）", rule.PackageName),
				Suggestion:  "考虑缩小到具体版本范围（如 <x.y.z），避免误伤已修复版本",
			})
		}

		// stale: 规则未匹配到任何 artifact
		if isRuleStale(db, rule) {
			suggestions = append(suggestions, optimizationSuggestion{
				Type:        "stale",
				RuleID:      rule.ID,
				PackageName: rule.PackageName,
				Version:     rule.Version,
				MatchType:   string(rule.MatchType),
				Detail:      fmt.Sprintf("规则未匹配到任何已存在的 artifact（package=%s, version=%s）", rule.PackageName, rule.Version),
				Suggestion:  "该规则可能已过期，确认无相关包后可考虑删除",
			})
		}

		// redundant: 被同包同类型的另一条 wildcard "*" 规则覆盖
		if coveringID := findCoveringRule(db, rule); coveringID > 0 {
			suggestions = append(suggestions, optimizationSuggestion{
				Type:        "redundant",
				RuleID:      rule.ID,
				PackageName: rule.PackageName,
				Version:     rule.Version,
				MatchType:   string(rule.MatchType),
				Detail:      fmt.Sprintf("规则被 ID=%d 的 wildcard * 规则完全覆盖", coveringID),
				Suggestion:  "该规则已冗余，可考虑删除以精简规则集",
			})
		}
	}

	resp := optimizerResponse{
		TotalRules:  len(rules),
		Suggestions: suggestions,
	}
	result, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %w", err)
	}
	return string(result), nil
}

// isRuleStale 查询 artifacts 表，判断规则是否匹配到任何已存在的包版本。
// 查询失败时返回 false（保守不标记）。
func isRuleStale(db *gorm.DB, rule *model.BlockRule) bool {
	var versions []string
	if err := db.Model(&model.Artifact{}).
		Where("name = ?", rule.PackageName).
		Distinct("version").
		Pluck("version", &versions).Error; err != nil {
		return false
	}
	matched := matchVersionsByRule(versions, string(rule.MatchType), rule.Version)
	return len(matched) == 0
}

// findCoveringRule 查找是否存在同包同类型的 wildcard "*" 规则覆盖当前规则。
// 返回覆盖规则的 ID，不存在则返回 0。
// 覆盖条件：另一条规则 PackageName + PackageType 相同，MatchType=wildcard，Version="*"，且 ID 不同。
// wildcard "*" 规则自身不会被覆盖（除非完全相同，那是重复而非冗余覆盖）。
func findCoveringRule(db *gorm.DB, rule *model.BlockRule) uint {
	if rule.MatchType == model.BlockMatchWildcard && rule.Version == "*" {
		return 0
	}
	var covering model.BlockRule
	err := db.Where(
		"package_name = ? AND package_type = ? AND match_type = ? AND version = ? AND enabled = ? AND id != ?",
		rule.PackageName, rule.PackageType, model.BlockMatchWildcard, "*", true, rule.ID,
	).First(&covering).Error
	if err != nil {
		return 0
	}
	return covering.ID
}
