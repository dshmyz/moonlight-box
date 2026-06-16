<template>
  <div class="webhook-management">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-bell"></i>
        </div>
        <div class="header-text">
          <h2>Webhook 管理</h2>
          <p class="header-subtitle">管理系统 Webhook 回调</p>
        </div>
      </div>
      <el-button type="primary" class="create-btn" @click="showCreateDialog">
        <i class="fa-solid fa-plus"></i>
        <span>创建 Webhook</span>
      </el-button>
    </header>

    <div class="content-panel" v-loading="loading">
      <el-table
        :data="webhooks"
        style="width: 100%"
        :header-cell-style="{ background: '#fafbfc' }"
        :row-class-name="tableRowClass"
        @row-mouse-enter="handleRowEnter"
        @row-mouse-leave="handleRowLeave"
      >
        <el-table-column prop="id" label="ID" width="70" align="center" />
        <el-table-column prop="name" label="名称" min-width="150">
          <template #default="{ row }">
            <span class="webhook-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="url" label="URL" min-width="250">
          <template #default="{ row }">
            <el-tooltip :content="row.url" placement="top">
              <span class="url-text">{{ row.url }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="events" label="事件" min-width="200">
          <template #default="{ row }">
            <div class="event-tags">
              <el-tag
                v-for="event in (row.events || '').split(',').slice(0, 3)"
                :key="event"
                size="small"
                class="event-tag"
              >
                {{ event }}
              </el-tag>
              <el-tag v-if="(row.events || '').split(',').length > 3" size="small" type="info">
                +{{ (row.events || '').split(',').length - 3 }}
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :class="['status-tag', row.status === 'active' ? 'status-tag--active' : 'status-tag--disabled']" size="small">
              {{ row.status === 'active' ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="170" align="center">
          <template #default="{ row }">
            <span class="time-text">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" align="center">
          <template #default="{ row }">
            <div class="operation-buttons">
              <el-button class="btn-edit" size="small" @click="showEditDialog(row)">
                <i class="fa-solid fa-pen"></i>
              </el-button>
              <el-button class="btn-test" size="small" @click="testWebhook(row)" :loading="row.testing">
                <i class="fa-solid fa-paper-plane"></i> 测试
              </el-button>
              <el-button class="btn-history" size="small" @click="showDeliveryDialog(row)">
                <i class="fa-solid fa-clock-rotate-left"></i>
              </el-button>
              <el-button class="btn-delete" size="small" type="text" @click="handleDelete(row)">
                <i class="fa-solid fa-trash"></i>
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑 Webhook' : '创建 Webhook'"
      width="600px"
      class="webhook-dialog"
    >
      <el-form :model="formData" :rules="formRules" ref="formRef" label-width="80px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="formData.name" placeholder="请输入 Webhook 名称" />
        </el-form-item>
        <el-form-item label="URL" prop="url">
          <el-input v-model="formData.url" placeholder="请输入 Webhook URL" />
        </el-form-item>
        <el-form-item label="密钥" prop="secret">
          <el-input
            v-model="formData.secret"
            type="password"
            show-password
            placeholder="请输入密钥（可选）"
          />
        </el-form-item>
        <el-form-item label="事件" prop="events">
          <el-checkbox-group v-model="formData.events" class="event-checkbox-group">
            <el-checkbox value="package.created">包创建</el-checkbox>
            <el-checkbox value="package.updated">包更新</el-checkbox>
            <el-checkbox value="package.deleted">包删除</el-checkbox>
            <el-checkbox value="repository.created">仓库创建</el-checkbox>
            <el-checkbox value="repository.updated">仓库更新</el-checkbox>
            <el-checkbox value="repository.deleted">仓库删除</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item label="状态" prop="enabled">
          <el-switch v-model="formData.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deliveryVisible" title="交付历史" width="800px" class="delivery-dialog">
      <div class="delivery-header">
        <i class="fa-solid fa-clock-rotate-left"></i>
        <span>Webhook 交付记录</span>
      </div>
      <el-table :data="deliveries" v-loading="deliveryLoading" style="width: 100%">
        <el-table-column prop="id" label="ID" width="80" align="center" />
        <el-table-column prop="event" label="事件" width="150">
          <template #default="{ row }">
            <span class="event-name">{{ row.event }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="response_code" label="HTTP 状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :class="['code-tag', row.response_code >= 200 && row.response_code < 300 ? 'code-tag--success' : 'code-tag--error']" size="small">
              {{ row.response_code }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="duration" label="响应时间" width="120" align="center">
          <template #default="{ row }">
            <span class="duration-text">{{ row.duration }} ms</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="交付时间" min-width="180" align="center">
          <template #default="{ row }">
            <span class="time-text">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { webhookApi, type Webhook, type WebhookDelivery } from '@/api/webhook'

const loading = ref(false)
const submitting = ref(false)
const deliveryLoading = ref(false)
const webhooks = ref<(Webhook & { testing?: boolean })[]>([])
const deliveries = ref<WebhookDelivery[]>([])
const hoveredRow = ref<number | null>(null)

const dialogVisible = ref(false)
const deliveryVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<FormInstance>()
const currentWebhook = ref<Webhook | null>(null)

const formData = ref({
  name: '',
  url: '',
  secret: '',
  events: [] as string[],
  enabled: true,
})

const formRules: FormRules = {
  name: [
    { required: true, message: '请输入 Webhook 名称', trigger: 'blur' },
    { min: 2, max: 100, message: '长度在 2 到 100 个字符', trigger: 'blur' },
  ],
  url: [
    { required: true, message: '请输入 Webhook URL', trigger: 'blur' },
    { type: 'url', message: '请输入有效的 URL', trigger: 'blur' },
  ],
  events: [
    { required: true, message: '请至少选择一个事件', trigger: 'change', type: 'array', min: 1 },
  ],
}

function tableRowClass({ rowIndex }: { rowIndex: number }) {
  return rowIndex === hoveredRow.value ? 'row-hovered' : ''
}

function handleRowEnter({ rowIndex }: { rowIndex: number }) {
  hoveredRow.value = rowIndex
}

function handleRowLeave() {
  hoveredRow.value = null
}

const formatDate = (date: string): string => {
  if (!date || date === '') return '-'
  const d = new Date(date)
  if (isNaN(d.getTime())) return '-'
  return d.toLocaleString('zh-CN')
}

const loadWebhooks = async () => {
  loading.value = true
  try {
    const res = await webhookApi.list()
    const data = res as any
    webhooks.value = data?.items || []
  } catch {
    ElMessage.error('加载 Webhook 列表失败')
  } finally {
    loading.value = false
  }
}

const showCreateDialog = () => {
  isEdit.value = false
  currentWebhook.value = null
  formData.value = {
    name: '',
    url: '',
    secret: '',
    events: [],
    enabled: true,
  }
  dialogVisible.value = true
}

const showEditDialog = (webhook: Webhook) => {
  isEdit.value = true
  currentWebhook.value = webhook
  formData.value = {
    name: webhook.name,
    url: webhook.url,
    secret: '',
    events: (webhook.events || '').split(',').filter(e => e.trim()),
    enabled: webhook.status === 'active',
  }
  dialogVisible.value = true
}

const submitForm = async () => {
  if (!formRef.value) return

  formRef.value.validate((valid) => {
    if (!valid) return

    submitWebhook()
  })
}

const submitWebhook = async () => {
  submitting.value = true
  try {
    if (isEdit.value && currentWebhook.value) {
      await webhookApi.update(currentWebhook.value.id, formData.value)
      ElMessage.success('Webhook 更新成功')
    } else {
      await webhookApi.create(formData.value)
      ElMessage.success('Webhook 创建成功')
    }
    dialogVisible.value = false
    await loadWebhooks()
  } catch {
    ElMessage.error(isEdit.value ? '更新 Webhook 失败' : '创建 Webhook 失败')
  } finally {
    submitting.value = false
  }
}

const testWebhook = async (webhook: Webhook & { testing?: boolean }) => {
  webhook.testing = true
  try {
    const res = await webhookApi.test(webhook.id)
    const data = res as any
    if (data?.success) {
      ElMessage.success('Webhook 测试成功')
    } else {
      ElMessage.error(data?.message || 'Webhook 测试失败')
    }
  } catch {
    ElMessage.error('Webhook 测试失败')
  } finally {
    webhook.testing = false
  }
}

const showDeliveryDialog = async (webhook: Webhook) => {
  deliveryVisible.value = true
  deliveryLoading.value = true
  try {
    const res = await webhookApi.getDeliveries(webhook.id)
    const data = res as any
    deliveries.value = data?.items || []
  } catch {
    ElMessage.error('加载交付历史失败')
  } finally {
    deliveryLoading.value = false
  }
}

const handleDelete = async (webhook: Webhook) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除 Webhook "${webhook.name}" 吗？`,
      '警告',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )

    await webhookApi.delete(webhook.id)
    ElMessage.success('Webhook 删除成功')
    await loadWebhooks()
  } catch (err: unknown) {
    if (err !== 'cancel' && err !== 'Error: cancel') {
      ElMessage.error('删除 Webhook 失败')
    }
  }
}

onMounted(loadWebhooks)
</script>

<style scoped>
.webhook-management {
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
  background: linear-gradient(135deg, #14b8a6 0%, #0d9488 100%);
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

:deep(.el-table .row-hovered) {
  background: #f8fafc;
}

.webhook-name {
  font-weight: 500;
  color: #1f2937;
}

.url-text {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-family-mono);
  font-size: 13px;
  color: #6b7280;
}

.event-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.event-tag {
  background: #f0f9ff;
  color: #0369a1;
  border-color: #bae6fd;
}

.status-tag {
  border: none;
  font-weight: 500;
}

.status-tag--active {
  background: #ecfdf5;
  color: #059669;
}

.status-tag--disabled {
  background: #fef2f2;
  color: #dc2626;
}

.time-text {
  font-size: 13px;
  color: #6b7280;
}

.operation-buttons {
  display: flex;
  align-items: center;
  gap: 4px;
}

.btn-edit {
  background: #f3f4f6;
  color: #374151;
  border-color: #e5e7eb;
}

.btn-edit:hover {
  background: #e5e7eb;
}

.btn-test {
  background: #eff6ff;
  color: #2563eb;
  border-color: #bfdbfe;
}

.btn-test:hover {
  background: #dbeafe;
}

.btn-history {
  background: #fff7ed;
  color: #c2410c;
  border-color: #fed7aa;
}

.btn-history:hover {
  background: #ffedd5;
}

.btn-delete {
  color: #ef4444;
}

.btn-delete:hover {
  background: #fef2f2;
}

.delivery-header {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid #f3f4f6;
  display: flex;
  align-items: center;
  gap: 8px;
  color: #6b7280;
}

.delivery-header i {
  color: #14b8a6;
}

.event-name {
  font-weight: 500;
  color: #374151;
}

.code-tag {
  border: none;
  font-weight: 500;
}

.code-tag--success {
  background: #ecfdf5;
  color: #059669;
}

.code-tag--error {
  background: #fef2f2;
  color: #dc2626;
}

.duration-text {
  font-weight: 500;
  color: #6b7280;
}

:deep(.event-checkbox-group) {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
</style>
