<template>
  <div class="custom-tabs">
    <div class="custom-tabs-header">
      <button
        v-for="tab in tabs"
        :key="tab.name"
        :class="['custom-tab', { 'is-active': modelValue === tab.name }]"
        @click="handleClick(tab.name)"
      >
        {{ tab.label }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
interface Tab {
  name: string
  label: string
}

interface Props {
  tabs: Tab[]
  modelValue: string
}

defineProps<Props>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const handleClick = (name: string) => {
  emit('update:modelValue', name)
}
</script>

<style scoped>
.custom-tabs {
  margin-bottom: var(--spacing-xl);
}

.custom-tabs-header {
  display: flex;
  gap: var(--spacing-xs);
  border-bottom: 2px solid var(--color-border);
  padding-bottom: 0;
}

.custom-tab {
  padding: var(--spacing-md) var(--spacing-xl);
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-medium);
  cursor: pointer;
  position: relative;
  transition: all var(--transition-base);
  border-radius: var(--radius-md) var(--radius-md) 0 0;
}

.custom-tab:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-hover);
}

.custom-tab.is-active {
  color: var(--color-primary);
  font-weight: var(--font-weight-semibold);
}

.custom-tab.is-active::after {
  content: '';
  position: absolute;
  bottom: -2px;
  left: 0;
  right: 0;
  height: 2px;
  background: var(--color-primary);
}
</style>
