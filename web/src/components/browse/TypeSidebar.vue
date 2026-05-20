<template>
  <div class="type-sidebar">
    <div
      v-for="group in groups"
      :key="group.type"
      class="type-item"
      :class="{ 'type-active': activeType === group.type }"
      @click="$emit('update:activeType', group.type)"
    >
      <span class="type-icon" :style="{ background: group.color }"><i :class="group.icon"></i></span>
      <span class="type-label">{{ group.label }}</span>
      <span class="type-count">{{ group.count }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  activeType: string
  groups: Array<{ type: string; label: string; icon: string; color: string; count: number }>
}>()

defineEmits<{
  'update:activeType': [type: string]
}>()
</script>

<style scoped>
.type-sidebar {
  width: 160px;
  flex-shrink: 0;
  background: var(--lunar-bg-card);
  border: 1px solid var(--lunar-border);
  border-radius: 10px;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 500px;
  overflow-y: auto;
}

.type-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}

.type-item:hover {
  background: var(--lunar-bg-glass);
}

.type-active {
  background: rgba(196, 181, 253, 0.12);
  border: 1px solid var(--lunar-border-hover);
}

.type-icon {
  width: 24px;
  height: 24px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  flex-shrink: 0;
}

.type-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--lunar-silver);
  flex: 1;
}

.type-count {
  font-size: 12px;
  color: var(--lunar-silver-dim);
  background: var(--lunar-bg-glass);
  padding: 2px 6px;
  border-radius: 10px;
  min-width: 20px;
  text-align: center;
}

.type-active .type-count {
  background: rgba(196, 181, 253, 0.15);
  color: var(--lunar-accent);
}
</style>