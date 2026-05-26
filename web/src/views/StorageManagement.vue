<template>
  <div class="storage-management">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-hard-drive"></i>
        </div>
        <div class="header-text">
          <h2>存储管理</h2>
          <p class="header-subtitle">配置本地存储和云存储后端</p>
        </div>
      </div>
      <el-button type="primary" class="create-btn" @click="handleAdd">
        <el-icon><Plus /></el-icon>
        <span>新增存储</span>
      </el-button>
    </header>

    <div class="content-panel" v-loading="loading">
      <el-table
        :data="storages"
        style="width: 100%"
        :header-cell-style="{ background: '#fafbfc' }"
        :row-class-name="tableRowClass"
        @row-mouse-enter="handleRowEnter"
        @row-mouse-leave="handleRowLeave"
      >
        <el-table-column prop="name" label="名称" min-width="150" />
        <el-table-column prop="type" label="类型" width="110" align="center">
          <template #default="{ row }">
            <el-tag :class="['type-tag', `type-tag--${row.type}`]" size="small">
              <span class="tag-icon"><i :class="getTypeIcon(row.type)"></i></span>
              {{ getTypeLabel(row.type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="180" show-overflow-tooltip />
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :class="['status-tag', row.is_active ? 'status-tag--active' : 'status-tag--disabled']" size="small">
              <span class="status-dot"></span>
              {{ row.is_active ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="默认" width="80" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.is_default" type="warning" size="small">
              <i class="fa-solid fa-star"></i> 默认
            </el-tag>
            <span v-else class="no-default">-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" align="center">
          <template #default="{ row }">
            <div class="operation-buttons">
              <el-button class="btn-test" size="small" @click="handleTest(row)">
                <i class="fa-solid fa-plug"></i> 测试
              </el-button>
              <el-button class="btn-default" size="small" @click="handleSetDefault(row)" :disabled="row.is_default">
                <i class="fa-solid fa-star"></i> 默认
              </el-button>
              <el-button class="btn-edit" size="small" @click="handleEdit(row)">
                <i class="fa-solid fa-pencil"></i>
              </el-button>
              <el-button class="btn-delete" size="small" link @click="handleDelete(row)" :disabled="row.is_default">
                <i class="fa-solid fa-trash"></i>
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑存储' : '新增存储'"
      width="600px"
      @close="resetForm"
    >
      <el-form :model="form" :rules="rules" ref="formRef" label-width="100px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="存储后端名称" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-select v-model="form.type" placeholder="选择存储类型" style="width: 100%">
            <el-option v-for="opt in typeOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="描述（可选）" />
        </el-form-item>

        <template v-if="form.type === 'local' && form.config?.local">
          <el-divider>本地存储配置</el-divider>
          <el-form-item label="存储路径" prop="config.local.base_path">
            <el-input v-model="form.config.local.base_path" placeholder="./data/packages" />
          </el-form-item>
          <el-form-item label="最大容量(GB)" prop="config.local.max_size_gb">
            <el-input-number v-model="form.config.local.max_size_gb" :min="1" :max="10000" />
          </el-form-item>
        </template>

        <template v-if="form.type === 's3' && form.config?.s3">
          <el-divider>S3 存储配置</el-divider>
          <el-form-item label="Endpoint" prop="config.s3.endpoint">
            <el-input v-model="form.config.s3.endpoint" placeholder="https://s3.amazonaws.com" />
          </el-form-item>
          <el-form-item label="Region" prop="config.s3.region">
            <el-input v-model="form.config.s3.region" placeholder="us-east-1" />
          </el-form-item>
          <el-form-item label="Access Key" prop="config.s3.access_key_id">
            <el-input v-model="form.config.s3.access_key_id" placeholder="AK" />
          </el-form-item>
          <el-form-item label="Secret Key" prop="config.s3.secret_access_key">
            <el-input v-model="form.config.s3.secret_access_key" type="password" show-password placeholder="SK" />
          </el-form-item>
          <el-form-item label="Bucket" prop="config.s3.bucket">
            <el-input v-model="form.config.s3.bucket" placeholder="bucket name" />
          </el-form-item>
          <el-form-item label="基础路径">
            <el-input v-model="form.config.s3.base_path" placeholder="packages" />
          </el-form-item>
          <el-form-item label="最大容量(GB)">
            <el-input-number v-model="form.config.s3.max_size_gb" :min="1" :max="100000" />
          </el-form-item>
          <el-form-item label="使用 SSL">
            <el-switch v-model="form.config.s3.use_ssl" />
          </el-form-item>
        </template>

        <el-form-item label="启用">
          <el-switch v-model="form.is_active" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { storageBackendApi, type StorageBackend } from '@/api/storageBackend'
import type { FormInstance, FormRules } from 'element-plus'
import { useTableRowHover } from '@/composables/useTableRowHover'

const { tableRowClass, handleRowEnter, handleRowLeave } = useTableRowHover()

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<FormInstance>()
const storages = ref<StorageBackend[]>([])

function getTypeIcon(type: string): string {
  const icons: Record<string, string> = {
    local: 'fa-solid fa-folder',
    s3: 'fa-solid fa-cloud',
    obs: 'fa-solid fa-cloud',
  }
  return icons[type] || 'fa-solid fa-box'
}

const typeOptions = [
  { label: '本地存储', value: 'local' },
  { label: 'S3 / MinIO', value: 's3' },
]

const emptyConfig = {
  local: { base_path: './data/packages', max_size_gb: 100 },
  s3: { endpoint: '', region: '', access_key_id: '', secret_access_key: '', bucket: '', base_path: '', max_size_gb: 1000, use_ssl: true },
  obs: { endpoint: '', access_key_id: '', secret_access_key: '', bucket: '', base_path: '', max_size_gb: 1000 },
}

const form = reactive<Partial<StorageBackend>>({
  name: '',
  type: 'local',
  description: '',
  config: { ...emptyConfig },
  is_active: true,
  is_default: false,
})

const rules: FormRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择类型', trigger: 'change' }],
}

onMounted(loadStorages)

async function loadStorages() {
  loading.value = true
  try {
    const res = await storageBackendApi.list()
    storages.value = res || []
  } catch {
    ElMessage.error('加载存储列表失败')
  } finally {
    loading.value = false
  }
}

function handleAdd() {
  isEdit.value = false
  dialogVisible.value = true
}

function handleEdit(row: StorageBackend) {
  isEdit.value = true
  Object.assign(form, {
    id: row.id,
    name: row.name,
    type: row.type,
    description: row.description,
    config: row.config || { ...emptyConfig },
    is_active: row.is_active,
    is_default: row.is_default,
  })
  dialogVisible.value = true
}

async function handleSubmit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    if (isEdit.value && form.id) {
      await storageBackendApi.update(form.id, form)
      ElMessage.success('更新成功')
    } else {
      await storageBackendApi.create(form)
      ElMessage.success('创建成功')
    }
    dialogVisible.value = false
    await loadStorages()
  } catch (error: any) {
    ElMessage.error(error.message || '操作失败')
  } finally {
    saving.value = false
  }
}

async function handleDelete(row: StorageBackend) {
  try {
    await ElMessageBox.confirm(`确定要删除存储 "${row.name}" 吗？`, '删除确认', { type: 'warning' })
    await storageBackendApi.delete(row.id)
    ElMessage.success('删除成功')
    await loadStorages()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error(error.message || '删除失败')
    }
  }
}

async function handleSetDefault(row: StorageBackend) {
  try {
    await storageBackendApi.setDefault(row.id)
    ElMessage.success('已设为默认存储')
    await loadStorages()
  } catch (error: any) {
    ElMessage.error(error.message || '设置失败')
  }
}

async function handleTest(row: StorageBackend) {
  try {
    const res = await storageBackendApi.testConnection(row)
    const data = res as any
    if (data?.success) {
      ElMessage.success('连接测试成功')
    } else {
      ElMessage.error(data?.message || '连接测试失败')
    }
  } catch (error: any) {
    ElMessage.error(error.message || '连接测试失败')
  }
}

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    type: 'local',
    description: '',
    config: { ...emptyConfig },
    is_active: true,
    is_default: false,
  })
  formRef.value?.resetFields()
}

