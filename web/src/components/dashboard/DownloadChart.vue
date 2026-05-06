<template>
  <div class="download-chart-card">
    <div class="chart-header">
      <span class="chart-title">下载量趋势</span>
      <span class="chart-subtitle">最近 7 天</span>
    </div>
    <div class="chart-container">
      <div class="bar-chart">
        <div
          v-for="(count, index) in data"
          :key="index"
          class="bar-wrapper"
        >
          <div class="bar-tooltip">{{ formatNumber(count) }}</div>
          <div class="bar-track">
            <div
              class="bar"
              :style="{ height: getBarHeight(count) + '%' }"
              :class="`bar--${index}`"
            />
          </div>
          <div class="bar-label">{{ getDayLabel(index) }}</div>
        </div>
      </div>
      <div class="chart-grid">
        <div class="grid-line" v-for="i in 4" :key="i" :style="{ bottom: (i * 25) + '%' }" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  data: number[]
}>()

const maxValue = computed(() => Math.max(...props.data, 1))

const getBarHeight = (count: number) => {
  return Math.max((count / maxValue.value) * 100, 4)
}

const formatNumber = (num: number) => {
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}

const getDayLabel = (index: number) => {
  const days = ['日', '一', '二', '三', '四', '五', '六']
  const date = new Date()
  date.setDate(date.getDate() - (6 - index))
  return '周' + days[date.getDay()]
}
</script>

<style scoped>
.download-chart-card {
  background: #ffffff;
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: 16px;
  padding: 20px 24px;
  position: relative;
  overflow: hidden;
}

.chart-header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 24px;
}

.chart-title {
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
}

.chart-subtitle {
  font-size: 12px;
  color: #64748b;
}

.chart-container {
  position: relative;
  height: 180px;
}

.bar-chart {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  height: 100%;
  padding: 0 4px;
  position: relative;
  z-index: 2;
}

.chart-grid {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
}

.grid-line {
  position: absolute;
  left: 0;
  right: 0;
  height: 1px;
  background: rgba(0, 0, 0, 0.04);
}

.bar-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  height: 100%;
  justify-content: flex-end;
  position: relative;
}

.bar-tooltip {
  position: absolute;
  top: -24px;
  font-size: 11px;
  color: #94a3b8;
  font-variant-numeric: tabular-nums;
  opacity: 0;
  transition: opacity 0.2s;
}

.bar-wrapper:hover .bar-tooltip {
  opacity: 1;
}

.bar-track {
  flex: 1;
  width: 28px;
  background: rgba(0, 0, 0, 0.03);
  border-radius: 6px 6px 0 0;
  display: flex;
  align-items: flex-end;
  overflow: hidden;
}

.bar {
  width: 100%;
  border-radius: 6px 6px 0 0;
  transition: height 0.5s cubic-bezier(0.34, 1.56, 0.64, 1);
  position: relative;
}

.bar::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, rgba(255,255,255,0.2) 0%, transparent 60%);
  border-radius: inherit;
}

.bar--0 { background: linear-gradient(180deg, #dbeafe 0%, #93c5fd 100%); }
.bar--1 { background: linear-gradient(180deg, #e9d5ff 0%, #c4b5fd 100%); }
.bar--2 { background: linear-gradient(180deg, #fbcfe8 0%, #f9a8d4 100%); }
.bar--3 { background: linear-gradient(180deg, #fef3c7 0%, #fde68a 100%); }
.bar--4 { background: linear-gradient(180deg, #dcfce7 0%, #bbf7d0 100%); }
.bar--5 { background: linear-gradient(180deg, #ccfbf1 0%, #99f6e4 100%); }
.bar--6 { background: linear-gradient(180deg, #e0e7ff 0%, #c7d2fe 100%); }

.bar-label {
  font-size: 11px;
  color: #64748b;
  font-weight: 500;
}
</style>
