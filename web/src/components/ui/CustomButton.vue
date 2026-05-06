<template>
  <button
    :class="[
      'custom-button',
      `custom-button--${type}`,
      `custom-button--${size}`,
      {
        'is-disabled': disabled,
        'is-loading': loading,
      },
    ]"
    :disabled="disabled || loading"
    @click="handleClick"
  >
    <span v-if="loading" class="custom-button__loading">
      <svg class="loading-icon" viewBox="0 0 24 24">
        <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" fill="none" />
      </svg>
    </span>
    <span v-if="icon && !loading" class="custom-button__icon">
      <component :is="icon" />
    </span>
    <span class="custom-button__text">
      <slot />
    </span>
  </button>
</template>

<script setup lang="ts">
import { type Component } from 'vue'

interface Props {
  type?: 'primary' | 'secondary' | 'outline' | 'text' | 'success' | 'warning' | 'danger'
  size?: 'small' | 'medium' | 'large'
  disabled?: boolean
  loading?: boolean
  icon?: Component
}

const props = withDefaults(defineProps<Props>(), {
  type: 'primary',
  size: 'medium',
  disabled: false,
  loading: false,
})

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

const handleClick = (event: MouseEvent) => {
  if (!props.disabled && !props.loading) {
    emit('click', event)
  }
}
</script>

<style scoped>
.custom-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: none;
  cursor: pointer;
  font-family: inherit;
  font-weight: 500;
  letter-spacing: 0.5px;
  transition: all 0.25s ease;
  border-radius: 8px;
  outline: none;
}

.custom-button:active {
  transform: scale(0.97);
}

/* Size variants */
.custom-button--small {
  padding: 8px 16px;
  font-size: 13px;
  height: 32px;
}

.custom-button--medium {
  padding: 10px 20px;
  font-size: 14px;
  height: 40px;
}

.custom-button--large {
  padding: 12px 24px;
  font-size: 15px;
  height: 48px;
}

/* Type variants */
.custom-button--primary {
  background: #3b82f6;
  color: #ffffff;
}

.custom-button--primary:hover:not(:disabled) {
  background: #2563eb;
}

.custom-button--secondary {
  background: #ffffff;
  color: #374151;
  border: 1px solid #d1d5db;
}

.custom-button--secondary:hover:not(:disabled) {
  border-color: #9ca3af;
}

.custom-button--outline {
  background: transparent;
  color: #3b82f6;
  border: 1px solid #3b82f6;
}

.custom-button--outline:hover:not(:disabled) {
  background: #3b82f6;
  color: #ffffff;
}

.custom-button--text {
  background: transparent;
  color: #6b7280;
  padding: 6px 12px;
}

.custom-button--text:hover:not(:disabled) {
  background: rgba(59, 130, 246, 0.1);
  color: #3b82f6;
}

.custom-button--success {
  background: #10b981;
  color: #ffffff;
}

.custom-button--success:hover:not(:disabled) {
  background: #059669;
}

.custom-button--warning {
  background: #f59e0b;
  color: #ffffff;
}

.custom-button--warning:hover:not(:disabled) {
  background: #d97706;
}

.custom-button--danger {
  background: #ef4444;
  color: #ffffff;
}

.custom-button--danger:hover:not(:disabled) {
  background: #dc2626;
}

/* States */
.custom-button.is-disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.custom-button.is-loading {
  cursor: wait;
}

.custom-button__loading {
  display: inline-flex;
}

.loading-icon {
  width: 16px;
  height: 16px;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.custom-button__icon {
  display: inline-flex;
  align-items: center;
  font-size: 16px;
}

.custom-button__text {
  display: inline-flex;
}
</style>
