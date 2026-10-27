<template>
  <div class="connections">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-network-wired"></i>
        </div>
        <div class="header-text">
          <h2>活跃连接</h2>
          <p class="header-subtitle">实时查看当前请求与回源连接、流量与持续时间</p>
        </div>
      </div>
      <div class="header-actions">
        <el-switch
          v-model="autoRefresh"
          active-text="自动刷新"
          @change="toggleAutoRefresh"
        />
        <el-button class="refresh-btn" @click="loadConnections" :loading="loading">
          <i class="fa-solid fa-refresh"></i>
          <span>刷新</span>
        </el-button>
      </div>
    </header>

    <div class="stats-bar">
      <div class="stat-card stat-card--request">
        <div class="stat-icon">
          <i class="fa-solid fa-arrows-rotate"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.count.request || 0 }}</span>
          <span class="stat-label">入站请求</span>
        </div>
      </div>
      <div class="stat-card stat-card--upstream">
        <div class="stat-icon">
          <i class="fa-solid fa-cloud-arrow-down"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.count.upstream || 0 }}</span>
          <span class="stat-label">回源连接</span>
        </div>
      </div>
    </div>

    <div class="info-card">
      <div class="card-header">
        <div class="card-title">
          <i class="fa-solid fa-list"></i>
          <span>连接列表</span>
        </div>
      </div>
      <el-table :data="stats.entries" style="width: 100%" v-loading="loading" size="small">
        <el-table-column label="类型" width="100">
          <template #default="{ row }">
            <el-tag :type="row.kind === 'upstream' ? 'warning' : 'primary'" size="small">
              {{ row.kind === 'upstream' ? '回源' : '请求' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="remote" label="远端地址" min-width="160" />
        <el-table-column prop="repository" label="仓库" min-width="120" show-overflow-tooltip />
        <el-table-column prop="path" label="路径" min-width="220" show-overflow-tooltip />
        <el-table-column label="持续时间" width="110">
          <template #default="{ row }">
            {{ formatDuration(row.started_at) }}
          </template>
        </el-table-column>
        <el-table-column label="读取" width="100" align="right">
          <template #default="{ row }">
            {{ formatBytes(row.bytes_read) }}
          </template>
        </el-table-column>
        <el-table-column label="写入" width="100" align="right">
          <template #default="{ row }">
            {{ formatBytes(row.bytes_written) }}
          </template>
        </el-table-column>
        <el-table-column label="方法" width="80">
          <template #default="{ row }">
            {{ row.extra?.method || '-' }}
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { systemApi, type ConnectionStats } from '@/api/system'

const loading = ref(false)
const autoRefresh = ref(false)
const stats = ref<ConnectionStats>({ count: {}, entries: [] })
let timer: number | null = null

async function loadConnections() {
  loading.value = true
  try {
    const res = await systemApi.getConnections()
    stats.value = res || { count: {}, entries: [] }
  } finally {
    loading.value = false
  }
}

function toggleAutoRefresh(on: boolean) {
  if (on) {
    timer = window.setInterval(loadConnections, 3000)
    loadConnections()
  } else if (timer) {
    window.clearInterval(timer)
    timer = null
  }
}

function formatDuration(startedAt: string): string {
  const ms = Date.now() - new Date(startedAt).getTime()
  if (ms < 1000) return `${ms}ms`
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m${s % 60}s`
}

function formatBytes(n: number): string {
  if (!n) return '0'
  if (n < 1024) return `${n}B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}KB`
  return `${(n / 1024 / 1024).toFixed(1)}MB`
}

onMounted(loadConnections)
onUnmounted(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<style scoped>
.connections {
  padding: 24px;
}
.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
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
  background: var(--el-color-primary-light-9);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  color: var(--el-color-primary);
}
.header-text h2 {
  margin: 0;
  font-size: 20px;
}
.header-subtitle {
  margin: 4px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.stats-bar {
  display: flex;
  gap: 16px;
  margin-bottom: 24px;
}
.stat-card {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
}
.stat-icon {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
}
.stat-card--request .stat-icon {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}
.stat-card--upstream .stat-icon {
  background: var(--el-color-warning-light-9);
  color: var(--el-color-warning);
}
.stat-value {
  display: block;
  font-size: 22px;
  font-weight: 600;
}
.stat-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.info-card {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  padding: 16px;
}
.card-header {
  margin-bottom: 12px;
}
.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}
</style>
