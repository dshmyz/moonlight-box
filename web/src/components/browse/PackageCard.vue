<template>
  <div class="package-card" @click="$emit('click')">
    <div class="card-top">
      <el-tag :type="getTypeColor(pkg.type)" size="small" effect="plain">
        {{ getPackageTypeLabel(pkg.type) }}
      </el-tag>
      <span class="package-name">{{ pkg.name }}</span>
    </div>
    <p class="package-desc">{{ pkg.description || '暂无描述' }}</p>
    <div class="card-bottom">
      <div class="meta-item">
        <el-icon><PriceTag /></el-icon>
        <span>{{ pkg.latest_version || '-' }}</span>
      </div>
      <div class="meta-item">
        <el-icon><Download /></el-icon>
        <span>{{ formatNumber(pkg.download_count || 0) }}</span>
      </div>
      <div class="meta-item">
        <el-icon><Clock /></el-icon>
        <span>{{ formatRelativeTime(pkg.updated_at) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Download, PriceTag, Clock } from '@element-plus/icons-vue'
import type { Package } from '@/api/package'

defineProps<{
  pkg: Package
}>()

defineEmits<{
  click: []
}>()

function getTypeColor(type: string) {
  const t = type === 'maven' ? 'maven2' : type
  const colors: Record<string, string> = {
    npm: '',
    maven2: 'success',
    pypi: 'warning',
    go: 'info',
  }
  return colors[t] || 'info'
}

function getPackageTypeLabel(type: string) {
  const t = type === 'maven' ? 'maven2' : type
  const labels: Record<string, string> = {
    npm: 'npm',
    maven2: 'Maven',
    pypi: 'PyPI',
    go: 'Go',
  }
  return labels[t] || type
}

function formatNumber(num: number) {
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`
  if (num >= 1000) return `${(num / 1000).toFixed(1)}K`
  return String(num)
}

function formatRelativeTime(timeStr: string) {
  const date = new Date(timeStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffDays < 0) return date.toLocaleDateString('zh-CN')
  if (diffDays === 0) return '今天'
  if (diffDays === 1) return '昨天'
  if (diffDays < 30) return `${diffDays} 天前`
  if (diffDays < 365) return `${Math.floor(diffDays / 30)} 个月前`
  return `${Math.floor(diffDays / 365)} 年前`
}
</script>

<style scoped>
.package-card {
  background: #fff;
  border-radius: 8px;
  padding: 20px;
  cursor: pointer;
  transition: all 0.2s;
}

.package-card:hover {
  background: #fafbfc;
}

.card-top {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.card-top :deep(.el-tag) {
  border-radius: 4px;
  font-size: 11px;
  padding: 0 6px;
  height: 20px;
  line-height: 18px;
}

.package-name {
  font-size: 15px;
  font-weight: 500;
  color: #1f2937;
  letter-spacing: -0.2px;
}

.package-desc {
  color: #6b7280;
  font-size: 13px;
  margin: 0 0 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.6;
}

.card-bottom {
  display: flex;
  gap: 20px;
  align-items: center;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #9ca3af;
}

.meta-item .el-icon {
  font-size: 14px;
}
</style>
