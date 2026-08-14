<template>
  <el-table
    :data="packages"
    :size="density"
    v-loading="loading"
    style="width: 100%"
    empty-text="暂无数据"
    @selection-change="onSelectionChange"
  >
    <el-table-column v-if="mode === 'admin'" type="selection" width="48" />

    <el-table-column prop="name" label="包名" min-width="220">
      <template #default="{ row }">
        <div class="package-info">
          <div class="package-icon" :style="{ background: getTypeBg(row.type) }">
            <i class="fa-solid fa-box"></i>
          </div>
          <div class="package-content">
            <div class="package-name-row">
              <span class="package-name" @click="$emit('view-detail', row)">
                {{ row.display_name || row.name }}
              </span>
              <el-button link class="copy-name-btn" @click="copyName(row)">
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </div>
            <div v-if="columns.description !== false" class="package-description">
              {{ row.description || '暂无描述' }}
            </div>
          </div>
        </div>
      </template>
    </el-table-column>

    <el-table-column prop="type" label="类型" width="100">
      <template #default="{ row }">
        <el-tag
          :color="getTypeBg(row.type)"
          effect="plain"
          size="small"
          :style="{ color: getTypeColor(row.type), borderColor: getTypeColor(row.type) + '33' }"
        >
          {{ row.type }}
        </el-tag>
      </template>
    </el-table-column>

    <el-table-column v-if="columns.source !== false" prop="repository_type" label="来源" width="170" align="center">
      <template #default="{ row }">
        <div class="source-cell">
          <el-tag :type="row.repository_type === 'proxy' ? 'warning' : 'success'" size="small">
            {{ row.repository_type === 'proxy' ? '代理' : '本地' }}
          </el-tag>
          <span v-if="displayRepos(row).length > 0" class="source-repos">
            <span v-for="repo in displayRepos(row)" :key="repo" class="source-repo">{{ repo }}</span>
          </span>
        </div>
      </template>
    </el-table-column>

    <el-table-column v-if="columns.versions !== false" prop="versions_count" label="版本" width="80" align="center">
      <template #default="{ row }">
        <span class="version-count" @click="$emit('view-versions', row)">{{ row.versions_count || 0 }}</span>
      </template>
    </el-table-column>

    <el-table-column v-if="columns.downloads !== false" prop="download_count" label="下载" width="100" align="center">
      <template #default="{ row }">
        <span class="download-count">{{ formatNumber(row.download_count) }}</span>
      </template>
    </el-table-column>

    <el-table-column v-if="columns.updatedAt !== false" prop="updated_at" label="更新时间" width="160">
      <template #default="{ row }">
        <span class="update-time">{{ formatDate(row.updated_at) }}</span>
      </template>
    </el-table-column>

    <el-table-column label="操作" width="160" fixed="right">
      <template #default="{ row }">
        <div class="action-buttons">
          <el-button class="btn-view-detail" size="small" type="primary" @click="$emit('view-detail', row)">
            详情
          </el-button>
          <el-button
            v-if="mode === 'admin'"
            v-permission="'package:delete'"
            class="btn-delete"
            size="small"
            type="danger"
            @click="$emit('delete-package', row)"
          >
            删除
          </el-button>
        </div>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { CopyDocument } from '@element-plus/icons-vue'
import type { Package } from '@/api/package'
import { formatNumber, formatDate } from '@/utils/format'
import { copyToClipboard } from '@/utils/clipboard'
import { PACKAGE_TYPE_HEX_COLORS } from '@/constants/package'

interface ColumnConfig {
  description?: boolean
  source?: boolean
  versions?: boolean
  downloads?: boolean
  updatedAt?: boolean
}

defineProps<{
  packages: Package[]
  loading: boolean
  mode: 'admin' | 'public'
  density: 'small' | 'default' | 'large'
  selectedIds: number[]
  columns: ColumnConfig
}>()

const emit = defineEmits<{
  'update:selectedIds': [ids: number[]]
  'view-versions': [pkg: Package]
  'view-detail': [pkg: Package]
  'delete-package': [pkg: Package]
}>()

function getTypeColor(type: string): string {
  return PACKAGE_TYPE_HEX_COLORS[type] || PACKAGE_TYPE_HEX_COLORS.generic
}

function getTypeBg(type: string): string {
  const hex = getTypeColor(type)
  return `${hex}1a`  // 10% 透明度背景
}

function copyName(row: Package) {
  copyToClipboard(`${row.type}:${row.name}`)
}

// 来源列只展示实际所在仓库；组合仓库仅是访问入口，不在此列展示（见详情页配置命令）
function displayRepos(row: Package): string[] {
  return row.repositories?.length ? row.repositories : (row.repository_name ? [row.repository_name] : [])
}

function onSelectionChange(rows: Package[]) {
  emit('update:selectedIds', rows.map(r => r.id))
}
</script>

<style scoped>
.package-info { display: flex; align-items: flex-start; gap: 12px; }
.package-icon {
  width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
  border-radius: 8px; font-size: 14px; flex-shrink: 0;
}
.package-content { flex: 1; min-width: 0; }
.package-name-row { display: flex; align-items: center; gap: 4px; }
.package-name { color: #1e293b; cursor: pointer; font-weight: 600; font-size: 14px; }
.package-name:hover { color: #6366f1; }
.copy-name-btn { opacity: 0; transition: opacity 0.2s; padding: 2px; }
.package-info:hover .copy-name-btn { opacity: 1; }
.package-description { font-size: 12px; color: #94a3b8; margin-top: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.version-count { font-weight: 600; color: #1e293b; font-size: 14px; cursor: pointer; }
.version-count:hover { color: #6366f1; }
.download-count { font-weight: 600; color: #6366f1; font-size: 14px; }
.update-time { color: #94a3b8; font-size: 13px; }
.action-buttons { display: flex; gap: 8px; flex-wrap: nowrap; }
/* 上下结构：badge 一行、仓库名独占整行宽度，长仓库名不被 badge 挤压截断 */
.source-cell { display: flex; flex-direction: column; align-items: center; gap: 2px; }
.source-repos { display: inline-flex; flex-direction: column; align-items: center; gap: 2px; width: 100%; }
.source-repo {
  font-size: 11px;
  font-family: var(--font-family-mono);
  color: var(--lunar-silver-muted);
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
