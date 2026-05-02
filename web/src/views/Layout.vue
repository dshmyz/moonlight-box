<template>
  <el-container class="layout-container">
    <AppSidebar :is-collapsed="isCollapsed" />
    <el-container class="main-container">
      <AppHeader :is-collapsed="isCollapsed" @toggle-collapse="toggleCollapse" />
      <el-main class="app-main">
        <router-view />
      </el-main>
    </el-container>
    
    <!-- AI助手悬浮按钮 -->
    <AIAssistant />
  </el-container>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AppHeader from '@/components/layout/AppHeader.vue'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import AIAssistant from '@/components/ai/AIAssistant.vue'

const isCollapsed = ref(false)

function toggleCollapse() {
  isCollapsed.value = !isCollapsed.value
}
</script>

<style scoped>
.layout-container {
  height: 100vh;
  display: flex;
  flex-direction: row;
}

.main-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #f5f7fa;
}

.app-main {
  padding: 24px;
  overflow-y: auto;
  flex: 1;
}
</style>
