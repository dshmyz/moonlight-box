# AI集成设计文档

**版本：** 1.0  
**日期：** 2026-05-02  
**作者：** AI Assistant  
**状态：** 待审查

## 1. 概述

### 1.1 背景

Moonlight Registry 是一个企业级包管理仓库系统，支持 npm、Maven、PyPI、Go、NuGet 等多种包管理器。为了提升用户体验和运维效率，需要集成 AI 功能，为仓库管理员和开发者提供智能辅助能力。

### 1.2 目标

- 为仓库管理员提供问题排查、安全分析、运维监控等智能辅助
- 为开发者提供包查询、用法推荐、代码示例生成等服务
- 支持多种交互方式（Web 聊天、REST API、CLI 工具）
- 集成内网大模型，确保数据安全

### 1.3 范围

**包含：**
- AI 服务核心框架
- 工具调用系统
- 会话管理
- 流式响应
- 安全控制
- Web 聊天界面
- REST API 接口

**不包含：**
- CLI 工具实现（后续扩展）
- 自定义模型训练
- 多租户隔离

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                      用户交互层                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                 │
│  │ Web聊天  │  │ REST API │  │   CLI    │                 │
│  └──────────┘  └──────────┘  └──────────┘                 │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│                      AI服务层                                │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              AIService (核心服务)                     │  │
│  │  - 对话管理                                           │  │
│  │  - 工具调用编排                                       │  │
│  │  - 上下文构建                                         │  │
│  └──────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              ToolManager (工具管理器)                 │  │
│  │  - 工具注册与发现                                     │  │
│  │  - 权限控制                                           │  │
│  │  - 执行调度                                           │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│                      工具层                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ 日志查询工具 │  │ 数据库工具  │  │ 包信息工具  │        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ 安全分析工具 │  │ 代码生成工具│  │ 统计分析工具│        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│                      数据层                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                 │
│  │  数据库   │  │  日志文件 │  │  缓存    │                 │
│  └──────────┘  └──────────┘  └──────────┘                 │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│                    外部AI模型                                │
│            (内网大模型 - ChatGLM/Qwen等)                     │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 目录结构

```
internal/
├── ai/
│   ├── service.go           # AI核心服务
│   ├── client.go            # AI模型客户端
│   ├── tool_manager.go      # 工具管理器
│   ├── context_builder.go   # 上下文构建器
│   ├── session_manager.go   # 会话管理器
│   ├── rate_limiter.go      # 限流器
│   ├── sanitizer.go         # 数据脱敏
│   ├── cache.go             # 缓存管理
│   ├── tools/
│   │   ├── tool.go          # 工具接口定义
│   │   ├── log_query.go     # 日志查询工具
│   │   ├── db_query.go      # 数据库查询工具
│   │   ├── package_info.go  # 包信息查询工具
│   │   ├── security.go      # 安全分析工具
│   │   ├── code_gen.go      # 代码生成工具
│   │   └── stats.go         # 统计分析工具
│   └── models/
│       ├── request.go       # 请求模型
│       └── response.go      # 响应模型
├── handler/
│   └── ai_handler.go        # AI相关HTTP接口
└── config/
    └── config.go            # 添加AI配置
```

## 3. 核心组件设计

### 3.1 配置设计