const typeLabelMap: Record<string, string> = { local: '本地', s3: 'S3', obs: 'OBS' }

function getTypeLabel(type: string) {
  return typeLabelMap[type] || type
}
</script>

<style scoped>
.storage-management {
  /* padding: 20px; */
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
  background: linear-gradient(135deg, #3b82f6 0%, #1d4ed8 100%);
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

.create-btn .el-icon {
  font-size: 16px;
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

:deep(.el-table .row-hovered) {
  background: #f8fafc;
}

.type-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 12px;
}

.type-tag--local {
  background: #f0fdf4;
  color: #16a34a;
}

.type-tag--s3 {
  background: #eff6ff;
  color: #2563eb;
}

.type-tag--obs {
  background: #fffbeb;
  color: #d97706;
}

.tag-icon {
  font-size: 12px;
}

.status-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 10px;
  border-radius: 20px;
  font-size: 12px;
}

.status-tag--active {
  background: #f0fdf4;
  color: #16a34a;
}

.status-tag--active .status-dot {
  background: #22c55e;
}

.status-tag--disabled {
  background: #f3f4f6;
  color: #6b7280;
}

.status-tag--disabled .status-dot {
  background: #9ca3af;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.no-default {
  color: #9ca3af;
  font-size: 13px;
}

.operation-buttons {
  display: flex;
  align-items: center;
  gap: 4px;
}

.btn-test {
  background: #f0f9ff;
  color: #0369a1;
  border-color: #bae6fd;
}

.btn-test:hover {
  background: #e0f2fe;
}

.btn-default {
  background: #fffbeb;
  color: #d97706;
  border-color: #fde68a;
}

.btn-default:hover:not(:disabled) {
  background: #fef3c7;
}

.btn-edit {
  color: #6b7280;
}

.btn-edit:hover {
  background: #f3f4f6;
}

.btn-delete {
  color: #ef4444;
}

.btn-delete:hover {
  background: #fef2f2;
}
</style>
