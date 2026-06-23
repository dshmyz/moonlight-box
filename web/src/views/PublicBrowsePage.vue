<template>
  <div class="public-browse">
    <!-- Hero -->
    <PublicPackageHero />

    <!-- 双 Tab -->
    <PublicBrowseTabs
      v-model:active-tab="activeTab"
      :package-count="total"
    />

    <!-- 包 Tab -->
    <template v-if="activeTab === 'packages'">
      <div class="packages-section">
        <!-- 统一工具栏 -->
        <PackageSearchBar
          :query="query"
          :recent-searches="recentSearches"
          :loading="loading"
          :has-active-filter="hasActiveFilter"
          :view-mode="viewMode"
          @update:query="onQueryUpdate"
          @update:view-mode="viewMode = $event"
          @search="search"
          @add-recent="addRecentSearch"
          @clear-recent="clearRecentSearches"
          @open-filter="showFilter = !showFilter"
        />

        <!-- 行内筛选面板 -->
        <PackageFilterPanel
          v-show="showFilter"
          :visible="showFilter"
          v-model:repository="query.repository"
          v-model:version="query.version"
          mode="public"
          @apply="onFilterApply"
          @reset="onFilterApply"
        />

        <!-- 统计 -->
        <div v-if="searchTime > 0 && !loading && packages.length > 0" class="search-stats">
          找到 {{ total }} 个包（耗时 {{ searchTime }}ms）
        </div>

        <!-- 骨架屏 -->
        <div v-if="loading && packages.length === 0" class="skeleton-grid">
          <div v-for="i in 6" :key="i" class="skeleton-card" />
        </div>

        <!-- 空状态 -->
        <PackageEmptyState
          v-else-if="!loading && packages.length === 0"
          :variant="emptyVariant"
          mode="public"
          :error-message="error || ''"
          @clear-filters="onClearFilters"
          @retry="search"
        />

        <!-- 网格视图 -->
        <div v-else-if="viewMode === 'grid'" class="packages-container">
          <PackageGrid
            :packages="packages"
            mode="public"
            @view-detail="onViewDetail"
          />
        </div>

        <!-- 表格视图 -->
        <div v-else class="packages-container">
          <PackageTable
            :packages="packages"
            :loading="loading"
            mode="public"
            density="default"
            :selected-ids="[]"
            :columns="{ description: true, source: true, versions: true, downloads: true, updatedAt: true }"
            @view-detail="onViewDetail"
          />
        </div>

        <!-- 分页 -->
        <div class="pagination-wrapper">
          <PackagePagination
            v-if="total > 0"
            :total="total"
            :page="query.page"
            :page-size="query.pageSize"
            :page-size-options="pageSizeOptions"
            @update:page="onPageChange"
            @update:page-size="onPageSizeChange"
          />
        </div>
      </div>
    </template>

    <!-- 仓库 Tab -->
    <template v-else>
      <div class="repos-section">
        <div class="repos-main">
          <RepositoryShowcase />
        </div>
        <aside class="repos-sidebar">
          <RepositoryStatusPanel />
        </aside>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { usePackageSearch, type PackageQuery } from '@/composables/usePackageSearch'
import type { Package } from '@/api/package'
import PublicPackageHero from '@/components/package-explorer/PublicPackageHero.vue'
import PublicBrowseTabs from '@/components/package-explorer/PublicBrowseTabs.vue'
import PackageSearchBar from '@/components/package-explorer/PackageSearchBar.vue'
import PackageFilterPanel from '@/components/package-explorer/PackageFilterPanel.vue'
import PackageGrid from '@/components/package-explorer/PackageGrid.vue'
import PackageTable from '@/components/package-explorer/PackageTable.vue'
import PackagePagination from '@/components/package-explorer/PackagePagination.vue'
import PackageEmptyState from '@/components/package-explorer/PackageEmptyState.vue'
import RepositoryShowcase from '@/components/browse/RepositoryShowcase.vue'
import RepositoryStatusPanel from '@/components/browse/RepositoryStatusPanel.vue'

const {
  query, packages, total, loading, error, searchTime,
  recentSearches, hasActiveFilter,
  search, resetFilters, setQuery,
  addRecentSearch, clearRecentSearches,
} = usePackageSearch({
  mode: 'public',
  defaultPageSize: 24,
  pageSizeOptions: [12, 24, 48, 96],
})

const pageSizeOptions = [12, 24, 48, 96]
const activeTab = ref<'packages' | 'repositories'>('packages')
const showFilter = ref(false)
const viewMode = ref<'table' | 'grid'>('table')
const router = useRouter()

const emptyVariant = computed<'empty' | 'no-match' | 'error'>(() => {
  if (error.value) return 'error'
  if (query.q || hasActiveFilter.value) return 'no-match'
  return 'empty'
})

function onQueryUpdate(patch: Partial<PackageQuery>) {
  setQuery(patch)
}

function onPageChange(p: number) {
  setQuery({ page: p })
  search()
}

function onPageSizeChange(size: number) {
  setQuery({ pageSize: size, page: 1 })
  search()
}

function onFilterApply() {
  setQuery({ page: 1 })
  search()
}

function onClearFilters() {
  resetFilters()
  setQuery({ q: '', type: 'all', page: 1 })
  search()
}

function onViewDetail(pkg: Package) {
  router.push({ name: 'PackageDetail', params: { type: pkg.type, name: pkg.name } })
}



onMounted(() => {
  search()
})
</script>

<style scoped>
.public-browse {
  min-height: 100%;
  margin: -24px 0 0;
}
.packages-section {
  padding: 0 0 32px;
}
.search-stats {
  font-size: 12px;
  color: var(--lunar-silver-muted);
  margin: 14px 0 4px;
  font-family: var(--font-family-mono);
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.01em;
}
.packages-container {
  margin-top: 12px;
}
.skeleton-grid {
  margin-top: 12px;
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}
.skeleton-card {
  height: 132px;
  background: linear-gradient(90deg, var(--lunar-bg-glass) 25%, var(--lunar-border) 50%, var(--lunar-bg-glass) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: var(--radius-md);
  border: 1px solid var(--lunar-border);
}
@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
.pagination-wrapper {
  margin-top: 20px;
}
.repos-section {
  display: flex;
  gap: 24px;
  padding: 16px 0;
}
.repos-main { flex: 1; min-width: 0; }
.repos-sidebar { width: 320px; flex-shrink: 0; }
</style>
