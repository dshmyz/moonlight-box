<template>
  <div class="browse-page" @keydown="handleKeydown">
    <HeroSection
      ref="heroRef"
      v-model:search-query="searchQuery"
      v-model:selected-type="selectedType"
      @search="handleHeroSearch"
    />

    <div class="lunar-tabs-bar">
      <button
        class="lunar-tab"
        :class="{ 'lunar-tab--active': activeTab === 'packages' }"
        @click="activeTab = 'packages'"
      >
        <el-icon><Box /></el-icon>
        包
      </button>
      <button
        class="lunar-tab"
        :class="{ 'lunar-tab--active': activeTab === 'repositories' }"
        @click="activeTab = 'repositories'"
      >
        <el-icon><FolderOpened /></el-icon>
        仓库
      </button>

      <div v-if="activeTab === 'packages'" class="lunar-tab-actions">
        <el-select v-model="sortBy" class="lunar-sort-select" @change="handleSortChange">
          <el-option label="按名称" value="name" />
          <el-option label="按更新时间" value="updated_at" />
          <el-option label="按下载量" value="downloads" />
        </el-select>
        <span class="lunar-stats-count">{{ total }} 个包</span>
      </div>
    </div>

    <div v-if="activeTab === 'packages' && !loading && packages.length > 0" class="stats-inline">
      <span class="stat-chip">
        <el-icon><Box /></el-icon>
        {{ formatNumber(total) }} 个包
      </span>
      <span v-if="searchTime > 0" class="stat-chip stat-chip-dim">
        <el-icon><Clock /></el-icon>
        {{ searchTime }}ms
      </span>
    </div>

    <div v-if="activeTab === 'packages'" class="results-section">
      <!-- 骨架屏 -->
      <div v-if="loading" class="package-grid">
        <div v-for="i in 8" :key="i" class="package-card skeleton-card">
          <div class="skeleton-inner">
            <div class="skeleton-line short"></div>
            <div class="skeleton-line long"></div>
            <div class="skeleton-line medium"></div>
            <div class="skeleton-meta">
              <div class="skeleton-line tiny"></div>
              <div class="skeleton-line tiny"></div>
              <div class="skeleton-line tiny"></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 包列表 - 响应式网格布局 -->
      <div v-else-if="packages.length > 0" class="package-grid">
        <PackageCard
          v-for="pkg in packages"
          :key="pkg.id"
          :pkg="pkg"
          @click="goToDetail(pkg)"
        />
      </div>

      <el-empty
        v-else
        description="暂无匹配的包"
        class="lunar-empty"
      />

      <div v-if="total > pageSize" class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[12, 24, 48, 96]"
          layout="total, sizes, prev, pager, next"
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
          class="lunar-pagination"
        />
      </div>
    </div>

    <div v-if="activeTab === 'repositories'" class="repos-layout">
      <div class="repos-main">
        <RepositoryShowcase />
      </div>
      <aside class="repos-sidebar">
        <RepositoryStatusPanel />
      </aside>
    </div>

    <div class="keyboard-hint" :class="{ 'hint-faded': hintFaded }">
      <span class="hint-item"><kbd>/</kbd> 聚焦搜索</span>
      <span class="hint-item"><kbd>Esc</kbd> 清空</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Box, FolderOpened, Clock } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { packageApi, type Package } from '@/api/package'
import { formatNumber } from '@/utils/format'
import PackageCard from '@/components/browse/PackageCard.vue'
import RepositoryShowcase from '@/components/browse/RepositoryShowcase.vue'
import HeroSection from '@/components/browse/HeroSection.vue'
import RepositoryStatusPanel from '@/components/browse/RepositoryStatusPanel.vue'

const router = useRouter()
const route = useRoute()

const heroRef = ref<InstanceType<typeof HeroSection> | null>(null)

const activeTab = ref('packages')
const loading = ref(false)
const searchQuery = ref('')
const selectedType = ref('all')
const sortBy = ref('name')
const currentPage = ref(1)
const pageSize = ref(24)
const total = ref(0)
const packages = ref<Package[]>([])
const searchTime = ref(0)
const hintFaded = ref(false)
let hintTimer: ReturnType<typeof setTimeout> | null = null

// URL 参数同步 - 读取初始值
function initFromUrl() {
  const query = route.query
  if (query.q) searchQuery.value = query.q as string
  if (query.type) selectedType.value = query.type as string
  if (query.sort) sortBy.value = query.sort as string
  if (query.page) currentPage.value = parseInt(query.page as string) || 1
  if (query.page_size) pageSize.value = parseInt(query.page_size as string) || 24
}

// URL 参数同步 - 更新 URL
function updateUrl() {
  const query: Record<string, string> = {}
  if (searchQuery.value) query.q = searchQuery.value
  if (selectedType.value !== 'all') query.type = selectedType.value
  if (sortBy.value !== 'name') query.sort = sortBy.value
  if (currentPage.value !== 1) query.page = String(currentPage.value)
  if (pageSize.value !== 24) query.page_size = String(pageSize.value)

  router.replace({ query })
}

