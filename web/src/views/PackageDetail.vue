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
import { packageApi, type Package, type PackageVersion } from '@/api/package'

const route = useRoute()
const loading = ref(false)

const isAdminRoute = computed(() => {
  return route.path.startsWith('/admin')
})

const pkg = ref<(Package & { repository?: string }) | null>(null)
const versions = ref<PackageVersion[]>([])
const selectedVersion = ref('')

async function loadPackageDetail() {
  const pkgType = route.params.type as string
  const pkgName = route.params.name as string
  if (!pkgType || !pkgName) return

  loading.value = true
  try {
    const searchResult = await packageApi.search({
      q: pkgName,
      type: pkgType,
      page: 1,
      page_size: 1,
    })
    const found = searchResult.list?.find(
      (p) => p.name === pkgName && p.type === pkgType
    )
    if (found) {
      pkg.value = { 
        ...found, 
        repository: found.repository_name && found.repository_name.trim() !== '' 
          ? found.repository_name 
          : 'default' 
      }
    }
  } catch (error) {
    console.error('Failed to load package info:', error)
  }

  try {
    const versionResult = await packageApi.getVersions(pkgType, pkgName)
    const versionList = versionResult.versions || []
    versions.value = versionList
    if (versionList.length > 0) {
      const latest = versionList.find((v) => v.status === 'published')
      selectedVersion.value = latest?.version || versionList[0].version
      if (pkg.value && !pkg.value.latest_version) {
        pkg.value.latest_version = selectedVersion.value
      }
    }
  } catch (error) {
    console.error('Failed to load versions:', error)
    if (!pkg.value) {
      ElMessage.error('加载包详情失败')
    }
  }

  if (!pkg.value && versions.value.length > 0) {
    pkg.value = {
      id: 0,
      name: pkgName,
      display_name: pkgName,
      type: pkgType,
      description: '',
      download_count: 0,
      updated_at: '',
      repository: 'default',
    }
  }

  loading.value = false
}

function handleSelectVersion(version: string) {
  selectedVersion.value = version
}

async function handleDownload(version: PackageVersion & { selectedFile?: any }) {
  if (!pkg.value || !version.files || version.files.length === 0) {
    ElMessage.error('没有可下载的文件')
    return
  }

  let file = version.selectedFile || version.files[0]
  
  if (!version.selectedFile && pkg.value.type === 'maven') {
    const primaryFile = version.files.find(f => f.file_type === 'primary')
    if (primaryFile) {
      file = primaryFile
    }
  }
  
  let downloadUrl = ''
  let downloadFilename = file.filename
  
  if (pkg.value.type === 'go') {
    downloadUrl = `/repo/${pkg.value.repository}/${pkg.value.name}/@v/${version.version}.zip`
    downloadFilename = `${version.version}.zip`
  } else if (pkg.value.type === 'maven') {
    const parts = pkg.value.name.split(':')
    if (parts.length === 2) {
      const groupPath = parts[0].replace(/\./g, '/')
      const extension = file.filename.endsWith('.pom') ? 'pom' : 
                       file.filename.endsWith('-sources.jar') ? 'sources.jar' :
                       file.filename.endsWith('-javadoc.jar') ? 'javadoc.jar' : 'jar'
      downloadUrl = `/repo/${pkg.value.repository}/${groupPath}/${parts[1]}/${version.version}/${file.filename}`
      downloadFilename = file.filename
    } else {
      downloadUrl = `/repo/${pkg.value.repository}/${pkg.value.name}/${version.version}/${file.filename}`
    }
  } else if (pkg.value.type === 'pypi') {
    downloadUrl = `/repo/${pkg.value.repository}/packages/${file.filename}`
  } else {
    downloadUrl = `/repo/${pkg.value.repository}/${pkg.value.name}/-/${file.filename}`
  }
  
  try {
    ElMessage.info(`开始下载 ${pkg.value.name}@${version.version} - ${file.filename}`)
    
    const response = await fetch(downloadUrl)
    if (!response.ok) {
      throw new Error(`下载失败: ${response.status} ${response.statusText}`)
    }
    
    const blob = await response.blob()
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = downloadFilename
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    window.URL.revokeObjectURL(url)
    
    ElMessage.success(`下载完成 ${file.filename}`)
  } catch (error) {
    console.error('Download failed:', error)
    ElMessage.error(`下载失败: ${error}`)
  }
}

async function handleDeprecate(data: { id: number; version: string; reason: string }) {
  try {
    await packageApi.deprecateVersion(data.id, data.reason)
    const target = versions.value.find((v) => v.id === data.id)
    if (target) target.status = 'deprecated'
    ElMessage.success(`版本 ${data.version} 已废弃`)
  } catch (error) {
    ElMessage.error('废弃版本失败')
    console.error('Failed to deprecate version:', error)
  }
}

async function handleRestore(data: { id: number; version: string }) {
  try {
    await packageApi.restoreVersion(data.id)
    const target = versions.value.find((v) => v.id === data.id)
    if (target) target.status = 'published'
    ElMessage.success(`版本 ${data.version} 已恢复`)
  } catch (error) {
    ElMessage.error('恢复版本失败')
    console.error('Failed to restore version:', error)
  }
}

async function handleYank(data: { id: number; version: string }) {
  try {
    await packageApi.yankVersion(data.id)
    const target = versions.value.find((v) => v.id === data.id)
    if (target) target.status = 'yanked'
    ElMessage.success(`版本 ${data.version} 已撤回`)
  } catch (error) {
    ElMessage.error('撤回版本失败')
    console.error('Failed to yank version:', error)
  }
}

async function handleDelete(data: { id: number; version: string }) {
  try {
    await packageApi.deleteVersion(data.id)
    const index = versions.value.findIndex((v) => v.id === data.id)
    if (index !== -1) versions.value.splice(index, 1)
    ElMessage.success(`版本 ${data.version} 已删除`)
  } catch (error) {
    ElMessage.error('删除版本失败')
    console.error('Failed to delete version:', error)
  }
}

onMounted(() => {
  loadPackageDetail()
})
</script>

<style scoped>
.package-detail-page {
  min-height: 400px;
}
</style>
