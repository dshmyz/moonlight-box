<template>
  <div class="repository-list">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-server"></i>
        </div>
        <div class="header-text">
          <h2>仓库管理</h2>
          <p class="header-subtitle">管理本地、代理和虚拟仓库</p>
        </div>
      </div>
      <el-button type="primary" class="create-btn" @click="openCreateDialog">
        <el-icon><Plus /></el-icon>
        <span>创建仓库</span>
      </el-button>
    </header>

    <div class="stats-bar">
      <div class="stat-card stat-card--local">
        <div class="stat-icon">
          <i class="fa-solid fa-folder"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ localCount }}</span>
          <span class="stat-label">Local 仓库</span>
        </div>
      </div>
      <div class="stat-card stat-card--proxy">
        <div class="stat-icon">
          <i class="fa-solid fa-rotate"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ proxyCount }}</span>
          <span class="stat-label">Proxy 仓库</span>
        </div>
      </div>
      <div class="stat-card stat-card--virtual">
        <div class="stat-icon">
          <i class="fa-solid fa-wand-magic-sparkles"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ virtualCount }}</span>
          <span class="stat-label">Virtual 仓库</span>
        </div>
      </div>
    </div>

    <div class="content-panel" v-loading="loading">
      <el-tabs v-model="activeTab" class="type-tabs">
        <el-tab-pane v-for="tab in tabOptions" :key="tab.name" :label="tab.label" :name="tab.name">
          <div class="tab-content">
            <el-table 
              :data="filteredRepos" 
              row-key="name" 
              style="width: 100%"
              :header-cell-style="{ background: '#fafbfc' }"
            >
              <el-table-column prop="name" label="名称" min-width="160" show-overflow-tooltip>
                <template #default="{ row }">
                  <div class="repo-info">
                    <div class="repo-icon" :class="`repo-icon--${row.type}`"><i :class="getRepoIcon(row.type)"></i></div>
                    <div class="repo-content">
                      <div class="repo-name">{{ row.display_name || row.name }}</div>
                      <div class="repo-description">{{ row.description || '暂无描述' }}</div>
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column prop="type" label="类型" width="80" align="center">
                <template #default="{ row }">
                  <el-tag :class="['type-tag', `type-tag--${row.type}`]" size="small">{{ row.type }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="url" label="URL" min-width="180" show-overflow-tooltip>
                <template #default="{ row }">
                  <span class="repo-url">{{ row.url || '-' }}</span>
                </template>
              </el-table-column>
              <el-table-column label="代理地址" min-width="200" show-overflow-tooltip>
                <template #default="{ row }">
                  <template v-if="row.type === 'proxy' && row.remote_url">
                    <span class="remote-url">{{ row.remote_url }}</span>
                  </template>
                  <span v-else class="no-value">-</span>
                </template>
              </el-table-column>
              <el-table-column label="同步状态" width="140">
                <template #default="{ row }">
                  <div v-if="row.type === 'proxy' && row.metadata_sync_enabled" class="sync-status">
                    <el-tag :class="['sync-tag', getSyncStatusClass(row.last_sync_status)]" size="small">
                      {{ getSyncStatusText(row.last_sync_status) }}
                    </el-tag>
                  </div>
                  <span v-else class="no-sync">-</span>
                </template>
              </el-table-column>
              <el-table-column label="健康" width="80" align="center">
                <template #default="{ row }">
                  <el-tooltip :content="getHealthTooltip(row)" placement="top" :show-after="300">
                    <span class="health-dot" :class="getHealthClass(row)"></span>
                  </el-tooltip>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="80" align="center">
                <template #default="{ row }">
                  <el-switch
                    :model-value="row.enabled"
                    size="small"
                    @change="(val: boolean) => toggleEnabled(row, val)"
                  />
                </template>
              </el-table-column>
              <el-table-column label="操作" width="220">
                <template #default="{ row }">
                  <div class="operation-buttons">
                    <el-button class="btn-edit" size="small" @click="openEditDialog(row)">
                      编辑
                    </el-button>
                    <el-button class="btn-delete" size="small" link @click="confirmDelete(row)">删除</el-button>
                    <!-- <el-dropdown v-if="row.type === 'proxy'" trigger="click" @command="(cmd: string) => handleProxyCommand(cmd, row)">
                      <el-button class="btn-proxy" size="small" type="primary">
                        更多
                        <el-icon class="el-icon--right"><ArrowDown /></el-icon>
                      </el-button>
                      <template #dropdown>
                        <el-dropdown-menu>
                          <el-dropdown-item command="sync" :disabled="row.syncing">
                            <el-icon><Refresh /></el-icon>
                            {{ row.syncing ? '同步中...' : '同步元数据' }}
                          </el-dropdown-item>
                          <el-dropdown-item command="history">
                            <el-icon><Clock /></el-icon>
                            同步历史
                          </el-dropdown-item>
                        </el-dropdown-menu>
                      </template>
                    </el-dropdown> -->
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>

    <RepositoryFormDialog
      v-model="showDialog"
      :edit-data="editingRepo"
      @submit="handleFormSubmit"
    />

    <SyncHistoryDrawer
      v-model="showSyncHistory"
      :repo-id="selectedRepoId"
      :repo-name="selectedRepoName"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Plus, ArrowDown, Refresh, Clock } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { repositoryApi, type Repository, type RepositoryWithHealth } from '@/api/repository'
