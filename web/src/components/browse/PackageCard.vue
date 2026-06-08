<template>
  <div
    class="package-card"
    role="button"
    tabindex="0"
    @click="$emit('click')"
    @keydown.enter="$emit('click')"
    @keydown.space.prevent="$emit('click')"
  >
    <div class="card-inner" v-memo="[pkg.id, pkg.name, pkg.description, pkg.type, pkg.latest_version, pkg.download_count, pkg.updated_at, pkg.repository_name, pkg.license]">
      <div class="card-top">
        <span class="type-badge" :style="typeBadgeStyle">{{ getPackageTypeLabel(pkg.type) }}</span>
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
      <div v-if="pkg.repository_name || pkg.license" class="card-tags">
        <span v-if="pkg.repository_name" class="tag-item tag-repo">
          <el-icon><FolderOpened /></el-icon>
          {{ pkg.repository_name }}
        </span>
        <span v-if="pkg.license" class="tag-item tag-license">
          {{ pkg.license }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Download, PriceTag, Clock, FolderOpened } from '@element-plus/icons-vue'
import type { Package } from '@/api/package'
import { formatNumber, formatRelativeTime } from '@/utils/format'
import { getPackageTypeLabel, getPackageTypeHexColor, getPackageTypeHexColorRGB } from '@/constants/package'

const props = defineProps<{
  pkg: Package
}>()

defineEmits<{
  click: []
}>()

const typeBadgeStyle = computed(() => {
  const color = getPackageTypeHexColor(props.pkg.type)
  const rgb = getPackageTypeHexColorRGB(props.pkg.type)
  return {
    background: `rgba(${rgb}, 0.12)`,
    color: color,
    borderColor: `rgba(${rgb}, 0.25)`,
  }
})
</script>

<style scoped>
.package-card {
  position: relative;
  border-radius: 10px;
  cursor: pointer;
  overflow: hidden;
  transition: all 0.3s ease;
  outline: none;
}

.package-card:focus-visible {
  outline: 2px solid var(--lunar-accent);
  outline-offset: 2px;
}

.card-inner {
  padding: 20px 24px;
  background: var(--lunar-bg-card);
  border: 1px solid var(--lunar-border);
  border-radius: 10px;
  transition: border-color 0.3s ease, box-shadow 0.3s ease;
  transform: translateZ(0);
}

.package-card:hover .card-inner {
  border-color: var(--lunar-border-hover);
  box-shadow: var(--lunar-shadow-glow);
}

.card-top {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.type-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 700;
  border: 1px solid;
  letter-spacing: 0.5px;
}

.package-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--lunar-silver);
  letter-spacing: -0.2px;
}

.package-desc {
  color: var(--lunar-silver-muted);
  font-size: 13px;
  margin: 0 0 14px;
  line-height: 1.6;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
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
  color: var(--lunar-silver-dim);
}

.meta-item .el-icon {
  font-size: 14px;
  color: var(--lunar-accent-soft);
}

.card-tags {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  flex-wrap: wrap;
}

.tag-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.3px;
}

.tag-repo {
  background: rgba(100, 116, 139, 0.1);
  color: var(--lunar-silver-muted);
  border: 1px solid rgba(100, 116, 139, 0.2);
}

.tag-repo .el-icon {
  font-size: 11px;
}

.tag-license {
  background: rgba(34, 197, 94, 0.08);
  color: #4ade80;
  border: 1px solid rgba(34, 197, 94, 0.2);
}
</style>