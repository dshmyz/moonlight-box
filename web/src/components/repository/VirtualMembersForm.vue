<template>
  <el-alert
    title="拖拽成员可调整优先级顺序，排在越前面优先级越高"
    type="info"
    :closable="false"
    show-icon
    style="margin-bottom: 16px"
  />

  <draggable
    v-model="members"
    item-key="name"
    handle=".drag-handle"
    animation="200"
    ghost-class="member-ghost"
    @end="handleMemberChange"
  >
    <template #item="{ element, index }">
      <div class="member-item">
        <span class="drag-handle" title="拖拽排序">
          <i class="fa-solid fa-grip-vertical"></i>
        </span>
        <span class="member-index">{{ index + 1 }}</span>
        <el-select
          v-model="element.name"
          placeholder="请选择仓库"
          filterable
          :disabled="loading"
          validate-event="false"
          @change="handleMemberChange"
        >
          <el-option
            v-for="repo in availableRepos"
            :key="repo.name"
            :label="repo.display_name || repo.name"
            :value="repo.name"
          />
        </el-select>
        <el-button
          type="danger"
          :icon="Delete"
          circle
          size="small"
          @click="removeMember(index)"
        />
      </div>
    </template>
  </draggable>

  <el-button
    type="primary"
    @click="addMember"
    :disabled="loading || availableRepos.length === 0"
    style="margin-top: 8px"
  >
    <el-icon><Plus /></el-icon>
    添加成员
  </el-button>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import draggable from 'vuedraggable'
import { Plus, Delete } from '@element-plus/icons-vue'
import { repositoryApi, type Repository } from '@/api/repository'

interface Member {
  name: string
}

interface Props {
  membersText: string
  packageType: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:membersText': [value: string]
}>()

const members = ref<Member[]>([])
const availableRepos = ref<Repository[]>([])
const loading = ref(false)

const loadAvailableRepos = async () => {
  loading.value = true
  try {
    const res = await repositoryApi.list()
    const allRepos: any[] = (res && typeof res === 'object' && 'items' in res)
      ? (res as any).items
      : (res as any[]) || []
    availableRepos.value = allRepos.filter((repo: any) =>
      repo.type !== 'virtual' &&
      (!props.packageType || repo.package_type === props.packageType)
    )
  } catch {
    availableRepos.value = []
  } finally {
    loading.value = false
  }
}

watch(
  () => props.membersText,
  (val) => {
    if (val) {
      members.value = val
        .split('\n')
        .filter(name => name.trim())
        .map(name => ({ name: name.trim() }))
    } else {
      members.value = []
    }
  },
  { immediate: true }
)

watch(
  () => props.packageType,
  () => {
    loadAvailableRepos()
  }
)

const addMember = () => {
  members.value.push({ name: '' })
}

const removeMember = (index: number) => {
  members.value.splice(index, 1)
  handleMemberChange()
}

const handleMemberChange = () => {
  const text = members.value
    .map(m => m.name)
    .filter(Boolean)
    .join('\n')
  emit('update:membersText', text)
}

onMounted(() => {
  loadAvailableRepos()
})
</script>

<style scoped>
.members-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.member-item {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #fff;
  padding: 4px 0;
  border-radius: 4px;
  transition: box-shadow 0.2s;
}

.member-item:hover {
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

.member-ghost {
  opacity: 0.5;
  background: #ecf5ff;
  border-radius: 4px;
}

.drag-handle {
  cursor: grab;
  color: #c0c4cc;
  font-size: 14px;
  padding: 4px;
  flex-shrink: 0;
}

.drag-handle:active {
  cursor: grabbing;
}

.member-index {
  min-width: 20px;
  text-align: center;
  color: #909399;
  font-size: 13px;
  flex-shrink: 0;
}
</style>
