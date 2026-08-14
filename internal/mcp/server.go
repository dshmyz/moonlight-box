package mcp

import (
	"context"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/service"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

type mcpContextKey string

const mcpUserContextKey mcpContextKey = "mcp_user"

// SetMCPUser 将认证用户注入 context
func SetMCPUser(ctx context.Context, user *MCPUser) context.Context {
	return context.WithValue(ctx, mcpUserContextKey, user)
}

// MCPUserFromContext 从 context 中提取 MCP 认证用户
func MCPUserFromContext(ctx context.Context) *MCPUser {
	if u, ok := ctx.Value(mcpUserContextKey).(*MCPUser); ok {
		return u
	}
	return nil
}

// MCPServer 封装 mcp-go 服务端，直接使用 service 层
type MCPServer struct {
	mcp       *server.MCPServer
	cfg       *config.Config
	db        *gorm.DB
	repoSvc   *service.RepositoryService
	auditSvc  *service.AuditService
	blockSvc  *service.BlockRuleService
	permCache *service.PermissionCacheService
	readOnly  bool
}

// MCPUser MCP 请求中的认证用户信息
type MCPUser struct {
	UserID   uint
	Username string
	Roles    []string
	Static   bool // 静态 token 认证，拥有全部权限
}

// NewMCPServer 创建 MCP Server
func NewMCPServer(
	cfg *config.Config,
	db *gorm.DB,
	repoSvc *service.RepositoryService,
	auditSvc *service.AuditService,
	blockSvc *service.BlockRuleService,
	permCache *service.PermissionCacheService,
) *MCPServer {
	s := &MCPServer{
		cfg:      cfg,
		db:       db,
		repoSvc:  repoSvc,
		auditSvc: auditSvc,
		blockSvc: blockSvc,
		permCache: permCache,
		readOnly: cfg.MCP.ReadOnly,
	}

	mcpServer := server.NewMCPServer(
		"moonlight-box",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	s.mcp = mcpServer
	s.registerTools()

	return s
}

// GetMCPServer 返回底层 mcp-go 服务器
func (s *MCPServer) GetMCPServer() *server.MCPServer {
	return s.mcp
}

// addWriteTool 在非只读模式下注册写操作工具
func (s *MCPServer) addWriteTool(tool mcpgo.Tool, handler server.ToolHandlerFunc) {
	if !s.readOnly {
		s.mcp.AddTool(tool, handler)
	}
}

// checkPermission 检查当前 MCP 用户是否有指定权限
// 静态 token 认证直接放行；其他认证方式查 RBAC
func (s *MCPServer) checkPermission(ctx context.Context, resource, action string) (bool, error) {
	user := MCPUserFromContext(ctx)
	if user == nil {
		return false, fmt.Errorf("未认证")
	}
	if user.Static {
		return true, nil
	}
	if s.permCache == nil {
		return false, fmt.Errorf("权限服务未配置")
	}
	return s.permCache.HasPermission(user.UserID, resource, action)
}

// registerTools 注册所有 MCP 工具
func (s *MCPServer) registerTools() {
	// --- 包管理（只读） ---
	s.mcp.AddTool(
		mcpgo.NewTool("search_packages",
			mcpgo.WithDescription("搜索包仓库中的包，支持按名称、类型、关键词搜索"),
			mcpgo.WithString("keyword", mcpgo.Required(), mcpgo.Description("搜索关键词")),
			mcpgo.WithString("package_type", mcpgo.Description("包类型: npm, maven, pypi, go, yum, apt, generic")),
			mcpgo.WithNumber("page", mcpgo.Description("页码，默认 1")),
			mcpgo.WithNumber("page_size", mcpgo.Description("每页数量，默认 20")),
		),
		s.handleSearchPackages,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("list_package_versions",
			mcpgo.WithDescription("列出指定包的所有版本"),
			mcpgo.WithString("package_type", mcpgo.Required(), mcpgo.Description("包类型: npm, maven, pypi, go, yum, apt, generic")),
			mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("包名称")),
		),
		s.handleListPackageVersions,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("list_version_files",
			mcpgo.WithDescription("列出指定版本的文件列表"),
			mcpgo.WithString("package_type", mcpgo.Required(), mcpgo.Description("包类型")),
			mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("包名称")),
			mcpgo.WithString("version", mcpgo.Required(), mcpgo.Description("版本号")),
		),
		s.handleListVersionFiles,
	)

	s.addWriteTool(
		mcpgo.NewTool("deprecate_package_version",
			mcpgo.WithDescription("废弃指定包版本（标记为 deprecated）"),
			mcpgo.WithString("package_type", mcpgo.Required(), mcpgo.Description("包类型")),
			mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("包名称")),
			mcpgo.WithString("version", mcpgo.Required(), mcpgo.Description("版本号")),
			mcpgo.WithString("message", mcpgo.Description("废弃原因")),
		),
		s.handleDeprecateVersion,
	)

	s.addWriteTool(
		mcpgo.NewTool("yank_package_version",
			mcpgo.WithDescription("撤回指定包版本（从仓库中移除但保留元数据）"),
			mcpgo.WithString("package_type", mcpgo.Required(), mcpgo.Description("包类型")),
			mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("包名称")),
			mcpgo.WithString("version", mcpgo.Required(), mcpgo.Description("版本号")),
		),
		s.handleYankVersion,
	)

	// --- 仓库管理 ---
	s.mcp.AddTool(
		mcpgo.NewTool("list_repositories",
			mcpgo.WithDescription("列出所有仓库"),
			mcpgo.WithString("repo_type", mcpgo.Description("仓库类型: hosted, proxy, group")),
			mcpgo.WithString("format", mcpgo.Description("包格式: npm, maven, pypi, go, yum, apt, generic")),
		),
		s.handleListRepositories,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("get_repository",
			mcpgo.WithDescription("获取仓库详细信息"),
			mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("仓库名称")),
		),
		s.handleGetRepository,
	)

	s.addWriteTool(
		mcpgo.NewTool("create_repository",
			mcpgo.WithDescription("创建新仓库"),
			mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("仓库名称")),
			mcpgo.WithString("repo_type", mcpgo.Required(), mcpgo.Description("仓库类型: hosted, proxy, group")),
			mcpgo.WithString("format", mcpgo.Required(), mcpgo.Description("包格式: npm, maven, pypi, go, yum, apt, generic")),
			mcpgo.WithString("description", mcpgo.Description("仓库描述")),
			mcpgo.WithString("remote_url", mcpgo.Description("远程仓库 URL（proxy 类型必填）")),
		),
		s.handleCreateRepository,
	)

	s.addWriteTool(
		mcpgo.NewTool("delete_repository",
			mcpgo.WithDescription("删除仓库（危险操作，不可恢复）"),
			mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("仓库名称")),
		),
		s.handleDeleteRepository,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("get_repository_members",
			mcpgo.WithDescription("获取仓库成员列表"),
			mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("仓库名称")),
		),
		s.handleGetRepositoryMembers,
	)

	// --- 安全扫描 ---
	s.mcp.AddTool(
		mcpgo.NewTool("list_vulnerabilities",
			mcpgo.WithDescription("列出已发现的安全漏洞"),
			mcpgo.WithString("severity", mcpgo.Description("严重级别过滤: critical, high, medium, low"), mcpgo.Enum("critical", "high", "medium", "low")),
			mcpgo.WithNumber("limit", mcpgo.Description("返回数量限制，默认 50")),
		),
		s.handleListVulnerabilities,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("get_security_stats",
			mcpgo.WithDescription("获取安全扫描统计信息"),
		),
		s.handleGetSecurityStats,
	)

	s.addWriteTool(
		mcpgo.NewTool("trigger_security_scan",
			mcpgo.WithDescription("触发指定包的安全扫描"),
			mcpgo.WithString("artifact_id", mcpgo.Required(), mcpgo.Description("包版本 ID")),
		),
		s.handleTriggerScan,
	)

	// --- 缓存管理 ---
	s.mcp.AddTool(
		mcpgo.NewTool("list_caches",
			mcpgo.WithDescription("列出所有缓存区域"),
		),
		s.handleListCaches,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("get_cache_stats",
			mcpgo.WithDescription("获取缓存统计信息"),
			mcpgo.WithString("cache_name", mcpgo.Description("指定缓存名称，不传则返回全部")),
		),
		s.handleGetCacheStats,
	)

	// --- 封禁规则 ---
	s.mcp.AddTool(
		mcpgo.NewTool("list_block_rules",
			mcpgo.WithDescription("列出所有封禁规则"),
		),
		s.handleListBlockRules,
	)

	s.addWriteTool(
		mcpgo.NewTool("create_block_rule",
			mcpgo.WithDescription("创建封禁规则"),
			mcpgo.WithString("rule_type", mcpgo.Required(), mcpgo.Description("规则类型"), mcpgo.Enum("package", "version", "license", "cve")),
			mcpgo.WithString("pattern", mcpgo.Required(), mcpgo.Description("匹配模式（支持通配符）")),
			mcpgo.WithString("message", mcpgo.Description("封禁原因")),
		),
		s.handleCreateBlockRule,
	)

	s.addWriteTool(
		mcpgo.NewTool("delete_block_rule",
			mcpgo.WithDescription("删除封禁规则"),
			mcpgo.WithString("rule_id", mcpgo.Required(), mcpgo.Description("规则 ID")),
		),
		s.handleDeleteBlockRule,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("get_block_stats",
			mcpgo.WithDescription("获取封禁统计信息"),
		),
		s.handleGetBlockStats,
	)

	// --- 审计日志 ---
	s.mcp.AddTool(
		mcpgo.NewTool("list_audit_logs",
			mcpgo.WithDescription("查询操作审计日志"),
			mcpgo.WithString("username", mcpgo.Description("按用户名过滤")),
			mcpgo.WithString("action", mcpgo.Description("按操作类型过滤")),
			mcpgo.WithNumber("limit", mcpgo.Description("返回数量，默认 50")),
		),
		s.handleListAuditLogs,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("list_download_logs",
			mcpgo.WithDescription("查询包下载日志"),
			mcpgo.WithString("package_name", mcpgo.Description("按包名过滤")),
			mcpgo.WithString("remote_addr", mcpgo.Description("按客户端 IP 过滤")),
			mcpgo.WithNumber("limit", mcpgo.Description("返回数量，默认 50")),
		),
		s.handleListDownloadLogs,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("get_download_stats",
			mcpgo.WithDescription("获取下载统计信息"),
		),
		s.handleGetDownloadStats,
	)

	// --- 系统管理 ---
	s.mcp.AddTool(
		mcpgo.NewTool("get_system_info",
			mcpgo.WithDescription("获取系统信息（版本、运行时、存储等）"),
		),
		s.handleGetSystemInfo,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("get_dashboard_stats",
			mcpgo.WithDescription("获取仪表盘统计数据"),
		),
		s.handleGetDashboardStats,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("list_health_status",
			mcpgo.WithDescription("获取所有远程仓库的健康状态"),
		),
		s.handleListHealthStatus,
	)

	s.mcp.AddTool(
		mcpgo.NewTool("reset_circuit_breaker",
			mcpgo.WithDescription("重置指定仓库的熔断器"),
			mcpgo.WithString("repo_id", mcpgo.Required(), mcpgo.Description("仓库 ID")),
		),
		s.handleResetCircuitBreaker,
	)
}

// textResult 构造文本类型的 MCP 工具结果
func textResult(text string) *mcpgo.CallToolResult {
	return mcpgo.NewToolResultText(text)
}

// errorResult 构造错误类型的 MCP 工具结果
func errorResult(format string, args ...interface{}) *mcpgo.CallToolResult {
	return mcpgo.NewToolResultError(fmt.Sprintf(format, args...))
}
