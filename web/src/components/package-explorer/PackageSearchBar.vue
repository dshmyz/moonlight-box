<template>
  <div class="package-search-bar">
    <!-- 搜索框 -->
    <div class="search-wrapper">
      <el-input
        ref="searchInputRef"
        v-model="localQuery"
        class="search-input"
        placeholder="搜索包名、描述或标签（按 / 聚焦）"
        clearable
        @input="onInput"
        @clear="onClear"
        @keyup.enter="onEnter"
        @focus="showRecent = true"
        @blur="onBlur"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
        <template v-if="loading" #suffix>
          <el-icon class="is-loading"><Loading /></el-icon>
        </template>
      </el-input>

      <div v-if="showRecent && recentSearches.length > 0" class="recent-dropdown">
        <div class="recent-header">
          <span>最近搜索</span>
          <el-button link size="small" @click="$emit('clear-recent')">清空历史</el-button>
        </div>
        <div
          v-for="term in recentSearches"
          :key="term"
          class="recent-item"
          @mousedown.prevent="onRecentClick(term)"
        >
          <el-icon><Clock /></el-icon>
          <span class="recent-term">{{ term }}</span>
        </div>
      </div>
    </div>

    <!-- 右侧操作（跨两行） -->
    <div class="primary-actions">
      <el-select
        :model-value="query.sort"
        class="sort-select"
        @change="(v: string) => emitUpdate({ sort: v as any })"
      >
        <el-option label="更新时间" value="updated_at" />
        <el-option label="下载量" value="download_count" />
        <el-option label="名称" value="name" />
      </el-select>

      <div class="view-toggle">
        <el-button :class="{ active: viewMode === 'table' }" @click="$emit('update:viewMode', 'table')" title="表格视图">
          <el-icon><List /></el-icon>
        </el-button>
        <el-button :class="{ active: viewMode === 'grid' }" @click="$emit('update:viewMode', 'grid')" title="网格视图">
          <el-icon><Grid /></el-icon>
        </el-button>
      </div>

      <el-badge :is-dot="hasActiveFilter">
        <el-button class="filter-btn" @click="$emit('open-filter')">
          <el-icon><Filter /></el-icon>高级查询
        </el-button>
      </el-badge>
    </div>

    <!-- chips -->
    <div class="type-chips">
      <button
        v-for="opt in allTypes"
        :key="opt.value"
        class="type-chip"
        :class="{ 'type-chip--active': (query.type === 'all' ? '' : query.type) === opt.value }"
        :style="getChipStyle(opt.value)"
        @click="onTypeClick(opt.value)"
      >
        <span v-if="opt.value" class="type-dot" :style="{ background: getDotColor(opt.value) }"></span>
        <span class="chip-label">{{ opt.label }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Search, Loading, Clock, Filter, List, Grid } from '@element-plus/icons-vue'
import { PACKAGE_TYPE_OPTIONS, PACKAGE_TYPE_HEX_COLORS } from '@/constants/package'
import type { PackageQuery } from '@/composables/usePackageSearch'

const props = defineProps<{
  query: PackageQuery
  recentSearches: string[]
  loading: boolean
  hasActiveFilter: boolean
  viewMode: 'table' | 'grid'
}>()

const emit = defineEmits<{
  'update:query': [patch: Partial<PackageQuery>]
  'update:viewMode': [mode: 'table' | 'grid']
  search: []
  'add-recent': [term: string]
  'clear-recent': []
  'open-filter': []
}>()

const searchInputRef = ref()
const localQuery = ref(props.query.q)
const showRecent = ref(false)

watch(() => props.query.q, (v) => {
  if (v !== localQuery.value) localQuery.value = v
})

const allTypes = computed(() => [
  { value: '', label: '全部' },
  ...PACKAGE_TYPE_OPTIONS.map(o => ({ value: o.value, label: o.label })),
])

