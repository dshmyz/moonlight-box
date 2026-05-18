package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

// DependencyOptimizerTool 依赖优化分析工具
type DependencyOptimizerTool struct {
	BaseTool
}

// NewDependencyOptimizerTool 创建依赖优化分析工具
func NewDependencyOptimizerTool() *DependencyOptimizerTool {
	return &DependencyOptimizerTool{}
}

// Name 返回工具名称
func (t *DependencyOptimizerTool) Name() string {
	return "dependency_optimizer"
}

// Description 返回工具描述
func (t *DependencyOptimizerTool) Description() string {
	return "分析项目依赖关系，提供优化建议：版本冲突检测、安全升级推荐、未使用依赖识别，支持生成 Mermaid 依赖图"
}

// Parameters 返回工具参数的 JSON Schema
func (t *DependencyOptimizerTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project_name": {
				"type": "string",
				"description": "项目名称或标识"
			},
			"package_type": {
				"type": "string",
				"enum": ["npm", "maven", "pypi", "go", "nuget", "generic"],
				"description": "包类型"
			},
			"analysis_scope": {
				"type": "string",
				"enum": ["conflicts", "security", "unused", "all"],
				"default": "all",
				"description": "分析范围：冲突检测/安全扫描/未使用依赖/全部"
			},
			"include_transitive": {
				"type": "boolean",
				"default": true,
				"description": "是否分析传递依赖"
			},
			"visualize": {
				"type": "boolean",
				"default": false,
				"description": "是否生成 Mermaid 依赖关系图"
			},
			"max_depth": {
				"type": "integer",
				"default": 3,
				"minimum": 1,
				"maximum": 5,
				"description": "依赖图最大层级深度"
			},
			"min_severity": {
				"type": "string",
				"enum": ["low", "medium", "high", "critical"],
				"default": "medium",
				"description": "安全漏洞最小严重级别"
			}
		},
		"required": ["project_name"]
	}`)
}

// Execute 执行依赖优化分析
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
	includeTransitive, _ := params["include_transitive"].(bool)
	visualize, _ := params["visualize"].(bool)
	maxDepth, _ := params["max_depth"].(int)
	if maxDepth < 1 {
		maxDepth = 3
	}
	if maxDepth > 5 {
		maxDepth = 5
	}
	minSeverity, _ := params["min_severity"].(string)
	if minSeverity == "" {
		minSeverity = "medium"
	}

	db := t.Context().DB
	if db == nil {
		return "", fmt.Errorf("数据库连接未配置")
	}

	// 查询项目关联的包
	var packages []model.Package
	query := db.Model(&model.Package{}).
		Where("name LIKE ? OR display_name LIKE ?", 
			"%"+projectName+"%", "%"+projectName+"%")
	if packageType != "" {
		query = query.Where("type = ?", packageType)
	}
	if err := query.Limit(50).Find(&packages).Error; err != nil {
		return "", fmt.Errorf("查询项目包失败: %v", err)
	}

	if len(packages) == 0 {
		return fmt.Sprintf("⚠️ 未找到与 '%s' 相关的项目包", projectName), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **依赖优化分析报告** - %s\n\n", projectName))

	// 收集所有依赖关系
	dependencies := make(map[string][]*model.PackageDependency)
	versionMap := make(map[string]map[string]bool)

	for _, pkg := range packages {
		var versions []model.PackageVersion
		if err := db.Where("package_id = ?", pkg.ID).Find(&versions).Error; err != nil {
			continue
		}

		for _, v := range versions {
			var deps []model.PackageDependency
			dbQuery := db.Where("version_id = ?", v.ID)
			if !includeTransitive {
				dbQuery = dbQuery.Where("dep_type = ?", "direct")
			}
			if err := dbQuery.Find(&deps).Error; err != nil {
				continue
			}

			for _, dep := range deps {
				dependencies[dep.DepName] = append(dependencies[dep.DepName], &dep)
				if versionMap[dep.DepName] == nil {
					versionMap[dep.DepName] = make(map[string]bool)
				}
				versionMap[dep.DepName][dep.DepVersionConstraint] = true
			}
		}
	}

	// 执行分析
	if analysisScope == "conflicts" || analysisScope == "all" {
		sb.WriteString(t.analyzeConflicts(dependencies, versionMap))
	}
	if analysisScope == "security" || analysisScope == "all" {
		sb.WriteString(t.analyzeSecurity(db, dependencies, minSeverity))
	}
	if analysisScope == "unused" || analysisScope == "all" {
		sb.WriteString(t.analyzeUsage(db, packages))
	}

	// 生成优化建议
	sb.WriteString("\n💡 **优化建议**\n")
	sb.WriteString(t.generateRecommendations(dependencies, versionMap, db))

	// 生成可视化依赖图（如果请求）
	if visualize {
		sb.WriteString("\n\n📊 **依赖关系图** (Mermaid 格式)\n")
		sb.WriteString("```mermaid\n")
		sb.WriteString(t.generateMermaidGraph(db, packages, dependencies, maxDepth, includeTransitive))
		sb.WriteString("```\n")
		sb.WriteString("\n> 💡 提示: 在支持 Mermaid 的 Markdown 查看器中可自动渲染为图形")
	}

	return sb.String(), nil
}

