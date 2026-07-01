<template>
  <div class="package-detail-page">
    <!-- 骨架屏：首次加载时显示 -->
    <div v-if="isInitialLoading" class="skeleton-container">
      <div class="skeleton-header" />
      <div class="skeleton-table" />
      <div class="skeleton-content">
        <div class="skeleton-main" />
        <div class="skeleton-sidebar" />
      </div>
    </div>

    <!-- 实际内容 -->
    <template v-else-if="pkg">
      <PackageHeader :pkg="pkg" @deleted="handlePackageDeleted" />

      <VersionTable
        :versions="versions"
        :selected-version="selectedVersion"
        :show-admin-actions="isAdminRoute"
        :pkg-type="route.params.type as string"
        :pkg-name="route.params.name as string"
        :repository-id="pkg?.repository_id"
        @select="handleSelectVersion"
        @deprecate="handleDeprecate"
        @restore="handleRestore"
        @yank="handleYank"
        @delete="handleDelete"
      />

      <el-row :gutter="24">
        <el-col :xs="24" :lg="16">
          <PackageUsageGuide :pkg="pkg" :selected-version="selectedVersion" />
        </el-col>
        <el-col :xs="24" :lg="8">
          <PackageInfoSidebar :pkg="pkg" :versions="versions" :selected-version="selectedVersion" />
        </el-col>
      </el-row>
    </template>

    <el-empty v-else description="包不存在或已被移除" />

    <!-- 后台刷新指示器 -->
    <div v-if="isRefreshing && !isInitialLoading" class="refresh-indicator">
      <el-icon class="is-loading"><Refresh /></el-icon>
      刷新中...
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import PackageHeader from '@/components/package-detail/PackageHeader.vue'
import PackageUsageGuide from '@/components/package-detail/PackageUsageGuide.vue'
import VersionTable from '@/components/package-detail/VersionTable.vue'
import PackageInfoSidebar from '@/components/package-detail/PackageInfoSidebar.vue'
import { ElMessage } from 'element-plus'
import { packageApi, type Package, type PackageVersion, type PackageVersionTarget } from '@/api/package'

// 包详情缓存
interface PackageCache {
  pkg: Package & { repository?: string }
  versions: PackageVersion[]
  timestamp: number
}

interface VersionActionData {
  id: number
  version: string
  name?: string
  repository_id?: number
}

const CACHE_TTL = 5 * 60 * 1000 // 缓存 5 分钟
const packageCache = new Map<string, PackageCache>()
const lastSelectedVersionCache = new Map<string, string>() // 记忆用户选择的版本

const route = useRoute()
const loading = ref(false)
const isInitialLoading = ref(true) // 是否首次加载（需要显示骨架屏）
const isRefreshing = ref(false) // 是否正在后台刷新

const isAdminRoute = computed(() => {
  return route.path.startsWith('/admin')
})

const pkg = ref<(Package & { repository?: string }) | null>(null)
const versions = ref<PackageVersion[]>([])
const selectedVersion = ref('')

function getCacheKey(type: string, name: string) {
  return `${type}:${decodeURIComponent(name)}`
}

// 优先从缓存加载，同时后台刷新
async function loadFromCacheOrFetch(type: string, name: string, forceRefresh = false) {
  const cacheKey = getCacheKey(type, name)
  const cached = packageCache.get(cacheKey)

  if (cached && !forceRefresh) {
    // 优先使用缓存数据 - 秒开！
    const { pkg: cachedPkg, versions: cachedVersions } = cached
    pkg.value = cachedPkg
    versions.value = cachedVersions
    isInitialLoading.value = false

    // 恢复用户上次选择的版本，或默认选择最新版本
    const lastSelected = lastSelectedVersionCache.get(cacheKey)
    if (lastSelected && cachedVersions.some(v => v.version === lastSelected)) {
      selectedVersion.value = lastSelected
    } else {
      const latest = cachedVersions.find((v) => v.status === 'published')
      selectedVersion.value = latest?.version || cachedVersions[0].version
    }

    // 检查缓存是否过期，如果过期则后台刷新
    if (Date.now() - cached.timestamp > CACHE_TTL) {
      refreshInBackground(type, name)
    }
    return
  }

  // 没有缓存或强制刷新，发起请求
  loading.value = true
  await fetchPackageDetail(type, name)
  isInitialLoading.value = false
}

// 后台刷新数据（静默更新，不阻塞 UI）
async function refreshInBackground(type: string, name: string) {
  if (isRefreshing.value) return
  isRefreshing.value = true

  try {
    await fetchAndCachePackageDetail(type, name, false)
    // 如果数据有变化，可以显示一个小的刷新提示
  } catch (error) {
    // 静默失败，不影响用户
  } finally {
    isRefreshing.value = false
  }
}

