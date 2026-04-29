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
      <el-input
        v-model="member.name"
        placeholder="输入仓库名称"
        @input="handleMemberChange"
      />
      <el-button
        type="danger"
        :icon="Delete"
        circle
        size="small"
        @click="removeMember(index)"
      />
    </div>
  </div>
  <el-button type="primary" plain @click="addMember" style="margin-top: 8px">
    <el-icon><Plus /></el-icon>
    添加成员
  </el-button>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Plus, Delete } from '@element-plus/icons-vue'

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
