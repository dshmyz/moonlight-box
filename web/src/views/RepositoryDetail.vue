<template>
  <div class="repo-detail-page">
    <div class="detail-header">
      <div class="header-left">
        <button class="back-btn" @click="goBack">
          <el-icon><ArrowLeft /></el-icon>
        </button>
        <div class="header-info">
          <div class="repo-title-row">
            <div class="repo-icon" :class="`repo-icon--${repo?.type}`">
              <i :class="getRepoIcon(repo?.type || '')"></i>
            </div>
            <div>
              <h1 class="repo-name">{{ repo?.display_name || repo?.name }}</h1>
              <span class="repo-type">{{ getTypeLabel(repo?.type || '') }}</span>
            </div>
          </div>
          <p v-if="repo?.description" class="repo-desc">{{ repo.description }}</p>
        </div>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="openEditDialog">
          <el-icon><Edit /></el-icon>
          编辑
        </el-button>
      </div>
    </div>

    <div class="detail-content">
      <div class="content-left">
        <el-card class="info-card">
          <template #header>
            <span class="card-title">基本信息</span>
          </template>
          <div class="info-grid">
            <div class="info-item">
              <label>仓库名称</label>
              <span>{{ repo?.name }}</span>
            </div>
            <div class="info-item">
              <label>显示名称</label>
              <span>{{ repo?.display_name || '-' }}</span>
            </div>
            <div class="info-item">
              <label>类型</label>
              <el-tag :class="['type-tag', `type-tag--${repo?.type}`]">{{ getTypeLabel(repo?.type || '') }}</el-tag>
            </div>
            <div class="info-item">
              <label>包类型</label>
              <span>{{ getPackageTypeLabel(repo?.package_type || '') }}</span>
            </div>
            <div class="info-item">
              <label>状态</label>
              <el-tag :class="repo?.enabled ? 'status-tag--enabled' : 'status-tag--disabled'">
                {{ repo?.enabled ? '已启用' : '已禁用' }}
              </el-tag>
            </div>
            <div class="info-item">
              <label>公开可见</label>
              <span>{{ repo?.public_visible ? '是' : '否' }}</span>
            </div>
            <div class="info-item">
              <label>创建时间</label>
              <span>{{ formatTime(repo?.created_at) }}</span>
            </div>
            <div class="info-item">
              <label>更新时间</label>
              <span>{{ formatTime(repo?.updated_at) }}</span>
            </div>
          </div>
        </el-card>

        <el-card v-if="repo?.type === 'proxy'" class="info-card">
          <template #header>
            <span class="card-title">代理配置</span>
          </template>
          <div class="info-grid">
            <div class="info-item">
              <label>远程地址</label>
              <code class="url-text">{{ repo?.config?.remote_url }}</code>
            </div>
            <div class="info-item">
              <label>优先级</label>
              <span>{{ repo?.config?.proxy_priority || 0 }}</span>
            </div>
            <div class="info-item">
              <label>超时时间</label>
              <span>{{ repo?.config?.timeout_seconds || 0 }} 秒</span>
            </div>
            <div class="info-item">
              <label>最大重定向次数</label>
              <span>{{ repo?.config?.max_redirects || 5 }}</span>
            </div>
            <div class="info-item">
              <label>跳过证书验证</label>
              <span>{{ repo?.config?.insecure_skip_verify ? '是' : '否' }}</span>
            </div>
          </div>
        </el-card>

        <el-card class="info-card">
          <template #header>
            <span class="card-title">存储配置</span>
          </template>
          <div class="info-grid">
            <div class="info-item">
              <label>存储后端</label>
              <span>{{ repo?.storage_backend_id ? `ID: ${repo.storage_backend_id}` : '默认' }}</span>
            </div>
            <div class="info-item">
              <label>允许覆盖</label>
              <span>{{ repo?.allow_overwrite ? '是' : '否' }}</span>
            </div>
            <div class="info-item">
              <label>允许删除</label>
              <span>{{ repo?.allow_delete ? '是' : '否' }}</span>
            </div>
          </div>
        </el-card>

        <el-card v-if="repo?.config?.cache_enabled" class="info-card">
          <template #header>
            <span class="card-title">缓存配置</span>
          </template>
          <div class="info-grid">
            <div class="info-item">
              <label>缓存启用</label>
              <span>{{ repo?.config?.cache_enabled ? '是' : '否' }}</span>
            </div>
            <div class="info-item">
              <label>缓存有效期</label>
              <span>{{ repo?.config?.cache_ttl_seconds }} 秒</span>
            </div>
            <div class="info-item">
              <label>负缓存有效期</label>
              <span>{{ repo?.config?.cache_negative_ttl }} 秒</span>
            </div>
            <div class="info-item">
              <label>最大缓存大小</label>
              <span>{{ repo?.config?.cache_max_size_gb }} GB</span>
            </div>
          </div>
        </el-card>
      </div>

      <div class="content-right">
        <el-card class="health-card">
          <template #header>
            <span class="card-title">健康状态</span>
          </template>
          <div v-if="repo?.health_info" class="health-content">
            <div class="health-status-row">
              <div :class="['health-indicator', getHealthClass(repo)]"></div>
              <div class="health-status-text">
                <span class="health-status-label">{{ getHealthStatusText(repo) }}</span>
                <span class="health-status-detail">{{ getHealthDetail(repo) }}</span>
              </div>
            </div>
            <div v-if="repo.health_info.circuit_breaker" class="circuit-info">
              <div class="circuit-item">
                <label>熔断状态</label>
                <span>{{ repo.health_info.circuit_breaker.state }}</span>
              </div>
              <div class="circuit-item">
                <label>成功次数</label>
                <span>{{ repo.health_info.circuit_breaker.success_count }}</span>
              </div>
              <div class="circuit-item">
                <label>失败次数</label>
                <span>{{ repo.health_info.circuit_breaker.failure_count }}</span>
              </div>
            </div>
          </div>
          <div v-else class="no-health-info">
            <i class="fa-solid fa-circle-question"></i>
            <span>暂无健康检查信息</span>
          </div>
        </el-card>

        <el-card v-if="repo?.type === 'virtual'" class="members-card">
          <template #header>
            <span class="card-title">成员仓库</span>
            <el-button size="small" type="primary" @click="openMemberDialog">
              <el-icon><Plus /></el-icon>
              添加
            </el-button>
          </template>
          <div v-if="members.length > 0" class="members-list">
            <div v-for="member in members" :key="member.id" class="member-item">
              <div class="member-info">
                <div class="member-icon" :class="`repo-icon--${member.member_repo?.type}`">
                  <i :class="getRepoIcon(member.member_repo?.type || '')"></i>
                </div>
                <div>
                  <span class="member-name">{{ member.member_repo?.display_name || member.member_repo?.name }}</span>
                  <span class="member-type">{{ member.member_repo?.type }}</span>
                </div>
              </div>
              <div class="member-actions">
                <span class="member-position">优先级: {{ member.position }}</span>
                <el-button size="small" text @click="removeMember(member.member_repo?.name || '')">删除</el-button>
              </div>
            </div>
          </div>
          <div v-else class="no-members">
            <i class="fa-solid fa-inbox"></i>
            <span>暂无成员仓库</span>
          </div>
        </el-card>
      </div>
    </div>

    <RepositoryFormDialog
      v-model="showEditDialog"
      :edit-data="editingRepo"
      @submit="handleFormSubmit"
    />

    <el-dialog v-model="showMemberDialog" title="添加成员仓库" width="500px">
      <el-form label-width="80px">
        <el-form-item label="选择仓库">
          <el-select
            v-model="newMemberName"
            placeholder="请选择仓库"
            filterable
            style="width: 100%"
          >
            <el-option
              v-for="repo in availableRepos"
              :key="repo.name"
              :label="repo.display_name || repo.name"
              :value="repo.name"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="newMemberPosition" :min="1" :max="100" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showMemberDialog = false">取消</el-button>
        <el-button type="primary" @click="addMember" :disabled="!newMemberName">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Edit, Plus } from '@element-plus/icons-vue'
