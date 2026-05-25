# Moonlight Registry AI 集成文档

## 目录

- [功能概述](#功能概述)
- [架构设计](#架构设计)
- [配置说明](#配置说明)
- [API接口文档](#api接口文档)
- [工具使用说明](#工具使用说明)
- [示例代码](#示例代码)
- [最佳实践](#最佳实践)
- [故障排查](#故障排查)

---

## 功能概述

Moonlight Registry AI 集成为私有仓库管理系统提供了智能化的运维和管理能力。通过集成AI服务，用户可以使用自然语言与系统交互，执行复杂的查询和分析任务。

### 核心能力

1. **智能对话**
   - 自然语言交互
   - 上下文感知的多轮对话
   - 会话管理

2. **工具调用**
   - 日志查询与分析
   - 数据库统计查询
   - 包信息查询
   - 安全分析
   - 代码生成

3. **安全与限流**
   - 用户级别的请求限流
   - 敏感信息脱敏
   - 工具调用审计

4. **性能优化**
   - 响应缓存
   - 会话管理
   - 并发控制

---

## 架构设计

### 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                        客户端层                               │
│                   (Web UI / API Client)                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      HTTP Handler 层                         │
│                    (ai_handler.go)                           │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │  Chat    │  │  Tools   │  │ Session  │  │  Stats   │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      AI Service 层                           │
│                    (service.go)                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Client     │  │   Session    │  │  RateLimiter │     │
│  │  (AI调用)    │  │  (会话管理)  │  │  (限流控制)  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Cache      │  │  Sanitizer   │  │ToolManager   │     │
│  │  (缓存)      │  │  (脱敏)      │  │  (工具管理)  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      Tools 层                                │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌──────────┐│
│  │ LogQuery   │ │ DBQuery    │ │PackageInfo │ │Security  ││
│  │  Tool      │ │  Tool      │ │   Tool     │ │Analysis  ││
│  └────────────┘ └────────────┘ └────────────┘ └──────────┘│
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    外部服务层                                 │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐              │
│  │ AI Provider│ │  Database  │ │   Logs     │              │
│  │(ChatGLM等) │ │  (SQLite)  │ │  Files     │              │
│  └────────────┘ └────────────┘ └────────────┘              │
└─────────────────────────────────────────────────────────────┘
```

### 核心组件说明

#### 1. AI Service (`internal/ai/service.go`)
核心服务协调器，负责：
- 协调各个子组件
- 处理聊天请求
- 管理工具调用循环
- 提供统一的服务接口

#### 2. AI Client (`internal/ai/client.go`)
AI服务客户端，负责：
- 与AI提供商通信
- 构建和解析请求/响应
- 处理流式和非流式响应

#### 3. Session Manager (`internal/ai/session_manager.go`)
会话管理器，负责：
- 创建和管理用户会话
- 维护对话历史
- 会话过期清理

#### 4. Rate Limiter (`internal/ai/rate_limiter.go`)
限流器，负责：
- 用户级别的请求限流
- Token使用量统计
- 限流状态查询

#### 5. Tool Manager (`internal/ai/tool_manager.go`)
工具管理器，负责：
- 工具注册和管理
- 工具执行和权限控制
- 审计日志记录

#### 6. Response Cache (`internal/ai/cache.go`)
响应缓存，负责：
- 缓存AI响应
- 缓存命中查询
- 缓存清理

#### 7. Sanitizer (`internal/ai/sanitizer.go`)
脱敏器，负责：
- 敏感信息识别和替换
- 工具结果脱敏
- 用户输入清理

---

## 配置说明

### 配置文件结构

配置文件位于 `configs/ai-config.yaml`，包含以下主要部分：

```yaml
ai:
  enabled: true                    # 是否启用AI服务
  provider: "chatglm"              # AI提供商
  base_url: "http://localhost:8000/v1"  # API地址
  api_key: ""                      # API密钥
  model: "chatglm3-6b"            # 模型名称
  max_tokens: 2048                # 最大token数
  temperature: 0.7                # 温度参数
  timeout: 30s                    # 超时时间

  tools:                          # 工具配置
    enabled: true
    allowed_tools: [...]
    max_execution_time: 10
    enable_audit_log: true

  rate_limit:                     # 限流配置
    requests_per_minute: 20
    requests_per_day: 500
    tokens_per_day: 100000

  cache:                          # 缓存配置
    enabled: true
    ttl: 1h
    max_size: 1000

  session:                        # 会话配置
    max_age: 24h
    max_messages: 50
```

### 配置项详解

#### AI服务基础配置

| 配置项 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `enabled` | bool | 是 | 是否启用AI服务 |
| `provider` | string | 是 | AI提供商：chatglm, openai, anthropic, custom |
| `base_url` | string | 是 | AI服务API地址 |
| `api_key` | string | 否 | API密钥（建议使用环境变量） |
| `model` | string | 是 | 模型名称 |
| `max_tokens` | int | 否 | 最大生成token数，默认2048 |
| `temperature` | float | 否 | 温度参数(0.0-2.0)，默认0.7 |
| `timeout` | duration | 否 | 请求超时时间，默认30s |

#### 工具配置

| 配置项 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `tools.enabled` | bool | 否 | 是否启用工具调用，默认true |
| `tools.allowed_tools` | []string | 否 | 允许的工具列表，空表示允许所有 |
| `tools.max_execution_time` | int | 否 | 工具最大执行时间(秒)，默认10 |
| `tools.enable_audit_log` | bool | 否 | 是否启用审计日志，默认true |

#### 限流配置

| 配置项 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `rate_limit.requests_per_minute` | int | 否 | 每分钟请求限制，默认20 |
| `rate_limit.requests_per_day` | int | 否 | 每天请求限制，默认500 |
| `rate_limit.tokens_per_day` | int | 否 | 每天token限制，默认100000 |

#### 缓存配置

| 配置项 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `cache.enabled` | bool | 否 | 是否启用缓存，默认true |
| `cache.ttl` | duration | 否 | 缓存有效期，默认1h |
| `cache.max_size` | int | 否 | 最大缓存条目数，默认1000 |

#### 会话配置

| 配置项 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `session.max_age` | duration | 否 | 会话最大存活时间，默认24h |
| `session.max_messages` | int | 否 | 会话最大消息数，默认50 |

### 环境变量

支持通过环境变量覆盖配置：

```bash
# API密钥（推荐）
export MOONLIGHT_AI_API_KEY="your-api-key"

# AI服务地址
export MOONLIGHT_AI_BASE_URL="http://localhost:8000/v1"

# AI提供商
export MOONLIGHT_AI_PROVIDER="chatglm"
```

---

## API接口文档

### 基础信息

- **Base URL**: `http://localhost:9081/api/v1`
- **认证方式**: JWT Token
- **请求格式**: JSON
- **响应格式**: JSON

### 通用响应格式

```json
{
  "code": 200,
  "message": "success",
  "data": { ... }
}
```

错误响应：
```json
{
  "code": 400,
  "message": "错误描述"
}
```

### 接口列表

#### 1. AI聊天

**POST** `/api/v1/ai/chat`

与AI助手进行对话。

**请求参数**：

```json
{
  "session_id": "可选，会话ID",
  "message": "必需，用户消息"
}
```

**响应示例**：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "session_id": "sess_abc123",
    "message": "AI的回复内容",
    "tool_calls": [
      {
        "name": "query_logs",
        "params": {
          "level": "error",
          "limit": 10
        },
        "result": "查询结果..."
      }
    ],
    "usage": {
      "prompt_tokens": 150,
      "completion_tokens": 80,
      "total_tokens": 230
    }
  }
}
```

**状态码**：
- `200`: 成功
- `400`: 请求参数错误
- `401`: 未授权
- `429`: 请求过于频繁
- `500`: 服务器错误

#### 2. 获取工具列表

**GET** `/api/v1/ai/tools`

获取所有可用的AI工具。

**响应示例**：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "tools": [
      {
        "name": "query_logs",
        "description": "查询系统日志",
        "parameters": { ... }
      },
      {
        "name": "query_database",
        "description": "查询数据库统计",
        "parameters": { ... }
      }
    ]
  }
}
```

#### 3. 删除会话

**DELETE** `/api/v1/ai/sessions/:id`

删除指定的AI会话。

**路径参数**：
- `id`: 会话ID

**响应示例**：

```json
{
  "code": 204,
  "message": "删除成功"
}
```

**状态码**：
- `204`: 删除成功
- `401`: 未授权
- `403`: 无权删除此会话
- `404`: 会话不存在

#### 4. 获取限流状态

**GET** `/api/v1/ai/rate-limit`

获取当前用户的AI请求限流状态。

**响应示例**：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "minute_limit": 20,
    "minute_used": 5,
    "minute_remaining": 15,
    "day_limit": 500,
    "day_used": 50,
    "day_remaining": 450,
    "token_limit": 100000,
    "token_used": 5000,
    "token_remaining": 95000
  }
}
```

#### 5. 获取服务统计

**GET** `/api/v1/ai/stats`

获取AI服务的统计信息。

**响应示例**：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "session_count": 10,
    "tool_count": 5,
    "cache_stats": {
      "enabled": true,
      "size": 50,
      "max_size": 1000,
      "hits": 100,
      "misses": 50,
      "hit_rate": 0.67
    },
    "audit_log_count": 25
  }
}
```

#### 6. 获取缓存统计

**GET** `/api/v1/ai/cache/stats`

获取AI响应缓存的统计信息。

**响应示例**：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "enabled": true,
    "size": 50,
    "max_size": 1000,
    "hits": 100,
    "misses": 50,
    "hit_rate": 0.67
  }
}
```

#### 7. 获取审计日志

**GET** `/api/v1/ai/audit-logs`

获取AI工具调用的审计日志。

**查询参数**：
- `limit`: 返回数量限制，默认100

**响应示例**：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "logs": [
      {
        "timestamp": "2026-05-02T10:30:00Z",
        "user_id": 1,
        "username": "admin",
        "tool_name": "query_logs",
        "params": { ... },
        "result": "...",
        "error": ""
      }
    ]
  }
}
```

#### 8. 健康检查

**GET** `/api/v1/ai/health`

检查AI服务是否正常。

**响应示例**：

成功：
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "status": "healthy"
  }
}
```

失败：
```json
{
  "status": "unhealthy",
  "error": "数据库连接异常"
}
```

---

## 工具使用说明

### 工具列表

#### 1. 日志查询工具 (query_logs)

查询系统日志，支持多条件过滤。

**参数**：

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| start_time | string | 否 | 开始时间 (格式: 2006-01-02 15:04:05) |
| end_time | string | 否 | 结束时间 (格式: 2006-01-02 15:04:05) |
| level | string | 否 | 日志级别 (debug, info, warn, error) |
| keyword | string | 否 | 关键词搜索 |
| source | string | 否 | 日志来源 |
| limit | int | 否 | 返回结果数量限制，默认100 |

**示例**：

```json
{
  "name": "query_logs",
  "params": {
    "level": "error",
    "start_time": "2026-05-01 00:00:00",
    "end_time": "2026-05-02 23:59:59",
    "keyword": "failed",
    "limit": 20
  }
}
```

#### 2. 数据库查询工具 (query_database)

查询数据库统计信息。

**参数**：

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| query_type | string | 是 | 查询类型 (stats, count, list) |
| table | string | 否 | 表名 |
| condition | string | 否 | 查询条件 |

**示例**：

```json
{
  "name": "query_database",
  "params": {
    "query_type": "stats",
    "table": "packages"
  }
}
```

#### 3. 包信息查询工具 (query_package_info)

查询包的详细信息。

**参数**：

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| name | string | 是 | 包名 |
| version | string | 否 | 版本号 |
| repository | string | 否 | 仓库名 |

**示例**：

```json
{
  "name": "query_package_info",
  "params": {
    "name": "lodash",
    "version": "4.17.21"
  }
}
```

#### 4. 安全分析工具 (analyze_security)

执行安全分析。

**参数**：

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| target | string | 是 | 分析目标 (package, repository, system) |
| name | string | 否 | 包名或仓库名 |
| severity | string | 否 | 严重级别过滤 |

**示例**：

```json
{
  "name": "analyze_security",
  "params": {
    "target": "package",
    "name": "lodash",
    "severity": "high"
  }
}
```

#### 5. 代码生成工具 (generate_demo_code)

生成示例代码。

**参数**：

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| language | string | 是 | 编程语言 |
| package_name | string | 否 | 包名 |
| use_case | string | 否 | 使用场景 |

**示例**：

```json
{
  "name": "generate_demo_code",
  "params": {
    "language": "javascript",
    "package_name": "lodash",
    "use_case": "array manipulation"
  }
}
```

### 工具调用流程

1. **用户发送消息**：用户通过自然语言描述需求
2. **AI分析意图**：AI判断是否需要调用工具
3. **生成工具调用**：AI生成工具调用请求
4. **执行工具**：系统执行相应的工具
5. **返回结果**：工具执行结果返回给AI
6. **生成回复**：AI基于工具结果生成最终回复

---

## 示例代码

### JavaScript/TypeScript 示例

#### 基础聊天

```javascript
const axios = require('axios');

