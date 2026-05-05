<template>
  <div class="package-list">
    <CustomCard>
      <template #header>
        <div class="card-header">
          <span>包管理</span>
          <CustomButton type="primary" size="small">上传包</CustomButton>
        </div>
      </template>

      <div class="filter-bar">
        <CustomInput
          v-model="searchQuery"
          placeholder="搜索包名或描述"
          clearable
          style="width: 300px"
          @input="handleSearch"
        >
          <template #prefix>
            <Search />
          </template>
        </CustomInput>

        <CustomSelect
          v-model="filterType"
          placeholder="包类型"
          clearable
          style="width: 150px"
          :options="typeOptions"
          @change="handleFilter"
        />

        <CustomSelect
          v-model="sortBy"
          placeholder="排序方式"
          style="width: 180px"
          :options="sortOptions"
          @change="handleSort"
        />

        <div class="view-mode-toggle">
          <CustomButton
            :type="viewMode === 'table' ? 'primary' : 'secondary'"
            size="small"
            @click="viewMode = 'table'"
          >
            <FontAwesomeIcon icon="fa-solid fa-list" />
          </CustomButton>
          <CustomButton
            :type="viewMode === 'card' ? 'primary' : 'secondary'"
            size="small"
            @click="viewMode = 'card'"
          >
            <FontAwesomeIcon icon="fa-solid fa-grip" />
          </CustomButton>
        </div>
      </div>

      <PackageTable
        v-if="viewMode === 'table'"
        :packages="packages"
        :loading="loading"
        @view-versions="handleViewVersions"
        @view-detail="handleViewDetail"
      />

      <PackageCards
        v-if="viewMode === 'card'"
        :packages="packages"
        :loading="loading"
        @view-versions="handleViewVersions"
        @view-detail="handleViewDetail"
      />

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
    </CustomCard>

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
import { Search } from '@element-plus/icons-vue'
import PackageTable from '@/components/package/PackageTable.vue'
import PackageCards from '@/components/package/PackageCards.vue'
import VersionDrawer from '@/components/package/VersionDrawer.vue'
import CustomCard from '@/components/ui/CustomCard.vue'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomInput from '@/components/ui/CustomInput.vue'
import CustomSelect from '@/components/ui/CustomSelect.vue'
import { packageApi, type Package } from '@/api/package'
import { ElMessage } from 'element-plus'

const router = useRouter()
const loading = ref(false)
const packages = ref<Package[]>([])
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)

const searchQuery = ref('')
const filterType = ref('')
const sortBy = ref('updated_at')
const viewMode = ref<'table' | 'card'>('table')

const showVersionDrawer = ref(false)
const selectedPackage = ref<Package | null>(null)

const typeOptions = [
  { label: '全部', value: '' },
  { label: 'npm', value: 'npm' },
  { label: 'Maven', value: 'maven' },
  { label: 'PyPI', value: 'pypi' },
  { label: 'Go', value: 'go' },
  { label: 'NuGet', value: 'nuget' },
]

const sortOptions = [
  { label: '更新时间', value: 'updated_at' },
  { label: '下载量', value: 'download_count' },
  { label: '名称', value: 'name' },
]

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

.view-mode-toggle {
  display: flex;
  gap: var(--spacing-xs);
}

.pagination-wrapper {
  margin-top: var(--spacing-xl);
  display: flex;
  justify-content: flex-end;
}
</style>
