<template>
  <div class="audit-logs">
    <div class="page-header">
      <h2>审计日志</h2>
      <CustomButton type="secondary" :icon="Refresh" @click="loadLogs">刷新</CustomButton>
    </div>

    <div class="filter-bar">
      <CustomSelect
        v-model="filterAction"
        :options="actionOptions"
        placeholder="操作类型"
        style="width: 160px"
        @change="loadLogs"
      />
      <CustomInput
        v-model="filterIP"
        placeholder="IP 地址"
        clearable
        style="width: 160px"
        @enter="loadLogs"
        @clear="loadLogs"
      />
      <el-date-picker
        v-model="dateRange"
        type="daterange"
        range-separator="至"
        start-placeholder="开始日期"
        end-placeholder="结束日期"
        style="width: 240px"
        @change="loadLogs"
      />
      <CustomButton @click="loadLogs">搜索</CustomButton>
    </div>

    <CustomTable
      :columns="columns"
      :data="logs"
      :loading="loading"
      row-key="id"
    >
      <template #action="{ row }">
        <CustomTag :type="actionTagType(row.action)" size="small">{{ actionLabel(row.action) }}</CustomTag>
      </template>
      <template #response_status="{ row }">
        <CustomTag v-if="row.response_status" :type="row.response_status < 400 ? 'success' : 'danger'" size="small">
          {{ row.response_status }}
        </CustomTag>
      </template>
      <template #created_at="{ row }">
        {{ formatDate(row.created_at) }}
      </template>
    </CustomTable>

    <el-pagination
      v-if="total > pageSize"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      layout="total, prev, pager, next"
      class="pagination"
      @current-change="handlePageChange"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import request from '@/api/request'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomSelect from '@/components/ui/CustomSelect.vue'
import CustomInput from '@/components/ui/CustomInput.vue'
import CustomTable from '@/components/ui/CustomTable.vue'
import CustomTag from '@/components/ui/CustomTag.vue'

const loading = ref(false)
const logs = ref<any[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filterAction = ref('')
const filterIP = ref('')
const dateRange = ref<[string, string] | null>(null)

const actionOptions = [
  { label: '全部', value: '' },
  { label: '登录', value: 'login' },
  { label: '登出', value: 'logout' },
  { label: '上传', value: 'package_upload' },
  { label: '下载', value: 'package_download' },
  { label: '删除', value: 'package_delete' },
  { label: '阻断', value: 'block' },
  { label: '创建用户', value: 'user_create' },
  { label: '更新用户', value: 'user_update' },
  { label: '配置变更', value: 'config_change' },
]

const columns = [
  { prop: 'id', label: 'ID', width: '60px' },
  { prop: 'action', label: '操作', width: '120px' },
  { prop: 'resource_type', label: '资源类型', width: '110px' },
  { prop: 'resource_name', label: '资源名称', width: '160px' },
  { prop: 'ip_address', label: 'IP 地址', width: '140px' },
  { prop: 'response_status', label: '状态码', width: '80px' },
  { prop: 'user_agent', label: 'User Agent', width: '180px' },
  { prop: 'details', label: '详情', width: '180px' },
  { prop: 'created_at', label: '时间', width: '180px' },
]

function formatDate(d: string): string {
  if (!d) return '-'
  return new Date(d).toLocaleString('zh-CN')
}

function actionLabel(action: string): string {
  const map: Record<string, string> = {
    login: '登录', logout: '登出', package_upload: '上传',
    package_download: '下载', package_delete: '删除', block: '阻断',
    user_create: '创建用户', user_update: '更新用户', config_change: '配置变更',
  }
  return map[action] || action
}

function actionTagType(action: string): 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info' {
  const map: Record<string, 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'> = {
    login: 'success', logout: 'info', package_upload: 'primary',
    package_download: 'default', package_delete: 'danger', block: 'danger',
  }
  return map[action] || 'info'
}

async function loadLogs() {
  loading.value = true
  try {
    const params: Record<string, any> = { page: page.value, page_size: pageSize.value }
    if (filterAction.value) params.action = filterAction.value
    if (filterIP.value) params.ip_address = filterIP.value

    const res = await request.get('/audit/logs', { params })
    const data = res as any
    logs.value = data?.items || []
    total.value = data?.pagination?.total || 0
  } catch (e: any) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

function handlePageChange(p: number) {
  page.value = p
  loadLogs()
}

onMounted(() => {
  loadLogs()
})
</script>

<style scoped>
.audit-logs {
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

.filter-bar {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
  align-items: center;
}

.pagination {
  margin-top: var(--spacing-lg);
  display: flex;
  justify-content: flex-end;
}

.audit-logs :deep(.custom-table td) {
  max-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
