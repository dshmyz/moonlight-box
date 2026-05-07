<template>
  <div class="migration-progress-card">
    <div class="card-header">
      <h3 class="card-title">
        <i class="fa-solid fa-spinner"></i>
        迁移进度
      </h3>
    </div>
    <div class="card-body">
      <el-progress :percentage="percentage" :status="progressStatus" />
      <div class="stats">
        <div class="stat-item">
          <span class="stat-label">总计</span>
          <span class="stat-value">{{ total }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">已完成</span>
          <span class="stat-value success">{{ processed }}</span>
        </div>
        <div class="stat-item">
          <span class="stat-label">失败</span>
          <span class="stat-value danger">{{ failed }}</span>
        </div>
      </div>
      <div class="actions">
        <el-button v-if="status === 'running'" type="danger" @click="$emit('cancel')">
          <i class="fa-solid fa-x"></i>
          取消
        </el-button>
      </div>
      <div v-if="logs.length" class="logs">
        <h4 class="logs-title">
          <i class="fa-solid fa-file-text"></i>
          日志
        </h4>
        <ul class="logs-list">
          <li v-for="(log, i) in logs" :key="i" class="log-item">{{ log }}</li>
        </ul>
      </div>
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
.migration-progress-card {
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  border: 1px solid #e5e7eb;
  overflow: hidden;
}

.card-header {
  padding: 16px 20px;
  border-bottom: 1px solid #e5e7eb;
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  color: #1f2937;
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}

.card-title i {
  color: #8b5cf6;
}

.card-body {
  padding: 20px;
}

.stats {
  display: flex;
  gap: 32px;
  margin-top: 16px;
}

.stat-item {
  display: flex;
  flex-direction: column;
}

.stat-label {
  font-size: 12px;
  color: #9ca3af;
  margin-bottom: 4px;
}

.stat-value {
  font-size: 18px;
  font-weight: 600;
  color: #1f2937;
}

.stat-value.success {
  color: #10b981;
}

.stat-value.danger {
  color: #ef4444;
}

.actions {
  margin-top: 20px;
}

.actions button {
  display: flex;
  align-items: center;
  gap: 6px;
}

.logs {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid #e5e7eb;
}

.logs-title {
  font-size: 13px;
  font-weight: 500;
  color: #374151;
  margin: 0 0 12px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.logs-title i {
  color: #6b7280;
}

.logs-list {
  list-style: none;
  padding: 0;
  margin: 0;
  max-height: 200px;
  overflow-y: auto;
}

.log-item {
  padding: 6px 12px;
  margin-bottom: 4px;
  background: #f9fafb;
  border-radius: 6px;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 12px;
  color: #4b5563;
  line-height: 1.5;
}
</style>