import RepositoryFormDialog from '@/components/repository/RepositoryFormDialog.vue'
import SyncHistoryDrawer from '@/components/repository/SyncHistoryDrawer.vue'
import { confirm, success, error } from '@/utils/message'

interface LocalRepository extends RepositoryWithHealth {
  syncing?: boolean
}

const loading = ref(false)
const activeTab = ref('all')
const showDialog = ref(false)
const editingRepo = ref<Repository | null>(null)
const repos = ref<LocalRepository[]>([])
const showSyncHistory = ref(false)
const selectedRepoId = ref<number | null>(null)
const selectedRepoName = ref('')

const tabOptions = [
  { name: 'all', label: '全部' },
  { name: 'local', label: 'Local' },
  { name: 'proxy', label: 'Proxy' },
  { name: 'virtual', label: 'Virtual' },
]

const localCount = computed(() => repos.value.filter(r => r.type === 'local').length)
const proxyCount = computed(() => repos.value.filter(r => r.type === 'proxy').length)
const virtualCount = computed(() => repos.value.filter(r => r.type === 'virtual').length)

const filteredRepos = computed(() => {
  if (activeTab.value === 'all') return repos.value
  return repos.value.filter(r => r.type === activeTab.value)
})

const getRepoIcon = (type: string) => {
  switch (type) {
    case 'local': return 'fa-solid fa-folder'
    case 'proxy': return 'fa-solid fa-rotate'
    case 'virtual': return 'fa-solid fa-wand-magic-sparkles'
    default: return 'fa-solid fa-box'
  }
}

const getSyncStatusClass = (status: string) => {
  switch (status) {
    case 'success': return 'sync-tag--success'
    case 'failed': return 'sync-tag--failed'
    case 'partial': return 'sync-tag--partial'
    default: return 'sync-tag--pending'
  }
}

const getHealthClass = (row: LocalRepository) => {
  if (!row.enabled) return 'health-dot--disabled'
  
  if (row.health_info?.health_status) {
    const health = row.health_info.health_status
    if (!health.is_healthy) return 'health-dot--error'
    if (health.consecutive_failures > 0) return 'health-dot--warning'
    return 'health-dot--healthy'
  }
  
  return 'health-dot--unknown'
}

const getHealthTooltip = (row: LocalRepository) => {
  if (!row.enabled) return '仓库已禁用，健康检查未运行'
  
  if (row.health_info?.health_status) {
    const health = row.health_info.health_status
    const responseTimeMs = Math.round(health.response_time / 1_000_000)
    
    if (!health.is_healthy) {
      return `不健康 | 错误: ${health.last_check_error || '未知错误'} | 连续失败: ${health.consecutive_failures}次`
    }
    if (health.consecutive_failures > 0) {
      return `警告 | 最近有 ${health.consecutive_failures} 次失败，但当前已恢复 | 响应: ${responseTimeMs}ms`
    }
    return `健康 | 响应时间: ${responseTimeMs}ms | 最后检查: ${new Date(health.last_check_time).toLocaleString('zh-CN')}`
  }
  
  return '健康状态未知，等待首次检查完成'
}

const handleProxyCommand = (cmd: string, row: LocalRepository) => {
  if (cmd === 'sync') {
    handleSyncMetadata(row)
  } else if (cmd === 'history') {
    openSyncHistory(row)
  }
}

const loadRepos = async () => {
  loading.value = true
  try {
    const res = await repositoryApi.list()
    repos.value = res || []
    loading.value = false
  } catch (err) {
    ElMessage.error('加载仓库列表失败')
    loading.value = false
  }
}

const openCreateDialog = () => {
  editingRepo.value = null
  showDialog.value = true
}

const openEditDialog = (repo: Repository) => {
  editingRepo.value = { ...repo }
  showDialog.value = true
}

const openSyncHistory = (repo: Repository) => {
  selectedRepoId.value = repo.id
  selectedRepoName.value = repo.display_name || repo.name
  showSyncHistory.value = true
}

