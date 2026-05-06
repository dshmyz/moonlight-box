<template>
  <el-drawer
    v-model="visible"
    :title="`${repoName} - 同步历史`"
    direction="rtl"
    size="600px"
  >
    <div v-loading="loading" class="sync-history">
      <el-empty v-if="tasks.length === 0 && !loading" description="暂无同步历史" />
      <div v-else class="task-list">
        <div v-for="task in tasks" :key="task.id" class="task-card">
          <div class="task-header">
            <el-tag :type="getStatusType(task.status)" size="small">
              {{ getStatusText(task.status) }}
            </el-tag>
            <span class="task-time">{{ formatTime(task.created_at) }}</span>
          </div>
          <div class="task-body">
            <div class="task-stats">
              <div class="stat-item">
                <span class="stat-label">总计</span>
                <span class="stat-value">{{ task.total_packages }}</span>
              </div>
              <div class="stat-item success">
                <span class="stat-label">成功</span>
                <span class="stat-value">{{ task.synced_packages }}</span>
              </div>
              <div class="stat-item danger">
                <span class="stat-label">失败</span>
                <span class="stat-value">{{ task.failed_packages }}</span>
              </div>
              <div class="stat-item warning">
                <span class="stat-label">跳过</span>
                <span class="stat-value">{{ task.skipped_packages }}</span>
              </div>
            </div>
            <div v-if="task.error_message" class="task-error">
              <el-icon><WarningFilled /></el-icon>
              {{ task.error_message }}
            </div>
            <div v-if="task.started_at && task.completed_at" class="task-duration">
              耗时: {{ calculateDuration(task.started_at, task.completed_at) }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { WarningFilled } from '@element-plus/icons-vue'
import request from '@/api/request'

interface SyncTask {
  id: number
  status: string
  total_packages: number
  synced_packages: number
  failed_packages: number
  skipped_packages: number
  error_message: string
  started_at: string
  completed_at: string
  created_at: string
}

const props = defineProps<{
  modelValue: boolean
  repoId: number | null
  repoName: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const visible = ref(props.modelValue)
const loading = ref(false)
const tasks = ref<SyncTask[]>([])

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val && props.repoId) {
    loadSyncHistory()
  }
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

async function loadSyncHistory() {
  if (!props.repoId) return
  loading.value = true
  try {
    const res = await request.get(`/repositories/${props.repoId}/sync-history`, {
      params: { limit: 50 }
    })
    tasks.value = (res as any) || []
  } catch (e) {
    console.error('Failed to load sync history:', e)
  } finally {
    loading.value = false
  }
}

function getStatusType(status: string): 'success' | 'warning' | 'danger' | 'info' {
  const map: Record<string, 'success' | 'warning' | 'danger' | 'info'> = {
    completed: 'success',
    running: 'warning',
    failed: 'danger',
    pending: 'info',
  }
  return map[status] || 'info'
}

function getStatusText(status: string): string {
  const map: Record<string, string> = {
    completed: '已完成',
    running: '进行中',
    failed: '失败',
    pending: '等待中',
  }
  return map[status] || status
}

function formatTime(time: string): string {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}

function calculateDuration(start: string, end: string): string {
  const startTime = new Date(start).getTime()
  const endTime = new Date(end).getTime()
  const seconds = Math.floor((endTime - startTime) / 1000)
  if (seconds < 60) return `${seconds}秒`
  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = seconds % 60
  return `${minutes}分${remainingSeconds}秒`
}
</script>

<style scoped>
.sync-history {
  padding: 0 16px;
}

.task-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.task-card {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 16px;
}

.task-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.task-time {
  color: #64748b;
  font-size: 13px;
}

.task-stats {
  display: flex;
  gap: 16px;
  margin-bottom: 12px;
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.stat-label {
  font-size: 12px;
  color: #94a3b8;
}

.stat-value {
  font-size: 18px;
  font-weight: 600;
  color: #0f172a;
}

.stat-item.success .stat-value {
  color: #10b981;
}

.stat-item.danger .stat-value {
  color: #ef4444;
}

.stat-item.warning .stat-value {
  color: #f59e0b;
}

.task-error {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #ef4444;
  font-size: 13px;
  background: #fef2f2;
  padding: 8px 12px;
  border-radius: 4px;
  margin-bottom: 12px;
}

.task-duration {
  font-size: 13px;
  color: #64748b;
}
</style>
