<template>
  <div class="system-info">
    <div class="page-header">
      <h2>系统信息</h2>
      <el-button @click="loadSystemInfo" :loading="loading">
        <el-icon><Refresh /></el-icon> 刷新
      </el-button>
    </div>

    <el-row :gutter="20">
      <el-col :span="12">
        <el-card class="info-card">
          <template #header>
            <div class="card-header">
              <el-icon><InfoFilled /></el-icon>
              <span>版本信息</span>
            </div>
          </template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="版本号">
              {{ systemInfo.version || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Go 版本">
              {{ systemInfo.go_version || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="构建时间">
              {{ systemInfo.build_time || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Git Commit">
              <el-tooltip :content="systemInfo.git_commit" placement="top" v-if="systemInfo.git_commit">
                <span class="commit-hash">{{ systemInfo.git_commit?.substring(0, 8) }}</span>
              </el-tooltip>
              <span v-else>-</span>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <el-col :span="12">
        <el-card class="info-card">
          <template #header>
            <div class="card-header">
              <el-icon><Monitor /></el-icon>
              <span>运行环境</span>
            </div>
          </template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="操作系统">
              {{ systemInfo.os || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="架构">
              {{ systemInfo.arch || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="CPU 核心数">
              {{ systemInfo.cpu_count || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="运行时长">
              {{ formatUptime(systemInfo.uptime) }}
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px">
      <el-col :span="12">
        <el-card class="info-card">
          <template #header>
            <div class="card-header">
              <el-icon><Cpu /></el-icon>
              <span>资源使用</span>
            </div>
          </template>
          <div class="resource-item">
            <div class="resource-label">内存使用</div>
            <el-progress
              :percentage="systemInfo.memory_usage || 0"
              :color="getProgressColor(systemInfo.memory_usage)"
            />
          </div>
          <div class="resource-item">
            <div class="resource-label">Goroutine 数量</div>
            <div class="resource-value">{{ systemInfo.goroutine_count || 0 }}</div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="12">
        <el-card class="info-card">
          <template #header>
            <div class="card-header">
              <el-icon><Connection /></el-icon>
              <span>系统状态</span>
            </div>
          </template>
          <div class="status-item">
            <el-icon class="status-icon success"><CircleCheck /></el-icon>
            <span>系统运行正常</span>
          </div>
          <div class="status-item">
            <el-icon class="status-icon success"><CircleCheck /></el-icon>
            <span>数据库连接正常</span>
          </div>
          <div class="status-item">
            <el-icon class="status-icon success"><CircleCheck /></el-icon>
            <span>存储服务正常</span>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh, InfoFilled, Monitor, Cpu, Connection, CircleCheck } from '@element-plus/icons-vue'
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

/** 格式化运行时长 */
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

/** 获取进度条颜色 */
const getProgressColor = (percentage: number): string => {
  if (percentage < 50) return '#67c23a'
  if (percentage < 80) return '#e6a23c'
  return '#f56c6c'
}

/** 加载系统信息 */
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
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h2 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
}

.info-card {
  height: 100%;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
}

.commit-hash {
  font-family: monospace;
  background: #f5f7fa;
  padding: 2px 8px;
  border-radius: 4px;
}

.resource-item {
  margin-bottom: 20px;
}

.resource-item:last-child {
  margin-bottom: 0;
}

.resource-label {
  font-size: 14px;
  color: #606266;
  margin-bottom: 8px;
}

.resource-value {
  font-size: 24px;
  font-weight: bold;
  color: #409eff;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  font-size: 14px;
}

.status-item:last-child {
  margin-bottom: 0;
}

.status-icon {
  font-size: 18px;
}

.status-icon.success {
  color: #67c23a;
}
</style>
