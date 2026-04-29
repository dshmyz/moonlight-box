<template>
  <el-card shadow="hover" class="activity-card">
    <template #header>
      <span>最近活动</span>
    </template>
    <el-empty v-if="activities.length === 0" description="暂无活动记录" />
    <el-timeline v-else>
      <el-timeline-item
        v-for="activity in activities"
        :key="activity.id"
        :timestamp="activity.time"
        :type="activity.type"
        placement="top"
      >
        {{ activity.description }}
      </el-timeline-item>
    </el-timeline>
  </el-card>
</template>

<script setup lang="ts">
/** 活动记录数据类型 */
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
.activity-card :deep(.el-timeline-item__timestamp) {
  font-size: 12px;
  color: #909399;
}
</style>
