<template>
  <el-card class="version-table-card">
    <template #header>
      <div class="card-header">
        <span class="card-title">版本列表</span>
        <div class="header-actions">
          <el-radio-group v-model="cacheFilter" size="small" class="cache-filter">
            <el-radio-button value="all">全部 ({{ versions.length }})</el-radio-button>
            <el-radio-button value="cached">已缓存 ({{ cachedCount }})</el-radio-button>
            <el-radio-button value="uncached">未缓存 ({{ uncachedCount }})</el-radio-button>
          </el-radio-group>
        </div>
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
            <div v-for="file in visibleFiles(row)" :key="file.id" class="file-item">
              <a
                v-if="file.download_url"
                class="file-download-link"
                :href="file.download_url"
                :download="file.filename"
                @click.stop
              >
                {{ file.filename }}
              </a>
              <span v-else class="file-download-link file-download-link--disabled">
                {{ file.filename }}
              </span>
              <span class="file-size">({{ formatSize(file.size_bytes) }})</span>
            </div>
            <button v-if="hiddenFileCount(row) > 0" class="more-files-hint" type="button" @click.stop="toggleFiles(row)">
              更多文件（{{ hiddenFileCount(row) }}）
            </button>
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

    <div v-if="filteredVersions.length > pageSize" class="pagination-wrapper">
      <el-pagination
        v-model:current-page="currentPage"
        :page-size="pageSize"
        :total="filteredVersions.length"
        layout="prev, pager, next"
        small
        background
      />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ElMessageBox } from 'element-plus'
import { formatNumber, formatSize, formatDate } from '@/utils/format'
import { getVersionStatusColor, getVersionStatusLabel } from '@/constants/package'
import { copyToClipboard } from '@/utils/clipboard'
import type { PackageVersion, PackageFile } from '@/api/package'

const props = defineProps<{
  versions: PackageVersion[]
  selectedVersion: string
  showAdminActions?: boolean
}>()

const emit = defineEmits<{
  select: [version: string]
  deprecate: [data: { id: number; version: string; reason: string }]
  restore: [data: { id: number; version: string }]
  yank: [data: { id: number; version: string }]
  delete: [data: { id: number; version: string }]
}>()

const pageSize = 10
const currentPage = ref(1)
const cacheFilter = ref<'all' | 'cached' | 'uncached'>('all')
const expandedFiles = ref<Set<string>>(new Set())

// 筛选变化时重置页码
watch(cacheFilter, () => {
  currentPage.value = 1
})

// 统计已缓存/未缓存数量
const cachedCount = computed(() => props.versions.filter(v => v.files_downloaded).length)
const uncachedCount = computed(() => props.versions.filter(v => !v.files_downloaded).length)

// 按缓存状态排序，已缓存优先，同时保持发布时间倒序
const sortedVersions = computed(() => {
  return [...props.versions].sort((a, b) => {
    // 已缓存的排前面
    if (a.files_downloaded !== b.files_downloaded) {
      return a.files_downloaded ? -1 : 1
    }
    // 同状态按发布时间倒序
    return new Date(b.published_at).getTime() - new Date(a.published_at).getTime()
  })
})

// 根据筛选条件过滤版本
const filteredVersions = computed(() => {
  switch (cacheFilter.value) {
    case 'cached':
      return sortedVersions.value.filter(v => v.files_downloaded)
    case 'uncached':
      return sortedVersions.value.filter(v => !v.files_downloaded)
    default:
      return sortedVersions.value
  }
})

const pagedVersions = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return filteredVersions.value.slice(start, start + pageSize)
})

function hasDefaultVisibleFiles(files: PackageFile[]) {
  return files.some(file => file.attributes?.default_visible === true || file.attributes?.default_visible === 'true')
}

function visibleFiles(row: PackageVersion) {
  const files = row.files || []
  if (expandedFiles.value.has(row.version) || !hasDefaultVisibleFiles(files)) return files
  return files.filter(file => file.attributes?.default_visible === true || file.attributes?.default_visible === 'true')
}

function hiddenFileCount(row: PackageVersion) {
  const files = row.files || []
  if (expandedFiles.value.has(row.version)) return 0
  return files.length - visibleFiles(row).length
}

function toggleFiles(row: PackageVersion) {
  const next = new Set(expandedFiles.value)
  if (next.has(row.version)) {
    next.delete(row.version)
  } else {
    next.add(row.version)
  }
  expandedFiles.value = next
}

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

function copyChecksum(checksum: string) {
  if (!checksum) return
  copyToClipboard(checksum, '校验和已复制')
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
  color: var(--lunar-silver);
}

.version-count {
  font-size: 13px;
  color: var(--lunar-silver-dim);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.cache-filter {
  --el-radio-button-checked-bg-color: var(--lunar-accent);
  --el-radio-button-checked-border-color: var(--lunar-accent);
}

.cache-filter :deep(.el-radio-button__inner) {
  background: var(--lunar-bg-glass);
  border-color: var(--lunar-border);
  color: var(--lunar-silver-muted);
}

.cache-filter :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  background: var(--lunar-gradient-btn);
  border-color: var(--lunar-accent);
  color: var(--lunar-bg-deep);
  box-shadow: none;
}

.version-text {
  font-weight: 500;
  color: var(--lunar-silver);
  font-family: var(--font-family-mono);
  font-size: 13px;
  transition: color 0.2s;
}

.version-selected {
  color: var(--lunar-accent);
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
  background: var(--lunar-bg-glass);
  color: var(--lunar-silver-dim);
}

.size-text,
.downloads-text {
  color: var(--lunar-silver-muted);
  font-size: 13px;
}

.checksum-text {
  font-family: var(--font-family-mono);
  font-size: 12px;
  color: var(--lunar-silver-dim);
  cursor: pointer;
  transition: color 0.2s;
}

.checksum-text:hover {
  color: var(--lunar-accent);
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

.file-download-link {
  color: var(--lunar-accent);
  font-size: 12px;
  text-decoration: none;
  word-break: break-all;
}

.file-download-link:hover {
  text-decoration: underline;
}

.file-download-link--disabled {
  color: var(--lunar-silver-dim);
  cursor: not-allowed;
}

.more-files-hint {
  align-self: flex-start;
  background: transparent;
  border: 0;
  color: var(--lunar-silver-dim);
  cursor: pointer;
  font-size: 12px;
  padding: 0;
}

.more-files-hint:hover {
  color: var(--lunar-accent);
  text-decoration: underline;
}

.file-size {
  font-size: 12px;
  color: var(--lunar-silver-dim);
}

.no-files {
  color: var(--lunar-silver-dim);
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