import { repositoryApi, type Repository, type RepositoryWithHealth, type RepositoryMember } from '@/api/repository'
import RepositoryFormDialog from '@/components/repository/RepositoryFormDialog.vue'
import { success, error } from '@/utils/message'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const repo = ref<RepositoryWithHealth | null>(null)
const members = ref<RepositoryMember[]>([])
const showEditDialog = ref(false)
const showMemberDialog = ref(false)
const editingRepo = ref<Repository | null>(null)
const availableRepos = ref<Repository[]>([])
const newMemberName = ref('')
const newMemberPosition = ref(1)

const getRepoIcon = (type: string) => {
  switch (type) {
    case 'local': return 'fa-solid fa-folder'
    case 'proxy': return 'fa-solid fa-rotate'
    case 'virtual': return 'fa-solid fa-wand-magic-sparkles'
    default: return 'fa-solid fa-box'
  }
}

const getTypeLabel = (type: string) => {
  const map: Record<string, string> = { local: '本地仓库', proxy: '代理仓库', virtual: '虚拟仓库' }
  return map[type] || type
}

const getPackageTypeLabel = (type: string) => {
  const map: Record<string, string> = {
    npm: 'npm',
    maven: 'Maven',
    pypi: 'PyPI',
    go: 'Go',
    yum: 'Yum',
    apt: 'Apt',
    generic: 'Generic',
  }
  return map[type] || type
}

