<template>
  <Transition name="filter-panel">
    <section v-if="visible" class="filter-panel-inline" data-test="filter-panel">
      <div class="filter-row">
        <div class="filter-section">
          <label class="filter-label">仓库</label>
          <el-select
            :model-value="repository"
            placeholder="全部仓库"
            clearable
            filterable
            size="default"
            @update:model-value="(v: string) => emit('update:repository', v || '')"
          >
            <el-option
              v-for="repo in repositories"
              :key="repo.id"
              :label="repo.name"
              :value="repo.name"
            >
              <span>{{ repo.name }}</span>
              <el-tag size="small" type="info">{{ repo.type }}</el-tag>
            </el-option>
          </el-select>
        </div>

        <div class="filter-section">
          <label class="filter-label">版本</label>
          <el-input
            :model-value="version"
            placeholder="支持精确版本号或通配符，如 1.2.*"
            clearable
            size="default"
            @update:model-value="(v: string) => emit('update:version', v)"
          />
        </div>

        <div class="filter-actions">
          <el-button data-test="reset" size="default" @click="onReset">重置</el-button>
          <el-button data-test="apply" type="primary" size="default" @click="emit('apply')">搜索</el-button>
        </div>
      </div>
    </section>
  </Transition>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { repositoryApi, type RepositoryWithHealth } from '@/api/repository'
import { publicRepoApi } from '@/api/public'

const props = defineProps<{
  visible: boolean
  repository: string
  version: string
  mode?: 'admin' | 'public'
}>()

const emit = defineEmits<{
  'update:repository': [v: string]
  'update:version': [v: string]
  apply: []
  reset: []
}>()

const repositories = ref<RepositoryWithHealth[]>([])

async function loadRepositories() {
  try {
    if (props.mode === 'public') {
      // 公共端用免认证接口
      const list = await publicRepoApi.list()
      repositories.value = list as unknown as RepositoryWithHealth[]
    } else {
      const res = await repositoryApi.list({ page: 1, page_size: 200 })
      const list = Array.isArray(res) ? res : (res as any).items || []
      repositories.value = list as RepositoryWithHealth[]
    }
  } catch {
    repositories.value = []
  }
}

watch(() => props.visible, (v) => {
  if (v && repositories.value.length === 0) {
    loadRepositories()
  }
}, { immediate: true })

function onReset() {
  emit('update:repository', '')
  emit('update:version', '')
  emit('reset')
}
</script>

<style scoped>
.filter-panel-inline {
  background: var(--lunar-bg-surface);
  border: 1px solid var(--lunar-border);
  border-radius: var(--radius-md);
  padding: 16px 20px;
  box-shadow: var(--lunar-shadow-card);
  margin-top: 8px;
}
.filter-row {
  display: flex;
  gap: 16px;
  align-items: flex-end;
  flex-wrap: wrap;
}
.filter-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1;
  min-width: 200px;
}
.filter-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--lunar-silver-muted);
}
.filter-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}
.filter-panel-enter-active,
.filter-panel-leave-active {
  transition: all 0.25s ease;
}
.filter-panel-enter-from,
.filter-panel-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
.filter-panel-enter-to,
.filter-panel-leave-from {
  opacity: 1;
  transform: translateY(0);
}
</style>