async function fetchPackageDetail(type: string, name: string) {
  const cacheKey = getCacheKey(type, name)
  loading.value = true

  try {
    const [searchResult, versionResult] = await Promise.all([
      packageApi.search({
        name: name,
        type: type,
        page: 1,
        page_size: 1,
      }),
      packageApi.getVersions(type, name),
    ])

    const list = (searchResult.list || []).map(p => ({
      ...p,
      type: p.type || p.package_type || p.format || 'generic',
    }))
    const found = list.find(
      (p) => p.name === name && p.type === type
    )

    if (found) {
      const pkgData = {
        ...found,
        repository: found.repository_name && found.repository_name.trim() !== ''
          ? found.repository_name
          : 'default'
      }
      pkg.value = pkgData

      const versionList = versionResult.versions || []
      versions.value = versionList

      // 默认选中最新版本
      const latest = versionList.find((v) => v.status === 'published')
      selectedVersion.value = latest?.version || versionList[0]?.version || ''

      if (pkg.value && !pkg.value.latest_version) {
        pkg.value.latest_version = selectedVersion.value
      }

      // 更新缓存
      packageCache.set(cacheKey, {
        pkg: pkgData,
        versions: versionList,
        timestamp: Date.now()
      })
    } else if (versionResult.versions?.length) {
      // 没有找到包信息但有版本
      const versionList = versionResult.versions || []
      versions.value = versionList
      const latest = versionList.find((v) => v.status === 'published')
      selectedVersion.value = latest?.version || versionList[0].version

      pkg.value = {
        id: 0,
        name: name,
        display_name: name,
        type: type,
        description: '',
        download_count: 0,
        updated_at: '',
        repository: 'default',
      }
    }
  } catch (error) {
    console.error('Failed to load package info:', error)
    ElMessage.error('加载包详情失败')
  } finally {
    loading.value = false
  }
}

async function fetchAndCachePackageDetail(type: string, name: string, showLoading = true) {
  if (showLoading) loading.value = true

  try {
    const cacheKey = getCacheKey(type, name)
    const [searchResult, versionResult] = await Promise.all([
      packageApi.search({
        name: name,
        type: type,
        page: 1,
        page_size: 1,
      }),
      packageApi.getVersions(type, name),
    ])

    const list = (searchResult.list || []).map(p => ({
      ...p,
      type: p.type || p.package_type || p.format || 'generic',
    }))
    const found = list.find(
      (p) => p.name === name && p.type === type
    )

    if (found) {
      const pkgData = {
        ...found,
        repository: found.repository_name && found.repository_name.trim() !== ''
          ? found.repository_name
          : 'default'
      }

      const versionList = versionResult.versions || []

      // 更新缓存
      packageCache.set(cacheKey, {
        pkg: pkgData,
        versions: versionList,
        timestamp: Date.now()
      })

      // 如果当前显示的就是这个包，更新 UI
      const currentCacheKey = getCacheKey(
        route.params.type as string,
        route.params.name as string
      )
      if (cacheKey === currentCacheKey) {
        pkg.value = pkgData
        versions.value = versionList

        // 如果当前选中的版本在新版本列表中，保留选择
        if (!versionList.some(v => v.version === selectedVersion.value)) {
          const latest = versionList.find((v) => v.status === 'published')
          selectedVersion.value = latest?.version || versionList[0]?.version || ''
        }
      }
    }
  } catch (error) {
    console.error('Failed to refresh package:', error)
  } finally {
    if (showLoading) loading.value = false
  }
}

function handleSelectVersion(version: string) {
  // 记住用户选择的版本
  const cacheKey = getCacheKey(route.params.type as string, route.params.name as string)
  lastSelectedVersionCache.set(cacheKey, version)
  selectedVersion.value = version
}

// 监听路由变化，切换包时重新加载
watch(() => [route.params.type, route.params.name], ([newType, newName]) => {
  if (newType && newName) {
    loadFromCacheOrFetch(newType as string, newName as string)
  }
})

onMounted(() => {
  const pkgType = route.params.type as string
  const pkgName = route.params.name as string
  if (pkgType && pkgName) {
    loadFromCacheOrFetch(pkgType, pkgName)
  }
})

function toVersionTarget(data: VersionActionData): PackageVersionTarget {
  return {
    type: route.params.type as string,
    name: data.name || (route.params.name as string),
    version: data.version,
    repository_id: data.repository_id || pkg.value?.repository_id,
  }
}

async function handleDeprecate(data: VersionActionData & { reason: string }) {
  try {
    await packageApi.deprecatePackageVersion(toVersionTarget(data), data.reason)
    updateLocalVersionStatus(data.version, 'deprecated')
    // 更新缓存
    updateCacheVersionStatus(data.version, 'deprecated')
    ElMessage.success(`版本 ${data.version} 已废弃`)
  } catch (error) {
    ElMessage.error('废弃版本失败')
    console.error('Failed to deprecate version:', error)
  }
}

