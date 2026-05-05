<template>
  <div class="custom-table-wrapper">
    <div v-if="loading" class="custom-table-loading">
      <div class="loading-spinner"></div>
      <span>加载中...</span>
    </div>

    <div v-else-if="!data || data.length === 0" class="custom-table-empty">
      <slot name="empty">
        <div class="empty-content">
          <svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
          <p>暂无数据</p>
        </div>
      </slot>
    </div>

    <table v-else class="custom-table" :class="{ 'is-striped': striped, 'is-bordered': bordered }">
      <thead>
        <tr>
          <th
            v-for="column in columns"
            :key="column.prop"
            :style="{ width: column.width, textAlign: column.align || 'left' }"
            :class="{ 'is-sortable': column.sortable }"
            @click="handleSort(column)"
          >
            <div class="th-content">
              <span>{{ column.label }}</span>
              <span v-if="column.sortable" class="sort-icon">
                <svg v-if="sortProp !== column.prop" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M7 10l5-5 5 5H7zm10 4l-5 5-5-5h10z" />
                </svg>
                <svg v-else-if="sortOrder === 'asc'" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M7 14l5-5 5 5H7z" />
                </svg>
                <svg v-else viewBox="0 0 24 24" fill="currentColor">
                  <path d="M7 10l5 5 5-5H7z" />
                </svg>
              </span>
            </div>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, index) in data" :key="rowKey ? row[rowKey] : index">
          <td
            v-for="column in columns"
            :key="column.prop"
            :style="{ textAlign: column.align || 'left' }"
          >
            <slot v-if="$slots[column.prop]" :name="column.prop" :row="row" :index="index">
              {{ row[column.prop] }}
            </slot>
            <span v-else>{{ row[column.prop] }}</span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

interface Column {
  prop: string
  label: string
  width?: string
  align?: 'left' | 'center' | 'right'
  sortable?: boolean
}

interface Props {
  columns: Column[]
  data: any[]
  loading?: boolean
  striped?: boolean
  bordered?: boolean
  rowKey?: string
  defaultSortProp?: string
  defaultSortOrder?: 'asc' | 'desc'
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  striped: true,
  bordered: false,
  rowKey: '',
  defaultSortProp: '',
  defaultSortOrder: 'asc',
})

const emit = defineEmits<{
  'sort-change': [{ prop: string; order: 'asc' | 'desc' }]
}>()

const sortProp = ref(props.defaultSortProp)
const sortOrder = ref<'asc' | 'desc'>(props.defaultSortOrder)

const handleSort = (column: Column) => {
  if (!column.sortable) return

  if (sortProp.value === column.prop) {
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortProp.value = column.prop
    sortOrder.value = 'asc'
  }

  emit('sort-change', {
    prop: sortProp.value,
    order: sortOrder.value,
  })
}
</script>

<style scoped>
.custom-table-wrapper {
  background: var(--color-bg-card);
  border-radius: var(--radius-xl);
  overflow: hidden;
  border: 1px solid var(--color-border);
}

.custom-table {
  width: 100%;
  border-collapse: collapse;
  border-spacing: 0;
}

.custom-table thead {
  background: #fafbfc;
}

.custom-table th {
  padding: var(--spacing-lg) var(--spacing-xl);
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  border-bottom: 2px solid var(--color-border);
  user-select: none;
}

.custom-table th.is-sortable {
  cursor: pointer;
  transition: background var(--transition-fast);
}

.custom-table th.is-sortable:hover {
  background: #f1f5f9;
}

.th-content {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.sort-icon {
  display: inline-flex;
  opacity: 0.4;
  transition: opacity var(--transition-fast);
}

.sort-icon svg {
  width: 14px;
  height: 14px;
}

.custom-table th.is-sortable:hover .sort-icon {
  opacity: 0.7;
}

.custom-table td {
  padding: var(--spacing-lg) var(--spacing-xl);
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
  border-bottom: 1px solid var(--color-border-light);
  transition: background var(--transition-fast);
}

.custom-table tbody tr:hover td {
  background: var(--color-bg-hover);
}

.custom-table.is-striped tbody tr:nth-child(even) td {
  background: #fafbfc;
}

.custom-table.is-striped tbody tr:nth-child(even):hover td {
  background: var(--color-bg-hover);
}

.custom-table.is-bordered th,
.custom-table.is-bordered td {
  border: 1px solid var(--color-border);
}

.custom-table-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-2xl) * 2;
  color: var(--color-text-secondary);
  gap: var(--spacing-md);
}

.loading-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.custom-table-empty {
  padding: var(--spacing-2xl) * 2;
}

.empty-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--color-text-tertiary);
  gap: var(--spacing-md);
}

.empty-icon {
  width: 48px;
  height: 48px;
  opacity: 0.5;
}

.empty-content p {
  margin: 0;
  font-size: var(--font-size-base);
}
</style>
