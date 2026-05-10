<template>
  <el-card class="version-table-card">
    <template #header>
      <div class="card-header">
        <span class="card-title">版本列表</span>
        <span class="version-count">共 {{ versions.length }} 个版本</span>
      </div>
    </template>

    <el-table
      :data="pagedVersions"
      highlight-current-row
      style="width: 100%"
      @row-click="handleRowClick"
    >
      <el-table-column prop="version" label="版本号" min-width="140">
        <template #default="{ row }">
          <span class="version-text" :class="{ 'version-selected': row.version === selectedVersion }">
            {{ row.version }}
          </span>
          <el-tag v-if="row.is_latest" type="primary" size="small" effect="plain" class="latest-tag">
            Latest
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="140" align="center">
        <template #default="{ row }">
          <div class="status-cell">
            <el-tag :type="getVersionStatusColor(row.status)" size="small">
              {{ getVersionStatusLabel(row.status) }}
            </el-tag>
            <el-tooltip :content="row.files_downloaded ? '文件已下载到本地' : '仅元数据，文件未下载'" placement="top">
              <el-tag 
                :class="['cache-status-tag', row.files_downloaded ? 'cache-status-tag--cached' : 'cache-status-tag--uncached']"
                effect="plain"
                size="small"
              >
                {{ row.files_downloaded ? '已缓存' : '未缓存' }}
              </el-tag>
            </el-tooltip>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="published_at" label="发布时间" width="180">
        <template #default="{ row }">
          {{ formatDate(row.published_at) }}
        </template>
      </el-table-column>
      <el-table-column label="大小" width="90" align="right">
        <template #default="{ row }">
          <span class="size-text">{{ formatSize(row.files && row.files.length > 0 ? row.files[0].size_bytes : row.size_bytes) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="下载量" width="90" align="right">
        <template #default="{ row }">
          <span class="downloads-text">{{ formatNumber(row.download_count) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="SHA256" min-width="120">
        <template #default="{ row }">
          <el-tooltip :content="row.checksum_sha256 || ''" placement="top" :show-after="300">
            <span class="checksum-text" @click.stop="copyChecksum(row.checksum_sha256 || '')">{{ row.checksum_sha256 || '-' }}</span>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column label="文件" min-width="200">
        <template #default="{ row }">
          <div v-if="row.files && row.files.length > 0" class="files-list">
            <div v-for="file in row.files" :key="file.id" class="file-item">
              <el-button 
                size="small" 
                type="primary" 
                link 
                @click.stop="handleFileDownload(row, file)"
              >
                <el-icon><Download /></el-icon>
                {{ file.filename }}
              </el-button>
              <span class="file-size">({{ formatSize(file.size_bytes) }})</span>
            </div>
          </div>
          <span v-else class="no-files">-</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" :width="showAdminActions ? '200' : '80'" fixed="right" align="center">
        <template #default="{ row }">
          <div class="action-buttons">
            <template v-if="showAdminActions">
              <el-button
                v-if="row.status === 'published'"
                size="small"
                type="warning"
                link
                @click.stop="handleDeprecate(row)"
              >
                废弃
              </el-button>
              <el-button
                v-if="row.status === 'deprecated'"
                size="small"
                type="success"
                link
                @click.stop="handleRestore(row)"
              >
                恢复
              </el-button>
              <el-button
                v-if="row.status === 'published' || row.status === 'deprecated'"
                size="small"
                type="danger"
                link
                @click.stop="handleYank(row)"
              >
                撤回
              </el-button>
              <el-button
                v-if="row.status === 'yanked'"
                size="small"
                type="danger"
                link
                @click.stop="handleDelete(row)"
              >
                删除
              </el-button>
            </template>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="versions.length > pageSize" class="pagination-wrapper">
      <el-pagination
        v-model:current-page="currentPage"
        :page-size="pageSize"
        :total="versions.length"
        layout="prev, pager, next"
        small
        background
      />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Download } from '@element-plus/icons-vue'
import { formatNumber, formatSize, formatDate } from '@/utils/format'
import { getVersionStatusColor, getVersionStatusLabel } from '@/constants/package'
import type { PackageVersion, PackageFile } from '@/api/package'

const props = defineProps<{
  versions: PackageVersion[]
  selectedVersion: string
  showAdminActions?: boolean
}>()

const emit = defineEmits<{
  download: [version: PackageVersion & { selectedFile?: PackageFile }]
  select: [version: string]
  deprecate: [data: { id: number; version: string; reason: string }]
  restore: [data: { id: number; version: string }]
  yank: [data: { id: number; version: string }]
  delete: [data: { id: number; version: string }]
}>()

const pageSize = 10
const currentPage = ref(1)

const pagedVersions = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return props.versions.slice(start, start + pageSize)
})

function handleRowClick(row: PackageVersion) {
  emit('select', row.version)
}

function handleDeprecate(row: PackageVersion) {
  ElMessageBox.prompt('请输入废弃原因', '废弃版本', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    inputPattern: /.+/,
    inputErrorMessage: '废弃原因不能为空',
  }).then(({ value }) => {
    emit('deprecate', { id: row.id, version: row.version, reason: value })
  }).catch(() => {})
}

function handleRestore(row: PackageVersion) {
  ElMessageBox.confirm('确定要恢复此版本吗？', '恢复版本', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  }).then(() => {
    emit('restore', { id: row.id, version: row.version })
  }).catch(() => {})
}

function handleYank(row: PackageVersion) {
  ElMessageBox.confirm('撤回后此版本将无法下载，确定要撤回吗？', '撤回版本', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  }).then(() => {
    emit('yank', { id: row.id, version: row.version })
  }).catch(() => {})
}

function handleDelete(row: PackageVersion) {
  ElMessageBox.confirm('删除后无法恢复，确定要删除此版本吗？', '删除版本', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'error',
  }).then(() => {
    emit('delete', { id: row.id, version: row.version })
  }).catch(() => {})
}

async function copyChecksum(checksum: string) {
  if (!checksum) return
  try {
    await navigator.clipboard.writeText(checksum)
    ElMessage.success('校验和已复制')
  } catch {
    ElMessage.error('复制失败')
  }
}

function handleFileDownload(row: PackageVersion, file: PackageFile) {
  emit('download', { ...row, selectedFile: file })
}
</script>

<style scoped>
.version-table-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.version-count {
  font-size: 13px;
  color: #909399;
}

.version-text {
  font-weight: 500;
  color: #303133;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 13px;
  transition: color 0.2s;
}

.version-selected {
  color: #409eff;
}

.latest-tag {
  margin-left: 8px;
}

.status-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.download-status-tag {
  font-size: 12px;
  padding: 4px 10px;
}

.cache-status-tag {
  border-radius: 6px;
  font-weight: 500;
}

.cache-status-tag--cached {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.cache-status-tag--uncached {
  background: linear-gradient(135deg, #f1f5f9 0%, #e2e8f0 100%);
  color: #64748b;
}

.size-text,
.downloads-text {
  color: #606266;
  font-size: 13px;
}

.checksum-text {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 12px;
  color: #909399;
  cursor: pointer;
  transition: color 0.2s;
}

.checksum-text:hover {
  color: #409eff;
}

.files-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.file-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.file-size {
  font-size: 12px;
  color: #909399;
}

.no-files {
  color: #c0c4cc;
}

.action-buttons {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: center;
}

:deep(.el-table__row) {
  cursor: pointer;
}
</style>
