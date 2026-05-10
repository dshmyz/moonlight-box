# AI集成实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在 Moonlight Registry 中集成 AI 功能，为管理员和开发者提供智能辅助能力，支持问题排查、包查询、用法推荐和代码生成。

**架构：** 采用轻量级AI助手架构，通过工具调用机制让AI能够查询仓库内部数据（日志、数据库、包信息等），提供Web聊天、REST API两种交互方式，支持流式响应和会话管理。

**技术栈：** Go 1.26+、Gin、GORM、Vue 3 + TypeScript、内网大模型（ChatGLM/Qwen）

---

## 文件结构

### 后端文件

**核心服务：**
- `internal/ai/service.go` - AI核心服务，负责对话管理、工具调用编排
- `internal/ai/client.go` - AI模型客户端，处理与内网大模型的HTTP通信
- `internal/ai/tool_manager.go` - 工具管理器，负责工具注册、权限控制、执行调度
- `internal/ai/session_manager.go` - 会话管理器，管理对话历史和上下文
- `internal/ai/rate_limiter.go` - 限流器，防止服务被滥用
- `internal/ai/sanitizer.go` - 数据脱敏，保护敏感信息
- `internal/ai/cache.go` - 响应缓存，提升性能
- `internal/ai/context_builder.go` - 上下文构建器，构建AI所需的上下文
- `internal/ai/models/request.go` - 请求模型定义
- `internal/ai/models/response.go` - 响应模型定义

**工具实现：**
- `internal/ai/tools/tool.go` - 工具接口定义和基础工具
- `internal/ai/tools/log_query.go` - 日志查询工具
- `internal/ai/tools/db_query.go` - 数据库查询工具
- `internal/ai/tools/package_info.go` - 包信息查询工具
- `internal/ai/tools/security.go` - 安全分析工具
- `internal/ai/tools/code_gen.go` - 代码生成工具
- `internal/ai/tools/stats.go` - 统计分析工具

**HTTP接口：**
- `internal/handler/ai_handler.go` - AI相关HTTP接口

**配置：**
- `internal/config/config.go` - 添加AI配置结构（修改）
- `configs/config.example.yaml` - 添加AI配置示例（修改）

**主程序：**
- `cmd/registry/main.go` - 初始化AI服务并注册路由（修改）

### 前端文件

**组件：**
- `web/src/components/ai/AIAssistant.vue` - AI助手主组件
- `web/src/components/ai/ChatWindow.vue` - 聊天窗口
- `web/src/components/ai/MessageList.vue` - 消息列表
- `web/src/components/ai/MessageItem.vue` - 单条消息展示
- `web/src/components/ai/InputBox.vue` - 输入框
- `web/src/components/ai/ToolCallDisplay.vue` - 工具调用展示
- `web/src/components/ai/SuggestionList.vue` - 建议列表
- `web/src/components/ai/FeedbackModal.vue` - 反馈弹窗

**逻辑：**
- `web/src/composables/ai/useChat.ts` - 聊天逻辑
- `web/src/composables/ai/useStream.ts` - 流式响应处理
- `web/src/composables/ai/useTools.ts` - 工具相关逻辑

**类型：**
- `web/src/types/ai.ts` - AI相关类型定义

### 测试文件

- `internal/ai/service_test.go` - AI服务测试
- `internal/ai/tool_manager_test.go` - 工具管理器测试
- `internal/ai/tools/log_query_test.go` - 日志查询工具测试
- `internal/ai/tools/db_query_test.go` - 数据库查询工具测试
- `internal/handler/ai_handler_test.go` - HTTP接口测试

---

## 任务分解

### 任务 1：添加AI配置

**文件：**
- 修改：`internal/config/config.go`
- 修改：`configs/config.example.yaml`

- [ ] **步骤 1：在config.go中添加AI配置结构**

在 `Config` 结构体中添加 `AI` 字段：

```go
type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Database DatabaseConfig `mapstructure:"database"`
    Storage  StorageConfig  `mapstructure:"storage"`
    Auth     AuthConfig     `mapstructure:"auth"`
    Security SecurityConfig `mapstructure:"security"`
    Cache    CacheConfig    `mapstructure:"cache"`
    Logging  LoggingConfig  `mapstructure:"logging"`
    Proxy    ProxyConfig    `mapstructure:"proxy"`
    AI       AIConfig       `mapstructure:"ai"` // 新增
}

type AIConfig struct {
    Enabled      bool              `mapstructure:"enabled"`
    Provider     string            `mapstructure:"provider"`
    BaseURL      string            `mapstructure:"base_url"`
    APIKey       string            `mapstructure:"api_key"`
    Model        string            `mapstructure:"model"`
    MaxTokens    int               `mapstructure:"max_tokens"`
    Temperature  float64           `mapstructure:"temperature"`
    Timeout      time.Duration     `mapstructure:"timeout"`
    Tools        ToolsConfig       `mapstructure:"tools"`
    RateLimit    RateLimitConfig   `mapstructure:"rate_limit"`
    Cache        CacheConfig       `mapstructure:"cache"`
    Session      SessionConfig     `mapstructure:"session"`
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

- [ ] **步骤 2：在defaults.go中添加AI配置默认值**

在 `setDefaults` 函数中添加：

```go
// AI 配置默认值
v.SetDefault("ai.enabled", false)
v.SetDefault("ai.provider", "chatglm")
v.SetDefault("ai.base_url", "http://localhost:8000/v1")
v.SetDefault("ai.model", "chatglm3-6b")
v.SetDefault("ai.max_tokens", 2048)
v.SetDefault("ai.temperature", 0.7)
v.SetDefault("ai.timeout", "30s")
v.SetDefault("ai.tools.enabled", true)
v.SetDefault("ai.tools.max_execution_time", 10)
v.SetDefault("ai.tools.enable_audit_log", true)
v.SetDefault("ai.rate_limit.requests_per_minute", 20)
v.SetDefault("ai.rate_limit.requests_per_day", 500)
v.SetDefault("ai.rate_limit.tokens_per_day", 100000)
v.SetDefault("ai.cache.enabled", true)
v.SetDefault("ai.cache.ttl", "1h")
v.SetDefault("ai.cache.max_size", 1000)
v.SetDefault("ai.session.max_age", "24h")
v.SetDefault("ai.session.max_messages", 50)
```

- [ ] **步骤 3：在config.example.yaml中添加AI配置示例**

```yaml
# AI 配置
ai:
  enabled: false
  provider: "chatglm"  # chatglm, qwen, custom
  base_url: "http://192.168.1.100:8000/v1"
  api_key: ""
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

- [ ] **步骤 4：Commit配置变更**

```bash
git add internal/config/config.go internal/config/defaults.go configs/config.example.yaml
git commit -m "feat: add AI configuration structure"
```

---

### 任务 2：创建AI模型定义

**文件：**
- 创建：`internal/ai/models/request.go`
- 创建：`internal/ai/models/response.go`

- [ ] **步骤 1：创建request.go定义请求模型**

```go
package models

type ChatRequest struct {
    Model       string          `json:"model"`
    Messages    []Message       `json:"messages"`
    Tools       []ToolDefinition `json:"tools,omitempty"`
    Temperature float64         `json:"temperature"`
    MaxTokens   int             `json:"max_tokens"`
    Stream      bool            `json:"stream"`
}

type Message struct {
    Role       string                   `json:"role"`
    Content    string                   `json:"content"`
    ToolCalls  []ToolCall               `json:"tool_calls,omitempty"`
    ToolCallID string                   `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
    ID       string       `json:"id"`
    Type     string       `json:"type"`
    Function FunctionCall `json:"function"`
}

type FunctionCall struct {
    Name      string                 `json:"name"`
    Arguments map[string]interface{} `json:"arguments"`
}

type ToolDefinition struct {
    Type     string             `json:"type"`
    Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Parameters  json.RawMessage `json:"parameters"`
}
```

- [ ] **步骤 2：创建response.go定义响应模型**

```go
package models

type ChatResponse struct {
    ID      string   `json:"id"`
    Model   string   `json:"model"`
    Choices []Choice `json:"choices"`
    Usage   Usage    `json:"usage"`
}

type Choice struct {
    Index        int     `json:"index"`
    Message      Message `json:"message"`
    FinishReason string  `json:"finish_reason"`
}

type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}

type StreamChunk struct {
    ID      string `json:"id"`
    Object  string `json:"object"`
    Created int64  `json:"created"`
    Model   string `json:"model"`
    Choices []struct {
        Index int `json:"index"`
        Delta struct {
            Role    string `json:"role,omitempty"`
            Content string `json:"content,omitempty"`
        } `json:"delta"`
        FinishReason string `json:"finish_reason,omitempty"`
    } `json:"choices"`
}
```

- [ ] **步骤 3：Commit模型定义**

```bash
git add internal/ai/models/
git commit -m "feat: add AI request and response models"
```

---

### 任务 3：创建工具接口

**文件：**
- 创建：`internal/ai/tools/tool.go`

- [ ] **步骤 1：定义工具接口和基础结构**

```go
package tools

import (
    "context"
    "encoding/json"
    
    "github.com/moonlight-box/registry/internal/model"
    "github.com/moonlight-box/registry/internal/config"
    "gorm.io/gorm"
)

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
    LogPath   string
}

type BaseTool struct {
    ctx *ToolContext
}

func (t *BaseTool) SetContext(ctx *ToolContext) {
    t.ctx = ctx
}

func (t *BaseTool) Context() *ToolContext {
    return t.ctx
}
```

- [ ] **步骤 2：Commit工具接口**

```bash
git add internal/ai/tools/tool.go
git commit -m "feat: add tool interface definition"
```

---

### 任务 4：实现AI客户端

**文件：**
- 创建：`internal/ai/client.go`

- [ ] **步骤 1：创建AI客户端结构**

```go
package ai

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
    
    "github.com/moonlight-box/registry/internal/ai/models"
    "github.com/moonlight-box/registry/internal/config"
)

type AIClient struct {
    httpClient *http.Client
    baseURL    string
    apiKey     string
    model      string
}

func NewAIClient(cfg *config.AIConfig) *AIClient {
    return &AIClient{
        httpClient: &http.Client{
            Timeout: cfg.Timeout,
        },
        baseURL: cfg.BaseURL,
        apiKey:  cfg.APIKey,
        model:   cfg.Model,
    }
}

func (c *AIClient) Call(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error) {
    req.Model = c.model
    
    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
    }
    
    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }
    
    httpReq.Header.Set("Content-Type", "application/json")
    if c.apiKey != "" {
        httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
    }
    
    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("do request: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
    }
    
    var chatResp models.ChatResponse
    if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }
    
    return &chatResp, nil
}

