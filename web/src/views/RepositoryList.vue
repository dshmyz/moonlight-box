<template>
  <div class="repository-list">
    <div class="page-header">
      <h2>仓库管理</h2>
      <el-button type="primary" @click="openCreateDialog">
        <el-icon><Plus /></el-icon> 创建仓库
      </el-button>
    </div>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="全部" name="all" />
      <el-tab-pane label="Local" name="local" />
      <el-tab-pane label="Proxy" name="proxy" />
      <el-tab-pane label="Virtual" name="virtual" />
    </el-tabs>

    <el-table :data="filteredRepos" v-loading="loading" style="width: 100%">
      <el-table-column prop="name" label="仓库名称" width="180" />
      <el-table-column prop="display_name" label="显示名称" />
      <el-table-column prop="type" label="类型" width="100">
        <template #default="{ row }">
          <el-tag :type="getTypeTag(row.type)" size="small">
            {{ row.type }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="package_type" label="包类型" width="100" />
      <el-table-column label="同步状态" width="200">
        <template #default="{ row }">
          <div v-if="row.type === 'proxy' && row.metadata_sync_enabled">
            <el-tag :type="getSyncStatusType(row.last_sync_status)">
              {{ getSyncStatusText(row.last_sync_status) }}
            </el-tag>
            <div class="sync-time" v-if="row.last_metadata_sync_at">
              {{ formatTime(row.last_metadata_sync_at) }}
            </div>
          </div>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column prop="remote_url" label="远程地址" show-overflow-tooltip />
      <el-table-column prop="enabled" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'danger'" size="small">
            {{ row.enabled ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="300">
        <template #default="{ row }">
          <el-button-group>
            <el-button size="small" @click="openEditDialog(row)">编辑</el-button>
            <el-button
              v-if="row.type === 'proxy'"
              size="small"
              type="primary"
              @click="handleSyncMetadata(row)"
              :loading="row.syncing"
            >
              同步元数据
            </el-button>
            <el-popconfirm title="确定删除此仓库?" @confirm="deleteRepo(row.name)">
              <template #reference>
                <el-button size="small" type="danger">删除</el-button>
              </template>
            </el-popconfirm>
          </el-button-group>
        </template>
      </el-table-column>
    </el-table>

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

interface LocalRepository extends Repository {
  syncing?: boolean
}

const loading = ref(false)
const activeTab = ref('all')
const showDialog = ref(false)
const editingRepo = ref<Repository | null>(null)
const repos = ref<LocalRepository[]>([])

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
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
}

.sync-time {
  font-size: 12px;
  color: #86909c;
  margin-top: 4px;
}
</style>