async function handleRestore(data: VersionActionData) {
  try {
    await packageApi.restorePackageVersion(toVersionTarget(data))
    updateLocalVersionStatus(data.version, 'published')
    // 更新缓存
    updateCacheVersionStatus(data.version, 'published')
    ElMessage.success(`版本 ${data.version} 已恢复`)
  } catch (error) {
    ElMessage.error('恢复版本失败')
    console.error('Failed to restore version:', error)
  }
}

async function handleYank(data: VersionActionData) {
  try {
    await packageApi.yankPackageVersion(toVersionTarget(data))
    updateLocalVersionStatus(data.version, 'yanked')
    // 更新缓存
    updateCacheVersionStatus(data.version, 'yanked')
    ElMessage.success(`版本 ${data.version} 已撤回`)
  } catch (error) {
    ElMessage.error('撤回版本失败')
    console.error('Failed to yank version:', error)
  }
}

async function handleDelete(data: VersionActionData) {
  try {
    await packageApi.deletePackageVersion(toVersionTarget(data))
    const index = versions.value.findIndex((v) => v.version === data.version)
    if (index !== -1) versions.value.splice(index, 1)
    // 从缓存中移除
    removeVersionFromCache(data.version)
    ElMessage.success(`版本 ${data.version} 已删除`)
  } catch (error) {
    ElMessage.error('删除版本失败')
    console.error('Failed to delete version:', error)
  }
}

function updateLocalVersionStatus(version: string, status: string) {
  versions.value
    .filter((v) => v.version === version)
    .forEach((target) => {
      target.status = status
    })
}

// 更新缓存中的版本状态
function updateCacheVersionStatus(version: string, status: string) {
  const cacheKey = getCacheKey(route.params.type as string, route.params.name as string)
  const cached = packageCache.get(cacheKey)
  if (cached) {
    cached.versions
      .filter((v) => v.version === version)
      .forEach((target) => {
        target.status = status
      })
    cached.timestamp = Date.now() // 更新缓存时间
  }
}

// 从缓存中移除版本
function removeVersionFromCache(version: string) {
  const cacheKey = getCacheKey(route.params.type as string, route.params.name as string)
  const cached = packageCache.get(cacheKey)
  if (cached) {
    const index = cached.versions.findIndex((v) => v.version === version)
    if (index !== -1) cached.versions.splice(index, 1)
    cached.timestamp = Date.now()
  }
}

function handlePackageDeleted() {
  // 包已删除，导航已在 PackageHeader 中处理
}

</script>

<style scoped>
.package-detail-page {
  min-height: 400px;
}

/* 骨架屏 */
.skeleton-container {
  animation: fadeIn 0.3s ease;
}

.skeleton-header {
  height: 180px;
  background: var(--lunar-bg-card);
  border: 1px solid var(--lunar-border);
  border-radius: 10px;
  margin-bottom: 24px;
  animation: skeletonPulse 1.5s ease-in-out infinite;
}

.skeleton-table {
  height: 400px;
  background: var(--lunar-bg-card);
  border: 1px solid var(--lunar-border);
  border-radius: 10px;
  margin-bottom: 24px;
  animation: skeletonPulse 1.5s ease-in-out infinite;
  animation-delay: 0.1s;
}

.skeleton-content {
  display: grid;
  grid-template-columns: 1fr 320px;
  gap: 24px;
}

.skeleton-main {
  height: 300px;
  background: var(--lunar-bg-card);
  border: 1px solid var(--lunar-border);
  border-radius: 10px;
  animation: skeletonPulse 1.5s ease-in-out infinite;
  animation-delay: 0.2s;
}

.skeleton-sidebar {
  height: 400px;
  background: var(--lunar-bg-card);
  border: 1px solid var(--lunar-border);
  border-radius: 10px;
  animation: skeletonPulse 1.5s ease-in-out infinite;
  animation-delay: 0.3s;
}

@keyframes skeletonPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.7; }
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

/* 后台刷新指示器 */
.refresh-indicator {
  position: fixed;
  bottom: 24px;
  right: 24px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: var(--lunar-bg-glass);
  border: 1px solid var(--lunar-border);
  border-radius: 8px;
  font-size: 13px;
  color: var(--lunar-silver-muted);
  backdrop-filter: blur(8px);
  z-index: 100;
}

.refresh-indicator .el-icon {
  color: var(--lunar-accent);
}

@media (max-width: 992px) {
  .skeleton-content {
    grid-template-columns: 1fr;
  }

  .skeleton-sidebar {
    height: 300px;
  }
}
</style>