func (c *AIClient) Stream(ctx context.Context, req models.ChatRequest) (<-chan models.StreamChunk, error) {
    req.Model = c.model
    req.Stream = true
    
    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
    }
    
    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }
    
    httpReq.Header.Set("Content-Type", "application/json")
    if c.apiKey != "" {
        httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
    }
    
    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("do request: %w", err)
    }
    
    if resp.StatusCode != http.StatusOK {
        defer resp.Body.Close()
        bodyBytes, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
    }
    
    ch := make(chan models.StreamChunk, 100)
    
    go func() {
        defer close(ch)
        defer resp.Body.Close()
        
        decoder := json.NewDecoder(resp.Body)
        for {
            var chunk models.StreamChunk
            if err := decoder.Decode(&chunk); err != nil {
                if err == io.EOF {
                    return
                }
                return
            }
            
            select {
            case ch <- chunk:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return ch, nil
}
```

- [ ] **步骤 2：Commit AI客户端**

```bash
git add internal/ai/client.go
git commit -m "feat: add AI client for internal LLM"
```

---

### 任务 5：实现会话管理器

**文件：**
- 创建：`internal/ai/session_manager.go`

- [ ] **步骤 1：创建会话管理器**

```go
package ai

import (
    "sync"
    "time"
    
    "github.com/google/uuid"
    "github.com/moonlight-box/registry/internal/ai/models"
    "github.com/moonlight-box/registry/internal/config"
)

type Session struct {
    ID        string
    UserID    uint
    Messages  []models.Message
    CreatedAt time.Time
    UpdatedAt time.Time
}

type SessionManager struct {
    sessions    map[string]*Session
    mu          sync.RWMutex
    maxAge      time.Duration
    maxMessages int
}

func NewSessionManager(cfg *config.SessionConfig) *SessionManager {
    sm := &SessionManager{
        sessions:    make(map[string]*Session),
        maxAge:      cfg.MaxAge,
        maxMessages: cfg.MaxMessages,
    }
    
    go sm.cleanupExpiredSessions()
    
    return sm
}

func (sm *SessionManager) GetOrCreateSession(userID uint, sessionID string) *Session {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    if sessionID == "" {
        sessionID = uuid.New().String()
    }
    
    if session, exists := sm.sessions[sessionID]; exists {
        if time.Since(session.UpdatedAt) < sm.maxAge {
            return session
        }
        delete(sm.sessions, sessionID)
    }
    
    session := &Session{
        ID:        sessionID,
        UserID:    userID,
        Messages:  []models.Message{},
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
    sm.sessions[sessionID] = session
    return session
}

func (sm *SessionManager) AddMessage(sessionID string, msg models.Message) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    if session, exists := sm.sessions[sessionID]; exists {
        session.Messages = append(session.Messages, msg)
        session.UpdatedAt = time.Now()
        
        if len(session.Messages) > sm.maxMessages {
            keepCount := sm.maxMessages - 10
            if keepCount < 0 {
                keepCount = sm.maxMessages
            }
            session.Messages = session.Messages[len(session.Messages)-keepCount:]
        }
    }
}

func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    
    session, exists := sm.sessions[sessionID]
    return session, exists
}

func (sm *SessionManager) DeleteSession(sessionID string) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    delete(sm.sessions, sessionID)
}

func (sm *SessionManager) cleanupExpiredSessions() {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    
    for range ticker.C {
        sm.mu.Lock()
        for id, session := range sm.sessions {
            if time.Since(session.UpdatedAt) > sm.maxAge {
                delete(sm.sessions, id)
            }
        }
        sm.mu.Unlock()
    }
}
```

- [ ] **步骤 2：Commit会话管理器**

```bash
git add internal/ai/session_manager.go
git commit -m "feat: add session manager for conversation history"
```

---

### 任务 6：实现限流器

**文件：**
- 创建：`internal/ai/rate_limiter.go`

- [ ] **步骤 1：创建限流器**

```go
package ai

import (
    "sync"
    "time"
    
    "github.com/moonlight-box/registry/internal/config"
)

type RateLimit struct {
    MinuteCount int
    DayCount    int
    TokenCount  int
    ResetAt     time.Time
    DayResetAt  time.Time
}

type RateLimiter struct {
    requests map[uint]*RateLimit
    mu       sync.RWMutex
    config   *config.RateLimitConfig
}

func NewRateLimiter(cfg *config.RateLimitConfig) *RateLimiter {
    return &RateLimiter{
        requests: make(map[uint]*RateLimit),
        config:   cfg,
    }
}

func (rl *RateLimiter) Allow(userID uint) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    now := time.Now()
    
    if limit, exists := rl.requests[userID]; exists {
        if now.Sub(limit.ResetAt) > time.Minute {
            limit.MinuteCount = 0
            limit.ResetAt = now
        }
        
        if now.Sub(limit.DayResetAt) > 24*time.Hour {
            limit.DayCount = 0
            limit.TokenCount = 0
            limit.DayResetAt = now
        }
        
        if limit.MinuteCount >= rl.config.RequestsPerMinute {
            return false
        }
        
        if limit.DayCount >= rl.config.RequestsPerDay {
            return false
        }
    }
    
    return true
}

func (rl *RateLimiter) Record(userID uint, tokens int) {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    if limit, exists := rl.requests[userID]; exists {
        limit.MinuteCount++
        limit.DayCount++
        limit.TokenCount += tokens
    } else {
        now := time.Now()
        rl.requests[userID] = &RateLimit{
            MinuteCount: 1,
            DayCount:    1,
            TokenCount:  tokens,
            ResetAt:     now,
            DayResetAt:  now,
        }
    }
}

func (rl *RateLimiter) GetStatus(userID uint) (int, int, int) {
    rl.mu.RLock()
    defer rl.mu.RUnlock()
    
    if limit, exists := rl.requests[userID]; exists {
        return limit.MinuteCount, limit.DayCount, limit.TokenCount
    }
    return 0, 0, 0
}
```

- [ ] **步骤 2：Commit限流器**

```bash
git add internal/ai/rate_limiter.go
git commit -m "feat: add rate limiter for AI requests"
```

---

### 任务 7：实现数据脱敏

**文件：**
- 创建：`internal/ai/sanitizer.go`

- [ ] **步骤 1：创建数据脱敏器**

```go
package ai

import (
    "regexp"
    "strings"
)

type DataSanitizer struct {
    patterns []*regexp.Regexp
}

func NewDataSanitizer() *DataSanitizer {
    return &DataSanitizer{
        patterns: []*regexp.Regexp{
            regexp.MustCompile(`(?i)password['":\s]*['"]?([^'" \n]+)['"]?`),
            regexp.MustCompile(`(?i)api[_-]?key['":\s]*['"]?([^'" \n]+)['"]?`),
            regexp.MustCompile(`(?i)secret['":\s]*['"]?([^'" \n]+)['"]?`),
            regexp.MustCompile(`(?i)token['":\s]*['"]?([^'" \n]+)['"]?`),
            regexp.MustCompile(`(?i)authorization['":\s]*['"]?([^'" \n]+)['"]?`),
            regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
            regexp.MustCompile(`\b[\w\.-]+@[\w\.-]+\.\w{2,}\b`),
        },
    }
}

func (ds *DataSanitizer) Sanitize(input string) string {
    result := input
    for _, pattern := range ds.patterns {
        result = pattern.ReplaceAllStringFunc(result, func(match string) string {
            return "[REDACTED]"
        })
    }
    return result
}

func (ds *DataSanitizer) SanitizeToolResult(toolName string, result string) string {
    switch toolName {
    case "query_database":
        return ds.sanitizeDBResult(result)
    case "query_logs":
        return ds.sanitizeLogResult(result)
    default:
        return ds.Sanitize(result)
    }
}

func (ds *DataSanitizer) sanitizeDBResult(result string) string {
    return ds.Sanitize(result)
}

func (ds *DataSanitizer) sanitizeLogResult(result string) string {
    return ds.Sanitize(result)
}

func (ds *DataSanitizer) ContainsSensitiveData(input string) bool {
    for _, pattern := range ds.patterns {
        if pattern.MatchString(input) {
            return true
        }
    }
    return false
}
```

- [ ] **步骤 2：Commit数据脱敏器**

```bash
git add internal/ai/sanitizer.go
git commit -m "feat: add data sanitizer for sensitive information"
```

---

### 任务 8：实现响应缓存

**文件：**
- 创建：`internal/ai/cache.go`

- [ ] **步骤 1：创建响应缓存**

```go
package ai

import (
    "crypto/sha256"
    "encoding/hex"
    "sync"
    "time"
    
    "github.com/moonlight-box/registry/internal/config"
)

type CacheEntry struct {
    Response  string
    CreatedAt time.Time
    HitCount  int
}

type ResponseCache struct {
    cache   map[string]*CacheEntry
    mu      sync.RWMutex
    ttl     time.Duration
    maxSize int
}

func NewResponseCache(cfg *config.CacheConfig) *ResponseCache {
    if !cfg.Enabled {
        return nil
    }
    
    rc := &ResponseCache{
        cache:   make(map[string]*CacheEntry),
        ttl:     cfg.TTL,
        maxSize: cfg.MaxSize,
    }
    
    go rc.cleanupExpiredEntries()
    
    return rc
}

func (rc *ResponseCache) Get(query string) (string, bool) {
    if rc == nil {
        return "", false
    }
    
    rc.mu.RLock()
    defer rc.mu.RUnlock()
    
    key := rc.hashQuery(query)
    if entry, exists := rc.cache[key]; exists {
        if time.Since(entry.CreatedAt) < rc.ttl {
            entry.HitCount++
            return entry.Response, true
        }
    }
    return "", false
}

func (rc *ResponseCache) Set(query, response string) {
    if rc == nil {
        return
    }
    
    rc.mu.Lock()
    defer rc.mu.Unlock()
    
    if len(rc.cache) >= rc.maxSize {
        rc.evictOldest()
    }
    
    key := rc.hashQuery(query)
    rc.cache[key] = &CacheEntry{
        Response:  response,
        CreatedAt: time.Now(),
        HitCount:  0,
    }
}

func (rc *ResponseCache) hashQuery(query string) string {
    normalized := strings.ToLower(strings.TrimSpace(query))
    hash := sha256.Sum256([]byte(normalized))
    return hex.EncodeToString(hash[:])
}

func (rc *ResponseCache) evictOldest() {
    var oldestKey string
    var oldestTime time.Time
    
    for key, entry := range rc.cache {
        if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
            oldestKey = key
            oldestTime = entry.CreatedAt
        }
    }
    
    if oldestKey != "" {
        delete(rc.cache, oldestKey)
    }
}

func (rc *ResponseCache) cleanupExpiredEntries() {
    ticker := time.NewTicker(10 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        rc.mu.Lock()
        for key, entry := range rc.cache {
            if time.Since(entry.CreatedAt) > rc.ttl {
                delete(rc.cache, key)
            }
        }
        rc.mu.Unlock()
    }
}

func (rc *ResponseCache) Clear() {
    if rc == nil {
        return
    }
    
    rc.mu.Lock()
    defer rc.mu.Unlock()
    
    rc.cache = make(map[string]*CacheEntry)
}

func (rc *ResponseCache) Stats() (int, int) {
    if rc == nil {
        return 0, 0
    }
    
    rc.mu.RLock()
    defer rc.mu.RUnlock()
    
    totalHits := 0
    for _, entry := range rc.cache {
        totalHits += entry.HitCount
    }
    
    return len(rc.cache), totalHits
}
```

- [ ] **步骤 2：Commit响应缓存**

```bash
git add internal/ai/cache.go
git commit -m "feat: add response cache for AI queries"
```

---

### 任务 9：实现工具管理器

**文件：**
- 创建：`internal/ai/tool_manager.go`

- [ ] **步骤 1：创建工具管理器**

```go
package ai

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "github.com/moonlight-box/registry/internal/ai/models"
    "github.com/moonlight-box/registry/internal/ai/tools"
    "github.com/moonlight-box/registry/internal/model"
    "github.com/moonlight-box/registry/internal/repository"
)

type ToolManager struct {
    tools       map[string]tools.Tool
    permissions map[string][]string
    auditRepo   *repository.AuditRepository
    sanitizer   *DataSanitizer
}

func NewToolManager(auditRepo *repository.AuditRepository) *ToolManager {
    return &ToolManager{
        tools:       make(map[string]tools.Tool),
        permissions: make(map[string][]string),
        auditRepo:   auditRepo,
        sanitizer:   NewDataSanitizer(),
    }
}

func (tm *ToolManager) RegisterTool(tool tools.Tool, allowedRoles []string) {
    tm.tools[tool.Name()] = tool
    if allowedRoles != nil {
        tm.permissions[tool.Name()] = allowedRoles
    }
}

