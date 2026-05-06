package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/moonlight-box/registry/internal/ai/models"
	"github.com/moonlight-box/registry/internal/ai/tools"
	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"gorm.io/gorm"
)

// AIService AI服务核心
type AIService struct {
	config         *config.AIConfig
	db             *gorm.DB
	client         *AIClient
	sessionManager *SessionManager
	rateLimiter    *RateLimiter
	sanitizer      *Sanitizer
	cache          *ResponseCache
	toolManager    *ToolManager
	auditRepo      *repository.AuditRepository

	stopChan chan struct{}
	stopOnce sync.Once
}

// NewAIService 创建AI服务
func NewAIService(cfg *config.AIConfig, db *gorm.DB, auditRepo *repository.AuditRepository) *AIService {
	// 创建客户端
	client := NewAIClient(cfg)

	// 创建会话管理器
	sessionManager := NewSessionManager(&cfg.Session)

	// 创建限流器
	rateLimiter := NewRateLimiter(&cfg.RateLimit)

	// 创建脱敏器
	sanitizer := NewSanitizer(DefaultSanitizerConfig())

	// 创建缓存
	var cache *ResponseCache
	if cfg.Cache.Enabled {
		cache = NewResponseCache(&cfg.Cache)
	}

	// 创建工具管理器
	toolManager := NewToolManager(&cfg.Tools)

	service := &AIService{
		config:         cfg,
		db:             db,
		client:         client,
		sessionManager: sessionManager,
		rateLimiter:    rateLimiter,
		sanitizer:      sanitizer,
		cache:          cache,
		toolManager:    toolManager,
		auditRepo:      auditRepo,
		stopChan:       make(chan struct{}),
	}

	return service
}

// ChatRequest 聊天请求
type ChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	ToolCalls []ToolCallInfo `json:"tool_calls,omitempty"`
	Usage     *UsageInfo     `json:"usage,omitempty"`
}

// ToolCallInfo 工具调用信息
type ToolCallInfo struct {
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params,omitempty"`
	Result string                 `json:"result,omitempty"`
	Error  string                 `json:"error,omitempty"`
}

// UsageInfo 使用量信息
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Chat 处理聊天请求
func (s *AIService) Chat(ctx context.Context, userID uint, sessionID string, message string) (*ChatResponse, error) {
	// 检查限流
	if !s.rateLimiter.Allow(userID) {
		status := s.rateLimiter.GetStatus(userID)
		return nil, fmt.Errorf("请求过于频繁，请稍后再试。当前限制: 每分钟%d次，每天%d次",
			status.MinuteLimit, status.DayLimit)
	}

	// 获取或创建会话
	session := s.sessionManager.GetOrCreateSession(userID, sessionID)

	// 对用户消息进行脱敏
	sanitizedMessage := s.sanitizer.Sanitize(message)

	// 检查缓存
	if s.cache != nil {
		if cached, ok := s.cache.Get(sanitizedMessage); ok {
			// 记录使用量（缓存命中也计数）
			s.rateLimiter.Record(userID, 0)
			return &ChatResponse{
				SessionID: session.ID,
				Message:   cached,
			}, nil
		}
	}

	// 添加用户消息到会话
	userMsg := models.Message{
		Role:    "user",
		Content: sanitizedMessage,
	}
	if err := s.sessionManager.AddMessage(session.ID, userMsg); err != nil {
		return nil, fmt.Errorf("添加消息失败: %w", err)
	}

	// 获取用户信息
	user, err := s.getUser(userID)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	// 构建AI请求
	req := s.buildChatRequest(session, user)

	// 调用AI并处理工具调用循环
	response, toolCalls, usage, err := s.chatWithToolLoop(ctx, req, user)
	if err != nil {
		return nil, err
	}

	// 对响应进行脱敏
	sanitizedResponse := s.sanitizer.Sanitize(response)

	// 添加助手消息到会话
	assistantMsg := models.Message{
		Role:    "assistant",
		Content: sanitizedResponse,
	}
	if err := s.sessionManager.AddMessage(session.ID, assistantMsg); err != nil {
		return nil, fmt.Errorf("添加消息失败: %w", err)
	}

	// 记录使用量
	if usage != nil {
		s.rateLimiter.Record(userID, usage.TotalTokens)
	}

	// 缓存响应
	if s.cache != nil && len(toolCalls) == 0 {
		s.cache.Set(sanitizedMessage, sanitizedResponse)
	}

	return &ChatResponse{
		SessionID: session.ID,
		Message:   sanitizedResponse,
		ToolCalls: toolCalls,
		Usage:     usage,
	}, nil
}

