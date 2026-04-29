<template>
  <div class="package-card" @click="$emit('click')">
    <div class="card-main">
      <div class="card-header">
        <el-tag :type="getPackageTypeColor(pkg.type)" size="small" effect="plain" class="type-tag">
          {{ getPackageTypeLabel(pkg.type) }}
        </el-tag>
        <span class="package-name">{{ pkg.name }}</span>
      </div>
      <p class="package-desc">{{ pkg.description || '暂无描述' }}</p>
    </div>
    <div class="card-footer">
      <div class="meta-item">
        <el-icon><PriceTag /></el-icon>
        <span class="meta-label">版本</span>
        <span class="meta-value">{{ pkg.latest_version || '-' }}</span>
      </div>
      <div class="meta-item">
        <el-icon><Download /></el-icon>
        <span class="meta-label">下载量</span>
        <span class="meta-value">{{ formatNumber(pkg.download_count || 0) }}</span>
      </div>
      <div class="meta-item">
        <el-icon><Clock /></el-icon>
        <span class="meta-label">更新于</span>
        <span class="meta-value">{{ formatRelativeTime(pkg.updated_at) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Download, PriceTag, Clock } from '@element-plus/icons-vue'
import type { Package } from '@/api/package'
import { formatNumber, formatRelativeTime } from '@/utils/format'
import { getPackageTypeColor, getPackageTypeLabel } from '@/constants/package'

defineProps<{
  pkg: Package
}>()

defineEmits<{
  click: []
}>()
</script>

<style scoped>
.package-card {
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 16px 20px;
  cursor: pointer;
  transition: all 0.2s ease;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 24px;
}

.package-card:hover {
  border-color: #0f172a;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.08);
}

.card-main {
  flex: 1;
  min-width: 0;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

.type-tag {
  min-width: 56px;
  text-align: center;
}

.package-name {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
  letter-spacing: -0.3px;
}

.package-desc {
  color: #64748b;
  font-size: 13px;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.5;
}

.card-footer {
  display: flex;
  gap: 20px;
  align-items: center;
  flex-shrink: 0;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.meta-label {
  font-size: 11px;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}

.meta-value {
  font-size: 13px;
  color: #0f172a;
  font-weight: 600;
}

.meta-item .el-icon {
  font-size: 14px;
  color: #94a3b8;
}
</style>