function getDotColor(type: string): string {
  return PACKAGE_TYPE_HEX_COLORS[type] || PACKAGE_TYPE_HEX_COLORS.generic
}
function getChipStyle(type: string) {
  const isActive = (props.query.type === 'all' ? '' : props.query.type) === type
  if (!isActive || !type) return {}
  const color = getDotColor(type)
  return { borderColor: color, color, background: `${color}1a` }
}

let debounceTimer: ReturnType<typeof setTimeout> | null = null

function emitUpdate(patch: Partial<PackageQuery>) {
  emit('update:query', patch)
  emit('search')
}

function onInput() {
  emit('update:query', { q: localQuery.value })
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    emit('search')
  }, 500)
}

function onEnter() {
  if (debounceTimer) clearTimeout(debounceTimer)
  emit('update:query', { q: localQuery.value })
  emit('search')
  const term = localQuery.value.trim()
  if (term) emit('add-recent', term)
}

function onClear() {
  localQuery.value = ''
  if (debounceTimer) clearTimeout(debounceTimer)
  emit('update:query', { q: '' })
  emit('search')
}

function onTypeClick(value: string) {
  emit('update:query', { type: value || 'all', page: 1 })
  emit('search')
}

function onRecentClick(term: string) {
  localQuery.value = term
  if (debounceTimer) clearTimeout(debounceTimer)
  emit('update:query', { q: term, page: 1 })
  emit('search')
  showRecent.value = false
}

function onBlur() {
  setTimeout(() => { showRecent.value = false }, 200)
}

defineExpose({
  focus: () => searchInputRef.value?.focus?.(),
})
</script>

<style scoped>
.package-search-bar {
  padding: 14px 0 12px;
  display: grid;
  grid-template-columns: 1fr auto;
  grid-template-rows: auto auto;
  align-items: center;
  gap: 8px 12px;
}

/* 搜索框占第一行左侧 */
.search-wrapper {
  position: relative;
  grid-column: 1;
  grid-row: 1;
}

/* 右侧操作区占第一行右侧 */
.primary-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  grid-column: 2;
  grid-row: 1;
  align-self: center;
}

/* chips 占第二行，跨满整行 */
.type-chips {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  grid-column: 1 / 3;
  grid-row: 2;
}

.search-input { width: 100%; }
.recent-dropdown {
  position: absolute; top: 100%; left: 0; right: 0; margin-top: 4px;
  background: var(--lunar-bg-surface); border: 1px solid var(--lunar-border);
  border-radius: var(--radius-md);
  box-shadow: var(--lunar-shadow-elevated); z-index: 100; padding: 8px;
  min-width: 280px;
}
.recent-header { display: flex; justify-content: space-between; align-items: center; padding: 4px 12px; font-size: 12px; color: var(--lunar-silver-muted); }
.recent-item { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: var(--radius-sm); cursor: pointer; color: var(--lunar-silver-muted); font-size: 13px; }
.recent-item:hover { background: var(--lunar-bg-glass); }
.recent-term { font-family: var(--font-family-mono); }

.sort-select { width: 124px; }
.view-toggle {
  display: flex;
  border: 1px solid var(--lunar-border);
  border-radius: var(--radius-md);
  overflow: hidden;
  height: 32px;
}
.view-toggle .el-button {
  border: none;
  border-radius: 0;
  padding: 0 10px;
  height: 32px;
}
.view-toggle .el-button + .el-button { border-left: 1px solid var(--lunar-border); }
.view-toggle .el-button.active {
  background: var(--lunar-accent);
  color: #fff;
  border-color: var(--lunar-accent);
}

.type-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 12px;
  border: 1px solid var(--lunar-border);
  border-radius: var(--radius-full);
  background: transparent;
  color: var(--lunar-silver-muted);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
  white-space: nowrap;
}
.type-chip:hover {
  border-color: var(--lunar-border-hover);
  background: var(--lunar-bg-glass);
}
.type-chip--active { font-weight: 600; }
.type-dot { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0; }
.chip-label { font-family: var(--font-family-mono); letter-spacing: -0.01em; }
</style>
