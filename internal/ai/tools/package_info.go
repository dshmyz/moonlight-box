package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
)

// PackageInfoTool 包信息查询工具
type PackageInfoTool struct {
	BaseTool
}

// NewPackageInfoTool 创建包信息查询工具
func NewPackageInfoTool() *PackageInfoTool {
	return &PackageInfoTool{}
}

// Name 返回工具名称
func (t *PackageInfoTool) Name() string {
	return "package_info"
}

// Description 返回工具描述
func (t *PackageInfoTool) Description() string {
	return "查询包的详细信息、版本历史、依赖关系"
}

// Parameters 返回工具参数的 JSON Schema
func (t *PackageInfoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"package_name": {
				"type": "string",
				"description": "包名称"
			},
			"package_type": {
				"type": "string",
				"description": "包类型 (npm, maven, pypi, go, nuget, yum, apt, generic)"
			},
			"version": {
				"type": "string",
				"description": "指定版本号"
			},
			"include_dependencies": {
				"type": "boolean",
				"description": "是否包含依赖信息",
				"default": false
			},
			"include_readme": {
				"type": "boolean",
				"description": "是否包含README内容",
				"default": false
			}
		},
		"required": ["package_name"]
	}`)
}

// Execute 执行工具并返回结果
func (t *PackageInfoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	packageName, ok := params["package_name"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: package_name")
	}

	packageType, _ := params["package_type"].(string)
	version, _ := params["version"].(string)
	includeDeps, _ := params["include_dependencies"].(bool)
	includeReadme, _ := params["include_readme"].(bool)

	db := t.Context().DB
	if db == nil {
		return "", fmt.Errorf("数据库连接未配置")
	}

	var versions []model.Component
	versionQuery := db.Model(&model.Component{}).Where("name = ?", packageName)
	if packageType != "" {
		versionQuery = versionQuery.Where("format = ?", packageType)
	}
	if version != "" {
		versionQuery = versionQuery.Where("version = ?", version)
	}
	if includeDeps {
		versionQuery = versionQuery.Preload("Dependencies")
	}
	if err := versionQuery.Order("published_at DESC").Find(&versions).Error; err != nil {
		return "", fmt.Errorf("查询组件信息失败: %v", err)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("未找到包: %s", packageName)
	}

	pkg := versions[0]
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 **%s** (%s)\n\n", pkg.Name, pkg.Format))

	if pkg.DisplayName != "" {
		sb.WriteString(fmt.Sprintf("📝 显示名称: %s\n", pkg.DisplayName))
	}
	if pkg.Description != "" {
		sb.WriteString(fmt.Sprintf("📄 描述: %s\n", pkg.Description))
	}
	if pkg.License != "" {
		sb.WriteString(fmt.Sprintf("⚖️  许可证: %s\n", pkg.License))
	}

	sb.WriteString(fmt.Sprintf("⬇️  总下载次数: %d\n", pkg.DownloadCount))
	sb.WriteString(fmt.Sprintf("📋 版本数量: %d\n\n", len(versions)))

	// 显示版本信息
	if len(versions) > 0 {
		sb.WriteString("📜 版本历史:\n")
		for i, v := range versions {
			if i >= 10 { // 最多显示10个版本
				sb.WriteString(fmt.Sprintf("   ... 还有 %d 个版本\n", len(versions)-10))
				break
			}

			sb.WriteString(fmt.Sprintf("   %d. **%s**", i+1, v.Version))
			if v.Status != model.StatusPublished {
				sb.WriteString(fmt.Sprintf(" (%s)", v.Status))
			}
			sb.WriteString(fmt.Sprintf(" - %s", v.PublishedAt.Format("2006-01-02")))
			sb.WriteString(fmt.Sprintf(" (下载: %d)", v.DownloadCount))

			if v.SizeBytes > 0 {
				sb.WriteString(fmt.Sprintf(" [%.2f KB]", float64(v.SizeBytes)/1024))
			}
			sb.WriteString("\n")

			// 显示依赖信息
			if includeDeps && len(v.Dependencies) > 0 {
				sb.WriteString("      依赖:\n")
				for _, dep := range v.Dependencies {
					optional := ""
					if dep.IsOptional {
						optional = " (可选)"
					}
					sb.WriteString(fmt.Sprintf("        - %s@%s [%s]%s\n",
						dep.DepName, dep.DepVersionConstraint, dep.DepType, optional))
				}
			}
		}
		sb.WriteString("\n")
	}

	// 显示README（如果需要）
	if includeReadme {
		// 查找README文件
		var readmeFile model.Asset
		for _, v := range versions {
			if err := db.Where("component_id = ? AND (filename LIKE ? OR filename LIKE ?)",
				v.ID, "%README%", "%readme%").
				First(&readmeFile).Error; err == nil {
				sb.WriteString("📖 README:\n")
				sb.WriteString("   (文件路径: " + readmeFile.Path + ")\n")
				// 注意: 实际读取README内容需要文件系统访问
				sb.WriteString("   请使用文件读取工具查看完整内容\n\n")
				break
			}
		}
	}

	// 显示安全信息
	var scanResult model.ScanResult
	if err := db.Where("component_id = ?", versions[0].ID).
		Preload("Vulnerabilities").
		First(&scanResult).Error; err == nil {
		sb.WriteString("🔒 安全状态:\n")
		if scanResult.ScanStatus == model.ScanStatusCompleted {
			sb.WriteString(fmt.Sprintf("   总漏洞数: %d\n", scanResult.TotalVulnerabilities))
			if scanResult.TotalVulnerabilities > 0 {
				sb.WriteString(fmt.Sprintf("   严重: %d, 高危: %d, 中危: %d, 低危: %d\n",
					scanResult.CriticalCount, scanResult.HighCount,
					scanResult.MediumCount, scanResult.LowCount))
			} else {
				sb.WriteString("   ✅ 未发现已知漏洞\n")
			}
		} else {
			sb.WriteString(fmt.Sprintf("   扫描状态: %s\n", scanResult.ScanStatus))
		}
	}

	return sb.String(), nil
}