const API_BASE = 'http://localhost:9081/api/v1';
const JWT_TOKEN = 'your-jwt-token';

async function chat(message, sessionId = null) {
  try {
    const response = await axios.post(
      `${API_BASE}/ai/chat`,
      {
        session_id: sessionId,
        message: message
      },
      {
        headers: {
          'Authorization': `Bearer ${JWT_TOKEN}`,
          'Content-Type': 'application/json'
        }
      }
    );

    return response.data;
  } catch (error) {
    console.error('Chat error:', error.response?.data || error.message);
    throw error;
  }
}

// 使用示例
async function main() {
  // 第一次对话
  const response1 = await chat('帮我查询最近的错误日志');
  console.log('AI回复:', response1.data.message);
  console.log('会话ID:', response1.data.session_id);

  // 继续对话（使用相同的会话ID）
  const response2 = await chat(
    '这些错误是什么原因导致的？',
    response1.data.session_id
  );
  console.log('AI回复:', response2.data.message);
}

main();
```

#### 获取工具列表

```javascript
async function getTools() {
  try {
    const response = await axios.get(`${API_BASE}/ai/tools`, {
      headers: {
        'Authorization': `Bearer ${JWT_TOKEN}`
      }
    });

    console.log('可用工具:', response.data.data.tools);
    return response.data.data.tools;
  } catch (error) {
    console.error('获取工具列表失败:', error);
    throw error;
  }
}
```

#### 查询限流状态

```javascript
async function getRateLimitStatus() {
  try {
    const response = await axios.get(`${API_BASE}/ai/rate-limit`, {
      headers: {
        'Authorization': `Bearer ${JWT_TOKEN}`
      }
    });

    const status = response.data.data;
    console.log(`今日已使用: ${status.day_used}/${status.day_limit} 次`);
    console.log(`Token已使用: ${status.token_used}/${status.token_limit}`);

    return status;
  } catch (error) {
    console.error('查询限流状态失败:', error);
    throw error;
  }
}
```

### Python 示例

```python
import requests

