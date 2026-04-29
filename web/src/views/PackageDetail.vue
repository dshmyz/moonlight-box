<template>
  <div v-loading="loading" class="package-detail-page">
    <template v-if="!loading && pkg">
      <PackageHeader :pkg="pkg" />

      <VersionTable
        :versions="versions"
        :selected-version="selectedVersion"
        :show-admin-actions="isAdminRoute"
        @select="handleSelectVersion"
        @download="handleDownload"
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

    <el-empty v-if="!loading && !pkg" description="包不存在或已被移除" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import PackageHeader from '@/components/package-detail/PackageHeader.vue'
import PackageUsageGuide from '@/components/package-detail/PackageUsageGuide.vue'
import VersionTable from '@/components/package-detail/VersionTable.vue'
import PackageInfoSidebar from '@/components/package-detail/PackageInfoSidebar.vue'
import { ElMessage } from 'element-plus'
import { packageApi } from '@/api/package'

const route = useRoute()
const loading = ref(false)

// 判断是否为管理后台路由
const isAdminRoute = computed(() => {
  return route.path.startsWith('/admin')
})

interface PackageInfo {
  id: number
  name: string
  type: string
  description: string
  latest_version: string
  download_count: number
  repository: string
  license: string
}

interface VersionInfo {
  version: string
  published_at: string
  downloads: number
  is_latest: boolean
  size: number
  checksum: string
  status: string
}

const pkg = ref<PackageInfo | null>(null)
const versions = ref<VersionInfo[]>([])
const selectedVersion = ref('')

function handleSelectVersion(version: string) {
  selectedVersion.value = version
}

function handleDownload(version: VersionInfo) {
  ElMessage.info(`正在下载 ${pkg.value?.name}@${version.version}`)
}

async function handleDeprecate(data: { version: string; reason: string }) {
  try {
    const versionInfo = versions.value.find(v => v.version === data.version)
    if (versionInfo) {
      // 这里需要版本ID，暂时使用版本号作为标识
      // 实际使用时需要从 API 返回的数据中获取版本 ID
      ElMessage.success(`版本 ${data.version} 已废弃`)
      versionInfo.status = 'deprecated'
    }
  } catch (error) {
    ElMessage.error('废弃版本失败')
    console.error('Failed to deprecate version:', error)
  }
}

async function handleRestore(data: { version: string }) {
  try {
    const versionInfo = versions.value.find(v => v.version === data.version)
    if (versionInfo) {
      ElMessage.success(`版本 ${data.version} 已恢复`)
      versionInfo.status = 'published'
    }
  } catch (error) {
    ElMessage.error('恢复版本失败')
    console.error('Failed to restore version:', error)
  }
}

async function handleYank(data: { version: string }) {
  try {
    const versionInfo = versions.value.find(v => v.version === data.version)
    if (versionInfo) {
      ElMessage.success(`版本 ${data.version} 已撤回`)
      versionInfo.status = 'yanked'
    }
  } catch (error) {
    ElMessage.error('撤回版本失败')
    console.error('Failed to yank version:', error)
  }
}

async function handleDelete(data: { version: string }) {
  try {
    const index = versions.value.findIndex(v => v.version === data.version)
    if (index !== -1) {
      versions.value.splice(index, 1)
      ElMessage.success(`版本 ${data.version} 已删除`)
    }
  } catch (error) {
    ElMessage.error('删除版本失败')
    console.error('Failed to delete version:', error)
  }
}

onMounted(() => {
  loading.value = true
  setTimeout(() => {
    pkg.value = {
      id: 1,
      name: route.params.name as string,
      type: route.params.type as string,
      description: 'A modern JavaScript utility library delivering modularity, performance & extras. This package provides a wide range of functions for common programming tasks.',
      latest_version: '4.17.21',
      download_count: 50000000,
      repository: 'npm-virtual',
      license: 'MIT',
    }
    versions.value = [
      { version: '4.17.21', published_at: '2024-01-15', downloads: 50000000, is_latest: true, size: 832848, checksum: 'a324d7e46e299e0de3b', status: 'published' },
      { version: '4.17.20', published_at: '2023-06-10', downloads: 20000000, is_latest: false, size: 831200, checksum: 'b534e8f57f309f1ec4c', status: 'published' },
      { version: '4.17.19', published_at: '2023-01-05', downloads: 8000000, is_latest: false, size: 829600, checksum: 'c745f9g68g410g2fd5d', status: 'published' },
      { version: '4.17.15', published_at: '2022-09-20', downloads: 3000000, is_latest: false, size: 824000, checksum: 'd856g0h79h521h3ge6e', status: 'deprecated' },
      { version: '4.17.10', published_at: '2022-03-15', downloads: 1500000, is_latest: false, size: 818400, checksum: 'e967h1i80i632i4hf7f', status: 'yanked' },
    ]
    selectedVersion.value = pkg.value.latest_version
    loading.value = false
  }, 500)
})
</script>

<style scoped>
.package-detail-page {
  min-height: 400px;
}
</style>
