<template>
  <div class="storage-management">
    <div class="page-header">
      <h2>存储管理</h2>
      <el-button type="primary" @click="handleAdd">
        <el-icon><Plus /></el-icon>
        新增存储
      </el-button>
    </div>

    <el-table :data="storages" v-loading="loading" style="width: 100%" stripe>
      <el-table-column prop="name" label="名称" min-width="120" />
      <el-table-column prop="type" label="类型" width="100">
        <template #default="{ row }">
          <el-tag :type="getTypeTag(row.type)" effect="plain">
            {{ getTypeLabel(row.type) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="描述" min-width="200" show-overflow-tooltip />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.is_active ? 'success' : 'info'" size="small">
            {{ row.is_active ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="默认" width="80" align="center">
        <template #default="{ row }">
          <el-tag v-if="row.is_default" type="warning" size="small">默认</el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="260" fixed="right">
        <template #default="{ row }">
          <el-button size="small" text type="primary" @click="handleTest(row)">测试连接</el-button>
          <el-button size="small" text type="warning" @click="handleSetDefault(row)" :disabled="row.is_default">
            设为默认
          </el-button>
          <el-button size="small" text type="primary" @click="handleEdit(row)">编辑</el-button>
          <el-button size="small" text type="danger" @click="handleDelete(row)" :disabled="row.is_default">
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新增/编辑对话框 -->
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
            <el-option label="本地存储" value="local" />
            <el-option label="S3 / MinIO" value="s3" />
            <el-option label="华为云 OBS" value="obs" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" rows="2" placeholder="描述（可选）" />
        </el-form-item>

        <!-- 本地存储配置 -->
        <template v-if="form.type === 'local'">
          <el-divider>本地存储配置</el-divider>
          <el-form-item label="存储路径" prop="config.local.base_path">
            <el-input v-model="form.config.local.base_path" placeholder="./data/packages" />
          </el-form-item>
          <el-form-item label="最大容量(GB)" prop="config.local.max_size_gb">
            <el-input-number v-model="form.config.local.max_size_gb" :min="1" :max="10000" />
          </el-form-item>
        </template>

        <!-- S3 配置 -->
        <template v-if="form.type === 's3'">
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

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<FormInstance>()
const storages = ref<StorageBackend[]>([])

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

const typeTagMap: Record<string, string> = { local: '', s3: 'success', obs: 'warning' }
const typeLabelMap: Record<string, string> = { local: '本地', s3: 'S3', obs: 'OBS' }

function getTypeTag(type: string) {
  return typeTagMap[type] || 'info'
}

function getTypeLabel(type: string) {
  return typeLabelMap[type] || type
}
</script>

<style scoped>
.storage-management {
  padding: 24px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h2 {
  font-size: 20px;
  font-weight: 600;
  color: #303133;
  margin: 0;
}
</style>
