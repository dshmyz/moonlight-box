<template>
  <div class="dashboard">
    <div class="page-header">
      <h2>仪表盘</h2>
      <el-button @click="loadStats" :loading="loading" circle>
        <el-icon><Refresh /></el-icon>
      </el-button>
    </div>

    <section v-loading="loading">
      <StatCards :stats="stats" />

      <el-row :gutter="16">
        <el-col :span="24">
          <h3 class="section-title">仓库状态</h3>
        </el-col>
      </el-row>
      <el-row :gutter="16" class="repo-grid">
        <el-col
          v-for="repo in stats.repositories"
          :key="repo.name"
          :xs="24" :sm="12" :md="8" :lg="6"
        >
          <RepoStatusCard :repo="repo" />
        </el-col>
      </el-row>

      <el-row :gutter="16" class="chart-row">
        <el-col :span="16">
          <DownloadChart :data="stats.downloads_last_7_days" />
        </el-col>
        <el-col :span="8">
          <StorageCard :storage="stats.storage" />
        </el-col>
      </el-row>

      <el-row :gutter="16" class="activity-row">
        <el-col :xs="24" :md="14">
          <TopPackages :packages="stats.top_packages" />
        </el-col>
        <el-col :xs="24" :md="10">
          <ActivityFeed :activities="activities" />
        </el-col>
      </el-row>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { dashboardApi, type DashboardStats } from '@/api/dashboard'
import RepoStatusCard from '@/components/dashboard/RepoStatusCard.vue'
import DownloadChart from '@/components/dashboard/DownloadChart.vue'
import StorageCard from '@/components/dashboard/StorageCard.vue'
import ActivityFeed from '@/components/dashboard/ActivityFeed.vue'
import TopPackages from '@/components/dashboard/TopPackages.vue'
import StatCards from '@/components/dashboard/StatCards.vue'

/** 加载状态 */
const loading = ref(false)

/** 仪表盘统计数据 */
const stats = ref<DashboardStats>({
  repositories: [],
  storage: { total_bytes: 0, used_bytes: 0, usage_percent: 0 },
  cache: { hit_rate: 0, total_entries: 0 },
  downloads_last_7_days: [],
  top_packages: [],
})

/** 活动记录数据类型 */
interface Activity {
  id: number
  time: string
  type: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  description: string
}

/** 最近活动列表（初始为演示数据） */
const activities = ref<Activity[]>([
  { id: 1, time: '14:32', type: 'primary', description: '系统初始化完成，创建默认仓库' },
  { id: 2, time: '14:30', type: 'success', description: 'admin 用户登录' },
])

/** 加载仪表盘统计数据 */
const loadStats = async () => {
  loading.value = true
  try {
    const res = await dashboardApi.getStats()
    const data = res as any
    stats.value = {
      repositories: data?.repositories ?? [],
      storage: data?.storage ?? { total_bytes: 0, used_bytes: 0, usage_percent: 0 },
      cache: data?.cache ?? { hit_rate: 0, total_entries: 0 },
      downloads_last_7_days: data?.downloads_last_7_days ?? [],
      top_packages: data?.top_packages ?? [],
    }
  } catch (err) {
    console.error('获取仪表盘统计数据失败:', err)
  } finally {
    loading.value = false
  }
}

onMounted(loadStats)
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  margin: 0;
  color: var(--color-text-primary);
}

.section-title {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-lg);
}

.repo-grid {
  margin-bottom: var(--spacing-sm);
}

.chart-row,
.activity-row {
  margin-top: var(--spacing-xl);
}
</style>
