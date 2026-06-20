<template>
  <div class="package-grid">
    <div
      v-for="pkg in packages"
      :key="pkg.id"
      class="package-card"
      @click="$emit('view-detail', pkg)"
    >
      <div class="card-header">
        <div class="card-icon" :style="{ background: getTypeBg(pkg.type) }">
          <i class="fa-solid fa-box"></i>
        </div>
        <div class="card-title">
          <div class="card-name-row">
            <span class="card-name">{{ pkg.display_name || pkg.name }}</span>
            <el-button
              link
              class="copy-name-btn"
              @click.stop="copyName(pkg)"
            >
              <el-icon><CopyDocument /></el-icon>
            </el-button>
          </div>
          <el-tag size="small" effect="light" :style="{ color: getTypeColor(pkg.type) }">
            {{ pkg.type }}
          </el-tag>
        </div>
      </div>

      <p class="card-desc">{{ pkg.description || '暂无描述' }}</p>

      <div class="card-meta">
        <span v-if="pkg.latest_version" class="meta-item">
          <el-icon><PriceTag /></el-icon>
          {{ pkg.latest_version }}
        </span>
        <span class="meta-item">
          <el-icon><Download /></el-icon>
          {{ formatNumber(pkg.download_count) }}
        </span>
        <span class="meta-item">
          <el-icon><Clock /></el-icon>
          {{ formatDate(pkg.updated_at) }}
        </span>
      </div>

      <div v-if="mode === 'admin'" class="card-actions" @click.stop>
        <el-button class="btn-view-versions" size="small" @click="$emit('view-versions', pkg)">
          查看版本
        </el-button>
        <el-button
          v-permission="'package:delete'"
          class="btn-delete"
          size="small"
          type="danger"
          plain
          @click="$emit('delete-package', pkg)"
        >
          删除
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { CopyDocument, PriceTag, Download, Clock } from '@element-plus/icons-vue'
import type { Package } from '@/api/package'
import { formatNumber, formatDate } from '@/utils/format'
import { copyToClipboard } from '@/utils/clipboard'
import { PACKAGE_TYPE_HEX_COLORS } from '@/constants/package'

defineProps<{
  packages: Package[]
  mode: 'admin' | 'public'
}>()

defineEmits<{
  'view-detail': [pkg: Package]
  'view-versions': [pkg: Package]
  'delete-package': [pkg: Package]
}>()

function getTypeColor(type: string): string {
  return PACKAGE_TYPE_HEX_COLORS[type] || PACKAGE_TYPE_HEX_COLORS.generic
}
function getTypeBg(type: string): string {
  return `${getTypeColor(type)}1a`
}
function copyName(pkg: Package) {
  copyToClipboard(`${pkg.type}:${pkg.name}`)
}
</script>

<style scoped>
.package-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}
.package-card {
  padding: 20px;
  background: #fff;
  border-radius: 12px;
  border: 1px solid #f1f5f9;
  cursor: pointer;
  transition: all 0.2s ease;
}
.package-card:hover {
  border-color: #c7d2fe;
  box-shadow: 0 4px 16px rgba(99, 102, 241, 0.08);
  transform: translateY(-2px);
}
.card-header { display: flex; align-items: flex-start; gap: 12px; margin-bottom: 12px; }
.card-icon {
  width: 40px; height: 40px; border-radius: 10px;
  display: flex; align-items: center; justify-content: center;
  font-size: 16px; flex-shrink: 0;
}
.card-title { flex: 1; min-width: 0; }
.card-name-row { display: flex; align-items: center; gap: 4px; margin-bottom: 6px; }
.card-name { font-size: 15px; font-weight: 600; color: #1e293b; }
.copy-name-btn { opacity: 0; transition: opacity 0.2s; padding: 2px; }
.package-card:hover .copy-name-btn { opacity: 1; }
.card-desc {
  font-size: 13px; color: #64748b; margin: 0 0 12px;
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical;
  overflow: hidden; min-height: 38px;
}
.card-meta { display: flex; flex-wrap: wrap; gap: 12px; font-size: 12px; color: #94a3b8; }
.meta-item { display: inline-flex; align-items: center; gap: 4px; }
.card-actions { display: flex; gap: 8px; margin-top: 12px; padding-top: 12px; border-top: 1px solid #f1f5f9; }
</style>
