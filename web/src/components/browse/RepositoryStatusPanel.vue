<template>
  <div class="repo-status-panel">
    <div class="panel-header">
      <span class="panel-title">
        <i class="fa-solid fa-satellite-dish"></i>
        仓库状态
      </span>
      <span class="badge-online">
        <span class="online-dot"></span>
        在线
      </span>
    </div>
    <div class="repo-status-list">
      <div
        v-for="repo in repositories"
        :key="repo.name"
        class="repo-status-item"
      >
        <span class="repo-status-dot" :class="getStatusDotClass(repo)"></span>
        <div class="repo-status-info">
          <div class="repo-status-name">{{ repo.display_name || repo.name }}</div>
          <div class="repo-status-type">{{ getPackageTypeLabel(repo.package_type) }} {{ repo.type === 'proxy' ? '代理仓库' : '本地仓库' }}</div>
        </div>
        <span class="repo-status-badge" :class="getStatusBadgeClass(repo)">
          {{ getStatusText(repo) }}
        </span>
      </div>
      <div v-if="loading" class="loading-placeholder">
        <div v-for="i in 3" :key="i" class="skeleton-item">
          <div class="skeleton-dot"></div>
          <div class="skeleton-text">
            <div class="skeleton-line"></div>
            <div class="skeleton-line short"></div>
          </div>
        </div>
      </div>
      <el-empty v-if="!loading && repositories.length === 0" description="暂无仓库" :image-size="48" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { repositoryApi, type RepositoryWithHealth } from '@/api/repository'
import { getPackageTypeLabel } from '@/constants/package'

const repositories = ref<RepositoryWithHealth[]>([])
const loading = ref(false)

async function loadRepositories() {
  loading.value = true
  try {
    const res = await repositoryApi.list({ page_size: 8 })
    repositories.value = Array.isArray(res) ? res : (res.items || [])
  } catch (error) {
    console.error('Failed to load repositories:', error)
  } finally {
    loading.value = false
  }
}

function getStatusDotClass(repo: RepositoryWithHealth): string {
  if (!repo.enabled) return 'repo-status-dot--disabled'
  if (repo.health_info?.health_status?.is_healthy === false) return 'repo-status-dot--error'
  if (repo.health_info?.circuit_breaker?.state === 'open') return 'repo-status-dot--warning'
  return 'repo-status-dot--active'
}

function getStatusBadgeClass(repo: RepositoryWithHealth): string {
  if (!repo.enabled) return 'repo-status-badge--disabled'
  if (repo.health_info?.health_status?.is_healthy === false) return 'repo-status-badge--down'
  if (repo.health_info?.circuit_breaker?.state === 'open') return 'repo-status-badge--degraded'
  return 'repo-status-badge--healthy'
}

function getStatusText(repo: RepositoryWithHealth): string {
  if (!repo.enabled) return '已禁用'
  if (repo.health_info?.health_status?.is_healthy === false) return '异常'
  if (repo.health_info?.circuit_breaker?.state === 'open') return '熔断'
  return '健康'
}

onMounted(() => {
  loadRepositories()
})
</script>

<style scoped>
.repo-status-panel {
  background: var(--lunar-bg-card, rgba(255, 255, 255, 0.85));
  border: 1px solid var(--lunar-border, rgba(139, 92, 246, 0.12));
  border-radius: 12px;
  overflow: hidden;
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.panel-header {
  padding: 14px 18px;
  border-bottom: 1px solid var(--lunar-border, rgba(139, 92, 246, 0.12));
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--lunar-silver, #1e1b4b);
  display: flex;
  align-items: center;
  gap: 8px;
}

.panel-title i {
  font-size: 14px;
  color: var(--lunar-accent, #7c3aed);
}

.badge-online {
  font-size: 11px;
  padding: 3px 10px;
  border-radius: 20px;
  background: rgba(16, 185, 129, 0.1);
  color: #059669;
  display: flex;
  align-items: center;
  gap: 5px;
  font-weight: 500;
}

.online-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 6px rgba(16, 185, 129, 0.5);
  animation: pulse 2s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.repo-status-list {
  padding: 8px 0;
}

.repo-status-item {
  padding: 10px 18px;
  display: flex;
  align-items: center;
  gap: 12px;
  transition: background 0.2s;
}

.repo-status-item:hover {
  background: var(--lunar-bg-glass, rgba(139, 92, 246, 0.06));
}

.repo-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.repo-status-dot--active {
  background: #10b981;
  box-shadow: 0 0 6px rgba(16, 185, 129, 0.4);
}

.repo-status-dot--warning {
  background: #f59e0b;
  box-shadow: 0 0 6px rgba(245, 158, 11, 0.4);
}

.repo-status-dot--error {
  background: #ef4444;
  box-shadow: 0 0 6px rgba(239, 68, 68, 0.4);
}

.repo-status-dot--disabled {
  background: #94a3b8;
}

.repo-status-info {
  flex: 1;
  min-width: 0;
}

.repo-status-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--lunar-silver, #1e1b4b);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.repo-status-type {
  font-size: 11px;
  color: var(--lunar-silver-dim, #6b6fa3);
  margin-top: 2px;
}

.repo-status-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 500;
  flex-shrink: 0;
}

.repo-status-badge--healthy {
  background: rgba(16, 185, 129, 0.1);
  color: #059669;
}

.repo-status-badge--degraded {
  background: rgba(245, 158, 11, 0.1);
  color: #d97706;
}

.repo-status-badge--down {
  background: rgba(239, 68, 68, 0.1);
  color: #dc2626;
}

.repo-status-badge--disabled {
  background: rgba(148, 163, 184, 0.1);
  color: #64748b;
}

.loading-placeholder {
  padding: 10px 18px;
}

.skeleton-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
}

.skeleton-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--lunar-border, #e2e8f0);
}

.skeleton-text {
  flex: 1;
}

.skeleton-line {
  height: 12px;
  background: var(--lunar-border, #e2e8f0);
  border-radius: 4px;
  margin-bottom: 4px;
}

.skeleton-line.short {
  width: 60%;
  height: 10px;
}
</style>
