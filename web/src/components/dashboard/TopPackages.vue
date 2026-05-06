<template>
  <div class="top-packages-card">
    <div class="card-header">
      <span class="card-title">
        <span class="title-icon">◈</span>
        热门下载
      </span>
      <span class="card-badge">TOP 5</span>
    </div>
    <div v-if="!packages || packages.length === 0" class="empty-tip">暂无数据</div>
    <div v-else class="package-list">
      <div
        v-for="(pkg, index) in packages"
        :key="pkg.name"
        class="package-item"
        :class="`rank-${index + 1}`"
      >
        <div class="rank-indicator" :class="`indicator--${index + 1}`">
          <span class="rank-number">{{ index + 1 }}</span>
        </div>
        <div class="package-info">
          <div class="package-name-row">
            <span class="package-name">{{ pkg.name }}</span>
            <span class="type-tag">{{ getTypeLabel(pkg.type) }}</span>
            <span v-if="pkg.license" class="license-tag">{{ pkg.license }}</span>
          </div>
          <div v-if="pkg.description" class="package-desc">{{ pkg.description }}</div>
        </div>
        <div class="download-count">
          <span class="download-value">{{ formatNumber(pkg.download_count) }}</span>
          <span class="download-unit">次</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { PackageTop } from '@/api/dashboard'

defineProps<{
  packages: PackageTop[]
}>()

const typeLabels: Record<string, string> = {
  npm: 'npm',
  maven: 'Maven',
  pypi: 'PyPI',
  go: 'Go',
  nuget: 'NuGet',
  yum: 'Yum',
  apt: 'Apt',
  generic: 'Generic',
}

function getTypeLabel(type: string) {
  return typeLabels[type] || type
}

function formatNumber(num: number) {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}
</script>

<style scoped>
.top-packages-card {
  background: #ffffff;
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: 16px;
  padding: 20px 24px;
  height: 100%;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
}

.title-icon {
  font-size: 18px;
  color: #818cf8;
}

.card-badge {
  padding: 3px 10px;
  background: rgba(99, 102, 241, 0.15);
  border: 1px solid rgba(99, 102, 241, 0.3);
  border-radius: 20px;
  font-size: 11px;
  font-weight: 700;
  color: #818cf8;
  letter-spacing: 0.05em;
}

.empty-tip {
  text-align: center;
  color: #64748b;
  padding: 32px 0;
  font-size: 14px;
}

.package-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.package-item {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 14px;
  border-radius: 10px;
  background: #f8fafc;
  border: 1px solid rgba(0, 0, 0, 0.04);
  transition: all 0.2s ease;
}

.package-item:hover {
  background: #f1f5f9;
  border-color: rgba(0, 0, 0, 0.08);
  transform: translateX(4px);
}

.rank-indicator {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.indicator--1 {
  background: linear-gradient(135deg, #fbbf24 0%, #f59e0b 100%);
  box-shadow: 0 4px 12px rgba(251, 191, 36, 0.3);
}

.indicator--2 {
  background: linear-gradient(135deg, #94a3b8 0%, #64748b 100%);
  box-shadow: 0 4px 12px rgba(148, 163, 184, 0.3);
}

.indicator--3 {
  background: linear-gradient(135deg, #d97706 0%, #b45309 100%);
  box-shadow: 0 4px 12px rgba(217, 119, 6, 0.3);
}

.indicator--4, .indicator--5 {
  background: rgba(100, 116, 139, 0.2);
  border: 1px solid rgba(100, 116, 139, 0.3);
}

.rank-number {
  font-size: 13px;
  font-weight: 700;
  color: #fff;
}

.indicator--4 .rank-number, .indicator--5 .rank-number {
  color: #94a3b8;
}

.package-info {
  flex: 1;
  min-width: 0;
}

.package-name-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.package-name {
  font-size: 14px;
  font-weight: 500;
  color: #1e293b;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.type-tag {
  flex-shrink: 0;
  padding: 1px 6px;
  background: rgba(99, 102, 241, 0.15);
  border: 1px solid rgba(99, 102, 241, 0.25);
  border-radius: 4px;
  font-size: 10px;
  color: #818cf8;
  font-weight: 500;
}

.license-tag {
  flex-shrink: 0;
  padding: 1px 6px;
  background: rgba(148, 163, 184, 0.1);
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 4px;
  font-size: 10px;
  color: #94a3b8;
}

.package-desc {
  font-size: 12px;
  color: #64748b;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.download-count {
  text-align: right;
  flex-shrink: 0;
}

.download-value {
  font-size: 16px;
  font-weight: 700;
  color: #60a5fa;
  font-variant-numeric: tabular-nums;
}

.download-unit {
  font-size: 11px;
  color: #64748b;
  margin-left: 2px;
}
</style>
