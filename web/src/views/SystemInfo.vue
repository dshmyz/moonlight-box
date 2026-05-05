<template>
  <div class="system-info">
    <div class="page-header">
      <h2>系统信息</h2>
      <CustomButton @click="loadSystemInfo" :loading="loading">
        <el-icon><Refresh /></el-icon> 刷新
      </CustomButton>
    </div>

    <div class="info-grid">
      <CustomCard title="版本信息">
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
      </CustomCard>

      <CustomCard title="运行环境">
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
      </CustomCard>

      <CustomCard title="资源使用">
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
      </CustomCard>

      <CustomCard title="系统状态">
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
      </CustomCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh, CircleCheck } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { systemApi, type SystemInfo } from '@/api/system'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomCard from '@/components/ui/CustomCard.vue'

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
  if (percentage < 50) return 'var(--color-success)'
  if (percentage < 80) return 'var(--color-warning)'
  return 'var(--color-danger)'
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
  padding: var(--spacing-xl);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  margin: 0;
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-xl);
}

.commit-hash {
  font-family: monospace;
  background: var(--color-bg-hover);
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
}

.resource-item {
  margin-bottom: var(--spacing-xl);
}

.resource-item:last-child {
  margin-bottom: 0;
}

.resource-label {
  font-size: var(--font-size-base);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-sm);
}

.resource-value {
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-bold);
  color: var(--color-primary);
}

.status-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
}

.status-item:last-child {
  margin-bottom: 0;
}

.status-icon {
  font-size: var(--font-size-lg);
}

.status-icon.success {
  color: var(--color-success);
}
</style>
