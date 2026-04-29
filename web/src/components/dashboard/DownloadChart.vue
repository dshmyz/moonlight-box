<template>
  <el-card shadow="hover" class="download-chart-card">
    <template #header>
      <span>下载量趋势（7 天）</span>
    </template>
    <div class="chart-container">
      <div class="bar-chart">
        <div
          v-for="(count, index) in data"
          :key="index"
          class="bar-wrapper"
        >
          <div class="bar-value">{{ formatNumber(count) }}</div>
          <div
            class="bar"
            :style="{ height: getBarHeight(count) + 'px' }"
          />
          <div class="bar-label">{{ getDayLabel(index) }}</div>
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  data: number[]
}>()

/** 计算数据中的最大值（避免空数组情况） */
const maxValue = computed(() => Math.max(...props.data, 1))

/** 根据数据和最大值计算柱状图高度，最大 120px，最小 4px */
const getBarHeight = (count: number) => {
  return Math.max((count / maxValue.value) * 120, 4)
}

/** 格式化数字：超过 1000 显示为 K */
const formatNumber = (num: number) => {
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}

/** 根据索引生成过去 7 天的星期标签 */
const getDayLabel = (index: number) => {
  const days = ['日', '一', '二', '三', '四', '五', '六']
  const date = new Date()
  date.setDate(date.getDate() - (6 - index))
  return '周' + days[date.getDay()]
}
</script>

<style scoped>
.chart-container {
  padding: 16px 0;
}
.bar-chart {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  height: 180px;
  padding: 0 8px;
}
.bar-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}
.bar {
  width: 24px;
  background: linear-gradient(180deg, #409eff 0%, #337ecc 100%);
  border-radius: 4px 4px 0 0;
  transition: height 0.3s ease;
}
.bar-value {
  font-size: 11px;
  color: #606266;
  font-weight: 500;
}
.bar-label {
  font-size: 12px;
  color: #909399;
}
</style>
