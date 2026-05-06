<template>
  <div :class="['stat-card', `stat-card--${variant}`]">
    <div class="stat-card__content">
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
  variant?: 'blue' | 'green' | 'orange' | 'purple'
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
  position: relative;
  padding: 20px 22px;
  border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  transition: transform 0.25s ease, box-shadow 0.25s ease;
  background: #ffffff;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.08);
}

.stat-card__content {
  position: relative;
  z-index: 1;
}

.stat-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.stat-card__label {
  font-size: 13px;
  font-weight: 500;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  color: #64748b;
}

.stat-card__icon {
  font-size: 20px;
}

.stat-card__value {
  font-size: 30px;
  font-weight: 700;
  letter-spacing: -0.02em;
  margin-bottom: 8px;
  font-variant-numeric: tabular-nums;
  color: #1e293b;
}

.stat-card__trend {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #64748b;
}

.stat-card__trend-icon {
  font-size: 13px;
}

/* Blue */
.stat-card--blue {
  background: linear-gradient(145deg, #f8fafc 0%, #f1f5f9 100%);
  border-color: rgba(59, 130, 246, 0.15);
}
.stat-card--blue .stat-card__icon { color: #3b82f6; }

/* Green */
.stat-card--green {
  background: linear-gradient(145deg, #f0fdf4 0%, #f0fdfa 100%);
  border-color: rgba(16, 185, 129, 0.15);
}
.stat-card--green .stat-card__icon { color: #10b981; }

/* Orange */
.stat-card--orange {
  background: linear-gradient(145deg, #fffbeb 0%, #fefce8 100%);
  border-color: rgba(245, 158, 11, 0.15);
}
.stat-card--orange .stat-card__icon { color: #f59e0b; }

/* Purple */
.stat-card--purple {
  background: linear-gradient(145deg, #faf5ff 0%, #f5f3ff 100%);
  border-color: rgba(139, 92, 246, 0.15);
}
.stat-card--purple .stat-card__icon { color: #8b5cf6; }
</style>