```go
type AIConfig struct {
    Enabled      bool              `mapstructure:"enabled"`
    Provider     string            `mapstructure:"provider"`     // chatglm, qwen, custom
    BaseURL      string            `mapstructure:"base_url"`     // 内网AI服务地址
    APIKey       string            `mapstructure:"api_key"`      // API密钥
    Model        string            `mapstructure:"model"`        // 模型名称
    MaxTokens    int               `mapstructure:"max_tokens"`   // 最大token数
    Temperature  float64           `mapstructure:"temperature"`  // 温度参数
    Timeout      time.Duration     `mapstructure:"timeout"`      // 请求超时
    Tools        ToolsConfig       `mapstructure:"tools"`        // 工具配置
    RateLimit    RateLimitConfig   `mapstructure:"rate_limit"`   // 限流配置
    Cache        CacheConfig       `mapstructure:"cache"`        // 缓存配置
    Session      SessionConfig     `mapstructure:"session"`      // 会话配置
}

type ToolsConfig struct {
    Enabled           bool     `mapstructure:"enabled"`
    AllowedTools      []string `mapstructure:"allowed_tools"`
    MaxExecutionTime  int      `mapstructure:"max_execution_time"`
    EnableAuditLog    bool     `mapstructure:"enable_audit_log"`
}

type RateLimitConfig struct {
    RequestsPerMinute int `mapstructure:"requests_per_minute"`
    RequestsPerDay    int `mapstructure:"requests_per_day"`
    TokensPerDay      int `mapstructure:"tokens_per_day"`
}

type CacheConfig struct {
    Enabled  bool          `mapstructure:"enabled"`
    TTL      time.Duration `mapstructure:"ttl"`
    MaxSize  int           `mapstructure:"max_size"`
}

type SessionConfig struct {
    MaxAge      time.Duration `mapstructure:"max_age"`
    MaxMessages int           `mapstructure:"max_messages"`
}
```

### 3.2 工具接口设计

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

type ToolContext struct {
    User      *model.User
    DB        *gorm.DB
    Config    *config.Config
    AuditLog  *service.AuditService
}

type BaseTool struct {
    ctx *ToolContext
}

func (t *BaseTool) SetContext(ctx *ToolContext) {
    t.ctx = ctx
}
```

### 3.3 AI服务核心

```go
type AIService struct {
    config       *config.AIConfig
    client       *AIClient
    toolManager  *ToolManager
    sessionMgr   *SessionManager
    rateLimiter  *RateLimiter
    sanitizer    *DataSanitizer
    cache        *ResponseCache
    auditService *service.AuditService
    metrics      *AIMetrics
}

func (s *AIService) Chat(
    ctx context.Context,
    req *ChatRequest,
    user *model.User,
    session *Session,
) (*ChatResponse, error) {
    // 1. 限流检查
    if !s.rateLimiter.Allow(user.ID) {
        return nil, ErrRateLimitExceeded
    }
    
    // 2. 数据脱敏
    sanitizedMessage := s.sanitizer.Sanitize(req.Message)
    
    // 3. 检查缓存
    if s.cache != nil {
        if cached, ok := s.cache.Get(sanitizedMessage); ok {
            return &ChatResponse{Message: cached}, nil
        }
    }
    
    // 4. 构建请求
    messages := s.buildMessages(session, sanitizedMessage)
    tools := s.toolManager.GetToolDefinitions()
    
    // 5. 调用AI模型
    startTime := time.Now()
    resp, err := s.callWithRetry(ctx, ChatRequest{
        Model:    s.config.Model,
        Messages: messages,
        Tools:    tools,
    })
    
    if err != nil {
        s.metrics.RecordError("model_call_failed")
        return nil, err
    }
    
    // 6. 处理工具调用
    if resp.ToolCalls != nil && len(resp.ToolCalls) > 0 {
        toolResults := s.executeToolCalls(ctx, resp.ToolCalls, user)
        
        // 将工具结果返回给AI
        messages = append(messages,
            Message{Role: "assistant", ToolCalls: resp.ToolCalls},
            Message{Role: "tool", Content: toolResults},
        )
        
        // 再次调用AI生成最终回答
        resp, err = s.callWithRetry(ctx, ChatRequest{
            Model:    s.config.Model,
            Messages: messages,
        })
        if err != nil {
            return nil, err
        }
    }
    
    // 7. 记录指标
    s.metrics.RecordRequest(time.Since(startTime), resp.Usage.TotalTokens)
    s.rateLimiter.Record(user.ID, resp.Usage.TotalTokens)
    
    // 8. 更新会话
    s.sessionMgr.AddMessage(session.ID, Message{
        Role:    "user",
        Content: sanitizedMessage,
    })
    s.sessionMgr.AddMessage(session.ID, resp.Message)
    
    // 9. 缓存结果
    if s.cache != nil {
        s.cache.Set(sanitizedMessage, resp.Message.Content)
    }
    
    return &ChatResponse{
        SessionID: session.ID,
        Message:   resp.Message.Content,
        Timestamp: time.Now().Unix(),
    }, nil
}
```

### 3.4 工具管理器

```go
type ToolManager struct {
    tools       map[string]Tool
    permissions map[string][]string
    auditRepo   *repository.AuditRepository
    sanitizer   *DataSanitizer
    checker     *ToolSecurityChecker
}

