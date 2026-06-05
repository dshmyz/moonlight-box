package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/model"
)

type PackageInfoTool struct{ BaseTool }

func NewPackageInfoTool() *PackageInfoTool { return &PackageInfoTool{} }

func (t *PackageInfoTool) Name() string { return "package_info" }

func (t *PackageInfoTool) Description() string {
	return "查询包的详细信息、版本历史"
}

func (t *PackageInfoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"package_name": {"type": "string"},
			"package_type": {"type": "string"},
			"version": {"type": "string"}
		},
		"required": ["package_name"]
	}`)
}

func (t *PackageInfoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	packageName, ok := params["package_name"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: package_name")
	}
	packageType, _ := params["package_type"].(string)
	version, _ := params["version"].(string)

	db := t.Context().DB
	if db == nil {
		return "", fmt.Errorf("数据库连接未配置")
	}

	var artifacts []model.Artifact
	q := db.Model(&model.Artifact{}).Where("name = ?", packageName)
	if packageType != "" {
		q = q.Where("format = ?", packageType)
	}
	if version != "" {
		q = q.Where("version = ?", version)
	}
	if err := q.Order("created_at DESC").Find(&artifacts).Error; err != nil {
		return "", fmt.Errorf("查询包信息失败: %v", err)
	}
	if len(artifacts) == 0 {
		return fmt.Sprintf("未找到包: %s", packageName), nil
	}

	a := artifacts[0]
	name := a.Name

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 **%s** (%s)\n\n", name, a.Format))
	sb.WriteString(fmt.Sprintf("📋 版本数量: %d\n\n", len(artifacts)))

	sb.WriteString("📜 版本历史:\n")
	for i, v := range artifacts {
		if i >= 10 {
			sb.WriteString(fmt.Sprintf("   ... 还有 %d 个版本\n", len(artifacts)-10))
			break
		}
		ver := v.Version
		status := "published"
		if v.Metadata != nil {
			if s, ok := v.Metadata["status"].(string); ok {
				status = s
			}
		}
		sb.WriteString(fmt.Sprintf("   %d. **%s**", i+1, ver))
		if status != "published" {
			sb.WriteString(fmt.Sprintf(" (%s)", status))
		}
		sb.WriteString(fmt.Sprintf(" - %s\n", v.CreatedAt.Format("2006-01-02")))
	}
	sb.WriteString("\n")

	return sb.String(), nil
}