// 搜索处理
function handleHeroSearch(query: string, type: string) {
  searchQuery.value = query
  selectedType.value = type
  currentPage.value = 1
  handleSearch()
}

const handleSearch = async () => {
  loading.value = true
  try {
    const params: Record<string, unknown> = {
      q: searchQuery.value,
      page: currentPage.value,
      page_size: pageSize.value,
      sort: sortBy.value === 'downloads' ? 'updated_at' : sortBy.value,
    }
    if (selectedType.value !== 'all') {
      params.type = selectedType.value
    }

    const res = await packageApi.search(params as { q?: string; type?: string; sort?: string; page?: number; page_size?: number })
    packages.value = (res.list || []).map(pkg => ({
      ...pkg,
      type: pkg.type || pkg.package_type || pkg.format || 'generic',
    }))
    total.value = res.total || 0
    searchTime.value = res.search_time_ms || 0
  } catch {
    packages.value = []
    total.value = 0
    ElMessage.error('搜索失败，请稍后重试')
  } finally {
    loading.value = false
  }

  // 更新 URL
  updateUrl()
}

function handleSortChange() {
  currentPage.value = 1
  handleSearch()
}

function handlePageChange() {
  handleSearch()
  // 滚动到顶部
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function handleSizeChange() {
  currentPage.value = 1
  handleSearch()
}

function goToDetail(pkg: { id: number; type: string; name: string }) {
  router.push(`/packages/${pkg.type}/${encodeURIComponent(pkg.name)}`)
}

// 键盘快捷键处理
function handleKeydown(event: KeyboardEvent) {
  // 如果焦点在输入框中，不处理
  const target = event.target as HTMLElement
  if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA') {
    if (event.key === 'Escape') {
      target.blur()
    }
    return
  }

  switch (event.key) {
    case '/':
      event.preventDefault()
      // 聚焦搜索框
      const searchInput = heroRef.value?.$el?.querySelector?.('input') || document.querySelector('.lunar-search-input input') as HTMLInputElement
      if (searchInput) {
        searchInput.focus()
      }
      break
    case 'Escape':
      if (searchQuery.value) {
        searchQuery.value = ''
        selectedType.value = 'all'
        currentPage.value = 1
        handleSearch()
      }
      break
  }
}

// 监听路由变化（仅用于外部链接变化，如浏览器前进后退）
watch(() => route.query, () => {
  const query = route.query
  const newSearch = (query.q as string) || ''
  const newType = (query.type as string) || 'all'
  const newSort = (query.sort as string) || 'name'
  const newPage = parseInt((query.page as string) || '1')

  // 只有当值确实发生变化时才更新并搜索
  if (newSearch !== searchQuery.value || newType !== selectedType.value ||
      newSort !== sortBy.value || newPage !== currentPage.value) {
    searchQuery.value = newSearch
    selectedType.value = newType
    sortBy.value = newSort
    currentPage.value = newPage
    handleSearch()
  }
}, { immediate: false })

onMounted(() => {
  initFromUrl()
  handleSearch()
  hintTimer = setTimeout(() => {
    hintFaded.value = true
  }, 5000)
})

onUnmounted(() => {
  if (hintTimer) clearTimeout(hintTimer)
})
</script>

<style scoped>
.browse-page {
  width: 100%;
}

/* Tab Bar */
.lunar-tabs-bar {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0;
  margin-bottom: 24px;
  border-bottom: 1px solid var(--lunar-border);
}

.lunar-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 12px 24px;
  font-size: 14px;
  font-weight: 600;
  color: var(--lunar-silver-muted);
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  transition: all 0.25s ease;
  margin-bottom: -1px;
}

.lunar-tab:hover {
  color: var(--lunar-silver);
}

.lunar-tab--active {
  color: var(--lunar-accent);
  border-bottom-color: var(--lunar-accent);
}

.lunar-tab-actions {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-left: auto;
  padding-bottom: 12px;
}

.lunar-sort-select {
  min-width: 140px;
}

.lunar-sort-select :deep(.el-input__wrapper) {
  background: var(--lunar-bg-glass);
  border: 1px solid var(--lunar-border);
  border-radius: 8px;
  box-shadow: none;
  height: 32px;
}

.lunar-sort-select :deep(.el-input__wrapper:hover) {
  border-color: var(--lunar-border-hover);
}

.lunar-sort-select :deep(.el-input__wrapper.is-focus) {
  border-color: var(--lunar-accent);
  box-shadow: var(--lunar-shadow-glow);
}

.lunar-sort-select :deep(.el-input__inner) {
  color: var(--lunar-silver);
  font-size: 13px;
  font-weight: 600;
}

.lunar-stats-count {
  color: var(--lunar-accent);
  font-size: 13px;
  font-weight: 700;
  white-space: nowrap;
}

/* 统计概览 */
.stats-inline {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 16px;
}