func (tm *ToolManager) ExecuteTool(
    ctx context.Context,
    toolName string,
    params map[string]interface{},
    user *model.User,
) (string, error) {
    // 1. 检查工具是否存在
    tool, exists := tm.tools[toolName]
    if !exists {
        return "", fmt.Errorf("tool not found: %s", toolName)
    }
    
    // 2. 权限检查
    if !tm.hasPermission(user, toolName) {
        return "", fmt.Errorf("permission denied for tool: %s", toolName)
    }
    
    // 3. 参数安全检查
    if err := tm.checker.ValidateParams(toolName, params); err != nil {
        return "", fmt.Errorf("invalid parameters: %w", err)
    }
    
    // 4. 执行工具
    startTime := time.Now()
    result, err := tool.Execute(ctx, params)
    duration := time.Since(startTime)
    
    // 5. 结果脱敏
    result = tm.sanitizer.SanitizeToolResult(toolName, result)
    
    // 6. 记录审计日志
    if tm.auditRepo != nil {
        tm.recordAuditLog(user.ID, toolName, params, result, err, duration)
    }
    
    return result, err
}

func (tm *ToolManager) GetToolDefinitions() []ToolDefinition {
    var definitions []ToolDefinition
    for _, tool := range tm.tools {
        definitions = append(definitions, ToolDefinition{
            Type: "function",
            Function: FunctionDefinition{
                Name:        tool.Name(),
                Description: tool.Description(),
                Parameters:  tool.Parameters(),
            },
        })
    }
    return definitions
}
```

## 4. 工具实现

### 4.1 日志查询工具

**功能：** 查询系统日志，支持按时间、级别、关键词过滤

**参数：**
- `start_time`: 开始时间
- `end_time`: 结束时间
- `level`: 日志级别（debug/info/warn/error）
- `keyword`: 搜索关键词
- `source`: 日志来源（proxy/adapter/auth等）
- `limit`: 返回条数限制

**权限：** 需要管理员权限

**示例：**
```
用户：最近有哪些错误日志？
AI调用：query_logs(level="error", limit=20)
返回：找到15条错误日志...
```

### 4.2 数据库查询工具

**功能：** 查询仓库数据库，获取包信息、下载记录、安全扫描结果等

**查询类型：**
- `package_info`: 包信息查询
- `download_stats`: 下载统计
- `security_scan`: 安全扫描结果
- `dependencies`: 依赖关系
- `repository_info`: 仓库信息
- `block_rules`: 阻断规则
- `user_activity`: 用户活动

**权限：** 根据查询类型和用户角色控制

**示例：**
```
用户：查询lodash包的信息
AI调用：query_database(query_type="package_info", package_name="lodash")
返回：📦 lodash@4.17.21...
```

### 4.3 包信息查询工具

**功能：** 查询包的详细信息，包括版本历史、依赖关系、使用示例

**参数：**
- `package_name`: 包名称
- `package_type`: 包类型（npm/maven/pypi等）
- `version`: 指定版本
- `include_dependencies`: 是否包含依赖信息
- `include_readme`: 是否包含README

**权限：** 所有用户

### 4.4 安全分析工具

**功能：** 分析包的安全问题，包括漏洞详情、影响范围、修复建议

**分析类型：**
- `vulnerabilities`: 漏洞列表
- `impact`: 影响分析
- `fix_suggestion`: 修复建议
- `all`: 完整分析

**权限：** 需要安全查看权限

### 4.5 代码生成工具

**功能：** 生成包的使用示例代码

**参数：**
- `package_name`: 包名称
- `package_type`: 包类型
- `language`: 编程语言
- `scenario`: 使用场景（basic/advanced/testing/production）
- `framework`: 框架名称

**权限：** 所有用户

## 5. API接口设计

### 5.1 聊天接口

**POST /api/v1/ai/chat**

请求：
```json
{
  "session_id": "optional-session-id",
  "message": "查询lodash包的信息",
  "stream": false
}
```

响应：
```json
{
  "session_id": "session-123",
  "message": "📦 lodash@4.17.21...",
  "tool_calls": [
    {
      "name": "query_package_info",
      "params": {"package_name": "lodash"},
      "result": "...",
      "duration_ms": 123
    }
  ],
  "timestamp": 1234567890
}
```

### 5.2 流式聊天接口

**POST /api/v1/ai/chat/stream**

响应（Server-Sent Events）：
```
event: message
data: {"content": "📦"}

