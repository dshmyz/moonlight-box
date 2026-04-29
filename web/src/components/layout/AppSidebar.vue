<template>
  <el-aside :width="isCollapsed ? '64px' : '220px'" class="app-sidebar">
    <div class="sidebar-logo">
      <div class="logo-icon">
        <el-icon><Box /></el-icon>
      </div>
      <span v-if="!isCollapsed" class="logo-text">Moonlight</span>
    </div>

    <el-menu
      :default-active="activeMenu"
      :collapse="isCollapsed"
      router
      class="sidebar-menu"
    >
      <el-menu-item index="/admin/dashboard">
        <el-icon><Odometer /></el-icon>
        <template #title>仪表盘</template>
      </el-menu-item>

      <el-menu-item index="/admin/packages">
        <el-icon><Box /></el-icon>
        <template #title>包管理</template>
      </el-menu-item>

      <el-menu-item index="/admin/repositories">
        <el-icon><Files /></el-icon>
        <template #title>仓库管理</template>
      </el-menu-item>

      <el-menu-item index="/admin/block-rules">
        <el-icon><Lock /></el-icon>
        <template #title>阻断规则</template>
      </el-menu-item>

      <el-menu-item index="/admin/security">
        <el-icon><WarnTriangleFilled /></el-icon>
        <template #title>安全中心</template>
      </el-menu-item>

      <el-menu-item index="/admin/users">
        <el-icon><User /></el-icon>
        <template #title>用户管理</template>
      </el-menu-item>

      <el-menu-item index="/admin/audit">
        <el-icon><Document /></el-icon>
        <template #title>审计日志</template>
      </el-menu-item>

      <el-menu-item index="/admin/cache">
        <el-icon><Coin /></el-icon>
        <template #title>缓存管理</template>
      </el-menu-item>

      <el-menu-item index="/admin/storage">
        <el-icon><FolderOpened /></el-icon>
        <template #title>存储管理</template>
      </el-menu-item>

      <el-menu-item index="/admin/settings">
        <el-icon><Setting /></el-icon>
        <template #title>系统设置</template>
      </el-menu-item>
    </el-menu>
  </el-aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Odometer, Box, Files, Coin, Lock, Setting, FolderOpened, WarnTriangleFilled, User, Document } from '@element-plus/icons-vue'

defineProps<{
  isCollapsed: boolean
}>()

const route = useRoute()
const activeMenu = computed(() => route.path)
</script>

<style scoped>
.app-sidebar {
  background: #1a1c23;
  transition: width 0.3s ease;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.sidebar-logo {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 0 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.logo-icon {
  width: 28px;
  height: 28px;
  background: linear-gradient(135deg, #409eff 0%, #337ecc 100%);
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 15px;
  flex-shrink: 0;
}

.logo-text {
  font-size: 16px;
  font-weight: 700;
  color: #fff;
  white-space: nowrap;
  letter-spacing: -0.3px;
}

.sidebar-menu {
  border-right: none;
  background: transparent;
  flex: 1;
  padding-top: 8px;
}

.sidebar-menu :deep(.el-menu-item) {
  color: rgba(255, 255, 255, 0.7);
}

.sidebar-menu :deep(.el-menu-item:hover) {
  background: rgba(255, 255, 255, 0.08);
  color: #fff;
}

.sidebar-menu :deep(.el-menu-item.is-active) {
  background: rgba(64, 158, 255, 0.2);
  color: #409eff;
}

.sidebar-menu :deep(.el-menu-item.is-active::before) {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  background: #409eff;
  border-radius: 0 2px 2px 0;
}

.sidebar-menu :deep(.el-menu--collapse) {
  padding-top: 8px;
}
</style>
