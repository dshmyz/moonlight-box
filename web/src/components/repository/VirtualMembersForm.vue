<template>
  <el-alert
    title="成员仓库将按照从上到下的顺序进行优先级排序"
    type="info"
    :closable="false"
    show-icon
    style="margin-bottom: 16px"
  />

  <div class="members-list">
    <div v-for="(member, index) in members" :key="index" class="member-item">
      <span class="member-index">{{ index + 1 }}</span>
      <el-select
        v-model="member.name"
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
  </div>
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
import { Plus, Delete } from '@element-plus/icons-vue'
import { repositoryApi, type Repository } from '@/api/repository'

interface Member {
  name: string
}

interface Props {
  membersText: string
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
    availableRepos.value = res || []
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
}

.member-index {
  min-width: 20px;
  text-align: center;
  color: #909399;
  font-size: 13px;
}
</style>
