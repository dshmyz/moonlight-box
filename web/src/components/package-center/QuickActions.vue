<template>
  <section class="quick-actions">
    <div
      v-for="action in actions"
      :key="action.type"
      class="quick-action-card"
      @click="$emit('filter', action.type)"
    >
      <div class="quick-action-icon" :class="`quick-action-icon--${action.type}`">
        <i :class="action.icon"></i>
      </div>
      <div class="quick-action-info">
        <h4>{{ action.label }}</h4>
        <p>{{ formatNumber(packageTypeStats[action.type] || 0) }} 个包 · {{ action.ecosystem }}</p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { formatNumber } from '@/utils/format'

defineProps<{
  packageTypeStats: Record<string, number>
}>()

defineEmits<{
  filter: [type: string]
}>()

const actions = [
  { type: 'npm', label: 'NPM 包管理', icon: 'fa-brands fa-npm', ecosystem: 'Node.js 生态' },
  { type: 'maven', label: 'Maven 包管理', icon: 'fa-solid fa-java', ecosystem: 'Java 生态' },
  { type: 'pypi', label: 'PyPI 包管理', icon: 'fa-brands fa-python', ecosystem: 'Python 生态' },
  { type: 'go', label: 'Go 模块管理', icon: 'fa-solid fa-cubes', ecosystem: 'Go 生态' },
]
</script>

<style scoped>
.quick-actions {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  max-width: 1400px;
  margin: 0 auto;
  padding: 24px 32px;
}

.quick-action-card {
  background: var(--color-bg-primary, #ffffff);
  border-radius: 14px;
  padding: 20px;
  border: 1px solid var(--color-border, #e2e8f0);
  display: flex;
  align-items: center;
  gap: 16px;
  cursor: pointer;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  transform: translateZ(0);
}

.quick-action-card:hover {
  border-color: var(--color-primary-light, #818cf8);
  box-shadow: 0 8px 20px rgba(99, 102, 241, 0.12);
  transform: translateY(-2px) translateZ(0);
}

.quick-action-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  flex-shrink: 0;
}

.quick-action-icon--npm {
  background: linear-gradient(135deg, #dbeafe 0%, #93c5fd 100%);
  color: #1d4ed8;
}

.quick-action-icon--maven {
  background: linear-gradient(135deg, #dcfce7 0%, #86efac 100%);
  color: #16a34a;
}

.quick-action-icon--pypi {
  background: linear-gradient(135deg, #fef3c7 0%, #fcd34d 100%);
  color: #d97706;
}

.quick-action-icon--go {
  background: linear-gradient(135deg, #ccfbf1 0%, #5eead4 100%);
  color: #0d9488;
}

.quick-action-info h4 {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-primary, #0f172a);
  margin-bottom: 4px;
}

.quick-action-info p {
  font-size: 12px;
  color: var(--color-text-tertiary, #94a3b8);
}

@media (max-width: 1024px) {
  .quick-actions {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 640px) {
  .quick-actions {
    grid-template-columns: 1fr;
  }
}
</style>
