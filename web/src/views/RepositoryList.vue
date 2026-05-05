<template>
  <div class="repository-list">
    <div class="page-header">

      <h2>仓库管理</h2>
      <CustomButton type="primary" @click="openCreateDialog">
        <template #icon>
          <Plus />
        </template>
        创建仓库
      </CustomButton>
    </div>

    <CustomTabs v-model="activeTab" :tabs="tabOptions" />

    <CustomTable
      :columns="tableColumns"
      :data="filteredRepos"
      :loading="loading"
      row-key="name"
    >
      <template #type="{ row }">
        <CustomTag :type="getTypeTag(row.type)" size="small">
          {{ row.type }}
        </CustomTag>
      </template>

      <template #sync_status="{ row }">
        <div v-if="row.type === 'proxy' && row.metadata_sync_enabled">
          <CustomTag :type="getSyncStatusType(row.last_sync_status)" size="small">
            {{ getSyncStatusText(row.last_sync_status) }}
          </CustomTag>
          <div class="sync-time" v-if="row.last_metadata_sync_at">
            {{ formatTime(row.last_metadata_sync_at) }}
          </div>
        </div>
        <span v-else>-</span>
      </template>

      <template #enabled="{ row }">
        <CustomTag :type="row.enabled ? 'success' : 'danger'" size="small">
          {{ row.enabled ? '启用' : '禁用' }}
        </CustomTag>
      </template>

      <template #operations="{ row }">
        <div class="operation-buttons">
          <CustomButton size="small" @click="openEditDialog(row)">编辑</CustomButton>
          <CustomButton
            v-if="row.type === 'proxy'"
            size="small"
            type="primary"
            @click="handleSyncMetadata(row)"
            :loading="row.syncing"
          >
            同步元数据
          </CustomButton>
          <el-popconfirm title="确定删除此仓库?" @confirm="deleteRepo(row.name)">
            <template #reference>
              <CustomButton size="small" type="outline">删除</CustomButton>
            </template>
          </el-popconfirm>
        </div>
      </template>
    </CustomTable>

    <RepositoryFormDialog
      v-model="showDialog"
      :edit-data="editingRepo"
      @submit="handleFormSubmit"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { repositoryApi, type Repository } from '@/api/repository'
import RepositoryFormDialog from '@/components/repository/RepositoryFormDialog.vue'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomTable from '@/components/ui/CustomTable.vue'
import CustomTag from '@/components/ui/CustomTag.vue'
import CustomTabs from '@/components/ui/CustomTabs.vue'

interface LocalRepository extends Repository {
  syncing?: boolean
}

const loading = ref(false)
const activeTab = ref('all')
const showDialog = ref(false)
const editingRepo = ref<Repository | null>(null)
const repos = ref<LocalRepository[]>([])

const tabOptions = [
  { name: 'all', label: '全部' },
  { name: 'local', label: 'Local' },
  { name: 'proxy', label: 'Proxy' },
  { name: 'virtual', label: 'Virtual' },
]

const tableColumns = [
  { prop: 'name', label: '仓库名称', width: '180px' },
  { prop: 'display_name', label: '显示名称' },
  { prop: 'type', label: '类型', width: '100px' },
  { prop: 'package_type', label: '包类型', width: '100px' },
  { prop: 'sync_status', label: '同步状态', width: '200px' },
  { prop: 'remote_url', label: '远程地址' },
  { prop: 'enabled', label: '状态', width: '80px' },
  { prop: 'operations', label: '操作', width: '300px' },
]

const filteredRepos = computed(() => {
  if (activeTab.value === 'all') return repos.value
  return repos.value.filter(r => r.type === activeTab.value)
})

const getTypeTag = (type: string) => {
  switch (type) {
    case 'local':
      return 'success'
    case 'proxy':
      return 'warning'
    case 'virtual':
      return 'primary'
    default:
      return 'info'
  }
}

const loadRepos = async () => {
  loading.value = true
  try {
    const res = await repositoryApi.list()
    repos.value = res || []
  } catch (err) {
    ElMessage.error('加载仓库列表失败')
  } finally {
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

const handleFormSubmit = () => {
  loadRepos()
}

const deleteRepo = async (name: string) => {
  try {
    await repositoryApi.delete(name)
    ElMessage.success('删除成功')
    loadRepos()
  } catch (err) {
    ElMessage.error('删除失败')
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

const getSyncStatusType = (status: string) => {
  switch (status) {
    case 'success':
      return 'success'
    case 'failed':
      return 'danger'
    case 'partial':
      return 'warning'
    default:
      return 'info'
  }
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

const formatTime = (time: string) => {
  return new Date(time).toLocaleString('zh-CN')
}

onMounted(loadRepos)
</script>

<style scoped>
.repository-list {
  padding: var(--spacing-xl);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
}

.page-header h2 {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
}

.sync-time {
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
  margin-top: var(--spacing-xs);
}

.operation-buttons {
  display: flex;
  gap: var(--spacing-sm);
  align-items: center;
}
</style>
