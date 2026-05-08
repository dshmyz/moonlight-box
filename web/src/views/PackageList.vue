<template>
  <div class="package-list">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">📦</div>
        <div class="header-text">
          <h2>包管理</h2>
          <p class="header-subtitle">管理和分发您的软件包</p>
        </div>
      </div>
      <el-button type="primary" class="upload-btn" @click="showUploadDialog = true">
        <el-icon><Upload /></el-icon>
        <span>上传包</span>
      </el-button>
    </header>

    <div class="toolbar">
      <div class="search-wrapper">
        <el-input
          v-model="searchQuery"
          placeholder="搜索包名或描述..."
          clearable
          class="search-input"
          @input="handleSearch"
          @keyup.enter="loadPackages"
        >
          <template #prefix>
            <Search class="search-icon" />
          </template>
        </el-input>
      </div>

      <div class="type-filters">
        <el-radio-group v-model="filterType" @change="handleFilter">
          <el-radio-button label="">全部</el-radio-button>
          <el-radio-button label="npm">npm</el-radio-button>
          <el-radio-button label="maven">Maven</el-radio-button>
          <el-radio-button label="pypi">PyPI</el-radio-button>
          <el-radio-button label="go">Go</el-radio-button>
          <el-radio-button label="yum">Yum</el-radio-button>
          <el-radio-button label="apt">Apt</el-radio-button>
          <el-radio-button label="generic">Generic</el-radio-button>
        </el-radio-group>
      </div>

      <div class="toolbar-actions">
        <el-select v-model="sortBy" class="sort-select" @change="handleSort">
          <el-option v-for="opt in sortOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
        </el-select>

        <div class="view-toggle">
          <el-button :class="{ active: viewMode === 'table' }" @click="viewMode = 'table'">
            <el-icon><List /></el-icon>
          </el-button>
          <el-button :class="{ active: viewMode === 'card' }" @click="viewMode = 'card'">
            <el-icon><Grid /></el-icon>
          </el-button>
        </div>
      </div>
    </div>

    <div class="content-panel" v-loading="loading">
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

      <div class="list-footer" v-if="total > 0">
        <div class="footer-info">
          <span class="total-badge">{{ total }}</span>
          <span class="total-label">个包</span>
        </div>
        <el-pagination
          v-model:current-page="currentPage"
          :page-size="pageSize"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="sizes, prev, pager, next"
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
        />
      </div>
    </div>

    <VersionDrawer
      v-model="showVersionDrawer"
      :package-type="selectedPackage?.type || ''"
      :package-name="selectedPackage?.name || ''"
    />

    <UploadPackageDialog
      v-model="showUploadDialog"
      @uploaded="handleUploadSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Search, Upload, List, Grid } from '@element-plus/icons-vue'
import PackageTable from '@/components/package/PackageTable.vue'
import PackageCards from '@/components/package/PackageCards.vue'
import VersionDrawer from '@/components/package/VersionDrawer.vue'
import UploadPackageDialog from '@/components/package/UploadPackageDialog.vue'
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
const showUploadDialog = ref(false)

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

function handleUploadSuccess() {
  loadPackages()
}

onMounted(() => {
  loadPackages()
})
</script>

<style scoped>
.package-list {
  min-height: calc(100vh - 60px);
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 28px;
  padding: 20px 24px;
  background: #ffffff;
  border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 20px;
}

.header-text h2 {
  font-size: 22px;
  font-weight: 700;
  margin: 0;
  color: #1e293b;
  letter-spacing: -0.02em;
}

.header-subtitle {
  font-size: 13px;
  color: #64748b;
  margin: 4px 0 0;
}

.upload-btn {
  height: 42px;
  padding: 0 24px;
  border-radius: 10px;
  font-weight: 600;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border: none;
  box-shadow: 0 4px 14px rgba(37, 99, 235, 0.3);
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.upload-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 24px rgba(37, 99, 235, 0.35);
}

.upload-btn:active {
  transform: translateY(0);
}

.upload-btn .el-icon {
  font-size: 16px;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-bottom: 24px;
  padding: 16px 20px;
  background: #ffffff;
  border-radius: 12px;
  border: 1px solid rgba(0, 0, 0, 0.04);
  flex-wrap: wrap;
}

.search-wrapper {
  position: relative;
  flex-shrink: 0;
}

.search-input {
  width: 280px;
}

.search-input :deep(.el-input__wrapper) {
  border-radius: 10px;
  box-shadow: none;
  border: 1px solid #e2e8f0;
  padding: 0 16px;
  height: 40px;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  background: #f8fafc;
}

