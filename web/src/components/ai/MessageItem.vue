<template>
  <div :class="['message-item', message.role]">
    <div class="message-avatar">
      <el-avatar :size="32" :icon="roleIcon" />
    </div>
    <div class="message-content">
      <div class="message-header">
        <span class="role-name">{{ roleName }}</span>
        <span v-if="message.timestamp" class="message-time">{{ formatTime(message.timestamp) }}</span>
      </div>
      
      <div class="message-text">
        <MarkdownRenderer 
          v-if="message.role === 'assistant'" 
          :content="message.content" 
          @node-click="handleNodeClick"
          @optimize="handleOptimize"
        />
        <template v-else>{{ message.content }}</template>
      </div>
      
      <div v-if="message.tool_calls?.length" class="tool-calls">
        <el-collapse>
          <el-collapse-item title="工具调用详情">
            <ToolCallDisplay
              v-for="(call, index) in message.tool_calls"
              :key="index"
              :tool-call="call"
              :show-details="false"
            />
          </el-collapse-item>
        </el-collapse>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { User, ChatDotRound } from '@element-plus/icons-vue'
import { ElCollapse, ElCollapseItem } from 'element-plus'
import type { ToolCallResult } from '@/api/ai'
import ToolCallDisplay from './ToolCallDisplay.vue'
import MarkdownRenderer from './MarkdownRenderer.vue'

interface MessageData {
  role: 'user' | 'assistant'
  content: string
  session_id: string
  message: string
  timestamp?: number
  tokens_used?: number
  cached?: boolean
  tool_calls?: ToolCallResult[]
}

interface Props {
  message: MessageData
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'optimize', pkgName: string, pkgType?: string): void
}>()

const roleIcon = computed(() => 
  props.message.role === 'user' ? User : ChatDotRound
)

const roleName = computed(() =>
  props.message.role === 'user' ? '你' : 'AI助手'
)

const formatTime = (timestamp?: number) => {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  if (isNaN(date.getTime())) return ''
  return date.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit'
  })
}

const handleNodeClick = (node: { name: string; type: string }) => {
  // 节点点击：显示轻量提示，详情由弹窗处理
  ElMessage.info(`📦 ${node.name} (${node.type}) - 点击节点查看完整详情`)
}

const handleOptimize = (pkgName: string, _pkgType?: string) => {
  ElMessage.success(`✨ 正在分析 ${pkgName} 的优化方案...`)
  emit('optimize', pkgName, _pkgType)
}
</script>

<style scoped>
.message-item {
  display: flex;
  gap: 12px;
  padding: 16px 0;
}

.message-item.user {
  flex-direction: row-reverse;
}

.message-avatar {
  flex-shrink: 0;
}

.message-content {
  flex: 1;
  min-width: 0;
}

.message-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.role-name {
  font-weight: 500;
  color: var(--el-text-color-primary);
}

.message-time {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.message-text {
  padding: 12px 16px;
  background: var(--el-fill-color-light);
  border-radius: 8px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}

.message-item.user .message-text {
  background: var(--el-color-primary-light-9);
}

.tool-calls {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
</style>
