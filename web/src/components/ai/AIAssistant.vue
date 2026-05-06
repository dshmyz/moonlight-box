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
      :direction="isMaximized ? 'btt' : 'rtl'"
      :size="isMaximized ? '100%' : '500px'"
      :before-close="handleClose"
      class="ai-drawer"
      :class="{ 'maximized': isMaximized }"
      :show-close="false"
    >
      <template #header>
        <div class="drawer-header">
          <div class="header-left">
            <div class="header-icon-wrapper">
              <el-icon :size="20"><ChatDotRound /></el-icon>
            </div>
            <span class="drawer-title">AI助手</span>
          </div>
          <div class="header-actions">
            <el-button 
              class="action-btn" 
              @click="toggleMaximize"
              :title="isMaximized ? '还原' : '最大化'"
              circle
            >
              <i :class="isMaximized ? 'fa-solid fa-compress' : 'fa-solid fa-expand'"></i>
            </el-button>
            <el-button 
              class="action-btn" 
              @click="visible = false"
              title="关闭"
              circle
            >
              <i class="fa-solid fa-xmark"></i>
            </el-button>
          </div>
        </div>
      </template>
      <ChatWindow />
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ChatDotRound } from '@element-plus/icons-vue'
import ChatWindow from './ChatWindow.vue'

const visible = ref(false)
const isMaximized = ref(false)

const handleClose = (done: () => void) => {
  isMaximized.value = false
  done()
}

const toggleMaximize = () => {
  isMaximized.value = !isMaximized.value
}
</script>

<style scoped>
.ai-float-button {
  position: fixed;
  right: 24px;
  bottom: 24px;
  width: 56px;
  height: 56px;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  border: none;
  box-shadow: 0 8px 24px rgba(99, 102, 241, 0.35);
  z-index: 1000;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.ai-float-button:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 32px rgba(99, 102, 241, 0.4);
}

.drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 4px 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-icon-wrapper {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
}

.drawer-title {
  font-size: 16px;
  font-weight: 600;
  color: #0f172a;
  letter-spacing: -0.01em;
}

.header-actions {
  display: flex;
  gap: 4px;
}

.action-btn {
  width: 32px !important;
  height: 32px !important;
  padding: 0 !important;
  background: transparent !important;
  border: none !important;
  color: #64748b !important;
  transition: all 0.2s ease !important;
}

.action-btn:hover {
  background: #f1f5f9 !important;
  color: #0f172a !important;
}

.ai-drawer :deep(.el-drawer__header) {
  margin-bottom: 0;
  padding: 20px 24px;
  border-bottom: 1px solid #e5e7eb;
  background: #fafbfc;
}

.ai-drawer :deep(.el-drawer__body) {
  padding: 0;
  display: flex;
  flex-direction: column;
  background: #ffffff;
}

.ai-drawer :deep(.el-drawer) {
  border-radius: 16px 0 0 16px;
  box-shadow: -8px 0 32px rgba(0, 0, 0, 0.12);
}

.ai-drawer.maximized :deep(.el-drawer) {
  width: 100vw !important;
  height: 100vh !important;
  border-radius: 0;
  box-shadow: none;
}

.ai-drawer.maximized :deep(.el-drawer__header) {
  padding: 20px 32px;
}
</style>
