<template>
  <div class="package-explorer">
    <!-- 头部（仅 admin） -->
    <header v-if="mode === 'admin'" class="explorer-header">
      <div class="header-content">
        <div class="header-icon"><i class="fa-solid fa-box"></i></div>
        <div>
          <h2>包管理</h2>
          <p class="header-subtitle">管理和分发您的软件包</p>
        </div>
      </div>
      <el-button
        v-permission="'package:write'"
        type="primary"
        class="upload-btn"
        @click="showUpload = true"
      >
        <el-icon><Upload /></el-icon>上传包
      </el-button>
    </header>

    <!-- 搜索栏 -->
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
      @open-filter="showFilter = true"
    />

    <!-- 行内筛选面板 -->
    <PackageFilterPanel
      v-show="showFilter"
      :visible="showFilter"
      v-model:repository="query.repository"
      v-model:version="query.version"
      @apply="onFilterApply"
      @reset="onFilterApply"
    />

    <!-- 批量操作栏（admin + 选中后） -->
    <div v-if="mode === 'admin' && selectedIds.length > 0" class="batch-bar">
      <span class="batch-info">已选 {{ selectedIds.length }} 项</span>
      <div class="batch-actions">
        <el-button v-permission="'package:delete'" type="danger" plain @click="onBatchDelete">
          批量删除
        </el-button>
        <el-button @click="onBatchExport">导出 CSV</el-button>
        <el-button link @click="selectedIds = []">取消选择</el-button>
      </div>
    </div>

    <!-- 结果区 -->
    <div class="content-panel">
      <div v-if="searchTime > 0 && !loading" class="search-stats">
        找到 {{ total }} 个包（耗时 {{ searchTime }}ms）
      </div>

      <!-- 骨架屏（首次加载） -->
      <PackageSkeleton v-if="loading && packages.length === 0" :variant="viewMode" />

      <!-- 空状态 -->
      <PackageEmptyState
        v-else-if="!loading && packages.length === 0"
        :variant="emptyVariant"
        :mode="mode"
        :error-message="error || ''"
        @upload="showUpload = true"
        @clear-filters="onClearFilters"
        @retry="search"
      />

      <!-- 表格视图 -->
      <PackageTable
        v-else-if="viewMode === 'table'"
        :packages="packages"
        :loading="loading"
        :mode="mode"
        :density="density"
        :selected-ids="selectedIds"
        :columns="columns"
        @update:selected-ids="selectedIds = $event"
        @view-versions="onViewVersions"
        @view-detail="onViewDetail"
        @delete-package="onDeletePackage"
      />

      <!-- 网格视图 -->
      <PackageGrid
        v-else
        :packages="packages"
        :mode="mode"
        @view-versions="onViewVersions"
        @view-detail="onViewDetail"
        @delete-package="onDeletePackage"
      />

      <PackagePagination
        :total="total"
        :page="query.page"
        :page-size="query.pageSize"
        :page-size-options="pageSizeOptions"
        @update:page="onPageChange"
        @update:page-size="onPageSizeChange"
      />
    </div>

    <!-- 版本抽屉（复用现有组件） -->
    <VersionDrawer
      v-model="showVersionDrawer"
      :package-type="selectedPackage?.type || ''"
      :package-name="selectedPackage?.name || ''"
    />

    <!-- 上传对话框（复用现有组件，admin only） -->
    <UploadPackageDialog
      v-if="mode === 'admin'"
      v-model="showUpload"
      @uploaded="refresh"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Upload } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { usePackageSearch, type PackageQuery } from '@/composables/usePackageSearch'
import { packageApi, type Package } from '@/api/package'
import PackageSearchBar from './PackageSearchBar.vue'
import PackageFilterPanel from './PackageFilterPanel.vue'
import PackageTable from './PackageTable.vue'
import PackageGrid from './PackageGrid.vue'
import PackagePagination from './PackagePagination.vue'
import PackageSkeleton from './PackageSkeleton.vue'
import PackageEmptyState from './PackageEmptyState.vue'
import VersionDrawer from '@/components/package/VersionDrawer.vue'
import UploadPackageDialog from '@/components/package/UploadPackageDialog.vue'

const props = defineProps<{
  mode: 'admin' | 'public'
  pageSizeOptions?: number[]
  defaultPageSize?: number
}>()

const router = useRouter()

const {
  query, packages, total, loading, error, searchTime,
  recentSearches, hasActiveFilter,
  search, refresh, resetFilters, setQuery,
  addRecentSearch, clearRecentSearches,
} = usePackageSearch({
  mode: props.mode,
  pageSizeOptions: props.pageSizeOptions,
  defaultPageSize: props.defaultPageSize,
})

const pageSizeOptions = props.pageSizeOptions ?? [20, 50, 100]

