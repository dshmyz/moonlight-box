<template>
  <div v-if="total > 0" class="pagination-wrapper">
    <div class="pagination-info">
      <span class="total-badge">{{ total }}</span>
      <span class="total-label">个包</span>
    </div>
    <el-pagination
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      :page-sizes="pageSizeOptions"
      layout="sizes, prev, pager, next"
      @current-change="onCurrentChange"
      @size-change="onSizeChange"
    />
  </div>
</template>

<script setup lang="ts">
defineProps<{
  total: number
  page: number
  pageSize: number
  pageSizeOptions: number[]
}>()

const emit = defineEmits<{
  'update:page': [page: number]
  'update:pageSize': [size: number]
}>()

function onCurrentChange(p: number) {
  emit('update:page', p)
}

function onSizeChange(size: number) {
  emit('update:pageSize', size)
  emit('update:page', 1)
}
</script>

<style scoped>
.pagination-wrapper {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 0;
  border-top: 1px solid var(--lunar-border);
}
.pagination-info { display: flex; align-items: center; gap: 8px; }
.total-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  height: 24px;
  padding: 0 10px;
  background: var(--lunar-bg-glass);
  color: var(--lunar-accent);
  font-size: 12px;
  font-weight: 600;
  font-family: var(--font-family-mono);
  font-variant-numeric: tabular-nums;
  border-radius: var(--radius-full);
}
.total-label {
  font-size: 12px;
  color: var(--lunar-silver-muted);
  font-family: var(--font-family-mono);
  font-variant-numeric: tabular-nums;
}
</style>
