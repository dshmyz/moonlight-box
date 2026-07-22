package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/dshmyz/moonlight-box/internal/ai/models"
	"github.com/dshmyz/moonlight-box/internal/ai/tools"
	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
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
	SessionID string         `json:"session_id"`
	Message   string         `json:"message"`
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

	// 重新获取会话（因为AddMessage修改的是内存中的session，而session是深拷贝）
	session = s.sessionManager.GetSession(session.ID)
	if session == nil {
		return nil, fmt.Errorf("获取会话失败")
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
		log.Printf("[AI] 第%d轮调用，消息数: %d", i+1, len(req.Messages))

		// 调用AI
		resp, err := s.client.Call(ctx, req)
		if err != nil {
			log.Printf("[AI] AI调用失败: %v", err)
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
			log.Printf("[AI] AI返回空响应")
			return "", toolCallsInfo, &totalUsage, fmt.Errorf("AI返回空响应")
		}

		choice := resp.Choices[0]
		message := choice.Message

		// 检查是否有工具调用
		if len(message.ToolCalls) == 0 {
			// 没有工具调用，返回最终响应
			log.Printf("[AI] 没有工具调用，返回最终响应，内容长度: %d", len(message.Content))
			return message.Content, toolCallsInfo, &totalUsage, nil
		}

		log.Printf("[AI] 检测到%d个工具调用", len(message.ToolCalls))

		// 处理工具调用
		assistantMsg := models.Message{
			Role:      "assistant",
			Content:   message.Content,
			ToolCalls: message.ToolCalls,
		}
		req.Messages = append(req.Messages, assistantMsg)

		// 执行每个工具调用
		for _, toolCall := range message.ToolCalls {
			log.Printf("[AI] 执行工具: %s", toolCall.Function.Name)

			// 解析参数
			params, err := ParseToolCallParams(toolCall.Function.Arguments)
			if err != nil {
				log.Printf("[AI] 解析工具参数失败: %v, 参数: %s", err, string(toolCall.Function.Arguments))
				return "", toolCallsInfo, &totalUsage, fmt.Errorf("解析工具参数失败: %w", err)
			}

			toolCallInfo := ToolCallInfo{
				Name:   toolCall.Function.Name,
				Params: params,
			}

			// 执行工具
			result, err := s.toolManager.ExecuteTool(ctx, toolCall.Function.Name, params, user)
			if err != nil {
				log.Printf("[AI] 工具执行失败: %v", err)
				toolCallInfo.Error = err.Error()
				result = fmt.Sprintf("工具执行失败: %s", err.Error())
			} else {
				log.Printf("[AI] 工具执行成功，结果长度: %d", len(result))
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

	log.Printf("[AI] 工具调用次数超过限制")
	return "", toolCallsInfo, &totalUsage, fmt.Errorf("工具调用次数超过限制")
}

// buildChatRequest 构建聊天请求
func (s *AIService) buildChatRequest(session *Session, user *model.User) *models.ChatRequest {
	messages := make([]models.Message, 0, len(session.Messages)+1)

	// 检查是否有可用工具
	hasTools := s.config.Tools.Enabled && s.toolManager.ToolCount() > 0

	// 如果有工具可用，使用系统消息；否则不使用系统消息（避免模型不调用工具）
	if hasTools {
		systemPrompt := s.buildSystemPrompt(user)
		messages = append(messages, models.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for _, msg := range session.Messages {
		messages = append(messages, models.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// 构建请求
	req := &models.ChatRequest{
		Model:       s.config.Model,
		Messages:    messages,
		Temperature: &s.config.Temperature,
	}

	if s.config.MaxTokens > 0 {
		req.MaxTokens = &s.config.MaxTokens
	}

	// 添加工具
	if hasTools {
		req.Tools = s.toolManager.GetToolDefinitions()
		req.ToolChoice = "auto"
	}

	return req
}

// buildSystemPrompt 构建系统提示词
func (s *AIService) buildSystemPrompt(user *model.User) string {
	var sb strings.Builder

	sb.WriteString("你是 Moonlight Registry 的AI助手。当用户的请求可以用工具完成时，必须调用相应的工具，不要直接回复。只有当用户问好或闲聊时，才直接回复。\n\n")

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
	sb.WriteString("- 回复使用中文，简洁明了\n")

	// 安全相关提示词：引导 AI 在安全分析后主动建议生成阻断规则
	sb.WriteString("\n## 安全策略建议\n")
	sb.WriteString("- 当使用 security_analysis 工具发现 critical 或 high 级别漏洞，且漏洞存在 FixedVersion 时，")
	sb.WriteString("应主动建议用户调用 block_rule_generator 工具生成阻断规则草案\n")
	sb.WriteString("- 用户描述阻断需求（如\"阻断所有 log4j 1.x\"）时，调用 block_rule_generator 工具的 description 源生成规则草案\n")
	sb.WriteString("- block_rule_generator 只生成 preview 草案，不自动写入数据库。需告知用户在管理后台确认后手动创建\n")
	sb.WriteString("- 生成 range 规则时，版本约束用 semver 格式（如 <2.17.1、>=1.0.0 <2.0.0）\n")
	sb.WriteString("- 当用户想审查或精简现有阻断规则时，调用 block_rule_optimizer 工具（operation=analyze）获取优化建议\n")
	sb.WriteString("- block_rule_optimizer 检测三类问题：over_broad（过宽规则）、stale（过期规则）、redundant（冗余规则），只读分析不修改规则\n")

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

// DeleteSession 删除会话
func (s *AIService) DeleteSession(sessionID string) {
	s.sessionManager.DeleteSession(sessionID)
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

// StreamChat 流式聊天
func (s *AIService) StreamChat(ctx context.Context, userID uint, sessionID string, message string) (<-chan *StreamChatChunk, error) {
	// 检查限流
	if !s.rateLimiter.Allow(userID) {
		return nil, fmt.Errorf("请求过于频繁")
	}

	// 创建输出channel
	output := make(chan *StreamChatChunk, 100)

	// 获取或创建会话
	session := s.sessionManager.GetOrCreateSession(userID, sessionID)

	// 对用户消息进行脱敏
	sanitizedMessage := s.sanitizer.Sanitize(message)

	// 添加用户消息到会话
	userMsg := models.Message{
		Role:    "user",
		Content: sanitizedMessage,
	}
	if err := s.sessionManager.AddMessage(session.ID, userMsg); err != nil {
		return nil, fmt.Errorf("添加消息失败: %w", err)
	}

	// 重新获取会话
	session = s.sessionManager.GetSession(session.ID)
	if session == nil {
		return nil, fmt.Errorf("获取会话失败")
	}

	// 获取用户信息
	user, err := s.getUser(userID)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	// 构建AI请求
	req := s.buildChatRequest(session, user)

	// 启动goroutine处理流式响应
	go func() {
		defer close(output)

		// 设置流式标志
		req.Stream = true

		// 调用流式AI
		stream, err := s.client.Stream(ctx, req)
		if err != nil {
			output <- &StreamChatChunk{Error: err}
			return
		}

		var fullContent strings.Builder
		var currentToolCall *models.ToolCall
		var currentToolArgs strings.Builder
		var toolCalls []models.ToolCall

		for chunk := range stream {
			if chunk.Error != nil {
				output <- &StreamChatChunk{Error: chunk.Error}
				return
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]

			// 处理工具调用
			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					if tc.ID != "" {
						// 新的工具调用开始
						if currentToolCall != nil {
							// 保存上一个工具调用的参数
							currentToolCall.Function.Arguments = json.RawMessage(currentToolArgs.String())
							toolCalls = append(toolCalls, *currentToolCall)
						}
						currentToolCall = &models.ToolCall{
							ID:   tc.ID,
							Type: tc.Type,
							Function: models.FunctionCall{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						}
						currentToolArgs.Reset()
						// 写入参数片段（解包引号）
						if len(tc.Function.Arguments) > 0 {
							writeToolArgs(&currentToolArgs, tc.Function.Arguments)
						}
					} else if currentToolCall != nil {
						// 追加参数片段（解包引号）
						if len(tc.Function.Arguments) > 0 {
							writeToolArgs(&currentToolArgs, tc.Function.Arguments)
						}
					}
				}
				continue
			}

			// 处理文本内容（实时流式输出）
			if choice.Delta.Content != "" {
				fullContent.WriteString(choice.Delta.Content)
				output <- &StreamChatChunk{
					SessionID: session.ID,
					Content:   choice.Delta.Content,
					Done:      false,
				}
			}

			// 检查是否完成
			if choice.FinishReason == "stop" || choice.FinishReason == "tool_calls" {
				// 保存最后一个工具调用
				if currentToolCall != nil {
					currentToolCall.Function.Arguments = json.RawMessage(currentToolArgs.String())
					toolCalls = append(toolCalls, *currentToolCall)
					currentToolCall = nil
				}

				// 如果有工具调用，执行工具
				if len(toolCalls) > 0 {
					// 先发送工具调用通知，让用户知道正在调用工具
					for _, tc := range toolCalls {
						output <- &StreamChatChunk{
							SessionID: session.ID,
							Content:   fmt.Sprintf("\n[正在调用工具: %s]\n", tc.Function.Name),
							Done:      false,
						}
					}

					// 执行工具调用
					var toolResults []ToolCallResultInfo
					for _, tc := range toolCalls {
						// 解析参数
						var params map[string]interface{}
						args := tc.Function.Arguments

						// 清理多次转义的 JSON
						cleanedArgs := cleanJSONArgs(args)

						// 尝试解析
						if err := json.Unmarshal(cleanedArgs, &params); err != nil {
							toolResults = append(toolResults, ToolCallResultInfo{
								Name:  tc.Function.Name,
								Error: fmt.Sprintf("参数解析失败: %v", err),
							})
							continue
						}

						// 执行工具
						result, err := s.toolManager.ExecuteTool(ctx, tc.Function.Name, params, user)

						if err != nil {
							toolResults = append(toolResults, ToolCallResultInfo{
								Name:   tc.Function.Name,
								Params: params,
								Error:  err.Error(),
							})
						} else {
							toolResults = append(toolResults, ToolCallResultInfo{
								Name:   tc.Function.Name,
								Params: params,
								Result: result,
							})
						}
					}

					// 发送工具调用结果
					for _, result := range toolResults {
						output <- &StreamChatChunk{
							SessionID: session.ID,
							ToolCall:  &result,
							Done:      false,
						}
					}

					// 将工具调用结果添加到请求中，继续流式调用
					// 构建助手消息，包含工具调用
					assistantMsg := models.Message{
						Role:    "assistant",
						Content: fullContent.String(),
					}

					// 添加工具调用信息
					if len(toolCalls) > 0 {
						assistantMsg.ToolCalls = make([]models.ToolCall, len(toolCalls))
						for i, tc := range toolCalls {
							assistantMsg.ToolCalls[i] = tc
						}
					}
					req.Messages = append(req.Messages, assistantMsg)

					// 添加工具响应消息，使用正确的工具调用ID
					for i, result := range toolResults {
						toolCallID := ""
						if i < len(toolCalls) {
							toolCallID = toolCalls[i].ID
						}
						req.Messages = append(req.Messages, models.Message{
							Role:       "tool",
							Content:    result.Result,
							ToolCallID: toolCallID,
						})
					}

					// 重置状态，继续流式调用
					fullContent.Reset()
					toolCalls = nil
					currentToolCall = nil
					currentToolArgs.Reset()

					// 再次调用流式AI获取最终回复
					stream, err = s.client.Stream(ctx, req)
					if err != nil {
						output <- &StreamChatChunk{Error: err}
						return
					}

					// 继续处理流式响应
					continue
				}

				// 添加助手消息到会话
				assistantMsg := models.Message{
					Role:    "assistant",
					Content: fullContent.String(),
				}
				s.sessionManager.AddMessage(session.ID, assistantMsg)

				// 发送完成信号
				output <- &StreamChatChunk{
					SessionID: session.ID,
					Content:   "",
					Done:      true,
				}
				return
			}
		}
	}()

	return output, nil
}

// ToolCallResultInfo 工具调用结果信息
type ToolCallResultInfo struct {
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params"`
	Result string                 `json:"result"`
	Error  string                 `json:"error,omitempty"`
}

// StreamChatChunk 流式聊天块
type StreamChatChunk struct {
	SessionID string              `json:"session_id"`
	Content   string              `json:"content"`
	ToolCall  *ToolCallResultInfo `json:"tool_call,omitempty"`
	Done      bool                `json:"done"`
	Error     error               `json:"-"`
}

// writeToolArgs 写入工具参数片段，自动解包引号
func writeToolArgs(builder *strings.Builder, chunk json.RawMessage) {
	// 尝试解包为字符串（流式模式下每个 chunk 可能被引号包裹）
	var str string
	if err := json.Unmarshal(chunk, &str); err == nil {
		builder.WriteString(str)
		return
	}
	// 如果解包失败，直接写入原始数据
	builder.Write(chunk)
}

// cleanJSONArgs 清理多次转义的 JSON 参数
func cleanJSONArgs(args json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return args
	}

	// 最多尝试解包 5 次，处理多层转义
	for i := 0; i < 5; i++ {
		trimmed := strings.TrimSpace(string(args))
		if len(trimmed) == 0 {
			return args
		}

		// 如果已经是 JSON 对象或数组，验证有效性后返回
		if trimmed[0] == '{' || trimmed[0] == '[' {
			var tmp interface{}
			if err := json.Unmarshal(args, &tmp); err == nil {
				return args
			}
		}

		// 如果是 JSON 字符串，尝试解包
		if trimmed[0] == '"' {
			var str string
			var unqErr error
			if unqErr = json.Unmarshal(args, &str); unqErr == nil {
				args = json.RawMessage(str)
				continue
			}
		}

		// 处理非标准格式："""{...}""" 或类似的多重引号
		if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `"`) {
			// 移除所有包裹的引号
			cleaned := strings.Trim(trimmed, `"`)
			// 移除多余的转义
			cleaned = strings.ReplaceAll(cleaned, `\"`, `"`)
			cleaned = strings.ReplaceAll(cleaned, `\\`, `\`)
			cleaned = strings.TrimSpace(cleaned)

			if len(cleaned) > 0 && (cleaned[0] == '{' || cleaned[0] == '[') {
				var tmp interface{}
				if err := json.Unmarshal([]byte(cleaned), &tmp); err == nil {
					return json.RawMessage(cleaned)
				}
			}
		}

		break
	}

	return args
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
	SessionCount  int         `json:"session_count"`
	ToolCount     int         `json:"tool_count"`
	CacheStats    *CacheStats `json:"cache_stats,omitempty"`
	AuditLogCount int         `json:"audit_log_count"`
}
