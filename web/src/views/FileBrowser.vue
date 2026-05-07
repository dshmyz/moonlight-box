<template>
  <div class="file-browser">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-folder-open"></i>
        </div>
        <div class="header-text">
          <h2>文件浏览器</h2>
          <p class="header-subtitle">浏览和管理存储中的文件</p>
        </div>
      </div>
      <el-button class="refresh-btn" @click="refresh">
        <i class="fa-solid fa-refresh"></i>
        <span>刷新</span>
      </el-button>
    </header>

    <div class="content-panel" v-loading="loading">
      <div class="breadcrumb-container">
        <el-breadcrumb separator="/">
          <el-breadcrumb-item @click="navigateTo('/')">
            <i class="fa-solid fa-home"></i>
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
        style="width: 100%"
        :header-cell-style="{ background: '#fafbfc' }"
        :row-class-name="getRowClassName"
        @row-click="handleRowClick"
      >
        <el-table-column label="名称" min-width="250">
          <template #default="{ row }">
            <div class="file-name">
              <div class="file-icon" :class="{ 'file-icon--dir': row.is_dir }">
                <i v-if="row.is_dir" class="fa-solid fa-folder"></i>
                <i v-else class="fa-solid fa-file"></i>
              </div>
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="大小" width="120" align="center">
          <template #default="{ row }">
            <span v-if="!row.is_dir" class="file-size">{{ formatSize(row.size) }}</span>
            <span v-else class="directory-label">-</span>
          </template>
        </el-table-column>

        <el-table-column label="修改时间" width="180" align="center">
          <template #default="{ row }">
            <span class="file-time">{{ row.mod_time }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="140" align="center">
          <template #default="{ row }">
            <div class="operation-buttons">
              <el-button
                v-if="!row.is_dir"
                class="btn-download"
                size="small"
                @click.stop="downloadFile(row)"
              >
                <i class="fa-solid fa-download"></i>
                <span>下载</span>
              </el-button>
              <el-button
                v-else
                class="btn-open"
                size="small"
                @click.stop="navigateTo(row.path)"
              >
                <i class="fa-solid fa-arrow-right"></i>
                <span>打开</span>
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="footer-info">
        <span class="footer-count">共 {{ files.length }} 个项目</span>
        <span class="footer-hint">
          <i class="fa-solid fa-info-circle"></i>
          点击目录进入，点击文件下载
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
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

const formatSize = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i]
}

const downloadFile = async (row: FileInfo) => {
  try {
    const blob = await fileApi.download(row.path) as unknown as Blob
    const url = window.URL.createObjectURL(new Blob([blob]))
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
  /* padding: 20px; */
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  background: #fff;
  border-radius: 16px;
  margin-bottom: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 24px;
}

.header-text h2 {
  font-size: 20px;
  font-weight: 600;
  margin: 0;
  color: #1f2937;
  letter-spacing: -0.2px;
}

.header-subtitle {
  font-size: 13px;
  color: #9ca3af;
  margin: 4px 0 0;
}

.refresh-btn {
  height: 40px;
  padding: 0 20px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: #f3f4f6;
  border-color: #e5e7eb;
  color: #374151;
  transition: all 0.2s ease;
}

.refresh-btn:hover {
  background: #e5e7eb;
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.breadcrumb-container {
  margin-bottom: 20px;
  padding: 16px 20px;
  background: #f8fafc;
  border-radius: 10px;
  border: 1px solid #e5e7eb;
}

:deep(.el-breadcrumb) {
  font-size: 14px;
}

:deep(.el-breadcrumb__item) {
  cursor: pointer;
}

:deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
  color: #1f2937;
  font-weight: 500;
}

:deep(.el-breadcrumb__inner) {
  color: #6b7280;
  display: flex;
  align-items: center;
  gap: 6px;
}

:deep(.el-breadcrumb__inner:hover) {
  color: #2563eb;
}

.breadcrumb-link {
  color: #2563eb;
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

.file-icon {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  color: #6b7280;
  background: #f3f4f6;
}

.file-icon--dir {
  background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%);
  color: #2563eb;
}

.file-name span {
  font-size: 14px;
  color: #1f2937;
}

.file-size {
  font-size: 13px;
  color: #6b7280;
}

.file-time {
  font-size: 13px;
  color: #9ca3af;
}

.directory-label {
  color: #9ca3af;
  font-size: 13px;
}

.directory-row {
  cursor: pointer;
}

.directory-row:hover {
  background: #f8fafc;
}

.file-row:hover {
  background: #f8fafc;
}

.operation-buttons {
  display: flex;
  align-items: center;
}

.btn-download {
  background: #f0f9ff;
  color: #0369a1;
  border-color: #bae6fd;
}

.btn-download:hover {
  background: #e0f2fe;
}

.btn-open {
  background: #f0fdf4;
  color: #059669;
  border-color: #bbf7d0;
}

.btn-open:hover {
  background: #dcfce7;
}

.footer-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 16px;
  margin-top: 16px;
  border-top: 1px solid #f3f4f6;
}

.footer-count {
  font-size: 13px;
  color: #6b7280;
}

.footer-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #9ca3af;
}
</style>