func (tm *ToolManager) ExecuteTool(
    ctx context.Context,
    toolName string,
    params map[string]interface{},
    user *model.User,
) (string, error) {
    tool, exists := tm.tools[toolName]
    if !exists {
        return "", fmt.Errorf("tool not found: %s", toolName)
    }
    
    if !tm.hasPermission(user, toolName) {
        return "", fmt.Errorf("permission denied for tool: %s", toolName)
    }
    
    startTime := time.Now()
    result, err := tool.Execute(ctx, params)
    duration := time.Since(startTime)
    
    result = tm.sanitizer.SanitizeToolResult(toolName, result)
    
    if tm.auditRepo != nil {
        tm.recordAuditLog(user.ID, toolName, params, result, err, duration)
    }
    
    return result, err
}

func (tm *ToolManager) hasPermission(user *model.User, toolName string) bool {
    allowedRoles, exists := tm.permissions[toolName]
    if !exists {
        return true
    }
    
    for _, role := range user.Roles {
        for _, allowedRole := range allowedRoles {
            if role.Name == allowedRole {
                return true
            }
        }
    }
    
    return false
}

func (tm *ToolManager) recordAuditLog(
    userID uint,
    toolName string,
    params map[string]interface{},
    result string,
    err error,
    duration time.Duration,
) {
    status := "success"
    if err != nil {
        status = "failed"
    }
    
    paramsJSON, _ := json.Marshal(params)
    
    auditLog := &model.AuditLog{
        UserID:    userID,
        Action:    "ai_tool_call",
        Resource:  toolName,
        Details: map[string]interface{}{
            "params":   string(paramsJSON),
            "result":   result,
            "status":   status,
            "duration": duration.String(),
        },
    }
    
    tm.auditRepo.Create(auditLog)
}

func (tm *ToolManager) GetToolDefinitions() []models.ToolDefinition {
    var definitions []models.ToolDefinition
    for _, tool := range tm.tools {
        definitions = append(definitions, models.ToolDefinition{
            Type: "function",
            Function: models.FunctionDefinition{
                Name:        tool.Name(),
                Description: tool.Description(),
                Parameters:  tool.Parameters(),
            },
        })
    }
    return definitions
}

func (tm *ToolManager) ListTools() []ToolInfo {
    var toolList []ToolInfo
    for name, tool := range tm.tools {
        toolList = append(toolList, ToolInfo{
            Name:        name,
            Description: tool.Description(),
            Parameters:  tool.Parameters(),
        })
    }
    return toolList
}

type ToolInfo struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Parameters  json.RawMessage `json:"parameters"`
}
```

- [ ] **步骤 2：Commit工具管理器**

```bash
git add internal/ai/tool_manager.go
git commit -m "feat: add tool manager for AI function calling"
```

---

### 任务 10：实现日志查询工具

**文件：**
- 创建：`internal/ai/tools/log_query.go`

- [ ] **步骤 1：创建日志查询工具**

```go
package tools

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"
)

type LogQueryTool struct {
    BaseTool
}

func (t *LogQueryTool) Name() string {
    return "query_logs"
}

func (t *LogQueryTool) Description() string {
    return "查询系统日志，支持按时间、级别、关键词过滤。用于排查系统问题和错误。"
}

func (t *LogQueryTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "start_time": {
                "type": "string",
                "description": "开始时间，格式：2006-01-02 15:04:05"
            },
            "end_time": {
                "type": "string",
                "description": "结束时间，格式：2006-01-02 15:04:05"
            },
            "level": {
                "type": "string",
                "enum": ["debug", "info", "warn", "error"],
                "description": "日志级别"
            },
            "keyword": {
                "type": "string",
                "description": "搜索关键词"
            },
            "source": {
                "type": "string",
                "description": "日志来源：proxy, adapter, auth, migration等"
            },
            "limit": {
                "type": "integer",
                "description": "返回条数限制，默认100，最大500"
            }
        },
        "required": []
    }`)
}

type LogEntry struct {
    Time    time.Time
    Level   string
    Source  string
    Message string
}

func (t *LogQueryTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    startTime, _ := params["start_time"].(string)
    endTime, _ := params["end_time"].(string)
    level, _ := params["level"].(string)
    keyword, _ := params["keyword"].(string)
    source, _ := params["source"].(string)
    limit := 100
    if l, ok := params["limit"].(float64); ok {
        limit = int(l)
        if limit > 500 {
            limit = 500
        }
    }
    
    logPath := t.Context().LogPath
    if logPath == "" {
        logPath = t.Context().Config.Storage.Local.BasePath + "/logs"
    }
    
    logFiles, err := t.getLogFiles(logPath, source)
    if err != nil {
        return "", err
    }
    
    var logs []LogEntry
    for _, file := range logFiles {
        fileLogs, err := t.parseLogFile(file, startTime, endTime, level, keyword, limit-len(logs))
        if err != nil {
            continue
        }
        logs = append(logs, fileLogs...)
        if len(logs) >= limit {
            break
        }
    }
    
    return t.formatLogs(logs), nil
}

func (t *LogQueryTool) getLogFiles(logPath, source string) ([]string, error) {
    var files []string
    
    err := filepath.Walk(logPath, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if !info.IsDir() && strings.HasSuffix(info.Name(), ".log") {
            if source == "" || strings.Contains(info.Name(), source) {
                files = append(files, path)
            }
        }
        return nil
    })
    
    return files, err
}

func (t *LogQueryTool) parseLogFile(filePath, startTime, endTime, level, keyword string, limit int) ([]LogEntry, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return nil, err
    }
    defer file.Close()
    
    var logs []LogEntry
    scanner := bufio.NewScanner(file)
    
    for scanner.Scan() && len(logs) < limit {
        line := scanner.Text()
        entry := t.parseLogLine(line)
        
        if !t.matchFilter(entry, startTime, endTime, level, keyword) {
            continue
        }
        
        logs = append(logs, entry)
    }
    
    return logs, nil
}

func (t *LogQueryTool) parseLogLine(line string) LogEntry {
    entry := LogEntry{}
    
    parts := strings.SplitN(line, " ", 4)
    if len(parts) >= 4 {
        if time, err := time.Parse("2006-01-02T15:04:05.000Z", parts[0]); err == nil {
            entry.Time = time
        }
        entry.Level = strings.ToLower(strings.Trim(parts[1], "[]"))
        entry.Source = strings.Trim(parts[2], "[]")
        entry.Message = parts[3]
    } else {
        entry.Message = line
    }
    
    return entry
}

func (t *LogQueryTool) matchFilter(entry LogEntry, startTime, endTime, level, keyword string) bool {
    if level != "" && entry.Level != level {
        return false
    }
    
    if keyword != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(keyword)) {
        return false
    }
    
    if startTime != "" {
        if st, err := time.Parse("2006-01-02 15:04:05", startTime); err == nil {
            if entry.Time.Before(st) {
                return false
            }
        }
    }
    
    if endTime != "" {
        if et, err := time.Parse("2006-01-02 15:04:05", endTime); err == nil {
            if entry.Time.After(et) {
                return false
            }
        }
    }
    
    return true
}

func (t *LogQueryTool) formatLogs(logs []LogEntry) string {
    var builder strings.Builder
    builder.WriteString(fmt.Sprintf("找到 %d 条日志记录：\n\n", len(logs)))
    
    for i, log := range logs {
        builder.WriteString(fmt.Sprintf("[%d] %s [%s] %s - %s\n",
            i+1,
            log.Time.Format("2006-01-02 15:04:05"),
            log.Level,
            log.Source,
            log.Message,
        ))
    }
    
    return builder.String()
}
```

- [ ] **步骤 2：Commit日志查询工具**

```bash
git add internal/ai/tools/log_query.go
git commit -m "feat: add log query tool for AI"
```

---

### 任务 11：实现数据库查询工具

**文件：**
- 创建：`internal/ai/tools/db_query.go`

- [ ] **步骤 1：创建数据库查询工具**

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "time"
    
    "github.com/moonlight-box/registry/internal/model"
)

type DatabaseQueryTool struct {
    BaseTool
}

func (t *DatabaseQueryTool) Name() string {
    return "query_database"
}

func (t *DatabaseQueryTool) Description() string {
    return "查询仓库数据库，获取包信息、下载记录、安全扫描结果、仓库配置等数据。"
}

