<template>
  <el-aside :width="isCollapsed ? '64px' : '240px'" class="app-sidebar">
    <div class="sidebar-logo">
      <div class="logo-icon">
        <svg viewBox="0 0 24 24" class="moon-icon" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" fill="currentColor"/>
        </svg>
      </div>
      <span v-if="!isCollapsed" class="logo-text">Moonlight</span>
    </div>

    <div class="sidebar-content">
      <!-- 仪表盘 -->
      <div class="menu-group">
        <div v-if="!isCollapsed" class="menu-group-title">仪表盘</div>
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
        </el-menu>
      </div>

      <!-- 制品管理 -->
      <div class="menu-group">
        <div v-if="!isCollapsed" class="menu-group-title">制品管理</div>
        <el-menu
          :default-active="activeMenu"
          :collapse="isCollapsed"
          router
          class="sidebar-menu"
        >
          <el-menu-item index="/admin/packages">
            <el-icon><Box /></el-icon>
            <template #title>包管理</template>
          </el-menu-item>
          <el-menu-item index="/admin/repositories">
            <el-icon><Files /></el-icon>
            <template #title>仓库管理</template>
          </el-menu-item>
        </el-menu>
      </div>

      <!-- 安全合规 -->
      <div class="menu-group">
        <div v-if="!isCollapsed" class="menu-group-title">安全合规</div>
        <el-menu
          :default-active="activeMenu"
          :collapse="isCollapsed"
          router
          class="sidebar-menu"
        >
          <el-menu-item index="/admin/security">
            <el-icon><WarnTriangleFilled /></el-icon>
            <template #title>安全中心</template>
          </el-menu-item>
          <el-menu-item index="/admin/block-rules">
            <el-icon><Lock /></el-icon>
            <template #title>阻断规则</template>
          </el-menu-item>
        </el-menu>
      </div>

      <!-- 存储与缓存 -->
      <div class="menu-group">
        <div v-if="!isCollapsed" class="menu-group-title">存储与缓存</div>
        <el-menu
          :default-active="activeMenu"
          :collapse="isCollapsed"
          router
          class="sidebar-menu"
        >
          <el-menu-item index="/admin/storage">
            <el-icon><FolderOpened /></el-icon>
            <template #title>存储管理</template>
          </el-menu-item>
          <el-menu-item index="/admin/cache">
            <el-icon><Coin /></el-icon>
            <template #title>缓存管理</template>
          </el-menu-item>
          <el-menu-item index="/admin/files">
            <el-icon><Folder /></el-icon>
            <template #title>文件浏览</template>
          </el-menu-item>
        </el-menu>
      </div>

      <!-- 系统管理 -->
      <div class="menu-group">
        <div v-if="!isCollapsed" class="menu-group-title">系统管理</div>
        <el-menu
          :default-active="activeMenu"
          :collapse="isCollapsed"
          router
          class="sidebar-menu"
        >
          <el-menu-item index="/admin/users">
            <el-icon><User /></el-icon>
            <template #title>用户管理</template>
          </el-menu-item>
          <el-menu-item index="/admin/roles">
            <el-icon><Key /></el-icon>
            <template #title>角色管理</template>
          </el-menu-item>
          <el-menu-item index="/admin/profile">
            <el-icon><UserFilled /></el-icon>
            <template #title>个人设置</template>
          </el-menu-item>
          <el-menu-item index="/admin/audit">
            <el-icon><Document /></el-icon>
            <template #title>审计日志</template>
          </el-menu-item>
          <el-menu-item index="/admin/proxy-downloads">
            <el-icon><Download /></el-icon>
            <template #title>代理下载日志</template>
          </el-menu-item>
          <el-menu-item index="/admin/system-config">
            <el-icon><Tools /></el-icon>
            <template #title>系统配置</template>
          </el-menu-item>
          <el-menu-item index="/admin/system-info">
            <el-icon><InfoFilled /></el-icon>
            <template #title>系统信息</template>
          </el-menu-item>
          <el-menu-item index="/admin/backups">
            <el-icon><Download /></el-icon>
            <template #title>备份管理</template>
          </el-menu-item>
          <el-menu-item index="/admin/webhooks">
            <el-icon><Link /></el-icon>
            <template #title>Webhook 管理</template>
          </el-menu-item>
          <el-menu-item index="/admin/migration">
            <el-icon><Upload /></el-icon>
            <template #title>数据迁移</template>
          </el-menu-item>
          <el-menu-item index="/admin/cas-settings">
            <el-icon><Setting /></el-icon>
            <template #title>CAS 设置</template>
          </el-menu-item>
          <el-menu-item index="/admin/help">
            <el-icon><QuestionFilled /></el-icon>
            <template #title>帮助中心</template>
          </el-menu-item>
        </el-menu>
      </div>
    </div>
  </el-aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Odometer, Box, Files, Coin, Lock, Setting, FolderOpened, WarnTriangleFilled, User, Document, Download, Link, Tools, InfoFilled, Folder, Upload, QuestionFilled, Key, UserFilled } from '@element-plus/icons-vue'

