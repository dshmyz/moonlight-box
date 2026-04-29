<template>
  <el-card shadow="hover" class="storage-card">
    <template #header>
      <div class="card-header">
        <span>存储容量</span>
        <span class="usage-text">{{ usagePercent }}% 已用</span>
      </div>
    </template>
    <el-progress
      :percentage="usagePercent"
      :color="progressColor"
      :stroke-width="20"
      :show-text="false"
    />
    <div class="storage-details">
      <div class="detail-item">
        <span class="label">已使用</span>
        <span class="value">{{ formatBytes(storage.used_bytes) }}</span>
      </div>
      <div class="detail-item">
        <span class="label">总容量</span>
        <span class="value">{{ formatBytes(storage.total_bytes) }}</span>
      </div>
      <div class="detail-item">
        <span class="label">可用</span>
        <span class="value">{{ formatBytes(storage.total_bytes - storage.used_bytes) }}</span>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { StorageInfo } from '@/api/dashboard'

const props = defineProps<{
  storage: StorageInfo
}>()

/** 计算使用百分比（取整） */
const usagePercent = computed(() => Math.round(props.storage.usage_percent))

/** 根据使用百分比动态设置进度条颜色：红色（>=90%）、橙色（>=70%）、蓝色 */
const progressColor = computed(() => {
  if (usagePercent.value >= 90) return '#f56c6c'
  if (usagePercent.value >= 70) return '#e6a23c'
  return '#409eff'
})

/** 格式化字节数为人类可读单位 */
const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
}
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.usage-text {
  font-weight: 600;
  color: #606266;
}
.storage-details {
  display: flex;
  justify-content: space-between;
  margin-top: 16px;
}
.detail-item {
  text-align: center;
}
.detail-item .label {
  display: block;
  font-size: 12px;
  color: #909399;
}
.detail-item .value {
  display: block;
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-top: 4px;
}
</style>
