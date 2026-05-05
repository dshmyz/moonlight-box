<template>
  <div class="file-browser">
    <CustomCard title="文件浏览器" hoverable class="browser-card">
      <template #header>
        <div class="card-header">
          <span class="title">文件浏览器</span>
          <CustomButton :icon="Refresh" size="small" @click="refresh" />
        </div>
      </template>

      <div class="breadcrumb-container">
        <el-breadcrumb separator="/">
          <el-breadcrumb-item @click="navigateTo('/')">
            <el-icon><HomeFilled /></el-icon>
            <span>根目录</span>
          </el-breadcrumb-item>
          <el-breadcrumb-item
            v-for="(segment, index) in pathSegments"
            :key="index"
            @click="navigateToSegment(index)"
          >
            <span class="breadcrumb-link">{{ segment }}</span>
          </el-breadcrumb-item>
        </el-breadcrumb>
      </div>

      <CustomTable :columns="columns" :data="files" :loading="loading" row-key="path">
        <template #name="{ row }">
          <div class="file-name" @click="row.is_dir && navigateTo(row.path)">
            <el-icon :size="20" :color="getFileIconColor(row)">
              <component :is="getFileIcon(row)" />
            </el-icon>
            <span>{{ row.name }}</span>
          </div>
        </template>
        <template #size="{ row }">
          <span v-if="!row.is_dir">{{ formatSize(row.size) }}</span>
          <span v-else class="directory-label">-</span>
        </template>
        <template #mod_time="{ row }">
          {{ row.mod_time }}
        </template>
        <template #actions="{ row }">
          <CustomButton
            v-if="!row.is_dir"
            type="text"
            size="small"
            @click.stop="downloadFile(row)"
          >
            <el-icon><Download /></el-icon>
            下载
          </CustomButton>
          <CustomButton
            v-else
            type="text"
            size="small"
            @click.stop="navigateTo(row.path)"
          >
            <el-icon><FolderOpened /></el-icon>
            打开
          </CustomButton>
        </template>
      </CustomTable>

      <div class="footer-info">
        <span>共 {{ files.length }} 个项目</span>
      </div>
    </CustomCard>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Refresh,
  HomeFilled,
  Folder,
  Document,
  Download,
  FolderOpened,
} from '@element-plus/icons-vue'
import { fileApi } from '@/api/file'
import CustomCard from '@/components/ui/CustomCard.vue'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomTable from '@/components/ui/CustomTable.vue'

interface FileInfo {
  name: string
  path: string
  is_dir: boolean
  size: number
  mod_time: string
}

const loading = ref(false)
const currentPath = ref('/')
const files = ref<FileInfo[]>([])

const columns = [
  { prop: 'name', label: '名称' },
  { prop: 'size', label: '大小', width: '120px' },
  { prop: 'mod_time', label: '修改时间', width: '180px' },
  { prop: 'actions', label: '操作', width: '150px', align: 'center' as const },
]

const pathSegments = computed(() => {
  if (currentPath.value === '/' || currentPath.value === '') {
    return []
  }
  return currentPath.value.split('/').filter(Boolean)
})

const loadDirectory = async (path: string) => {
  loading.value = true
  try {
    const response = await fileApi.browse(path) as any
    files.value = response.files || []
    currentPath.value = path
  } catch (error: any) {
    ElMessage.error(error.message || '加载目录失败')
  } finally {
    loading.value = false
  }
}

const navigateTo = (path: string) => {
  loadDirectory(path)
}

const navigateToSegment = (index: number) => {
  const segments = pathSegments.value.slice(0, index + 1)
  const path = '/' + segments.join('/')
  navigateTo(path)
}

const refresh = () => {
  loadDirectory(currentPath.value)
}

const getFileIcon = (row: FileInfo) => {
  return row.is_dir ? Folder : Document
}

const getFileIconColor = (row: FileInfo) => {
  return row.is_dir ? '#409EFF' : '#909399'
}

const formatSize = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i]
}

const downloadFile = async (row: FileInfo) => {
  try {
    const response = await fileApi.download(row.path) as any
    const url = window.URL.createObjectURL(new Blob([response.data]))
    const link = document.createElement('a')
    link.href = url
    link.setAttribute('download', row.name)
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    window.URL.revokeObjectURL(url)
    ElMessage.success('下载成功')
  } catch (error: any) {
    ElMessage.error(error.message || '下载失败')
  }
}

onMounted(() => {
  loadDirectory('/')
})
</script>

<style scoped>
.file-browser {
  padding: var(--spacing-xl);
}

.browser-card {
  min-height: calc(100vh - 140px);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-header .title {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-bold);
  color: var(--color-text-primary);
}

.breadcrumb-container {
  margin-bottom: var(--spacing-lg);
  padding: var(--spacing-md);
  background-color: var(--color-bg-hover);
  border-radius: var(--radius-sm);
}

.breadcrumb-link {
  cursor: pointer;
  color: var(--color-primary);
}

.breadcrumb-link:hover {
  text-decoration: underline;
}

.file-name {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  cursor: pointer;
}

.directory-label {
  color: var(--color-text-tertiary);
}

.footer-info {
  margin-top: var(--spacing-lg);
  padding-top: var(--spacing-md);
  border-top: 1px solid var(--color-border);
  color: var(--color-text-tertiary);
  font-size: var(--font-size-sm);
}
</style>