const handleFormSubmit = () => {
  loadRepos()
}

const confirmDelete = async (row: Repository) => {
  const ok = await confirm({
    title: '删除确认',
    message: `确定要删除仓库 "${row.name}" 吗？`,
    type: 'warning',
  })
  if (ok) {
    await deleteRepo(row.name)
  }
}

const deleteRepo = async (name: string) => {
  try {
    await repositoryApi.delete(name)
    success('删除成功')
    loadRepos()
  } catch (err) {
    error('删除失败')
  }
}

const toggleEnabled = async (repo: Repository, enabled: boolean) => {
  try {
    await repositoryApi.update(repo.name, { enabled })
    ElMessage.success(enabled ? '已启用' : '已禁用')
    loadRepos()
  } catch (err) {
    ElMessage.error('状态更新失败')
  }
}

const handleSyncMetadata = async (repo: LocalRepository) => {
  try {
    repo.syncing = true
    const task = await repositoryApi.triggerSync(repo.name)

    ElMessage.success('同步任务已启动')

    // 轮询任务状态
    pollSyncTaskStatus(task.id)
  } catch (error: any) {
    ElMessage.error(error.response?.data?.message || '启动同步失败')
  } finally {
    repo.syncing = false
  }
}

const pollSyncTaskStatus = async (taskId: number) => {
  const poll = async () => {
    try {
      const task = await repositoryApi.getSyncTaskStatus(String(taskId))

      if (task.status === 'running') {
        setTimeout(poll, 2000)
      } else {
        // 刷新仓库列表
        await loadRepos()

        if (task.status === 'completed') {
          ElMessage.success(`同步完成：${task.synced_packages}/${task.total_packages} 个包`)
        } else if (task.status === 'failed') {
          ElMessage.error(`同步失败：${task.error_message}`)
        }
      }
    } catch (error) {
      console.error('Failed to poll task status:', error)
    }
  }

  poll()
}

const getSyncStatusText = (status: string) => {
  switch (status) {
    case 'success':
      return '成功'
    case 'failed':
      return '失败'
    case 'partial':
      return '部分成功'
    default:
      return '未同步'
  }
}

onMounted(loadRepos)
</script>

<style scoped>
.repository-list {
  min-height: calc(100vh - 60px);
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
  /* padding: 24px; */
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  padding: 20px 24px;
  background: #ffffff;
  border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
}

.header-text h2 {
  font-size: 22px;
  font-weight: 700;
  margin: 0;
  color: #1e293b;
  letter-spacing: -0.02em;
}

.header-subtitle {
  font-size: 13px;
  color: #64748b;
  margin: 4px 0 0;
}

.create-btn {
  height: 42px;
  padding: 0 24px;
  border-radius: 10px;
  font-weight: 600;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border: none;
  box-shadow: 0 4px 14px rgba(99, 102, 241, 0.3);
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.create-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 24px rgba(99, 102, 241, 0.35);
}

.create-btn:active {
  transform: translateY(0);
}

.create-btn .el-icon {
  font-size: 16px;
}

.stats-bar {
  display: flex;
  gap: 20px;
  margin-bottom: 24px;
}

.stat-card {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 24px;
  background: #ffffff;
  border-radius: 14px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.06);
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

.stat-card--local .stat-icon {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
}

.stat-card--proxy .stat-icon {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
}

.stat-card--virtual .stat-icon {
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
}

.stat-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-value {
  font-size: 26px;
  font-weight: 700;
  color: #1e293b;
  line-height: 1;
}

.stat-label {
  font-size: 13px;
  color: #64748b;
}

.content-panel {
  background: #ffffff;
  border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  overflow: hidden;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.04);
}

.type-tabs {
  height: 100%;
}

.type-tabs :deep(.el-tabs__header) {
  margin: 0;
  background: #fafbfc;
  border-bottom: 1px solid rgba(0, 0, 0, 0.04);
  padding: 0 20px;
}

.type-tabs :deep(.el-tabs__nav-wrap::after) {
  display: none;
}

.type-tabs :deep(.el-tabs__item) {
  font-size: 14px;
  color: #64748b;
  padding: 16px 20px;
  height: auto;
  line-height: 1.5;
  transition: all 0.2s ease;
}

.type-tabs :deep(.el-tabs__item.is-active) {
  color: #6366f1;
  font-weight: 600;
}

.type-tabs :deep(.el-tabs__active-bar) {
  height: 3px;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border-radius: 3px 3px 0 0;
}

.type-tabs :deep(.el-tabs__nav) {
  height: auto;
}

