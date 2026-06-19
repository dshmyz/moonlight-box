<template>
  <div class="empty-state">
    <el-empty :description="description">
      <template #image>
        <el-icon class="empty-icon"><Box /></el-icon>
      </template>
      <div class="empty-actions">
        <template v-if="variant === 'empty' && mode === 'admin'">
          <el-button type="primary" @click="$emit('upload')">
            <el-icon><Upload /></el-icon>
            上传第一个包
          </el-button>
        </template>
        <template v-if="variant === 'no-match'">
          <p class="empty-hint">尝试调整搜索词或清空筛选条件</p>
          <el-button data-test="clear-filters" @click="$emit('clear-filters')">清空所有筛选</el-button>
        </template>
        <template v-if="variant === 'error'">
          <p class="empty-error-msg">{{ errorMessage }}</p>
          <el-button data-test="retry" type="primary" @click="$emit('retry')">重试</el-button>
        </template>
      </div>
    </el-empty>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Box, Upload } from '@element-plus/icons-vue'

const props = defineProps<{
  variant: 'empty' | 'no-match' | 'error'
  mode: 'admin' | 'public'
  errorMessage?: string
}>()

defineEmits<{
  upload: []
  'clear-filters': []
  retry: []
}>()

const description = computed(() => {
  switch (props.variant) {
    case 'empty': return '暂无包'
    case 'no-match': return '未找到匹配的包'
    case 'error': return '加载失败'
  }
})
</script>

<style scoped>
.empty-state { padding: 60px 20px; text-align: center; }
.empty-icon { font-size: 64px; color: #cbd5e1; }
.empty-actions { margin-top: 16px; }
.empty-hint { color: #94a3b8; font-size: 13px; margin: 0 0 12px; }
.empty-error-msg { color: #ef4444; font-size: 13px; margin: 0 0 12px; }
</style>
