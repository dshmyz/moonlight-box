<template>
  <div class="chat-window">
    <div class="chat-header">
      <h3>AI助手</h3>
      <div class="header-actions">
        <el-button text @click="handleClearSession">
          <el-icon><Delete /></el-icon>
          清空会话
        </el-button>
        <el-button text @click="showHistory = !showHistory">
          <el-icon><Clock /></el-icon>
          历史记录
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
import { Delete, Clock } from '@element-plus/icons-vue'
import { aiApi, type ChatResponse } from '@/api/ai'
import { useChatHistory, type Message } from '@/composables/useChatHistory'
import { useCommands } from '@/composables/useCommands'
import { useStreamChat } from '@/composables/useStreamChat'
import MessageList from './MessageList.vue'
import InputBox from './InputBox.vue'
import SuggestionList from './SuggestionList.vue'

const { saveHistory, loadHistory, clearHistory, getRecentSessions } = useChatHistory()
const { parseCommand, executeCommand } = useCommands()
const { isStreaming, streamChat, abort } = useStreamChat()

const messages = ref<Message[]>([])
const sessionId = ref<string>('')
const loading = ref(false)
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
  
  try {
    const response = await aiApi.chat({
      session_id: sessionId.value,
      message
    })

    sessionId.value = response.session_id

    messages.value.push({
      ...response,
      role: 'assistant',
      content: response.message
    })
  } catch (error: any) {
    ElMessage.error(error.message || '发送失败')
    messages.value.pop()
  } finally {
    loading.value = false
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
  padding: 16px;
  border-bottom: 1px solid var(--el-border-color);
}

.chat-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 500;
}

.header-actions {
  display: flex;
  gap: 8px;
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
</style>
