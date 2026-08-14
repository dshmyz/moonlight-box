package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

func (s *MCPServer) handleListCaches(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	// 缓存信息从系统运行时获取，这里返回基本统计
	return textResult(formatJSON(map[string]interface{}{
		"message": "缓存管理功能请通过 HTTP API /api/v1/cache/caches 查看",
		"hint":    "MCP 工具可直接查询数据库中的包和仓库数据",
	})), nil
}

func (s *MCPServer) handleGetCacheStats(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return textResult(formatJSON(map[string]interface{}{
		"message": "缓存统计请通过 HTTP API /api/v1/cache/stats 查看",
	})), nil
}
