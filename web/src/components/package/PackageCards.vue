<template>
  <div class="package-cards" v-loading="loading">
    <div class="card-grid">
      <div
        v-for="pkg in packages"
        :key="pkg.id"
        class="package-card"
      >
        <div class="card-header">
          <div class="card-title">
            <div class="package-name" @click="$emit('view-detail', pkg)">
              {{ pkg.display_name || pkg.name }}
            </div>
            <div class="package-version">{{ pkg.latest_version || 'N/A' }}</div>
          </div>
          <div class="card-tags">
            <el-tag :type="getPackageTypeColor(pkg.type)" size="small">
              {{ pkg.type }}
            </el-tag>
            <el-tag :type="pkg.repository_type === 'proxy' ? 'warning' : 'success'" size="small">
              {{ pkg.repository_type === 'proxy' ? '代理' : '本地' }}
            </el-tag>
          </div>
        </div>

        <div class="card-body">
          {{ pkg.description || '暂无描述' }}
        </div>

        <div class="card-stats">
          <div><strong>{{ pkg.versions_count || 0 }}</strong> 版本</div>
          <div><strong>{{ formatNumber(pkg.download_count) }}</strong> 下载</div>
          <div>更新于 {{ formatDate(pkg.updated_at) }}</div>
        </div>

        <div class="card-actions">
          <el-button size="small" @click="$emit('view-versions', pkg)">
            查看版本
          </el-button>
          <el-button size="small" type="primary" @click="$emit('view-detail', pkg)">
            查看详情
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Package } from '@/api/package'
import { formatNumber, formatDate } from '@/utils/format'
import { getPackageTypeColor } from '@/constants/package'

defineProps<{
  packages: Package[]
  loading: boolean
}>()

defineEmits<{
  'view-versions': [pkg: Package]
  'view-detail': [pkg: Package]
}>()
</script>

<style scoped>
.package-cards {
  width: 100%;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.package-card {
  background: white;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 16px;
  transition: all 0.3s;
}

.package-card:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: start;
  margin-bottom: 12px;
}

.card-title {
  flex: 1;
}

.card-tags {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.package-name {
  font-size: 18px;
  font-weight: 600;
  color: #409eff;
  cursor: pointer;
  margin-bottom: 4px;
}

.package-name:hover {
  text-decoration: underline;
}

.package-version {
  font-size: 12px;
  color: #909399;
}

.card-body {
  font-size: 14px;
  color: #606266;
  margin-bottom: 16px;
  line-height: 1.6;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.card-stats {
  display: flex;
  gap: 24px;
  margin-bottom: 16px;
  font-size: 13px;
  color: #909399;
}

.card-stats strong {
  color: #606266;
}

.card-actions {
  display: flex;
  gap: 8px;
}

.card-actions .el-button {
  flex: 1;
}
</style>
