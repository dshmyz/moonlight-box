<template>
  <div class="code-block">
    <div class="code-header">
      <span class="code-title">{{ title }}</span>
      <el-button
        text
        size="small"
        @click="copyCode"
        :icon="CopyDocument"
      >
        {{ copied ? '已复制' : '复制' }}
      </el-button>
    </div>
    <pre><code>{{ code }}</code></pre>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { CopyDocument } from '@element-plus/icons-vue'
import { copyToClipboard } from '@/utils/clipboard'

interface Props {
  code: string
  title?: string
}

const props = withDefaults(defineProps<Props>(), {
  title: '命令'
})

const copied = ref(false)

const copyCode = () => {
  copyToClipboard(props.code).then((success) => {
    if (success) {
      copied.value = true
      setTimeout(() => {
        copied.value = false
      }, 2000)
    }
  })
}
</script>

<style scoped>
.code-block {
  margin: 10px 0;
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  overflow: hidden;
}

.code-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  background: #f5f7fa;
  border-bottom: 1px solid #e4e7ed;
}

.code-title {
  font-size: 12px;
  color: #606266;
}

pre {
  margin: 0;
  padding: 12px;
  background: #fafafa;
  overflow-x: auto;
}

code {
  font-family: var(--font-family-mono);
  font-size: 13px;
  color: #303133;
}
</style>