API_BASE = 'http://localhost:9081/api/v1'
JWT_TOKEN = 'your-jwt-token'

headers = {
    'Authorization': f'Bearer {JWT_TOKEN}',
    'Content-Type': 'application/json'
}

def chat(message, session_id=None):
    """发送聊天消息"""
    payload = {
        'message': message
    }
    if session_id:
        payload['session_id'] = session_id

    response = requests.post(
        f'{API_BASE}/ai/chat',
        json=payload,
        headers=headers
    )

    if response.status_code == 200:
        return response.json()
    elif response.status_code == 429:
        raise Exception('请求过于频繁，请稍后再试')
    else:
        raise Exception(f'请求失败: {response.text}')

def get_tools():
    """获取工具列表"""
    response = requests.get(
        f'{API_BASE}/ai/tools',
        headers=headers
    )
    return response.json()['data']['tools']

def get_rate_limit_status():
    """获取限流状态"""
    response = requests.get(
        f'{API_BASE}/ai/rate-limit',
        headers=headers
    )
    return response.json()['data']

# 使用示例
if __name__ == '__main__':
    # 查询日志
    result = chat('帮我查询最近1小时的错误日志')
    print(f"AI回复: {result['data']['message']}")

    # 继续对话
    session_id = result['data']['session_id']
    result2 = chat('这些错误是什么原因？', session_id)
    print(f"AI回复: {result2['data']['message']}")

    # 查看限流状态
    status = get_rate_limit_status()
    print(f"今日剩余请求次数: {status['day_remaining']}")
