<template>
  <div class="storage-card">
    <div class="storage-header">
      <span class="storage-title">存储容量</span>
      <span class="storage-percent" :style="{ color: progressColor }">{{ usagePercent }}%</span>
    </div>
    <div class="storage-content">
      <div class="storage-ring">
        <svg viewBox="0 0 100 100">
          <circle
            class="ring-bg"
            cx="50" cy="50" r="42"
            fill="none"
            stroke-width="8"
          />
          <circle
            class="ring-progress"
            cx="50" cy="50" r="42"
            fill="none"
            stroke-width="8"
            :stroke="progressColor"
            :stroke-dasharray="circumference"
            :stroke-dashoffset="dashOffset"
            stroke-linecap="round"
          />
        </svg>
        <div class="ring-center">
          <span class="ring-icon">◈</span>
        </div>
      </div>
      <div class="storage-details">
        <div class="detail-row">
          <span class="detail-label">已使用</span>
          <span class="detail-value used">{{ formatBytes(storage.used_bytes) }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">总容量</span>
          <span class="detail-value total">{{ formatBytes(storage.total_bytes) }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">可用</span>
          <span class="detail-value available">{{ formatBytes(storage.total_bytes - storage.used_bytes) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { StorageInfo } from '@/api/dashboard'

const props = defineProps<{
  storage: StorageInfo
}>()

const usagePercent = computed(() => Math.round(props.storage.usage_percent))

const circumference = 2 * Math.PI * 42

const dashOffset = computed(() => {
  return circumference - (usagePercent.value / 100) * circumference
})

const progressColor = computed(() => {
  if (usagePercent.value >= 90) return '#ef4444'
  if (usagePercent.value >= 70) return '#f59e0b'
  return '#3b82f6'
})

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
}
</script>

<style scoped>
.storage-card {
  background: #ffffff;
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: 16px;
  padding: 20px 24px;
  height: 100%;
}

.storage-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.storage-title {
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
}

.storage-percent {
  font-size: 14px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.storage-content {
  display: flex;
  align-items: center;
  gap: 24px;
}

.storage-ring {
  position: relative;
  width: 100px;
  height: 100px;
  flex-shrink: 0;
}

.storage-ring svg {
  transform: rotate(-90deg);
}

.ring-bg {
  stroke: rgba(0, 0, 0, 0.06);
}

.ring-progress {
  transition: stroke-dashoffset 0.8s cubic-bezier(0.4, 0, 0.2, 1);
}

.ring-center {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.ring-icon {
  font-size: 24px;
  color: #94a3b8;
}

.storage-details {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.detail-label {
  font-size: 13px;
  color: #64748b;
}

.detail-value {
  font-size: 13px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.detail-value.used { color: #f87171; }
.detail-value.total { color: #94a3b8; }
.detail-value.available { color: #34d399; }
</style>
