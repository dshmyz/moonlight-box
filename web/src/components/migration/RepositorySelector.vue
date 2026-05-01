<template>
  <div class="repository-selector">
    <h3>选择要迁移的仓库</h3>
    <el-checkbox v-model="selectAll" @change="toggleSelectAll">全选</el-checkbox>
    <el-table :data="repositories" @selection-change="handleSelectionChange">
      <el-table-column type="selection" width="55" />
      <el-table-column prop="name" label="仓库名称" />
      <el-table-column prop="format" label="格式" />
      <el-table-column prop="type" label="类型" />
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { NexusRepo } from '@/api/migration'

const props = defineProps<{
  repositories: NexusRepo[]
}>()

const emit = defineEmits<{
  selected: [repos: string[]]
}>()

const selectAll = ref(false)
const selectedRepos = ref<string[]>([])

function toggleSelectAll() {
  if (selectAll.value) {
    selectedRepos.value = props.repositories.map((r) => r.name)
  } else {
    selectedRepos.value = []
  }
  emit('selected', selectedRepos.value)
}

function handleSelectionChange(rows: NexusRepo[]) {
  selectedRepos.value = rows.map((r) => r.name)
  selectAll.value = rows.length === props.repositories.length
  emit('selected', selectedRepos.value)
}

watch(
  () => props.repositories,
  () => {
    selectedRepos.value = []
    selectAll.value = false
  }
)
</script>