// chatWithToolLoop 执行聊天并处理工具调用循环
func (s *AIService) chatWithToolLoop(ctx context.Context, req *models.ChatRequest, user *model.User) (string, []ToolCallInfo, *UsageInfo, error) {
	const maxIterations = 10 // 最多10轮工具调用

	var toolCallsInfo []ToolCallInfo
	var totalUsage UsageInfo

	for i := 0; i < maxIterations; i++ {
		// 调用AI
		resp, err := s.client.Call(ctx, req)
		if err != nil {
			return "", nil, nil, fmt.Errorf("AI调用失败: %w", err)
		}

		// 统计使用量
		if resp.Usage != nil {
			totalUsage.PromptTokens += resp.Usage.PromptTokens
			totalUsage.CompletionTokens += resp.Usage.CompletionTokens
			totalUsage.TotalTokens += resp.Usage.TotalTokens
		}

		// 检查响应
		if len(resp.Choices) == 0 {
			return "", toolCallsInfo, &totalUsage, fmt.Errorf("AI返回空响应")
		}

		choice := resp.Choices[0]
		message := choice.Message

		// 检查是否有工具调用
		if len(message.ToolCalls) == 0 {
			// 没有工具调用，返回最终响应
			return message.Content, toolCallsInfo, &totalUsage, nil
		}

		// 处理工具调用
		assistantMsg := models.Message{
			Role:      "assistant",
			Content:   message.Content,
			ToolCalls: message.ToolCalls,
		}
		req.Messages = append(req.Messages, assistantMsg)

		// 执行每个工具调用
		for _, toolCall := range message.ToolCalls {
			// 解析参数
			params, err := ParseToolCallParams(toolCall.Function.Arguments)
			if err != nil {
				return "", toolCallsInfo, &totalUsage, fmt.Errorf("解析工具参数失败: %w", err)
			}

			toolCallInfo := ToolCallInfo{
				Name:   toolCall.Function.Name,
				Params: params,
			}

			// 执行工具
			result, err := s.toolManager.ExecuteTool(ctx, toolCall.Function.Name, params, user)
			if err != nil {
				toolCallInfo.Error = err.Error()
				result = fmt.Sprintf("工具执行失败: %s", err.Error())
			} else {
				// 对工具结果进行脱敏
				result = s.sanitizer.SanitizeToolResult(toolCall.Function.Name, result)
				toolCallInfo.Result = result
			}

			toolCallsInfo = append(toolCallsInfo, toolCallInfo)

			// 添加工具结果到消息
			toolResultMsg := models.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			}
			req.Messages = append(req.Messages, toolResultMsg)
		}
	}

	return "", toolCallsInfo, &totalUsage, fmt.Errorf("工具调用次数超过限制")
}

// buildChatRequest 构建聊天请求
func (s *AIService) buildChatRequest(session *Session, user *model.User) *models.ChatRequest {
	// 复制会话消息
	messages := make([]models.Message, len(session.Messages))
	copy(messages, session.Messages)

	// 添加系统提示词
	systemPrompt := s.buildSystemPrompt(user)
	messages = append([]models.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}, messages...)

	// 构建请求
	req := &models.ChatRequest{
		Model:       s.config.Model,
		Messages:    messages,
		Temperature: &s.config.Temperature,
	}

	if s.config.MaxTokens > 0 {
		req.MaxTokens = &s.config.MaxTokens
	}

	// 添加工具定义
	if s.config.Tools.Enabled && s.toolManager.ToolCount() > 0 {
		req.Tools = s.toolManager.GetToolDefinitions()
	}

	return req
}

// buildSystemPrompt 构建系统提示词
func (s *AIService) buildSystemPrompt(user *model.User) string {
	var sb strings.Builder

	sb.WriteString("你是 Moonlight Registry 的AI助手，帮助用户管理和查询私有仓库。\n\n")

	sb.WriteString("## 你的能力\n")
	sb.WriteString("- 查询系统日志\n")
	sb.WriteString("- 查询数据库统计信息\n")
	sb.WriteString("- 查询包详细信息\n")
	sb.WriteString("- 执行安全分析\n")
	sb.WriteString("- 生成代码示例\n\n")

	sb.WriteString("## 用户信息\n")
	sb.WriteString(fmt.Sprintf("- 用户名: %s\n", user.Username))
	if len(user.Roles) > 0 {
		roles := make([]string, len(user.Roles))
		for i, role := range user.Roles {
			roles[i] = role.Name
		}
		sb.WriteString(fmt.Sprintf("- 角色: %s\n", strings.Join(roles, ", ")))
	}

	sb.WriteString("\n## 注意事项\n")
	sb.WriteString("- 使用工具查询信息时，请确保参数正确\n")
	sb.WriteString("- 如果不确定用户意图，请主动询问\n")
	sb.WriteString("- 对于敏感操作，请确认用户权限\n")
	sb.WriteString("- 回复使用中文，简洁明了\n")

	return sb.String()
}