func (t *DatabaseQueryTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "query_type": {
                "type": "string",
                "enum": [
                    "package_info",
                    "download_stats",
                    "security_scan",
                    "dependencies",
                    "repository_info",
                    "block_rules",
                    "user_activity"
                ],
                "description": "查询类型"
            },
            "package_name": {
                "type": "string",
                "description": "包名称"
            },
            "package_type": {
                "type": "string",
                "enum": ["npm", "maven", "pypi", "go", "nuget", "yum", "apt"],
                "description": "包类型"
            },
            "repository_name": {
                "type": "string",
                "description": "仓库名称"
            },
            "time_range": {
                "type": "string",
                "enum": ["1h", "24h", "7d", "30d", "90d"],
                "description": "时间范围"
            },
            "limit": {
                "type": "integer",
                "description": "返回条数限制"
            }
        },
        "required": ["query_type"]
    }`)
}

func (t *DatabaseQueryTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    queryType := params["query_type"].(string)
    
    switch queryType {
    case "package_info":
        return t.queryPackageInfo(params)
    case "download_stats":
        return t.queryDownloadStats(params)
    case "security_scan":
        return t.querySecurityScan(params)
    case "dependencies":
        return t.queryDependencies(params)
    case "repository_info":
        return t.queryRepositoryInfo(params)
    case "block_rules":
        return t.queryBlockRules(params)
    case "user_activity":
        return t.queryUserActivity(params)
    default:
        return "", fmt.Errorf("unknown query type: %s", queryType)
    }
}

func (t *DatabaseQueryTool) queryPackageInfo(params map[string]interface{}) (string, error) {
    packageName, _ := params["package_name"].(string)
    packageType, _ := params["package_type"].(string)
    
    if packageName == "" {
        return "", fmt.Errorf("package_name is required")
    }
    
    var pkg model.Package
    query := t.Context().DB.Where("name = ?", packageName)
    if packageType != "" {
        query = query.Where("type = ?", packageType)
    }
    
    if err := query.First(&pkg).Error; err != nil {
        return "", fmt.Errorf("package not found: %s", packageName)
    }
    
    var versions []model.PackageVersion
    t.Context().DB.Where("package_id = ?", pkg.ID).Order("created_at desc").Limit(10).Find(&versions)
    
    var builder strings.Builder
    builder.WriteString(fmt.Sprintf("📦 包信息：\n"))
    builder.WriteString(fmt.Sprintf("名称：%s\n", pkg.Name))
    builder.WriteString(fmt.Sprintf("类型：%s\n", pkg.Type))
    builder.WriteString(fmt.Sprintf("最新版本：%s\n", pkg.Version))
    builder.WriteString(fmt.Sprintf("描述：%s\n", pkg.Description))
    builder.WriteString(fmt.Sprintf("下载次数：%d\n", pkg.DownloadCount))
    builder.WriteString(fmt.Sprintf("创建时间：%s\n", pkg.CreatedAt.Format("2006-01-02")))
    builder.WriteString(fmt.Sprintf("更新时间：%s\n", pkg.UpdatedAt.Format("2006-01-02")))
    
    builder.WriteString("\n📋 最近版本：\n")
    for i, v := range versions {
        builder.WriteString(fmt.Sprintf("  %d. %s (%s) - %s\n",
            i+1,
            v.Version,
            v.CreatedAt.Format("2006-01-02"),
            v.Status,
        ))
    }
    
    return builder.String(), nil
}

func (t *DatabaseQueryTool) queryDownloadStats(params map[string]interface{}) (string, error) {
    packageName, _ := params["package_name"].(string)
    timeRange, _ := params["time_range"].(string)
    if timeRange == "" {
        timeRange = "7d"
    }
    
    duration := t.parseDuration(timeRange)
    startTime := time.Now().Add(-duration)
    
    var stats []struct {
        Date    time.Time
        Count   int
        Version string
    }
    
    query := t.Context().DB.Table("audit_logs").
        Select("DATE(created_at) as date, COUNT(*) as count, details->>'version' as version").
        Where("action = ?", "download").
        Where("created_at >= ?", startTime).
        Group("DATE(created_at), details->>'version'")
    
    if packageName != "" {
        query = query.Where("details->>'package_name' = ?", packageName)
    }
    
    query.Order("date desc").Find(&stats)
    
    var builder strings.Builder
    builder.WriteString(fmt.Sprintf("📊 下载统计（过去%s）：\n\n", timeRange))
    
    totalDownloads := 0
    for _, stat := range stats {
        builder.WriteString(fmt.Sprintf("%s: %d 次下载 (版本: %s)\n",
            stat.Date.Format("2006-01-02"),
            stat.Count,
            stat.Version,
        ))
        totalDownloads += stat.Count
    }
    
    builder.WriteString(fmt.Sprintf("\n总计：%d 次下载\n", totalDownloads))
    
    return builder.String(), nil
}

func (t *DatabaseQueryTool) querySecurityScan(params map[string]interface{}) (string, error) {
    packageName, _ := params["package_name"].(string)
    
    var scans []model.SecurityScan
    query := t.Context().DB.Preload("Package").Order("scanned_at desc")
    
    if packageName != "" {
        query = query.Joins("JOIN packages ON packages.id = security_scans.package_id").
            Where("packages.name = ?", packageName)
    }
    
    query.Limit(20).Find(&scans)
    
    var builder strings.Builder
    builder.WriteString("🔒 安全扫描结果：\n\n")
    
    for _, scan := range scans {
        builder.WriteString(fmt.Sprintf("📦 %s@%s\n", scan.Package.Name, scan.Version))
        builder.WriteString(fmt.Sprintf("   扫描时间：%s\n", scan.ScannedAt.Format("2006-01-02 15:04")))
        builder.WriteString(fmt.Sprintf("   状态：%s\n", scan.Status))
        
        if scan.CriticalCount > 0 {
            builder.WriteString(fmt.Sprintf("   🔴 严重漏洞：%d\n", scan.CriticalCount))
        }
        if scan.HighCount > 0 {
            builder.WriteString(fmt.Sprintf("   🟠 高危漏洞：%d\n", scan.HighCount))
        }
        if scan.MediumCount > 0 {
            builder.WriteString(fmt.Sprintf("   🟡 中危漏洞：%d\n", scan.MediumCount))
        }
        if scan.LowCount > 0 {
            builder.WriteString(fmt.Sprintf("   🟢 低危漏洞：%d\n", scan.LowCount))
        }
        builder.WriteString("\n")
    }
    
    return builder.String(), nil
}

func (t *DatabaseQueryTool) queryDependencies(params map[string]interface{}) (string, error) {
    packageName, _ := params["package_name"].(string)
    packageType, _ := params["package_type"].(string)
    
    if packageName == "" {
        return "", fmt.Errorf("package_name is required")
    }
    
    var pkg model.Package
    query := t.Context().DB.Where("name = ?", packageName)
    if packageType != "" {
        query = query.Where("type = ?", packageType)
    }
    
    if err := query.First(&pkg).Error; err != nil {
        return "", fmt.Errorf("package not found: %s", packageName)
    }
    
    var deps []model.Dependency
    t.Context().DB.Where("package_id = ?", pkg.ID).Find(&deps)
    
    var builder strings.Builder
    builder.WriteString(fmt.Sprintf("📋 %s 的依赖关系：\n\n", packageName))
    
    for _, dep := range deps {
        builder.WriteString(fmt.Sprintf("  - %s@%s\n", dep.Name, dep.Version))
    }
    
    return builder.String(), nil
}

func (t *DatabaseQueryTool) queryRepositoryInfo(params map[string]interface{}) (string, error) {
    repoName, _ := params["repository_name"].(string)
    
    if repoName == "" {
        return "", fmt.Errorf("repository_name is required")
    }
    
    var repo model.Repository
    if err := t.Context().DB.Where("name = ?", repoName).First(&repo).Error; err != nil {
        return "", fmt.Errorf("repository not found: %s", repoName)
    }
    
    var builder strings.Builder
    builder.WriteString(fmt.Sprintf("📦 仓库信息：\n"))
    builder.WriteString(fmt.Sprintf("名称：%s\n", repo.Name))
    builder.WriteString(fmt.Sprintf("类型：%s\n", repo.Type))
    builder.WriteString(fmt.Sprintf("代理：%v\n", repo.Proxy))
    builder.WriteString(fmt.Sprintf("状态：%s\n", repo.Status))
    builder.WriteString(fmt.Sprintf("创建时间：%s\n", repo.CreatedAt.Format("2006-01-02")))
    
    return builder.String(), nil
}

func (t *DatabaseQueryTool) queryBlockRules(params map[string]interface{}) (string, error) {
    var rules []model.BlockRule
    t.Context().DB.Order("created_at desc").Limit(20).Find(&rules)
    
    var builder strings.Builder
    builder.WriteString("🚫 阻断规则列表：\n\n")
    
    for i, rule := range rules {
        builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, rule.Name))
        builder.WriteString(fmt.Sprintf("   类型：%s\n", rule.Type))
        builder.WriteString(fmt.Sprintf("   规则：%s\n", rule.Rule))
        builder.WriteString(fmt.Sprintf("   状态：%s\n\n", rule.Status))
    }
    
    return builder.String(), nil
}

func (t *DatabaseQueryTool) queryUserActivity(params map[string]interface{}) (string, error) {
    timeRange, _ := params["time_range"].(string)
    if timeRange == "" {
        timeRange = "7d"
    }
    
    duration := t.parseDuration(timeRange)
    startTime := time.Now().Add(-duration)
    
    var activities []struct {
        User   string
        Action string
        Count  int
    }
    
    t.Context().DB.Table("audit_logs").
        Select("users.username as user, audit_logs.action, COUNT(*) as count").
        Joins("JOIN users ON users.id = audit_logs.user_id").
       	Where("audit_logs.created_at >= ?", startTime).
        Group("users.username, audit_logs.action").
        Order("count desc").
        Limit(20).
        Find(&activities)
    
    var builder strings.Builder
    builder.WriteString(fmt.Sprintf("👥 用户活动统计（过去%s）：\n\n", timeRange))
    
    for _, activity := range activities {
        builder.WriteString(fmt.Sprintf("%s - %s: %d 次\n",
            activity.User,
            activity.Action,
            activity.Count,
        ))
    }
    
    return builder.String(), nil
}

func (t *DatabaseQueryTool) parseDuration(timeRange string) time.Duration {
    switch timeRange {
    case "1h":
        return 1 * time.Hour
    case "24h":
        return 24 * time.Hour
    case "7d":
        return 7 * 24 * time.Hour
    case "30d":
        return 30 * 24 * time.Hour
    case "90d":
        return 90 * 24 * time.Hour
    default:
        return 7 * 24 * time.Hour
    }
}
```

- [ ] **步骤 2：Commit数据库查询工具**

```bash
git add internal/ai/tools/db_query.go
git commit -m "feat: add database query tool for AI"
```

---

### 任务 12：实现包信息查询工具

**文件：**
- 创建：`internal/ai/tools/package_info.go`

- [ ] **步骤 1：创建包信息查询工具**

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    
    "github.com/moonlight-box/registry/internal/model"
)

type PackageInfoTool struct {
    BaseTool
}

func (t *PackageInfoTool) Name() string {
    return "query_package_info"
}

func (t *PackageInfoTool) Description() string {
    return "查询包的详细信息，包括版本历史、依赖关系、使用示例等。"
}

func (t *PackageInfoTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "package_name": {
                "type": "string",
                "description": "包名称"
            },
            "package_type": {
                "type": "string",
                "enum": ["npm", "maven", "pypi", "go", "nuget"],
                "description": "包类型"
            },
            "version": {
                "type": "string",
                "description": "指定版本，不填则查询最新版本"
            },
            "include_dependencies": {
                "type": "boolean",
                "description": "是否包含依赖信息"
            },
            "include_readme": {
                "type": "boolean",
                "description": "是否包含README内容"
            }
        },
        "required": ["package_name"]
    }`)
}

func (t *PackageInfoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    packageName := params["package_name"].(string)
    packageType, _ := params["package_type"].(string)
    version, _ := params["version"].(string)
    includeDeps, _ := params["include_dependencies"].(bool)
    includeReadme, _ := params["include_readme"].(bool)
    
    var pkg model.Package
    query := t.Context().DB.Where("name = ?", packageName)
    if packageType != "" {
        query = query.Where("type = ?", packageType)
    }
    
    if err := query.First(&pkg).Error; err != nil {
        return "", fmt.Errorf("package not found: %s", packageName)
    }
    
    var builder strings.Builder
    builder.WriteString(fmt.Sprintf("📦 %s\n\n", pkg.Name))
    builder.WriteString(fmt.Sprintf("类型：%s\n", pkg.Type))
    builder.WriteString(fmt.Sprintf("最新版本：%s\n", pkg.Version))
    builder.WriteString(fmt.Sprintf("描述：%s\n", pkg.Description))
    builder.WriteString(fmt.Sprintf("作者：%s\n", pkg.Author))
    builder.WriteString(fmt.Sprintf("许可证：%s\n", pkg.License))
    builder.WriteString(fmt.Sprintf("下载次数：%d\n", pkg.DownloadCount))
    
    if includeDeps {
        builder.WriteString("\n📋 依赖关系：\n")
        deps := t.getDependencies(pkg.ID, version)
        for _, dep := range deps {
            builder.WriteString(fmt.Sprintf("  - %s@%s\n", dep.Name, dep.Version))
        }
    }
    
    if includeReadme {
        builder.WriteString("\n📖 README：\n")
        readme := t.getReadme(pkg.ID, version)
        if len(readme) > 500 {
            readme = readme[:500] + "...\n(内容已截断，完整内容请查看详情页)"
        }
        builder.WriteString(readme)
    }
    
    return builder.String(), nil
}

func (t *PackageInfoTool) getDependencies(packageID uint, version string) []model.Dependency {
    var deps []model.Dependency
    query := t.Context().DB.Where("package_id = ?", packageID)
    if version != "" {
        query = query.Where("version = ?", version)
    }
    query.Find(&deps)
    return deps
}

func (t *PackageInfoTool) getReadme(packageID uint, version string) string {
    var pkgVersion model.PackageVersion
    query := t.Context().DB.Where("package_id = ?", packageID)
    if version != "" {
        query = query.Where("version = ?", version)
    } else {
        query = query.Order("created_at desc")
    }
    
    if err := query.First(&pkgVersion).Error; err != nil {
        return "README 未找到"
    }
    
    if readme, ok := pkgVersion.Metadata["readme"].(string); ok {
        return readme
    }
    
    return "README 未找到"
}
```

- [ ] **步骤 2：Commit包信息查询工具**

```bash
git add internal/ai/tools/package_info.go
git commit -m "feat: add package info query tool for AI"
```

---

### 任务 13：实现安全分析工具

**文件：**
- 创建：`internal/ai/tools/security.go`

- [ ] **步骤 1：创建安全分析工具**

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
)

type SecurityAnalysisTool struct {
    BaseTool
}

func (t *SecurityAnalysisTool) Name() string {
    return "analyze_security"
}

func (t *SecurityAnalysisTool) Description() string {
    return "分析包的安全问题，包括漏洞详情、影响范围、修复建议等。"
}

