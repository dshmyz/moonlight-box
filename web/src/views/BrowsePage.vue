<template>
  <div class="browse-page">
    <div class="hero-section">
      <h1 class="page-title">软件包中心</h1>
      <p class="page-desc">统一管理、搜索和分发多语言软件包</p>
    </div>

    <div class="search-bar-wrapper">
      <el-input
        v-model="searchQuery"
        placeholder="搜索包名、描述或标签..."
        size="large"
        clearable
        class="search-input"
        @keyup.enter="handleSearch"
        @clear="handleSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
        <template #append>
          <el-button @click="handleSearch">搜索</el-button>
        </template>
      </el-input>
    </div>

    <el-tabs v-model="activeTab" class="browse-tabs">
      <el-tab-pane label="包" name="packages">
        <div class="results-section">
          <div class="filter-card">
            <div class="filter-group">
              <span class="filter-label">类型:</span>
              <el-radio-group v-model="selectedType" @change="handleSearch" size="small">
                <el-radio-button value="all">全部</el-radio-button>
                <el-radio-button value="npm">npm</el-radio-button>
                <el-radio-button value="maven">Maven</el-radio-button>
                <el-radio-button value="pypi">PyPI</el-radio-button>
                <el-radio-button value="go">Go</el-radio-button>
              </el-radio-group>
            </div>
            <div class="filter-divider" />
            <div class="filter-group">
              <span class="filter-label">排序:</span>
              <el-select v-model="sortBy" size="small" @change="handleSearch">
                <el-option label="按名称" value="name" />
                <el-option label="按更新时间" value="updated_at" />
                <el-option label="按下载量" value="downloads" />
              </el-select>
            </div>
            <div class="filter-divider" />
            <div class="filter-stats">
              <span class="stats-count">{{ total }} 个包</span>
            </div>
          </div>

          <div v-loading="loading" class="package-list">
            <el-empty v-if="packages.length === 0 && !loading" description="暂无匹配的包" />
            <template v-else>
              <PackageCard
                v-for="pkg in packages"
                :key="pkg.id"
                :pkg="pkg"
                @click="goToDetail(pkg)"
              />
            </template>
          </div>

          <div v-if="total > pageSize" class="pagination-container">
            <el-pagination
              v-model:current-page="currentPage"
              v-model:page-size="pageSize"
              :total="total"
              :page-sizes="[10, 20, 50, 100]"
              layout="total, sizes, prev, pager, next"
              @current-change="handleSearch"
              @size-change="handleSearch"
            />
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="仓库" name="repositories">
        <RepositoryShowcase />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { packageApi, type Package } from '@/api/package'
import PackageCard from '@/components/browse/PackageCard.vue'
import RepositoryShowcase from '@/components/browse/RepositoryShowcase.vue'

const router = useRouter()
const route = useRoute()

const activeTab = ref('packages')
const loading = ref(false)
const searchQuery = ref('')
const selectedType = ref('all')
const sortBy = ref('name')
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)
const packages = ref<Package[]>([])

const handleSearch = async () => {
  loading.value = true
  try {
    const params: Record<string, unknown> = {
      q: searchQuery.value,
      page: currentPage.value,
      page_size: pageSize.value,
      sort: sortBy.value,
    }
    if (selectedType.value !== 'all') {
      params.type = selectedType.value
    }

    const res = await packageApi.search(params as { q: string; type?: string; scope?: string; sort?: string; page?: number; page_size?: number })
    packages.value = res.list || []
    total.value = res.total || 0
  } catch {
    packages.value = []
    total.value = 0
    ElMessage.error('搜索失败，请稍后重试')
  } finally {
    loading.value = false
  }
}

function goToDetail(pkg: { id: number; type: string; name: string }) {
  router.push(`/packages/${pkg.type}/${encodeURIComponent(pkg.name)}`)
}

watch(() => route.query.q, (newVal) => {
  if (newVal && typeof newVal === 'string') {
    searchQuery.value = newVal
    handleSearch()
  }
})

onMounted(() => {
  if (route.query.q && typeof route.query.q === 'string') {
    searchQuery.value = route.query.q
  }
  handleSearch()
})
</script>

<style scoped>
.browse-page {
  width: 100%;
}

.hero-section {
  background: transparent;
  border: none;
  border-radius: 8px;
  padding: 0 0 20px;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #1f2937;
  margin: 0 0 4px;
  letter-spacing: -0.3px;
  line-height: 1.2;
}

