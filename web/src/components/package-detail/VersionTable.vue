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
      <el-table-column prop="status" label="状态" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="getStatusType(row.status)" size="small">
            {{ getStatusLabel(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="published_at" label="发布时间" width="120" />
      <el-table-column prop="size" label="大小" width="90" align="right">
        <template #default="{ row }">
          <span class="size-text">{{ formatSize(row.size) }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="downloads" label="下载量" width="90" align="right">
        <template #default="{ row }">
          <span class="downloads-text">{{ formatNumber(row.downloads) }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="checksum" label="SHA256" min-width="120">
        <template #default="{ row }">
          <el-tooltip :content="row.checksum" placement="top" :show-after="300">
            <span class="checksum-text" @click.stop="copyChecksum(row.checksum)">{{ row.checksum }}</span>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column label="操作" :width="showAdminActions ? '240' : '80'" fixed="right" align="center">
        <template #default="{ row }">
          <div class="action-buttons">
            <el-button size="small" type="primary" link @click.stop="$emit('download', row)">
              下载
            </el-button>
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

const props = defineProps<{
  versions: Array<{
    version: string
    published_at: string
    downloads: number
    is_latest?: boolean
    size: number
    checksum: string
    status: string
  }>
  selectedVersion: string
  showAdminActions?: boolean
}>()

const emit = defineEmits<{
  download: [version: { version: string; published_at: string; downloads: number; is_latest?: boolean; size: number; checksum: string; status: string }]
  select: [version: string]
  deprecate: [version: { version: string }]
  restore: [version: { version: string }]
  yank: [version: { version: string }]
  delete: [version: { version: string }]
}>()

const pageSize = 10
const currentPage = ref(1)

const pagedVersions = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return props.versions.slice(start, start + pageSize)
})

function handleRowClick(row: { version: string }) {
  emit('select', row.version)
}

function handleDeprecate(row: { version: string }) {
  ElMessageBox.prompt('请输入废弃原因', '废弃版本', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    inputPattern: /.+/,
    inputErrorMessage: '废弃原因不能为空',
  }).then(({ value }) => {
    emit('deprecate', { version: row.version, reason: value })
  }).catch(() => {
    // 用户取消
  })
}

function handleRestore(row: { version: string }) {
  ElMessageBox.confirm('确定要恢复此版本吗？', '恢复版本', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  }).then(() => {
    emit('restore', row)
  }).catch(() => {
    // 用户取消
  })
}

function handleYank(row: { version: string }) {
  ElMessageBox.confirm('撤回后此版本将无法下载，确定要撤回吗？', '撤回版本', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  }).then(() => {
    emit('yank', row)
  }).catch(() => {
    // 用户取消
  })
}

function handleDelete(row: { version: string }) {
  ElMessageBox.confirm('删除后无法恢复，确定要删除此版本吗？', '删除版本', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'error',
  }).then(() => {
    emit('delete', row)
  }).catch(() => {
    // 用户取消
  })
}

function getStatusType(status: string) {
  const map: Record<string, string> = {
    published: 'success',
    deprecated: 'warning',
    yanked: 'danger',
    draft: 'info',
  }
  return map[status] || 'info'
}

function getStatusLabel(status: string) {
  const map: Record<string, string> = {
    published: '已发布',
    deprecated: '已弃用',
    yanked: '已撤回',
    draft: '草稿',
  }
  return map[status] || status
}

function formatNumber(num: number) {
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`
  if (num >= 1000) return `${(num / 1000).toFixed(1)}K`
  return String(num)
}

function formatSize(bytes: number) {
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}

async function copyChecksum(checksum: string) {
  try {
    await navigator.clipboard.writeText(checksum)
    ElMessage.success('校验和已复制')
  } catch {
    ElMessage.error('复制失败')
  }
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
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
  max-width: 120px;
}

.checksum-text:hover {
  color: #409eff;
}

.pagination-wrapper {
  display: flex;
  justify-content: center;
  margin-top: 16px;
}

.action-buttons {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  justify-content: center;
}

:deep(.el-table__row) {
  cursor: pointer;
}
</style>
