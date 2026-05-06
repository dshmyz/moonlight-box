<template>
  <div class="custom-pagination" :class="`custom-pagination--${size}`">
    <div class="custom-pagination-total" v-if="showTotal">
      共 {{ total }} 条
    </div>

    <button
      class="custom-pagination-btn"
      :disabled="currentPage <= 1"
      @click="handlePrev"
    >
      <svg viewBox="0 0 24 24" width="16" height="16">
        <path fill="currentColor" d="M15.41 7.41L14 6l-6 6 6 6 1.41-1.41L10.83 12z" />
      </svg>
    </button>

    <div class="custom-pagination-pages">
      <button
        v-for="page in visiblePages"
        :key="page"
        :class="['custom-pagination-page', { 'is-active': page === currentPage }]"
        :disabled="page === '...'"
        @click="page !== '...' && handlePageChange(page as number)"
      >
        {{ page }}
      </button>
    </div>

    <button
      class="custom-pagination-btn"
      :disabled="currentPage >= totalPages"
      @click="handleNext"
    >
      <svg viewBox="0 0 24 24" width="16" height="16">
        <path fill="currentColor" d="M8.59 16.59L10 18l6-6-6-6-1.41 1.41L13.17 12z" />
      </svg>
    </button>

    <select
      v-if="showSizes"
      class="custom-pagination-size"
      :value="pageSize"
      @change="handleSizeChange"
    >
      <option v-for="size in sizes" :key="size" :value="size">
        {{ size }} 条/页
      </option>
    </select>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  currentPage: number
  pageSize?: number
  total: number
  sizes?: number[]
  size?: 'small' | 'medium' | 'large'
  showTotal?: boolean
  showSizes?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  pageSize: 20,
  sizes: () => [10, 20, 50, 100],
  size: 'medium',
  showTotal: true,
  showSizes: true,
})

const emit = defineEmits<{
  'update:currentPage': [value: number]
  'update:pageSize': [value: number]
  'current-change': [value: number]
  'size-change': [value: number]
}>()

const totalPages = computed(() => Math.ceil(props.total / props.pageSize))

const visiblePages = computed(() => {
  const pages: (number | string)[] = []
  const total = totalPages.value
  const current = props.currentPage

  if (total <= 7) {
    for (let i = 1; i <= total; i++) {
      pages.push(i)
    }
  } else {
    pages.push(1)
    if (current > 3) pages.push('...')
    const start = Math.max(2, current - 1)
    const end = Math.min(total - 1, current + 1)
    for (let i = start; i <= end; i++) {
      pages.push(i)
    }
    if (current < total - 2) pages.push('...')
    pages.push(total)
  }

  return pages
})

const handlePrev = () => {
  if (props.currentPage > 1) {
    handlePageChange(props.currentPage - 1)
  }
}

const handleNext = () => {
  if (props.currentPage < totalPages.value) {
    handlePageChange(props.currentPage + 1)
  }
}

const handlePageChange = (page: number) => {
  emit('update:currentPage', page)
  emit('current-change', page)
}

const handleSizeChange = (event: Event) => {
  const target = event.target as HTMLSelectElement
  const size = Number(target.value)
  emit('update:pageSize', size)
  emit('size-change', size)
  emit('update:currentPage', 1)
}
</script>

<style scoped>
.custom-pagination {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.custom-pagination-total {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  margin-right: var(--spacing-md);
}

.custom-pagination-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 32px;
  padding: 0;
  border: 1px solid var(--color-border);
  background: var(--color-bg-card);
  color: var(--color-text-primary);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.custom-pagination-btn:hover:not(:disabled) {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.custom-pagination-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.custom-pagination-pages {
  display: flex;
  gap: var(--spacing-xs);
}

.custom-pagination-page {
  min-width: 32px;
  height: 32px;
  padding: 0 var(--spacing-sm);
  border: 1px solid var(--color-border);
  background: var(--color-bg-card);
  color: var(--color-text-primary);
  border-radius: var(--radius-md);
  cursor: pointer;
  font-size: var(--font-size-base);
  transition: all var(--transition-fast);
}

.custom-pagination-page:hover:not(:disabled):not(.is-active) {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.custom-pagination-page.is-active {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: white;
  font-weight: var(--font-weight-semibold);
}

.custom-pagination-page:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.custom-pagination-size {
  height: 32px;
  padding: 0 var(--spacing-md);
  border: 1px solid var(--color-border);
  background: var(--color-bg-card);
  color: var(--color-text-primary);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  cursor: pointer;
  outline: none;
  transition: border-color var(--transition-fast);
}

.custom-pagination-size:hover {
  border-color: var(--color-primary);
}

.custom-pagination--small .custom-pagination-page,
.custom-pagination--small .custom-pagination-btn,
.custom-pagination--small .custom-pagination-size {
  min-width: 28px;
  height: 28px;
  font-size: var(--font-size-sm);
}

.custom-pagination--large .custom-pagination-page,
.custom-pagination--large .custom-pagination-btn,
.custom-pagination--large .custom-pagination-size {
  min-width: 36px;
  height: 36px;
  font-size: var(--font-size-lg);
}
</style>
