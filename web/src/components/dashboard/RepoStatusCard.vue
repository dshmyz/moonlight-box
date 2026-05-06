<template>
  <div class="repo-card" :class="statusClass">
    <div class="repo-glass">
      <div class="repo-glow" />
      <div class="repo-content">
        <div class="repo-header">
          <span class="repo-name">{{ repo.name }}</span>
          <span class="repo-status-badge" :class="`badge--${repo.status}`">
            <span class="badge-dot" />
            {{ statusLabel }}
          </span>
        </div>
        <div class="repo-stats">
          <div class="stat-item">
            <span class="stat-value">{{ repo.package_count }}</span>
            <span class="stat-label">包</span>
          </div>
          <div class="stat-item">
            <span class="stat-value downloads">↓ {{ formatNumber(repo.download_count_today) }}</span>
            <span class="stat-label">今日</span>
          </div>
          <div class="stat-item">
            <span class="stat-value">{{ formatBytes(repo.storage_bytes) }}</span>
            <span class="stat-label">存储</span>
          </div>
        </div>
        <div class="repo-footer">
          <span class="repo-type-tag">{{ repo.package_type }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { RepoStatus } from '@/api/dashboard'

const props = defineProps<{
  repo: RepoStatus
}>()

const statusMap: Record<string, { class: string; label: string }> = {
  healthy: { class: 'healthy', label: '健康' },
  syncing: { class: 'syncing', label: '同步中' },
  error:   { class: 'error',   label: '异常' },
  unknown: { class: 'unknown', label: '未知' },
}

const statusClass = computed(() => 'repo-card--' + (statusMap[props.repo.status]?.class || 'unknown'))
const statusLabel = computed(() => statusMap[props.repo.status]?.label || '未知')

const formatNumber = (num: number) => {
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
}
</script>

<style scoped>
.repo-card {
  position: relative;
  border-radius: 12px;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.repo-card:hover {
  transform: translateY(-2px);
}

.repo-glass {
  position: relative;
  background: #ffffff;
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: inherit;
  overflow: hidden;
  padding: 14px 16px;
  margin-bottom: 16px;
}

.repo-card:hover .repo-glass {
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.06);
}

.repo-content {
  position: relative;
  z-index: 1;
}

.repo-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 12px;
  gap: 8px;
}

.repo-name {
  font-size: 13px;
  font-weight: 600;
  color: #1e293b;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.repo-status-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  border-radius: 16px;
  font-size: 10px;
  font-weight: 600;
  flex-shrink: 0;
}

.badge-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
}

.badge--healthy {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.3);
}
.badge--healthy .badge-dot { background: #10b981; box-shadow: 0 0 4px #10b981; }

.badge--syncing {
  background: rgba(245, 158, 11, 0.15);
  color: #fbbf24;
  border: 1px solid rgba(245, 158, 11, 0.3);
}
.badge--syncing .badge-dot { background: #f59e0b; box-shadow: 0 0 4px #f59e0b; animation: pulse 1.5s infinite; }

.badge--error {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
  border: 1px solid rgba(239, 68, 68, 0.3);
}
.badge--error .badge-dot { background: #ef4444; box-shadow: 0 0 4px #ef4444; }

.badge--unknown {
  background: rgba(148, 163, 184, 0.1);
  color: #94a3b8;
  border: 1px solid rgba(148, 163, 184, 0.2);
}
.badge--unknown .badge-dot { background: #94a3b8; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.repo-stats {
  display: flex;
  gap: 12px;
  margin-bottom: 10px;
}

.stat-item {
  flex: 1;
  text-align: center;
}

.stat-value {
  display: block;
  font-size: 18px;
  font-weight: 700;
  color: #60a5fa;
  font-variant-numeric: tabular-nums;
}

.stat-value.downloads {
  color: #a78bfa;
}

.stat-label {
  font-size: 10px;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.repo-footer {
  text-align: right;
}

.repo-type-tag {
  display: inline-block;
  padding: 2px 6px;
  background: rgba(99, 102, 241, 0.15);
  border: 1px solid rgba(99, 102, 241, 0.3);
  border-radius: 4px;
  font-size: 10px;
  color: #818cf8;
  font-weight: 500;
}
</style>
