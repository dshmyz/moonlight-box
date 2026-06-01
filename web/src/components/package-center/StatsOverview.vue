<template>
  <section class="stats-overview">
    <div v-for="(item, index) in statCards" :key="index" class="stat-card" :class="`stat-card--${item.type}`">
      <div class="stat-header">
        <div class="stat-icon" :class="`stat-icon--${item.type}`">
          <i :class="item.icon"></i>
        </div>
        <span v-if="item.trend" class="stat-trend" :class="item.trend > 0 ? 'stat-trend--up' : 'stat-trend--down'">
          {{ item.trend > 0 ? '↑' : '↓' }} {{ Math.abs(item.trend) }}%
        </span>
      </div>
      <div class="stat-value">{{ item.value }}</div>
      <div class="stat-label">{{ item.label }}</div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { formatNumber } from '@/utils/format'
import type { DashboardStats } from '@/api/dashboard'

const props = defineProps<{
  stats: DashboardStats | null
  loading: boolean
}>()

const statCards = computed(() => {
  const totalPackages = props.stats?.repositories?.reduce((sum, r) => sum + r.package_count, 0) || 0
  const totalDownloads = props.stats?.repositories?.reduce((sum, r) => sum + r.download_count_today, 0) || 0
  const activeRepos = props.stats?.repositories?.filter(r => r.status === 'active').length || 0
  const cacheHitRate = props.stats?.cache?.hit_rate || 0

  return [
    {
      type: 'packages',
      icon: 'fa-solid fa-box',
      value: formatNumber(totalPackages),
      label: '总包数量',
      trend: 12,
    },
    {
      type: 'downloads',
      icon: 'fa-solid fa-download',
      value: formatNumber(totalDownloads),
      label: '今日下载',
      trend: 23,
    },
    {
      type: 'repos',
      icon: 'fa-solid fa-database',
      value: activeRepos.toString(),
      label: '活跃仓库',
      trend: -2,
    },
    {
      type: 'cache',
      icon: 'fa-solid fa-bolt',
      value: `${(cacheHitRate * 100).toFixed(1)}%`,
      label: '缓存命中率',
      trend: 5,
    },
  ]
})
</script>

<style scoped>
.stats-overview {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 20px;
  max-width: 1400px;
  margin: 0 auto;
  padding: 32px 32px 0;
}

.stat-card {
  background: var(--color-bg-primary, #ffffff);
  border-radius: 16px;
  padding: 24px;
  border: 1px solid var(--color-border, #e2e8f0);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
  overflow: hidden;
  transform: translateZ(0);
}

.stat-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 4px;
  opacity: 0;
  transition: opacity 0.3s;
}

.stat-card--packages::before {
  background: linear-gradient(90deg, #3b82f6 0%, #60a5fa 100%);
}

.stat-card--downloads::before {
  background: linear-gradient(90deg, #10b981 0%, #34d399 100%);
}

.stat-card--repos::before {
  background: linear-gradient(90deg, #f59e0b 0%, #fbbf24 100%);
}

.stat-card--cache::before {
  background: linear-gradient(90deg, #8b5cf6 0%, #a78bfa 100%);
}

.stat-card:hover {
  transform: translateY(-2px) translateZ(0);
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.08);
}

.stat-card:hover::before {
  opacity: 1;
}

.stat-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
}

.stat-icon--packages {
  background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%);
  color: #3b82f6;
}

.stat-icon--downloads {
  background: linear-gradient(135deg, #d1fae5 0%, #a7f3d0 100%);
  color: #10b981;
}

.stat-icon--repos {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #f59e0b;
}

.stat-icon--cache {
  background: linear-gradient(135deg, #ede9fe 0%, #c4b5fd 100%);
  color: #8b5cf6;
}

.stat-trend {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 20px;
  font-weight: 600;
}

.stat-trend--up {
  background: #d1fae5;
  color: #059669;
}

.stat-trend--down {
  background: #fee2e2;
  color: #dc2626;
}

.stat-value {
  font-size: 32px;
  font-weight: 700;
  color: var(--color-text-primary, #0f172a);
  line-height: 1;
  margin-bottom: 8px;
}

.stat-label {
  font-size: 14px;
  color: var(--color-text-tertiary, #94a3b8);
}

@media (max-width: 1024px) {
  .stats-overview {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 640px) {
  .stats-overview {
    grid-template-columns: 1fr;
  }
}
</style>
