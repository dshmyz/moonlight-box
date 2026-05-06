<template>
  <el-row :gutter="16" class="stat-cards">
    <el-col :xs="12" :sm="12" :md="6">
      <StatCard
        label="总仓库数"
        :value="totalRepos"
        icon="◈"
        :trend="repoTrend"
        trend-period="上周"
        variant="blue"
      />
    </el-col>
    <el-col :xs="12" :sm="12" :md="6">
      <StatCard
        label="总包数量"
        :value="totalPackages"
        icon="◇"
        :trend="packageTrend"
        trend-period="上周"
        variant="green"
      />
    </el-col>
    <el-col :xs="12" :sm="12" :md="6">
      <StatCard
        label="下载量"
        :value="totalDownloads"
        icon="▾"
        :trend="downloadTrend"
        trend-period="上周"
        variant="orange"
      />
    </el-col>
    <el-col :xs="12" :sm="12" :md="6">
      <StatCard
        label="缓存命中率"
        :value="cacheHitRateDisplay"
        icon="◉"
        :trend="cacheTrend"
        trend-period="上周"
        variant="purple"
      />
    </el-col>
  </el-row>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import StatCard from '@/components/ui/StatCard.vue'
import type { DashboardStats } from '@/api/dashboard'

const props = defineProps<{
  stats: DashboardStats
}>()

/** 总仓库数 */
const totalRepos = computed(() => {
  return props.stats.repositories.length
})

/** 总包数量 */
const totalPackages = computed(() => {
  return props.stats.repositories.reduce((sum, repo) => sum + repo.package_count, 0)
})

/** 总下载量 */
const totalDownloads = computed(() => {
  return props.stats.downloads_last_7_days.reduce((sum, count) => sum + count, 0)
})

/** 缓存命中率显示值 */
const cacheHitRateDisplay = computed(() => {
  return `${props.stats.cache.hit_rate.toFixed(1)}%`
})

/** 仓库趋势（模拟数据，实际应从后端获取） */
const repoTrend = computed(() => {
  // 这里应该从后端数据计算，暂时返回模拟值
  return 5.2
})

/** 包数量趋势（模拟数据，实际应从后端获取） */
const packageTrend = computed(() => {
  // 这里应该从后端数据计算，暂时返回模拟值
  return 12.8
})

/** 下载量趋势（模拟数据，实际应从后端获取） */
const downloadTrend = computed(() => {
  // 这里应该从后端数据计算，暂时返回模拟值
  return 8.5
})

/** 缓存命中率趋势（模拟数据，实际应从后端获取） */
const cacheTrend = computed(() => {
  // 这里应该从后端数据计算，暂时返回模拟值
  return 3.2
})
</script>

<style scoped>
.stat-cards {
  margin-bottom: 24px;
}
</style>
