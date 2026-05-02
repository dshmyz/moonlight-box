<template>
  <div>
    <!-- 悬浮按钮 -->
    <el-button
      class="ai-float-button"
      type="primary"
      circle
      :icon="ChatDotRound"
      @click="visible = true"
    />
    
    <!-- AI助手抽屉 -->
    <el-drawer
      v-model="visible"
      title="AI助手"
      direction="rtl"
      size="500px"
      :before-close="handleClose"
    >
      <ChatWindow />
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ChatDotRound } from '@element-plus/icons-vue'
import ChatWindow from './ChatWindow.vue'

const visible = ref(false)

const handleClose = (done: () => void) => {
  done()
}
</script>

<style scoped>
.ai-float-button {
  position: fixed;
  right: 24px;
  bottom: 24px;
  width: 56px;
  height: 56px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  z-index: 1000;
}
</style>
