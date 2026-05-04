<template>
  <span :class="['type-tag', `type-tag--${type}`]">
    {{ type.toUpperCase() }}
  </span>
</template>

<script setup lang="ts">
interface Props {
  type: 'npm' | 'pypi' | 'maven' | 'nuget' | 'go'
}

defineProps<Props>()
</script>

<style scoped>
.type-tag {
  padding: var(--spacing-xs) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-medium);
  display: inline-block;
}

.type-tag--npm {
  background: #dbeafe;
  color: #1e40ab;
}

.type-tag--pypi {
  background: #dcfce7;
  color: #15803d;
}

.type-tag--maven {
  background: #fef3c7;
  color: #92400e;
}

.type-tag--nuget {
  background: #fce7f3;
  color: #9f1239;
}

.type-tag--go {
  background: #e0e7ff;
  color: #3730a3;
}
</style>