event: message
data: {"content": " lodash"}

event: tool_call
data: {"tool": "query_package_info", "params": {...}}

event: message
data: {"content": "@4.17.21..."}

event: done
data: {"session_id": "session-123", "timestamp": 1234567890}
```

### 5.3 工具列表接口

**GET /api/v1/ai/tools**

响应：
```json
{
  "tools": [
    {
      "name": "query_logs",
      "description": "查询系统日志",
      "parameters": {...},
      "category": "troubleshoot"
    }
  ]
}
```

### 5.4 反馈接口

**POST /api/v1/ai/feedback**

请求：
```json
{
  "session_id": "session-123",
  "message_id": "msg-456",
  "rating": 5,
  "comment": "非常有帮助"
}
```

## 6. 前端设计

### 6.1 组件结构

```
src/components/ai/
├── AIAssistant.vue          # AI助手主组件
├── ChatWindow.vue           # 聊天窗口
├── MessageList.vue          # 消息列表
├── MessageItem.vue          # 单条消息
├── InputBox.vue             # 输入框
├── ToolCallDisplay.vue      # 工具调用展示
├── SuggestionList.vue       # 建议列表
├── FeedbackModal.vue        # 反馈弹窗
└── composables/
    ├── useChat.ts           # 聊天逻辑
    ├── useStream.ts         # 流式响应
    └── useTools.ts          # 工具相关
