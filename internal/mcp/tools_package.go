package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/model"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

func (s *MCPServer) handleSearchPackages(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	keyword := args["keyword"].(string)
	packageType, _ := args["package_type"].(string)
	page := 1
	pageSize := 20
	if v, ok := args["page"].(float64); ok && v > 0 {
		page = int(v)
	}
	if v, ok := args["page_size"].(float64); ok && v > 0 {
		pageSize = int(v)
	}

	var artifacts []model.Artifact
	q := s.db.Model(&model.Artifact{}).Where("name LIKE ?", "%"+keyword+"%")
	if packageType != "" {
		q = q.Where("format = ?", packageType)
	}

	var total int64
	q.Count(&total)

	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&artifacts).Error; err != nil {
		return errorResult("搜索失败: %v", err), nil
	}

	return textResult(formatJSON(map[string]interface{}{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"items":     artifacts,
	})), nil
}

func (s *MCPServer) handleListPackageVersions(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	packageType := args["package_type"].(string)
	name := args["name"].(string)

	var artifacts []model.Artifact
	if err := s.db.Where("name = ? AND format = ?", name, packageType).
		Order("created_at DESC").Find(&artifacts).Error; err != nil {
		return errorResult("查询版本列表失败: %v", err), nil
	}

	if len(artifacts) == 0 {
		return textResult(fmt.Sprintf("未找到包 %s (%s)", name, packageType)), nil
	}

	type ver struct {
		Version   string `json:"version"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
	}
	versions := make([]ver, len(artifacts))
	for i, a := range artifacts {
		status := "published"
		if a.Metadata != nil {
			if st, ok := a.Metadata["status"].(string); ok {
				status = st
			}
		}
		versions[i] = ver{Version: a.Version, Status: status, CreatedAt: a.CreatedAt.Format("2006-01-02 15:04:05")}
	}

	return textResult(formatJSON(map[string]interface{}{
		"package": name, "format": packageType, "total": len(versions), "versions": versions,
	})), nil
}

func (s *MCPServer) handleListVersionFiles(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	var artifact model.Artifact
	if err := s.db.Where("name = ? AND version = ? AND format = ?",
		args["name"], args["version"], args["package_type"]).
		First(&artifact).Error; err != nil {
		return errorResult("未找到 %s@%s (%s)", args["name"], args["version"], args["package_type"]), nil
	}

	return textResult(formatJSON(map[string]interface{}{
		"package":  artifact.Name, "version": artifact.Version, "format": artifact.Format,
		"metadata": artifact.Metadata,
	})), nil
}

func (s *MCPServer) handleDeprecateVersion(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if ok, err := s.checkPermission(ctx, "package", "write"); !ok {
		return errorResult("权限不足: %v", err), nil
	}

	args := req.GetArguments()
	name, version := args["name"].(string), args["version"].(string)
	repoID, _ := args["repository_id"].(float64)

	// 加载 artifact（限制在指定仓库内）
	var artifact model.Artifact
	q := s.db.Where("name = ? AND version = ?", name, version)
	if repoID > 0 {
		q = q.Where("repository_id = ?", uint(repoID))
	}
	if err := q.First(&artifact).Error; err != nil {
		return errorResult("未找到 %s@%s", name, version), nil
	}

	// 更新 metadata（跨数据库兼容）
	if artifact.Metadata == nil {
		artifact.Metadata = make(model.JSONB)
	}
	artifact.Metadata["status"] = "deprecated"
	if err := s.db.Model(&artifact).Update("metadata", artifact.Metadata).Error; err != nil {
		return errorResult("废弃版本失败: %v", err), nil
	}
	return textResult("✅ 已废弃 " + name + "@" + version), nil
}

func (s *MCPServer) handleYankVersion(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if ok, err := s.checkPermission(ctx, "package", "write"); !ok {
		return errorResult("权限不足: %v", err), nil
	}

	args := req.GetArguments()
	name, version := args["name"].(string), args["version"].(string)
	repoID, _ := args["repository_id"].(float64)

	// 加载 artifact（限制在指定仓库内）
	var artifact model.Artifact
	q := s.db.Where("name = ? AND version = ?", name, version)
	if repoID > 0 {
		q = q.Where("repository_id = ?", uint(repoID))
	}
	if err := q.First(&artifact).Error; err != nil {
		return errorResult("未找到 %s@%s", name, version), nil
	}

	// 更新 metadata（跨数据库兼容）
	if artifact.Metadata == nil {
		artifact.Metadata = make(model.JSONB)
	}
	artifact.Metadata["status"] = "yanked"
	if err := s.db.Model(&artifact).Update("metadata", artifact.Metadata).Error; err != nil {
		return errorResult("撤回版本失败: %v", err), nil
	}
	return textResult("✅ 已撤回 " + name + "@" + version), nil
}

func formatJSON(v interface{}) string {
	switch val := v.(type) {
	case json.RawMessage:
		var buf bytes.Buffer
		if json.Indent(&buf, val, "", "  ") == nil {
			return buf.String()
		}
		return string(val)
	default:
		b, _ := json.MarshalIndent(v, "", "  ")
		return string(b)
	}
}
