<template>
  <el-drawer
    :model-value="visible"
    title="高级筛选"
    direction="rtl"
    size="380px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <div class="filter-panel">
      <div class="filter-section">
        <label class="filter-label">仓库</label>
        <el-select
          :model-value="repository"
          placeholder="全部仓库"
          clearable
          filterable
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
          @update:model-value="(v: string) => emit('update:version', v)"
        />
      </div>
    </div>

    <template #footer>
      <div class="filter-footer">
        <el-button data-test="reset" @click="onReset">重置</el-button>
        <el-button data-test="apply" type="primary" @click="emit('apply')">应用</el-button>
      </div>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { repositoryApi, type RepositoryWithHealth } from '@/api/repository'

const props = defineProps<{
  visible: boolean
  repository: string
  version: string
}>()

const emit = defineEmits<{
  'update:visible': [v: boolean]
  'update:repository': [v: string]
  'update:version': [v: string]
  apply: []
  reset: []
}>()

const repositories = ref<RepositoryWithHealth[]>([])

async function loadRepositories() {
  try {
    const res = await repositoryApi.list({ page: 1, page_size: 200 })
    // list 返回联合类型：数组或分页对象，兼容两种
    const list = Array.isArray(res) ? res : (res as any).items || []
    repositories.value = list as RepositoryWithHealth[]
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
.filter-panel { display: flex; flex-direction: column; gap: 24px; }
.filter-section { display: flex; flex-direction: column; gap: 8px; }
.filter-label { font-size: 13px; font-weight: 600; color: #475569; }
.filter-footer { display: flex; justify-content: flex-end; gap: 12px; }
</style>
