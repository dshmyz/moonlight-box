# AI 流式回复优化设计文档

**日期**: 2026-05-06
**主题**: AI 流式回复性能优化与用户体验提升

## 问题描述

当前 AI 流式回复存在两个主要问题：
1. **响应速度慢**：完整流程约 18 秒（两次 AI 调用 + 工具执行）
2. **缺乏进度反馈**：用户等待期间无任何提示，体验不佳

## 优化方案

采用 **方案 A（缓存优化）+ 方案 B（前端加载状态）** 组合策略

### 方案 A：缓存优化

#### 1. 工具级缓存

**目标**：对工具执行结果进行缓存，避免重复查询数据库

**实现位置**：`internal/ai/tools/block_log_analyzer.go`

**缓存策略**：
- 缓存 key：`block_log:{analysis_type}:{time_range}`
- 缓存 TTL：5 分钟（统计类数据不需要实时更新）
- 缓存失效：新增阻断记录时自动清除

**代码结构**：
```go
type BlockLogAnalyzerTool struct {
    cache *sync.Map  // 简单内存缓存
    // ... 现有字段
}

func (t *BlockLogAnalyzerTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // 1. 检查缓存
    cacheKey := t.buildCacheKey(params)
    if cached, ok := t.cache.Load(cacheKey); ok {
        return cached, nil
    }
    
    // 2. 执行查询
    result := t.queryDatabase(ctx, params)
    
    // 3. 写入缓存
    t.cache.Store(cacheKey, result)
    
    return result, nil
}
```

#### 2. AI 响应缓存

**目标**：对相同用户消息直接返回缓存结果

**实现位置**：`internal/ai/cache.go`（已存在，需重新启用）

**配置修改**：
```yaml
ai:
  cache:
    enabled: true
    ttl: 30m
    max_size: 1000
```

### 方案 B：前端加载状态

#### 1. ChatWindow.vue 修改

**新增状态显示**：
- `loadingStatus`：当前加载状态文本
- `loadingPhase`：加载阶段（analyzing / querying / generating）

**状态流转**：
```typescript
// 发送消息时
loadingStatus.value = '正在分析您的请求...'
loadingPhase.value = 'analyzing'

// 收到工具调用时
loadingStatus.value = '正在查询阻断日志数据...'
loadingPhase.value = 'querying'

// AI 开始生成回复时
loadingStatus.value = '正在生成分析结果...'
loadingPhase.value = 'generating'
```

**UI 组件**：
```vue
<div v-if="loading" class="loading-indicator">
  <el-icon class="is-loading"><Loading /></el-icon>
  <span>{{ loadingStatus }}</span>
</div>
```

#### 2. useStreamChat.ts 修改

**新增回调**：
```typescript
export interface StreamCallbacks {
  onChunk: (content: string, done: boolean, toolCall?: ToolCallResult) => void
  onStatus?: (status: string, phase: string) => void  // 新增
}
```

**状态推送逻辑**：
```typescript
// 检测到工具调用时
if (data.tool_call) {
  callbacks.onStatus?.('正在查询阻断日志数据...', 'querying')
  callbacks.onChunk(fullContent, false, data.tool_call)
}

// 检测到文本内容时（工具调用后）
if (data.content && hasToolCall) {
  callbacks.onStatus?.('正在生成分析结果...', 'generating')
}
```

#### 3. MessageItem.vue 修改

**工具调用结果折叠显示**：
```vue
<div v-if="message.tool_calls && message.tool_calls.length > 0" class="tool-calls">
  <el-collapse>
    <el-collapse-item title="工具调用详情">
      <div v-for="tool in message.tool_calls" :key="tool.name" class="tool-call">
        <el-tag>{{ tool.name }}</el-tag>
        <pre>{{ tool.result }}</pre>
      </div>
    </el-collapse-item>
  </el-collapse>
</div>
```

## 数据流

### 优化前
```
用户发送消息 → [等待 18 秒] → 显示完整结果
```

### 优化后
```
用户发送消息 
  → 立即显示"正在分析您的请求..." 
  → 检测到工具调用 → 显示"正在查询阻断日志数据..."
  → 工具执行完成 → 显示"正在生成分析结果..."
  → AI 流式输出 → 逐步显示结果
  → 完成
```

## 缓存层级

```
用户请求 
  → L1: AI 缓存（相同问题直接返回，TTL 30m）
  → L2: 工具缓存（相同查询直接返回，TTL 5m）
  → L3: 数据库查询
```

## 配置变更

### config.yaml
```yaml
ai:
  cache:
    enabled: true  # 从 false 改为 true
    ttl: 30m       # 从 1h 改为 30m
    max_size: 1000
  
  # 新增工具缓存配置
  tools:
    cache:
      enabled: true
      default_ttl: 5m
```

## 预期效果

| 场景 | 优化前 | 优化后 |
|------|--------|--------|
| 首次调用 | 18 秒 | 18 秒（不变） |
| 重复调用（相同问题） | 18 秒 | 2-3 秒（命中 AI 缓存） |
| 重复调用（不同问题，相同工具） | 18 秒 | 10-12 秒（命中工具缓存） |
| 用户体验 | 无反馈，感觉卡住 | 全程进度提示 |

## 测试计划

1. **功能测试**：
   - 验证缓存命中/未命中场景
   - 验证加载状态显示正确
   - 验证工具调用结果折叠显示

2. **性能测试**：
   - 测量首次调用耗时
   - 测量重复调用耗时
   - 验证缓存 TTL 生效

3. **边界测试**：
   - 缓存失效后重新查询
   - 并发请求缓存处理
   - 缓存内存占用监控
