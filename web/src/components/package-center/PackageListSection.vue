<template>
  <section class="package-section">
    <div class="section-header">
      <h2 class="section-title">软件包列表</h2>
      <div class="section-tabs">
        <button
          v-for="tab in tabs"
          :key="tab.value"
          class="section-tab"
          :class="{ 'section-tab--active': activeTab === tab.value }"
          @click="$emit('tab-change', tab.value)"
        >
          {{ tab.label }}
        </button>
      </div>
    </div>

    <div class="filter-bar">
      <div class="filter-chips">
        <span
          class="filter-chip"
          :class="{ 'filter-chip--active': activeFilter === '' }"
          @click="$emit('filter', '')"
        >
          全部类型
        </span>
        <span
          v-for="type in packageTypes"
          :key="type.value"
          class="filter-chip"
          :class="{ 'filter-chip--active': activeFilter === type.value }"
          @click="$emit('filter', type.value)"
        >
          {{ type.label }}
        </span>
      </div>

      <div class="filter-divider"></div>

      <div class="sort-select">
        <label>排序：</label>
        <select :value="sortBy" @change="$emit('sort', ($event.target as HTMLSelectElement).value)">
          <option value="updated_at">最近更新</option>
          <option value="download_count">下载量</option>
          <option value="name">名称</option>
        </select>
      </div>
    </div>

    <div v-loading="loading" class="package-list">
      <div
        v-for="pkg in packages"
        :key="pkg.id"
        class="package-item"
        @click="$emit('view-detail', pkg)"
      >
        <div class="package-icon" :class="`package-icon--${pkg.type}`">
          <i :class="getPackageIcon(pkg.type)"></i>
        </div>
        <div class="package-info">
          <div class="package-name">
            {{ pkg.display_name || pkg.name }}
            <span v-if="pkg.latest_version" class="badge badge--latest">v{{ pkg.latest_version }}</span>
          </div>
          <div class="package-desc">{{ pkg.description || '暂无描述' }}</div>
          <div class="package-meta">
            <span class="meta-tag">
              <i class="fa-solid fa-code-branch"></i>
              {{ getPackageTypeLabel(pkg.type) }}
            </span>
            <span v-if="pkg.license" class="meta-tag">
              <i class="fa-solid fa-scale-balanced"></i>
              {{ pkg.license }}
            </span>
            <span class="meta-tag">
              <i class="fa-solid fa-clock"></i>
              {{ formatRelativeTime(pkg.updated_at) }}
            </span>
            <span v-if="pkg.repository_type === 'proxy'" class="meta-tag">
              <i class="fa-solid fa-globe"></i>
              代理
            </span>
            <span v-else class="meta-tag">
              <i class="fa-solid fa-building"></i>
              本地
            </span>
          </div>
        </div>
        <div class="package-stats">
          <div class="stat-small">
            <span>下载</span>
            <span class="value">{{ formatNumber(pkg.download_count) }}</span>
          </div>
          <div class="stat-small">
            <span>版本</span>
            <span class="value">{{ pkg.versions_count || 0 }}</span>
          </div>
        </div>
        <div class="package-actions" @click.stop>
          <button class="btn-icon" title="查看版本" @click="$emit('view-versions', pkg)">
            <i class="fa-solid fa-clock-rotate-left"></i>
          </button>
          <button class="btn-icon" title="查看详情" @click="$emit('view-detail', pkg)">
            <i class="fa-solid fa-eye"></i>
          </button>
          <button class="btn-icon btn-icon--danger" title="删除" @click="$emit('delete-package', pkg)">
            <i class="fa-solid fa-trash"></i>
          </button>
        </div>
      </div>

      <el-empty v-if="!loading && packages.length === 0" description="暂无数据" />
    </div>

    <div v-if="total > 0" class="pagination">
      <div class="pagination-info">
        显示 {{ (currentPage - 1) * pageSize + 1 }}-{{ Math.min(currentPage * pageSize, total) }} / 共 {{ total }} 个包
      </div>
      <el-pagination
        v-model:current-page="currentPageModel"
        :page-size="pageSize"
        :total="total"
        :page-sizes="[20, 50, 100]"
        layout="sizes, prev, pager, next"
        @current-change="$emit('page-change', $event)"
        @size-change="$emit('size-change', $event)"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Package } from '@/api/package'
import { formatNumber, formatRelativeTime } from '@/utils/format'
import { getPackageTypeLabel } from '@/constants/package'

const props = defineProps<{
  packages: Package[]
  loading: boolean
  total: number
  currentPage: number
  pageSize: number
  activeFilter: string
  activeTab: string
  sortBy: string
}>()

defineEmits<{
  search: [query: string]
  filter: [type: string]
  sort: [sort: string]
  'tab-change': [tab: string]
  'page-change': [page: number]
  'size-change': [size: number]
  'view-detail': [pkg: Package]
  'view-versions': [pkg: Package]
  'delete-package': [pkg: Package]
}>()

const currentPageModel = ref(props.currentPage)

watch(() => props.currentPage, (val) => {
  currentPageModel.value = val
})

const tabs = [
  { value: 'all', label: '全部' },
  { value: 'recent', label: '最近使用' },
]

const packageTypes = [
  { value: 'npm', label: 'npm' },
  { value: 'maven', label: 'Maven' },
  { value: 'pypi', label: 'PyPI' },
  { value: 'go', label: 'Go' },
  { value: 'yum', label: 'Yum' },
  { value: 'apt', label: 'Apt' },
  { value: 'generic', label: 'Generic' },
]