// 局部 UI 状态
const viewMode = ref<'table' | 'grid'>(props.mode === 'admin' ? 'table' : 'grid')
const selectedIds = ref<number[]>([])
const showFilter = ref(false)
const showVersionDrawer = ref(false)
const showUpload = ref(false)
const selectedPackage = ref<Package | null>(null)

// 列设置与密度（admin only，存 localStorage）
const columnsStorageKey = `package-explorer:columns:${props.mode}`
const densityStorageKey = `package-explorer:density:${props.mode}`
const columns = ref(loadColumns())
const density = ref<'small' | 'default' | 'large'>(loadDensity())

interface ColumnConfig {
  description?: boolean
  source?: boolean
  versions?: boolean
  downloads?: boolean
  updatedAt?: boolean
}

function loadColumns(): ColumnConfig {
  try {
    return JSON.parse(localStorage.getItem(columnsStorageKey) || '{}')
  } catch {
    return {}
  }
}
function loadDensity(): 'small' | 'default' | 'large' {
  return (localStorage.getItem(densityStorageKey) as 'small' | 'default' | 'large') || 'default'
}

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
  selectedIds.value = []
  search()
}

function onPageSizeChange(size: number) {
  setQuery({ pageSize: size, page: 1 })
  selectedIds.value = []
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

function onViewVersions(pkg: Package) {
  selectedPackage.value = pkg
  showVersionDrawer.value = true
}

function onViewDetail(pkg: Package) {
  const routeName = props.mode === 'admin' ? 'AdminPackageDetail' : 'PackageDetail'
  router.push({ name: routeName, params: { type: pkg.type, name: pkg.name } })
}

async function onDeletePackage(pkg: Package) {
  try {
    await ElMessageBox.confirm(
      `确定要删除包 "${pkg.display_name || pkg.name}" 及其所有版本吗？此操作不可恢复！`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' }
    )
    await packageApi.deletePackage(pkg)
    ElMessage.success('包已删除')
    await refresh()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除包失败')
  }
}

async function onBatchDelete() {
  const selected = packages.value.filter(p => selectedIds.value.includes(p.id))
  const names = selected.slice(0, 5).map(p => p.display_name || p.name).join('、')
  const suffix = selected.length > 5 ? ` 等 ${selected.length} 个包` : ''
  try {
    await ElMessageBox.confirm(
      `确定要删除 ${names}${suffix} 吗？此操作不可恢复！`,
      '批量删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' }
    )

    const failed: string[] = []
    for (const pkg of selected) {
      try {
        await packageApi.deletePackage(pkg)
      } catch {
        failed.push(pkg.display_name || pkg.name)
      }
    }

    if (failed.length === 0) {
      ElMessage.success(`已删除 ${selected.length} 个包`)
    } else {
      ElMessage.warning(`删除完成，${failed.length} 个失败：${failed.slice(0, 3).join('、')}`)
    }

    selectedIds.value = []
    await refresh()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('批量删除失败')
  }
}

function onBatchExport() {
  const selected = packages.value.filter(p => selectedIds.value.includes(p.id))
  const headers = ['type', 'name', 'version', 'downloads', 'updated_at']
  const rows = selected.map(p => [p.type, p.name, p.latest_version || '', p.download_count, p.updated_at])
  const csv = [headers.join(','), ...rows.map(r => r.join(','))].join('\n')
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `packages-${Date.now()}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

onMounted(() => {
  search()
})
</script>

<style scoped>
.package-explorer { display: flex; flex-direction: column; gap: 16px; }
.explorer-header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 20px 24px; background: #fff; border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.06); box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}
.header-content { display: flex; align-items: center; gap: 16px; }
.header-icon {
  width: 44px; height: 44px; border-radius: 12px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  display: flex; align-items: center; justify-content: center; color: #fff; font-size: 20px;
}
.explorer-header h2 { font-size: 22px; font-weight: 700; margin: 0; color: #1e293b; }
.header-subtitle { font-size: 13px; color: #64748b; margin: 4px 0 0; }
.upload-btn {
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border: none; box-shadow: 0 4px 14px rgba(37, 99, 235, 0.3);
}
.batch-bar {
  display: flex; justify-content: space-between; align-items: center;
  padding: 12px 20px; background: #eef2ff; border-radius: 10px;
  border: 1px solid #c7d2fe;
}
.batch-info { font-size: 14px; font-weight: 600; color: #4f46e5; }
.batch-actions { display: flex; gap: 8px; align-items: center; }
.content-panel {
  background: #fff; border-radius: 16px; border: 1px solid rgba(0, 0, 0, 0.06);
  overflow: hidden; box-shadow: 0 4px 16px rgba(0, 0, 0, 0.04);
}
.search-stats {
  padding: 12px 24px; font-size: 13px; color: #64748b;
  border-bottom: 1px solid rgba(0, 0, 0, 0.04); background: #fafbfc;
}
</style>
