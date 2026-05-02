<template>
  <div class="file-browser">
    <el-card class="browser-card">
      <template #header>
        <div class="card-header">
          <span class="title">文件浏览器</span>
          <el-button @click="refresh" :icon="Refresh" circle />
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

      <el-table
        :data="files"
        v-loading="loading"
        @row-click="handleRowClick"
        style="width: 100%"
        :row-class-name="getRowClassName"
      >
        <el-table-column label="名称" min-width="300">
          <template #default="{ row }">
            <div class="file-name">
              <el-icon :size="20" :color="getFileIconColor(row)">
                <component :is="getFileIcon(row)" />
              </el-icon>
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="大小" width="120">
          <template #default="{ row }">
            <span v-if="!row.is_dir">{{ formatSize(row.size) }}</span>
            <span v-else class="directory-label">-</span>
          </template>
        </el-table-column>

        <el-table-column label="修改时间" width="180">
          <template #default="{ row }">
            {{ row.mod_time }}
          </template>
        </el-table-column>

        <el-table-column label="操作" width="150" align="center">
          <template #default="{ row }">
            <el-button
              v-if="!row.is_dir"
              link
              type="primary"
              @click.stop="downloadFile(row)"
            >
              <el-icon><Download /></el-icon>
              下载
            </el-button>
            <el-button
              v-else
              link
              type="primary"
              @click.stop="navigateTo(row.path)"
            >
              <el-icon><FolderOpened /></el-icon>
              打开
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="footer-info">
        <span>共 {{ files.length }} 个项目</span>
      </div>
    </el-card>
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

const pathSegments = computed(() => {
  if (currentPath.value === '/' || currentPath.value === '') {
    return []
  }
  return currentPath.value.split('/').filter(Boolean)
})

const loadDirectory = async (path: string) => {
  loading.value = true
  try {
    const response = await fileApi.browse(path)
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

const handleRowClick = (row: FileInfo) => {
  if (row.is_dir) {
    navigateTo(row.path)
  }
}

const getRowClassName = ({ row }: { row: FileInfo }) => {
  return row.is_dir ? 'directory-row' : 'file-row'
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
    const response = await fileApi.download(row.path)
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
  padding: 20px;
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
  font-size: 18px;
  font-weight: bold;
}

.breadcrumb-container {
  margin-bottom: 20px;
  padding: 15px;
  background-color: #f5f7fa;
  border-radius: 4px;
}

.breadcrumb-link {
  cursor: pointer;
  color: #409eff;
}

.breadcrumb-link:hover {
  text-decoration: underline;
}

.file-name {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}

.directory-label {
  color: #909399;
}

.directory-row {
  cursor: pointer;
}

.directory-row:hover {
  background-color: #f5f7fa;
}

.file-row:hover {
  background-color: #f5f7fa;
}

.footer-info {
  margin-top: 20px;
  padding-top: 15px;
  border-top: 1px solid #ebeef5;
  color: #909399;
  font-size: 14px;
}
</style>
