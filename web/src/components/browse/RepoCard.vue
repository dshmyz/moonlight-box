<template>
  <div
    class="repo-card"
    :class="{ 'repo-disabled': !repo.enabled }"
  >
    <div class="card-inner">
      <div class="repo-card-header">
        <span class="repo-card-name">{{ repo.display_name || repo.name }}</span>
        <span class="repo-type-badge" :class="`badge-${repo.type}`">{{ typeLabel }}</span>
      </div>
      <p v-if="repo.description" class="repo-card-desc">{{ repo.description }}</p>
      <div class="repo-card-config">
        <code class="config-cmd" @click="$emit('copy', configCommand)">{{ configCommand }}</code>
        <el-button size="small" text class="copy-btn" @click="$emit('copy', configCommand)">
          <el-icon><CopyDocument /></el-icon>
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CopyDocument } from '@element-plus/icons-vue'
import type { PublicRepoListItem } from '@/api/public'

const props = defineProps<{
  repo: PublicRepoListItem
  configCommand: string
}>()

defineEmits<{
  'copy': [text: string]
}>()

const typeLabel = computed(() => {
  const map: Record<string, string> = { local: '本地', proxy: '代理', virtual: '虚拟' }
  return map[props.repo.type] || props.repo.type
})
</script>

<style scoped>
.repo-card {
  position: relative;
  border-radius: 10px;
  overflow: hidden;
  transition: all 0.3s ease;
}

.card-inner {
  padding: 16px 20px;
  background: var(--lunar-bg-card);
  border: 1px solid var(--lunar-border);
  border-radius: 10px;
  transition: border-color 0.3s ease, box-shadow 0.3s ease;
}

.repo-card:hover .card-inner {
  border-color: var(--lunar-border-hover);
  box-shadow: var(--lunar-shadow-glow);
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
  color: var(--lunar-silver);
  letter-spacing: -0.2px;
}

.repo-type-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 700;
  border: 1px solid;
  letter-spacing: 0.5px;
}

.badge-local {
  background: rgba(196, 181, 253, 0.15);
  color: var(--lunar-accent);
  border-color: rgba(196, 181, 253, 0.3);
}

.badge-proxy {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
  border-color: rgba(16, 185, 129, 0.3);
}

.badge-virtual {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
  border-color: rgba(245, 158, 11, 0.3);
}

.repo-card-desc {
  font-size: 13px;
  color: var(--lunar-silver-muted);
  margin: 0 0 10px;
  line-height: 1.4;
}

.repo-card-config {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--lunar-bg-glass);
  border: 1px solid var(--lunar-border);
  border-radius: 6px;
}

.config-cmd {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 12px;
  color: var(--lunar-accent);
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

.copy-btn {
  color: var(--lunar-silver-muted);
}

.copy-btn:hover {
  color: var(--lunar-accent);
}
</style>