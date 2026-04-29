<template>
  <div class="cache-management">
    <div class="page-header">
      <h2>缓存管理</h2>
      <div>
        <el-button type="warning" @click="handleClearCache" :loading="clearing">
          清空缓存
        </el-button>
      </div>
    </div>

    <el-row :gutter="20">
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-number">{{ stats.total_entries }}</div>
          <div class="stat-label">缓存条目数</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-number">{{ formatSize(stats.total_size) }}</div>
          <div class="stat-label">缓存大小</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-number">{{ stats.expired_entries }}</div>
          <div class="stat-label">过期条目</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-number">{{ stats.max_size_gb }} GB</div>
          <div class="stat-label">最大容量</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="invalidate-card">
      <template #header>
        <div class="card-header">
          <span>缓存失效</span>
        </div>
      </template>
      <el-form :inline="true" :model="invalidateForm">
        <el-form-item label="匹配模式">
          <el-input
            v-model="invalidateForm.pattern"
            placeholder="proxy:*:lodash:*"
            @keyup.enter="handleInvalidate"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleInvalidate">使缓存失效</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { cacheApi } from '@/api/repository'

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

/** 格式化字节数显示 */
const formatSize = (bytes: number) => {
  if (!bytes || bytes < 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

/** 加载缓存统计数据 */
const loadStats = async () => {
  loading.value = true
  try {
    const res = await cacheApi.getStats()
    stats.value = res.data || {}
  } catch {
    ElMessage.error('加载缓存统计失败')
  } finally {
    loading.value = false
  }
}

/** 清空所有缓存 */
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
    // 用户取消操作不提示错误
    if (err !== 'cancel' && err !== 'Error: cancel') {
      ElMessage.error('清空缓存失败')
    }
  } finally {
    clearing.value = false
  }
}

/** 使指定模式的缓存失效 */
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
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
}

.stat-card {
  text-align: center;
}

.stat-card .stat-number {
  font-size: 28px;
  font-weight: bold;
  color: #409eff;
}

.stat-card .stat-label {
  font-size: 14px;
  color: #909399;
  margin-top: 8px;
}

.invalidate-card {
  margin-top: 20px;
}

.card-header {
  font-size: 16px;
  font-weight: 600;
}
</style>
