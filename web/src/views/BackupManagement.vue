<template>
  <div class="backup-management">
    <div class="page-header">
      <h2>备份管理</h2>
      <el-button type="primary" @click="showCreateDialog">
        <el-icon><Plus /></el-icon> 创建备份
      </el-button>
    </div>

    <el-table :data="backups" v-loading="loading" style="width: 100%">
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="name" label="备份名称" min-width="180" />
      <el-table-column prop="description" label="描述" min-width="200">
        <template #default="{ row }">
          {{ row.description || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="size" label="大小" width="120">
        <template #default="{ row }">
          {{ formatSize(row.size) }}
        </template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="120">
        <template #default="{ row }">
          <el-tag :type="getStatusType(row.status)" size="small">
            {{ getStatusText(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="180">
        <template #default="{ row }">
          {{ formatDate(row.created_at) }}
        </template>
      </el-table-column>
      <el-table-column prop="created_by" label="创建人" width="120">
        <template #default="{ row }">
          {{ row.created_by || '-' }}
        </template>
      </el-table-column>
      <el-table-column label="操作" width="200" fixed="right">
        <template #default="{ row }">
          <el-button
            size="small"
            type="primary"
            @click="handleRestore(row)"
            :disabled="row.status !== 'completed'"
          >
            恢复
          </el-button>
          <el-button
            size="small"
            type="danger"
            @click="handleDelete(row)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="createVisible" title="创建备份" width="500px">
      <el-form :model="createForm" :rules="createRules" ref="createFormRef" label-width="80px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="createForm.name" placeholder="请输入备份名称" />
        </el-form-item>
        <el-form-item label="描述" prop="description">
          <el-input
            v-model="createForm.description"
            type="textarea"
            :rows="3"
            placeholder="请输入备份描述（可选）"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" @click="createBackup" :loading="creating">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { backupApi, type Backup } from '@/api/backup'

const loading = ref(false)
const creating = ref(false)
const backups = ref<Backup[]>([])
const createVisible = ref(false)
const createFormRef = ref<FormInstance>()

const createForm = ref({
  name: '',
  description: '',
})

const createRules: FormRules = {
  name: [
    { required: true, message: '请输入备份名称', trigger: 'blur' },
    { min: 2, max: 100, message: '长度在 2 到 100 个字符', trigger: 'blur' },
  ],
}

/** 格式化文件大小 */
const formatSize = (bytes: number): string => {
  if (!bytes || bytes < 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

/** 格式化日期 */
const formatDate = (date: string): string => {
  if (!date) return '-'
  return new Date(date).toLocaleString('zh-CN')
}

/** 获取状态类型 */
const getStatusType = (status: string): string => {
  const typeMap: Record<string, string> = {
    pending: 'info',
    creating: 'warning',
    completed: 'success',
    failed: 'danger',
  }
  return typeMap[status] || 'info'
}

/** 获取状态文本 */
const getStatusText = (status: string): string => {
  const textMap: Record<string, string> = {
    pending: '等待中',
    creating: '创建中',
    completed: '已完成',
    failed: '失败',
  }
  return textMap[status] || status
}

/** 加载备份列表 */
const loadBackups = async () => {
  loading.value = true
  try {
    const res = await backupApi.list()
    const data = res as any
    backups.value = data?.list || []
  } catch {
    ElMessage.error('加载备份列表失败')
  } finally {
    loading.value = false
  }
}

/** 显示创建对话框 */
const showCreateDialog = () => {
  createForm.value = { name: '', description: '' }
  createVisible.value = true
}

/** 创建备份 */
const createBackup = async () => {
  if (!createFormRef.value) return

  await createFormRef.value.validate(async (valid) => {
    if (!valid) return

    creating.value = true
    try {
      await backupApi.create(createForm.value)
      ElMessage.success('备份创建成功')
      createVisible.value = false
      await loadBackups()
    } catch {
      ElMessage.error('创建备份失败')
    } finally {
      creating.value = false
    }
  })
}

/** 恢复备份 */
const handleRestore = async (backup: Backup) => {
  try {
    await ElMessageBox.confirm(
      `确定要恢复备份 "${backup.name}" 吗？此操作将覆盖当前数据。`,
      '警告',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )

    await backupApi.restore(backup.id)
    ElMessage.success('备份恢复成功')
    await loadBackups()
  } catch (err: unknown) {
    if (err !== 'cancel' && err !== 'Error: cancel') {
      ElMessage.error('恢复备份失败')
    }
  }
}

/** 删除备份 */
const handleDelete = async (backup: Backup) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除备份 "${backup.name}" 吗？`,
      '警告',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )

    await backupApi.delete(backup.id)
    ElMessage.success('备份删除成功')
    await loadBackups()
  } catch (err: unknown) {
    if (err !== 'cancel' && err !== 'Error: cancel') {
      ElMessage.error('删除备份失败')
    }
  }
}

onMounted(loadBackups)
</script>

<style scoped>
.backup-management {
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h2 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
}
</style>
