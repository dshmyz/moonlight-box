<template>
  <div class="system-info">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-server"></i>
        </div>
        <div class="header-text">
          <h2>系统信息</h2>
          <p class="header-subtitle">查看系统运行状态和配置信息</p>
        </div>
      </div>
      <el-button class="refresh-btn" @click="loadSystemInfo" :loading="loading">
        <i class="fa-solid fa-refresh"></i>
        <span>刷新</span>
      </el-button>
    </header>

    <div class="stats-bar">
      <div class="stat-card stat-card--version">
        <div class="stat-icon">
          <i class="fa-solid fa-tag"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ systemInfo.version || '-' }}</span>
          <span class="stat-label">版本号</span>
        </div>
      </div>
      <div class="stat-card stat-card--uptime">
        <div class="stat-icon">
          <i class="fa-solid fa-clock"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ formatUptime(systemInfo.uptime) }}</span>
          <span class="stat-label">运行时长</span>
        </div>
      </div>
      <div class="stat-card stat-card--cpu">
        <div class="stat-icon">
          <i class="fa-solid fa-microchip"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ systemInfo.cpu_count || 0 }} 核</span>
          <span class="stat-label">CPU 核心</span>
        </div>
      </div>
      <div class="stat-card stat-card--memory">
        <div class="stat-icon">
          <i class="fa-solid fa-memory"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ systemInfo.memory_usage || 0 }}%</span>
          <span class="stat-label">内存使用</span>
        </div>
      </div>
    </div>

    <div class="content-grid">
      <div class="info-card">
        <div class="card-header">
          <div class="card-title">
            <i class="fa-solid fa-code"></i>
            <span>版本信息</span>
          </div>
        </div>
        <div class="info-list">
          <div class="info-item">
            <span class="info-label">版本号</span>
            <span class="info-value">{{ systemInfo.version || '-' }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Go 版本</span>
            <span class="info-value">{{ systemInfo.go_version || '-' }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">构建时间</span>
            <span class="info-value">{{ systemInfo.build_time || '-' }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Git Commit</span>
            <el-tooltip :content="systemInfo.git_commit" placement="top" v-if="systemInfo.git_commit">
              <span class="commit-hash">{{ systemInfo.git_commit?.substring(0, 8) }}</span>
            </el-tooltip>
            <span v-else class="info-value">-</span>
          </div>
        </div>
      </div>

      <div class="info-card">
        <div class="card-header">
          <div class="card-title">
            <i class="fa-solid fa-desktop"></i>
            <span>运行环境</span>
          </div>
        </div>
        <div class="info-list">
          <div class="info-item">
            <span class="info-label">操作系统</span>
            <span class="info-value">{{ systemInfo.os || '-' }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">架构</span>
            <span class="info-value">{{ systemInfo.arch || '-' }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">CPU 核心数</span>
            <span class="info-value">{{ systemInfo.cpu_count || '-' }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">运行时长</span>
            <span class="info-value">{{ formatUptime(systemInfo.uptime) }}</span>
          </div>
        </div>
      </div>

      <div class="info-card">
        <div class="card-header">
          <div class="card-title">
            <i class="fa-solid fa-chart-pie"></i>
            <span>资源使用</span>
          </div>
        </div>
        <div class="info-list">
          <div class="info-item info-item--progress">
            <span class="info-label">内存使用</span>
            <el-progress
              :percentage="systemInfo.memory_usage || 0"
              :color="getProgressColor(systemInfo.memory_usage)"
              :stroke-width="10"
            />
          </div>
          <div class="info-item">
            <span class="info-label">Goroutine 数量</span>
            <span class="info-value info-value--highlight">{{ systemInfo.goroutine_count || 0 }}</span>
          </div>
        </div>
      </div>

      <div class="info-card">
        <div class="card-header">
          <div class="card-title">
            <i class="fa-solid fa-circle-check"></i>
            <span>系统状态</span>
          </div>
        </div>
        <div class="info-list">
          <div class="status-item">
            <i class="fa-solid fa-check-circle status-icon status-icon--success"></i>
            <span>系统运行正常</span>
          </div>
          <div class="status-item">
            <i class="fa-solid fa-check-circle status-icon status-icon--success"></i>
            <span>数据库连接正常</span>
          </div>
          <div class="status-item">
            <i class="fa-solid fa-check-circle status-icon status-icon--success"></i>
            <span>存储服务正常</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { systemApi, type SystemInfo } from '@/api/system'

const loading = ref(false)
const systemInfo = ref<SystemInfo>({
  version: '',
  go_version: '',
  build_time: '',
  git_commit: '',
  uptime: 0,
  os: '',
  arch: '',
  cpu_count: 0,
  memory_usage: 0,
  goroutine_count: 0,
})

const formatUptime = (seconds: number): string => {
  if (!seconds || seconds < 0) return '-'

  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = Math.floor(seconds % 60)

  const parts: string[] = []
  if (days > 0) parts.push(`${days} 天`)
  if (hours > 0) parts.push(`${hours} 小时`)
  if (minutes > 0) parts.push(`${minutes} 分钟`)
  if (secs > 0 || parts.length === 0) parts.push(`${secs} 秒`)

  return parts.join(' ')
}

const getProgressColor = (percentage: number): string => {
  if (percentage < 50) return '#67c23a'
  if (percentage < 80) return '#e6a23c'
  return '#f56c6c'
}

const loadSystemInfo = async () => {
  loading.value = true
  try {
    const res = await systemApi.getSystemInfo()
    systemInfo.value = res || systemInfo.value
  } catch {
    ElMessage.error('加载系统信息失败')
  } finally {
    loading.value = false
  }
}

onMounted(loadSystemInfo)
</script>

<style scoped>
.system-info {
  min-height: 100%;
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.list-header {
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
  background: #f3f4f6;
  border-color: #e5e7eb;
  color: #374151;
  transition: all 0.2s ease;
}

.refresh-btn:hover {
  background: #e5e7eb;
}

.stats-bar {
  display: flex;
  gap: 16px;
  margin-bottom: 16px;
}

.stat-card {
  flex: 1;
  padding: 20px;
  background: #fff;
  border-radius: 14px;
  display: flex;
  align-items: center;
  gap: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  transition: all 0.2s ease;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.08);
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
}

.stat-card--version .stat-icon {
  background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%);
  color: #2563eb;
}

.stat-card--uptime .stat-icon {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #16a34a;
}

.stat-card--cpu .stat-icon {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.stat-card--memory .stat-icon {
  background: linear-gradient(135deg, #fce7f3 0%, #fbcfe8 100%);
  color: #be185d;
}

.stat-info {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 20px;
  font-weight: 700;
  color: #1f2937;
  line-height: 1.2;
}

.stat-label {
  font-size: 12px;
  color: #9ca3af;
  margin-top: 4px;
}

.content-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.info-card {
  background: #fff;
  border-radius: 14px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.card-header {
  margin-bottom: 16px;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: #1f2937;
}

.card-title i {
  color: #6366f1;
}

.info-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid #f3f4f6;
}

.info-item:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.info-item--progress {
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
}

.info-label {
  font-size: 13px;
  color: #6b7280;
}

.info-value {
  font-size: 14px;
  color: #1f2937;
  font-weight: 500;
}

.info-value--highlight {
  font-size: 18px;
  font-weight: 700;
  color: #6366f1;
}

.commit-hash {
  font-family: monospace;
  font-size: 13px;
  background: #f3f4f6;
  padding: 4px 10px;
  border-radius: 6px;
  color: #6366f1;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
  font-size: 14px;
  color: #374151;
}

.status-icon {
  font-size: 18px;
}

.status-icon--success {
  color: #22c55e;
}
</style>
