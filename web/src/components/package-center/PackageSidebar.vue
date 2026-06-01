<template>
  <aside class="sidebar">
    <div class="sidebar-card">
      <div class="sidebar-header">
        <span>🔥 热门包</span>
        <span class="badge-count">TOP 5</span>
      </div>
      <div class="trending-list">
        <div
          v-for="(pkg, index) in topPackages"
          :key="pkg.name"
          class="trending-item"
        >
          <span class="trending-rank" :class="index < 3 ? 'trending-rank--top3' : 'trending-rank--normal'">
            {{ index + 1 }}
          </span>
          <div class="trending-info">
            <div class="trending-name">{{ pkg.name }}</div>
            <div class="trending-type">{{ getPackageTypeLabel(pkg.type) }}</div>
          </div>
          <div class="trending-downloads">
            <i class="fa-solid fa-arrow-trend-up"></i>
            {{ formatNumber(pkg.download_count) }}
          </div>
        </div>
        <el-empty v-if="topPackages.length === 0" description="暂无数据" :image-size="60" />
      </div>
    </div>

    <div class="sidebar-card">
      <div class="sidebar-header">
        <span>🕐 最近更新</span>
        <span class="badge-count">24h</span>
      </div>
      <div class="recent-list">
        <div
          v-for="(item, index) in recentUpdates"
          :key="index"
          class="recent-item"
        >
          <div class="recent-icon" :class="item.isNew ? 'recent-icon--new' : 'recent-icon--update'">
            <i :class="item.isNew ? 'fa-solid fa-plus' : 'fa-solid fa-arrow-up'"></i>
          </div>
          <div class="recent-info">
            <div class="recent-title">
              {{ item.isNew ? '新增' : '更新' }} <strong>{{ item.name }}</strong>
              <span v-if="item.version">至 v{{ item.version }}</span>
            </div>
            <div class="recent-time">{{ formatRelativeTime(item.time) }}</div>
          </div>
        </div>
        <el-empty v-if="recentUpdates.length === 0" description="暂无更新" :image-size="60" />
      </div>
    </div>

    <div class="sidebar-card">
      <div class="sidebar-header">
        <span>📡 仓库状态</span>
        <span class="badge-count">在线</span>
      </div>
      <div class="repo-status-list">
        <div
          v-for="repo in repositories"
          :key="repo.name"
          class="repo-status-item"
        >
          <span class="repo-status-dot" :class="getStatusDotClass(repo.health_info?.health_status?.is_healthy)"></span>
          <div class="repo-status-info">
            <div class="repo-status-name">{{ repo.display_name || repo.name }}</div>
            <div class="repo-status-type">{{ getPackageTypeLabel(repo.package_type) }} {{ repo.type === 'proxy' ? '代理仓库' : '本地仓库' }}</div>
          </div>
          <span class="repo-status-badge" :class="getStatusBadgeClass(repo.health_info?.health_status?.is_healthy)">
            {{ repo.health_info?.health_status?.is_healthy === false ? '异常' : '健康' }}
          </span>
        </div>
        <el-empty v-if="repositories.length === 0" description="暂无仓库" :image-size="60" />
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import type { PackageTop } from '@/api/dashboard'
import type { RepositoryWithHealth } from '@/api/repository'
import { formatNumber, formatRelativeTime } from '@/utils/format'
import { getPackageTypeLabel } from '@/constants/package'

defineProps<{
  topPackages: PackageTop[]
  repositories: RepositoryWithHealth[]
  recentUpdates: Array<{
    name: string
    type: string
    version: string
    time: string
    isNew: boolean
  }>
}>()

function getStatusDotClass(isHealthy: boolean | undefined): string {
  if (isHealthy === false) return 'repo-status-dot--error'
  return 'repo-status-dot--active'
}

function getStatusBadgeClass(isHealthy: boolean | undefined): string {
  if (isHealthy === false) return 'repo-status-badge--down'
  return 'repo-status-badge--healthy'
}
</script>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.sidebar-card {
  background: var(--color-bg-primary, #ffffff);
  border-radius: 16px;
  border: 1px solid var(--color-border, #e2e8f0);
  overflow: hidden;
}

.sidebar-header {
  padding: 16px 20px;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-primary, #0f172a);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.badge-count {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  background: var(--color-bg-page, #f8fafc);
  color: var(--color-text-tertiary, #94a3b8);
}

.trending-list {
  padding: 8px 0;
}

.trending-item {
  padding: 12px 20px;
  display: flex;
  align-items: center;
  gap: 12px;
  transition: background 0.2s;
  cursor: pointer;
}

.trending-item:hover {
  background: var(--color-bg-hover, #f8fafc);
}

.trending-rank {
  width: 24px;
  height: 24px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 600;
  flex-shrink: 0;
}

.trending-rank--top3 {
  background: linear-gradient(135deg, #fef3c7 0%, #fcd34d 100%);
  color: #d97706;
}

.trending-rank--normal {
  background: var(--color-bg-page, #f8fafc);
  color: var(--color-text-tertiary, #94a3b8);
}

.trending-info {
  flex: 1;
  min-width: 0;
}

.trending-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-primary, #0f172a);
  margin-bottom: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trending-type {
  font-size: 11px;
  color: var(--color-text-tertiary, #94a3b8);
}

.trending-downloads {
  font-size: 12px;
  color: var(--color-text-tertiary, #94a3b8);
  display: flex;
  align-items: center;
  gap: 4px;
}

.trending-downloads i {
  color: #10b981;
  font-size: 10px;
}

.recent-list {
  padding: 8px 0;
}

.recent-item {
  padding: 12px 20px;
  display: flex;
  align-items: flex-start;
  gap: 12px;
  transition: background 0.2s;
  cursor: pointer;
}

.recent-item:hover {
  background: var(--color-bg-hover, #f8fafc);
}

.recent-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  flex-shrink: 0;
}

.recent-icon--update {
  background: #d1fae5;
  color: #059669;
}

.recent-icon--new {
  background: #dbeafe;
  color: #3b82f6;
}

.recent-info {
  flex: 1;
  min-width: 0;
}

.recent-title {
  font-size: 13px;
  color: var(--color-text-primary, #0f172a);
  margin-bottom: 4px;
  line-height: 1.4;
}

.recent-title strong {
  font-weight: 600;
}

.recent-time {
  font-size: 11px;
  color: var(--color-text-tertiary, #94a3b8);
}

.repo-status-list {
  padding: 12px 0;
}

.repo-status-item {
  padding: 12px 20px;
  display: flex;
  align-items: center;
  gap: 12px;
}

.repo-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.repo-status-dot--active {
  background: #10b981;
  box-shadow: 0 0 8px rgba(16, 185, 129, 0.4);
}

.repo-status-dot--warning {
  background: #f59e0b;
  box-shadow: 0 0 8px rgba(245, 158, 11, 0.4);
}

.repo-status-dot--error {
  background: #ef4444;
  box-shadow: 0 0 8px rgba(239, 68, 68, 0.4);
}

.repo-status-info {
  flex: 1;
}

.repo-status-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-primary, #0f172a);
}

.repo-status-type {
  font-size: 11px;
  color: var(--color-text-tertiary, #94a3b8);
}

.repo-status-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 500;
}

.repo-status-badge--healthy {
  background: #d1fae5;
  color: #059669;
}

.repo-status-badge--degraded {
  background: #fef3c7;
  color: #d97706;
}

.repo-status-badge--down {
  background: #fee2e2;
  color: #dc2626;
}

@media (max-width: 1200px) {
  .sidebar {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 768px) {
  .sidebar {
    grid-template-columns: 1fr;
  }
}
</style>
