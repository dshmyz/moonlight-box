<template>
  <div class="audit-logs">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-clipboard-list"></i>
        </div>
        <div class="header-text">
          <h2>审计日志</h2>
          <p class="header-subtitle">查看系统操作记录</p>
        </div>
      </div>
      <el-button class="refresh-btn" @click="loadLogs">
        <i class="fa-solid fa-rotate"></i>
        <span>刷新</span>
      </el-button>
    </header>

    <div class="toolbar">
      <el-select
        v-model="filterAction"
        placeholder="操作类型"
        clearable
        class="filter-select"
        @change="loadLogs"
      >
        <el-option v-for="opt in actionOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>

      <div class="search-wrapper">
        <i class="fa-solid fa-search search-icon-prefix"></i>
        <el-input
          v-model="filterIP"
          placeholder="IP 地址"
          clearable
          class="search-input"
          @keyup.enter="loadLogs"
          @clear="loadLogs"
        />
      </div>

      <el-button type="primary" class="search-btn" @click="loadLogs">
        <i class="fa-solid fa-search"></i> 搜索
      </el-button>
    </div>

    <div class="content-panel" v-loading="loading">
      <el-table
        :data="logs"
        style="width: 100%"
        :header-cell-style="{ background: '#fafbfc' }"
        :row-class-name="tableRowClass"
        @row-mouse-enter="handleRowEnter"
        @row-mouse-leave="handleRowLeave"
      >
        <el-table-column prop="id" label="ID" width="70" align="center" />
        <el-table-column prop="action" label="操作" width="100" align="center">
          <template #default="{ row }">
            <el-tag :class="['action-tag', `action-tag--${row.action}`]" size="small">{{ actionLabel(row.action) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="resource_type" label="资源类型" width="110" align="center">
          <template #default="{ row }">
            <span class="resource-type">{{ row.resource_type }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="resource_name" label="资源名称" min-width="160" show-overflow-tooltip />
        <el-table-column prop="ip_address" label="IP 地址" width="140" align="center">
          <template #default="{ row }">
            <span class="ip-text">{{ row.ip_address || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="response_status" label="状态码" width="80" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.response_status" :class="['status-tag', row.response_status < 400 ? 'status-tag--success' : 'status-tag--error']" size="small">
              {{ row.response_status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="user_agent" label="User Agent" min-width="180" show-overflow-tooltip />
        <el-table-column prop="details" label="详情" min-width="180" show-overflow-tooltip />
        <el-table-column prop="created_at" label="时间" width="170" align="center">
          <template #default="{ row }">
            <span class="time-text">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>
      </el-table>

      <div class="list-footer" v-if="total > 0">
        <div class="footer-info">
          <span class="total-badge">{{ total }}</span>
          <span class="total-label">条记录</span>
        </div>
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="sizes, prev, pager, next"
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { auditApi } from '@/api/audit'
import { formatDate } from '@/utils/format'
import { useTableRowHover } from '@/composables/useTableRowHover'

const { tableRowClass, handleRowEnter, handleRowLeave } = useTableRowHover()

const loading = ref(false)
const logs = ref<any[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filterAction = ref('')
const filterIP = ref('')

const actionOptions = [
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

function actionLabel(action: string): string {
  const map: Record<string, string> = {
    login: '登录',
    logout: '登出',
    package_upload: '上传',
    package_download: '下载',
    package_delete: '删除',
    block: '阻断',
    user_create: '创建用户',
    user_update: '更新用户',
    config_change: '配置变更',
  }
  return map[action] || action
}

async function loadLogs() {
  loading.value = true
  try {
    const params: Record<string, any> = { page: page.value, page_size: pageSize.value }
    if (filterAction.value) params.action = filterAction.value
    if (filterIP.value) params.ip_address = filterIP.value

    const res = await auditApi.list(params)
    logs.value = res?.items || []
    total.value = res?.pagination?.total || 0
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

function handlePageChange(p: number) {
  page.value = p
  loadLogs()
}

function handleSizeChange() {
  page.value = 1
  loadLogs()
}

onMounted(loadLogs)
</script>

<style scoped>
.audit-logs {
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
  background: linear-gradient(135deg, #f59e0b 0%, #d97706 100%);
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

.refresh-btn {
  height: 40px;
  padding: 0 20px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: #fff;
  border-color: #e5e7eb;
  color: #374151;
  transition: all 0.2s ease;
}

.refresh-btn:hover {
  background: #f9fafb;
  border-color: #d1d5db;
  color: #1f2937;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.filter-select {
  width: 160px;
}

.filter-select :deep(.el-input__wrapper) {
  border-radius: 10px;
  box-shadow: none;
  border: 1px solid #e5e7eb;
  padding: 0 12px;
  height: 38px;
  font-size: 14px;
  background: #fff;
  transition: all 0.2s ease;
}

.filter-select :deep(.el-input__wrapper:hover) {
  border-color: #d1d5db;
}

.filter-select :deep(.el-input__wrapper.is-focus) {
  border-color: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}

.search-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.search-icon-prefix {
  position: absolute;
  left: 14px;
  color: #9ca3af;
  font-size: 14px;
  z-index: 1;
}

.search-input {
  width: 180px;
}

.search-input :deep(.el-input__wrapper) {
  border-radius: 10px;
  box-shadow: none;
  border: 1px solid #e5e7eb;
  padding: 0 14px 0 38px;
  height: 38px;
  transition: all 0.2s ease;
  background: #fff;
}

.search-input :deep(.el-input__wrapper:hover) {
  border-color: #d1d5db;
}

.search-input :deep(.el-input__wrapper.is-focus) {
  border-color: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}

.search-input :deep(.el-input__inner) {
  height: 36px;
  font-size: 14px;
  color: #374151;
}

.search-btn {
  height: 38px;
  padding: 0 16px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border-color: transparent;
  transition: all 0.2s ease;
}

.search-btn:hover {
  background: linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%);
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

.action-tag {
  border: none;
  font-weight: 500;
}

.action-tag--login { background: #ecfdf5; color: #059669; }
.action-tag--logout { background: #f3f4f6; color: #6b7280; }
.action-tag--package_upload { background: #eff6ff; color: #2563eb; }
.action-tag--package_download { background: #ecfdf5; color: #059669; }
.action-tag--package_delete { background: #fef2f2; color: #dc2626; }
.action-tag--block { background: #fffbeb; color: #d97706; }
.action-tag--user_create { background: #eff6ff; color: #2563eb; }
.action-tag--user_update { background: #fffbeb; color: #d97706; }
.action-tag--config_change { background: #f3f4f6; color: #6b7280; }

.resource-type {
  font-weight: 500;
  color: #374151;
}

.ip-text {
  font-family: var(--font-family-mono);
  font-size: 13px;
  color: #6b7280;
}

.status-tag {
  border: none;
}

.status-tag--success {
  background: #ecfdf5;
  color: #059669;
}

.status-tag--error {
  background: #fef2f2;
  color: #dc2626;
}

.time-text {
  font-size: 13px;
  color: #6b7280;
}

.list-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-top: 1px solid #f3f4f6;
  background: #fafafa;
  border-radius: 0 0 16px 16px;
}

.footer-info {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.total-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 24px;
  padding: 0 10px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  border-radius: 6px;
}

.total-label {
  font-size: 13px;
  color: #6b7280;
}

.list-footer :deep(.el-pagination) {
  font-size: 13px;
}

.list-footer :deep(.el-pagination button) {
  border-radius: 6px;
}

.list-footer :deep(.el-pager li) {
  border-radius: 6px;
  min-width: 30px;
}
</style>