function getPackageIcon(type: string): string {
  const icons: Record<string, string> = {
    npm: 'fa-brands fa-npm',
    maven: 'fa-solid fa-java',
    pypi: 'fa-brands fa-python',
    go: 'fa-solid fa-cubes',
    yum: 'fa-solid fa-box',
    apt: 'fa-solid fa-box',
    generic: 'fa-solid fa-file-archive',
  }
  return icons[type] || icons.generic
}
</script>

<style scoped>
.package-section {
  background: var(--color-bg-primary, #ffffff);
  border-radius: 16px;
  border: 1px solid var(--color-border, #e2e8f0);
  overflow: hidden;
}

.section-header {
  padding: 20px 24px;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.section-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--color-text-primary, #0f172a);
}

.section-tabs {
  display: flex;
  gap: 4px;
  background: var(--color-bg-page, #f8fafc);
  padding: 4px;
  border-radius: 10px;
}

.section-tab {
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-tertiary, #94a3b8);
  cursor: pointer;
  transition: all 0.2s;
  border: none;
  background: transparent;
}

.section-tab:hover {
  color: var(--color-text-secondary, #475569);
}

.section-tab--active {
  background: white;
  color: var(--color-primary, #6366f1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.filter-bar {
  padding: 16px 24px;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.filter-chips {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.filter-chip {
  padding: 6px 14px;
  border-radius: 20px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid var(--color-border, #e2e8f0);
  background: white;
  color: var(--color-text-secondary, #475569);
}

.filter-chip:hover {
  border-color: var(--color-primary-light, #818cf8);
  color: var(--color-primary, #6366f1);
  background: #f0f1ff;
}

.filter-chip--active {
  background: linear-gradient(135deg, var(--color-primary, #6366f1) 0%, #4f46e5 100%);
  color: white;
  border-color: transparent;
}

.filter-divider {
  width: 1px;
  height: 24px;
  background: var(--color-border, #e2e8f0);
  margin: 0 4px;
}

.sort-select {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}

.sort-select label {
  font-size: 13px;
  color: var(--color-text-tertiary, #94a3b8);
}

.sort-select select {
  padding: 8px 12px;
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 8px;
  font-size: 13px;
  color: var(--color-text-secondary, #475569);
  background: white;
  cursor: pointer;
  outline: none;
}

.sort-select select:focus {
  border-color: var(--color-primary, #6366f1);
}

.package-list {
  min-height: 200px;
}

.package-item {
  padding: 20px 24px;
  border-bottom: 1px solid var(--color-border-light, #f3f4f6);
  display: flex;
  align-items: center;
  gap: 20px;
  transition: all 0.2s;
  cursor: pointer;
}

.package-item:hover {
  background: var(--color-bg-hover, #f8fafc);
}

.package-item:last-child {
  border-bottom: none;
}

.package-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
}

.package-icon--npm {
  background: linear-gradient(135deg, #dbeafe 0%, #93c5fd 100%);
  color: #1d4ed8;
}

.package-icon--maven {
  background: linear-gradient(135deg, #dcfce7 0%, #86efac 100%);
  color: #16a34a;
}

.package-icon--pypi {
  background: linear-gradient(135deg, #fef3c7 0%, #fcd34d 100%);
  color: #d97706;
}

.package-icon--go {
  background: linear-gradient(135deg, #ccfbf1 0%, #5eead4 100%);
  color: #0d9488;
}

.package-icon--yum,
.package-icon--apt {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #dc2626;
}

.package-icon--generic {
  background: linear-gradient(135deg, #f3e8ff 0%, #c084fc 100%);
  color: #7c3aed;
}

.package-info {
  flex: 1;
  min-width: 0;
}

.package-name {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary, #0f172a);
  margin-bottom: 4px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.badge {
  font-size: 10px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 600;
}

.badge--latest {
  background: #d1fae5;
  color: #059669;
}

.package-desc {
  font-size: 13px;
  color: var(--color-text-tertiary, #94a3b8);
  margin-bottom: 8px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.package-meta {
  display: flex;
  gap: 16px;
}

.meta-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--color-text-tertiary, #94a3b8);
}

.meta-tag i {
  font-size: 11px;
}

.package-stats {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 8px;
}

.stat-small {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--color-text-tertiary, #94a3b8);
}

.stat-small .value {
  font-weight: 600;
  color: var(--color-text-secondary, #475569);
}

.package-actions {
  display: flex;
  gap: 8px;
}

.btn-icon {
  width: 36px;
  height: 36px;
  padding: 0;
  border-radius: 8px;
  border: 1px solid var(--color-border, #e2e8f0);
  background: white;
  color: var(--color-text-tertiary, #94a3b8);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-icon:hover {
  border-color: var(--color-primary-light, #818cf8);
  color: var(--color-primary, #6366f1);
  background: #f0f1ff;
}

.btn-icon--danger:hover {
  border-color: #fca5a5;
  color: #ef4444;
  background: #fef2f2;
}

.pagination {
  padding: 20px 24px;
  border-top: 1px solid var(--color-border, #e2e8f0);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.pagination-info {
  font-size: 13px;
  color: var(--color-text-tertiary, #94a3b8);
}

@media (max-width: 768px) {
  .package-item {
    flex-wrap: wrap;
  }

  .package-stats {
    flex-direction: row;
    gap: 16px;
    width: 100%;
    justify-content: flex-start;
  }

  .package-actions {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
