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
  currentPage.value = 1
  try {
    const params: any = {
      q: searchQuery.value,
      page: currentPage.value,
      page_size: pageSize.value,
      sort: sortBy.value,
    }
    if (selectedType.value !== 'all') {
      params.type = selectedType.value
    }

    const res = await packageApi.search(params)
    const list = res.list || res.data?.list || []
    const totalVal = res.total ?? res.data?.total ?? 0
    if (list.length === 0 && !searchQuery.value) {
      const mock = getMockPackages()
      const filtered = selectedType.value === 'all' ? mock : mock.filter(p => normalizeType(p.type) === normalizeType(selectedType.value))
      packages.value = filtered
      total.value = filtered.length
    } else {
      packages.value = list
      total.value = totalVal
    }
  } catch {
    const mock = getMockPackages()
    const filtered = selectedType.value === 'all' ? mock : mock.filter(p => normalizeType(p.type) === normalizeType(selectedType.value))
    const q = searchQuery.value.toLowerCase()
    const matched = q ? filtered.filter(p => p.name.toLowerCase().includes(q) || p.description.toLowerCase().includes(q)) : filtered
    packages.value = matched
    total.value = matched.length
  } finally {
    loading.value = false
  }
}

function normalizeType(type: string) {
  return type === 'maven' ? 'maven2' : type
}

function getMockPackages(): Package[] {
  return [
    { id: 1, name: 'lodash', type: 'npm', description: 'A modern JavaScript utility library delivering modularity, performance & extras', latest_version: '4.17.21', download_count: 50000000, updated_at: '2024-01-15T10:00:00Z' },
    { id: 2, name: 'express', type: 'npm', description: 'Fast, unopinionated, minimalist web framework for Node.js', latest_version: '4.18.2', download_count: 32000000, updated_at: '2024-02-20T10:00:00Z' },
    { id: 3, name: 'axios', type: 'npm', description: 'Promise based HTTP client for the browser and Node.js', latest_version: '1.6.7', download_count: 45000000, updated_at: '2024-03-10T10:00:00Z' },
    { id: 4, name: 'vue', type: 'npm', description: 'The progressive JavaScript framework for building modern web UI', latest_version: '3.4.15', download_count: 18000000, updated_at: '2024-04-01T10:00:00Z' },
    { id: 5, name: 'react', type: 'npm', description: 'The library for web and native user interfaces', latest_version: '18.2.0', download_count: 28000000, updated_at: '2024-01-20T10:00:00Z' },
    { id: 6, name: 'com.google.guava:guava', type: 'maven2', description: 'Google Core Libraries for Java', latest_version: '33.0.0', download_count: 15000000, updated_at: '2024-02-28T10:00:00Z' },
    { id: 7, name: 'org.springframework.boot:spring-boot-starter-web', type: 'maven2', description: 'Spring Boot Web Starter', latest_version: '3.2.3', download_count: 22000000, updated_at: '2024-03-15T10:00:00Z' },
    { id: 8, name: 'com.fasterxml.jackson.core:jackson-databind', type: 'maven2', description: 'General data-binding functionality for Jackson', latest_version: '2.16.1', download_count: 18000000, updated_at: '2024-01-10T10:00:00Z' },
    { id: 9, name: 'flask', type: 'pypi', description: 'A simple framework for building complex web applications', latest_version: '3.0.2', download_count: 12000000, updated_at: '2024-02-15T10:00:00Z' },
    { id: 10, name: 'requests', type: 'pypi', description: 'Python HTTP for Humans', latest_version: '2.31.0', download_count: 35000000, updated_at: '2024-01-05T10:00:00Z' },
    { id: 11, name: 'numpy', type: 'pypi', description: 'Fundamental package for array computing in Python', latest_version: '1.26.4', download_count: 40000000, updated_at: '2024-03-01T10:00:00Z' },
    { id: 12, name: 'github.com/gin-gonic/gin', type: 'go', description: 'Gin is a HTTP web framework written in Go', latest_version: '1.9.1', download_count: 8000000, updated_at: '2024-02-10T10:00:00Z' },
    { id: 13, name: 'github.com/go-chi/chi', type: 'go', description: 'Lightweight, idiomatic, composable router for building Go HTTP services', latest_version: '5.0.12', download_count: 5000000, updated_at: '2024-01-25T10:00:00Z' },
    { id: 14, name: 'typescript', type: 'npm', description: 'TypeScript is a language for application-scale JavaScript', latest_version: '5.3.3', download_count: 38000000, updated_at: '2024-03-20T10:00:00Z' },
    { id: 15, name: 'fastapi', type: 'pypi', description: 'FastAPI framework, high performance, easy to learn, fast to code', latest_version: '0.110.0', download_count: 9000000, updated_at: '2024-03-05T10:00:00Z' },
  ]
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
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 20px 24px;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}

.page-title {
  font-size: 28px;
  font-weight: 800;
  color: #0f172a;
  margin: 0 0 6px;
  letter-spacing: -0.8px;
  line-height: 1.1;
}

.page-desc {
  color: #64748b;
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
  width: 480px;
}

.search-input :deep(.el-input__wrapper) {
  border-radius: 8px;
  border: 2px solid #e2e8f0;
  box-shadow: none;
  padding: 6px 12px;
  background: #fafbfc;
  transition: all 0.25s ease;
}

.search-input :deep(.el-input__wrapper:hover) {
  border-color: #0f172a;
  background: #ffffff;
}

.search-input :deep(.el-input__wrapper.is-focus) {
  border-color: #0f172a;
  background: #ffffff;
  box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
}

.search-input :deep(.el-input__inner) {
  font-size: 14px;
  color: #0f172a;
}

.search-input :deep(.el-input__prefix) {
  color: #94a3b8;
}

.search-input :deep(.el-input__prefix-inner > .el-icon) {
  font-size: 16px;
}

.search-input :deep(.el-input-group__append) {
  background: linear-gradient(135deg, #0f172a 0%, #1e293b 100%);
  color: #ffffff;
  border: none;
  border-radius: 0 8px 8px 0;
  font-weight: 600;
  font-size: 14px;
  padding: 0 28px;
  cursor: pointer;
  transition: all 0.2s ease;
  letter-spacing: 0.5px;
}

.search-input :deep(.el-input-group__append:hover) {
  background: linear-gradient(135deg, #1e293b 0%, #334155 100%);
}

.search-input :deep(.el-input-group__append:active) {
  transform: scale(0.97);
}

.search-input :deep(.el-input-group__append .el-button) {
  background: transparent;
  border: none;
  color: #ffffff;
  font-weight: 600;
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
  margin-top: 24px;
}

.filter-card {
  display: flex;
  align-items: center;
  gap: 24px;
  padding: 16px 20px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  margin-bottom: 20px;
}

.filter-group {
  display: flex;
  align-items: center;
  gap: 12px;
}

.filter-divider {
  width: 1px;
  height: 20px;
  background: #e2e8f0;
}

.filter-label {
  font-size: 13px;
  color: #475569;
  white-space: nowrap;
  font-weight: 600;
}

.filter-group :deep(.el-select) {
  min-width: 120px;
}

.filter-stats {
  margin-left: auto;
}

.stats-count {
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
  white-space: nowrap;
}

.package-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.pagination-container {
  display: flex;
  justify-content: center;
  margin-top: 32px;
  padding: 24px 0;
}
</style>
