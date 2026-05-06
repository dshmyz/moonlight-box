<template>
  <div class="cache-management">
    <header class="list-header">
      <div class="header-content">
        <div class="header-icon">
          <i class="fa-solid fa-bolt"></i>
        </div>
        <div class="header-text">
          <h2>缓存管理</h2>
          <p class="header-subtitle">管理缓存配置和清理缓存数据</p>
        </div>
      </div>
      <el-button type="danger" class="clear-btn" @click="handleClearCache" :loading="clearing">
        <i class="fa-solid fa-trash"></i>
        <span>清空缓存</span>
      </el-button>
    </header>

    <div class="stats-bar">
      <div class="stat-card stat-card--entries">
        <div class="stat-icon">
          <i class="fa-solid fa-boxes"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.total_entries.toLocaleString() }}</span>
          <span class="stat-label">缓存条目数</span>
        </div>
      </div>
      <div class="stat-card stat-card--size">
        <div class="stat-icon">
          <i class="fa-solid fa-hdd"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ formatSize(stats.total_size) }}</span>
          <span class="stat-label">缓存大小</span>
        </div>
      </div>
      <div class="stat-card stat-card--expired">
        <div class="stat-icon">
          <i class="fa-solid fa-clock"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.expired_entries.toLocaleString() }}</span>
          <span class="stat-label">过期条目</span>
        </div>
      </div>
      <div class="stat-card stat-card--max">
        <div class="stat-icon">
          <i class="fa-solid fa-gauge-high"></i>
        </div>
        <div class="stat-info">
          <span class="stat-value">{{ stats.max_size_gb }} GB</span>
          <span class="stat-label">最大容量</span>
        </div>
      </div>
    </div>

    <div class="content-panel">
      <div class="panel-header">
        <div class="panel-title">
          <i class="fa-solid fa-refresh-cw"></i>
          <span>缓存失效</span>
        </div>
      </div>
      <div class="invalidate-form">
        <div class="input-wrapper">
          <i class="fa-solid fa-search input-icon"></i>
          <el-input
            v-model="invalidateForm.pattern"
            placeholder="输入匹配模式，如: proxy:*:lodash:*"
            @keyup.enter="handleInvalidate"
          />
        </div>
        <el-button type="primary" class="invalidate-btn" @click="handleInvalidate">
          <i class="fa-solid fa-rotate-right"></i>
          <span>使缓存失效</span>
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { cacheApi } from '@/api/cache'

const loading = ref(false)
const clearing = ref(false)
const stats = ref({
  total_entries: 0,
  total_size: 0,
  expired_entries: 0,
  max_size_gb: 0,
})

const invalidateForm = ref({
  pattern: '',
})

const formatSize = (bytes: number) => {
  if (!bytes || bytes < 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

const loadStats = async () => {
  loading.value = true
  try {
    const res = await cacheApi.getStats()
    stats.value = res || {
      total_entries: 0,
      total_size: 0,
      expired_entries: 0,
      max_size_gb: 0,
    }
  } catch {
    ElMessage.error('加载缓存统计失败')
  } finally {
    loading.value = false
  }
}

const handleClearCache = async () => {
  try {
    await ElMessageBox.confirm('确定要清空所有缓存吗?', '警告', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    clearing.value = true
    await cacheApi.clear()
    ElMessage.success('缓存已清空')
    await loadStats()
  } catch (err: unknown) {
    if (err !== 'cancel' && err !== 'Error: cancel') {
      ElMessage.error('清空缓存失败')
    }
  } finally {
    clearing.value = false
  }
}

const handleInvalidate = async () => {
  if (!invalidateForm.value.pattern) {
    ElMessage.warning('请输入匹配模式')
    return
  }
  try {
    await cacheApi.invalidate(invalidateForm.value)
    ElMessage.success('缓存失效操作成功')
    await loadStats()
  } catch {
    ElMessage.error('缓存失效操作失败')
  }
}

onMounted(loadStats)
</script>

<style scoped>
.cache-management {
  /* padding: 20px; */
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
  background: linear-gradient(135deg, #f59e0b 0%, #d97706 100%);
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

.clear-btn {
  height: 40px;
  padding: 0 20px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  border-color: transparent;
  transition: all 0.2s ease;
}

.clear-btn:hover:not(:disabled) {
  background: linear-gradient(135deg, #dc2626 0%, #b91c1c 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.3);
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

.stat-card--entries .stat-icon {
  background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%);
  color: #2563eb;
}

.stat-card--size .stat-icon {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #16a34a;
}

.stat-card--expired .stat-icon {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  color: #d97706;
}

.stat-card--max .stat-icon {
  background: linear-gradient(135deg, #fce7f3 0%, #fbcfe8 100%);
  color: #be185d;
}

.stat-info {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: #1f2937;
  line-height: 1.2;
}

.stat-label {
  font-size: 12px;
  color: #9ca3af;
  margin-top: 4px;
}

.content-panel {
  background: #fff;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.panel-header {
  margin-bottom: 20px;
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #1f2937;
}

.panel-title i {
  color: #2563eb;
}

.invalidate-form {
  display: flex;
  gap: 16px;
  align-items: center;
}

.input-wrapper {
  flex: 1;
  max-width: 500px;
  position: relative;
}

.input-icon {
  position: absolute;
  left: 14px;
  top: 50%;
  transform: translateY(-50%);
  color: #9ca3af;
  font-size: 14px;
}

.input-wrapper :deep(.el-input__wrapper) {
  padding-left: 42px;
  border-radius: 10px;
  border: 1px solid #e5e7eb;
  transition: all 0.2s ease;
}

.input-wrapper :deep(.el-input__wrapper:hover) {
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.input-wrapper :deep(.el-input__wrapper.is-focus) {
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.invalidate-btn {
  height: 40px;
  padding: 0 24px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border-color: transparent;
  transition: all 0.2s ease;
}

.invalidate-btn:hover {
  background: linear-gradient(135deg, #1d4ed8 0%, #1e40af 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(37, 99, 235, 0.3);
}
</style>
