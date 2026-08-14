package mcp

import (
	"context"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/model"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

func (s *MCPServer) handleListBlockRules(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	var rules []model.BlockRule
	if err := s.db.Order("created_at DESC").Find(&rules).Error; err != nil {
		return errorResult("获取封禁规则列表失败: %v", err), nil
	}
	return textResult(formatJSON(rules)), nil
}

func (s *MCPServer) handleCreateBlockRule(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if ok, err := s.checkPermission(ctx, "block-rules", "write"); !ok {
		return errorResult("权限不足: %v", err), nil
	}

	args := req.GetArguments()
	ruleType := args["rule_type"].(string)
	pattern := args["pattern"].(string)
	message, _ := args["message"].(string)

	// rule_type 对应 PackageType，pattern 对应 PackageName
	rule := model.BlockRule{
		PackageName: pattern,
		PackageType: ruleType,
		MatchType:   model.BlockMatchWildcard,
		Reason:      message,
		Enabled:     true,
	}
	if err := s.db.Create(&rule).Error; err != nil {
		return errorResult("创建封禁规则失败: %v", err), nil
	}
	return textResult(formatJSON(map[string]interface{}{
		"message": "✅ 封禁规则已创建", "id": rule.ID, "package_name": rule.PackageName,
	})), nil
}

func (s *MCPServer) handleDeleteBlockRule(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if ok, err := s.checkPermission(ctx, "block-rules", "delete"); !ok {
		return errorResult("权限不足: %v", err), nil
	}

	ruleID := parseUint(req.GetArguments()["rule_id"].(string))
	r := s.db.Delete(&model.BlockRule{}, ruleID)
	if r.Error != nil {
		return errorResult("删除封禁规则失败: %v", r.Error), nil
	}
	if r.RowsAffected == 0 {
		return errorResult("未找到封禁规则: %d", ruleID), nil
	}
	return textResult(fmt.Sprintf("✅ 封禁规则 %d 已删除", ruleID)), nil
}

func (s *MCPServer) handleGetBlockStats(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	var total, active int64
	s.db.Model(&model.BlockRule{}).Count(&total)
	s.db.Model(&model.BlockRule{}).Where("enabled = ?", true).Count(&active)
	return textResult(formatJSON(map[string]interface{}{"total": total, "active": active})), nil
}
