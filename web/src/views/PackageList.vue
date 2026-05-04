<template>
  <div class="package-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>包管理</span>
          <el-button type="primary" size="small">上传包</el-button>
        </div>
      </template>

      <!-- 搜索和过滤栏 -->
      <div class="filter-bar">
        <el-input
          v-model="searchQuery"
          placeholder="搜索包名或描述"
          clearable
          style="width: 300px"
          @input="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>

        <el-select v-model="filterType" placeholder="包类型" clearable style="width: 150px" @change="handleFilter">
          <el-option label="全部" value="" />
          <el-option label="npm" value="npm" />
          <el-option label="Maven" value="maven" />
          <el-option label="PyPI" value="pypi" />
          <el-option label="Go" value="go" />
          <el-option label="NuGet" value="nuget" />
        </el-select>

        <el-select v-model="sortBy" placeholder="排序方式" style="width: 180px" @change="handleSort">
          <el-option label="更新时间" value="updated_at" />
          <el-option label="下载量" value="download_count" />
          <el-option label="名称" value="name" />
        </el-select>

        <el-radio-group v-model="viewMode" size="small">
          <el-radio-button value="table">
            <el-icon><List /></el-icon>
          </el-radio-button>
          <el-radio-button value="card">
            <el-icon><Grid /></el-icon>
          </el-radio-button>
        </el-radio-group>
      </div>

      <!-- 表格视图 -->
      <PackageTable
        v-if="viewMode === 'table'"
        :packages="packages"
        :loading="loading"
        @view-versions="handleViewVersions"
        @view-detail="handleViewDetail"
      />

      <!-- 卡片视图 -->
      <PackageCards
        v-if="viewMode === 'card'"
        :packages="packages"
        :loading="loading"
        @view-versions="handleViewVersions"
        @view-detail="handleViewDetail"
      />

      <!-- 分页 -->
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          :page-size="pageSize"
          :total="total"
          layout="total, prev, pager, next, sizes"
          :page-sizes="[20, 50, 100]"
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
        />
      </div>
    </el-card>

    <!-- 版本抽屉 -->
    <VersionDrawer
      v-model="showVersionDrawer"
      :package-type="selectedPackage?.type || ''"
      :package-name="selectedPackage?.name || ''"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Search, List, Grid } from '@element-plus/icons-vue'
import PackageTable from '@/components/package/PackageTable.vue'
import PackageCards from '@/components/package/PackageCards.vue'
import VersionDrawer from '@/components/package/VersionDrawer.vue'
import { packageApi, type Package } from '@/api/package'
import { ElMessage } from 'element-plus'

const router = useRouter()
const loading = ref(false)
const packages = ref<Package[]>([])
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)

// 搜索和过滤
const searchQuery = ref('')
const filterType = ref('')
const sortBy = ref('updated_at')
const viewMode = ref<'table' | 'card'>('table')

// 版本抽屉
const showVersionDrawer = ref(false)
const selectedPackage = ref<Package | null>(null)

let searchTimer: ReturnType<typeof setTimeout> | null = null

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
    packages.value = response.list || []
    total.value = response.total || 0
  } catch (error) {
    ElMessage.error('加载包列表失败')
    console.error('Failed to load packages:', error)
  } finally {
    loading.value = false
  }
}

function handleSearch() {
  if (searchTimer) {
    clearTimeout(searchTimer)
  }
  searchTimer = setTimeout(() => {
    currentPage.value = 1
    loadPackages()
  }, 300)
}

function handleFilter() {
  currentPage.value = 1
  loadPackages()
}

function handleSort() {
  currentPage.value = 1
  loadPackages()
}

function handlePageChange() {
  loadPackages()
}

function handleSizeChange() {
  currentPage.value = 1
  loadPackages()
}

function handleViewVersions(pkg: Package) {
  selectedPackage.value = pkg
  showVersionDrawer.value = true
}

function handleViewDetail(pkg: Package) {
  router.push({
    name: 'AdminPackageDetail',
    params: { type: pkg.type, name: pkg.name },
  })
}

onMounted(() => {
  loadPackages()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.filter-bar {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-xl);
  align-items: center;
  flex-wrap: wrap;
}

.pagination-wrapper {
  margin-top: var(--spacing-xl);
  display: flex;
  justify-content: flex-end;
}
</style>
