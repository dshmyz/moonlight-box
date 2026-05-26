<template>
  <div class="backup-management">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-database"></i>
        </div>
        <div class="header-text">
          <h2>备份管理</h2>
          <p class="header-subtitle">管理系统数据备份</p>
        </div>
      </div>
      <el-button type="primary" class="create-btn" @click="showCreateDialog">
        <i class="fa-solid fa-plus"></i>
        <span>创建备份</span>
      </el-button>
    </header>

    <div class="content-panel schedule-panel">
      <div class="panel-header">
        <div class="panel-title">
          <i class="fa-solid fa-clock"></i>
          <span>定时备份设置</span>
        </div>
        <el-button type="primary" size="small" @click="saveScheduleConfig" :loading="savingConfig">
          <i class="fa-solid fa-save"></i> 保存
        </el-button>
      </div>
      <el-form :model="scheduleConfig" label-width="100px" class="schedule-form">
        <el-row :gutter="24">
          <el-col :span="8">
            <el-form-item label="启用定时备份">
              <el-switch v-model="scheduleConfig.enabled" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="备份间隔">
              <el-select v-model="scheduleConfig.interval" placeholder="选择备份间隔" :disabled="!scheduleConfig.enabled">
                <el-option label="每 1 小时" value="1h" />
                <el-option label="每 6 小时" value="6h" />
                <el-option label="每 12 小时" value="12h" />
                <el-option label="每 24 小时" value="24h" />
                <el-option label="每 48 小时" value="48h" />
                <el-option label="每 7 天" value="168h" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="备份时间">
              <el-time-picker
                v-model="scheduleTime"
                format="HH:mm"
                value-format="HH:mm"
                placeholder="选择时间"
                :disabled="!scheduleConfig.enabled"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
    </div>

    <div class="content-panel" v-loading="loading">
      <el-table
        :data="backups"
        style="width: 100%"
        :header-cell-style="{ background: '#fafbfc' }"
        :row-class-name="tableRowClass"
        @row-mouse-enter="handleRowEnter"
        @row-mouse-leave="handleRowLeave"
      >
        <el-table-column prop="id" label="ID" width="70" align="center" />
        <el-table-column prop="name" label="备份名称" min-width="180">
          <template #default="{ row }">
            <span class="backup-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="200">
          <template #default="{ row }">
            <span class="description-text">{{ row.description || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120" align="center">
          <template #default="{ row }">
            <span class="size-text">{{ formatSize(row.size) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :class="['status-tag', `status-tag--${row.status}`]" size="small">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="170" align="center">
          <template #default="{ row }">
            <span class="time-text">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_by" label="创建人" width="120" align="center">
          <template #default="{ row }">
            <span class="creator-text">{{ row.created_by || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" align="center">
          <template #default="{ row }">
            <div class="operation-buttons">
              <el-button
                class="btn-restore"
                size="small"
                @click="handleRestore(row)"
                :disabled="row.status !== 'completed'"
              >
                <i class="fa-solid fa-rotate"></i> 恢复
              </el-button>
              <el-button
                class="btn-delete"
                size="small"
                type="text"
                @click="handleDelete(row)"
              >
                <i class="fa-solid fa-trash"></i>
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="createVisible" title="创建备份" width="500px" class="create-dialog">
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
import { ref, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { backupApi, type Backup } from '@/api/backup'
import { formatDate, formatSize } from '@/utils/format'
import { useTableRowHover } from '@/composables/useTableRowHover'

const { tableRowClass, handleRowEnter, handleRowLeave } = useTableRowHover()

const loading = ref(false)
const creating = ref(false)
const savingConfig = ref(false)
const backups = ref<Backup[]>([])
const createVisible = ref(false)
const createFormRef = ref<FormInstance>()
const scheduleTime = ref('02:00')

const createForm = ref({
  name: '',
  description: '',
})

const scheduleConfig = ref({
  enabled: true,
  interval: '24h',
  time: '02:00',
})

watch(() => scheduleConfig.value.enabled, (enabled) => {
  if (!enabled) {
    scheduleConfig.value.interval = '24h'
    scheduleTime.value = '02:00'
  }
})

const createRules: FormRules = {
  name: [
    { required: true, message: '请输入备份名称', trigger: 'blur' },
    { min: 2, max: 100, message: '长度在 2 到 100 个字符', trigger: 'blur' },
  ],
}

const getStatusText = (status: string): string => {
  const textMap: Record<string, string> = {
    pending: '等待中',
    creating: '创建中',
    completed: '已完成',
    failed: '失败',
  }
  return textMap[status] || status
}

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

const loadScheduleConfig = async () => {
  try {
    const config = await backupApi.getConfig()
    scheduleConfig.value = config as any
    scheduleTime.value = (config as any).time || '02:00'
  } catch {
    ElMessage.error('加载备份配置失败')
  }
}

const saveScheduleConfig = async () => {
  savingConfig.value = true
  try {
    await backupApi.updateConfig({
      enabled: scheduleConfig.value.enabled,
      interval: scheduleConfig.value.interval,
      time: scheduleTime.value,
    })
    ElMessage.success('备份配置保存成功')
    await loadScheduleConfig()
  } catch {
    ElMessage.error('保存备份配置失败')
  } finally {
    savingConfig.value = false
  }
}

const showCreateDialog = () => {
  createForm.value = { name: '', description: '' }
  createVisible.value = true
}

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

onMounted(() => {
  loadBackups()
  loadScheduleConfig()
})
</script>

<style scoped>
.backup-management {
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
  background: linear-gradient(135deg, #06b6d4 0%, #0891b2 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 22px;
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

.create-btn {
  height: 40px;
  padding: 0 20px;
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

.create-btn:hover {
  background: linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.3);
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.schedule-panel {
  margin-bottom: 16px;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid #f1f5f9;
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: #1f2937;
}

.panel-title i {
  color: #06b6d4;
}

.schedule-form {
  padding: 8px 0;
}

.schedule-form :deep(.el-form-item) {
  margin-bottom: 0;
}

.schedule-form :deep(.el-form-item__label) {
  font-weight: 500;
  color: #475569;
}

:deep(.el-table .row-hovered) {
  background: #f8fafc;
}

.backup-name {
  font-weight: 500;
  color: #1f2937;
}

.description-text {
  color: #6b7280;
  font-size: 13px;
}

.size-text {
  font-weight: 500;
  color: #059669;
}

.status-tag {
  border: none;
  font-weight: 500;
}

.status-tag--pending { background: #f3f4f6; color: #6b7280; }
.status-tag--creating { background: #fffbeb; color: #d97706; }
.status-tag--completed { background: #ecfdf5; color: #059669; }
.status-tag--failed { background: #fef2f2; color: #dc2626; }

.time-text {
  font-size: 13px;
  color: #6b7280;
}

.creator-text {
  color: #374151;
}

.operation-buttons {
  display: flex;
  align-items: center;
  gap: 4px;
}

.btn-restore {
  background: #f0fdf4;
  color: #15803d;
  border-color: #bbf7d0;
}

.btn-restore:hover {
  background: #dcfce7;
}

.btn-delete {
  color: #ef4444;
}

.btn-delete:hover {
  background: #fef2f2;
}
</style>
