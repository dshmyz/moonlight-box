<template>
  <div class="chat-window">
    <div class="chat-header">
      <div class="header-left">
        <span class="header-subtitle">智能问答 · 代码助手</span>
      </div>
      <div class="header-actions">
        <el-button class="header-btn" @click="handleClearSession" title="清空会话">
          <i class="fa-solid fa-trash-can"></i>
        </el-button>
        <el-button class="header-btn" @click="showHistory = !showHistory" title="历史记录">
          <i class="fa-solid fa-clock-rotate-left"></i>
        </el-button>
      </div>
    </div>
    
    <!-- 历史记录侧边栏 -->
    <el-drawer
      v-model="showHistory"
      title="历史会话"
      direction="rtl"
      size="300px"
    >
      <div class="history-list">
        <div
          v-for="history in recentHistories"
          :key="history.sessionId"
          class="history-item"
          @click="loadHistorySession(history.sessionId)"
        >
          <div class="history-time">
            {{ formatHistoryTime(history.lastUpdated) }}
          </div>
          <div class="history-preview">
            {{ getHistoryPreview(history.messages) }}
          </div>
        </div>
        <el-empty v-if="recentHistories.length === 0" description="暂无历史记录" />
      </div>
    </el-drawer>
    
    <!-- 建议列表 -->
    <SuggestionList
      v-if="messages.length === 0"
      @select="handleSuggestionSelect"
    />
    
    <MessageList :messages="messages" />
    
    <!-- 加载状态指示器 -->
    <div v-if="loading" class="loading-indicator">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span class="loading-text">{{ loadingStatus }}</span>
    </div>
    
    <InputBox
      :disabled="loading || isStreaming"
      :loading="loading || isStreaming"
      @send="handleSend"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Loading } from '@element-plus/icons-vue'
import { aiApi } from '@/api/ai'
import { useChatHistory, type Message } from '@/composables/useChatHistory'
import { useCommands } from '@/composables/useCommands'
import { useStreamChat, type LoadingPhase } from '@/composables/useStreamChat'
import MessageList from './MessageList.vue'
import InputBox from './InputBox.vue'
import SuggestionList from './SuggestionList.vue'

const { saveHistory, loadHistory, clearHistory, getRecentSessions } = useChatHistory()
const { parseCommand, executeCommand } = useCommands()
const { isStreaming, streamChat } = useStreamChat()

const messages = ref<Message[]>([])
const sessionId = ref<string>('')
const loading = ref(false)
const loadingStatus = ref('')
const loadingPhase = ref<LoadingPhase>('done')
const showHistory = ref(false)
const recentHistories = ref<any[]>([])

onMounted(() => {
  recentHistories.value = getRecentSessions(10)
})

watch(messages, (newMessages) => {
  if (sessionId.value && newMessages.length > 0) {
    saveHistory(sessionId.value, newMessages)
  }
}, { deep: true })

const handleSend = async (message: string) => {
  const parsed = parseCommand(message)
  
  if (parsed.isCommand && parsed.command) {
    const commandResponse = executeCommand(parsed)
    
    if (parsed.command === 'help') {
      messages.value.push({
        role: 'assistant',
        content: commandResponse,
        session_id: sessionId.value,
        message: commandResponse,
        timestamp: Date.now() / 1000,
        tokens_used: 0,
        cached: false
      })
      return
    }
    
    message = commandResponse
  }
  
  messages.value.push({
    role: 'user',
    content: message,
    session_id: sessionId.value,
    message: '',
    timestamp: Date.now() / 1000,
    tokens_used: 0,
    cached: false
  })

  loading.value = true
  loadingStatus.value = '正在分析您的请求...'
  loadingPhase.value = 'analyzing'
  
  // 创建助手消息占位符
  const assistantMessageIndex = messages.value.length
  messages.value.push({
    role: 'assistant',
    content: '',
    session_id: sessionId.value,
    message: '',
    timestamp: Date.now() / 1000,
    tokens_used: 0,
    cached: false,
    tool_calls: []
  })
  
  try {
    await streamChat(
      {
        session_id: sessionId.value,
        message
      },
      (content, done, toolCall) => {
        const msg = messages.value[assistantMessageIndex]
        if (toolCall) {
          // 添加工具调用信息
          if (!msg.tool_calls) {
            msg.tool_calls = []
          }
          msg.tool_calls.push({
            name: toolCall.name,
            params: toolCall.params,
            result: toolCall.result,
            error: toolCall.error,
            success: !toolCall.error,
            duration_ms: 0
          })
        } else if (content) {
          // 更新文本内容
          msg.content = content
          msg.message = content
        }
        
        if (done) {
          sessionId.value = msg.session_id || sessionId.value
        }
      },
      (status, phase) => {
        loadingStatus.value = status
        loadingPhase.value = phase as LoadingPhase
      },
      true // 使用流式输出
    )
  } catch (error: any) {
    ElMessage.error(error.message || '发送失败')
    messages.value.splice(assistantMessageIndex, 1)
  } finally {
    loading.value = false
    loadingStatus.value = ''
    loadingPhase.value = 'done'
  }
}

const handleSuggestionSelect = (suggestion: string) => {
  handleSend(suggestion)
}

const handleClearSession = async () => {
  if (sessionId.value) {
    try {
      await aiApi.clearSession(sessionId.value)
      clearHistory(sessionId.value)
    } catch (error) {
      // 忽略清除错误
    }
  }
  messages.value = []
  sessionId.value = ''
  ElMessage.success('会话已清空')
}

const loadHistorySession = (historySessionId: string) => {
  const historyMessages = loadHistory(historySessionId)
  if (historyMessages.length > 0) {
    sessionId.value = historySessionId
    messages.value = historyMessages
    showHistory.value = false
  }
}

const formatHistoryTime = (timestamp: number) => {
  return new Date(timestamp).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

const getHistoryPreview = (msgs: Message[]) => {
  if (msgs.length === 0) return '空会话'
  const firstMsg = msgs[0]
  return firstMsg.content.substring(0, 50) + (firstMsg.content.length > 50 ? '...' : '')
}
</script>

<style scoped>
.chat-window {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--el-bg-color);
  border-radius: 8px;
  overflow: hidden;
}

.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid var(--el-border-color);
  background: var(--el-bg-color);
}

.header-left {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.header-subtitle {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  letter-spacing: 0.02em;
}

.header-actions {
  display: flex;
  gap: 4px;
}

.header-btn {
  width: 28px !important;
  height: 28px !important;
  padding: 0 !important;
  background: transparent !important;
  border: none !important;
  color: #64748b !important;
  transition: all 0.2s ease !important;
}

.header-btn:hover {
  background: #f1f5f9 !important;
  color: #0f172a !important;
}

.history-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.history-item {
  padding: 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.3s;
}

.history-item:hover {
  background: var(--el-fill-color);
  transform: translateX(4px);
}

.history-time {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 4px;
}

.history-preview {
  font-size: 14px;
  color: var(--el-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

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
