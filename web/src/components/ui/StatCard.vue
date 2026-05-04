<template>
  <div :class="['stat-card', `stat-card--${variant}`]">
    <div class="stat-card__header">
      <span class="stat-card__label">{{ label }}</span>
      <span class="stat-card__icon">{{ icon }}</span>
    </div>
    <div class="stat-card__value">{{ formattedValue }}</div>
    <div v-if="trend" class="stat-card__trend">
      <span class="stat-card__trend-icon">{{ trend > 0 ? '↑' : '↓' }}</span>
      <span class="stat-card__trend-text">{{ Math.abs(trend) }}% 较{{ trendPeriod }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  label: string
  value: number | string
  icon?: string
  trend?: number
  trendPeriod?: string
  variant?: 'blue' | 'green' | 'orange' | 'pink'
}

const props = withDefaults(defineProps<Props>(), {
  icon: '',
  trend: 0,
  trendPeriod: '上周',
  variant: 'blue',
})

const formattedValue = computed(() => {
  if (typeof props.value === 'string') return props.value
  if (props.value >= 1000) {
    return (props.value / 1000).toFixed(1) + 'K'
  }
  return props.value.toLocaleString()
})
</script>

<style scoped>
.stat-card {
  padding: var(--spacing-xl);
  border-radius: var(--radius-xl);
  border: 1px solid;
  transition: transform var(--transition-fast), box-shadow var(--transition-fast);
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
}

.stat-card--blue {
  background: linear-gradient(135deg, #eff6ff 0%, #dbeafe 100%);
  border-color: #bfdbfe;
}

.stat-card--blue .stat-card__label {
  color: var(--color-primary);
}

.stat-card--blue .stat-card__value {
  color: #1e40af;
}

.stat-card--blue .stat-card__trend-text {
  color: #60a5fa;
}

.stat-card--green {
  background: linear-gradient(135deg, #f0fdf4 0%, #dcfce7 100%);
  border-color: #bbf7d0;
}

.stat-card--green .stat-card__label {
  color: var(--color-success);
}

.stat-card--green .stat-card__value {
  color: #15803d;
}

.stat-card--green .stat-card__trend-text {
  color: #4ade80;
}

.stat-card--orange {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  border-color: #fcd34d;
}

.stat-card--orange .stat-card__label {
  color: var(--color-warning);
}

.stat-card--orange .stat-card__value {
  color: #92400e;
}

.stat-card--orange .stat-card__trend-text {
  color: #fbbf24;
}

.stat-card--pink {
  background: linear-gradient(135deg, #fce7f3 0%, #fbcfe8 100%);
  border-color: #f9a8d4;
}

.stat-card--pink .stat-card__label {
  color: #db2777;
}

.stat-card--pink .stat-card__value {
  color: #9f1239;
}

.stat-card--pink .stat-card__trend-text {
  color: #f472b6;
}

.stat-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--spacing-sm);
}

.stat-card__label {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
}

.stat-card__icon {
  font-size: 18px;
}

.stat-card__value {
  font-size: var(--font-size-3xl);
  font-weight: var(--font-weight-bold);
  margin-bottom: var(--spacing-xs);
}

.stat-card__trend {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-xs);
}

.stat-card__trend-icon {
  font-size: 14px;
}
</style>
