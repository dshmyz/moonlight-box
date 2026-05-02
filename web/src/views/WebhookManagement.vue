<template>
  <div class="webhook-management">
    <div class="page-header">
      <h2>Webhook 管理</h2>
      <el-button type="primary" @click="showCreateDialog">
        <el-icon><Plus /></el-icon> 创建 Webhook
      </el-button>
    </div>

    <el-table :data="webhooks" v-loading="loading" style="width: 100%">
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="name" label="名称" min-width="150" />
      <el-table-column prop="url" label="URL" min-width="250">
        <template #default="{ row }">
          <el-tooltip :content="row.url" placement="top">
            <span class="url-text">{{ row.url }}</span>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column prop="events" label="事件" min-width="200">
        <template #default="{ row }">
          <el-tag
            v-for="event in (row.events || '').split(',').slice(0, 3)"
            :key="event"
            size="small"
            style="margin-right: 4px; margin-bottom: 4px"
          >
            {{ event }}
          </el-tag>
          <el-tag v-if="(row.events || '').split(',').length > 3" size="small" type="info">
            +{{ (row.events || '').split(',').length - 3 }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'danger'" size="small">
            {{ row.status === 'active' ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="180">
        <template #default="{ row }">
          {{ formatDate(row.created_at) }}
        </template>
      </el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="showEditDialog(row)">编辑</el-button>
          <el-button size="small" type="success" @click="testWebhook(row)" :loading="row.testing">
            测试
          </el-button>
          <el-button size="small" type="info" @click="showDeliveryDialog(row)">
            历史
          </el-button>
          <el-button size="small" type="danger" @click="handleDelete(row)">
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 创建/编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑 Webhook' : '创建 Webhook'"
      width="600px"
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
          <el-checkbox-group v-model="formData.events">
            <el-checkbox label="package.created">包创建</el-checkbox>
            <el-checkbox label="package.updated">包更新</el-checkbox>
            <el-checkbox label="package.deleted">包删除</el-checkbox>
            <el-checkbox label="repository.created">仓库创建</el-checkbox>
            <el-checkbox label="repository.updated">仓库更新</el-checkbox>
            <el-checkbox label="repository.deleted">仓库删除</el-checkbox>
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

    <!-- 交付历史对话框 -->
    <el-dialog v-model="deliveryVisible" title="交付历史" width="800px">
      <el-table :data="deliveries" v-loading="deliveryLoading" style="width: 100%">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="event" label="事件" width="150" />
        <el-table-column prop="response_code" label="HTTP 状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.response_code >= 200 && row.response_code < 300 ? 'success' : 'danger'" size="small">
              {{ row.response_code }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="duration" label="响应时间" width="120">
          <template #default="{ row }">
            {{ row.duration }} ms
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="交付时间" min-width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { webhookApi, type Webhook, type WebhookDelivery } from '@/api/webhook'

const loading = ref(false)
const submitting = ref(false)
const deliveryLoading = ref(false)
const webhooks = ref<(Webhook & { testing?: boolean })[]>([])
const deliveries = ref<WebhookDelivery[]>([])

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

/** 格式化日期 */
const formatDate = (date: string): string => {
  if (!date) return '-'
  return new Date(date).toLocaleString('zh-CN')
}

/** 加载 Webhook 列表 */
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

/** 显示创建对话框 */
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

/** 显示编辑对话框 */
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

/** 提交表单 */
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

/** 测试 Webhook */
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

/** 显示交付历史对话框 */
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

/** 删除 Webhook */
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

.url-text {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.el-checkbox-group) {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
</style>