func (t *SecurityAnalysisTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "package_name": {
                "type": "string",
                "description": "包名称"
            },
            "cve_id": {
                "type": "string",
                "description": "CVE编号，如CVE-2020-1234"
            },
            "severity": {
                "type": "string",
                "enum": ["critical", "high", "medium", "low"],
                "description": "漏洞严重程度"
            },
            "analysis_type": {
                "type": "string",
                "enum": ["vulnerabilities", "impact", "fix_suggestion", "all"],
                "description": "分析类型"
            }
        },
        "required": ["analysis_type"]
    }`)
}

func (t *SecurityAnalysisTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    analysisType := params["analysis_type"].(string)
    packageName, _ := params["package_name"].(string)
    cveID, _ := params["cve_id"].(string)
    severity, _ := params["severity"].(string)
    
    switch analysisType {
    case "vulnerabilities":
        return t.listVulnerabilities(packageName, severity)
    case "impact":
        return t.analyzeImpact(packageName, cveID)
    case "fix_suggestion":
        return t.getFixSuggestion(packageName, cveID)
    case "all":
        return t.fullAnalysis(packageName, cveID, severity)
    default:
        return "", fmt.Errorf("unknown analysis type: %s", analysisType)
    }
}

func (t *SecurityAnalysisTool) listVulnerabilities(packageName, severity string) (string, error) {
    query := t.Context().DB.Table("security_scans").
        Joins("JOIN packages ON packages.id = security_scans.package_id")
    
    if packageName != "" {
        query = query.Where("packages.name = ?", packageName)
    }
    
    var vulnerabilities []struct {
        PackageName string
        Version     string
        CVE         string
        Severity    string
        Description string
    }
    
    query.Select("packages.name as package_name, security_scans.version, "+
        "security_scans.details->>'cve' as cve, "+
        "security_scans.details->>'severity' as severity, "+
        "security_scans.details->>'description' as description").
        Where("security_scans.status = ?", "vulnerable").
        Order("security_scans.scanned_at desc").
        Limit(20).
        Find(&vulnerabilities)
    
    var builder strings.Builder
    builder.WriteString("🔍 漏洞列表：\n\n")
    
    if len(vulnerabilities) == 0 {
        builder.WriteString("✅ 未发现漏洞\n")
        return builder.String(), nil
    }
    
    for i, vuln := range vulnerabilities {
        if severity != "" && vuln.Severity != severity {
            continue
        }
        
        emoji := t.getSeverityEmoji(vuln.Severity)
        builder.WriteString(fmt.Sprintf("%d. %s %s@%s\n", i+1, emoji, vuln.PackageName, vuln.Version))
        builder.WriteString(fmt.Sprintf("   CVE: %s\n", vuln.CVE))
        builder.WriteString(fmt.Sprintf("   严重程度: %s\n", vuln.Severity))
        builder.WriteString(fmt.Sprintf("   描述: %s\n\n", vuln.Description))
    }
    
    return builder.String(), nil
}

func (t *SecurityAnalysisTool) analyzeImpact(packageName, cveID string) (string, error) {
    return "影响分析功能待实现", nil
}

func (t *SecurityAnalysisTool) getFixSuggestion(packageName, cveID string) (string, error) {
    var scan model.SecurityScan
    query := t.Context().DB.Preload("Package").
        Where("details->>'cve' = ?", cveID)
    
    if packageName != "" {
        query = query.Joins("JOIN packages ON packages.id = security_scans.package_id").
            Where("packages.name = ?", packageName)
    }
    
    if err := query.First(&scan).Error; err != nil {
        return "", fmt.Errorf("vulnerability not found: %s", cveID)
    }
    
    var builder strings.Builder
    builder.WriteString("💡 修复建议：\n\n")
    builder.WriteString(fmt.Sprintf("📦 包：%s@%s\n", scan.Package.Name, scan.Version))
    builder.WriteString(fmt.Sprintf("🆔 CVE：%s\n", cveID))
    builder.WriteString(fmt.Sprintf("⚠️  严重程度：%s\n\n", scan.Details["severity"]))
    
    builder.WriteString("🔧 修复方案：\n")
    
    if fixedVersion, ok := scan.Details["fixed_version"].(string); ok {
        builder.WriteString(fmt.Sprintf("1. 升级到版本 %s 或更高版本\n", fixedVersion))
    }
    
    if alternative, ok := scan.Details["alternative_package"].(string); ok {
        builder.WriteString(fmt.Sprintf("2. 考虑使用替代包：%s\n", alternative))
    }
    
    builder.WriteString("\n📝 详细说明：\n")
    if description, ok := scan.Details["fix_description"].(string); ok {
        builder.WriteString(description)
    }
    
    return builder.String(), nil
}

func (t *SecurityAnalysisTool) fullAnalysis(packageName, cveID, severity string) (string, error) {
    var builder strings.Builder
    
    builder.WriteString("🔒 完整安全分析：\n\n")
    
    vulns, _ := t.listVulnerabilities(packageName, severity)
    builder.WriteString(vulns)
    
    if cveID != "" {
        builder.WriteString("\n")
        fix, _ := t.getFixSuggestion(packageName, cveID)
        builder.WriteString(fix)
    }
    
    return builder.String(), nil
}

func (t *SecurityAnalysisTool) getSeverityEmoji(severity string) string {
    switch severity {
    case "critical":
        return "🔴"
    case "high":
        return "🟠"
    case "medium":
        return "🟡"
    case "low":
        return "🟢"
    default:
        return "⚪"
    }
}
```

- [ ] **步骤 2：Commit安全分析工具**

```bash
git add internal/ai/tools/security.go
git commit -m "feat: add security analysis tool for AI"
```

---

### 任务 14：实现代码生成工具

**文件：**
- 创建：`internal/ai/tools/code_gen.go`

- [ ] **步骤 1：创建代码生成工具**

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
)

type CodeGenTool struct {
    BaseTool
}

func (t *CodeGenTool) Name() string {
    return "generate_demo_code"
}

func (t *CodeGenTool) Description() string {
    return "生成包的使用示例代码，支持多种包管理器和编程语言。"
}

func (t *CodeGenTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "package_name": {
                "type": "string",
                "description": "包名称"
            },
            "package_type": {
                "type": "string",
                "enum": ["npm", "maven", "pypi", "go", "nuget"],
                "description": "包类型"
            },
            "language": {
                "type": "string",
                "description": "编程语言：javascript, typescript, python, java, go, csharp"
            },
            "scenario": {
                "type": "string",
                "enum": ["basic", "advanced", "testing", "production"],
                "description": "使用场景：基础用法、高级用法、测试示例、生产示例"
            },
            "framework": {
                "type": "string",
                "description": "框架：react, vue, express, spring, flask等"
            }
        },
        "required": ["package_name", "package_type", "language"]
    }`)
}

func (t *CodeGenTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    packageName := params["package_name"].(string)
    packageType := params["package_type"].(string)
    language := params["language"].(string)
    scenario, _ := params["scenario"].(string)
    if scenario == "" {
        scenario = "basic"
    }
    framework, _ := params["framework"].(string)
    
    var pkg model.Package
    if err := t.Context().DB.Where("name = ? AND type = ?", packageName, packageType).First(&pkg).Error; err != nil {
        return "", fmt.Errorf("package not found: %s", packageName)
    }
    
    code := t.generateCode(packageName, packageType, language, scenario, framework, pkg)
    
    var builder strings.Builder
    builder.WriteString(fmt.Sprintf("💻 %s (%s) 使用示例\n\n", packageName, language))
    builder.WriteString(fmt.Sprintf("场景：%s\n\n", t.getScenarioDescription(scenario)))
    builder.WriteString("```")
    builder.WriteString(t.getLanguageCode(language))
    builder.WriteString("\n")
    builder.WriteString(code)
    builder.WriteString("\n```\n\n")
    builder.WriteString("💡 提示：\n")
    builder.WriteString(t.getTips(packageType, scenario))
    
    return builder.String(), nil
}

func (t *CodeGenTool) generateCode(packageName, packageType, language, scenario, framework string, pkg model.Package) string {
    switch packageType {
    case "npm":
        return t.generateNPMCode(packageName, language, scenario, framework, pkg)
    case "pypi":
        return t.generatePythonCode(packageName, scenario, framework, pkg)
    case "go":
        return t.generateGoCode(packageName, scenario, pkg)
    case "maven":
        return t.generateJavaCode(packageName, scenario, framework, pkg)
    default:
        return t.generateGenericCode(packageName, language, scenario, pkg)
    }
}

func (t *CodeGenTool) generateNPMCode(packageName, language, scenario, framework string, pkg model.Package) string {
    var code strings.Builder
    
    if language == "typescript" {
        code.WriteString("import ")
    } else {
        code.WriteString("const ")
    }
    
    code.WriteString(fmt.Sprintf("%s from '%s';\n\n", t.toCamelCase(packageName), packageName))
    
    switch scenario {
    case "basic":
        code.WriteString(fmt.Sprintf("// 基础用法示例\n"))
        code.WriteString(fmt.Sprintf("const result = %s();\n", t.toCamelCase(packageName)))
        code.WriteString(fmt.Sprintf("console.log(result);\n"))
    case "advanced":
        code.WriteString(fmt.Sprintf("// 高级用法示例\n"))
        code.WriteString(fmt.Sprintf("const options = {\n"))
        code.WriteString(fmt.Sprintf("  timeout: 5000,\n"))
        code.WriteString(fmt.Sprintf("  retries: 3\n"))
        code.WriteString(fmt.Sprintf("};\n"))
        code.WriteString(fmt.Sprintf("const result = %s(options);\n", t.toCamelCase(packageName)))
    case "testing":
        code.WriteString(fmt.Sprintf("// 测试示例\n"))
        code.WriteString(fmt.Sprintf("describe('%s', () => {\n", packageName))
        code.WriteString(fmt.Sprintf("  it('should work correctly', () => {\n"))
        code.WriteString(fmt.Sprintf("    const result = %s();\n", t.toCamelCase(packageName)))
        code.WriteString(fmt.Sprintf("    expect(result).toBeDefined();\n"))
        code.WriteString(fmt.Sprintf("  });\n"))
        code.WriteString(fmt.Sprintf("});\n"))
    case "production":
        code.WriteString(fmt.Sprintf("// 生产环境示例\n"))
        code.WriteString(fmt.Sprintf("const logger = require('logger');\n\n"))
        code.WriteString(fmt.Sprintf("try {\n"))
        code.WriteString(fmt.Sprintf("  const result = await %s();\n", t.toCamelCase(packageName)))
        code.WriteString(fmt.Sprintf("  logger.info('Operation successful', { result });\n"))
        code.WriteString(fmt.Sprintf("} catch (error) {\n"))
        code.WriteString(fmt.Sprintf("  logger.error('Operation failed', { error });\n"))
        code.WriteString(fmt.Sprintf("  throw error;\n"))
        code.WriteString(fmt.Sprintf("}\n"))
    }
    
    return code.String()
}