defineProps<{
  isCollapsed: boolean
}>()

const route = useRoute()
const activeMenu = computed(() => route.path)
</script>

<style scoped>
.app-sidebar {
  background: var(--color-bg-sidebar);
  border-right: 1px solid var(--color-border);
  transition: width var(--transition-slow);
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
  padding: 0 var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.logo-icon {
  width: 32px;
  height: 32px;
  background: linear-gradient(135deg, var(--color-primary) 0%, var(--color-primary-dark) 100%);
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-text-inverse);
  font-size: 16px;
  font-weight: var(--font-weight-bold);
  flex-shrink: 0;
}

.moon-icon {
  width: 18px;
  height: 18px;
  color: var(--color-warning-light);
}

.logo-text {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-bold);
  color: var(--color-text-primary);
  white-space: nowrap;
  letter-spacing: -0.3px;
}

.sidebar-content {
  flex: 1;
  overflow-y: auto;
  padding: var(--spacing-md) 0;
}

.menu-group {
  margin-bottom: var(--spacing-xs);
}

.menu-group-title {
  padding: var(--spacing-xs) var(--spacing-xl) 6px;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.sidebar-menu {
  border-right: none;
  background: transparent;
  flex: 1;
  padding-top: var(--spacing-sm);
  overflow-y: auto;
}

.sidebar-menu :deep(.el-menu-item) {
  color: var(--color-text-secondary);
  font-size: var(--font-size-base);
  border-radius: var(--radius-lg);
  margin: 0 var(--spacing-sm);
  padding-left: var(--spacing-xl) !important;
  height: 40px;
  line-height: 40px;
  transition: all var(--transition-base);
}

.sidebar-menu :deep(.el-menu-item:hover) {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.sidebar-menu :deep(.el-menu-item.is-active) {
  background: var(--color-bg-active);
  color: var(--color-primary);
  font-weight: var(--font-weight-medium);
}

.sidebar-menu :deep(.el-menu-item.is-active::before) {
  display: none;
}

.sidebar-menu :deep(.el-menu-item .el-icon) {
  font-size: 16px;
}

.sidebar-menu :deep(.el-sub-menu__title) {
  color: var(--color-text-secondary);
  font-size: var(--font-size-base);
}

.sidebar-menu :deep(.el-sub-menu__title:hover) {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.sidebar-menu :deep(.el-sub-menu .el-menu) {
  background: transparent;
}

.sidebar-menu :deep(.el-sub-menu .el-menu .el-menu-item) {
  padding-left: 52px !important;
  min-width: auto;
}

.sidebar-menu :deep(.el-menu--collapse) {
  padding-top: var(--spacing-sm);
  width: 100%;
}

.sidebar-menu :deep(.el-menu--collapse .el-menu-item) {
  margin: 2px var(--spacing-sm);
  padding: 0 !important;
  justify-content: center;
}

.sidebar-menu :deep(.el-menu--collapse .el-menu-item .el-icon) {
  margin: 0;
}

/* 滚动条样式 */
.sidebar-content::-webkit-scrollbar {
  width: 6px;
}

.sidebar-content::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar-content::-webkit-scrollbar-thumb {
  background: var(--color-border);
  border-radius: 3px;
}

.sidebar-content::-webkit-scrollbar-thumb:hover {
  background: var(--color-border-dark);
}
</style>
