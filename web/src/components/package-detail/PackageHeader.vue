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
      <div class="header-content">
        <div class="header-info">
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
        <div v-if="showDeleteButton" class="header-actions">
          <el-button type="danger" :icon="Delete" @click="handleDeleteClick">
            删除包
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { PriceTag, Download, Document, Delete } from '@element-plus/icons-vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { useRouter, useRoute } from 'vue-router'
import { formatNumber } from '@/utils/format'
import { getPackageTypeColor, getPackageTypeLabel } from '@/constants/package'
import { packageApi } from '@/api/package'

const props = defineProps<{
  pkg: {
    id: number
    name: string
    display_name?: string
    type: string
    description?: string
    latest_version?: string
    download_count?: number
    license?: string
  }
}>()

const emit = defineEmits<{
  'deleted': []
}>()

const router = useRouter()
const route = useRoute()

const showDeleteButton = computed(() => {
  return route.path.startsWith('/admin')
})

async function handleDeleteClick() {
  try {
    await ElMessageBox.confirm(
      `确定要删除包 "${props.pkg.display_name || props.pkg.name}" 及其所有版本吗？此操作不可恢复！`,
      '删除确认',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
      }
    )

    await packageApi.deletePackage(props.pkg.id)
    ElMessage.success('包已删除')
    emit('deleted')
    router.push('/admin/packages')
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除包失败')
      console.error('Failed to delete package:', error)
    }
  }
}
</script>

<style scoped>
.package-header-section {
  margin-bottom: 24px;
}

.header-top {
  margin-bottom: 16px;
}

.header-top :deep(.el-breadcrumb__inner a) {
  color: var(--lunar-silver-muted);
  text-decoration: none;
}

.header-top :deep(.el-breadcrumb__inner a:hover) {
  color: var(--lunar-accent);
}

.header-body {
  background: var(--lunar-bg-card);
  border: 1px solid var(--lunar-border);
  border-radius: 10px;
  padding: 24px;
  transform: translateZ(0);
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.header-info {
  flex: 1;
  min-width: 0;
}

.header-actions {
  flex-shrink: 0;
  margin-left: 24px;
}

.package-title {
  font-size: 24px;
  font-weight: 700;
  color: var(--lunar-silver);
  margin: 12px 0 8px;
}

.package-description {
  color: var(--lunar-silver-muted);
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
  color: var(--lunar-silver-dim);
}

.meta-item .el-icon {
  font-size: 14px;
}
</style>