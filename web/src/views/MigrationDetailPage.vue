<template>
  <div class="migration-detail-page">
    <div class="page-header">
      <el-button @click="$router.back()" type="text" size="small">
        <i class="fa-solid fa-arrow-left" style="margin-right: 4px;"></i>
        返回迁移历史
      </el-button>
      <h2>任务详情</h2>
    </div>

    <div v-if="loading" class="loading-container">
      <el-icon class="is-loading"><Loading /></el-icon>
      <p>加载中...</p>
    </div>

    <div v-else-if="task" class="detail-content">
      <!-- 基本信息 -->
      <el-card class="detail-card">
        <template #header>
          <div class="card-header">
            <i class="fa-solid fa-info-circle"></i>
            <span>基本信息</span>
          </div>
        </template>
        <el-descriptions :column="2" border>
          <el-descriptions-item label="任务ID">{{ task.id }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="statusType(task.status)" size="small">{{ statusText(task.status) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="来源地址" :span="2">{{ task.source_url }}</el-descriptions-item>
          <el-descriptions-item label="目标仓库">{{ task.target_repository || '-' }}</el-descriptions-item>
          <el-descriptions-item label="源类型">{{ task.source_type }}</el-descriptions-item>
        </el-descriptions>
      </el-card>

      <!-- 执行进度 -->
      <el-card class="detail-card">
        <template #header>
          <div class="card-header">
            <i class="fa-solid fa-chart-line"></i>
            <span>执行进度</span>
          </div>
        </template>
        <div class="progress-stats">
          <div class="stat-item">
            <span class="stat-label">已处理</span>
            <span class="stat-value">{{ task.processed_items }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">失败</span>
            <span class="stat-value failed">{{ task.failed_items }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">总数</span>
            <span class="stat-value">{{ task.total_items }}</span>
          </div>
        </div>
        <el-progress
          :percentage="progressPercent"
          :status="task.status === 'failed' ? 'exception' : undefined"
          :stroke-width="20"
        />
        <div class="progress-note">
          <span class="note-text">
            已扫描 <strong>{{ task.total_items }}</strong> 个组件，已处理 <strong>{{ task.processed_items }}</strong> 个
          </span>
        </div>
      </el-card>

      <!-- 时间信息 -->
      <el-card class="detail-card">
        <template #header>
          <div class="card-header">
            <i class="fa-solid fa-clock"></i>
            <span>时间信息</span>
          </div>
        </template>
        <el-descriptions :column="2" border>
          <el-descriptions-item label="创建时间">{{ formatDateTime(task.created_at) }}</el-descriptions-item>
          <el-descriptions-item label="更新时间">{{ formatDateTime(task.updated_at) }}</el-descriptions-item>
          <el-descriptions-item label="开始时间">{{ task.started_at ? formatDateTime(task.started_at) : '-' }}</el-descriptions-item>
          <el-descriptions-item label="完成时间">{{ task.completed_at ? formatDateTime(task.completed_at) : '-' }}</el-descriptions-item>
        </el-descriptions>
      </el-card>

      <!-- 错误信息 -->
      <el-card v-if="task.error_message" class="detail-card error-card">
        <template #header>
          <div class="card-header">
            <i class="fa-solid fa-exclamation-triangle"></i>
            <span>错误信息</span>
          </div>
        </template>
        <div class="error-content">{{ task.error_message }}</div>
      </el-card>

      <!-- 迁移项目列表 -->
      <el-card class="detail-card">
        <template #header>
          <div class="card-header">
            <i class="fa-solid fa-list"></i>
            <span>迁移项目</span>
            <span class="item-count">（共 {{ itemTotal }} 条）</span>
          </div>
        </template>
        <el-table :data="migrationItems" size="default" v-loading="itemsLoading">
          <el-table-column prop="component_name" label="组件名称" min-width="150" show-overflow-tooltip />
          <el-table-column prop="version" label="版本" width="100" />
          <el-table-column prop="format" label="格式" width="80" />
          <el-table-column prop="status" label="状态" width="80" align="center">
            <template #default="{ row }">
              <el-tag :type="itemStatusType(row.status)" size="small">{{ itemStatusText(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="retry_count" label="重试" width="60" align="center" />
          <el-table-column label="错误" min-width="150" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="error-text">{{ row.error_message || '-' }}</span>
            </template>
          </el-table-column>
        </el-table>
        <div class="pagination-wrapper" v-if="itemTotal > 0">
          <el-pagination
            small
            layout="total, prev, pager, next"
            :total="itemTotal"
            :page-size="itemPageSize"
            v-model:current-page="itemCurrentPage"
            @current-change="loadMigrationItems"
          />
        </div>
      </el-card>

      <!-- 操作按钮 -->
      <div class="action-buttons">
        <el-button
          v-if="task.status === 'failed' && task.failed_items > 0"
          type="warning"
          size="large"
          @click="handleRetry"
        >
          <i class="fa-solid fa-rotate-right" style="margin-right: 4px;"></i>
          重试失败项目
        </el-button>
        <el-button
          v-if="task.status === 'running'"
          type="danger"
          size="large"
          @click="handleCancel"
        >
          <i class="fa-solid fa-stop" style="margin-right: 4px;"></i>
          取消任务
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Loading } from '@element-plus/icons-vue'
import type { MigrationTask } from '@/api/migration'
import { getMigrationStatus, listMigrationItems, retryFailedMigration, cancelMigration } from '@/api/migration'
import { success, error } from '@/utils/message'

const route = useRoute()

const task = ref<MigrationTask | null>(null)
const loading = ref(false)
const migrationItems = ref<any[]>([])
const itemsLoading = ref(false)
const itemTotal = ref(0)
const itemPageSize = ref(20)
const itemCurrentPage = ref(1)

const progressPercent = computed(() => {
  if (!task.value || task.value.total_items === 0) return 0
  return Math.round((task.value.processed_items / task.value.total_items) * 100)
})

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

function formatDateTime(timeStr: string) {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function itemStatusType(status: string) {
  const map: Record<string, string> = {
    completed: 'success',
    failed: 'danger',
    processing: '',
    pending: 'info',
  }
  return map[status] || 'info'
}

function itemStatusText(status: string) {
  const map: Record<string, string> = {
    completed: '成功',
    failed: '失败',
    processing: '处理中',
    pending: '待处理',
  }
  return map[status] || status
}

async function loadTask() {
  loading.value = true
  try {
    const taskId = Number(route.params.id)
    const res = (await getMigrationStatus(taskId)) as any
    const data = res?.data?.task || res?.data || res
    task.value = data
  } catch (e) {
    console.error('Load task failed:', e)
  } finally {
    loading.value = false
  }
}

async function loadMigrationItems() {
  if (!task.value) return
  
  itemsLoading.value = true
  try {
    const taskId = task.value.id
    const res = (await listMigrationItems(
      taskId,
      itemCurrentPage.value,
      itemPageSize.value
    )) as any
    
    const data = res?.data || res
    migrationItems.value = data?.list || []
    itemTotal.value = data?.total || 0
  } catch (e) {
    console.error('Load migration items failed:', e)
    migrationItems.value = []
    itemTotal.value = 0
  } finally {
    itemsLoading.value = false
  }
}

async function handleRetry() {
  if (!task.value) return
  try {
    await retryFailedMigration(task.value.id)
    success('已开始重试失败项目')
    await loadTask()
  } catch (e) {
    console.error('Retry failed:', e)
    error('重试失败')
  }
}

async function handleCancel() {
  if (!task.value) return
  try {
    await cancelMigration(task.value.id)
    success('已取消任务')
    await loadTask()
  } catch (e) {
    console.error('Cancel failed:', e)
    error('取消失败')
  }
}

onMounted(() => {
  loadTask().then(() => {
    loadMigrationItems()
  })
})
</script>

<style scoped>
.migration-detail-page {
  padding: 20px;
  background: #f5f7fa;
  min-height: 100vh;
}

.page-header {
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 8px 0 0 0;
  font-size: 20px;
  color: #1f2937;
}

.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 0;
  color: #6b7280;
}

.detail-content {
  max-width: 1200px;
  margin: 0 auto;
}

.detail-card {
  margin-bottom: 16px;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #1f2937;
}

.card-header i {
  color: #8b5cf6;
}

.item-count {
  font-size: 14px;
  font-weight: normal;
  color: #6b7280;
}

.progress-stats {
  display: flex;
  gap: 24px;
  margin-bottom: 16px;
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
  font-size: 20px;
  font-weight: 600;
  color: #1f2937;
}

.stat-value.failed {
  color: #ef4444;
}

.progress-note {
  margin-top: 8px;
  text-align: center;
}

.note-text {
  font-size: 13px;
  color: #6b7280;
}

.note-text strong {
  color: #1f2937;
  font-weight: 600;
}

.error-card {
  background: #fef2f2;
  border-color: #fecaca;
}

.error-card .card-header {
  color: #dc2626;
}

.error-card .card-header i {
  color: #dc2626;
}

.error-content {
  color: #991b1b;
  padding: 8px 0;
  word-break: break-all;
}

.error-text {
  color: #ef4444;
  font-size: 13px;
}

.pagination-wrapper {
  margin-top: 16px;
  display: flex;
  justify-content: center;
}

.action-buttons {
  display: flex;
  gap: 12px;
  justify-content: center;
  padding: 20px 0;
}
</style>
