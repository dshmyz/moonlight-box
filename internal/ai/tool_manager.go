package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/ai/models"
	"github.com/dshmyz/moonlight-box/internal/ai/tools"
	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/model"
)

// ToolManager 工具管理器
type ToolManager struct {
	tools       map[string]*registeredTool
	mu          sync.RWMutex
	config      *config.AIToolsConfig
	auditLogger *AuditLogger
}

// registeredTool 注册的工具
type registeredTool struct {
	tool         tools.Tool
	allowedRoles map[string]bool
}

// AuditLogger 审计日志记录器
type AuditLogger struct {
	entries []AuditEntry
	mu      sync.RWMutex
	enabled bool
	maxSize int
}

// AuditEntry 审计日志条目
type AuditEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	ToolName  string                 `json:"tool_name"`
	UserID    uint                   `json:"user_id"`
	Username  string                 `json:"username"`
	Params    map[string]interface{} `json:"params"`
	Result    string                 `json:"result"`
	Error     string                 `json:"error,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Success   bool                   `json:"success"`
}

// ToolInfo 工具信息
type ToolInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	AllowedRoles []string `json:"allowed_roles"`
}

// NewToolManager 创建一个新的工具管理器
func NewToolManager(cfg *config.AIToolsConfig) *ToolManager {
	tm := &ToolManager{
		tools:  make(map[string]*registeredTool),
		config: cfg,
		auditLogger: &AuditLogger{
			entries: make([]AuditEntry, 0),
			enabled: cfg.EnableAuditLog,
			maxSize: 10000, // 最多保留10000条审计日志
		},
	}
	return tm
}

// RegisterTool 注册工具
func (tm *ToolManager) RegisterTool(tool tools.Tool, allowedRoles []string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 将角色列表转换为map便于快速查找
	rolesMap := make(map[string]bool)
	for _, role := range allowedRoles {
		rolesMap[role] = true
	}

	tm.tools[tool.Name()] = &registeredTool{
		tool:         tool,
		allowedRoles: rolesMap,
	}
}

// ExecuteTool 执行工具
func (tm *ToolManager) ExecuteTool(ctx context.Context, toolName string, params map[string]interface{}, user *model.User) (string, error) {
	startTime := time.Now()

	// 获取工具
	tm.mu.RLock()
	registered, exists := tm.tools[toolName]
	tm.mu.RUnlock()

	if !exists {
		err := fmt.Errorf("tool not found: %s", toolName)
		tm.logAudit(toolName, user, params, "", err, 0, false)
		return "", err
	}

	// 检查权限
	if !tm.hasPermission(registered, user) {
		err := fmt.Errorf("permission denied for tool: %s", toolName)
		tm.logAudit(toolName, user, params, "", err, 0, false)
		return "", err
	}

	// 检查是否在允许的工具列表中
	if len(tm.config.AllowedTools) > 0 {
		allowed := false
		for _, name := range tm.config.AllowedTools {
			if name == toolName {
				allowed = true
				break
			}
		}
		if !allowed {
			err := fmt.Errorf("tool not allowed: %s", toolName)
			tm.logAudit(toolName, user, params, "", err, 0, false)
			return "", err
		}
	}

	// 设置工具上下文
	toolCtx := &tools.ToolContext{
		User: user,
	}
	registered.tool.SetContext(toolCtx)

	// 执行工具
	result, err := registered.tool.Execute(ctx, params)
	duration := time.Since(startTime)

	// 记录审计日志
	tm.logAudit(toolName, user, params, result, err, duration, err == nil)

	return result, err
}

// hasPermission 检查用户是否有权限执行工具
func (tm *ToolManager) hasPermission(registered *registeredTool, user *model.User) bool {
	// 如果没有设置角色限制，则允许所有用户
	if len(registered.allowedRoles) == 0 {
		return true
	}

	// 检查用户角色
	if user == nil || user.Roles == nil {
		return false
	}

	for _, role := range user.Roles {
		if registered.allowedRoles[role.Name] {
			return true
		}
	}

	return false
}

// GetToolDefinitions 获取工具定义列表（用于AI模型调用）
func (tm *ToolManager) GetToolDefinitions() []models.ToolDefinition {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	definitions := make([]models.ToolDefinition, 0, len(tm.tools))

	for _, registered := range tm.tools {
		def := models.ToolDefinition{
			Type: "function",
			Function: models.FunctionDefinition{
				Name:        registered.tool.Name(),
				Description: registered.tool.Description(),
				Parameters:  registered.tool.Parameters(),
			},
		}
		definitions = append(definitions, def)
	}

	return definitions
}

// ListTools 列出所有工具
func (tm *ToolManager) ListTools() []ToolInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	infos := make([]ToolInfo, 0, len(tm.tools))

	for name, registered := range tm.tools {
		roles := make([]string, 0, len(registered.allowedRoles))
		for role := range registered.allowedRoles {
			roles = append(roles, role)
		}

		infos = append(infos, ToolInfo{
			Name:         name,
			Description:  registered.tool.Description(),
			AllowedRoles: roles,
		})
	}

	return infos
}

// ToolCount 获取工具数量
func (tm *ToolManager) ToolCount() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.tools)
}

// logAudit 记录审计日志
func (tm *ToolManager) logAudit(toolName string, user *model.User, params map[string]interface{}, result string, err error, duration time.Duration, success bool) {
	if !tm.auditLogger.enabled {
		return
	}

	username := ""
	userID := uint(0)
	if user != nil {
		username = user.Username
		userID = user.ID
	}

	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	entry := AuditEntry{
		Timestamp: time.Now(),
		ToolName:  toolName,
		UserID:    userID,
		Username:  username,
		Params:    params,
		Result:    result,
		Error:     errorMsg,
		Duration:  duration,
		Success:   success,
	}

	tm.auditLogger.Add(entry)
}

// GetAuditLogs 获取审计日志
func (tm *ToolManager) GetAuditLogs(limit int) []AuditEntry {
	return tm.auditLogger.Get(limit)
}

// AuditLogger 方法

// Add 添加审计日志条目
func (al *AuditLogger) Add(entry AuditEntry) {
	al.mu.Lock()
	defer al.mu.Unlock()

	// 如果超过最大数量，删除最旧的条目
	if al.maxSize > 0 && len(al.entries) >= al.maxSize {
		al.entries = al.entries[1:]
	}

	al.entries = append(al.entries, entry)
}

// Get 获取审计日志
func (al *AuditLogger) Get(limit int) []AuditEntry {
	al.mu.RLock()
	defer al.mu.RUnlock()

	if limit <= 0 || limit > len(al.entries) {
		limit = len(al.entries)
	}

	// 返回最新的limit条记录
	start := len(al.entries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]AuditEntry, limit)
	copy(result, al.entries[start:])
	return result
}

// Clear 清空审计日志
func (al *AuditLogger) Clear() {
	al.mu.Lock()
	defer al.mu.Unlock()
	al.entries = make([]AuditEntry, 0)
}

// Count 获取审计日志数量
func (al *AuditLogger) Count() int {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return len(al.entries)
}

// ParseToolCallParams 解析工具调用参数
func ParseToolCallParams(arguments json.RawMessage) (map[string]interface{}, error) {
	// 首先尝试直接解析为 map
	var params map[string]interface{}
	if err := json.Unmarshal(arguments, &params); err == nil {
		return params, nil
	}

	// 如果失败，尝试解析为字符串，然后再解析为 map
	var argString string
	if err := json.Unmarshal(arguments, &argString); err != nil {
		return nil, fmt.Errorf("failed to parse tool call arguments: %w", err)
	}

	// 将字符串解析为 map
	if err := json.Unmarshal([]byte(argString), &params); err != nil {
		return nil, fmt.Errorf("failed to parse tool call arguments string: %w", err)
	}

	return params, nil
}
