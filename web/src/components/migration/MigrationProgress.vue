<template>
  <div class="migration-progress">
    <h3>迁移进度</h3>
    <el-progress :percentage="percentage" :status="progressStatus" />
    <div class="stats">
      <span>总计: {{ total }}</span>
      <span>已完成: {{ processed }}</span>
      <span>失败: {{ failed }}</span>
    </div>
    <div class="actions">
      <el-button v-if="status === 'running'" type="danger" @click="$emit('cancel')">
        取消
      </el-button>
    </div>
    <div v-if="logs.length" class="logs">
      <h4>日志</h4>
      <ul>
        <li v-for="(log, i) in logs" :key="i">{{ log }}</li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  total: number
  processed: number
  failed: number
  logs: string[]
}>()

defineEmits<{
  cancel: []
}>()

const percentage = computed(() => {
  if (props.total === 0) return 0
  return Math.round((props.processed / props.total) * 100)
})

const progressStatus = computed(() => {
  if (props.status === 'completed') return 'success'
  if (props.status === 'failed') return 'exception'
  if (props.status === 'cancelled') return 'warning'
  return undefined
})
</script>

<style scoped>
.stats {
  display: flex;
  gap: 20px;
  margin-top: 10px;
}
.logs {
  margin-top: 20px;
  max-height: 300px;
  overflow-y: auto;
}
.logs ul {
  list-style: none;
  padding: 0;
}
.logs li {
  padding: 4px 0;
  font-family: monospace;
  font-size: 12px;
}
</style>
