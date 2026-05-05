<template>
  <div class="backup-management">
    <div class="page-header">
      <h2>备份管理</h2>
      <CustomButton type="primary" :icon="Plus" @click="showCreateDialog">
        创建备份
      </CustomButton>
    </div>

    <CustomTable :columns="columns" :data="backups" :loading="loading" row-key="id">
      <template #description="{ row }">
        {{ row.description || '-' }}
      </template>
      <template #size="{ row }">
        {{ formatSize(row.size) }}
      </template>
      <template #status="{ row }">
        <CustomTag :type="getStatusType(row.status)" size="small">
          {{ getStatusText(row.status) }}
        </CustomTag>
      </template>
      <template #created_at="{ row }">
        {{ formatDate(row.created_at) }}
      </template>
      <template #created_by="{ row }">
        {{ row.created_by || '-' }}
      </template>
      <template #actions="{ row }">
        <div class="action-buttons">
          <CustomButton
            size="small"
            type="primary"
            @click="handleRestore(row)"
            :disabled="row.status !== 'completed'"
          >
            恢复
          </CustomButton>
          <CustomButton
            size="small"
            type="outline"
            @click="handleDelete(row)"
          >
            删除
          </CustomButton>
        </div>
      </template>
    </CustomTable>

    <CustomDialog v-model="createVisible" title="创建备份" width="500px">
      <el-form :model="createForm" :rules="createRules" ref="createFormRef" label-width="80px">
        <el-form-item label="名称" prop="name">
          <CustomInput v-model="createForm.name" placeholder="请输入备份名称" />
        </el-form-item>
        <el-form-item label="描述" prop="description">
          <CustomInput v-model="createForm.description" placeholder="请输入备份描述（可选）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <CustomButton type="secondary" @click="createVisible = false">取消</CustomButton>
        <CustomButton type="primary" @click="createBackup" :loading="creating">确定</CustomButton>
      </template>
    </CustomDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { backupApi, type Backup } from '@/api/backup'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomTable from '@/components/ui/CustomTable.vue'
import CustomTag from '@/components/ui/CustomTag.vue'
import CustomDialog from '@/components/ui/CustomDialog.vue'
import CustomInput from '@/components/ui/CustomInput.vue'

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

const columns = [
  { prop: 'id', label: 'ID', width: '80px' },
  { prop: 'name', label: '备份名称' },
  { prop: 'description', label: '描述' },
  { prop: 'size', label: '大小', width: '120px' },
  { prop: 'status', label: '状态', width: '120px' },
  { prop: 'created_at', label: '创建时间', width: '180px' },
  { prop: 'created_by', label: '创建人', width: '120px' },
  { prop: 'actions', label: '操作', width: '200px' },
]

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
const getStatusType = (status: string): 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info' => {
  const typeMap: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'> = {
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

.action-buttons {
  display: flex;
  gap: var(--spacing-sm);
}
</style>
