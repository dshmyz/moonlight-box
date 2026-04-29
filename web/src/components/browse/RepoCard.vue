<template>
  <div
    class="repo-card"
    :class="{ 'repo-disabled': !repo.enabled }"
  >
    <div class="repo-card-header">
      <span class="repo-card-name">{{ repo.display_name || repo.name }}</span>
      <el-tag :type="typeColor" size="small" effect="plain">
        {{ typeLabel }}
      </el-tag>
    </div>
    <p v-if="repo.description" class="repo-card-desc">{{ repo.description }}</p>
    <div class="repo-card-config">
      <code class="config-cmd" @click="$emit('copy', configCommand)">{{ configCommand }}</code>
      <el-button size="small" text @click="$emit('copy', configCommand)">
        <el-icon><CopyDocument /></el-icon>
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CopyDocument } from '@element-plus/icons-vue'
import type { Repository } from '@/api/repository'

const props = defineProps<{
  repo: Repository
  configCommand: string
}>()

defineEmits<{
  'copy': [text: string]
}>()

const typeColor = computed(() => {
  const map: Record<string, string> = { local: '', proxy: 'success', virtual: 'warning' }
  return map[props.repo.type] || 'info'
})

const typeLabel = computed(() => {
  const map: Record<string, string> = { local: '本地', proxy: '代理', virtual: '虚拟' }
  return map[props.repo.type] || props.repo.type
})
</script>

<style scoped>
.repo-card {
  padding: 16px 20px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  transition: all 0.2s ease;
}

.repo-card:hover {
  border-color: #0f172a;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.08);
}

.repo-disabled {
  opacity: 0.45;
}

.repo-card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

.repo-card-name {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
  white-space: nowrap;
  letter-spacing: -0.2px;
}

.repo-card-desc {
  font-size: 13px;
  color: #909399;
  margin: 0 0 10px 0;
  line-height: 1.4;
}

.repo-card-config {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: #fff;
  border-radius: 6px;
  border: 1px solid #ebeef5;
}

.config-cmd {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 12px;
  color: #409eff;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
  margin: 0;
}

.config-cmd:hover {
  text-decoration: underline;
}
</style>
