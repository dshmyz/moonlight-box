<template>
  <nav class="browse-tabs">
    <button
      v-for="tab in tabs"
      :key="tab.value"
      class="tab-item"
      :class="{ 'tab-item--active': activeTab === tab.value }"
      @click="$emit('update:activeTab', tab.value)"
    >
      {{ tab.label }}
      <span v-if="tab.value === 'packages' && packageCount && packageCount > 0" class="tab-count">
        {{ packageCount }}
      </span>
    </button>
  </nav>
</template>

<script setup lang="ts">
defineProps<{
  activeTab: 'packages' | 'repositories'
  packageCount?: number
}>()

defineEmits<{
  'update:activeTab': [tab: 'packages' | 'repositories']
}>()

const tabs = [
  { value: 'packages' as const, label: '软件包' },
  { value: 'repositories' as const, label: '仓库' },
]
</script>

<style scoped>
.browse-tabs {
  display: flex;
  gap: 24px;
  border-bottom: 1px solid var(--lunar-border);
  margin-top: 16px;
}
.tab-item {
  position: relative;
  padding: 10px 0;
  border: none;
  background: transparent;
  color: var(--lunar-silver-muted);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: color var(--transition-fast);
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.tab-item:hover {
  color: var(--lunar-silver);
}
.tab-item--active {
  color: var(--lunar-silver);
  font-weight: 600;
}
.tab-item--active::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: -1px;
  height: 2px;
  background: var(--lunar-accent);
}
.tab-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 16px;
  padding: 0 5px;
  font-size: 11px;
  font-weight: 500;
  color: var(--lunar-silver-dim);
  background: var(--lunar-bg-glass);
  border-radius: var(--radius-full);
}
</style>
