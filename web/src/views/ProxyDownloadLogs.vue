<template>
  <div class="proxy-download-logs">
    <div class="page-header">
      <h2>代理下载日志</h2>
      <CustomButton @click="loadLogs">
        <el-icon><Refresh /></el-icon> 刷新
      </CustomButton>
    </div>

    <div class="stat-cards">
      <CustomCard hoverable class="stat-card">
        <div class="stat-value">{{ stats.total_downloads || 0 }}</div>
        <div class="stat-label">总下载次数</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card success">
        <div class="stat-value">{{ stats.success_count || 0 }}</div>
        <div class="stat-label">成功</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card danger">
        <div class="stat-value">{{ stats.failed_count || 0 }}</div>
        <div class="stat-label">失败</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card info">
        <div class="stat-value">{{ stats.cached_count || 0 }}</div>
        <div class="stat-label">缓存命中</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card wide">
        <div class="stat-value">{{ formatBytes(stats.total_bytes || 0) }}</div>
        <div class="stat-label">总下载流量</div>
      </CustomCard>
    </div>

    <div class="filter-bar">
      <CustomSelect
        v-model="filterRepo"
        placeholder="仓库"
        style="width: 180px"
        @change="loadLogs"
        :options="repositories.map(r => ({ label: r.display_name || r.name, value: r.id }))"
      />
      <CustomSelect
        v-model="filterPkgType"
        placeholder="包类型"
        style="width: 120px"
        @change="loadLogs"
        :options="pkgTypeOptions"
      />
      <CustomSelect
        v-model="filterStatus"
        placeholder="状态"
        style="width: 100px"
        @change="loadLogs"
        :options="statusFilterOptions"
      />
      <el-date-picker v-model="dateRange" type="daterange" range-separator="至" start-placeholder="开始日期" end-placeholder="结束日期" style="width: 240px" @change="loadLogs" />
      <CustomButton type="primary" @click="loadLogs">搜索</CustomButton>
    </div>

    <CustomTable :columns="columns" :data="logs" :loading="loading" row-key="id">
      <template #repository="{ row }">
        {{ row.repository?.display_name || row.repository?.name || '-' }}
      </template>
      <template #package_type="{ row }">
        <CustomTag size="small">{{ row.package_type }}</CustomTag>
      </template>
      <template #status="{ row }">
        <CustomTag :type="statusTagType(row.status)" size="small">{{ statusLabel(row.status) }}</CustomTag>
      </template>
      <template #error_message="{ row }">
        <span v-if="row.error_message" class="error-message">{{ row.error_message }}</span>
        <span v-else>-</span>
      </template>
      <template #status_code="{ row }">
        <span v-if="row.status_code">{{ row.status_code }}</span>
        <span v-else>-</span>
      </template>
      <template #size_bytes="{ row }">
        {{ formatBytes(row.size_bytes) }}
      </template>
      <template #duration_ms="{ row }">
        {{ row.duration_ms ? `${row.duration_ms}ms` : '-' }}
      </template>
      <template #from_cache="{ row }">
        <CustomTag v-if="row.from_cache" type="info" size="small">命中</CustomTag>
        <span v-else>-</span>
      </template>
      <template #created_at="{ row }">
        {{ formatDate(row.created_at) }}
      </template>
    </CustomTable>

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
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { proxyDownloadApi, type ProxyDownloadLog, type ProxyDownloadStats } from '@/api/proxyDownload'
import { repositoryApi, type Repository } from '@/api/repository'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomSelect from '@/components/ui/CustomSelect.vue'
import CustomTable from '@/components/ui/CustomTable.vue'
import CustomTag from '@/components/ui/CustomTag.vue'
import CustomCard from '@/components/ui/CustomCard.vue'

const loading = ref(false)
const logs = ref<ProxyDownloadLog[]>([])
const stats = ref<ProxyDownloadStats>({} as ProxyDownloadStats)
const repositories = ref<Repository[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filterRepo = ref<number | null>(null)
const filterPkgType = ref('')
const filterStatus = ref('')
const dateRange = ref<[Date, Date] | null>(null)

const pkgTypeOptions = [
  { label: 'npm', value: 'npm' },
  { label: 'maven', value: 'maven' },
  { label: 'pypi', value: 'pypi' },
  { label: 'go', value: 'go' },
  { label: 'nuget', value: 'nuget' }
]

const statusFilterOptions = [
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '缓存', value: 'cached' }
]

const columns = [
  { prop: 'id', label: 'ID', width: '60px' },
  { prop: 'repository', label: '仓库', width: '140px' },
  { prop: 'package_type', label: '类型', width: '80px' },
  { prop: 'package_name', label: '包名' },
  { prop: 'version', label: '版本', width: '100px' },
  { prop: 'status', label: '状态', width: '80px' },
  { prop: 'error_message', label: '失败原因' },
  { prop: 'status_code', label: 'HTTP状态', width: '90px' },
  { prop: 'size_bytes', label: '大小', width: '100px' },
  { prop: 'duration_ms', label: '耗时', width: '90px' },
  { prop: 'from_cache', label: '缓存', width: '70px' },
  { prop: 'ip_address', label: 'IP', width: '130px' },
  { prop: 'created_at', label: '时间', width: '180px' },
]

function formatDate(d: string): string {
  if (!d) return '-'
  return new Date(d).toLocaleString('zh-CN')
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

function statusTagType(s: string): 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info' {
  const map: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'> = { success: 'success', failed: 'danger', cached: 'info' }
  return map[s] || 'info'
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
    repositories.value = res || []
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
    const res = await proxyDownloadApi.getStats(params)
    stats.value = res || ({} as ProxyDownloadStats)
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

    const res = await proxyDownloadApi.list(params)
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
.proxy-download-logs {
  padding: var(--spacing-xl);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}

.stat-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr) 2fr;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-2xl);
}

.stat-card {
  text-align: center;
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-bold);
  color: var(--color-text-primary);
}

.stat-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-tertiary);
  margin-top: var(--spacing-xs);
}

.stat-card.success .stat-value {
  color: var(--color-success);
}

.stat-card.danger .stat-value {
  color: var(--color-danger);
}

.stat-card.info .stat-value {
  color: var(--color-primary);
}

.filter-bar {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.pagination {
  margin-top: var(--spacing-lg);
  display: flex;
  justify-content: flex-end;
}

.error-message {
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}
</style>
