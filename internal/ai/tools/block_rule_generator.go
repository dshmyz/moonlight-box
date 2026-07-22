package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/service"
)

// BlockRuleGeneratorTool AI 工具：根据漏洞数据或用户描述生成阻断规则草案（preview-only）。
//
// 设计原则：
//   - Preview-only：工具只返回规则草案 JSON，不写入 DB。用户需在管理后台确认后手动创建。
//   - 双输入源：source=vulnerability 从漏洞表生成；source=description 从用户对话解析后的结构化参数生成。
//   - 安全约束：拒绝生成 PackageName="*" + Version="*" 的全局阻断规则。
//   - 合法性校验：所有草案必须通过 BlockRuleService.ValidateRule 校验。
type BlockRuleGeneratorTool struct {
	BaseTool
	scanRepo *repository.ScanRepository
	blockSvc *service.BlockRuleService
}

// NewBlockRuleGeneratorTool 创建阻断规则生成工具。
// scanRepo 用于 source=vulnerability 时查询漏洞数据。
// blockSvc 用于校验规则合法性（ValidateRule）。
func NewBlockRuleGeneratorTool(scanRepo *repository.ScanRepository, blockSvc *service.BlockRuleService) *BlockRuleGeneratorTool {
	return &BlockRuleGeneratorTool{
		scanRepo: scanRepo,
		blockSvc: blockSvc,
	}
}

func (t *BlockRuleGeneratorTool) Name() string {
	return "block_rule_generator"
}

func (t *BlockRuleGeneratorTool) Description() string {
	return "根据漏洞数据或用户描述生成阻断规则草案（preview-only，不自动写入）。" +
		"当安全分析发现 critical/high 漏洞且 FixedVersion 存在时，或用户描述阻断需求时调用此工具。" +
		"返回规则草案 JSON，用户需在管理后台确认后手动创建。"
}