.search-input :deep(.el-input__wrapper:hover) {
  border-color: #cbd5e1;
  background: #fff;
}

.search-input :deep(.el-input__wrapper.is-focus) {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
  background: #fff;
}

.search-input :deep(.el-input__inner) {
  height: 38px;
  font-size: 14px;
  color: #1e293b;
}

.search-input :deep(.el-input__inner::placeholder) {
  color: #94a3b8;
}

.search-icon {
  width: 16px;
  height: 16px;
  color: #94a3b8;
}

.type-filters :deep(.el-radio-group) {
  display: flex;
  gap: 8px;
}

.type-filters :deep(.el-radio-button__inner) {
  padding: 9px 18px;
  font-size: 13px;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  color: #64748b;
  box-shadow: none;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.type-filters :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border-color: #6366f1;
  color: #fff;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
}

.type-filters :deep(.el-radio-button:first-child .el-radio-button__inner) {
  border-radius: 8px;
}

.type-filters :deep(.el-radio-button:last-child .el-radio-button__inner) {
  border-radius: 8px;
}

.type-filters :deep(.el-radio-button__inner:hover) {
  color: #4f46e5;
  border-color: #c7d2fe;
  background: #f0f1ff;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-left: auto;
}

.sort-select {
  width: 140px;
}

.sort-select :deep(.el-input__wrapper) {
  border-radius: 8px;
  box-shadow: none;
  border: 1px solid #e2e8f0;
  padding: 0 14px;
  height: 40px;
  font-size: 13px;
  background: #f8fafc;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.sort-select :deep(.el-input__wrapper:hover) {
  border-color: #cbd5e1;
  background: #fff;
}

.sort-select :deep(.el-input__wrapper.is-focus) {
  border-color: #6366f1;
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
  background: #fff;
}

.sort-select :deep(.el-input__inner) {
  color: #1e293b;
  font-size: 13px;
}

.sort-select :deep(.el-select-dropdown) {
  border-radius: 10px;
  border: 1px solid #e2e8f0;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08);
  overflow: hidden;
}

.sort-select :deep(.el-select-dropdown__item) {
  padding: 10px 16px;
  font-size: 13px;
  color: #475569;
  transition: all 0.15s ease;
}

.sort-select :deep(.el-select-dropdown__item:hover) {
  background: #f1f5f9;
  color: #1e293b;
}

.sort-select :deep(.el-select-dropdown__item.selected) {
  color: #6366f1;
  font-weight: 600;
  background: #f0f1ff;
}

.sort-select :deep(.el-popper.is-light .el-popper__arrow::before) {
  background: #fff;
  border-color: #e2e8f0;
}

.view-toggle {
  display: flex;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  overflow: hidden;
  background: #f8fafc;
}

.view-toggle .el-button {
  border: none;
  border-radius: 0;
  padding: 10px 16px;
  background: transparent;
  color: #64748b;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.view-toggle .el-button:hover {
  background: #f1f5f9;
  color: #475569;
}

.view-toggle .el-button.active {
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  color: #fff;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.3);
}

.view-toggle .el-button + .el-button {
  border-left: 1px solid #e2e8f0;
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  overflow: hidden;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.04);
}

.list-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 18px 24px;
  border-top: 1px solid rgba(0, 0, 0, 0.04);
  background: #fafbfc;
}

.footer-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.total-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  height: 26px;
  padding: 0 12px;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.3);
}

.total-label {
  font-size: 13px;
  color: #64748b;
}

.list-footer :deep(.el-pagination) {
  font-size: 13px;
}

.list-footer :deep(.el-pagination button) {
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  background: #fff;
  transition: all 0.2s ease;
}

.list-footer :deep(.el-pagination button:hover) {
  background: #f1f5f9;
  border-color: #cbd5e1;
}

.list-footer :deep(.el-pagination .btn-next),
.list-footer :deep(.el-pagination .btn-prev) {
  padding: 6px 12px;
}

.list-footer :deep(.el-pager li) {
  margin: 0 4px;
  width: 28px;
  height: 28px;
  line-height: 28px;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
}

.list-footer :deep(.el-pager li:hover) {
  background: #f1f5f9;
  border-color: #cbd5e1;
}

.list-footer :deep(.el-pager li.active) {
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border-color: #6366f1;
  color: #fff;
}

.list-footer :deep(.el-pagination__sizes .el-select .el-input__wrapper) {
  border-radius: 8px;
  border: 1px solid #e2e8f0;
}

.list-footer :deep(.el-pager li) {
  border-radius: 6px;
  min-width: 30px;
}

.list-footer :deep(.el-pager li.is-active) {
  background: #2563eb;
}
</style>
