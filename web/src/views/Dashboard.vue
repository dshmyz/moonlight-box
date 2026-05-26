<template>
  <div class="dashboard">
    <header class="page-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-gauge-high"></i>
        </div>
        <div class="header-text">
          <h2>仪表盘</h2>
          <p class="header-subtitle">系统概览与实时统计</p>
        </div>
      </div>
      <el-button class="refresh-btn" @click="loadStats" :loading="loading">
        <i class="fa-solid fa-rotate"></i>
        <span>刷新</span>
      </el-button>
    </header>

    <div class="content-panel" v-loading="loading">
      <StatCards :stats="stats" />

      <div class="section">
        <h3 class="section-title">
          <i class="fa-solid fa-boxes"></i>
          <span>仓库状态</span>
        </h3>
        <el-row :gutter="16" class="repo-grid">
          <el-col
            v-for="repo in stats.repositories"
            :key="repo.name"
            :xs="24" :sm="12" :md="8" :lg="6"
          >
            <RepoStatusCard :repo="repo" />
          </el-col>
        </el-row>
      </div>

      <el-row :gutter="16" class="charts-row">
        <el-col :span="16">
          <DownloadChart :data="stats.downloads_last_7_days" />
        </el-col>
        <el-col :span="8">
          <StorageCard :storage="stats.storage" />
        </el-col>
      </el-row>

      <el-row :gutter="16" class="bottom-row">
        <el-col :xs="24" :md="14">
          <TopPackages :packages="stats.top_packages" />
        </el-col>
        <el-col :xs="24" :md="10">
          <ActivityFeed :activities="activities" />
        </el-col>
      </el-row>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { dashboardApi, type DashboardStats } from '@/api/dashboard'
import { ElMessage } from 'element-plus'
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
    const data = await dashboardApi.getStats()
    stats.value = {
      repositories: data.repositories ?? [],
      storage: data.storage ?? { total_bytes: 0, used_bytes: 0, usage_percent: 0 },
      cache: data.cache ?? { hit_rate: 0, total_entries: 0 },
      downloads_last_7_days: data.downloads_last_7_days ?? [],
      top_packages: data.top_packages ?? [],
    }
  } catch (err) {
    ElMessage.error('获取仪表盘数据失败')
    console.error('获取仪表盘统计数据失败:', err)
  } finally {
    loading.value = false
  }
}

onMounted(loadStats)
</script>

<style scoped>
.dashboard {
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  background: #fff;
  border-radius: 16px;
  margin-bottom: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 24px;
}

.header-text h2 {
  font-size: 20px;
  font-weight: 600;
  margin: 0;
  color: #1f2937;
  letter-spacing: -0.2px;
}

.header-subtitle {
  font-size: 13px;
  color: #9ca3af;
  margin: 4px 0 0;
}

.refresh-btn {
  height: 40px;
  padding: 0 20px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: #fff;
  border-color: #e5e7eb;
  color: #374151;
}

.refresh-btn:hover {
  background: #f9fafb;
}

.content-panel {
  padding: 0;
}

.section {
  margin-bottom: 20px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #374151;
  margin: 0 0 16px;
}

.section-title i {
  color: #8b5cf6;
}

.repo-grid {
  margin-bottom: 0;
}

.charts-row,
.bottom-row {
  margin-top: 20px;
}
</style>
