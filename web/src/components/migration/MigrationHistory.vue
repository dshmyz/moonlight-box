<template>
  <div class="migration-history">
    <h3>迁移历史</h3>
    <el-table :data="tasks">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="source_url" label="来源" />
      <el-table-column prop="status" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="processed_items" label="进度" width="120">
        <template #default="{ row }">
          {{ row.processed_items }}/{{ row.total_items }}
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="180" />
    </el-table>
  </div>
</template>

<script setup lang="ts">
import type { MigrationTask } from '@/api/migration'

defineProps<{
  tasks: MigrationTask[]
}>()

function statusType(status: string) {
  const map: Record<string, string> = {
    completed: 'success',
    failed: 'danger',
    cancelled: 'warning',
    running: '',
    pending: 'info',
  }
  return map[status] || 'info'
}
</script>
