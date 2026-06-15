<template>
  <div class="package-center">
    <StatsOverview :stats="stats" :loading="statsLoading" />

    <QuickActions :package-type-stats="packageTypeStats" @filter="handleQuickFilter" />

    <div class="content-layout">
      <PackageListSection
        :packages="packages"
        :loading="loading"
        :total="total"
        :current-page="currentPage"
        :page-size="pageSize"
        :active-filter="filterType"
        :active-tab="activeTab"
        :sort-by="sortBy"
        @search="handleSearch"
        @filter="handleFilter"
        @sort="handleSort"
        @tab-change="handleTabChange"
        @page-change="handlePageChange"
        @size-change="handleSizeChange"
        @view-detail="handleViewDetail"
        @view-versions="handleViewVersions"
        @delete-package="handleDeletePackage"
      />

      <PackageSidebar
        :top-packages="topPackages"
        :repositories="repositories"
        :recent-updates="recentUpdates"
      />
    </div>

    <VersionDrawer
      v-model="showVersionDrawer"
      :package-type="selectedPackage?.type || ''"
      :package-name="selectedPackage?.name || ''"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { packageApi, type Package } from '@/api/package'
import { dashboardApi, type DashboardStats } from '@/api/dashboard'
import { repositoryApi, type RepositoryWithHealth } from '@/api/repository'
import StatsOverview from '@/components/package-center/StatsOverview.vue'
import QuickActions from '@/components/package-center/QuickActions.vue'
import PackageListSection from '@/components/package-center/PackageListSection.vue'
import PackageSidebar from '@/components/package-center/PackageSidebar.vue'
import VersionDrawer from '@/components/package/VersionDrawer.vue'

const router = useRouter()

const loading = ref(false)
const statsLoading = ref(false)
const packages = ref<Package[]>([])
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)

const searchQuery = ref('')
const filterType = ref('')
const sortBy = ref('updated_at')
const activeTab = ref('all')

const showVersionDrawer = ref(false)
const selectedPackage = ref<Package | null>(null)

const stats = ref<DashboardStats | null>(null)
const repositories = ref<RepositoryWithHealth[]>([])

const topPackages = computed(() => stats.value?.top_packages || [])

const packageTypeStats = computed(() => {
  if (!stats.value?.repositories) return {}
  const typeMap: Record<string, number> = {}
  for (const repo of stats.value.repositories) {
    const type = repo.package_type
    typeMap[type] = (typeMap[type] || 0) + repo.package_count
  }
  return typeMap
})

const recentUpdates = ref<Array<{ name: string; type: string; version: string; time: string; isNew: boolean }>>([])

let searchTimer: ReturnType<typeof setTimeout> | null = null

async function loadStats() {
  statsLoading.value = true
  try {
    stats.value = await dashboardApi.getStats()
  } catch (error) {
    console.error('Failed to load stats:', error)
  } finally {
    statsLoading.value = false
  }
}

async function loadRepositories() {
  try {
    const res = await repositoryApi.list({ page_size: 10 })
    repositories.value = Array.isArray(res) ? res : (res.items || [])
  } catch (error) {
    console.error('Failed to load repositories:', error)
  }
}

async function loadPackages() {
  loading.value = true
  try {
    const response = await packageApi.search({
      q: searchQuery.value,
      type: filterType.value || undefined,
      sort: sortBy.value,
      page: currentPage.value,
      page_size: pageSize.value,
    })
    packages.value = (response.list || []).map(pkg => ({
      ...pkg,
      type: pkg.type || pkg.package_type || pkg.format || 'generic',
    }))
    total.value = response.total || 0

    if (currentPage.value === 1 && !searchQuery.value) {
      recentUpdates.value = packages.value.slice(0, 5).map(pkg => ({
        name: pkg.name,
        type: pkg.type,
        version: pkg.latest_version || '-',
        time: pkg.updated_at,
        isNew: false,
      }))
    }
  } catch (error) {
    ElMessage.error('加载包列表失败')
    console.error('Failed to load packages:', error)
  } finally {
    loading.value = false
  }
}

function handleSearch(query: string) {
  searchQuery.value = query
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    currentPage.value = 1
    loadPackages()
  }, 300)
}

function handleFilter(type: string) {
  filterType.value = type
  currentPage.value = 1
  loadPackages()
}

function handleSort(sort: string) {
  sortBy.value = sort
  currentPage.value = 1
  loadPackages()
}

function handleTabChange(tab: string) {
  activeTab.value = tab
  currentPage.value = 1
  loadPackages()
}

function handleQuickFilter(type: string) {
  filterType.value = type
  currentPage.value = 1
  loadPackages()
}

function handlePageChange(page: number) {
  currentPage.value = page
  loadPackages()
}

function handleSizeChange(size: number) {
  pageSize.value = size
  currentPage.value = 1
  loadPackages()
}

function handleViewDetail(pkg: Package) {
  router.push({
    name: 'AdminPackageDetail',
    params: { type: pkg.type, name: pkg.name },
  })
}

function handleViewVersions(pkg: Package) {
  selectedPackage.value = pkg
  showVersionDrawer.value = true
}

async function handleDeletePackage(pkg: Package) {
  try {
    await ElMessageBox.confirm(
      `确定要删除包 "${pkg.display_name || pkg.name}" 及其所有版本吗？此操作不可恢复！`,
      '删除确认',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
      }
    )
    await packageApi.deletePackage(pkg)
    ElMessage.success('包已删除')
    loadPackages()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除包失败')
      console.error('Failed to delete package:', error)
    }
  }
}

onMounted(() => {
  loadStats()
  loadRepositories()
  loadPackages()
})
</script>

<style scoped>
.package-center {
  min-height: calc(100vh - 60px);
  background: var(--color-bg-page, #f8fafc);
}

.content-layout {
  display: grid;
  grid-template-columns: 1fr 320px;
  gap: 24px;
  max-width: 1400px;
  margin: 0 auto;
  padding: 0 32px 32px;
}

@media (max-width: 1200px) {
  .content-layout {
    grid-template-columns: 1fr;
  }
}
</style>
