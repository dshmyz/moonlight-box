<template>
  <div class="user-migration">
    <div class="step-content">
      <div v-if="step === 0" class="connection-step">
        <div class="form-header">
          <div class="form-icon">
            <i class="fa-solid fa-plug"></i>
          </div>
          <div class="form-title">
            <h3>连接 Nexus</h3>
            <p class="form-desc">输入 Nexus 服务器信息以获取用户和角色数据</p>
          </div>
        </div>

        <el-form :model="form" label-position="top" class="connection-form">
          <el-form-item label="服务器地址" class="form-item">
            <el-input v-model="form.url" placeholder="https://nexus.example.com" prefix-icon="Link" />
          </el-form-item>
          <el-form-item label="用户名" class="form-item">
            <el-input v-model="form.username" placeholder="请输入用户名" prefix-icon="User" />
          </el-form-item>
          <el-form-item label="密码" class="form-item">
            <el-input v-model="form.password" type="password" show-password placeholder="请输入密码" prefix-icon="Lock" />
          </el-form-item>
          <el-form-item class="form-actions">
            <el-button type="primary" :loading="loading" @click="fetchPreview" class="submit-btn">
              <i class="fa-solid fa-circle-check"></i>
              获取用户和角色
            </el-button>
          </el-form-item>
        </el-form>
      </div>

      <div v-if="step === 1" class="preview-step">
        <div class="preview-header">
          <div class="preview-summary">
            <div class="summary-item">
              <span class="summary-value">{{ users.length }}</span>
              <span class="summary-label">用户</span>
            </div>
            <div class="summary-divider" />
            <div class="summary-item">
              <span class="summary-value">{{ internalUsers.length }}</span>
              <span class="summary-label">可同步用户</span>
            </div>
            <div class="summary-divider" />
            <div class="summary-item">
              <span class="summary-value">{{ roles.length }}</span>
              <span class="summary-label">角色</span>
            </div>
            <div class="summary-divider" />
            <div class="summary-item">
              <span class="summary-value">{{ internalRoles.length }}</span>
              <span class="summary-label">可同步角色</span>
            </div>
          </div>
        </div>

        <div class="preview-body">
          <el-tabs v-model="activeTab" class="preview-tabs">
            <el-tab-pane label="用户" :name="'users'">
              <el-table :data="users" size="small" max-height="360" class="preview-table">
                <el-table-column prop="userId" label="用户名" min-width="120" />
                <el-table-column label="显示名" min-width="120">
                  <template #default="{ row }">
                    {{ row.firstName }} {{ row.lastName }}
                  </template>
                </el-table-column>
                <el-table-column prop="email" label="邮箱" min-width="160" />
                <el-table-column label="状态" width="80">
                  <template #default="{ row }">
                    <el-tag :type="row.status === 'active' ? 'success' : 'danger'" size="small">
                      {{ row.status === 'active' ? '活跃' : '禁用' }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="角色" min-width="120">
                  <template #default="{ row }">
                    <el-tag v-for="role in row.roles" :key="role" size="small" style="margin: 1px 2px;">
                      {{ role }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="来源" width="80">
                  <template #default="{ row }">
                    <el-tag v-if="row.external" type="warning" size="small">外部</el-tag>
                    <el-tag v-else type="success" size="small">本地</el-tag>
                  </template>
                </el-table-column>
              </el-table>
            </el-tab-pane>
            <el-tab-pane label="角色" :name="'roles'">
              <el-table :data="roles" size="small" max-height="360" class="preview-table">
                <el-table-column prop="id" label="角色ID" min-width="120" />
                <el-table-column prop="name" label="名称" min-width="120" />
                <el-table-column prop="description" label="描述" min-width="200" show-overflow-tooltip />
                <el-table-column label="权限数" width="80" align="center">
                  <template #default="{ row }">
                    {{ row.privileges?.length || 0 }}
                  </template>
                </el-table-column>
                <el-table-column label="来源" width="80">
                  <template #default="{ row }">
                    <el-tag v-if="row.external" type="warning" size="small">外部</el-tag>
                    <el-tag v-else type="success" size="small">本地</el-tag>
                  </template>
                </el-table-column>
              </el-table>
            </el-tab-pane>
          </el-tabs>
        </div>

        <div class="preview-actions">
          <div class="sync-info">
            <i class="fa-solid fa-info-circle"></i>
            <span>外部用户和角色将被自动跳过，仅同步本地用户和角色</span>
          </div>
          <div class="action-buttons">
            <el-button @click="step = 0">
              <i class="fa-solid fa-arrow-left"></i>
              返回
            </el-button>
            <el-button type="primary" :loading="syncing" @click="startSync">
              <i class="fa-solid fa-user-plus"></i>
              同步用户和角色（{{ internalUsers.length }} 用户 / {{ internalRoles.length }} 角色）
            </el-button>
          </div>
        </div>
      </div>

      <div v-if="step === 2" class="progress-step">
        <el-card class="progress-card">
          <template #header>
            <div class="progress-header">
              <i class="fa-solid fa-spinner"></i>
              <span>用户迁移进度</span>
            </div>
          </template>
          <div class="progress-body">
            <div class="status-bar">
              <el-tag :type="syncStatusType" size="large">{{ syncStatusText }}</el-tag>
            </div>
            <div v-if="syncLogs.length" class="sync-logs">
              <h4 class="logs-title">
                <i class="fa-solid fa-file-text"></i>
                同步日志
              </h4>
              <ul class="logs-list">
                <li v-for="(log, i) in syncLogs" :key="i" class="log-item">{{ log }}</li>
              </ul>
            </div>
          </div>
        </el-card>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { listNexusUsers, listNexusRoles, syncUsersFromNexus, getMigrationStatus, type NexusUser, type NexusRole } from '@/api/migration'

const step = ref(0)
const loading = ref(false)
const syncing = ref(false)

const form = ref({
  url: '',
  username: '',
  password: '',
})

const users = ref<NexusUser[]>([])
const roles = ref<NexusRole[]>([])
const activeTab = ref('users')

const internalUsers = computed(() => users.value.filter(u => !u.external))
const internalRoles = computed(() => roles.value.filter(r => !r.external))

const currentTaskId = ref(0)
const syncStatus = ref('')
const syncLogs = ref<string[]>([])
const pollingTimer = ref<number | null>(null)

const syncStatusType = computed(() => {
  const map: Record<string, string> = {
    completed: 'success',
    failed: 'danger',
    cancelled: 'warning',
    running: '',
    pending: 'info',
  }
  return map[syncStatus.value] || 'info'
})

const syncStatusText = computed(() => {
  const map: Record<string, string> = {
    completed: '同步完成',
    failed: '同步失败',
    cancelled: '已取消',
    running: '同步中...',
    pending: '等待执行',
  }
  return map[syncStatus.value] || syncStatus.value
})

async function fetchPreview() {
  if (!form.value.url) {
    ElMessage.warning('请输入服务器地址')
    return
  }

  loading.value = true
  try {
    const [userRes, roleRes] = await Promise.all([
      listNexusUsers(form.value),
      listNexusRoles(form.value),
    ])
    users.value = Array.isArray(userRes) ? userRes : []
    roles.value = Array.isArray(roleRes) ? roleRes : []
    step.value = 1
    ElMessage.success('获取用户和角色成功')
  } catch (e: any) {
    ElMessage.error('获取数据失败: ' + (e.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

async function startSync() {
  syncing.value = true
  try {
    const res = (await syncUsersFromNexus(form.value)) as any
    currentTaskId.value = res?.id || 0
    step.value = 2
    syncStatus.value = 'running'
    startPolling()
  } catch (e: any) {
    ElMessage.error('创建同步任务失败: ' + (e.message || '未知错误'))
  } finally {
    syncing.value = false
  }
}

function startPolling() {
  pollingTimer.value = window.setInterval(async () => {
    if (!currentTaskId.value) return
    try {
      const res = (await getMigrationStatus(currentTaskId.value)) as any
      syncStatus.value = res?.task?.status ?? 'unknown'
      if (res?.logs) {
        syncLogs.value = res.logs
      }
      if (res?.task?.status !== 'running' && res?.task?.status !== 'pending') {
        stopPolling()
      }
    } catch {
      // ignore polling errors
    }
  }, 3000)
}

function stopPolling() {
  if (pollingTimer.value) {
    clearInterval(pollingTimer.value)
    pollingTimer.value = null
  }
}

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.user-migration {
  min-height: 380px;
}

.connection-step {
  max-width: 560px;
  margin: 0 auto;
}

.form-header {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 32px;
  padding-bottom: 24px;
  border-bottom: 1px solid #f0f0f0;
}

.form-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 20px;
  flex-shrink: 0;
}

.form-title h3 {
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 4px;
  color: #1f2937;
}

.form-desc {
  font-size: 13px;
  color: #9ca3af;
  margin: 0;
  line-height: 1.5;
}

.connection-form {
  padding: 8px 0;
}

.form-item {
  margin-bottom: 24px;
}

.form-item :deep(.el-form-item__label) {
  font-weight: 500;
  color: #374151;
  font-size: 14px;
  line-height: 1.5;
  margin-bottom: 8px;
  height: auto;
}

.form-item :deep(.el-input__wrapper) {
  border-radius: 10px;
  padding: 12px 16px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  transition: all 0.2s ease;
}

.form-item :deep(.el-input__wrapper:hover) {
  box-shadow: 0 2px 6px rgba(139, 92, 246, 0.15);
}

.form-item :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 3px rgba(139, 92, 246, 0.1);
}

.form-actions {
  margin-top: 32px;
  margin-bottom: 0;
}

.submit-btn {
  width: 100%;
  height: 44px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 15px;
  letter-spacing: 0.2px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  border: none;
  transition: all 0.2s ease;
}

.submit-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(139, 92, 246, 0.35);
}

.submit-btn:active {
  transform: translateY(0);
}

.submit-btn i {
  margin-right: 6px;
}

.preview-step {
  max-width: 900px;
  margin: 0 auto;
}

.preview-header {
  margin-bottom: 24px;
}

.preview-summary {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 24px;
  padding: 24px;
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
  border-radius: 12px;
  border: 1px solid #e2e8f0;
}

.summary-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.summary-value {
  font-size: 28px;
  font-weight: 700;
  color: #8b5cf6;
  line-height: 1.2;
}

.summary-label {
  font-size: 13px;
  color: #6b7280;
  font-weight: 500;
}

.summary-divider {
  width: 1px;
  height: 40px;
  background: #e2e8f0;
}

.preview-body {
  margin-bottom: 24px;
}

.preview-tabs :deep(.el-tabs__header) {
  margin-bottom: 16px;
}

.preview-table :deep(.el-table__header th) {
  background: #f8fafc;
  color: #374151;
  font-weight: 600;
}

.preview-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  background: #f9fafb;
  border-radius: 12px;
  border: 1px solid #e5e7eb;
}

.sync-info {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #6b7280;
}

.sync-info i {
  color: #8b5cf6;
}

.action-buttons {
  display: flex;
  gap: 12px;
}

.progress-step {
  max-width: 700px;
  margin: 0 auto;
}

.progress-card {
  border-radius: 12px;
}

.progress-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #1f2937;
}

.progress-header i {
  color: #8b5cf6;
}

.progress-body {
  padding: 8px 0;
}

.status-bar {
  margin-bottom: 20px;
}

.sync-logs {
  margin-top: 20px;
  padding-top: 20px;
  border-top: 1px solid #e5e7eb;
}

.logs-title {
  font-size: 14px;
  font-weight: 500;
  color: #374151;
  margin: 0 0 12px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.logs-title i {
  color: #6b7280;
}

.logs-list {
  list-style: none;
  padding: 0;
  margin: 0;
  max-height: 300px;
  overflow-y: auto;
}

.log-item {
  padding: 8px 12px;
  margin-bottom: 4px;
  background: #f9fafb;
  border-radius: 6px;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 12px;
  color: #4b5563;
  line-height: 1.5;
}
</style>
