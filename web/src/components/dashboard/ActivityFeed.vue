<template>
  <div class="activity-card">
    <div class="card-header">
      <span class="card-title">
        <span class="title-icon">◈</span>
        最近活动
      </span>
    </div>
    <el-empty v-if="activities.length === 0" description="暂无活动记录" />
    <div v-else class="activity-list">
      <div
        v-for="(activity, index) in activities"
        :key="activity.id"
        class="activity-item"
        :style="{ '--delay': index * 0.05 + 's' }"
      >
        <div class="activity-timeline">
          <div class="timeline-dot" :class="`dot--${activity.type}`" />
          <div v-if="index < activities.length - 1" class="timeline-line" />
        </div>
        <div class="activity-content">
          <span class="activity-time">{{ activity.time }}</span>
          <span class="activity-description">{{ activity.description }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
interface Activity {
  id: number
  time: string
  type: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  description: string
}

defineProps<{
  activities: Activity[]
}>()
</script>

<style scoped>
.activity-card {
  background: #ffffff;
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: 16px;
  padding: 20px 24px;
  height: 100%;
}

.card-header {
  margin-bottom: 20px;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
}

.title-icon {
  font-size: 18px;
  color: #818cf8;
}

.activity-list {
  display: flex;
  flex-direction: column;
}

.activity-item {
  display: flex;
  gap: 14px;
  animation: fadeSlideIn 0.3s ease forwards;
  animation-delay: var(--delay);
  opacity: 0;
}

@keyframes fadeSlideIn {
  from {
    opacity: 0;
    transform: translateX(-8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

.activity-timeline {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex-shrink: 0;
}

.timeline-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
  margin-top: 4px;
}

.dot--primary {
  background: #3b82f6;
  box-shadow: 0 0 8px rgba(59, 130, 246, 0.6);
}

.dot--success {
  background: #10b981;
  box-shadow: 0 0 8px rgba(16, 185, 129, 0.6);
}

.dot--warning {
  background: #f59e0b;
  box-shadow: 0 0 8px rgba(245, 158, 11, 0.6);
}

.dot--danger {
  background: #ef4444;
  box-shadow: 0 0 8px rgba(239, 68, 68, 0.6);
}

.dot--info {
  background: #94a3b8;
  box-shadow: 0 0 8px rgba(148, 163, 184, 0.4);
}

.timeline-line {
  width: 2px;
  flex: 1;
  min-height: 24px;
  background: linear-gradient(180deg, rgba(0,0,0,0.08), rgba(0,0,0,0.03));
  margin: 6px 0;
}

.activity-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-bottom: 16px;
  flex: 1;
}

.activity-time {
  font-size: 12px;
  color: #64748b;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.activity-description {
  font-size: 14px;
  color: #475569;
  line-height: 1.5;
}
</style>
