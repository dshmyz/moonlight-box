<template>
  <el-card shadow="hover" class="repo-card" :class="statusClass">
    <div class="repo-header">
      <span class="repo-name">{{ repo.name }}</span>
      <el-tag :type="statusTagType" size="small" effect="dark">{{ statusLabel }}</el-tag>
    </div>
    <div class="repo-stats">
      <div class="stat-item">
        <span class="stat-value">{{ repo.package_count }}</span>
        <span class="stat-label">包</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">↓ {{ formatNumber(repo.download_count_today) }}</span>
        <span class="stat-label">今日下载</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">{{ formatBytes(repo.storage_bytes) }}</span>
        <span class="stat-label">存储</span>
      </div>
    </div>
    <div class="repo-type">
      <el-tag size="small" effect="plain">{{ repo.package_type }}</el-tag>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { RepoStatus } from '@/api/dashboard'

const props = defineProps<{
  repo: RepoStatus
}>()

/** 状态映射表：将后端状态值映射为 CSS 类名、标签类型和显示文案 */
const statusMap: Record<string, { class: string; tag: string; label: string }> = {
  healthy: { class: 'status-healthy', tag: 'success', label: '健康' },
  syncing: { class: 'status-syncing', tag: 'warning', label: '同步中' },
  error:   { class: 'status-error',   tag: 'danger',  label: '异常' },
  unknown: { class: 'status-unknown', tag: 'info',    label: '未知' },
}

const statusClass = computed(() => statusMap[props.repo.status]?.class || 'status-unknown')
const statusTagType = computed(() => statusMap[props.repo.status]?.tag || 'info')
const statusLabel = computed(() => statusMap[props.repo.status]?.label || '未知')

/** 格式化数字：超过 1000 显示为 K */
const formatNumber = (num: number) => {
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}

/** 格式化字节数为人类可读单位 */
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
  border-radius: 8px;
  transition: all 0.3s ease;
}
.repo-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
}
.repo-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.repo-name {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}
.repo-stats {
  display: flex;
  gap: 16px;
  margin-bottom: 12px;
}
.stat-item {
  flex: 1;
  text-align: center;
}
.stat-value {
  display: block;
  font-size: 18px;
  font-weight: 700;
  color: #409eff;
}
.stat-label {
  font-size: 12px;
  color: #909399;
}
.repo-type {
  text-align: right;
}
.status-healthy { border-left: 3px solid #67c23a; }
.status-syncing { border-left: 3px solid #e6a23c; }
.status-error { border-left: 3px solid #f56c6c; }
.status-unknown { border-left: 3px solid #909399; }
</style>