.stat-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--lunar-silver-muted);
}

.stat-chip .el-icon {
  font-size: 14px;
  color: var(--lunar-accent);
}

.stat-chip-dim {
  color: var(--lunar-silver-dim);
}

.stat-chip-dim .el-icon {
  color: var(--lunar-accent-soft);
}

/* 响应式网格布局 */
.results-section {
  margin-top: 0;
}

/* 仓库 tab 布局 */
.repos-layout {
  display: grid;
  grid-template-columns: 1fr 280px;
  gap: 24px;
  margin-top: 0;
}

.repos-main {
  min-width: 0;
}

.repos-sidebar {
  position: sticky;
  top: 80px;
  align-self: start;
}

.package-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}

@media (max-width: 1024px) {
  .repos-layout {
    grid-template-columns: 1fr;
  }

  .repos-sidebar {
    position: static;
  }
}

@media (max-width: 768px) {
  .package-grid {
    grid-template-columns: 1fr;
  }

  .stats-inline {
    gap: 6px;
  }

  .lunar-tab-actions {
    gap: 8px;
  }

  .lunar-sort-select {
    min-width: 100px;
  }
}

/* 骨架屏 */
.skeleton-card {
  border-radius: 10px;
  overflow: hidden;
}

.skeleton-inner {
  padding: 20px 24px;
  background: var(--lunar-bg-card);
  border-radius: 10px;
  border: 1px solid var(--lunar-border);
  transform: translateZ(0);
}

.skeleton-line {
  height: 12px;
  background: var(--lunar-border);
  border-radius: 4px;
  animation: skeletonPulse 1.5s ease-in-out infinite;
  margin-bottom: 12px;
}

@keyframes skeletonPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.skeleton-line.short {
  width: 30%;
}

.skeleton-line.long {
  width: 100%;
}

.skeleton-line.medium {
  width: 70%;
}

.skeleton-line.tiny {
  width: 60px;
  height: 10px;
  margin-bottom: 0;
}

.skeleton-meta {
  display: flex;
  gap: 20px;
  margin-top: 16px;
}

/* 空状态 */
.lunar-empty :deep(.el-empty__description p) {
  color: var(--lunar-silver-muted);
}

.lunar-empty :deep(.el-empty__image svg) {
  fill: var(--lunar-silver-dim);
}

/* 分页 */
.pagination-container {
  display: flex;
  justify-content: center;
  margin-top: 32px;
  padding: 24px 0;
}

.lunar-pagination :deep(.el-pagination) {
  --el-pagination-bg-color: var(--lunar-bg-glass);
  --el-pagination-text-color: var(--lunar-silver-muted);
  --el-pagination-button-bg-color: var(--lunar-bg-glass);
  --el-pagination-hover-color: var(--lunar-accent);
}

.lunar-pagination :deep(.el-pager li) {
  background: var(--lunar-bg-glass);
  color: var(--lunar-silver-muted);
  border-radius: 6px;
  font-weight: 600;
}

.lunar-pagination :deep(.el-pager li:hover) {
  color: var(--lunar-accent);
}

.lunar-pagination :deep(.el-pager li.is-active) {
  background: var(--lunar-gradient-btn);
  color: var(--lunar-bg-deep);
}

.lunar-pagination :deep(.btn-prev),
.lunar-pagination :deep(.btn-next) {
  background: var(--lunar-bg-glass);
  color: var(--lunar-silver-muted);
  border-radius: 6px;
}

.lunar-pagination :deep(.btn-prev:hover),
.lunar-pagination :deep(.btn-next:hover) {
  color: var(--lunar-accent);
}

.lunar-pagination :deep(.el-pagination__total) {
  color: var(--lunar-silver-muted);
}

.lunar-pagination :deep(.el-pagination__sizes .el-input__wrapper) {
  background: var(--lunar-bg-glass);
  border: 1px solid var(--lunar-border);
  color: var(--lunar-silver);
  box-shadow: none;
}

/* 键盘快捷键提示 */
.keyboard-hint {
  position: fixed;
  bottom: 24px;
  right: 24px;
  display: flex;
  gap: 16px;
  padding: 8px 16px;
  background: var(--lunar-bg-glass);
  border: 1px solid var(--lunar-border);
  border-radius: 8px;
  backdrop-filter: blur(8px);
  opacity: 0.7;
  transition: opacity 0.4s ease;
  z-index: 100;
}

.keyboard-hint:hover,
.keyboard-hint:focus-within {
  opacity: 1;
}

.hint-faded {
  opacity: 0.2;
}

.hint-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--lunar-silver-dim);
}

kbd {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  font-size: 11px;
  font-family: var(--font-family-mono);
  color: var(--lunar-silver-muted);
  background: var(--lunar-bg-surface);
  border: 1px solid var(--lunar-border);
  border-radius: 4px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
}

@media (max-width: 768px) {
  .keyboard-hint {
    display: none;
  }
}
</style>
