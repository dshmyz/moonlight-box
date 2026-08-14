<template>
  <div class="package-grid">
    <article
      v-for="pkg in packages"
      :key="pkg.id"
      class="package-card"
      @click="$emit('view-detail', pkg)"
    >
      <header class="card-head">
        <h3 class="card-name">{{ pkg.display_name || pkg.name }}</h3>
        <button
          class="copy-name-btn"
          title="复制包标识"
          @click.stop="copyName(pkg)"
        >
          <el-icon><CopyDocument /></el-icon>
        </button>
        <span class="card-type-badge" :style="{ '--dot-color': getTypeColor(pkg.type) }">
          <span class="type-dot" />
          {{ pkg.type }}
        </span>
      </header>

      <div class="card-meta">
        <span v-if="pkg.latest_version" class="meta-version">{{ formatVersion(pkg.latest_version) }}</span>
        <span v-if="pkg.latest_version" class="meta-sep">·</span>
        <span class="meta-item">
          <el-icon><Download /></el-icon>
          <span class="meta-num">{{ formatNumber(pkg.download_count) }}</span>
        </span>
        <span class="meta-sep">·</span>
        <span class="meta-item">
          <el-icon><Clock /></el-icon>
          <span class="meta-date">{{ formatDate(pkg.updated_at) }}</span>
        </span>
      </div>

      <p class="card-desc">{{ pkg.description || '暂无描述' }}</p>

      <div v-if="displayRepos(pkg).length > 0" class="card-repos">
        <span
          v-for="repo in displayRepos(pkg)"
          :key="repo"
          class="repo-chip"
        >
          {{ repo }}
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
    </article>
  </div>
</template>

<script setup lang="ts">
import { CopyDocument, Download, Clock } from '@element-plus/icons-vue'
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

// 卡片只展示实际所在仓库；组合仓库仅是访问入口（见详情页配置命令）
const displayRepos = (pkg: Package) => pkg.repositories?.length ? pkg.repositories : (pkg.repository_name ? [pkg.repository_name] : [])
function formatVersion(v: string): string {
  if (!v) return ''
  return v.startsWith('v') ? v : `v${v}`
}
function copyName(pkg: Package) {
  copyToClipboard(`${pkg.type}:${pkg.name}`)
}
</script>

<style scoped>
.package-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}
.package-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 16px;
  background: var(--lunar-bg-card);
  border: 1px solid var(--lunar-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast), background var(--transition-fast);
  min-height: 132px;
  box-sizing: border-box;
}
.package-card:hover {
  border-color: var(--lunar-border-hover);
  background: var(--lunar-bg-card-hover);
  box-shadow: var(--lunar-shadow-card);
}

/* Row 1: name + copy + type badge. Aligns by baseline via flex. */
.card-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 22px;
}
.card-name {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-family: var(--font-family-mono);
  font-size: 14px;
  font-weight: 600;
  line-height: 1.3;
  letter-spacing: -0.01em;
  color: var(--lunar-silver);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: color var(--transition-fast);
}
.package-card:hover .card-name {
  color: var(--lunar-accent);
}
.copy-name-btn {
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--lunar-silver-dim);
  cursor: pointer;
  opacity: 0;
  transition: opacity var(--transition-fast);
  border-radius: var(--radius-sm);
}
.copy-name-btn:hover { color: var(--lunar-accent); background: var(--lunar-bg-glass); }
.package-card:hover .copy-name-btn { opacity: 1; }
.card-type-badge {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 20px;
  min-width: 48px;
  padding: 0 8px;
  font-size: 11px;
  font-weight: 500;
  font-family: var(--font-family-mono);
  letter-spacing: -0.01em;
  color: var(--lunar-accent);
  background: var(--lunar-bg-glass);
  border-radius: var(--radius-full);
  justify-content: center;
}
.type-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--dot-color, var(--lunar-accent));
  flex-shrink: 0;
}

/* Row 2: meta line, monospace + tabular nums for alignment */
.card-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 11.5px;
  color: var(--lunar-silver-muted);
  font-family: var(--font-family-mono);
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.01em;
  min-height: 16px;
}
.meta-version { color: var(--lunar-silver-muted); }
.meta-sep { color: var(--lunar-silver-dim); opacity: 0.6; }
.meta-item { display: inline-flex; align-items: center; gap: 4px; }
.meta-item .el-icon { font-size: 11px; color: var(--lunar-silver-dim); }
.meta-num, .meta-date { line-height: 1; }

/* Row 3: description */
.card-desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.45;
  color: var(--lunar-silver-dim);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  flex: 1;
}

/* 仓库 chips：展示实际所在仓库 */
.card-repos {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-height: 18px;
}
.repo-chip {
  display: inline-flex;
  align-items: center;
  max-width: 160px;
  padding: 1px 8px;
  font-size: 11px;
  font-family: var(--font-family-mono);
  color: var(--lunar-silver-muted);
  background: var(--lunar-bg-glass);
  border: 1px solid var(--lunar-border);
  border-radius: var(--radius-full);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-actions {
  display: flex;
  gap: 8px;
  margin-top: 2px;
  padding-top: 10px;
  border-top: 1px solid var(--lunar-border);
}
</style>
