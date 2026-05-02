<template>
  <div class="input-box">
    <div v-if="showCommandHint && commandHint" class="command-hint">
      <el-icon><InfoFilled /></el-icon>
      <span>{{ commandHint }}</span>
    </div>
    <el-input
      v-model="inputMessage"
      type="textarea"
      :rows="3"
      :placeholder="placeholder"
      :disabled="disabled"
      @keydown.enter.ctrl="handleSend"
      @input="handleInput"
    />
    <div class="input-actions">
      <div class="tips">
        <span class="tip">Ctrl + Enter 发送</span>
        <span class="tip">支持快捷命令（/help 查看帮助）</span>
      </div>
      <el-button
        type="primary"
        :disabled="!inputMessage.trim() || disabled"
        :loading="loading"
        @click="handleSend"
      >
        发送
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { InfoFilled } from '@element-plus/icons-vue'
import { useCommands } from '@/composables/useCommands'

interface Props {
  disabled?: boolean
  loading?: boolean
  placeholder?: string
  showCommandHint?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  loading: false,
  placeholder: '输入你的问题，例如：查询lodash包的信息',
  showCommandHint: true
})

const emit = defineEmits<{
  send: [message: string]
}>()

const { parseCommand, getCommand } = useCommands()
const inputMessage = ref('')

const commandHint = computed(() => {
  const parsed = parseCommand(inputMessage.value)
  if (parsed.isCommand && parsed.command) {
    const cmd = getCommand(parsed.command)
    if (cmd) {
      return `${cmd.description} - ${cmd.usage}`
    }
    return `未知命令: ${parsed.command}`
  }
  return ''
})

const handleInput = () => {
  // 可以在这里添加更多输入处理逻辑
}

const handleSend = () => {
  const message = inputMessage.value.trim()
  if (message) {
    emit('send', message)
    inputMessage.value = ''
  }
}
</script>

<style scoped>
.input-box {
  padding: 16px;
  border-top: 1px solid var(--el-border-color);
  background: var(--el-bg-color);
}

.command-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--el-color-info-light-9);
  border-radius: 4px;
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--el-text-color-regular);
}

.input-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 8px;
}

.tips {
  display: flex;
  gap: 16px;
}

.tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
