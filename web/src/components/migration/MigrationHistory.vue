<template>
  <div class="migration-history-card">
    <div class="card-header">
      <h3 class="card-title">
        <i class="fa-solid fa-history"></i>
        迁移历史
      </h3>
      <span class="card-badge">{{ tasks.length }}</span>
    </div>
    <div class="card-body">
      <div v-if="tasks.length === 0" class="empty-state">
        <i class="fa-solid fa-inbox"></i>
        <p>暂无迁移记录</p>
      </div>
      <el-table v-else :data="tasks" class="history-table">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="source_url" label="来源" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="url-text">{{ truncateUrl(row.source_url) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="target_repository" label="目标仓库" width="120" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="processed_items" label="进度" width="100">
          <template #default="{ row }">
            <span class="progress-text">{{ row.processed_items }}/{{ row.total_items }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="150">
          <template #default="{ row }">
            <span class="time-text">{{ formatTime(row.created_at) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { MigrationTask } from '@/api/migration'

defineProps<{
  tasks: MigrationTask[]
}>()

function statusType(status: string) {
  const map: Record<string, string> = {
    completed: 'success',
    failed: 'danger',
    cancelled: 'warning',
    running: '',
    pending: 'info',
  }
  return map[status] || 'info'
}

function statusText(status: string) {
  const map: Record<string, string> = {
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消',
    running: '运行中',
    pending: '待执行',
  }
  return map[status] || status
}

function truncateUrl(url: string) {
  if (url.length <= 40) return url
  return url.substring(0, 37) + '...'
}

function formatTime(timeStr: string) {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
</script>

<style scoped>
.migration-history-card {
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
  display: flex;
  align-items: center;
  justify-content: space-between;
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

.card-badge {
  padding: 2px 8px;
  background: #8b5cf6;
  color: #fff;
  font-size: 12px;
  font-weight: 500;
  border-radius: 10px;
}

.card-body {
  padding: 16px;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  color: #9ca3af;
}

.empty-state i {
  font-size: 40px;
  margin-bottom: 12px;
  opacity: 0.5;
}

.empty-state p {
  margin: 0;
  font-size: 14px;
}

.history-table {
  font-size: 13px;
}

.url-text {
  color: #3b82f6;
}

.progress-text {
  color: #6b7280;
}

.time-text {
  color: #9ca3af;
}
</style>
