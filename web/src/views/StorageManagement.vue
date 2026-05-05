<template>
  <div class="storage-management">
    <div class="page-header">
      <h2>存储管理</h2>
      <CustomButton type="primary" :icon="PlusIcon" @click="handleAdd">
        新增存储
      </CustomButton>
    </div>

    <CustomTable :columns="columns" :data="storages" :loading="loading" row-key="id" striped>
      <template #type="{ row }">
        <CustomTag :type="getTypeTag(row.type)" size="small">
          {{ getTypeLabel(row.type) }}
        </CustomTag>
      </template>
      <template #is_active="{ row }">
        <CustomTag :type="row.is_active ? 'success' : 'info'" size="small">
          {{ row.is_active ? '启用' : '禁用' }}
        </CustomTag>
      </template>
      <template #is_default="{ row }">
        <CustomTag v-if="row.is_default" type="warning" size="small">默认</CustomTag>
        <span v-else class="text-placeholder">-</span>
      </template>
      <template #actions="{ row }">
        <div class="action-buttons">
          <CustomButton type="text" size="small" @click="handleTest(row)">测试连接</CustomButton>
          <CustomButton type="text" size="small" @click="handleSetDefault(row)" :disabled="row.is_default">设为默认</CustomButton>
          <CustomButton type="text" size="small" @click="handleEdit(row)">编辑</CustomButton>
          <CustomButton type="text" size="small" @click="handleDelete(row)" :disabled="row.is_default">删除</CustomButton>
        </div>
      </template>
    </CustomTable>

    <!-- 新增/编辑对话框 -->
    <CustomDialog v-model="dialogVisible" :title="isEdit ? '编辑存储' : '新增存储'" width="600px">
      <el-form :model="form" :rules="rules" ref="formRef" label-width="100px">
        <el-form-item label="名称" prop="name">
          <CustomInput v-model="form.name" placeholder="存储后端名称" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <CustomSelect v-model="form.type" :options="storageTypeOptions" placeholder="选择存储类型" />
        </el-form-item>
        <el-form-item label="描述">
          <CustomInput v-model="form.description" placeholder="描述（可选）" />
        </el-form-item>

        <!-- 本地存储配置 -->
        <template v-if="form.type === 'local'">
          <el-divider>本地存储配置</el-divider>
          <el-form-item label="存储路径" prop="config.local.base_path">
            <CustomInput v-model="form.config.local.base_path" placeholder="./data/packages" />
          </el-form-item>
          <el-form-item label="最大容量(GB)" prop="config.local.max_size_gb">
            <el-input-number v-model="form.config.local.max_size_gb" :min="1" :max="10000" />
          </el-form-item>
        </template>

        <!-- S3 配置 -->
        <template v-if="form.type === 's3'">
          <el-divider>S3 存储配置</el-divider>
          <el-form-item label="Endpoint" prop="config.s3.endpoint">
            <CustomInput v-model="form.config.s3.endpoint" placeholder="https://s3.amazonaws.com" />
          </el-form-item>
          <el-form-item label="Region" prop="config.s3.region">
            <CustomInput v-model="form.config.s3.region" placeholder="us-east-1" />
          </el-form-item>
          <el-form-item label="Access Key" prop="config.s3.access_key_id">
            <CustomInput v-model="form.config.s3.access_key_id" placeholder="AK" />
          </el-form-item>
          <el-form-item label="Secret Key" prop="config.s3.secret_access_key">
            <CustomInput v-model="form.config.s3.secret_access_key" type="password" placeholder="SK" />
          </el-form-item>
          <el-form-item label="Bucket" prop="config.s3.bucket">
            <CustomInput v-model="form.config.s3.bucket" placeholder="bucket name" />
          </el-form-item>
          <el-form-item label="基础路径">
            <CustomInput v-model="form.config.s3.base_path" placeholder="packages" />
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
        <CustomButton type="secondary" @click="dialogVisible = false">取消</CustomButton>
        <CustomButton type="primary" @click="handleSubmit" :loading="saving">保存</CustomButton>
      </template>
    </CustomDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { storageBackendApi, type StorageBackend } from '@/api/storageBackend'
import type { FormInstance, FormRules } from 'element-plus'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomTable from '@/components/ui/CustomTable.vue'
import CustomTag from '@/components/ui/CustomTag.vue'
import CustomDialog from '@/components/ui/CustomDialog.vue'
import CustomInput from '@/components/ui/CustomInput.vue'
import CustomSelect from '@/components/ui/CustomSelect.vue'

const PlusIcon = Plus

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<FormInstance>()
const storages = ref<StorageBackend[]>([])

// 表格列定义
const columns = [
  { prop: 'name', label: '名称', width: '120px' },
  { prop: 'type', label: '类型', width: '100px' },
  { prop: 'description', label: '描述', width: '200px' },
  { prop: 'is_active', label: '状态', width: '100px' },
  { prop: 'is_default', label: '默认', width: '80px', align: 'center' as const },
  { prop: 'actions', label: '操作', width: '260px' },
]

// 存储类型选项
const storageTypeOptions = [
  { label: '本地存储', value: 'local' },
  { label: 'S3 / MinIO', value: 's3' },
  { label: '华为云 OBS', value: 'obs' },
]

const emptyConfig = {
  local: { base_path: './data/packages', max_size_gb: 100 },
  s3: { endpoint: '', region: '', access_key_id: '', secret_access_key: '', bucket: '', base_path: '', max_size_gb: 1000, use_ssl: true },
  obs: { endpoint: '', access_key_id: '', secret_access_key: '', bucket: '', base_path: '', max_size_gb: 1000 },
}

const form = reactive({
  name: '',
  type: 'local' as string,
  description: '',
  config: { ...emptyConfig } as any,
  is_active: true,
  is_default: false,
})

const rules: FormRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择类型', trigger: 'change' }],
}

// 对话框关闭时重置表单
watch(dialogVisible, (val) => {
  if (!val) {
    resetForm()
  }
})

onMounted(loadStorages)

async function loadStorages() {
  loading.value = true
  try {
    const res = await storageBackendApi.list()
    storages.value = (res as any) || []
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
    if (isEdit.value && (form as any).id) {
      await storageBackendApi.update((form as any).id, form)
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

const typeTagMap: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'> = { local: 'default', s3: 'success', obs: 'warning' }
const typeLabelMap: Record<string, string> = { local: '本地', s3: 'S3', obs: 'OBS' }

function getTypeTag(type: string): 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info' {
  return typeTagMap[type] || 'info'
}

function getTypeLabel(type: string) {
  return typeLabelMap[type] || type
}
</script>

<style scoped>
.storage-management {
  padding: var(--spacing-xl);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
}

.page-header h2 {
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
  margin: 0;
}

.action-buttons {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.text-placeholder {
  color: var(--color-text-tertiary);
}
</style>