func (t *CodeGenTool) generatePythonCode(packageName, scenario, framework string, pkg model.Package) string {
    var code strings.Builder
    
    code.WriteString(fmt.Sprintf("import %s\n\n", packageName))
    
    switch scenario {
    case "basic":
        code.WriteString(fmt.Sprintf("# 基础用法示例\n"))
        code.WriteString(fmt.Sprintf("result = %s.main()\n", packageName))
        code.WriteString(fmt.Sprintf("print(result)\n"))
    case "advanced":
        code.WriteString(fmt.Sprintf("# 高级用法示例\n"))
        code.WriteString(fmt.Sprintf("options = {\n"))
        code.WriteString(fmt.Sprintf("    'timeout': 5000,\n"))
        code.WriteString(fmt.Sprintf("    'retries': 3\n"))
        code.WriteString(fmt.Sprintf("}\n"))
        code.WriteString(fmt.Sprintf("result = %s.run(**options)\n", packageName))
    case "testing":
        code.WriteString(fmt.Sprintf("# 测试示例\n"))
        code.WriteString(fmt.Sprintf("import unittest\n\n"))
        code.WriteString(fmt.Sprintf("class Test%s(unittest.TestCase):\n", strings.Title(packageName)))
        code.WriteString(fmt.Sprintf("    def test_basic(self):\n"))
        code.WriteString(fmt.Sprintf("        result = %s.main()\n", packageName))
        code.WriteString(fmt.Sprintf("        self.assertIsNotNone(result)\n"))
    case "production":
        code.WriteString(fmt.Sprintf("# 生产环境示例\n"))
        code.WriteString(fmt.Sprintf("import logging\n\n"))
        code.WriteString(fmt.Sprintf("logger = logging.getLogger(__name__)\n\n"))
        code.WriteString(fmt.Sprintf("try:\n"))
        code.WriteString(fmt.Sprintf("    result = %s.main()\n", packageName))
        code.WriteString(fmt.Sprintf("    logger.info('Operation successful')\n"))
        code.WriteString(fmt.Sprintf("except Exception as e:\n"))
        code.WriteString(fmt.Sprintf("    logger.error('Operation failed', exc_info=True)\n"))
        code.WriteString(fmt.Sprintf("    raise\n"))
    }
    
    return code.String()
}

func (t *CodeGenTool) generateGoCode(packageName, scenario string, pkg model.Package) string {
    var code strings.Builder
    
    code.WriteString(fmt.Sprintf("package main\n\n"))
    code.WriteString(fmt.Sprintf("import \"%s\"\n\n", packageName))
    
    switch scenario {
    case "basic":
        code.WriteString(fmt.Sprintf("func main() {\n"))
        code.WriteString(fmt.Sprintf("    result := %s.Main()\n", t.toPascalCase(packageName)))
        code.WriteString(fmt.Sprintf("    fmt.Println(result)\n"))
        code.WriteString(fmt.Sprintf("}\n"))
    case "advanced":
        code.WriteString(fmt.Sprintf("func main() {\n"))
        code.WriteString(fmt.Sprintf("    options := %s.Options{\n", t.toPascalCase(packageName)))
        code.WriteString(fmt.Sprintf("        Timeout: 5000,\n"))
        code.WriteString(fmt.Sprintf("        Retries: 3,\n"))
        code.WriteString(fmt.Sprintf("    }\n"))
        code.WriteString(fmt.Sprintf("    result := %s.RunWithOptions(options)\n", t.toPascalCase(packageName)))
        code.WriteString(fmt.Sprintf("    fmt.Println(result)\n"))
        code.WriteString(fmt.Sprintf("}\n"))
    case "testing":
        code.WriteString(fmt.Sprintf("func Test%s(t *testing.T) {\n", t.toPascalCase(packageName)))
        code.WriteString(fmt.Sprintf("    result := %s.Main()\n", t.toPascalCase(packageName)))
        code.WriteString(fmt.Sprintf("    if result == \"\" {\n"))
        code.WriteString(fmt.Sprintf("        t.Error(\"Expected non-empty result\")\n"))
        code.WriteString(fmt.Sprintf("    }\n"))
        code.WriteString(fmt.Sprintf("}\n"))
    case "production":
        code.WriteString(fmt.Sprintf("func main() {\n"))
        code.WriteString(fmt.Sprintf("    log := logger.NewLogger()\n\n"))
        code.WriteString(fmt.Sprintf("    result, err := %s.Main()\n", t.toPascalCase(packageName)))
        code.WriteString(fmt.Sprintf("    if err != nil {\n"))
        code.WriteString(fmt.Sprintf("        log.Error(\"Operation failed\", \"error\", err)\n"))
        code.WriteString(fmt.Sprintf("        os.Exit(1)\n"))
        code.WriteString(fmt.Sprintf("    }\n"))
        code.WriteString(fmt.Sprintf("    log.Info(\"Operation successful\", \"result\", result)\n"))
        code.WriteString(fmt.Sprintf("}\n"))
    }
    
    return code.String()
}

func (t *CodeGenTool) generateJavaCode(packageName, scenario, framework string, pkg model.Package) string {
    var code strings.Builder
    
    className := t.toPascalCase(packageName)
    
    code.WriteString(fmt.Sprintf("import %s.*;\n\n", packageName))
    code.WriteString(fmt.Sprintf("public class Main {\n"))
    code.WriteString(fmt.Sprintf("    public static void main(String[] args) {\n"))
    
    switch scenario {
    case "basic":
        code.WriteString(fmt.Sprintf("        // 基础用法示例\n"))
        code.WriteString(fmt.Sprintf("        %s instance = new %s();\n", className, className))
        code.WriteString(fmt.Sprintf("        String result = instance.execute();\n"))
        code.WriteString(fmt.Sprintf("        System.out.println(result);\n"))
    case "advanced":
        code.WriteString(fmt.Sprintf("        // 高级用法示例\n"))
        code.WriteString(fmt.Sprintf("        Config config = new Config.Builder()\n"))
        code.WriteString(fmt.Sprintf("            .setTimeout(5000)\n"))
        code.WriteString(fmt.Sprintf("            .setRetries(3)\n"))
        code.WriteString(fmt.Sprintf("            .build();\n"))
        code.WriteString(fmt.Sprintf("        %s instance = new %s(config);\n", className, className))
        code.WriteString(fmt.Sprintf("        String result = instance.execute();\n"))
    case "testing":
        code.WriteString(fmt.Sprintf("        // 测试示例\n"))
        code.WriteString(fmt.Sprintf("        @Test\n"))
        code.WriteString(fmt.Sprintf("        public void test%s() {\n", className))
        code.WriteString(fmt.Sprintf("            %s instance = new %s();\n", className, className))
        code.WriteString(fmt.Sprintf("            String result = instance.execute();\n"))
        code.WriteString(fmt.Sprintf("            assertNotNull(result);\n"))
        code.WriteString(fmt.Sprintf("        }\n"))
    case "production":
        code.WriteString(fmt.Sprintf("        // 生产环境示例\n"))
        code.WriteString(fmt.Sprintf("        Logger logger = LoggerFactory.getLogger(Main.class);\n\n"))
        code.WriteString(fmt.Sprintf("        try {\n"))
        code.WriteString(fmt.Sprintf("            %s instance = new %s();\n", className, className))
        code.WriteString(fmt.Sprintf("            String result = instance.execute();\n"))
        code.WriteString(fmt.Sprintf("            logger.info(\"Operation successful: {}\", result);\n"))
        code.WriteString(fmt.Sprintf("        } catch (Exception e) {\n"))
        code.WriteString(fmt.Sprintf("            logger.error(\"Operation failed\", e);\n"))
        code.WriteString(fmt.Sprintf("            System.exit(1);\n"))
        code.WriteString(fmt.Sprintf("        }\n"))
    }
    
    code.WriteString(fmt.Sprintf("    }\n"))
    code.WriteString(fmt.Sprintf("}\n"))
    
    return code.String()
}

func (t *CodeGenTool) generateGenericCode(packageName, language, scenario string, pkg model.Package) string {
    return fmt.Sprintf("// %s 使用示例\n// 包类型: %s\n// 语言: %s\n// 场景: %s\n\n// 请参考包的官方文档获取详细使用说明", 
        packageName, language, scenario, pkg.Description)
}

func (t *CodeGenTool) getScenarioDescription(scenario string) string {
    descriptions := map[string]string{
        "basic":       "基础用法示例",
        "advanced":    "高级用法示例",
        "testing":     "单元测试示例",
        "production":  "生产环境示例",
    }
    return descriptions[scenario]
}

func (t *CodeGenTool) getLanguageCode(language string) string {
    codes := map[string]string{
        "javascript": "javascript",
        "typescript": "typescript",
        "python":     "python",
        "java":       "java",
        "go":         "go",
        "csharp":     "csharp",
    }
    if code, ok := codes[language]; ok {
        return code
    }
    return language
}

func (t *CodeGenTool) getTips(packageType, scenario string) string {
    tips := map[string]string{
        "npm":   "- 使用 npm install 或 yarn add 安装依赖\n- 建议锁定版本号以确保环境一致性\n- 定期更新依赖以获取安全补丁",
        "pypi":  "- 使用 pip install 安装依赖\n- 建议使用虚拟环境隔离项目依赖\n- 使用 requirements.txt 管理依赖版本",
        "go":    "- 使用 go get 安装依赖\n- 建议使用 Go Modules 管理依赖\n- 使用 go mod tidy 清理未使用的依赖",
        "maven": "- 在 pom.xml 中添加依赖配置\n- 使用 Maven 管理项目依赖\n- 定期更新依赖版本",
    }
    
    if tip, ok := tips[packageType]; ok {
        return tip
    }
    return "- 请参考包的官方文档获取详细使用说明"
}

func (t *CodeGenTool) toCamelCase(s string) string {
    parts := strings.Split(s, "-")
    for i := 1; i < len(parts); i++ {
        parts[i] = strings.Title(parts[i])
    }
    return strings.Join(parts, "")
}

func (t *CodeGenTool) toPascalCase(s string) string {
    parts := strings.Split(s, "-")
    for i := range parts {
        parts[i] = strings.Title(parts[i])
    }
    return strings.Join(parts, "")
}
```

- [ ] **步骤 2：Commit代码生成工具**

```bash
git add internal/ai/tools/code_gen.go
git commit -m "feat: add code generation tool for AI"
```

---

### 任务 15：实现AI核心服务

**文件：**
- 创建：`internal/ai/service.go`

- [ ] **步骤 1：创建AI核心服务**

```go
package ai

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "github.com/moonlight-box/registry/internal/ai/models"
    "github.com/moonlight-box/registry/internal/config"
    "github.com/moonlight-box/registry/internal/model"
    "github.com/moonlight-box/registry/internal/repository"
    "github.com/sirupsen/logrus"
)

type AIService struct {
    config       *config.AIConfig
    client       *AIClient
    toolManager  *ToolManager
    sessionMgr   *SessionManager
    rateLimiter  *RateLimiter
    sanitizer    *DataSanitizer
    cache        *ResponseCache
    auditRepo    *repository.AuditRepository
    logger       *logrus.Logger
}

func NewAIService(
    cfg *config.AIConfig,
    auditRepo *repository.AuditRepository,
    logPath string,
) *AIService {
    if !cfg.Enabled {
        return nil
    }
    
    client := NewAIClient(cfg)
    sessionMgr := NewSessionManager(&cfg.Session)
    rateLimiter := NewRateLimiter(&cfg.RateLimit)
    sanitizer := NewDataSanitizer()
    cache := NewResponseCache(&cfg.Cache)
    toolManager := NewToolManager(auditRepo)
    
    return &AIService{
        config:      cfg,
        client:      client,
        toolManager: toolManager,
        sessionMgr:  sessionMgr,
        rateLimiter: rateLimiter,
        sanitizer:   sanitizer,
        cache:       cache,
        auditRepo:   auditRepo,
        logger:      logrus.New(),
    }
}

func (s *AIService) RegisterTool(tool Tool, allowedRoles []string) {
    if s == nil {
        return
    }
    s.toolManager.RegisterTool(tool, allowedRoles)
}

