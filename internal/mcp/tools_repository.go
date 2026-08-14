package mcp

import (
	"context"

	"github.com/dshmyz/moonlight-box/internal/model"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

func (s *MCPServer) handleListRepositories(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	repoType, _ := args["repo_type"].(string)
	format, _ := args["format"].(string)

	var repos []model.Repository
	q := s.db.Model(&model.Repository{})
	if repoType != "" {
		q = q.Where("type = ?", repoType)
	}
	if format != "" {
		q = q.Where("package_type = ?", format)
	}
	if err := q.Order("created_at DESC").Find(&repos).Error; err != nil {
		return errorResult("获取仓库列表失败: %v", err), nil
	}
	return textResult(formatJSON(repos)), nil
}

func (s *MCPServer) handleGetRepository(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	name := req.GetArguments()["name"].(string)
	var repo model.Repository
	if err := s.db.Where("name = ?", name).First(&repo).Error; err != nil {
		return errorResult("未找到仓库: %s", name), nil
	}

	// 单独查 members，避免 Preload 外键问题
	var members []model.RepositoryMember
	s.db.Where("repository_id = ?", repo.ID).Find(&members)

	result := map[string]interface{}{
		"repository": repo,
		"members":    members,
	}
	return textResult(formatJSON(result)), nil
}

func (s *MCPServer) handleCreateRepository(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if ok, err := s.checkPermission(ctx, "repositories", "write"); !ok {
		return errorResult("权限不足: %v", err), nil
	}

	args := req.GetArguments()
	name := args["name"].(string)
	repoType := args["repo_type"].(string)
	format := args["format"].(string)

	repo := model.Repository{
		Name:        name,
		Type:        model.RepositoryType(repoType),
		PackageType: format,
	}
	if desc, ok := args["description"].(string); ok {
		repo.Description = desc
	}
	if remoteURL, ok := args["remote_url"].(string); ok && remoteURL != "" {
		repo.Config = &model.RepositoryConfig{RemoteURL: remoteURL}
	}

	// 使用 RepositoryService 创建，确保 runtime router 注册
	if err := s.repoSvc.Create(&repo, nil); err != nil {
		return errorResult("创建仓库失败: %v", err), nil
	}
	return textResult(formatJSON(map[string]interface{}{
		"message": "✅ 仓库创建成功", "id": repo.ID, "name": repo.Name,
	})), nil
}

func (s *MCPServer) handleDeleteRepository(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if ok, err := s.checkPermission(ctx, "repositories", "delete"); !ok {
		return errorResult("权限不足: %v", err), nil
	}

	name := req.GetArguments()["name"].(string)
	// 使用 RepositoryService 删除，确保 runtime router 注销
	if err := s.repoSvc.Delete(name); err != nil {
		return errorResult("删除仓库失败: %v", err), nil
	}
	return textResult("✅ 仓库 " + name + " 已删除"), nil
}

func (s *MCPServer) handleGetRepositoryMembers(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	name := req.GetArguments()["name"].(string)
	var repo model.Repository
	if err := s.db.Where("name = ?", name).First(&repo).Error; err != nil {
		return errorResult("未找到仓库: %s", name), nil
	}
	var members []model.RepositoryMember
	if err := s.db.Where("repository_id = ?", repo.ID).Find(&members).Error; err != nil {
		return errorResult("获取仓库成员失败: %v", err), nil
	}
	return textResult(formatJSON(members)), nil
}
