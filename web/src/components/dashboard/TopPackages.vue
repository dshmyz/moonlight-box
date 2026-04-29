<template>
  <el-card shadow="hover" class="top-packages-card">
    <template #header>
      <div class="card-header">
        <span class="card-title">
          <el-icon><TrendCharts /></el-icon>
          热门下载 Top 5
        </span>
      </div>
    </template>
    <div v-if="!packages || packages.length === 0" class="empty-tip">暂无数据</div>
    <div v-else class="package-list">
      <div
        v-for="(pkg, index) in packages"
        :key="pkg.name"
        class="package-item"
        :class="`rank-${index + 1}`"
      >
        <div class="rank-badge">{{ index + 1 }}</div>
        <div class="package-info">
          <div class="package-name-row">
            <span class="package-name">{{ pkg.name }}</span>
            <el-tag size="small" effect="plain" class="type-tag">{{ getTypeLabel(pkg.type) }}</el-tag>
            <el-tag v-if="pkg.license" size="small" class="license-tag" type="info" effect="plain">{{ pkg.license }}</el-tag>
          </div>
          <div v-if="pkg.description" class="package-desc">{{ pkg.description }}</div>
        </div>
        <div class="download-count">
          <span class="download-value">{{ formatNumber(pkg.download_count) }}</span>
          <span class="download-label">次</span>
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { TrendCharts } from '@element-plus/icons-vue'
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
  border-radius: 8px;
}

.card-header {
  display: flex;
  align-items: center;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.card-title .el-icon {
  color: #409eff;
}

.empty-tip {
  text-align: center;
  color: #909399;
  padding: 24px 0;
  font-size: 14px;
}

.package-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.package-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 6px;
  background: #fafafa;
  transition: all 0.2s;
}

.package-item:hover {
  background: #f0f2f5;
}

.rank-badge {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 700;
  color: #909399;
  background: #e4e7ed;
  flex-shrink: 0;
}

.rank-1 .rank-badge {
  background: linear-gradient(135deg, #ffd700 0%, #ffaa00 100%);
  color: #fff;
}

.rank-2 .rank-badge {
  background: linear-gradient(135deg, #c0c0c0 0%, #a0a0a0 100%);
  color: #fff;
}

.rank-3 .rank-badge {
  background: linear-gradient(135deg, #cd7f32 0%, #b87333 100%);
  color: #fff;
}

.package-info {
  flex: 1;
  min-width: 0;
}

.package-name-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}

.package-name {
  font-size: 14px;
  font-weight: 500;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.type-tag {
  flex-shrink: 0;
  font-size: 11px;
}

.license-tag {
  flex-shrink: 0;
  font-size: 11px;
}

.package-desc {
  font-size: 12px;
  color: #909399;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.download-count {
  text-align: right;
  flex-shrink: 0;
}

.download-value {
  font-size: 15px;
  font-weight: 700;
  color: #409eff;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
}

.download-label {
  font-size: 12px;
  color: #909399;
  margin-left: 2px;
}
</style>