```

### Go 示例

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	APIBase  = "http://localhost:9081/api/v1"
	JWTToken = "your-jwt-token"
)

type ChatRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message"`
}

type ChatResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
		ToolCalls []struct {
			Name   string                 `json:"name"`
			Params map[string]interface{} `json:"params"`
			Result string                 `json:"result"`
		} `json:"tool_calls,omitempty"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage,omitempty"`
	} `json:"data"`
}

func chat(message string, sessionID string) (*ChatResponse, error) {
	req := ChatRequest{
		Message: message,
	}
	if sessionID != "" {
		req.SessionID = sessionID
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(
		"POST",
		APIBase+"/ai/chat",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+JWTToken)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, err
	}

	return &chatResp, nil
}

func main() {
	// 第一次对话
	resp1, err := chat("帮我查询最近的错误日志", "")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("AI回复: %s\n", resp1.Data.Message)
	fmt.Printf("会话ID: %s\n", resp1.Data.SessionID)

	// 继续对话
	resp2, err := chat("这些错误是什么原因？", resp1.Data.SessionID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("AI回复: %s\n", resp2.Data.Message)
}
```

---

## 最佳实践

### 1. 配置管理

#### 使用环境变量

```bash
# 生产环境配置
export MOONLIGHT_AI_API_KEY="sk-xxxxx"
export MOONLIGHT_AI_BASE_URL="https://api.openai.com/v1"
export MOONLIGHT_AI_PROVIDER="openai"
```

#### 配置文件示例

```yaml
# 生产环境配置
ai:
  enabled: true
  provider: "${AI_PROVIDER}"  # 从环境变量读取
  api_key: "${AI_API_KEY}"
  base_url: "${AI_BASE_URL}"
  model: "gpt-4"
  max_tokens: 4096
  temperature: 0.7

  tools:
    enabled: true
    enable_audit_log: true  # 生产环境必须开启审计

  rate_limit:
    requests_per_minute: 10  # 生产环境降低限流阈值
    requests_per_day: 200
    tokens_per_day: 50000

  cache:
    enabled: true
    ttl: 2h
```

### 2. 错误处理

```javascript
async function chatWithErrorHandling(message, sessionId) {
  try {
    const response = await chat(message, sessionId);
    return response;
  } catch (error) {
    if (error.response?.status === 429) {
      // 限流错误，等待后重试
      const retryAfter = error.response.headers['retry-after'] || 60;
      console.log(`请求过于频繁，${retryAfter}秒后重试`);
      await sleep(retryAfter * 1000);
      return chat(message, sessionId);
    } else if (error.response?.status === 401) {
      // 认证错误，需要重新登录
      console.error('认证失败，请重新登录');
      throw new Error('Authentication failed');
    } else {
      // 其他错误
      console.error('AI服务错误:', error.message);
      throw error;
    }
  }
}
```

### 3. 会话管理

```javascript
class AIChatSession {
  constructor(apiBase, token) {
    this.apiBase = apiBase;
    this.token = token;
    this.sessionId = null;
  }

  async send(message) {
    const response = await axios.post(
      `${this.apiBase}/ai/chat`,
      {
        session_id: this.sessionId,
        message: message
      },
      {
        headers: {
          'Authorization': `Bearer ${this.token}`
        }
      }
    );

    // 保存会话ID用于后续对话
    this.sessionId = response.data.data.session_id;
    return response.data;
  }

  async clear() {
    if (this.sessionId) {
      await axios.delete(
        `${this.apiBase}/ai/sessions/${this.sessionId}`,
        {
          headers: {
            'Authorization': `Bearer ${this.token}`
          }
        }
      );
      this.sessionId = null;
    }
  }
}

// 使用示例
const session = new AIChatSession(API_BASE, JWT_TOKEN);
await session.send('查询最近的错误日志');
await session.send('这些错误是什么原因？');
await session.clear();  // 清理会话
```

### 4. 性能优化

#### 使用缓存

```javascript
// 检查缓存命中率
async function checkCacheStats() {
  const response = await axios.get(`${API_BASE}/ai/cache/stats`, {
    headers: { 'Authorization': `Bearer ${JWT_TOKEN}` }
  });

  const stats = response.data.data;
  if (stats.hit_rate < 0.3) {
    console.warn('缓存命中率较低，考虑调整缓存策略');
  }

  return stats;
}
```

#### 批量请求

```javascript
// 避免频繁请求，合并多个查询
async function batchQuery(queries) {
  const message = queries.map((q, i) => `${i + 1}. ${q}`).join('\n');
  return await chat(message);
}

// 使用示例
const queries = [
  '查询最近的错误日志',
  '统计包的下载数量',
  '检查安全漏洞'
];
const response = await batchQuery(queries);
```

### 5. 安全建议

1. **API密钥管理**
   - 使用环境变量存储API密钥
   - 定期轮换API密钥
   - 不同环境使用不同的密钥

2. **权限控制**
   - 为不同角色配置不同的工具权限
   - 敏感操作需要二次确认
   - 记录所有工具调用审计日志

3. **输入验证**
   - 对用户输入进行长度限制
   - 过滤敏感信息
   - 防止注入攻击

4. **限流保护**
   - 设置合理的限流阈值
   - 监控异常请求模式
   - 实现降级策略

---

## 故障排查

### 常见问题

#### 1. AI服务无法连接

**症状**：
```
Error: AI调用失败: connection refused
```

**排查步骤**：
1. 检查AI服务是否启动
   ```bash
   curl http://localhost:8000/v1/models
   ```

2. 检查配置文件中的 `base_url` 是否正确

3. 检查网络连接和防火墙设置

4. 查看服务日志
   ```bash
   tail -f logs/moonlight.log | grep "AI"
   ```

#### 2. 请求过于频繁

**症状**：
```json
{
  "code": 429,
  "message": "请求过于频繁，请稍后再试"
}
```

**解决方案**：
1. 查询限流状态
   ```bash
   curl -H "Authorization: Bearer $TOKEN" \
     http://localhost:9081/api/v1/ai/rate-limit
   ```

2. 调整限流配置
   ```yaml
   ai:
     rate_limit:
       requests_per_minute: 30  # 增加限制
   ```

3. 实现客户端限流
   ```javascript
   class RateLimiter {
     constructor(minLimit) {
       this.minLimit = minLimit;
       this.requests = [];
     }

     async waitIfNeeded() {
       const now = Date.now();
       this.requests = this.requests.filter(t => now - t < 60000);

       if (this.requests.length >= this.minLimit) {
         const waitTime = 60000 - (now - this.requests[0]);
         await sleep(waitTime);
       }

       this.requests.push(now);
     }
   }
   ```

#### 3. 工具调用失败

**症状**：
```json
{
  "tool_calls": [{
    "name": "query_logs",
    "error": "日志路径未配置"
  }]
}
```

**排查步骤**：
1. 检查工具配置
   ```yaml
   ai:
     tools:
       enabled: true
       allowed_tools:
         - query_logs
   ```

2. 检查工具所需资源（如日志路径、数据库连接）

3. 查看审计日志
   ```bash
   curl -H "Authorization: Bearer $TOKEN" \
     http://localhost:9081/api/v1/ai/audit-logs?limit=10
   ```

#### 4. 响应缓存问题

**症状**：相同问题返回不同答案，或缓存未生效

**排查步骤**：
1. 检查缓存配置
   ```yaml
   ai:
     cache:
       enabled: true
       ttl: 1h
   ```

2. 查看缓存统计
   ```bash
   curl -H "Authorization: Bearer $TOKEN" \
     http://localhost:9081/api/v1/ai/cache/stats
   ```

3. 检查缓存命中率，如果过低考虑调整缓存策略

#### 5. 会话丢失

**症状**：多轮对话时AI没有上下文记忆

**排查步骤**：
1. 确保每次请求使用相同的 `session_id`

2. 检查会话配置
   ```yaml
   ai:
     session:
       max_age: 24h
       max_messages: 50
   ```

3. 检查会话是否过期

### 日志分析

#### 查看AI服务日志

```bash
# 查看最近的AI相关日志
tail -f logs/moonlight.log | grep -E "\[AI\]|tool_call"

# 查看错误日志
grep "ERROR" logs/moonlight.log | grep "ai"

# 查看特定用户的AI请求
grep "user_id=123" logs/moonlight.log | grep "AI"
```

#### 日志级别调整

```yaml
logging:
  level: debug  # 调试时设置为debug
  format: console
```

### 性能监控

#### 监控指标

```bash
# 获取服务统计
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:9081/api/v1/ai/stats

# 输出示例
{
  "session_count": 10,
  "tool_count": 5,
  "cache_stats": {
    "hit_rate": 0.67
  },
  "audit_log_count": 25
}
```

#### 性能优化建议

1. **缓存命中率低**
   - 增加 `cache.ttl`
   - 增加 `cache.max_size`
   - 优化用户提问方式

2. **响应时间长**
   - 减少 `max_tokens`
   - 使用更快的模型
   - 优化工具执行效率

3. **内存占用高**
   - 减少 `session.max_messages`
   - 减少 `cache.max_size`
   - 定期清理过期会话

### 健康检查

```bash
# 检查AI服务健康状态
curl http://localhost:9081/api/v1/ai/health

# 成功响应
{
  "code": 200,
  "data": {
    "status": "healthy"
  }
}

# 失败响应
{
  "status": "unhealthy",
  "error": "数据库连接异常"
}
```

---

## 附录

### 支持的AI提供商

| 提供商 | Provider值 | 默认Base URL | 推荐模型 |
|--------|-----------|-------------|---------|
| ChatGLM | chatglm | http://localhost:8000/v1 | chatglm3-6b |
| OpenAI | openai | https://api.openai.com/v1 | gpt-4, gpt-3.5-turbo |
| Anthropic | anthropic | https://api.anthropic.com/v1 | claude-3-5-sonnet |
| 自定义 | custom | 用户自定义 | 用户自定义 |

### 工具权限配置

不同角色可以使用的工具：

| 角色 | 可用工具 |
|------|---------|
| admin | 所有工具 |
| developer | query_logs, query_package_info, generate_demo_code |
| viewer | query_package_info |

### API限流策略

| 限流维度 | 默认值 | 说明 |
|---------|-------|------|
| 每分钟请求 | 20次 | 防止短时间大量请求 |
| 每天请求 | 500次 | 控制每日使用量 |
| 每天Token | 100,000 | 控制API成本 |

---

## 更新日志

### v1.0.0 (2026-05-02)
- 初始版本发布
- 支持基础聊天功能
- 实现5个核心工具
- 添加限流和缓存机制
- 完善审计日志功能

---

## 联系支持

如有问题或建议，请联系：
- 项目地址：https://github.com/dshmyz/moonlight-box
- 问题反馈：https://github.com/dshmyz/moonlight-box/issues