const formatTime = (time?: string) => {
  if (!time || time === '') return '-'
  const date = new Date(time)
  if (isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN')
}

const getHealthClass = (repo: RepositoryWithHealth) => {
  if (!repo.enabled) return 'health-indicator--disabled'
  if (!repo.health_info?.health_status) return 'health-indicator--unknown'
  
  const health = repo.health_info.health_status
  if (!health.is_healthy) return 'health-indicator--error'
  if (health.consecutive_failures > 0) return 'health-indicator--warning'
  return 'health-indicator--healthy'
}

const getHealthStatusText = (repo: RepositoryWithHealth) => {
  if (!repo.enabled) return '已禁用'
  if (!repo.health_info?.health_status) return '未知'
  
  const health = repo.health_info.health_status
  if (!health.is_healthy) return '不健康'
  if (health.consecutive_failures > 0) return '警告'
  return '健康'
}

const getHealthDetail = (repo: RepositoryWithHealth) => {
  if (!repo.enabled) return '仓库已禁用，健康检查未运行'
  if (!repo.health_info?.health_status) return '等待首次检查'
  
  const health = repo.health_info.health_status
  const responseTimeMs = Math.round(health.response_time / 1_000_000)
  
  if (!health.is_healthy) {
    return `错误: ${health.last_check_error || '未知'} | 连续失败: ${health.consecutive_failures}次`
  }
  if (health.consecutive_failures > 0) {
    return `最近 ${health.consecutive_failures} 次失败，当前已恢复 | 响应: ${responseTimeMs}ms`
  }
  const lastCheckTime = health.last_check_time ? formatTime(health.last_check_time) : '-'
  return `响应时间: ${responseTimeMs}ms | 最后检查: ${lastCheckTime}`
}

async function loadRepo() {
  const name = route.params.name as string
  if (!name) return

  loading.value = true
  try {
    const res = await repositoryApi.get(name)
    repo.value = res as RepositoryWithHealth
    
    if (repo.value.type === 'virtual') {
      await loadMembers()
    }
  } catch (e) {
    error('加载仓库信息失败')
  } finally {
    loading.value = false
  }
}

async function loadMembers() {
  const name = route.params.name as string
  if (!name) return
  
  try {
    const res = await repositoryApi.getMembers(name)
    members.value = res as RepositoryMember[]
  } catch (e) {
    console.error('加载成员仓库失败', e)
  }
}

async function loadAvailableRepos() {
  try {
    const res = await repositoryApi.list()
    const allRepos: any[] = (res && typeof res === 'object' && 'items' in res)
      ? (res as any).items
      : (res as any[]) || []
    const currentRepoName = route.params.name as string
    availableRepos.value = allRepos.filter((repo: any) => 
      repo.type !== 'virtual' && 
      repo.name !== currentRepoName
    )
  } catch (e) {
    console.error('加载可用仓库失败', e)
  }
}

function goBack() {
  router.push('/admin/repositories')
}

function openEditDialog() {
  editingRepo.value = { ...repo.value } as Repository
  showEditDialog.value = true
}

function handleFormSubmit() {
  showEditDialog.value = false
  loadRepo()
}

function openMemberDialog() {
  newMemberName.value = ''
  newMemberPosition.value = 1
  loadAvailableRepos()
  showMemberDialog.value = true
}

async function addMember() {
  const name = route.params.name as string
  if (!name || !newMemberName.value) return
  
  try {
    await repositoryApi.addMember(name, {
      member_name: newMemberName.value,
      position: newMemberPosition.value
    })
    success('添加成功')
    showMemberDialog.value = false
    loadMembers()
  } catch (e) {
    error('添加失败')
  }
}

async function removeMember(memberName: string) {
  const name = route.params.name as string
  if (!name || !memberName) return
  
  try {
    await repositoryApi.removeMember(name, memberName)
    success('删除成功')
    loadMembers()
  } catch (e) {
    error('删除失败')
  }
}

onMounted(() => {
  loadRepo()
  loadAvailableRepos()
})
</script>

<style scoped>
.repo-detail-page {
  min-height: calc(100vh - 60px);
  background: #f8fafc;
  padding: 24px;
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 24px;
  padding: 24px;
  background: #ffffff;
  border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.06);
}

