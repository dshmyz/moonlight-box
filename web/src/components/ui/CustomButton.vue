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
  type?: 'primary' | 'secondary' | 'outline' | 'text'
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
  gap: var(--spacing-sm);
  border: none;
  cursor: pointer;
  font-family: inherit;
  font-weight: var(--font-weight-semibold);
  letter-spacing: 0.5px;
  transition: all var(--transition-base);
  border-radius: var(--radius-lg);
  outline: none;
}

.custom-button:active {
  transform: scale(0.97);
}

/* Size variants */
.custom-button--small {
  padding: 6px 16px;
  font-size: var(--font-size-sm);
  height: 32px;
}

.custom-button--medium {
  padding: 10px 24px;
  font-size: var(--font-size-base);
  height: 40px;
}

.custom-button--large {
  padding: 12px 32px;
  font-size: 15px;
  height: 48px;
}

/* Type variants */
.custom-button--primary {
  background: linear-gradient(135deg, #0f172a 0%, #1e293b 100%);
  color: #ffffff;
}

.custom-button--primary:hover:not(:disabled) {
  background: linear-gradient(135deg, #1e293b 0%, #334155 100%);
}

.custom-button--secondary {
  background: #f8fafc;
  color: #0f172a;
  border: 2px solid #e2e8f0;
}

.custom-button--secondary:hover:not(:disabled) {
  background: #ffffff;
  border-color: #0f172a;
}

.custom-button--outline {
  background: transparent;
  color: #0f172a;
  border: 2px solid #0f172a;
}

.custom-button--outline:hover:not(:disabled) {
  background: #0f172a;
  color: #ffffff;
}

.custom-button--text {
  background: transparent;
  color: #0f172a;
  padding: 6px 12px;
}

.custom-button--text:hover:not(:disabled) {
  background: rgba(15, 23, 42, 0.05);
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