func (s *AIService) Chat(
    ctx context.Context,
    userMessage string,
    user *model.User,
    sessionID string,
) (*ChatResponse, error) {
    if s == nil {
        return nil, fmt.Errorf("AI service is not enabled")
    }
    
    if !s.rateLimiter.Allow(user.ID) {
        return nil, fmt.Errorf("rate limit exceeded, please try again later")
    }
    
    sanitizedMessage := s.sanitizer.Sanitize(userMessage)
    
    if s.cache != nil {
        if cached, ok := s.cache.Get(sanitizedMessage); ok {
            return &ChatResponse{
                Message:  cached,
                Cached:   true,
            }, nil
        }
    }
    
    session := s.sessionMgr.GetOrCreateSession(user.ID, sessionID)
    
    messages := s.buildMessages(session, sanitizedMessage)
    
    tools := s.toolManager.GetToolDefinitions()
    
    req := models.ChatRequest{
        Model:       s.config.Model,
        Messages:    messages,
        Tools:       tools,
        Temperature: s.config.Temperature,
        MaxTokens:   s.config.MaxTokens,
    }
    
    startTime := time.Now()
    resp, err := s.callWithRetry(ctx, req)
    if err != nil {
        s.logger.WithError(err).Error("AI model call failed")
        return nil, err
    }
    
    var toolCallResults []ToolCallResult
    if resp.Choices[0].Message.ToolCalls != nil && len(resp.Choices[0].Message.ToolCalls) > 0 {
        toolCallResults = s.executeToolCalls(ctx, resp.Choices[0].Message.ToolCalls, user)
        
        messages = append(messages,
            models.Message{
                Role:      "assistant",
                ToolCalls: resp.Choices[0].Message.ToolCalls,
            },
            models.Message{
                Role:    "tool",
                Content: s.formatToolResults(toolCallResults),
            },
        )
        
        req.Messages = messages
        req.Tools = nil
        
        resp, err = s.callWithRetry(ctx, req)
        if err != nil {
            s.logger.WithError(err).Error("AI model call failed after tool execution")
            return nil, err
        }
    }
    
    duration := time.Since(startTime)
    s.rateLimiter.Record(user.ID, resp.Usage.TotalTokens)
    
    s.sessionMgr.AddMessage(session.ID, models.Message{
        Role:    "user",
        Content: sanitizedMessage,
    })
    s.sessionMgr.AddMessage(session.ID, resp.Choices[0].Message)
    
    if s.cache != nil {
        s.cache.Set(sanitizedMessage, resp.Choices[0].Message.Content)
    }
    
    s.logger.WithFields(logrus.Fields{
        "user_id":     user.ID,
        "session_id":  session.ID,
        "duration":    duration,
        "tokens":      resp.Usage.TotalTokens,
        "tool_calls":  len(toolCallResults),
    }).Info("AI chat completed")
    
    return &ChatResponse{
        SessionID:   session.ID,
        Message:     resp.Choices[0].Message.Content,
        ToolCalls:   toolCallResults,
        Timestamp:   time.Now().Unix(),
        TokensUsed:  resp.Usage.TotalTokens,
    }, nil
}

func (s *AIService) buildMessages(session *Session, userMessage string) []models.Message {
    messages := []models.Message{
        {
            Role:    "system",
            Content: s.getSystemPrompt(),
        },
    }
    
    if session != nil && len(session.Messages) > 0 {
        messages = append(messages, session.Messages...)
    }
    
    messages = append(messages, models.Message{
        Role:    "user",
        Content: userMessage,
    })
    
    return messages
}

func (s *AIService) getSystemPrompt() string {
    return `你是Moonlight仓库管理系统的AI助手。

你的职责：
1. 帮助用户查询包信息、解决依赖问题
2. 分析安全漏洞并提供修复建议
3. 排查系统问题、分析日志
4. 生成代码示例和使用指导

你可以使用以下工具：
- query_package_info: 查询包的详细信息
- query_logs: 查询系统日志
- query_database: 查询数据库信息
- analyze_security: 分析安全问题
- generate_demo_code: 生成示例代码

注意事项：
- 只回答与仓库管理相关的问题
- 对于敏感操作，提醒用户确认
- 如果不确定，请明确告知用户
- 使用简洁、专业的语言回答`
}

func (s *AIService) executeToolCalls(ctx context.Context, toolCalls []models.ToolCall, user *model.User) []ToolCallResult {
    var results []ToolCallResult
    
    for _, call := range toolCalls {
        startTime := time.Now()
        
        result, err := s.toolManager.ExecuteTool(ctx, call.Function.Name, call.Function.Arguments, user)
        
        duration := time.Since(startTime)
        
        toolResult := ToolCallResult{
            Name:      call.Function.Name,
            Params:    call.Function.Arguments,
            Result:    result,
            Duration:  duration.Milliseconds(),
        }
        
        if err != nil {
            toolResult.Error = err.Error()
            toolResult.Success = false
        } else {
            toolResult.Success = true
        }
        
        results = append(results, toolResult)
    }
    
    return results
}

func (s *AIService) formatToolResults(results []ToolCallResult) string {
    var formattedResults []string
    for _, result := range results {
        if result.Success {
            formattedResults = append(formattedResults, 
                fmt.Sprintf("Tool: %s\nResult: %s", result.Name, result.Result))
        } else {
            formattedResults = append(formattedResults,
                fmt.Sprintf("Tool: %s\nError: %s", result.Name, result.Error))
        }
    }
    return strings.Join(formattedResults, "\n\n")
}

func (s *AIService) callWithRetry(ctx context.Context, req models.ChatRequest) (*models.ChatResponse, error) {
    maxAttempts := 3
    delay := 1 * time.Second
    
    var lastErr error
    
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        resp, err := s.client.Call(ctx, req)
        if err == nil {
            return resp, nil
        }
        
        lastErr = err
        
        if !s.isRetryableError(err) {
            return nil, err
        }
        
        if attempt < maxAttempts {
            s.logger.WithFields(logrus.Fields{
                "attempt": attempt,
                "error":   err,
            }).Warn("Retrying AI model call")
            
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delay):
                delay *= 2
            }
        }
    }
    
    return nil, fmt.Errorf("max retry attempts reached: %w", lastErr)
}

func (s *AIService) isRetryableError(err error) bool {
    errStr := err.Error()
    retryableErrors := []string{
        "timeout",
        "connection refused",
        "too many requests",
        "service unavailable",
    }
    
    for _, retryable := range retryableErrors {
        if strings.Contains(strings.ToLower(errStr), retryable) {
            return true
        }
    }
    
    return false
}

func (s *AIService) ListTools() []ToolInfo {
    if s == nil {
        return nil
    }
    return s.toolManager.ListTools()
}

func (s *AIService) ClearSession(sessionID string) {
    if s == nil {
        return
    }
    s.sessionMgr.DeleteSession(sessionID)
}

type ChatResponse struct {
    SessionID  string            `json:"session_id"`
    Message    string            `json:"message"`
    ToolCalls  []ToolCallResult  `json:"tool_calls,omitempty"`
    Timestamp  int64             `json:"timestamp"`
    TokensUsed int               `json:"tokens_used"`
    Cached     bool              `json:"cached"`
}

type ToolCallResult struct {
    Name     string                 `json:"name"`
    Params   map[string]interface{} `json:"params"`
    Result   string                 `json:"result"`
    Error    string                 `json:"error,omitempty"`
    Success  bool                   `json:"success"`
    Duration int64                  `json:"duration_ms"`
}
```

- [ ] **步骤 2：Commit AI核心服务**

```bash
git add internal/ai/service.go
git commit -m "feat: add AI core service with tool calling support"
```

---

### 任务 16：实现AI HTTP接口

**文件：**
- 创建：`internal/handler/ai_handler.go`

- [ ] **步骤 1：创建AI HTTP接口**

```go
package handler

import (
    "net/http"
    
    "github.com/gin-gonic/gin"
    "github.com/moonlight-box/registry/internal/ai"
    "github.com/moonlight-box/registry/internal/middleware"
)

type AIHandler struct {
    aiService *ai.AIService
}

func NewAIHandler(aiService *ai.AIService) *AIHandler {
    return &AIHandler{
        aiService: aiService,
    }
}

func (h *AIHandler) RegisterRoutes(r *gin.RouterGroup) {
    ai := r.Group("/ai")
    {
        ai.POST("/chat", h.Chat)
        ai.GET("/tools", h.ListTools)
        ai.DELETE("/sessions/:id", h.ClearSession)
    }
}

type ChatRequest struct {
    SessionID string `json:"session_id"`
    Message   string `json:"message" binding:"required"`
}

func (h *AIHandler) Chat(c *gin.Context) {
    if h.aiService == nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{
            "error": "AI service is not enabled",
        })
        return
    }
    
    var req ChatRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "invalid request",
            "details": err.Error(),
        })
        return
    }
    
    user := middleware.GetCurrentUser(c)
    if user == nil {
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": "unauthorized",
        })
        return
    }
    
    response, err := h.aiService.Chat(
        c.Request.Context(),
        req.Message,
        user,
        req.SessionID,
    )
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": err.Error(),
        })
        return
    }
    
    c.JSON(http.StatusOK, response)
}

func (h *AIHandler) ListTools(c *gin.Context) {
    if h.aiService == nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{
            "error": "AI service is not enabled",
        })
        return
    }
    
    tools := h.aiService.ListTools()
    
    c.JSON(http.StatusOK, gin.H{
        "tools": tools,
    })
}

func (h *AIHandler) ClearSession(c *gin.Context) {
    if h.aiService == nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{
            "error": "AI service is not enabled",
        })
        return
    }
    
    sessionID := c.Param("id")
    
    h.aiService.ClearSession(sessionID)
    
    c.JSON(http.StatusOK, gin.H{
        "message": "session cleared",
    })
}
```

- [ ] **步骤 2：Commit AI HTTP接口**

```bash
git add internal/handler/ai_handler.go
git commit -m "feat: add AI HTTP endpoints"
```

---

### 任务 17：集成AI服务到主程序

**文件：**
- 修改：`cmd/registry/main.go`

- [ ] **步骤 1：在main.go中初始化AI服务**

在 `main` 函数中，在初始化其他服务之后添加：

```go
// 初始化 AI 服务
var aiService *ai.AIService
if cfg.AI.Enabled {
    aiService = ai.NewAIService(&cfg.AI, auditRepo, cfg.Storage.Local.BasePath+"/logs")
    
    // 注册工具
    toolCtx := &tools.ToolContext{
        DB:      db,
        Config:  cfg,
        LogPath: cfg.Storage.Local.BasePath + "/logs",
    }
    
    // 日志查询工具 - 仅管理员可用
    logQueryTool := &tools.LogQueryTool{}
    logQueryTool.SetContext(toolCtx)
    aiService.RegisterTool(logQueryTool, []string{"admin"})
    
    // 数据库查询工具 - 所有用户可用
    dbQueryTool := &tools.DatabaseQueryTool{}
    dbQueryTool.SetContext(toolCtx)
    aiService.RegisterTool(dbQueryTool, nil)
    
    // 包信息查询工具 - 所有用户可用
    packageInfoTool := &tools.PackageInfoTool{}
    packageInfoTool.SetContext(toolCtx)
    aiService.RegisterTool(packageInfoTool, nil)
    
    // 安全分析工具 - 需要安全查看权限
    securityTool := &tools.SecurityAnalysisTool{}
    securityTool.SetContext(toolCtx)
    aiService.RegisterTool(securityTool, []string{"admin", "security"})
    
    // 代码生成工具 - 所有用户可用
    codeGenTool := &tools.CodeGenTool{}
    codeGenTool.SetContext(toolCtx)
    aiService.RegisterTool(codeGenTool, nil)
    
    fmt.Println("AI service initialized")
}
```

- [ ] **步骤 2：在setupRouter中注册AI路由**

在 `setupRouter` 函数的 `protected` 路由组中添加：

```go
// AI 助手
if aiService != nil {
    aiHandler := handler.NewAIHandler(aiService)
    aiGroup := protected.Group("/ai")
    {
        aiGroup.POST("/chat", aiHandler.Chat)
        aiGroup.GET("/tools", aiHandler.ListTools)
        aiGroup.DELETE("/sessions/:id", aiHandler.ClearSession)
    }
}
```

- [ ] **步骤 3：添加必要的import**

在文件顶部添加：

```go
import (
    // ... 现有的 import
    "github.com/moonlight-box/registry/internal/ai"
    "github.com/moonlight-box/registry/internal/ai/tools"
)
```

- [ ] **步骤 4：Commit主程序集成**

```bash
git add cmd/registry/main.go
git commit -m "feat: integrate AI service into main application"
```

---

### 任务 18：编写单元测试

**文件：**
- 创建：`internal/ai/service_test.go`

- [ ] **步骤 1：编写AI服务测试**

```go
package ai