```

### 6.2 核心功能

- 流式消息显示
- 工具调用可视化
- Markdown渲染
- 代码高亮
- 快捷命令
- 历史记录
- 反馈机制

## 7. 安全设计

### 7.1 数据脱敏

自动识别和脱敏敏感信息：
- 密码
- API密钥
- Token
- IP地址
- 邮箱地址

### 7.2 权限控制

- 基于角色的工具访问控制
- 管理员工具需要管理员权限
- 敏感操作需要二次确认

### 7.3 注入防护

- SQL注入检测
- 命令注入检测
- 参数验证

### 7.4 限流保护

- 每分钟请求限制
- 每日请求限制
- Token使用限制

## 8. 性能优化

### 8.1 缓存策略

- 相似问题缓存
- 工具结果缓存
- TTL可配置

### 8.2 流式响应

- 减少用户等待时间
- 实时显示AI生成内容
- 工具调用状态实时更新

### 8.3 上下文管理

- 自动截断过长对话
- 历史对话摘要
- Token计数优化

## 9. 监控和可观测性

### 9.1 Prometheus指标

- `ai_requests_total`: 请求总数
- `ai_request_duration_seconds`: 请求耗时
- `ai_tool_calls_total`: 工具调用次数
- `ai_tokens_used_total`: Token使用量
- `ai_cache_hits_total`: 缓存命中数

### 9.2 告警规则

- AI服务错误率过高
- AI服务响应缓慢
- Token使用量异常

### 9.3 审计日志

记录所有AI交互：
- 用户ID
- 会话ID
- 消息内容
- 工具调用
- 时间戳

## 10. 实施计划

### 10.1 阶段划分

**阶段一：基础框架搭建（2-3天）**
- 添加AI配置
- 实现AI服务核心
- 实现工具管理器
- 实现基础HTTP接口

**阶段二：核心工具开发（3-4天）**
- 日志查询工具
- 数据库查询工具
- 包信息查询工具
- 安全分析工具
- 代码生成工具

**阶段三：增强功能开发（2-3天）**
- 会话管理
- 流式响应
- 限流和配额
- 数据脱敏
- 缓存机制

**阶段四：前端开发（3-4天）**
- 聊天界面组件
- 消息展示组件
- 流式响应处理
- 建议和快捷命令
- 反馈机制

**阶段五：测试和优化（2-3天）**
- 单元测试
- 集成测试
- 性能测试
- 安全测试
- 用户验收测试

**总计：12-17天**

### 10.2 依赖项

- Go 1.26+
- Vue 3 + TypeScript
- 内网大模型服务（ChatGLM/Qwen等）
- PostgreSQL/SQLite

### 10.3 风险和缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| AI模型不可用 | 高 | 实现重试机制、降级方案 |
| 响应速度慢 | 中 | 流式响应、缓存优化 |
| 敏感信息泄露 | 高 | 数据脱敏、权限控制 |
| 工具调用失败 | 中 | 错误处理、重试机制 |
| Token成本高 | 中 | 缓存、限流、上下文优化 |

## 11. 测试策略

### 11.1 单元测试

- AI服务核心逻辑测试
- 工具执行测试
- 数据脱敏测试
- 限流测试

### 11.2 集成测试

- 端到端聊天流程测试
- 工具调用集成测试
- 权限控制测试

### 11.3 性能测试

- 并发请求测试
- 响应时间测试
- 内存使用测试

### 11.4 安全测试

- SQL注入测试
- 权限绕过测试
- 敏感信息泄露测试

## 12. 部署方案

### 12.1 配置示例

```yaml
ai:
  enabled: true
  provider: "chatglm"
  base_url: "http://192.168.1.100:8000/v1"
  model: "chatglm3-6b"
  max_tokens: 2048
  temperature: 0.7
  timeout: 30s
  
  tools:
    enabled: true
    allowed_tools:
      - query_logs
      - query_database
      - query_package_info
      - analyze_security
      - generate_demo_code
    max_execution_time: 10
    enable_audit_log: true
  
  rate_limit:
    requests_per_minute: 20
    requests_per_day: 500
    tokens_per_day: 100000
  
  cache:
    enabled: true
    ttl: 1h
    max_size: 1000
  
  session:
    max_age: 24h
    max_messages: 50
```

### 12.2 Docker部署

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o registry ./cmd/registry

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/registry .
COPY --from=builder /app/configs ./configs
EXPOSE 8080
CMD ["./registry", "-config", "configs/config.yaml"]
```

### 12.3 Kubernetes部署

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: moonlight-registry
spec:
  replicas: 3
  selector:
    matchLabels:
      app: moonlight-registry
  template:
    metadata:
      labels:
        app: moonlight-registry
    spec:
      containers:
      - name: registry
        image: moonlight/registry:latest
        ports:
        - containerPort: 8080
        env:
        - name: MOONLIGHT_AI_ENABLED
          value: "true"
        - name: MOONLIGHT_AI_BASE_URL
          valueFrom:
            secretKeyRef:
              name: ai-config
              key: base-url
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
```

## 13. 后续扩展

### 13.1 短期扩展（1-3个月）

- CLI工具实现
- 更多工具类型（网络诊断、性能分析等）
- 对话历史导出
- 多语言支持

### 13.2 中期扩展（3-6个月）

- 自定义工具开发框架
- AI模型微调
- 多租户隔离
- 高级分析报告

### 13.3 长期扩展（6-12个月）

- 多AI模型支持
- AI训练数据收集
- 智能推荐系统
- 自动化运维

## 14. 参考资料

- OpenAI Function Calling API 文档
- Anthropic Claude API 文档
- ChatGLM API 文档
- Qwen API 文档
- MCP (Model Context Protocol) 规范

## 15. 附录

### 15.1 术语表

- **Tool Calling**: AI模型调用外部工具的能力
- **Session**: 用户与AI的对话会话
- **Stream**: 流式响应，逐步返回AI生成的内容
- **Token**: AI模型的文本处理单位
- **Rate Limiting**: 限流，防止服务被滥用

### 15.2 变更历史

| 版本 | 日期 | 变更内容 | 作者 |
|------|------|----------|------|
| 1.0 | 2026-05-02 | 初始版本 | AI Assistant |
