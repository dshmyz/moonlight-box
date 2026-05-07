<template>
  <div class="cache-management">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-bolt"></i>
        </div>
        <div class="header-text">
          <h2>缓存管理</h2>
          <p class="header-subtitle">管理缓存配置和清理缓存数据</p>
        </div>
      </div>
      <el-button type="danger" class="clear-btn" @click="handleClearCache" :loading="clearing">
        <i class="fa-solid fa-trash"></i>
        <span>清空缓存</span>
      </el-button>
    </header>

    <div class="stats-bar">
      <div class="stat-card stat-card--entries">
        <div class="stat-icon">
          <i class="fa-solid fa-boxes"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.positive_items.toLocaleString() }}</span>
          <span class="stat-label">正缓存条目</span>
        </div>
      </div>
      <div class="stat-card stat-card--negative">
        <div class="stat-icon">
          <i class="fa-solid fa-circle-xmark"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.negative_items.toLocaleString() }}</span>
          <span class="stat-label">负缓存条目</span>
        </div>
      </div>
      <div class="stat-card stat-card--size">
        <div class="stat-icon">
          <i class="fa-solid fa-hdd"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ formatSize(stats.used_bytes) }}</span>
          <span class="stat-label">已用空间</span>
        </div>
      </div>
      <div class="stat-card stat-card--expired">
        <div class="stat-icon">
          <i class="fa-solid fa-clock"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.expired_entries.toLocaleString() }}</span>
          <span class="stat-label">过期条目</span>
        </div>
        <el-button
          v-if="stats.expired_entries > 0"
          type="warning"
          size="small"
          class="cleanup-btn"
          @click="handleCleanupExpired"
          :loading="cleaningExpired"
        >
          清理
        </el-button>
      </div>
      <div class="stat-card stat-card--max">
        <div class="stat-icon">
          <i class="fa-solid fa-gauge-high"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.max_size_gb.toFixed(1) }} GB</span>
          <span class="stat-label">最大容量</span>
        </div>
      </div>
    </div>

    <div class="content-panel">
      <div class="panel-header">
        <div class="panel-title">
          <i class="fa-solid fa-list"></i>
          <span>缓存项列表</span>
        </div>
        <div class="header-actions">
          <div class="search-wrapper">
            <i class="fa-solid fa-search search-icon"></i>
            <el-input
              v-model="searchQuery"
              placeholder="搜索缓存键..."
              clearable
              @clear="handleSearch"
              @keyup.enter="handleSearch"
              class="search-input"
            />
          </div>
          <el-button type="primary" @click="handleSearch" :loading="loading">
            <i class="fa-solid fa-search"></i>
            <span>搜索</span>
          </el-button>
          <el-button @click="loadItems" :loading="loading">
            <i class="fa-solid fa-refresh"></i>
            <span>刷新</span>
          </el-button>
        </div>
      </div>

      <el-table
        :data="items"
        v-loading="loading"
        stripe
        class="cache-table"
        :header-cell-style="{ background: '#f8fafc', color: '#475569', fontWeight: 600 }"
      >
        <el-table-column prop="key" label="缓存键" min-width="300" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="key-cell">
              <span class="key-text" :title="row.key">{{ row.key }}</span>
              <el-tag v-if="row.is_negative" type="danger" size="small">负缓存</el-tag>
              <el-tag v-if="row.is_expired" type="warning" size="small">已过期</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120" align="right">
          <template #default="{ row }">
            {{ formatSize(row.size) }}
          </template>
        </el-table-column>
        <el-table-column prop="content_type" label="类型" width="120">
          <template #default="{ row }">
            <span class="content-type">{{ row.content_type || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="remaining_ttl" label="剩余TTL" width="120" align="center">
          <template #default="{ row }">
            <span v-if="!row.is_expired" class="ttl-text">{{ formatTTL(row.remaining_ttl) }}</span>
            <span v-else class="ttl-expired">已过期</span>
          </template>
        </el-table-column>
        <el-table-column prop="expiry" label="过期时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.expiry) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" align="center">
          <template #default="{ row }">
            <el-button
              type="danger"
              size="small"
              text
              @click="handleDeleteItem(row)"
              :loading="deletingKey === row.key"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrapper">
        <span class="total-count">共 {{ total }} 条</span>
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[20, 50, 100, 200]"
          :total="total"
          layout="sizes, prev, pager, next"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </div>

    <div class="content-panel invalidate-panel">
      <div class="panel-header">
        <div class="panel-title">
          <i class="fa-solid fa-refresh-cw"></i>
          <span>缓存失效</span>
        </div>
      </div>
      <div class="invalidate-form">
        <div class="input-wrapper">
          <i class="fa-solid fa-search input-icon"></i>
          <el-input
            v-model="invalidateForm.pattern"
            placeholder="输入匹配模式，如: npm:*:lodash"
            @keyup.enter="handleInvalidate"
          />
        </div>
        <el-button type="primary" class="invalidate-btn" @click="handleInvalidate" :loading="invalidating">
          <i class="fa-solid fa-rotate-right"></i>
          <span>使缓存失效</span>
        </el-button>
      </div>
      <div class="pattern-hints">
        <span class="hints-label">匹配模式说明：</span>
        <code>*</code> 匹配所有
        <code>npm:*</code> 匹配 npm 分片下所有
        <code>npm:*:lodash</code> 匹配 npm 分片下 lodash 相关
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { cacheApi, type CacheListItem, type CacheStats } from '@/api/cache'

const loading = ref(false)
const clearing = ref(false)
const invalidating = ref(false)
const cleaningExpired = ref(false)
const deletingKey = ref<string | null>(null)

const stats = ref<CacheStats>({
  total_items: 0,
  positive_items: 0,
  negative_items: 0,
  total_size: 0,
  used_bytes: 0,
  max_bytes: 0,
  max_items: 0,
  num_shards: 0,
  expired_entries: 0,
  max_size_gb: 0,
})

const items = ref<CacheListItem[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = ref(50)
const searchQuery = ref('')

const invalidateForm = ref({
  pattern: '',
})

const formatSize = (bytes: number) => {
  if (!bytes || bytes < 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

const formatTTL = (seconds: number) => {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`
}

const formatTime = (timeStr: string) => {
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

const loadStats = async () => {
  try {
    const res = await cacheApi.getStats()
    stats.value = {
      total_items: res.total_items || 0,
      positive_items: res.positive_items || 0,
      negative_items: res.negative_items || 0,
      total_size: res.total_size || 0,
      used_bytes: res.used_bytes || 0,
      max_bytes: res.max_bytes || 0,
      max_items: res.max_items || 0,
      num_shards: res.num_shards || 0,
      expired_entries: res.expired_entries || 0,
      max_size_gb: res.max_size_gb || 0,
    }
  } catch {
    ElMessage.error('加载缓存统计失败')
  }
}

const loadItems = async () => {
  loading.value = true
  try {
    const offset = (currentPage.value - 1) * pageSize.value
    const res = await cacheApi.list({
      offset,
      limit: pageSize.value,
      search: searchQuery.value,
    })
    items.value = res.items
    total.value = res.total
  } catch {
    ElMessage.error('加载缓存项失败')
  } finally {
    loading.value = false
  }
}

const handleSearch = async () => {
  currentPage.value = 1
  await loadItems()
}

const handleSizeChange = async () => {
  currentPage.value = 1
  await loadItems()
}

const handlePageChange = async () => {
  await loadItems()
}

const handleClearCache = async () => {
  try {
    await ElMessageBox.confirm('确定要清空所有缓存吗？', '警告', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    clearing.value = true
    await cacheApi.clear()
    ElMessage.success('缓存已清空')
    await loadStats()
    await loadItems()
  } catch (err: unknown) {
    if (err !== 'cancel' && err !== 'Error: cancel') {
      ElMessage.error('清空缓存失败')
    }
  } finally {
    clearing.value = false
  }
}

const handleDeleteItem = async (item: CacheListItem) => {
  try {
    await ElMessageBox.confirm(`确定要删除缓存项 "${item.key}" 吗？`, '确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    deletingKey.value = item.key
    await cacheApi.deleteItem(item.key)
    ElMessage.success('删除成功')
    await loadStats()
    await loadItems()
  } catch (err: unknown) {
    if (err !== 'cancel' && err !== 'Error: cancel') {
      ElMessage.error('删除失败')
    }
  } finally {
    deletingKey.value = null
  }
}

const handleCleanupExpired = async () => {
  try {
    await ElMessageBox.confirm('确定要清理所有过期缓存项吗？', '确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    cleaningExpired.value = true
    const res = await cacheApi.cleanupExpired()
    ElMessage.success(`已清理 ${res.cleaned} 个过期项`)
    await loadStats()
    await loadItems()
  } catch (err: unknown) {
    if (err !== 'cancel' && err !== 'Error: cancel') {
      ElMessage.error('清理失败')
    }
  } finally {
    cleaningExpired.value = false
  }
}

const handleInvalidate = async () => {
  if (!invalidateForm.value.pattern) {
    ElMessage.warning('请输入匹配模式')
    return
  }
  invalidating.value = true
  try {
    await cacheApi.invalidate({ pattern: invalidateForm.value.pattern })
    ElMessage.success('缓存失效操作成功')
    await loadStats()
    await loadItems()
    invalidateForm.value.pattern = ''
  } catch {
    ElMessage.error('缓存失效操作失败')
  } finally {
    invalidating.value = false
  }
}

onMounted(() => {
  loadStats()
  loadItems()
})
</script>

<style scoped>
.cache-management {
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
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #f59e0b 0%, #d97706 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 24px;
}

.header-text h2 {
  font-size: 20px;
  font-weight: 600;
  margin: 0;
  color: #1f2937;
  letter-spacing: -0.2px;
}

.header-subtitle {
  font-size: 13px;
  color: #9ca3af;
  margin: 4px 0 0;
}

.clear-btn {
  height: 40px;
  padding: 0 20px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  border-color: transparent;
  transition: all 0.2s ease;
}

.clear-btn:hover:not(:disabled) {
  background: linear-gradient(135deg, #dc2626 0%, #b91c1c 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.3);
}

.stats-bar {
  display: flex;
  gap: 16px;
  margin-bottom: 16px;
}

.stat-card {
  flex: 1;
  padding: 16px 20px;
  background: #fff;
  border-radius: 14px;
  display: flex;
  align-items: center;
  gap: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  transition: all 0.2s ease;
  position: relative;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.08);
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
}

.stat-card--entries .stat-icon {
  background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%);
  color: #2563eb;
}

.stat-card--negative .stat-icon {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.stat-card--size .stat-icon {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #16a34a;
}

.stat-card--expired .stat-icon {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #ef4444;
}

.stat-card--max .stat-icon {
  background: linear-gradient(135deg, #fce7f3 0%, #fbcfe8 100%);
  color: #be185d;
}

.stat-info {
  display: flex;
  flex-direction: column;
  flex: 1;
}

.stat-value {
  font-size: 22px;
  font-weight: 700;
  color: #1f2937;
  line-height: 1.2;
}

.stat-label {
  font-size: 12px;
  color: #9ca3af;
  margin-top: 4px;
}

.cleanup-btn {
  position: absolute;
  right: 16px;
  top: 50%;
  transform: translateY(-50%);
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  margin-bottom: 16px;
}

.invalidate-panel {
  margin-bottom: 0;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
  flex-wrap: wrap;
  gap: 12px;
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #1f2937;
}

.panel-title i {
  color: #2563eb;
}

.header-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}

.search-wrapper {
  position: relative;
  width: 260px;
}

.search-icon {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  color: #9ca3af;
  font-size: 14px;
  z-index: 1;
}

.search-input :deep(.el-input__wrapper) {
  padding-left: 36px;
}

.cache-table {
  margin: 0 -20px;
  padding: 0 20px;
}

.key-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.key-text {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 13px;
  color: #374151;
}

.content-type {
  font-size: 12px;
  color: #6b7280;
}

.ttl-text {
  font-size: 13px;
  color: #059669;
}

.ttl-expired {
  font-size: 13px;
  color: #ef4444;
}

.pagination-wrapper {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 16px;
  padding: 0 4px;
}

.total-count {
  font-size: 13px;
  color: #6b7280;
}

.invalidate-form {
  display: flex;
  gap: 16px;
  align-items: center;
}

.input-wrapper {
  flex: 1;
  max-width: 500px;
  position: relative;
}

.input-icon {
  position: absolute;
  left: 14px;
  top: 50%;
  transform: translateY(-50%);
  color: #9ca3af;
  font-size: 14px;
}

.input-wrapper :deep(.el-input__wrapper) {
  padding-left: 42px;
  border-radius: 10px;
  border: 1px solid #e5e7eb;
  transition: all 0.2s ease;
}

.input-wrapper :deep(.el-input__wrapper:hover) {
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.input-wrapper :deep(.el-input__wrapper.is-focus) {
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.invalidate-btn {
  height: 40px;
  padding: 0 24px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border-color: transparent;
  transition: all 0.2s ease;
}

.invalidate-btn:hover:not(:disabled) {
  background: linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.3);
}

.pattern-hints {
  margin-top: 12px;
  font-size: 12px;
  color: #9ca3af;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.pattern-hints .hints-label {
  font-weight: 500;
  color: #6b7280;
}

.pattern-hints code {
  background: #f3f4f6;
  padding: 2px 6px;
  border-radius: 4px;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 11px;
  color: #374151;
}
</style>