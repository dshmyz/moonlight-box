# AI 流式回复优化实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 优化 AI 流式回复性能（缓存优化）并提升用户体验（加载状态提示）

**架构：** 两层缓存（AI 响应缓存 + 工具结果缓存）+ 前端实时加载状态反馈

**技术栈：** Go（后端缓存）、Vue 3 + TypeScript（前端状态管理）、Element Plus（UI 组件）

---

## 文件结构

### 后端文件
- **修改**：`internal/ai/tools/block_log_analyzer.go` - 添加工具级缓存
- **修改**：`configs/config.yaml` - 重新启用 AI 缓存
- **修改**：`internal/ai/service.go` - 缓存状态传递逻辑

### 前端文件
- **修改**：`web/src/components/ai/ChatWindow.vue` - 加载状态显示
- **修改**：`web/src/composables/useStreamChat.ts` - 状态回调支持
- **修改**：`web/src/components/ai/MessageItem.vue` - 工具调用结果折叠显示

---

### 任务 1：工具级缓存实现

**文件：**
- 修改：`internal/ai/tools/block_log_analyzer.go`

- [ ] **步骤 1：添加缓存结构**

```go
// 在 BlockLogAnalyzerTool 结构体中添加缓存字段
type BlockLogAnalyzerTool struct {
    auditRepo *repository.AuditRepository
    cache     *sync.Map  // 新增：简单内存缓存
    cacheTTL  time.Duration
}

// 缓存条目
type cacheEntry struct {
    result    interface{}
    expiresAt time.Time
}
```

- [ ] **步骤 2：修改 Execute 方法，添加缓存逻辑**

```go
func (t *BlockLogAnalyzerTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // 构建缓存 key
    cacheKey := t.buildCacheKey(params)
    
    // 检查缓存
    if entry, ok := t.cache.Load(cacheKey); ok {
        cacheEntry := entry.(*cacheEntry)
        if time.Now().Before(cacheEntry.expiresAt) {
            return cacheEntry.result, nil
        }
        // 缓存过期，删除
        t.cache.Delete(cacheKey)
    }
    
    // 执行查询
    result, err := t.queryDatabase(ctx, params)
    if err != nil {
        return nil, err
    }
    
    // 写入缓存
    t.cache.Store(cacheKey, &cacheEntry{
        result:    result,
        expiresAt: time.Now().Add(t.cacheTTL),
    })
    
    return result, nil
}

// 构建缓存 key
func (t *BlockLogAnalyzerTool) buildCacheKey(params map[string]interface{}) string {
    analysisType := getStringParam(params, "analysis_type", "overview")
    timeRange := getStringParam(params, "time_range", "24h")
    return fmt.Sprintf("block_log:%s:%s", analysisType, timeRange)
}

func getStringParam(params map[string]interface{}, key, defaultValue string) string {
    if val, ok := params[key].(string); ok {
        return val
    }
    return defaultValue
}
```

- [ ] **步骤 3：修改 NewBlockLogAnalyzerTool 构造函数**

```go
func NewBlockLogAnalyzerTool(auditRepo *repository.AuditRepository) *BlockLogAnalyzerTool {
    return &BlockLogAnalyzerTool{
        auditRepo: auditRepo,
        cache:     &sync.Map{},
        cacheTTL:  5 * time.Minute,
    }
}
```

- [ ] **步骤 4：添加缓存清理方法**

```go
// ClearCache 清理所有缓存（用于新增阻断记录时调用）
func (t *BlockLogAnalyzerTool) ClearCache() {
    t.cache.Range(func(key, value interface{}) bool {
        t.cache.Delete(key)
        return true
    })
}
```

- [ ] **步骤 5：Commit**

```bash
git add internal/ai/tools/block_log_analyzer.go
git commit -m "feat: add tool-level cache for block log analyzer"
```

---

### 任务 2：重新启用 AI 缓存

**文件：**
- 修改：`configs/config.yaml`

- [ ] **步骤 1：修改配置文件**

```yaml
ai:
  cache:
    enabled: true  # 从 false 改为 true
    ttl: 30m       # 从 1h 改为 30m
    max_size: 1000
```

- [ ] **步骤 2：Commit**

```bash
git add configs/config.yaml
git commit -m "config: re-enable AI response cache with 30m TTL"
```

---

### 任务 3：前端加载状态支持

**文件：**
- 修改：`web/src/composables/useStreamChat.ts`

- [ ] **步骤 1：添加状态回调类型定义**

```typescript
export interface StreamCallbacks {
  onChunk: (content: string, done: boolean, toolCall?: ToolCallResult) => void
  onStatus?: (status: string, phase: string) => void  // 新增
}

export type LoadingPhase = 'analyzing' | 'querying' | 'generating' | 'done'
```

- [ ] **步骤 2：修改 streamChat 函数，添加状态推送**

```typescript
let hasToolCall = false

// 处理工具调用
if (data.tool_call) {
  hasToolCall = true
  callbacks.onStatus?.('正在查询阻断日志数据...', 'querying')
  callbacks.onChunk(fullContent, false, data.tool_call)
}

// 处理文本内容
if (data.content) {
  if (hasToolCall && fullContent === '') {
    callbacks.onStatus?.('正在生成分析结果...', 'generating')
  }
  fullContent += data.content
  callbacks.onChunk(fullContent, false)
}

// 处理完成信号
if (data.done) {
  callbacks.onStatus?.('完成', 'done')
  callbacks.onChunk(fullContent, true)
}
```

- [ ] **步骤 3：Commit**

```bash
git add web/src/composables/useStreamChat.ts
git commit -m "feat: add status callback support for streaming chat"
```

