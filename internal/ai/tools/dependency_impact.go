package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/database/dialect"
	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

// DependencyImpactTool 供应链影响面分析（blast radius）。
//
// 给定一个目标包（被阻断 / 含漏洞 / 被废弃），找出仓库中所有直接依赖它的包：
//  1. 通过 vulnerabilities.dependency_name 反查依赖方（所有格式）；
//  2. 通过 npm artifacts.attributes.dependencies/devDependencies 的 JSON 反查；
//  3. 结合阻断规则判断目标包当前的阻断状态。
//
// 只读分析，不修改任何数据。
type DependencyImpactTool struct {
	BaseTool
}

func NewDependencyImpactTool() *DependencyImpactTool {
	return &DependencyImpactTool{}
}

func (t *DependencyImpactTool) Name() string {
	return "dependency_impact"
}

func (t *DependencyImpactTool) Description() string {
	return "供应链影响面分析：找出所有直接依赖某个包（被阻断/含漏洞/被废弃）的上游项目。" +
		"当阻断规则涉及某个包、或安全分析发现漏洞时，调用此工具评估影响范围。只读分析。"
}

func (t *DependencyImpactTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"package_name": {
				"type": "string",
				"description": "目标包名（被依赖的包，如 log4j）"
			},
			"package_type": {
				"type": "string",
				"description": "目标包类型（npm/maven/pypi/go/generic），可空"
			},
			"limit": {
				"type": "integer",
				"description": "返回的依赖方数量上限，默认 50",
				"default": 50
			}
		},
		"required": ["package_name"]
	}`)
}

// dependentInfo 单个依赖方信息。
type dependentInfo struct {
	PackageName string   `json:"package_name"`
	PackageType string   `json:"package_type"`
	Versions    []string `json:"versions"`
	Dependency  string   `json:"dependency,omitempty"` // npm 依赖条目（dependencies/devDependencies）
	VulnCount   int      `json:"vuln_count,omitempty"` // 自身已发现漏洞数
	Repos       []string `json:"repositories,omitempty"`
}

// impactResponse 影响面分析结果。
type impactResponse struct {
	Target             string          `json:"target"`
	TargetType         string          `json:"target_type,omitempty"`
	Blocked            bool            `json:"blocked"`
	BlockingRules      []string        `json:"blocking_rules,omitempty"`
	TotalDependents    int             `json:"total_dependents"`
	TotalVulnDep       int             `json:"total_vulnerable_dependents"`
	DirectDependents   []dependentInfo `json:"direct_dependents"`
	DataSources        []string        `json:"data_sources"`
	RecommendationNote string          `json:"recommendation_note"`
}

func (t *DependencyImpactTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	target, ok := params["package_name"].(string)
	if !ok || strings.TrimSpace(target) == "" {
		return "", fmt.Errorf("缺少必需参数: package_name")
	}
	target = strings.TrimSpace(target)
	packageType, _ := params["package_type"].(string)
	limit := 50
	if l, ok := params["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}

	toolCtx := t.Context()
	if toolCtx == nil || toolCtx.DB == nil {
		return "", fmt.Errorf("工具上下文未配置 DB")
	}
	db := toolCtx.DB

	resp := impactResponse{
		Target:     target,
		TargetType: packageType,
	}

	// 1. 阻断状态
	resp.Blocked, resp.BlockingRules = checkBlockedTarget(db, target, packageType)

	// 2. 通过漏洞表反查依赖方
	depsFromVulns := findDependentsFromVulnerabilities(db, target)
	// 3. 通过 npm metadata 反查依赖方
	depsFromMetadata := findDependentsFromMetadata(db, target)

	resp.DirectDependents = mergeDependents(depsFromVulns, depsFromMetadata)
	resp.TotalDependents = len(resp.DirectDependents)
	if resp.TotalDependents > limit {
		resp.DirectDependents = resp.DirectDependents[:limit]
		resp.TotalDependents = limit
	}

	vulnDep := 0
	for _, d := range resp.DirectDependents {
		if d.VulnCount > 0 {
			vulnDep++
		}
	}
	resp.TotalVulnDep = vulnDep
	resp.DataSources = []string{"vulnerabilities.dependency_name", "artifacts.attributes.dependencies/devDependencies"}

	if resp.Blocked {
		resp.RecommendationNote = fmt.Sprintf(
			"目标包 %s 当前被阻断。阻断前请确认 %d 个直接依赖方已升级或替换；建议先发布升级通告再执行阻断。",
			target, resp.TotalDependents)
	} else {
		resp.RecommendationNote = fmt.Sprintf(
			"目标包未被阻断。若阻断 %s，将影响 %d 个直接依赖方。",
			target, resp.TotalDependents)
	}

	result, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %w", err)
	}
	return string(result), nil
}

// checkBlockedTarget 查询目标包是否命中启用中的阻断规则。
// 匹配规则：PackageName 为目标包名或 "*"，且 PackageType 为目标类型或 "*"。
// 命中即报告（含 match_type/version 描述），由管理员结合影响面决策。
func checkBlockedTarget(db *gorm.DB, target, packageType string) (bool, []string) {
	query := db.Model(&model.BlockRule{}).
		Where("enabled = ?", true).
		Where("package_name = ? OR package_name = ?", target, "*")
	if packageType != "" {
		query = query.Where("(package_type = ? OR package_type = ?)", packageType, "*")
	}
	var rules []model.BlockRule
	if err := query.Limit(20).Find(&rules).Error; err != nil || len(rules) == 0 {
		return false, nil
	}
	var descs []string
	for _, r := range rules {
		desc := fmt.Sprintf("规则#%d: %s %s %s", r.ID, r.PackageName, r.MatchType, r.Version)
		if r.Reason != "" {
			desc += " (" + r.Reason + ")"
		}
		if r.ConditionType != "" {
			desc += fmt.Sprintf(" [condition: %s %s %s]", r.ConditionType, r.ConditionOp, r.ConditionValue)
		}
		descs = append(descs, desc)
	}
	return true, descs
}

// findDependentsFromVulnerabilities 通过漏洞表反查：某包作为 dependency_name 出现时，
// 其所属 ScanResult 的组件（artifact）即为依赖方。
func findDependentsFromVulnerabilities(db *gorm.DB, target string) map[string]*dependentInfo {
	result := make(map[string]*dependentInfo)

	var vulns []struct {
		ScanResultID   uint
		DependencyName string
	}
	db.Model(&model.Vulnerability{}).
		Where("dependency_name = ?", target).
		Distinct("scan_result_id").
		Select("scan_result_id").
		Scan(&vulns)

	var components []struct {
		ID       uint
		Name     string
		Format   string
		Version  string
		RepoID   uint
		RepoName string
	}
	if len(vulns) > 0 {
		scanIDs := make([]uint, 0, len(vulns))
		for _, v := range vulns {
			scanIDs = append(scanIDs, v.ScanResultID)
		}
		db.Table("artifacts AS a").
			Joins("LEFT JOIN repositories AS r ON r.id = a.repository_id").
			Select("a.id AS id, a.name AS name, a.format AS format, a.version AS version, a.repository_id AS repo_id, r.name AS repo_name").
			Where("a.id IN (SELECT component_id FROM scan_results WHERE id IN ?)", scanIDs).
			Scan(&components)
	}

	for _, c := range components {
		key := c.Format + ":" + c.Name
		d, ok := result[key]
		if !ok {
			d = &dependentInfo{PackageName: c.Name, PackageType: c.Format}
			result[key] = d
		}
		if !containsStr(d.Versions, c.Version) {
			d.Versions = append(d.Versions, c.Version)
		}
		if c.RepoName != "" && !containsStr(d.Repos, c.RepoName) {
			d.Repos = append(d.Repos, c.RepoName)
		}
		d.Dependency = "scan_result"
		// 统计依赖方自身漏洞数
		var cnt int64
		db.Table("scan_results AS s").
			Joins("JOIN vulnerabilities AS v ON v.scan_result_id = s.id").
			Where("s.component_id = ?", c.ID).
			Count(&cnt)
		d.VulnCount += int(cnt)
	}
	return result
}

// findDependentsFromMetadata 通过 npm metadata 反查：attributes.dependencies/devDependencies
// 中包含目标包的 artifact 即为依赖方。
func findDependentsFromMetadata(db *gorm.DB, target string) map[string]*dependentInfo {
	result := make(map[string]*dependentInfo)

	depExpr := dialect.JSONTextExpr(db.Dialector.Name(), "attributes", "dependencies")
	devExpr := dialect.JSONTextExpr(db.Dialector.Name(), "attributes", "devDependencies")

	var artifacts []struct {
		ID         uint
		Name       string
		Format     string
		Version    string
		Attributes string
		RepoName   string
	}
	like := "%\"" + target + "\"%"
	err := db.Table("artifacts AS a").
		Joins("LEFT JOIN repositories AS r ON r.id = a.repository_id").
		Select("a.id AS id, a.name AS name, a.format AS format, a.version AS version, a.attributes AS attributes, r.name AS repo_name").
		Where("(? LIKE ? OR ? LIKE ?)",
			gorm.Expr(depExpr), like, gorm.Expr(devExpr), like).
		Limit(500).
		Scan(&artifacts).Error
	if err != nil {
		return result
	}

	for _, a := range artifacts {
		// Go 侧二次精确校验（LIKE 可能误匹配子串）
		var attrs map[string]string
		if err := json.Unmarshal([]byte(a.Attributes), &attrs); err != nil {
			continue
		}
		depKind, hit := "", false
		for _, kind := range []string{"dependencies", "devDependencies"} {
			raw, ok := attrs[kind]
			if !ok {
				continue
			}
			var deps map[string]interface{}
			if err := json.Unmarshal([]byte(raw), &deps); err != nil {
				continue
			}
			for name := range deps {
				if name == target {
					depKind, hit = kind, true
					break
				}
			}
		}
		if !hit {
			continue
		}

		key := a.Format + ":" + a.Name
		d, ok := result[key]
		if !ok {
			d = &dependentInfo{PackageName: a.Name, PackageType: a.Format}
			result[key] = d
		}
		if !containsStr(d.Versions, a.Version) {
			d.Versions = append(d.Versions, a.Version)
		}
		if a.RepoName != "" && !containsStr(d.Repos, a.RepoName) {
			d.Repos = append(d.Repos, a.RepoName)
		}
		if d.Dependency == "" {
			d.Dependency = depKind
		}
	}
	return result
}

// mergeDependents 合并两个来源的依赖方（按包名），按名称排序。
func mergeDependents(maps ...map[string]*dependentInfo) []dependentInfo {
	merged := make(map[string]*dependentInfo)
	for _, m := range maps {
		for k, d := range m {
			if existing, ok := merged[k]; ok {
				existing.Versions = mergeStrings(existing.Versions, d.Versions)
				existing.Repos = mergeStrings(existing.Repos, d.Repos)
				existing.VulnCount += d.VulnCount
				if existing.Dependency == "" {
					existing.Dependency = d.Dependency
				}
			} else {
				merged[k] = d
			}
		}
	}
	names := make([]string, 0, len(merged))
	for k := range merged {
		names = append(names, k)
	}
	sortStrings(names)
	result := make([]dependentInfo, 0, len(names))
	for _, k := range names {
		result = append(result, *merged[k])
	}
	return result
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func mergeStrings(a, b []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range append(a, b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func sortStrings(list []string) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j] < list[j-1]; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}
