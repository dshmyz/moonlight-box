<template>
  <div class="migration-history-card">
    <div class="card-header">
      <h3 class="card-title">
        <i class="fa-solid fa-history"></i>
        迁移历史
      </h3>
      <span class="card-badge">{{ tasks.length }}</span>
    </div>
    <div class="card-body" v-loading="loading">
      <div v-if="tasks.length === 0 && !loading" class="empty-state">
        <i class="fa-solid fa-inbox"></i>
        <p>暂无迁移记录</p>
      </div>
      <el-table v-else :data="paginatedTasks" class="history-table">
        <el-table-column prop="id" label="ID" width="60" align="center" />
        <el-table-column prop="source_url" label="来源地址" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="url-text">{{ row.source_url }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="target_repository" label="目标仓库" min-width="120" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="processed_items" label="进度" min-width="140">
          <template #default="{ row }">
            <div class="progress-wrapper">
              <span class="progress-text">已扫描 {{ row.total_items }}，已处理 {{ row.processed_items }}</span>
              <div class="progress-bar">
                <div class="progress-fill" :style="{ width: progressPercent(row) + '%' }"></div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="160">
          <template #default="{ row }">
            <span class="time-text">{{ formatTime(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right" align="center">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-tooltip content="查看详情" placement="top">
                <el-button
                  size="small"
                  type="primary"
                  link
                  @click="viewDetail(row)"
                >
                  <i class="fa-solid fa-eye"></i>
                </el-button>
              </el-tooltip>
              <el-tooltip content="重试失败项目" placement="top" v-if="row.status === 'failed' && row.failed_items > 0">
                <el-button
                  size="small"
                  type="warning"
                  link
                  @click="handleRetry(row)"
                >
                  <i class="fa-solid fa-rotate-right"></i>
                </el-button>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="tasks.length > pageSize" class="pagination-wrapper">
        <el-pagination
          layout="prev, pager, next"
          :total="tasks.length"
          :page-size="pageSize"
          :current-page="currentPage"
          @current-change="handlePageChange"
          small
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { MigrationTask } from '@/api/migration'
import { retryFailedMigration } from '@/api/migration'
import { success, error } from '@/utils/message'

const router = useRouter()

const props = defineProps<{
  tasks: MigrationTask[]
  loading?: boolean
}>()

const emit = defineEmits<{
  refresh: []
}>()

const currentPage = ref(1)
const pageSize = 10

const paginatedTasks = ref<MigrationTask[]>([])

watch(() => props.tasks, (newTasks) => {
  if (newTasks && newTasks.length > 0) {
    updatePaginatedTasks()
  }
}, { immediate: true })

const updatePaginatedTasks = () => {
  const start = (currentPage.value - 1) * pageSize
  paginatedTasks.value = props.tasks.slice(start, start + pageSize)
}

const handlePageChange = (page: number) => {
  currentPage.value = page
  updatePaginatedTasks()
}

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

function progressPercent(task: MigrationTask) {
  if (task.total_items === 0) return 0
  return Math.round((task.processed_items / task.total_items) * 100)
}

function viewDetail(task: MigrationTask) {
  router.push({ name: 'MigrationDetail', params: { id: String(task.id) } })
}

async function handleRetry(task: MigrationTask) {
  if (task.status !== 'failed' || task.failed_items === 0) return

  try {
    await retryFailedMigration(task.id)
    emit('refresh')
    success('已开始重试失败项目')
  } catch (err) {
    console.error('Retry failed:', err)
    error('重试失败项目失败')
  }
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
  font-size: 12px;
}

.time-text {
  color: #9ca3af;
}

.progress-wrapper {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.progress-bar {
  height: 6px;
  background: #e5e7eb;
  border-radius: 3px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #8b5cf6 0%, #6366f1 100%);
  border-radius: 3px;
  transition: width 0.3s ease;
}

.action-buttons {
  display: flex;
  justify-content: center;
  gap: 4px;
}

.pagination-wrapper {
  display: flex;
  justify-content: center;
  padding: 16px 0 8px;
}
</style>