---

### 任务 4：ChatWindow.vue 加载状态显示

**文件：**
- 修改：`web/src/components/ai/ChatWindow.vue`

- [ ] **步骤 1：添加加载状态变量**

```typescript
const loadingStatus = ref('')
const loadingPhase = ref<LoadingPhase>('done')
```

- [ ] **步骤 2：修改 handleSend 函数**

```typescript
const handleSend = async (message: string) => {
  // ... 现有代码 ...
  
  loading.value = true
  loadingStatus.value = '正在分析您的请求...'
  loadingPhase.value = 'analyzing'
  
  try {
    await streamChat(
      {
        session_id: sessionId.value,
        message
      },
      (content, done, toolCall) => {
        // ... 现有代码 ...
      },
      (status, phase) => {
        loadingStatus.value = status
        loadingPhase.value = phase as LoadingPhase
      },
      true
    )
  } catch (error: any) {
    // ... 现有代码 ...
  } finally {
    loading.value = false
    loadingStatus.value = ''
    loadingPhase.value = 'done'
  }
}
```

- [ ] **步骤 3：添加加载状态 UI**

```vue
<template>
  <!-- 在消息列表底部添加 -->
  <div v-if="loading" class="loading-indicator">
    <el-icon class="is-loading"><Loading /></el-icon>
    <span class="loading-text">{{ loadingStatus }}</span>
  </div>
</template>

<style scoped>
.loading-indicator {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  color: var(--el-text-color-secondary);
  font-size: 14px;
}

.loading-text {
  animation: pulse 2s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
```

- [ ] **步骤 4：Commit**

```bash
git add web/src/components/ai/ChatWindow.vue
git commit -m "feat: add loading status indicator for AI chat"
```

---

### 任务 5：MessageItem.vue 工具调用结果折叠显示

**文件：**
- 修改：`web/src/components/ai/MessageItem.vue`

- [ ] **步骤 1：添加工具调用折叠组件**

```vue
<template>
  <div class="message-item">
    <!-- 现有消息内容 -->
    <div v-if="message.content" class="message-content">
      {{ message.content }}
    </div>
    
    <!-- 工具调用结果（折叠显示） -->
    <div v-if="message.tool_calls && message.tool_calls.length > 0" class="tool-calls">
      <el-collapse>
        <el-collapse-item title="工具调用详情">
          <div v-for="(tool, index) in message.tool_calls" :key="index" class="tool-call-item">
            <div class="tool-call-header">
              <el-tag :type="tool.success ? 'success' : 'danger'" size="small">
                {{ tool.name }}
              </el-tag>
              <span v-if="tool.error" class="tool-error">{{ tool.error }}</span>
            </div>
            <pre v-if="tool.result" class="tool-result">{{ formatToolResult(tool.result) }}</pre>
          </div>
        </el-collapse-item>
      </el-collapse>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ElCollapse, ElCollapseItem, ElTag } from 'element-plus'

interface Props {
  message: {
    role: string
    content: string
    tool_calls?: Array<{
      name: string
      params?: any
      result?: any
      error?: string
      success: boolean
    }>
  }
}

defineProps<Props>()

const formatToolResult = (result: any): string => {
  if (typeof result === 'string') return result
  return JSON.stringify(result, null, 2)
}
</script>

<style scoped>
.tool-calls {
  margin-top: 12px;
}

.tool-call-item {
  margin-bottom: 12px;
  padding: 8px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
}

.tool-call-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.tool-error {
  color: var(--el-color-danger);
  font-size: 12px;
}

.tool-result {
  background: var(--el-bg-color);
  padding: 8px;
  border-radius: 4px;
  font-size: 12px;
  max-height: 200px;
  overflow: auto;
}
</style>
```

- [ ] **步骤 2：Commit**

```bash
git add web/src/components/ai/MessageItem.vue
git commit -m "feat: add collapsible tool call result display"
```

---

### 任务 6：构建和测试

- [ ] **步骤 1：构建后端**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
make build
```

预期：构建成功，无编译错误

- [ ] **步骤 2：构建前端**

```bash
cd web
npm run build
```

预期：构建成功，无 TypeScript 错误

- [ ] **步骤 3：启动服务并测试**

```bash
# 启动后端
./bin/moonlight-registry -config configs/config.yaml

# 在浏览器中测试
# 1. 发送消息"分析阻断日志"
# 2. 观察加载状态提示
# 3. 验证工具调用结果折叠显示
# 4. 再次发送相同消息，验证缓存命中（响应更快）
```

- [ ] **步骤 4：Commit**

```bash
git add .
git commit -m "test: verify AI streaming optimization"
```

---

## 自检

✅ **规格覆盖度**：
- 工具级缓存 → 任务 1
- AI 响应缓存 → 任务 2
- 前端加载状态 → 任务 3、4
- 工具调用折叠显示 → 任务 5
- 构建测试 → 任务 6

✅ **占位符扫描**：无"待定"或"TODO"

✅ **类型一致性**：
- `LoadingPhase` 类型在任务 3 定义，任务 4 使用
- `StreamCallbacks` 接口在任务 3 定义，任务 4 使用
- `ToolCallResult` 接口保持一致

---

计划已完成并保存到 `docs/superpowers/plans/2026-05-06-ai-streaming-optimization.md`。两种执行方式：

**1. 子代理驱动（推荐）** - 每个任务调度一个新的子代理，任务间进行审查，快速迭代

**2. 内联执行** - 在当前会话中使用 executing-plans 执行任务，批量执行并设有检查点

选哪种方式？
