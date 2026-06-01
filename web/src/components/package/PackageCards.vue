<template>
  <div class="package-cards" v-loading="loading">
    <div class="card-grid">
      <div
        v-for="pkg in packages"
        :key="pkg.id"
        class="package-card"
      >
        <div class="card-glass">
          <div class="card-header">
            <div class="card-icon"><i class="fa-solid fa-box"></i></div>
            <div class="card-title">
              <div class="package-name" @click="$emit('view-detail', pkg)">
                {{ pkg.display_name || formatPackageName(pkg.name, pkg.type) }}
              </div>
              <div class="package-version">{{ pkg.latest_version || 'N/A' }}</div>
            </div>
            <div class="card-tags">
              <el-tag :class="['type-tag', `type-tag--${pkg.type}`]" size="small">
                {{ pkg.type }}
              </el-tag>
              <el-tag :class="['source-tag', pkg.repository_type === 'proxy' ? 'source-tag--proxy' : 'source-tag--local']" size="small">
                {{ pkg.repository_type === 'proxy' ? '代理' : '本地' }}
              </el-tag>
            </div>
          </div>

          <div class="card-body">
            {{ pkg.description || '暂无描述' }}
          </div>

          <div class="card-stats">
            <div class="stat-item">
              <span class="stat-icon"><i class="fa-solid fa-chart-bar"></i></span>
              <span class="stat-value">{{ pkg.versions_count || 0 }}</span>
              <span class="stat-label">版本</span>
            </div>
            <div class="stat-item">
              <span class="stat-icon"><i class="fa-solid fa-download"></i></span>
              <span class="stat-value">{{ formatNumber(pkg.download_count) }}</span>
              <span class="stat-label">下载</span>
            </div>
            <div class="stat-item">
              <span class="stat-icon"><i class="fa-solid fa-clock"></i></span>
              <span class="stat-value">{{ formatDate(pkg.updated_at) }}</span>
              <span class="stat-label">更新</span>
            </div>
          </div>

          <div class="card-actions">
            <el-button class="btn-view-versions" size="small" @click="$emit('view-versions', pkg)">
              查看版本
            </el-button>
            <el-button class="btn-view-detail" size="small" type="primary" @click="$emit('view-detail', pkg)">
              查看详情
            </el-button>
            <el-button class="btn-delete" size="small" type="danger" @click="$emit('delete-package', pkg)">
              删除
            </el-button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Package } from '@/api/package'
import { formatNumber, formatDate } from '@/utils/format'
import { formatPackageName } from '@/constants/package'

defineProps<{
  packages: Package[]
  loading: boolean
}>()

defineEmits<{
  'view-versions': [pkg: Package]
  'view-detail': [pkg: Package]
  'delete-package': [pkg: Package]
}>()
</script>

<style scoped>
.package-cards {
  width: 100%;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 24px;
}

.package-card {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: transform;
  transform: translateZ(0);
}

.package-card:hover {
  transform: translateY(-4px) translateZ(0);
}

.card-glass {
  background: #ffffff;
  border-radius: 16px;
  padding: 24px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.04);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: box-shadow;
}

.package-card:hover .card-glass {
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.08);
  border-color: rgba(99, 102, 241, 0.15);
}

.card-header {
  display: flex;
  align-items: flex-start;
  margin-bottom: 16px;
  gap: 14px;
}

.card-icon {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
  border-radius: 12px;
  font-size: 16px;
  flex-shrink: 0;
}

.card-title {
  flex: 1;
  min-width: 0;
}

.card-tags {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}

.card-tags :deep(.el-tag) {
  border-radius: 6px;
  font-size: 11px;
  padding: 3px 10px;
  height: 22px;
  line-height: 18px;
  border: none;
}

.type-tag--npm {
  background: linear-gradient(135deg, #dbeafe 0%, #93c5fd 100%);
  color: #1d4ed8;
}

.type-tag--maven {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.type-tag--pypi {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.type-tag--go {
  background: linear-gradient(135deg, #d1fae5 0%, #6ee7b7 100%);
  color: #047857;
}

.source-tag--proxy {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.source-tag--local {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.package-name {
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-bottom: 4px;
  transition: color 0.2s ease;
}

.package-name:hover {
  color: #6366f1;
}

.package-version {
  font-size: 12px;
  color: #94a3b8;
}

.card-body {
  font-size: 13px;
  color: #64748b;
  margin-bottom: 20px;
  line-height: 1.6;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.card-stats {
  display: flex;
  gap: 16px;
  margin-bottom: 20px;
  padding: 14px;
  background: #f8fafc;
  border-radius: 10px;
}

.stat-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.stat-icon {
  font-size: 14px;
}

.stat-value {
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
}

.stat-label {
  font-size: 11px;
  color: #94a3b8;
}

.card-actions {
  display: flex;
  gap: 10px;
}

.card-actions .el-button {
  flex: 1;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 500;
  padding: 10px 16px;
}

.btn-view-versions {
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  color: #64748b;
  transition: all 0.2s ease;
}

.btn-view-versions:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
  color: #475569;
}

.btn-view-detail {
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border: none;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
  transition: all 0.2s ease;
}

.btn-view-detail:hover {
  transform: translateY(-2px) translateZ(0);
  box-shadow: 0 6px 16px rgba(99, 102, 241, 0.4);
}

.btn-delete {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  border: none;
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.3);
  transition: all 0.2s ease;
}

.btn-delete:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(239, 68, 68, 0.4);
}
</style>