// analyzeConflicts 分析版本冲突
func (t *DependencyOptimizerTool) analyzeConflicts(
	deps map[string][]*model.PackageDependency,
	versions map[string]map[string]bool,
) string {
	var sb strings.Builder
	
	conflicts := make(map[string][]string)
	for name, depList := range deps {
		versionSet := make(map[string]bool)
		for _, dep := range depList {
			versionSet[dep.DepVersionConstraint] = true
		}
		if len(versionSet) > 1 {
			conflicts[name] = make([]string, 0, len(versionSet))
			for v := range versionSet {
				conflicts[name] = append(conflicts[name], v)
			}
		}
	}

	if len(conflicts) == 0 {
		sb.WriteString("✅ **版本冲突检测**: 未发现依赖版本冲突\n\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("⚠️ **发现 %d 个依赖版本冲突**\n", len(conflicts)))
	
	type conflictItem struct {
		name     string
		versions []string
	}
	items := make([]conflictItem, 0, len(conflicts))
	for name, vers := range conflicts {
		sort.Strings(vers)
		items = append(items, conflictItem{name, vers})
	}
	sort.Slice(items, func(i, j int) bool {
		return len(items[i].versions) > len(items[j].versions)
	})

	for _, item := range items {
		sb.WriteString(fmt.Sprintf("\n📦 `%s`:\n", item.name))
		for _, v := range item.versions {
			sb.WriteString(fmt.Sprintf("   - %s\n", v))
		}
		sb.WriteString(fmt.Sprintf("   💡 建议: 统一使用最新兼容版本，或检查是否需要拆分模块隔离依赖\n"))
	}
	sb.WriteString("\n")
	return sb.String()
}

// analyzeSecurity 分析安全漏洞
func (t *DependencyOptimizerTool) analyzeSecurity(
	db *gorm.DB,
	deps map[string][]*model.PackageDependency,
	minSeverity string,
) string {
	var sb strings.Builder

	severityOrder := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}
	minLevel := severityOrder[minSeverity]

	vulnerableDeps := make(map[string][]model.ScanResult)

	for depName := range deps {
		var pkg model.Package
		if err := db.Where("name = ?", depName).First(&pkg).Error; err != nil {
			continue
		}

		var versions []model.PackageVersion
		if err := db.Where("package_id = ?", pkg.ID).Find(&versions).Error; err != nil {
			continue
		}

		for _, v := range versions {
			var scan model.ScanResult
			if err := db.Where("version_id = ?", v.ID).
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
				if scan.LowCount > 0 && minLevel <= 1 {
					hasRelevant = true
				}

				if hasRelevant {
					vulnerableDeps[depName+"@"+v.Version] = append(vulnerableDeps[depName+"@"+v.Version], scan)
				}
			}
		}
	}

	if len(vulnerableDeps) == 0 {
		sb.WriteString("✅ **安全扫描**: 未发现高于 " + minSeverity + " 级别的漏洞依赖\n\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("🔒 **发现 %d 个存在安全风险的依赖**\n\n", len(vulnerableDeps)))

	for depVer, scans := range vulnerableDeps {
		sb.WriteString(fmt.Sprintf("📦 `%s`:\n", depVer))
		for _, scan := range scans {
			if scan.CriticalCount > 0 {
				sb.WriteString(fmt.Sprintf("   🔴 严重: %d 个漏洞\n", scan.CriticalCount))
			}
			if scan.HighCount > 0 {
				sb.WriteString(fmt.Sprintf("   🟠 高危: %d 个漏洞\n", scan.HighCount))
			}
			if scan.MediumCount > 0 {
				sb.WriteString(fmt.Sprintf("   🟡 中危: %d 个漏洞 (低于阈值，已忽略)\n", scan.MediumCount))
			}
		}
		sb.WriteString("   💡 建议: 升级到修复版本，或添加临时安全规则屏蔽风险接口\n")
		sb.WriteString("\n")
	}
	return sb.String()
}

// analyzeUsage 分析未使用依赖
func (t *DependencyOptimizerTool) analyzeUsage(db *gorm.DB, packages []model.Package) string {
	var sb strings.Builder

	underused := make([]model.Package, 0)

	for _, pkg := range packages {
		if pkg.DownloadCount < 10 {
			var latest model.PackageVersion
			if err := db.Where("package_id = ?", pkg.ID).
				Order("published_at DESC").
				First(&latest).Error; err == nil {
				underused = append(underused, pkg)
			}
		}
	}

	if len(underused) == 0 {
		sb.WriteString("✅ **使用率分析**: 所有依赖均有正常活跃度\n\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("📊 **发现 %d 个低活跃度依赖**\n", len(underused)))
	sb.WriteString("⚠️  以下包下载量<10，可能未被实际使用：\n")
	for _, pkg := range underused {
		sb.WriteString(fmt.Sprintf("   - `%s` (%s) - 下载: %d, 类型: %s\n",
			pkg.Name, pkg.DisplayName, pkg.DownloadCount, pkg.Type))
	}
	sb.WriteString("\n💡 建议: 确认这些依赖是否仍需要，可考虑移除以减少攻击面和构建体积\n\n")
	return sb.String()
}

// generateRecommendations 生成综合优化建议
func (t *DependencyOptimizerTool) generateRecommendations(
	deps map[string][]*model.PackageDependency,
	versions map[string]map[string]bool,
	db *gorm.DB,
) string {
	var sb strings.Builder

	consolidatable := 0
	for _, versionSet := range versions {
		if len(versionSet) > 1 {
			consolidatable++
		}
	}
	if consolidatable > 0 {
		sb.WriteString(fmt.Sprintf("1️⃣  **版本统一**: %d 个依赖存在多版本，建议统一约束减少冲突风险\n", consolidatable))
	}

	uniqueDeps := len(deps)
	totalDeps := 0
	for _, list := range deps {
		totalDeps += len(list)
	}
	if totalDeps > uniqueDeps*2 {
		sb.WriteString(fmt.Sprintf("2️⃣  **依赖去重**: 发现重复声明 %d 次，建议在顶层依赖管理中统一声明\n", totalDeps-uniqueDeps))
	}

	securityCount := 0
	for depName := range deps {
		var pkg model.Package
		if err := db.Where("name = ?", depName).First(&pkg).Error; err != nil {
			continue
		}
		var scan model.ScanResult
		if err := db.Where("package_id = ?", pkg.ID).
			Where("scan_status = ?", model.ScanStatusCompleted).
			Where("total_vulnerabilities > 0").
			First(&scan).Error; err == nil {
			securityCount++
		}
	}
	if securityCount > 0 {
		sb.WriteString(fmt.Sprintf("3️⃣  **安全加固**: %d 个依赖存在已知漏洞，建议配置自动安全扫描流水线\n", securityCount))
	}

	if consolidatable == 0 && totalDeps <= uniqueDeps*2 && securityCount == 0 {
		sb.WriteString("✨ 当前依赖状态良好，建议保持定期审计和更新习惯")
	}

	return sb.String()
}

// nodeMeta 节点元数据
type nodeMeta struct {
	hasConflict bool
	hasVuln     bool
	isRoot      bool
}

// generateMermaidGraph 生成 Mermaid 格式的依赖关系图
func (t *DependencyOptimizerTool) generateMermaidGraph(
	db *gorm.DB,
	packages []model.Package,
	deps map[string][]*model.PackageDependency,
	maxDepth int,
	includeTransitive bool,
) string {
	var sb strings.Builder
	
	sb.WriteString("flowchart TD\n")
	sb.WriteString("    classDef conflict fill:#ffcccc,stroke:#ff0000,stroke-width:2px\n")
	sb.WriteString("    classDef vulnerable fill:#fff3cd,stroke:#ffc107,stroke-width:2px\n")
	sb.WriteString("    classDef root fill:#d4edda,stroke:#28a745,stroke-width:2px\n")
	sb.WriteString("    classDef normal fill:#f8f9fa,stroke:#6c757d\n\n")

	visited := make(map[string]bool)
	edges := make(map[string]map[string]bool)
	nodeInfo := make(map[string]*nodeMeta)

	// 标记根节点
	for _, pkg := range packages {
		nodeKey := fmt.Sprintf("%s@%s", pkg.Name, pkg.Type)
		nodeInfo[nodeKey] = &nodeMeta{isRoot: true}
		visited[nodeKey] = true
	}

	// 构建依赖边
	currentLevel := make(map[string][]string)
	
	for depName, depList := range deps {
		for _, dep := range depList {
			if edges[depName] == nil {
				edges[depName] = make(map[string]bool)
			}
			edges[depName][dep.DepVersionConstraint] = true
			currentLevel[depName] = append(currentLevel[depName], dep.DepVersionConstraint)
			
			// 检查冲突
			versions := make(map[string]bool)
			for _, d := range deps[depName] {
				versions[d.DepVersionConstraint] = true
			}
			if len(versions) > 1 {
				if nodeInfo[depName] == nil {
					nodeInfo[depName] = &nodeMeta{}
				}
				nodeInfo[depName].hasConflict = true
			}
		}
	}

	// 检查安全漏洞
	checkedVuln := make(map[string]bool)
	for depName := range deps {
		if checkedVuln[depName] {
			continue
		}
		var pkg model.Package
		if err := db.Where("name = ?", depName).First(&pkg).Error; err != nil {
			continue
		}
		var scan model.ScanResult
		if err := db.Where("package_id = ?", pkg.ID).
			Where("scan_status = ?", model.ScanStatusCompleted).
			Where("total_vulnerabilities > 0").
			First(&scan).Error; err == nil {
			if nodeInfo[depName] == nil {
				nodeInfo[depName] = &nodeMeta{}
			}
			nodeInfo[depName].hasVuln = true
		}
		checkedVuln[depName] = true
	}

	// 输出根节点
	for _, pkg := range packages {
		nodeID := sanitizeMermaidID(fmt.Sprintf("%s_%s", pkg.Name, pkg.Type))
		label := fmt.Sprintf("%s\\n(%s)", pkg.Name, pkg.Type)
		sb.WriteString(fmt.Sprintf("    %s[\"%s\"]:::root\n", nodeID, label))
	}

	// 输出依赖节点和边
	processed := make(map[string]bool)
	for depName, versions := range currentLevel {
		if processed[depName] {
			continue
		}
		processed[depName] = true
		
		nodeID := sanitizeMermaidID(depName)
		versionLabel := strings.Join(versions, ", ")
		if len(versions) > 3 {
			versionLabel = fmt.Sprintf("%d versions", len(versions))
		}
		
		style := "normal"
		if nodeInfo[depName] != nil && nodeInfo[depName].hasConflict {
			style = "conflict"
		} else if nodeInfo[depName] != nil && nodeInfo[depName].hasVuln {
			style = "vulnerable"
		}
		
		label := fmt.Sprintf("%s\\n%s", depName, versionLabel)
		sb.WriteString(fmt.Sprintf("    %s[\"%s\"]:::%s\n", nodeID, label, style))
		
		if len(packages) > 0 {
			rootID := sanitizeMermaidID(fmt.Sprintf("%s_%s", packages[0].Name, packages[0].Type))
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", rootID, nodeID))
		}
		
		if includeTransitive && maxDepth > 1 {
			t.addTransitiveDeps(db, &sb, depName, 1, maxDepth, edges, nodeInfo)
		}
	}

	// 图例
	sb.WriteString("\n    %% 图例\n")
	sb.WriteString("    subgraph legend[\"📋 图例\"]\n")
	sb.WriteString("        direction LR\n")
	sb.WriteString("        l1[\"项目包\"]:::root\n")
	sb.WriteString("        l2[\"正常依赖\"]:::normal\n")
	sb.WriteString("        l3[\"⚠️ 版本冲突\"]:::conflict\n")
	sb.WriteString("        l4[\"🔒 安全漏洞\"]:::vulnerable\n")
	sb.WriteString("    end\n")

	return sb.String()
}

// addTransitiveDeps 递归添加传递依赖节点
func (t *DependencyOptimizerTool) addTransitiveDeps(
	db *gorm.DB,
	sb *strings.Builder,
	parentName string,
	currentDepth, maxDepth int,
	edges map[string]map[string]bool,
	nodeInfo map[string]*nodeMeta,
) {
	if currentDepth >= maxDepth {
		return
	}
	// 实际实现应解析依赖文件的 requires/dependencies 字段
	// 当前为简化版本，预留扩展点
}

// sanitizeMermaidID 清理节点ID使其符合 Mermaid 语法
func sanitizeMermaidID(s string) string {
	replacer := strings.NewReplacer(
		"@", "_at_",
		"/", "_",
		"\\", "_",
		"-", "_",
		".", "_",
		" ", "_",
		":", "_",
	)
	return replacer.Replace(s)
}