import (
    "context"
    "testing"
    "time"
    
    "github.com/moonlight-box/registry/internal/config"
    "github.com/moonlight-box/registry/internal/model"
    "github.com/stretchr/testify/assert"
)

func TestNewAIService(t *testing.T) {
    cfg := &config.AIConfig{
        Enabled:     true,
        Provider:    "chatglm",
        BaseURL:     "http://localhost:8000/v1",
        Model:       "chatglm3-6b",
        MaxTokens:   2048,
        Temperature: 0.7,
        Timeout:     30 * time.Second,
    }
    
    service := NewAIService(cfg, nil, "/tmp/logs")
    
    assert.NotNil(t, service)
    assert.NotNil(t, service.client)
    assert.NotNil(t, service.sessionMgr)
    assert.NotNil(t, service.rateLimiter)
}

func TestNewAIService_Disabled(t *testing.T) {
    cfg := &config.AIConfig{
        Enabled: false,
    }
    
    service := NewAIService(cfg, nil, "/tmp/logs")
    
    assert.Nil(t, service)
}

func TestRateLimiter(t *testing.T) {
    cfg := &config.RateLimitConfig{
        RequestsPerMinute: 10,
        RequestsPerDay:    100,
        TokensPerDay:      10000,
    }
    
    limiter := NewRateLimiter(cfg)
    
    userID := uint(1)
    
    for i := 0; i < 10; i++ {
        assert.True(t, limiter.Allow(userID))
        limiter.Record(userID, 100)
    }
    
    assert.False(t, limiter.Allow(userID))
}

func TestDataSanitizer(t *testing.T) {
    sanitizer := NewDataSanitizer()
    
    input := "password=secret123 api_key=sk-1234567890"
    output := sanitizer.Sanitize(input)
    
    assert.NotContains(t, output, "secret123")
    assert.NotContains(t, output, "sk-1234567890")
    assert.Contains(t, output, "[REDACTED]")
}

func TestSessionManager(t *testing.T) {
    cfg := &config.SessionConfig{
        MaxAge:      24 * time.Hour,
        MaxMessages: 50,
    }
    
    sm := NewSessionManager(cfg)
    
    userID := uint(1)
    session := sm.GetOrCreateSession(userID, "")
    
    assert.NotNil(t, session)
    assert.NotEmpty(t, session.ID)
    assert.Equal(t, userID, session.UserID)
    
    sm.AddMessage(session.ID, model.Message{
        Role:    "user",
        Content: "test message",
    })
    
    retrieved, exists := sm.GetSession(session.ID)
    assert.True(t, exists)
    assert.Len(t, retrieved.Messages, 1)
}

func TestResponseCache(t *testing.T) {
    cfg := &config.CacheConfig{
        Enabled: true,
        TTL:     1 * time.Hour,
        MaxSize: 100,
    }
    
    cache := NewResponseCache(cfg)
    
    query := "test query"
    response := "test response"
    
    _, exists := cache.Get(query)
    assert.False(t, exists)
    
    cache.Set(query, response)
    
    cached, exists := cache.Get(query)
    assert.True(t, exists)
    assert.Equal(t, response, cached)
}
```

- [ ] **步骤 2：运行测试验证**

```bash
go test ./internal/ai -v
```

预期：所有测试通过

- [ ] **步骤 3：Commit测试**

```bash
git add internal/ai/service_test.go
git commit -m "test: add unit tests for AI service"
```

---

### 任务 19：编写集成测试

**文件：**
- 创建：`internal/handler/ai_handler_test.go`

- [ ] **步骤 1：编写HTTP接口测试**

```go
package handler

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/gin-gonic/gin"
    "github.com/moonlight-box/registry/internal/ai"
    "github.com/moonlight-box/registry/internal/config"
    "github.com/moonlight-box/registry/internal/middleware"
    "github.com/moonlight-box/registry/internal/model"
    "github.com/moonlight-box/registry/internal/repository"
    "github.com/stretchr/testify/assert"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

func setupTestRouter(t *testing.T) *gin.Engine {
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    assert.NoError(t, err)
    
    db.AutoMigrate(&model.User{}, &model.Role{})
    
    user := &model.User{
        Username: "testuser",
        Email:    "test@example.com",
    }
    db.Create(user)
    
    cfg := &config.AIConfig{
        Enabled:     true,
        Provider:    "chatglm",
        BaseURL:     "http://localhost:8000/v1",
        Model:       "chatglm3-6b",
        MaxTokens:   2048,
        Temperature: 0.7,
    }
    
    aiService := ai.NewAIService(cfg, nil, "/tmp/logs")
    
    router := gin.New()
    
    api := router.Group("/api/v1")
    api.Use(func(c *gin.Context) {
        c.Set("user", user)
        c.Next()
    })
    
    aiHandler := NewAIHandler(aiService)
    aiHandler.RegisterRoutes(api)
    
    return router
}

func TestAIHandler_Chat(t *testing.T) {
    router := setupTestRouter(t)
    
    reqBody := ChatRequest{
        Message: "test message",
    }
    body, _ := json.Marshal(reqBody)
    
    req := httptest.NewRequest("POST", "/api/v1/ai/chat", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusOK, w.Code)
}

func TestAIHandler_ListTools(t *testing.T) {
    router := setupTestRouter(t)
    
    req := httptest.NewRequest("GET", "/api/v1/ai/tools", nil)
    
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusOK, w.Code)
    
    var response map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &response)
    
    assert.Contains(t, response, "tools")
}
```

- [ ] **步骤 2：运行测试验证**

```bash
go test ./internal/handler -v
```

预期：所有测试通过

- [ ] **步骤 3：Commit集成测试**

```bash
git add internal/handler/ai_handler_test.go
git commit -m "test: add integration tests for AI handler"
```

---

### 任务 20：创建配置文件并测试

**文件：**
- 创建：`configs/config.yaml`（用于测试）

- [ ] **步骤 1：创建测试配置文件**

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "debug"

database:
  driver: "sqlite"
  dsn: "test.db"

storage:
  backend: "local"
  local:
    base_path: "./data"
    max_size_gb: 10

auth:
  jwt_secret: "test-secret-key"
  token_expiry: "24h"

ai:
  enabled: true
  provider: "chatglm"
  base_url: "http://localhost:8000/v1"
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

- [ ] **步骤 2：编译并运行程序**

```bash
go build -o registry ./cmd/registry
./registry -config configs/config.yaml
```

预期：程序成功启动，输出 "AI service initialized"

- [ ] **步骤 3：测试AI接口**

```bash
# 测试聊天接口
curl -X POST http://localhost:8080/api/v1/ai/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"message": "查询lodash包的信息"}'

# 测试工具列表接口
curl -X GET http://localhost:8080/api/v1/ai/tools \
  -H "Authorization: Bearer YOUR_TOKEN"
```

预期：返回正确的响应

- [ ] **步骤 4：Commit配置文件**

```bash
git add configs/config.yaml
git commit -m "chore: add test configuration file"
```

---

### 任务 21：编写文档

**文件：**
- 创建：`docs/ai-integration.md`

- [ ] **步骤 1：编写使用文档**

```markdown
# AI 集成使用指南

## 概述

Moonlight Registry 集成了 AI 功能，为管理员和开发者提供智能辅助能力。

## 功能特性

- **问题排查**：查询日志、分析错误、诊断问题
- **包信息查询**：查询包的详细信息、版本历史、依赖关系
- **安全分析**：分析漏洞、提供修复建议
- **代码生成**：生成包的使用示例代码

## 配置

在 `configs/config.yaml` 中配置 AI 服务：

```yaml
ai:
  enabled: true
  provider: "chatglm"
  base_url: "http://192.168.1.100:8000/v1"
  model: "chatglm3-6b"
  max_tokens: 2048
  temperature: 0.7
```

## API 接口

### 聊天接口

**POST /api/v1/ai/chat**

请求：
```json
{
  "session_id": "optional-session-id",
  "message": "查询lodash包的信息"
}
```

响应：
```json
{
  "session_id": "session-123",
  "message": "📦 lodash@4.17.21...",
  "tool_calls": [...],
  "timestamp": 1234567890
}
```

### 工具列表接口

**GET /api/v1/ai/tools**

响应：
```json
{
  "tools": [
    {
      "name": "query_logs",
      "description": "查询系统日志",
      "parameters": {...}
    }
  ]
}
```

## 使用示例

### 查询包信息

```
用户：查询lodash包的信息
AI：📦 lodash@4.17.21
类型：npm
描述：Lodash modular utilities...
```

### 排查问题

```
用户：最近有哪些错误日志？
AI：找到15条错误日志：
[1] 2026-05-02 10:30:15 [error] proxy - Connection failed...
```

### 安全分析

```
用户：lodash包有安全漏洞吗？
AI：🔒 安全扫描结果：
📦 lodash@4.17.20
🔴 严重漏洞：1
CVE-2020-8203...
```

### 代码生成

```
用户：生成express的使用示例
AI：💻 express (javascript) 使用示例
场景：基础用法示例
```javascript
const express = require('express');
const app = express();
...
```
```

## 权限控制

不同工具需要不同的权限：

- `query_logs` - 仅管理员可用
- `query_database` - 所有用户可用
- `query_package_info` - 所有用户可用
- `analyze_security` - 管理员或安全角色可用
- `generate_demo_code` - 所有用户可用

## 限流策略

- 每分钟最多 20 次请求
- 每天最多 500 次请求
- 每天最多 100,000 tokens

## 故障排查

### AI 服务未启用

检查配置文件中 `ai.enabled` 是否为 `true`。

### 限流错误

等待一段时间后重试，或联系管理员提升配额。

### 工具调用失败

检查用户权限和工具参数是否正确。

## 最佳实践

1. 使用会话ID保持对话上下文
2. 提供清晰、具体的问题描述
3. 定期清理会话历史
4. 关注安全扫描结果并及时修复
```

- [ ] **步骤 2：Commit文档**

```bash
git add docs/ai-integration.md
git commit -m "docs: add AI integration usage guide"
```

---

### 任务 22：最终验证和清理

- [ ] **步骤 1：运行所有测试**

```bash
go test ./... -v
```

预期：所有测试通过

- [ ] **步骤 2：检查代码质量**

```bash
go fmt ./...
go vet ./...
```

预期：无错误

- [ ] **步骤 3：构建生产版本**

```bash
go build -o registry ./cmd/registry
```

预期：构建成功

- [ ] **步骤 4：创建最终commit**

```bash
git add .
git commit -m "feat: complete AI integration implementation"
```

---

## 执行总结

完成以上所有任务后，AI集成功能将完全实现，包括：

✅ AI服务核心框架
✅ 工具调用系统（日志查询、数据库查询、包信息、安全分析、代码生成）
✅ 会话管理
✅ 限流保护
✅ 数据脱敏
✅ 响应缓存
✅ HTTP API接口
✅ 权限控制
✅ 单元测试和集成测试
✅ 使用文档

**后续工作：**
- 前端界面开发（需要单独的计划）
- CLI工具实现（可选）
- 性能优化和监控
- 用户反馈收集和分析
