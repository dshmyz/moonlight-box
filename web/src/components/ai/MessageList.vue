<template>
  <div ref="listRef" class="message-list">
    <div v-if="messages.length === 0" class="empty-state">
      <el-empty description="开始与AI助手对话吧" />
    </div>
    <MessageItem
      v-for="(msg, index) in messages"
      :key="index"
      :message="msg"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import MessageItem from './MessageItem.vue'

interface Props {
  messages: Array<any>
}

const props = defineProps<Props>()
const listRef = ref<HTMLElement>()

watch(() => props.messages.length, async () => {
  await nextTick()
  if (listRef.value) {
    listRef.value.scrollTop = listRef.value.scrollHeight
  }
})
</script>

<style scoped>
.message-list {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}

.empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}
</style>
