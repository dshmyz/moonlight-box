<template>
  <el-row :gutter="16" class="stat-cards">
    <el-col :xs="12" :sm="6">
      <div class="stat-card stat-blue">
        <div class="stat-icon">
          <el-icon><Box /></el-icon>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ totalPackages }}</span>
          <span class="stat-label">总包数</span>
        </div>
      </div>
    </el-col>
    <el-col :xs="12" :sm="6">
      <div class="stat-card stat-green">
        <div class="stat-icon">
          <el-icon><Download /></el-icon>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ formatNumber(totalDownloads) }}</span>
          <span class="stat-label">总下载</span>
        </div>
      </div>
    </el-col>
    <el-col :xs="12" :sm="6">
      <div class="stat-card stat-orange">
        <div class="stat-icon">
          <el-icon><Files /></el-icon>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ activeRepos }}</span>
          <span class="stat-label">活跃仓库</span>
        </div>
      </div>
    </el-col>
    <el-col :xs="12" :sm="6">
      <div class="stat-card stat-purple">
        <div class="stat-icon">
          <el-icon><Cpu /></el-icon>
        </div>
        <div class="stat-content">
          <span class="stat-value">{{ cacheHitRate }}%</span>
          <span class="stat-label">缓存命中率</span>
        </div>
      </div>
    </el-col>
  </el-row>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Box, Download, Files, Cpu } from '@element-plus/icons-vue'
import type { DashboardStats } from '@/api/dashboard'

const props = defineProps<{
  stats: DashboardStats
}>()

const totalPackages = computed(() => {
  return props.stats.repositories.reduce((sum, repo) => sum + repo.package_count, 0)
})

const totalDownloads = computed(() => {
  return props.stats.downloads_last_7_days.reduce((sum, count) => sum + count, 0)
})

const activeRepos = computed(() => {
  return props.stats.repositories.length
})

const cacheHitRate = computed(() => {
  return props.stats.cache.hit_rate.toFixed(1)
})

function formatNumber(num: number) {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}
</script>

<style scoped>
.stat-cards {
  margin-bottom: 24px;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  transition: all 0.3s ease;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  color: #fff;
  flex-shrink: 0;
}

.stat-blue .stat-icon {
  background: linear-gradient(135deg, #409eff 0%, #337ecc 100%);
}

.stat-green .stat-icon {
  background: linear-gradient(135deg, #67c23a 0%, #529b2e 100%);
}

.stat-orange .stat-icon {
  background: linear-gradient(135deg, #e6a23c 0%, #cf9236 100%);
}

.stat-purple .stat-icon {
  background: linear-gradient(135deg, #909399 0%, #7a7d83 100%);
}

.stat-content {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: #303133;
  line-height: 1.2;
}

.stat-label {
  font-size: 13px;
  color: #909399;
}
</style>
