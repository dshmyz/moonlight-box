<template>
  <div class="package-header-section">
    <div class="header-top">
      <el-breadcrumb separator="/">
        <el-breadcrumb-item>
          <router-link to="/">浏览仓库</router-link>
        </el-breadcrumb-item>
        <el-breadcrumb-item>
          <router-link to="/">
            <el-tag size="small" :type="getPackageTypeColor(pkg.type)">{{ getPackageTypeLabel(pkg.type) }}</el-tag>
          </router-link>
        </el-breadcrumb-item>
        <el-breadcrumb-item>{{ pkg.display_name || pkg.name }}</el-breadcrumb-item>
      </el-breadcrumb>
    </div>
    <div class="header-body">
      <el-tag :type="getPackageTypeColor(pkg.type)" size="large" effect="plain">
        {{ getPackageTypeLabel(pkg.type) }}
      </el-tag>
      <h1 class="package-title">{{ pkg.display_name || pkg.name }}</h1>
      <p class="package-description">{{ pkg.description || '暂无描述' }}</p>
      <div class="header-meta">
        <span class="meta-item">
          <el-icon><PriceTag /></el-icon>
          最新版本: {{ pkg.latest_version || '-' }}
        </span>
        <span class="meta-item">
          <el-icon><Download /></el-icon>
          总下载: {{ formatNumber(pkg.download_count || 0) }}
        </span>
        <span v-if="pkg.license" class="meta-item">
          <el-icon><Document /></el-icon>
          {{ pkg.license }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { PriceTag, Download, Document } from '@element-plus/icons-vue'
import { formatNumber } from '@/utils/format'
import { getPackageTypeColor, getPackageTypeLabel } from '@/constants/package'

defineProps<{
  pkg: {
    name: string
    display_name?: string
    type: string
    description?: string
    latest_version?: string
    download_count?: number
    license?: string
  }
}>()
</script>

<style scoped>
.package-header-section {
  margin-bottom: 24px;
}

.header-top {
  margin-bottom: 16px;
}

.header-top :deep(.el-breadcrumb__inner a) {
  color: #606266;
  text-decoration: none;
}

.header-top :deep(.el-breadcrumb__inner a:hover) {
  color: #409eff;
}

.header-body {
  background: #ffffff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 24px;
}

.package-title {
  font-size: 24px;
  font-weight: 700;
  color: #303133;
  margin: 12px 0 8px;
}

.package-description {
  color: #606266;
  font-size: 14px;
  margin: 0 0 16px;
  line-height: 1.6;
}

.header-meta {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #909399;
}

.meta-item .el-icon {
  font-size: 14px;
}
</style>
