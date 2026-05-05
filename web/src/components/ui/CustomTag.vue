<template>
  <span :class="['custom-tag', `custom-tag--${type}`, `custom-tag--${size}`]">
    <slot>{{ text }}</slot>
  </span>
</template>

<script setup lang="ts">
interface Props {
  type?: 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info'
  size?: 'small' | 'medium' | 'large'
  text?: string
}

withDefaults(defineProps<Props>(), {
  type: 'default',
  size: 'medium',
  text: '',
})
</script>

<style scoped>
.custom-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-full);
  font-weight: var(--font-weight-medium);
  letter-spacing: 0.3px;
  transition: all var(--transition-fast);
}

.custom-tag--small {
  padding: 2px 8px;
  font-size: 11px;
  height: 20px;
}

.custom-tag--medium {
  padding: var(--spacing-xs) var(--spacing-md);
  font-size: var(--font-size-xs);
  height: 24px;
}

.custom-tag--large {
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: var(--font-size-sm);
  height: 28px;
}

.custom-tag--default {
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
}

.custom-tag--primary {
  background: #dbeafe;
  color: #1e40af;
}

.custom-tag--success {
  background: var(--color-success-light);
  color: var(--color-success-dark);
}

.custom-tag--warning {
  background: var(--color-warning-light);
  color: var(--color-warning-dark);
}

.custom-tag--danger {
  background: var(--color-danger-light);
  color: var(--color-danger-dark);
}

.custom-tag--info {
  background: #e0e7ff;
  color: #3730a3;
}
</style>
