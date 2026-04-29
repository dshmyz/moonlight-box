<template>
  <el-table :data="packages" v-loading="loading" style="width: 100%" empty-text="暂无数据">
    <el-table-column prop="name" label="包名" min-width="200">
      <template #default="{ row }">
        <div>
          <div class="package-name" @click="$emit('view-detail', row)">
            {{ row.name }}
          </div>
          <div class="package-description">{{ row.description || '暂无描述' }}</div>
        </div>
      </template>
    </el-table-column>
    <el-table-column prop="type" label="类型" width="100">
      <template #default="{ row }">
        <el-tag :type="getTypeTag(row.type)" size="small">
          {{ row.type }}
        </el-tag>
      </template>
    </el-table-column>
    <el-table-column prop="versions_count" label="版本数" width="80" align="center" />
    <el-table-column prop="download_count" label="下载数" width="100" align="center">
      <template #default="{ row }">
        {{ formatNumber(row.download_count) }}
      </template>
    </el-table-column>
    <el-table-column prop="updated_at" label="更新时间" width="180">
      <template #default="{ row }">
        {{ formatDate(row.updated_at) }}
      </template>
    </el-table-column>
    <el-table-column label="操作" width="180" fixed="right">
      <template #default="{ row }">
        <el-button size="small" @click="$emit('view-versions', row)">
          查看版本
        </el-button>
        <el-button size="small" @click="$emit('view-detail', row)">
          详情
        </el-button>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import type { Package } from '@/api/package'
import { formatNumber, formatDate } from '@/utils/format'
import { getPackageTypeColor } from '@/constants/package'

defineProps<{
  packages: Package[]
  loading: boolean
}>()

defineEmits<{
  'view-versions': [pkg: Package]
  'view-detail': [pkg: Package]
}>()

const getTypeTag = (type: string) => {
  return getPackageTypeColor(type)
}
</script>

<style scoped>
.package-name {
  color: #409eff;
  cursor: pointer;
  font-weight: 600;
}

.package-name:hover {
  text-decoration: underline;
}

.package-description {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
