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
      <el-table-column prop="remote_url" label="远程地址" show-overflow-tooltip />
      <el-table-column prop="enabled" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'danger'" size="small">
            {{ row.enabled ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="200">
        <template #default="{ row }">
          <el-button size="small" @click="openEditDialog(row)">编辑</el-button>
          <el-popconfirm title="确定删除此仓库?" @confirm="deleteRepo(row.name)">
            <template #reference>
              <el-button size="small" type="danger">删除</el-button>
            </template>
          </el-popconfirm>
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

const loading = ref(false)
const activeTab = ref('all')
const showDialog = ref(false)
const editingRepo = ref<Repository | null>(null)
const repos = ref<Repository[]>([])

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
    repos.value = res.data || []
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
</style>
