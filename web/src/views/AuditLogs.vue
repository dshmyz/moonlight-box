<template>
  <div class="audit-logs">
    <div class="page-header">
      <h2>审计日志</h2>
      <el-button @click="loadLogs">
        <el-icon><Refresh /></el-icon> 刷新
      </el-button>
    </div>

    <div class="filter-bar">
      <el-select v-model="filterAction" placeholder="操作类型" clearable style="width: 160px" @change="loadLogs">
        <el-option label="登录" value="login" />
        <el-option label="登出" value="logout" />
        <el-option label="上传" value="package_upload" />
        <el-option label="下载" value="package_download" />
        <el-option label="删除" value="package_delete" />
        <el-option label="阻断" value="block" />
        <el-option label="创建用户" value="user_create" />
        <el-option label="更新用户" value="user_update" />
        <el-option label="配置变更" value="config_change" />
      </el-select>
      <el-input v-model="filterIP" placeholder="IP 地址" clearable style="width: 160px" @keyup.enter="loadLogs" @clear="loadLogs" />
      <el-date-picker v-model="dateRange" type="daterange" range-separator="至" start-placeholder="开始日期" end-placeholder="结束日期" style="width: 240px" @change="loadLogs" />
      <el-button type="primary" @click="loadLogs">搜索</el-button>
    </div>

    <el-table :data="logs" v-loading="loading" style="width: 100%">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="action" label="操作" width="120">
        <template #default="{ row }">
          <el-tag :type="actionTagType(row.action)" size="small">{{ actionLabel(row.action) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="resource_type" label="资源类型" width="110" />
      <el-table-column prop="resource_name" label="资源名称" min-width="160" show-overflow-tooltip />
      <el-table-column prop="ip_address" label="IP 地址" width="140" />
      <el-table-column prop="response_status" label="状态码" width="80">
        <template #default="{ row }">
          <el-tag v-if="row.response_status" :type="row.response_status < 400 ? 'success' : 'danger'" size="small">
            {{ row.response_status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="user_agent" label="User Agent" min-width="180" show-overflow-tooltip />
      <el-table-column prop="details" label="详情" min-width="180" show-overflow-tooltip />
      <el-table-column prop="created_at" label="时间" width="180">
        <template #default="{ row }">
          {{ formatDate(row.created_at) }}
        </template>
      </el-table-column>
    </el-table>

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

const loading = ref(false)
const logs = ref<any[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filterAction = ref('')
const filterIP = ref('')
const dateRange = ref<[string, string] | null>(null)

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

function actionTagType(action: string): string {
  const map: Record<string, string> = {
    login: 'success', logout: 'info', package_upload: 'primary',
    package_download: '', package_delete: 'danger', block: 'danger',
  }
  return map[action] || 'info'
}

async function loadLogs() {
  loading.value = true
  try {
    const params: Record<string, any> = { page: page.value, page_size: pageSize.value }
    if (filterAction.value) params.action = filterAction.value
    if (filterIP.value) params.ip_address = filterIP.value

    const res = await request.get('/api/v1/audit/logs', { params })
    if (res.data.code === 200) {
      logs.value = res.data.data.items || []
      total.value = res.data.data.pagination?.total || 0
    }
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

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}
</style>
