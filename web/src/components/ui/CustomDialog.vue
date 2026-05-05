<template>
  <teleport to="body">
    <transition name="dialog-fade">
      <div v-if="modelValue" class="custom-dialog-overlay" @click="handleOverlayClick">
        <div class="custom-dialog" :style="{ width }" @click.stop>
          <div class="custom-dialog-header">
            <h3 class="custom-dialog-title">{{ title }}</h3>
            <button class="custom-dialog-close" @click="close">
              <svg viewBox="0 0 24 24" width="20" height="20">
                <path
                  fill="currentColor"
                  d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"
                />
              </svg>
            </button>
          </div>
          <div class="custom-dialog-body">
            <slot />
          </div>
          <div v-if="$slots.footer" class="custom-dialog-footer">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<script setup lang="ts">
interface Props {
  modelValue: boolean
  title?: string
  width?: string
  closeOnClickOverlay?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  title: '',
  width: '500px',
  closeOnClickOverlay: true,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const close = () => {
  emit('update:modelValue', false)
}

const handleOverlayClick = () => {
  if (props.closeOnClickOverlay) {
    close()
  }
}
</script>

<style scoped>
.custom-dialog-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2000;
}

.custom-dialog {
  background: var(--color-bg-card);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-lg);
  max-height: 90vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.custom-dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-xl) var(--spacing-2xl);
  border-bottom: 1px solid var(--color-border);
}

.custom-dialog-title {
  margin: 0;
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}

.custom-dialog-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  transition: color var(--transition-fast);
  border-radius: var(--radius-md);
}

.custom-dialog-close:hover {
  color: var(--color-text-primary);
}

.custom-dialog-body {
  padding: var(--spacing-2xl);
  overflow-y: auto;
  flex: 1;
}

.custom-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  padding: var(--spacing-lg) var(--spacing-2xl);
  border-top: 1px solid var(--color-border);
  background: #fafbfc;
}

.dialog-fade-enter-active,
.dialog-fade-leave-active {
  transition: opacity var(--transition-base);
}

.dialog-fade-enter-active .custom-dialog,
.dialog-fade-leave-active .custom-dialog {
  transition: transform var(--transition-base);
}

.dialog-fade-enter-from,
.dialog-fade-leave-to {
  opacity: 0;
}

.dialog-fade-enter-from .custom-dialog,
.dialog-fade-leave-to .custom-dialog {
  transform: scale(0.9);
}
</style>