// getUser 获取用户信息
func (s *AIService) getUser(userID uint) (*model.User, error) {
	var user model.User
	if err := s.db.Preload("Roles").First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// RegisterTool 注册工具
func (s *AIService) RegisterTool(tool tools.Tool, allowedRoles []string) {
	s.toolManager.RegisterTool(tool, allowedRoles)
}

// GetSession 获取会话
func (s *AIService) GetSession(sessionID string) *Session {
	return s.sessionManager.GetSession(sessionID)
}

// DeleteSession 删除会话
func (s *AIService) DeleteSession(sessionID string) {
	s.sessionManager.DeleteSession(sessionID)
}

// DeleteUserSessions 删除用户的所有会话
func (s *AIService) DeleteUserSessions(userID uint) {
	s.sessionManager.DeleteUserSessions(userID)
}

// ListTools 列出所有工具
func (s *AIService) ListTools() []ToolInfo {
	return s.toolManager.ListTools()
}

// GetRateLimitStatus 获取限流状态
func (s *AIService) GetRateLimitStatus(userID uint) *RateLimitStatus {
	return s.rateLimiter.GetStatus(userID)
}

// GetCacheStats 获取缓存统计
func (s *AIService) GetCacheStats() *CacheStats {
	if s.cache == nil {
		return nil
	}
	return s.cache.GetStats()
}

// GetAuditLogs 获取审计日志
func (s *AIService) GetAuditLogs(limit int) []AuditEntry {
	return s.toolManager.GetAuditLogs(limit)
}

// Stop 停止服务
func (s *AIService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopChan)

		// 停止各个组件
		s.sessionManager.Stop()
		s.rateLimiter.Stop()
		if s.cache != nil {
			s.cache.Stop()
		}
		s.client.Close()
	})
}

// SetToolContext 设置工具上下文
func (s *AIService) SetToolContext(ctx *tools.ToolContext) {
	// 这个方法用于设置工具执行时的上下文
	// 工具管理器会在执行工具时设置上下文
}

// StreamChat 流式聊天（预留接口）
func (s *AIService) StreamChat(ctx context.Context, userID uint, sessionID string, message string) (<-chan *StreamChatChunk, error) {
	// 检查限流
	if !s.rateLimiter.Allow(userID) {
		return nil, fmt.Errorf("请求过于频繁")
	}

	// 创建输出channel
	output := make(chan *StreamChatChunk, 100)

	// 启动goroutine处理流式响应
	go func() {
		defer close(output)

		// 简单实现：调用Chat方法，然后一次性返回
		// 完整实现需要使用client.Stream方法
		resp, err := s.Chat(ctx, userID, sessionID, message)
		if err != nil {
			output <- &StreamChatChunk{Error: err}
			return
		}

		output <- &StreamChatChunk{
			SessionID: resp.SessionID,
			Content:   resp.Message,
			Done:      true,
		}
	}()

	return output, nil
}

// StreamChatChunk 流式聊天块
type StreamChatChunk struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	Done      bool   `json:"done"`
	Error     error  `json:"-"`
}

// HealthCheck 健康检查
func (s *AIService) HealthCheck() error {
	// 检查数据库连接
	if s.db != nil {
		sqlDB, err := s.db.DB()
		if err != nil {
			return fmt.Errorf("数据库连接异常: %w", err)
		}
		if err := sqlDB.Ping(); err != nil {
			return fmt.Errorf("数据库连接异常: %w", err)
		}
	}

	return nil
}

// GetStats 获取服务统计
func (s *AIService) GetStats() *ServiceStats {
	return &ServiceStats{
		SessionCount:  s.sessionManager.GetSessionCount(),
		ToolCount:     s.toolManager.ToolCount(),
		CacheStats:    s.GetCacheStats(),
		AuditLogCount: s.toolManager.auditLogger.Count(),
	}
}

// ServiceStats 服务统计
type ServiceStats struct {
	SessionCount  int          `json:"session_count"`
	ToolCount     int          `json:"tool_count"`
	CacheStats    *CacheStats  `json:"cache_stats,omitempty"`
	AuditLogCount int          `json:"audit_log_count"`
}
