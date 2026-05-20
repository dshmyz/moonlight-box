<template>
  <el-table 
    :data="packages" 
    v-loading="loading" 
    style="width: 100%" 
    empty-text="暂无数据"
    :header-cell-style="{ background: '#fafbfc' }"
    :row-class-name="tableRowClass"
    @row-mouse-enter="handleRowEnter"
    @row-mouse-leave="handleRowLeave"
  >
    <el-table-column prop="name" label="包名" min-width="180">
      <template #default="{ row }">
        <div class="package-info">
          <div class="package-icon"><i class="fa-solid fa-box"></i></div>
          <div class="package-content">
            <div class="package-name" @click="$emit('view-detail', row)">
              {{ row.display_name || row.name }}
            </div>
            <div class="package-description">{{ row.description || '暂无描述' }}</div>
          </div>
        </div>
      </template>
    </el-table-column>
    <el-table-column prop="type" label="类型" width="90">
      <template #default="{ row }">
        <el-tag :class="['type-tag', `type-tag--${row.type}`]" size="small">
          {{ row.type }}
        </el-tag>
      </template>
    </el-table-column>
    <el-table-column prop="repository_type" label="来源" width="80" align="center">
      <template #default="{ row }">
        <el-tag :class="['source-tag', row.repository_type === 'proxy' ? 'source-tag--proxy' : 'source-tag--local']" size="small">
          {{ row.repository_type === 'proxy' ? '代理' : '本地' }}
        </el-tag>
      </template>
    </el-table-column>
    <el-table-column prop="versions_count" label="版本" width="70" align="center">
      <template #default="{ row }">
        <span class="version-count">{{ row.versions_count || 0 }}</span>
      </template>
    </el-table-column>
    <el-table-column prop="download_count" label="下载" width="100" align="center">
      <template #default="{ row }">
        <span class="download-count">{{ formatNumber(row.download_count) }}</span>
      </template>
    </el-table-column>
    <el-table-column prop="updated_at" label="更新时间" width="160">
      <template #default="{ row }">
        <span class="update-time">{{ formatDate(row.updated_at) }}</span>
      </template>
    </el-table-column>
    <el-table-column label="操作" width="260" fixed="right">
      <template #default="{ row }">
        <div class="action-buttons">
          <el-button class="btn-view-versions" size="small" @click="$emit('view-versions', row)">
            查看版本
          </el-button>
          <el-button class="btn-view-detail" size="small" type="primary" @click="$emit('view-detail', row)">
            详情
          </el-button>
          <el-button class="btn-delete" size="small" type="danger" @click="$emit('delete-package', row)">
            删除
          </el-button>
        </div>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { Package } from '@/api/package'
import { formatNumber, formatDate } from '@/utils/format'

defineProps<{
  packages: Package[]
  loading: boolean
}>()

defineEmits<{
  'view-versions': [pkg: Package]
  'view-detail': [pkg: Package]
  'delete-package': [pkg: Package]
}>()

const hoveredRow = ref<number | null>(null)

const tableRowClass = ({ rowIndex }: { rowIndex: number }) => {
  return hoveredRow.value === rowIndex ? 'row-hovered' : ''
}

const handleRowEnter = ({ rowIndex }: { rowIndex: number }) => {
  hoveredRow.value = rowIndex
}

const handleRowLeave = () => {
  hoveredRow.value = null
}
</script>

<style scoped>
:deep(.el-table) {
  --el-table-header-text-color: #475569;
  --el-table-text-color: #1e293b;
  --el-table-border-color: rgba(0, 0, 0, 0.04);
}

:deep(.el-table th) {
  font-weight: 600;
  font-size: 13px;
  color: #64748b;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
}

:deep(.el-table td) {
  padding: 16px 12px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.03);
  transition: all 0.2s ease;
}

:deep(.el-table .row-hovered td) {
  background: #f8fafc;
}

:deep(.el-table .row-hovered) {
  box-shadow: inset 0 2px 8px rgba(0, 0, 0, 0.02);
}

.package-info {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.package-icon {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
  border-radius: 8px;
  font-size: 14px;
  flex-shrink: 0;
}

.package-content {
  flex: 1;
  min-width: 0;
}

.package-name {
  color: #1e293b;
  cursor: pointer;
  font-weight: 600;
  font-size: 14px;
  transition: color 0.2s ease;
}

.package-name:hover {
  color: #6366f1;
}

.package-description {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.type-tag {
  font-size: 12px;
  padding: 4px 12px;
  border-radius: 6px;
  font-weight: 500;
}

.type-tag--npm {
  background: linear-gradient(135deg, #dbeafe 0%, #93c5fd 100%);
  color: #1d4ed8;
  border: none;
}

.type-tag--maven {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
  border: none;
}

.type-tag--pypi {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
  border: none;
}

.type-tag--go {
  background: linear-gradient(135deg, #d1fae5 0%, #6ee7b7 100%);
  color: #047857;
  border: none;
}

.source-tag {
  font-size: 12px;
  padding: 4px 12px;
  border-radius: 6px;
  font-weight: 500;
  border: none;
}

.source-tag--proxy {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.source-tag--local {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #059669;
}

.version-count {
  font-weight: 600;
  color: #1e293b;
  font-size: 14px;
}

.download-count {
  font-weight: 600;
  color: #6366f1;
  font-size: 14px;
}

.update-time {
  color: #94a3b8;
  font-size: 13px;
}

.action-buttons {
  display: flex;
  gap: 10px;
  flex-wrap: nowrap;
}

.btn-view-versions {
  border-radius: 8px;
  padding: 6px 14px;
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  transition: all 0.2s ease;
}

.btn-view-versions:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
  color: #475569;
}

.btn-view-detail {
  border-radius: 8px;
  padding: 6px 14px;
  font-size: 12px;
  font-weight: 500;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border: none;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.3);
  transition: all 0.2s ease;
}

.btn-view-detail:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4);
}

.btn-delete {
  border-radius: 8px;
  padding: 6px 14px;
  font-size: 12px;
  font-weight: 500;
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  border: none;
  box-shadow: 0 2px 8px rgba(239, 68, 68, 0.3);
  transition: all 0.2s ease;
}

.btn-delete:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.4);
}
</style>
