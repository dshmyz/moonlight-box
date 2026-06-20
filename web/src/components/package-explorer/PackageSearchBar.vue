<template>
  <div class="package-search-bar">
    <div class="search-wrapper">
      <el-input
        ref="searchInputRef"
        v-model="localQuery"
        class="search-input"
        placeholder="搜索包名、描述或标签（按 / 聚焦）"
        clearable
        @input="onInput"
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
        <template #append>
          <el-button class="clear-btn" @click="onClear">
            <el-icon><Close /></el-icon>
          </el-button>
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
          <span>{{ term }}</span>
        </div>
      </div>
    </div>

    <div class="type-chips">
      <button
        v-for="opt in visibleTypes"
        :key="opt.value"
        class="type-chip"
        :class="{ 'type-chip--active': (query.type === 'all' ? '' : query.type) === opt.value }"
        :style="getChipStyle(opt.value)"
        @click="onTypeClick(opt.value)"
      >
        <span v-if="opt.value" class="type-dot" :style="{ background: getDotColor(opt.value) }"></span>
        {{ opt.label }}
      </button>
      <el-dropdown v-if="hiddenTypes.length > 0" trigger="click" @command="onTypeClick">
        <button class="type-chip type-chip--more">
          更多<el-icon><ArrowDown /></el-icon>
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="opt in hiddenTypes" :key="opt.value" :command="opt.value">
              {{ opt.label }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>

      <div class="toolbar-actions">
        <el-badge :is-dot="hasActiveFilter" class="filter-badge">
          <el-button @click="$emit('open-filter')">
            <el-icon><Filter /></el-icon>筛选
          </el-button>
        </el-badge>

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
          <el-button :class="{ active: viewMode === 'table' }" @click="$emit('update:viewMode', 'table')">
            <el-icon><List /></el-icon>
          </el-button>
          <el-button :class="{ active: viewMode === 'grid' }" @click="$emit('update:viewMode', 'grid')">
            <el-icon><Grid /></el-icon>
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Search, Loading, Close, Clock, ArrowDown, Filter, List, Grid } from '@element-plus/icons-vue'
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
const visibleTypes = computed(() => allTypes.value.slice(0, 6))
const hiddenTypes = computed(() => allTypes.value.slice(6))

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
  // 状态同步立即触发，仅 search 触发 debounced
  emit('update:query', { q: localQuery.value })
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    emit('search')
  }, 300)
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
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 16px 20px;
  background: #fff;
  border-radius: 12px;
  border: 1px solid rgba(0, 0, 0, 0.04);
}
.search-wrapper { position: relative; flex: 1; min-width: 240px; }
.search-input { width: 100%; }
.recent-dropdown {
  position: absolute; top: 100%; left: 0; right: 0; margin-top: 4px;
  background: #fff; border: 1px solid #e2e8f0; border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08); z-index: 100; padding: 8px;
}
.recent-header { display: flex; justify-content: space-between; align-items: center; padding: 4px 12px; font-size: 12px; color: #94a3b8; }
.recent-item { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: 6px; cursor: pointer; color: #475569; font-size: 13px; }
.recent-item:hover { background: #f1f5f9; }
.type-chips { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.type-chip {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 7px 14px; border: 1px solid #e2e8f0; border-radius: 8px;
  background: #f8fafc; color: #64748b; font-size: 13px; cursor: pointer;
  transition: all 0.2s ease;
}
.type-chip:hover { border-color: #cbd5e1; background: #fff; }
.type-chip--active { font-weight: 600; }
.type-dot { width: 8px; height: 8px; border-radius: 50%; }
.toolbar-actions { display: flex; align-items: center; gap: 12px; margin-left: auto; }
.sort-select { width: 130px; }
.view-toggle { display: flex; border: 1px solid #e2e8f0; border-radius: 8px; overflow: hidden; }
.view-toggle .el-button { border: none; border-radius: 0; }
.view-toggle .el-button + .el-button { border-left: 1px solid #e2e8f0; }
.view-toggle .el-button.active { background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%); color: #fff; }
</style>
