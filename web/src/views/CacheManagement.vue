<template>
  <div class="cache-management">
    <div class="page-header">
      <h2>缓存管理</h2>
      <div>
        <CustomButton type="secondary" @click="handleClearCache" :loading="clearing">
          清空缓存
        </CustomButton>
      </div>
    </div>

    <div class="stats-grid">
      <CustomCard hoverable class="stat-card">
        <div class="stat-number">{{ stats.total_entries }}</div>
        <div class="stat-label">缓存条目数</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card">
        <div class="stat-number">{{ formatSize(stats.total_size) }}</div>
        <div class="stat-label">缓存大小</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card">
        <div class="stat-number">{{ stats.expired_entries }}</div>
        <div class="stat-label">过期条目</div>
      </CustomCard>
      <CustomCard hoverable class="stat-card">
        <div class="stat-number">{{ stats.max_size_gb }} GB</div>
        <div class="stat-label">最大容量</div>
      </CustomCard>
    </div>

    <CustomCard title="缓存失效" class="invalidate-card">
      <div class="invalidate-form">
        <div class="form-item">
          <label class="form-label">匹配模式</label>
          <CustomInput
            v-model="invalidateForm.pattern"
            placeholder="proxy:*:lodash:*"
            @enter="handleInvalidate"
          />
        </div>
        <CustomButton type="primary" @click="handleInvalidate">使缓存失效</CustomButton>
      </div>
    </CustomCard>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { cacheApi } from '@/api/cache'
import CustomCard from '@/components/ui/CustomCard.vue'
import CustomButton from '@/components/ui/CustomButton.vue'
import CustomInput from '@/components/ui/CustomInput.vue'

const loading = ref(false)
const clearing = ref(false)
interface CacheStats {
  total_entries: number
  total_size: number
  expired_entries: number
  max_size_gb: number
}

const stats = ref<CacheStats>({
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
    stats.value = (res as CacheStats) || { total_entries: 0, total_size: 0, expired_entries: 0, max_size_gb: 0 }
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
  padding: var(--spacing-xl);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
}

.page-header h2 {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-xl);
}

.stat-card {
  text-align: center;
}

.stat-number {
  font-size: var(--font-size-3xl);
  font-weight: var(--font-weight-bold);
  color: var(--color-primary);
}

.stat-label {
  font-size: var(--font-size-base);
  color: var(--color-text-tertiary);
  margin-top: var(--spacing-sm);
}

.invalidate-card {
  margin-top: var(--spacing-xl);
}

.invalidate-form {
  display: flex;
  align-items: flex-end;
  gap: var(--spacing-lg);
}

.form-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.form-label {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-secondary);
}
</style>
