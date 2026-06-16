<template>
  <div class="download-logs">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-file-lines"></i>
        </div>
        <div class="header-text">
          <h2>下载日志</h2>
          <p class="header-subtitle">查看和分析包下载记录</p>
        </div>
      </div>
      <el-button class="refresh-btn" @click="loadLogs">
        <el-icon><Refresh /></el-icon>
        <span>刷新</span>
      </el-button>
    </header>

    <div class="stats-bar">
      <div class="stat-card stat-card--total">
        <div class="stat-icon">
          <i class="fa-solid fa-download"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.total_downloads || 0 }}</span>
          <span class="stat-label">总下载次数</span>
        </div>
      </div>
      <div class="stat-card stat-card--success">
        <div class="stat-icon stat-icon--success">
          <i class="fa-solid fa-check-circle"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.success_count || 0 }}</span>
          <span class="stat-label">成功</span>
        </div>
      </div>
      <div class="stat-card stat-card--failed">
        <div class="stat-icon stat-icon--failed">
          <i class="fa-solid fa-x-circle"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.failed_count || 0 }}</span>
          <span class="stat-label">失败</span>
        </div>
      </div>
      <div class="stat-card stat-card--cached">
        <div class="stat-icon stat-icon--cached">
          <i class="fa-solid fa-database"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.cached_count || 0 }}</span>
          <span class="stat-label">缓存命中</span>
        </div>
      </div>
      <div class="stat-card stat-card--traffic">
        <div class="stat-icon stat-icon--traffic">
          <i class="fa-solid fa-network-wired"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ formatBytes(stats.total_bytes || 0) }}</span>
          <span class="stat-label">总下载流量</span>
        </div>
      </div>
    </div>

    <div class="content-panel" v-loading="loading">
      <div class="filter-bar">
        <el-select
          v-model="filterRepo"
          placeholder="仓库"
          clearable
          class="filter-select"
          @change="loadLogs"
        >
          <el-option v-for="repo in repositories" :key="repo.id" :label="repo.display_name || repo.name" :value="repo.id" />
        </el-select>
        <el-select
          v-model="filterPkgType"
          placeholder="包类型"
          clearable
          class="filter-select"
          @change="loadLogs"
        >
          <el-option label="npm" value="npm" />
          <el-option label="Maven" value="maven" />
          <el-option label="PyPI" value="pypi" />
          <el-option label="Go" value="go" />
          <el-option label="Yum" value="yum" />
          <el-option label="Apt" value="apt" />
          <el-option label="Generic" value="generic" />
        </el-select>
        <el-select
          v-model="filterStatus"
          placeholder="状态"
          clearable
          class="filter-select"
          @change="loadLogs"
        >
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option label="缓存" value="cached" />
        </el-select>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          class="date-picker"
          @change="loadLogs"
        />
        <el-button type="primary" class="search-btn" @click="loadLogs">搜索</el-button>
      </div>

      <el-table
        :data="logs"
        style="width: 100%"
        :header-cell-style="{ background: '#fafbfc' }"
        :row-class-name="tableRowClass"
        @row-mouse-enter="handleRowEnter"
        @row-mouse-leave="handleRowLeave"
      >
        <el-table-column label="仓库" width="160">
          <template #default="{ row }">
            <div class="cell-multi-line">
              <span class="repo-name">{{ row.repository?.display_name || row.repository?.name || '-' }}</span>
              <div class="cell-line-secondary">
                <el-tag :class="['pkg-type-tag', `pkg-type-tag--${row.package_type}`]" size="small">{{ row.package_type }}</el-tag>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="包信息" min-width="200">
          <template #default="{ row }">
            <div class="cell-multi-line">
              <div class="cell-line-primary">{{ row.package_name || '-' }}</div>
              <div class="cell-line-secondary">
                <span v-if="row.version" class="version-text">{{ row.version }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <div class="cell-multi-line cell-multi-line--center">
              <el-tag :class="['status-tag', `status-tag--${row.status}`]" size="small">{{ statusLabel(row.status) }}</el-tag>
              <span v-if="row.status_code" :class="['status-code', getStatusCodeClass(row.status_code)]">{{ row.status_code }}</span>
              <span v-else class="no-code">-</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="大小" width="100" align="center">
          <template #default="{ row }">
            <span class="size-text">{{ formatBytes(row.size_bytes) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="耗时" width="80" align="center">
          <template #default="{ row }">
            <span :class="['duration-text', getDurationClass(row.duration_ms)]">
              {{ row.duration_ms != null ? row.duration_ms + 'ms' : '-' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="来源" width="80" align="center">
          <template #default="{ row }">
            <el-tooltip v-if="!row.from_cache && row.remote_url" :content="row.remote_url" placement="top" :show-after="200">
              <el-tag type="warning" size="small" class="remote-tag">远程</el-tag>
            </el-tooltip>
            <el-tag v-else-if="!row.from_cache" type="warning" size="small" class="remote-tag">远程</el-tag>
            <el-tag v-else type="info" size="small" class="cache-tag">缓存</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="error_message" label="失败原因" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.error_message" class="error-message">{{ row.error_message }}</span>
            <span v-else class="no-error">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="ip_address" label="IP" width="130" />
        <el-table-column prop="created_at" label="时间" width="180" align="center">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-if="total > pageSize"
        :current-page="page"
        :page-size="pageSize"
        :total="total"
        layout="total, prev, pager, next"
        class="pagination"
        @current-change="handlePageChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { downloadLogApi, type DownloadLog, type DownloadStats } from '@/api/downloadLog'
import { repositoryApi, type Repository } from '@/api/repository'

const loading = ref(false)
const logs = ref<DownloadLog[]>([])
const stats = ref<DownloadStats>({} as DownloadStats)
const repositories = ref<Repository[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filterRepo = ref<number | null>(null)
const filterPkgType = ref('')
const filterStatus = ref('')
const dateRange = ref<[Date, Date] | null>(null)
const hoveredRow = ref<number | null>(null)

function tableRowClass({ rowIndex }: { rowIndex: number }) {
  return rowIndex === hoveredRow.value ? 'row-hovered' : ''
}

function handleRowEnter({ rowIndex }: { rowIndex: number }) {
  hoveredRow.value = rowIndex
}

function handleRowLeave() {
  hoveredRow.value = null
}

function getStatusCodeClass(code: number): string {
  if (code >= 200 && code < 300) return 'status-code--success'
  if (code >= 300 && code < 400) return 'status-code--redirect'
  if (code >= 400 && code < 500) return 'status-code--client-error'
  if (code >= 500) return 'status-code--server-error'
  return ''
}

function formatDate(d: string): string {
  if (!d || d === '') return '-'
  const date = new Date(d)
  if (isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN')
}

function formatBytes(bytes: number): string {
  if (!bytes) return '-'
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)}MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)}GB`
}

function statusLabel(s: string): string {
  const map: Record<string, string> = { success: '成功', failed: '失败', cached: '缓存' }
  return map[s] || s
}

function getDurationClass(ms: number): string {
  if (!ms) return ''
  if (ms < 100) return 'duration-fast'
  if (ms < 500) return 'duration-normal'
  return 'duration-slow'
}

function formatDateForApi(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

async function loadRepositories() {
  try {
    const res = await repositoryApi.list()
    repositories.value = (res && typeof res === 'object' && 'items' in res)
      ? (res as any).items || []
      : (res as any[]) || []
  } catch {
    console.error('Failed to load repositories')
  }
}

async function loadStats() {
  try {
    const params: Record<string, any> = {}
    if (filterRepo.value) params.repository_id = filterRepo.value
    if (dateRange.value && dateRange.value.length === 2) {
      params.start_date = formatDateForApi(dateRange.value[0])
      params.end_date = formatDateForApi(dateRange.value[1])
    }
    const res = await downloadLogApi.getStats(params)
    stats.value = res || ({} as DownloadStats)
  } catch {
    console.error('Failed to load stats')
  }
}

async function loadLogs() {
  loading.value = true
  try {
    const params: Record<string, any> = { page: page.value, page_size: pageSize.value }
    if (filterRepo.value) params.repository_id = filterRepo.value
    if (filterPkgType.value) params.package_type = filterPkgType.value
    if (filterStatus.value) params.status = filterStatus.value
    if (dateRange.value && dateRange.value.length === 2) {
      params.start_date = formatDateForApi(dateRange.value[0])
      params.end_date = formatDateForApi(dateRange.value[1])
    }

    const res = await downloadLogApi.list(params)
    logs.value = res?.items || []
    total.value = res?.pagination?.total || 0
    loadStats()
  } catch {
    console.error('Failed to load logs')
  } finally {
    loading.value = false
  }
}

function handlePageChange(p: number) {
  page.value = p
  loadLogs()
}

onMounted(() => {
  loadRepositories()
  loadLogs()
})
</script>

<style scoped>
.download-logs {
  padding: var(--spacing-5);
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  background: #fff;
  border-radius: 16px;
  margin-bottom: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  width: 52px;
  height: 52px;
  border-radius: 14px;
  background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  color: #2563eb;
}

.header-text h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #1e293b;
}

.header-subtitle {
  margin: 4px 0 0;
  font-size: 13px;
  color: #94a3b8;
}

.refresh-btn {
  border-radius: 10px;
  padding: 10px 18px;
  font-weight: 500;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  color: #64748b;
  transition: all 0.2s ease;
}

.refresh-btn:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
  color: #475569;
}

.stats-bar {
  display: flex;
  gap: 16px;
  margin-bottom: 16px;
}

.stat-card {
  flex: 1;
  padding: 20px;
  background: #fff;
  border-radius: 14px;
  display: flex;
  align-items: center;
  gap: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  transition: all 0.2s ease;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #e2e8f0 0%, #cbd5e1 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  color: #64748b;
}

.stat-icon--success {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #16a34a;
}

.stat-icon--failed {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #dc2626;
}

.stat-icon--cached {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.stat-icon--traffic {
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
  color: #4f46e5;
}

.stat-info {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: #1e293b;
}

.stat-label {
  font-size: 13px;
  color: #94a3b8;
  margin-top: 2px;
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.filter-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid #f1f5f9;
}

.filter-select {
  width: 160px;
  border-radius: 10px;
}

.date-picker {
  width: 240px;
  border-radius: 10px;
}

.search-btn {
  border-radius: 10px;
  padding: 10px 20px;
  font-weight: 500;
}

:deep(.el-table .row-hovered) {
  background: #f8fafc;
}

.repo-name {
  color: #334155;
  font-size: 13px;
}

.cell-multi-line {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.cell-multi-line--center {
  align-items: center;
}

.cell-line-primary {
  font-size: 13px;
  color: #1e293b;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-line-secondary {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #64748b;
}

.version-text {
  color: #94a3b8;
  font-size: 12px;
}

.size-text {
  font-size: 13px;
  font-weight: 600;
  color: #475569;
}

.meta-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font-size: 12px;
  min-height: 20px;
}

.duration-text {
  font-size: 13px;
  font-weight: 500;
}

.duration-fast {
  color: #10b981;
}

.duration-normal {
  color: #64748b;
}

.duration-slow {
  color: #f59e0b;
}

.cache-tag {
  font-size: 11px;
  padding: 0 6px;
  height: 20px;
  line-height: 20px;
}

.remote-tag {
  font-size: 11px;
  padding: 0 6px;
  height: 20px;
  line-height: 20px;
}

:deep(.el-tooltip__trigger) {
  cursor: pointer;
}

:deep(.el-table) {
  overflow: visible;
}

:deep(.el-table__body-wrapper) {
  overflow: visible !important;
}

.pkg-type-tag {
  background: #f1f5f9;
  color: #64748b;
  border: none;
}

.status-tag {
  border: none;
}

.status-tag--success {
  background: #dcfce7;
  color: #16a34a;
}

.status-tag--failed {
  background: #fee2e2;
  color: #dc2626;
}

.status-tag--cached {
  background: #fef3c7;
  color: #d97706;
}

.status-code {
  font-weight: 600;
  font-size: 13px;
}

.status-code--success {
  color: #16a34a;
}

.status-code--redirect {
  color: #3b82f6;
}

.status-code--client-error {
  color: #f59e0b;
}

.status-code--server-error {
  color: #dc2626;
}

.error-message {
  color: #dc2626;
  font-size: 13px;
}

.no-error,
.no-code,
.no-cache {
  color: #94a3b8;
}

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

:deep(.el-pagination button) {
  border-radius: 8px;
}

:deep(.el-pager li) {
  border-radius: 8px;
  margin: 0 2px;
}

:deep(.el-pager li.is-active) {
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
}
</style>