.type-tabs :deep(.el-tabs__content) {
  padding: 0;
}

.tab-content {
  padding: 0;
}

:deep(.el-table) {
  --el-table-header-text-color: #475569;
  --el-table-text-color: #1e293b;
  --el-table-border-color: rgba(0, 0, 0, 0.04);
}

:deep(.el-table th) {
  font-weight: 600;
  font-size: 13px;
  color: #64748b;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  padding: 14px 12px;
}

:deep(.el-table td) {
  padding: 16px 12px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.03);
  transition: all 0.2s ease;
}

:deep(.el-table__body tr:hover td) {
  background: #f8fafc;
}

:deep(.el-table__header .cell) {
  white-space: nowrap;
}

:deep(.el-table__body .cell) {
  white-space: nowrap;
}

.repo-info {
  display: flex;
  align-items: center;
  gap: 12px;
  white-space: nowrap;
}

.repo-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  flex-shrink: 0;
}

.repo-icon--local {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
}

.repo-icon--proxy {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
}

.repo-icon--virtual {
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
}

.repo-content {
  flex: 1;
  min-width: 0;
}

.repo-name {
  font-weight: 600;
  font-size: 14px;
  color: #1e293b;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.repo-description {
  font-size: 12px;
  color: #94a3b8;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.type-tag {
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 6px;
  font-weight: 500;
  border: none;
}

.type-tag--local {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.type-tag--proxy {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.type-tag--virtual {
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
  color: #4f46e5;
}

.repo-url {
  font-size: 13px;
  color: #64748b;
  white-space: nowrap;
}

.remote-url {
  font-size: 13px;
  color: #059669;
  white-space: nowrap;
}

.no-value {
  color: #cbd5e1;
}

.sync-status {
  display: flex;
  align-items: center;
}

.sync-tag {
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 6px;
  font-weight: 500;
  border: none;
  display: inline-block;
  white-space: nowrap;
}

.sync-tag--success {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.sync-tag--failed {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #dc2626;
}

.sync-tag--partial {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.sync-tag--pending {
  background: linear-gradient(135deg, #f1f5f9 0%, #e2e8f0 100%);
  color: #64748b;
}

.no-sync {
  color: #cbd5e1;
}

.health-dot {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  box-shadow: 0 0 4px currentColor;
}

.health-dot--healthy {
  background: #10b981;
  box-shadow: 0 0 6px rgba(16, 185, 129, 0.5);
}

.health-dot--warning {
  background: #f59e0b;
  box-shadow: 0 0 6px rgba(245, 158, 11, 0.5);
}

.health-dot--error {
  background: #ef4444;
  box-shadow: 0 0 6px rgba(239, 68, 68, 0.5);
}

.health-dot--disabled {
  background: #94a3b8;
  box-shadow: none;
}

.health-dot--unknown {
  background: #94a3b8;
  box-shadow: none;
  border: 1px dashed #64748b;
}

.status-tag {
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 6px;
  font-weight: 500;
  border: none;
  white-space: nowrap;
}

.status-tag--enabled {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.status-tag--disabled {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #dc2626;
}

.operation-buttons {
  display: flex;
  gap: 8px;
  align-items: center;
}

.btn-edit {
  border-radius: 8px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  transition: all 0.2s ease;
}

.btn-edit:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
  color: #475569;
}

.btn-sync {
  border-radius: 8px;
  padding: 6px 14px;
  font-size: 12px;
  font-weight: 500;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border: none;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.3);
  transition: all 0.2s ease;
}

.btn-sync:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4);
}

.btn-proxy {
  border-radius: 8px;
  padding: 6px 8px;
  font-size: 12px;
  font-weight: 500;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border: none;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.3);
  transition: all 0.2s ease;
  display: inline-flex;
  align-items: center;
}

.btn-proxy:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4);
}

:deep(.el-dropdown-menu) {
  border-radius: 10px;
  border: 1px solid #e2e8f0;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08);
  padding: 6px;
}

:deep(.el-dropdown-menu__item) {
  padding: 10px 14px;
  border-radius: 6px;
  font-size: 13px;
  color: #475569;
  display: flex;
  align-items: center;
  gap: 10px;
}

:deep(.el-dropdown-menu__item:hover) {
  background: #f1f5f9;
  color: #1e293b;
}

:deep(.el-dropdown-menu__item.is-disabled) {
  color: #cbd5e1;
}

:deep(.el-dropdown-menu__item .el-icon) {
  font-size: 14px;
}

.btn-delete {
  border-radius: 8px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 500;
  color: #dc2626;
  transition: all 0.2s ease;
}

.btn-delete:hover {
  background: #fef2f2;
}
</style>