.header-left {
  display: flex;
  align-items: flex-start;
  gap: 16px;
}

.back-btn {
  padding: 10px;
  border: none;
  background: #f1f5f9;
  border-radius: 10px;
  cursor: pointer;
  color: #64748b;
  transition: all 0.2s ease;
}

.back-btn:hover {
  background: #e2e8f0;
  color: #475569;
}

.header-info {
  flex: 1;
}

.repo-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.repo-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
}

.repo-icon--local {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
}

.repo-icon--proxy {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
}

.repo-icon--virtual {
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
}

.repo-name {
  font-size: 24px;
  font-weight: 700;
  color: #1e293b;
  margin: 0;
}

.repo-type {
  font-size: 13px;
  color: #64748b;
  margin-left: 8px;
}

.repo-desc {
  font-size: 14px;
  color: #64748b;
  margin: 8px 0 0;
}

.header-right {
  flex-shrink: 0;
}

.detail-content {
  display: grid;
  grid-template-columns: 1fr 380px;
  gap: 24px;
}

.content-left {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.content-right {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.info-card {
  border-radius: 14px;
  border: 1px solid rgba(0, 0, 0, 0.06);
}

.card-title {
  font-weight: 600;
  color: #1e293b;
  font-size: 14px;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.info-item label {
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
}

.info-item span {
  font-size: 13px;
  color: #1e293b;
}

.url-text {
  font-family: var(--font-family-mono);
  font-size: 12px;
  color: #059669;
  word-break: break-all;
}

.type-tag {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 6px;
  font-weight: 500;
  border: none;
}

.type-tag--local {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.type-tag--proxy {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.type-tag--virtual {
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
  color: #4f46e5;
}

.status-tag--enabled {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.status-tag--disabled {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #dc2626;
}

.health-card {
  border-radius: 14px;
  border: 1px solid rgba(0, 0, 0, 0.06);
}

.health-content {
  padding: 8px 0;
}

.health-status-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.health-indicator {
  width: 16px;
  height: 16px;
  border-radius: 50%;
}

.health-indicator--healthy {
  background: #10b981;
  box-shadow: 0 0 8px rgba(16, 185, 129, 0.5);
}

.health-indicator--warning {
  background: #f59e0b;
  box-shadow: 0 0 8px rgba(245, 158, 11, 0.5);
}

.health-indicator--error {
  background: #ef4444;
  box-shadow: 0 0 8px rgba(239, 68, 68, 0.5);
}

.health-indicator--disabled {
  background: #94a3b8;
}

.health-indicator--unknown {
  background: #94a3b8;
  border: 1px dashed #64748b;
}

.health-status-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.health-status-label {
  font-weight: 600;
  font-size: 14px;
  color: #1e293b;
}

.health-status-detail {
  font-size: 12px;
  color: #64748b;
}

.circuit-info {
  display: flex;
  gap: 16px;
  padding-top: 12px;
  border-top: 1px solid #f1f5f9;
}

.circuit-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.circuit-item label {
  font-size: 11px;
  color: #94a3b8;
}

.circuit-item span {
  font-size: 12px;
  color: #475569;
}

.no-health-info,
.no-members {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 24px;
  color: #94a3b8;
}

.members-card {
  border-radius: 14px;
  border: 1px solid rgba(0, 0, 0, 0.06);
}

.members-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.member-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  background: #f8fafc;
  border-radius: 10px;
}

.member-info {
  display: flex;
  align-items: center;
  gap: 10px;
}

.member-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
}

.member-name {
  display: block;
  font-size: 13px;
  font-weight: 500;
  color: #1e293b;
}

.member-type {
  font-size: 11px;
  color: #94a3b8;
}

.member-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.member-position {
  font-size: 12px;
  color: #64748b;
}

@media (max-width: 1024px) {
  .detail-content {
    grid-template-columns: 1fr;
  }
}
</style>