.page-desc {
  color: #6b7280;
  font-size: 14px;
  margin: 0;
  line-height: 1.5;
}

.search-bar-wrapper {
  display: flex;
  justify-content: flex-start;
  margin-bottom: 16px;
}

.search-input {
  width: 100%;
}

.search-input :deep(.el-input__wrapper) {
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  box-shadow: none;
  padding: 0 14px;
  height: 40px;
  background: #fafafa;
  transition: all 0.2s;
}

.search-input :deep(.el-input__wrapper:hover),
.search-input :deep(.el-input__wrapper.is-focus) {
  border-color: #2563eb;
  background: #fff;
}

.search-input :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}

.search-input :deep(.el-input__inner) {
  font-size: 14px;
  color: #374151;
}

.search-input :deep(.el-input__prefix) {
  color: #9ca3af;
}

.search-input :deep(.el-input__prefix-inner > .el-icon) {
  font-size: 16px;
}

.search-input :deep(.el-input-group__append) {
  background: #2563eb;
  border: none;
  border-radius: 0 8px 8px 0;
  padding: 0 16px;
}

.search-input :deep(.el-input-group__append:hover) {
  background: #1d4ed8;
}

.search-input :deep(.el-input-group__append:active) {
  transform: scale(0.98);
}

.search-input :deep(.el-input-group__append .el-button) {
  background: transparent;
  border: none;
  color: #ffffff;
  font-weight: 500;
  font-size: 14px;
  padding: 0;
  height: auto;
  line-height: normal;
}

.browse-tabs {
  margin-top: 0;
}

.browse-tabs :deep(.el-tabs__nav-wrap::after) {
  height: 1px;
  background: #e2e8f0;
}

.browse-tabs :deep(.el-tabs__header) {
  margin-bottom: 0;
  border-bottom: 2px solid #0f172a;
}

.browse-tabs :deep(.el-tabs__item) {
  font-weight: 600;
  color: #64748b;
  transition: all 0.2s ease;
  font-size: 14px;
  padding: 0 24px;
  height: 48px;
  line-height: 48px;
}

.browse-tabs :deep(.el-tabs__item.is-active) {
  color: #0f172a;
}

.browse-tabs :deep(.el-tabs__active-bar) {
  background: #0f172a;
  height: 2px;
}

.results-section {
  margin-top: 20px;
}

.filter-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0;
  background: transparent;
  border: none;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.filter-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.filter-divider {
  display: none;
}

.filter-label {
  font-size: 13px;
  color: #6b7280;
  white-space: nowrap;
}

.filter-group :deep(.el-select) {
  min-width: 110px;
}

.filter-group :deep(.el-input__wrapper) {
  border-radius: 6px;
  border: 1px solid #e5e7eb;
  background: #fff;
  box-shadow: none;
  padding: 0 10px;
  height: 32px;
}

.filter-group :deep(.el-input__wrapper:hover),
.filter-group :deep(.el-input__wrapper.is-focus) {
  border-color: #2563eb;
}

.filter-stats {
  margin-left: auto;
}

.stats-count {
  color: #6b7280;
  font-size: 13px;
  white-space: nowrap;
}

.browse-tabs :deep(.el-radio-button__inner) {
  border-radius: 6px;
  border: 1px solid #e5e7eb;
  background: #fff;
  color: #6b7280;
  font-size: 13px;
  padding: 6px 12px;
  box-shadow: none;
}

.browse-tabs :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  background: #2563eb;
  border-color: #2563eb;
  color: #fff;
  box-shadow: none;
}

.browse-tabs :deep(.el-tabs__nav-wrap::after) {
  height: 1px;
  background: #e5e7eb;
}

.browse-tabs :deep(.el-tabs__header) {
  margin-bottom: 0;
  border-bottom: 1px solid #e5e7eb;
}

.browse-tabs :deep(.el-tabs__item) {
  font-weight: 500;
  color: #6b7280;
  transition: all 0.2s;
  font-size: 14px;
  padding: 0 20px;
  height: 44px;
  line-height: 44px;
}

.browse-tabs :deep(.el-tabs__item.is-active) {
  color: #2563eb;
}

.browse-tabs :deep(.el-tabs__active-bar) {
  background: #2563eb;
  height: 2px;
}

.package-list {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

@media (max-width: 1199px) {
  .package-list {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 767px) {
  .package-list {
    display: flex;
    flex-direction: column;
    gap: 0;
  }
}

.pagination-container {
  display: flex;
  justify-content: center;
  margin-top: 32px;
  padding: 24px 0;
}
</style>