func (t *BlockRuleGeneratorTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"operation": {
				"type": "string",
				"description": "操作类型，目前只支持 preview（返回草案，不写入 DB）",
				"enum": ["preview"]
			},
			"source": {
				"type": "string",
				"description": "规则来源",
				"enum": ["vulnerability", "description"]
			},
			"cve_id": {
				"type": "string",
				"description": "单个 CVE 编号（source=vulnerability 时与 cve_ids 二选一）"
			},
			"cve_ids": {
				"type": "array",
				"items": {"type": "string"},
				"description": "批量 CVE 编号数组（source=vulnerability 时与 cve_id 二选一）。不存在的 CVE 会被静默跳过"
			},
			"package_name": {
				"type": "string",
				"description": "包名（source=description 时必填）"
			},
			"version": {
				"type": "string",
				"description": "版本约束（source=description 时必填）。range 类型用 semver 约束如 <2.17.1；wildcard 用 *；exact 用具体版本"
			},
			"match_type": {
				"type": "string",
				"description": "匹配类型（source=description 时必填）",
				"enum": ["exact", "wildcard", "range"]
			},
			"package_type": {
				"type": "string",
				"description": "包类型（npm/maven/pypi/go/yum/apt/generic/all），默认 *",
				"default": "*"
			},
			"reason": {
				"type": "string",
				"description": "阻断原因（可选）"
			}
		},
		"required": ["operation", "source"]
	}`)
}

// ruleDraft 是返回给 AI/用户的规则草案 JSON 结构。
type ruleDraft struct {
	PackageName      string   `json:"package_name"`
	Version          string   `json:"version"`
	MatchType        string   `json:"match_type"`
	PackageType      string   `json:"package_type"`
	Reason           string   `json:"reason"`
	Severity         string   `json:"severity,omitempty"`
	AffectedCount    int      `json:"affected_count"`
	AffectedVersions []string `json:"affected_versions"`
	DuplicateOfID   uint   `json:"duplicate_of_id,omitempty"`
	DuplicateOfDesc string `json:"duplicate_of_desc,omitempty"`
}

// generatorResponse 是工具返回的完整 JSON 结构。
type generatorResponse struct {
	Preview        bool        `json:"preview"`
	Rules          []ruleDraft `json:"rules"`
	ActionRequired string      `json:"action_required"`
}

// maxPreviewRules 限制单次返回的规则数量，防止 CVE 影响包过多时刷屏。
const maxPreviewRules = 20

func (t *BlockRuleGeneratorTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	operation, ok := params["operation"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: operation")
	}
	if operation != "preview" {
		return "", fmt.Errorf("不支持的操作类型: %s（目前只支持 preview）", operation)
	}

	source, ok := params["source"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: source")
	}

	var drafts []ruleDraft
	var err error
	switch source {
	case "vulnerability":
		drafts, err = t.generateFromVulnerability(params)
	case "description":
		drafts, err = t.generateFromDescription(params)
	default:
		return "", fmt.Errorf("不支持的 source: %s（支持 vulnerability 或 description）", source)
	}
	if err != nil {
		return "", err
	}

	if len(drafts) == 0 {
		return "", fmt.Errorf("未能生成任何阻断规则草案")
	}

	// 安全约束：拒绝全局阻断规则（PackageName="*" + Version="*"）
	for _, d := range drafts {
		if d.PackageName == "*" && d.Version == "*" {
			return "", fmt.Errorf("拒绝生成全局阻断规则（package_name=* + version=*），这会导致整个仓库不可用")
		}
	}

	// 限制返回数量
	if len(drafts) > maxPreviewRules {
		drafts = drafts[:maxPreviewRules]
	}

	// 合法性校验：所有草案必须通过 ValidateRule
	for i := range drafts {
		d := &drafts[i]
		rule := &model.BlockRule{
			PackageName: d.PackageName,
			Version:     d.Version,
			MatchType:   model.BlockMatchType(d.MatchType),
			PackageType: d.PackageType,
			Reason:      d.Reason,
			Enabled:     true,
		}
		if err := t.blockSvc.ValidateRule(rule); err != nil {
			return "", fmt.Errorf("规则草案校验失败 (package=%s, version=%s, match_type=%s): %w",
				d.PackageName, d.Version, d.MatchType, err)
		}
	}

	// 影响分析：查询 artifacts 表统计每条草案会影响多少已存在的包版本
	t.analyzeImpact(drafts)

	// 去重检测：查询 DB 中是否已有相同的启用规则，标记重复草案
	t.detectDuplicates(drafts)

	resp := generatorResponse{
		Preview:        true,
		Rules:          drafts,
		ActionRequired: "请在管理后台 Block Rules 页面确认以上规则草案并手动创建。工具不会自动写入数据库。",
	}

	result, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %w", err)
	}
	return string(result), nil
}

// generateFromVulnerability 从 vulnerability 表查询 CVE 数据生成规则草案。
// 支持两种入参：
//   - cve_ids（数组）：批量处理多个 CVE，不存在的 CVE 静默跳过，按包名跨 CVE 去重；
//   - cve_id（单个字符串）：处理单个 CVE，找不到时返回错误。
//
// 单个 CVE 内部按 DependencyName 去重（FindVulnerabilitiesByCVE 已按 cvss_score DESC 排序，同名取第一条）。
func (t *BlockRuleGeneratorTool) generateFromVulnerability(params map[string]interface{}) ([]ruleDraft, error) {
	// 优先处理批量模式
	if cveIDs, ok := extractCVEIDs(params); ok && len(cveIDs) > 0 {
		return t.generateFromMultipleCVEs(cveIDs)
	}

	// 兼容单个 cve_id
	cveID, ok := params["cve_id"].(string)
	if !ok || strings.TrimSpace(cveID) == "" {
		return nil, fmt.Errorf("source=vulnerability 时 cve_id 或 cve_ids 必填")
	}

	vulns, err := t.scanRepo.FindVulnerabilitiesByCVE(cveID)
	if err != nil {
		return nil, fmt.Errorf("查询漏洞数据失败: %w", err)
	}
	if len(vulns) == 0 {
		return nil, fmt.Errorf("未找到 CVE %s 的漏洞数据", cveID)
	}

	drafts := buildDraftsFromVulns(cveID, vulns, make(map[string]bool))
	if len(drafts) == 0 {
		return nil, fmt.Errorf("CVE %s 的漏洞数据中没有有效的 dependency_name", cveID)
	}
	return drafts, nil
}

// extractCVEIDs 从 params 中提取 cve_ids 字符串数组，返回 (ids, ok)。
// ok=false 表示未提供 cve_ids；ok=true 但 ids 可能为空（全部为空字符串时）。
func extractCVEIDs(params map[string]interface{}) ([]string, bool) {
	raw, ok := params["cve_ids"]
	if !ok {
		return nil, false
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, false
	}
	var ids []string
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		if v := strings.TrimSpace(s); v != "" {
			ids = append(ids, v)
		}
	}
	return ids, true
}

// generateFromMultipleCVEs 批量处理多个 CVE。
// 每个不存在的 CVE（查询出错或无数据）静默跳过；按包名跨 CVE 去重。
// 若所有 CVE 均无数据，返回错误。
func (t *BlockRuleGeneratorTool) generateFromMultipleCVEs(cveIDs []string) ([]ruleDraft, error) {
	seen := make(map[string]bool)
	var drafts []ruleDraft
	for _, cveID := range cveIDs {
		vulns, err := t.scanRepo.FindVulnerabilitiesByCVE(cveID)
		if err != nil {
			continue // 查询失败静默跳过
		}
		if len(vulns) == 0 {
			continue // 不存在的 CVE 静默跳过
		}
		drafts = append(drafts, buildDraftsFromVulns(cveID, vulns, seen)...)
	}
	if len(drafts) == 0 {
		return nil, fmt.Errorf("批量查询的所有 CVE 均未找到漏洞数据: %v", cveIDs)
	}
	return drafts, nil
}

// buildDraftsFromVulns 将单个 CVE 的漏洞记录转换为规则草案。
// seen 用于跨 CVE 去重：已处理过的包名不再生成（同一包只保留首次出现的 CVE 数据）。
func buildDraftsFromVulns(cveID string, vulns []model.Vulnerability, seen map[string]bool) []ruleDraft {
	var drafts []ruleDraft
	for i := range vulns {
		v := &vulns[i]
		depName := strings.TrimSpace(v.DependencyName)
		if depName == "" || seen[depName] {
			continue
		}
		seen[depName] = true

		draft := ruleDraft{
			PackageName: depName,
			PackageType: "*",
			Reason:      fmt.Sprintf("Auto-blocked for %s: %s", cveID, v.Title),
			Severity:    string(v.Severity),
		}

		if fixed := strings.TrimSpace(v.FixedVersion); fixed != "" {
			draft.MatchType = string(model.BlockMatchRange)
			draft.Version = fmt.Sprintf("<%s", fixed)
		} else {
			draft.MatchType = string(model.BlockMatchWildcard)
			draft.Version = "*"
		}

		drafts = append(drafts, draft)
	}
	return drafts
}

// generateFromDescription 从用户对话解析后的结构化参数生成规则草案。
// AI 在对话中已经解析出包名/版本约束/match_type，直接传入。
func (t *BlockRuleGeneratorTool) generateFromDescription(params map[string]interface{}) ([]ruleDraft, error) {
	packageName, ok := params["package_name"].(string)
	if !ok || strings.TrimSpace(packageName) == "" {
		return nil, fmt.Errorf("source=description 时 package_name 必填")
	}
	version, ok := params["version"].(string)
	if !ok || strings.TrimSpace(version) == "" {
		return nil, fmt.Errorf("source=description 时 version 必填")
	}
	matchType, ok := params["match_type"].(string)
	if !ok || strings.TrimSpace(matchType) == "" {
		return nil, fmt.Errorf("source=description 时 match_type 必填")
	}

	packageType := "*"
	if pt, ok := params["package_type"].(string); ok && strings.TrimSpace(pt) != "" {
		packageType = pt
	}
	reason := ""
	if r, ok := params["reason"].(string); ok {
		reason = r
	}

	draft := ruleDraft{
		PackageName: packageName,
		Version:     version,
		MatchType:   matchType,
		PackageType: packageType,
		Reason:      reason,
	}
	return []ruleDraft{draft}, nil
}

// analyzeImpact 查询 artifacts 表，为每条草案计算受影响的包版本列表。
// 对每条草案：
//  1. 查 name 匹配的 artifacts 的 distinct versions
//  2. 根据规则的 MatchType 过滤版本：
//     - range：用 semver 约束匹配
//     - wildcard with Version="*"：所有版本都匹配
//     - wildcard with Version 含 *：用正则匹配
//     - exact：精确匹配
//
// 如果 DB 不可用（ToolContext 未设置），静默跳过影响分析。
func (t *BlockRuleGeneratorTool) analyzeImpact(drafts []ruleDraft) {
	ctx := t.Context()
	if ctx == nil || ctx.DB == nil {
		return
	}
	db := ctx.DB

	for i := range drafts {
		d := &drafts[i]
		// 查询该包名的所有 distinct versions
		var versions []string
		if err := db.Model(&model.Artifact{}).
			Where("name = ?", d.PackageName).
			Distinct("version").
			Pluck("version", &versions).Error; err != nil {
			continue // 查询失败静默跳过，不影响草案生成
		}

		matched := matchVersionsByRule(versions, d.MatchType, d.Version)
		d.AffectedCount = len(matched)
		d.AffectedVersions = matched
	}
}

// matchVersionsByRule 根据规则的 MatchType 和 Version 约束，从候选版本中筛选匹配的版本。
// 这是一个包级函数，供 block_rule_generator 和 block_rule_optimizer 复用。
func matchVersionsByRule(candidates []string, matchType, versionConstraint string) []string {
	var result []string
	switch matchType {
	case string(model.BlockMatchRange):
		constraint, err := semver.NewConstraint(versionConstraint)
		if err != nil {
			return nil
		}
		for _, v := range candidates {
			ver, err := semver.NewVersion(v)
			if err != nil {
				continue // 非语义化版本跳过
			}
			if constraint.Check(ver) {
				result = append(result, v)
			}
		}

	case string(model.BlockMatchWildcard):
		if versionConstraint == "*" {
			// Version="*" 匹配所有版本
			result = append(result, candidates...)
		} else {
			// 把 wildcard 转成正则匹配版本
			pattern := strings.ReplaceAll(regexp.QuoteMeta(versionConstraint), "\\*", ".*")
			re, err := regexp.Compile("^" + pattern + "$")
			if err != nil {
				return nil
			}
			for _, v := range candidates {
				if re.MatchString(v) {
					result = append(result, v)
				}
			}
		}

	case string(model.BlockMatchExact):
		for _, v := range candidates {
			if v == versionConstraint {
				result = append(result, v)
				break
			}
		}
	}
	return result
}

// detectDuplicates 查询 DB 中是否已有相同的启用规则（PackageName + Version + MatchType 相同），
// 如果存在则在草案中标记 DuplicateOfID 和 DuplicateOfDesc。
// 如果 DB 不可用，静默跳过。
func (t *BlockRuleGeneratorTool) detectDuplicates(drafts []ruleDraft) {
	ctx := t.Context()
	if ctx == nil || ctx.DB == nil {
		return
	}
	db := ctx.DB

	for i := range drafts {
		d := &drafts[i]
		var existing model.BlockRule
		err := db.Where(
			"package_name = ? AND version = ? AND match_type = ? AND enabled = ?",
			d.PackageName, d.Version, d.MatchType, true,
		).First(&existing).Error
		if err != nil {
			continue // 没找到或查询失败，不标记重复
		}
		d.DuplicateOfID = existing.ID
		d.DuplicateOfDesc = fmt.Sprintf("已存在相同规则 (ID=%d): %s", existing.ID, existing.Reason)
	}
}
