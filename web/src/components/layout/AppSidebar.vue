<template>
  <el-aside :width="isCollapsed ? '64px' : '220px'" class="app-sidebar">
    <div class="sidebar-logo">
      <svg class="logo-icon" viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="moonlight-sidebar" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" style="stop-color:#c4b5fd" />
            <stop offset="100%" style="stop-color:#8b5cf6" />
          </linearGradient>
        </defs>
        <circle cx="20" cy="20" r="12" fill="url(#moonlight-sidebar)"/>
        <circle cx="24" cy="16" r="10" fill="#fff"/>
        <circle cx="10" cy="10" r="1.5" fill="#c4b5fd"/>
        <circle cx="32" cy="14" r="1" fill="#c4b5fd" opacity="0.6"/>
        <circle cx="28" cy="30" r="1.2" fill="#c4b5fd" opacity="0.4"/>
      </svg>
      <span v-if="!isCollapsed" class="logo-text">Moonlight</span>
    </div>

    <el-menu
      :default-active="activeMenu"
      :default-openeds="defaultOpenedMenus"
      :collapse="isCollapsed"
      router
      class="sidebar-menu"
    >
      <template v-for="menu in visibleMenus" :key="menu.index">
        <el-menu-item v-if="!menu.children" :index="menu.index">
          <i :class="menu.icon + ' sidebar-icon'"></i>
          <template #title>{{ menu.title }}</template>
        </el-menu-item>

        <el-sub-menu v-else-if="hasVisibleChildren(menu)" :index="menu.index">
          <template #title>
            <i :class="menu.icon + ' sidebar-icon'"></i>
            <span>{{ menu.title }}</span>
          </template>
          <el-menu-item
            v-for="child in filterMenuChildren(menu.children)"
            :key="child.index"
            :index="child.index"
          >
            <i :class="child.icon + ' sidebar-icon'"></i>
            <template #title>{{ child.title }}</template>
          </el-menu-item>
        </el-sub-menu>
      </template>
    </el-menu>
  </el-aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { menuConfig, type MenuItem } from '@/config/menu'

defineProps<{
  isCollapsed: boolean
}>()

const route = useRoute()
const authStore = useAuthStore()
const activeMenu = computed(() => route.path)

function checkMenuPermission(menu: MenuItem): boolean {
  if (!menu.permission) return true
  return authStore.hasPermission(menu.permission.resource, menu.permission.action)
}

function filterMenuChildren(children?: MenuItem[]): MenuItem[] {
  if (!children) return []
  return children.filter(checkMenuPermission)
}

function hasVisibleChildren(menu: MenuItem): boolean {
  return filterMenuChildren(menu.children).length > 0
}

const visibleMenus = computed(() => {
  return menuConfig.filter(menu => {
    if (!menu.children) {
      return checkMenuPermission(menu)
    }
    return hasVisibleChildren(menu)
  })
})

const defaultOpenedMenus = computed(() => {
  return visibleMenus.value
    .filter(menu => menu.children && menu.children.length > 0)
    .map(menu => menu.index)
})
</script>

<style scoped>
.app-sidebar {
  background: linear-gradient(180deg, #1e293b 0%, #0f172a 100%);
  transition: width var(--transition-slow);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  border-right: 1px solid rgba(255, 255, 255, 0.05);
}

.sidebar-logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
  padding: 0 var(--spacing-lg);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  flex-shrink: 0;
  background: rgba(0, 0, 0, 0.1);
}

.logo-icon {
  width: 36px;
  height: 36px;
  flex-shrink: 0;
}

.logo-text {
  font-size: 18px;
  font-weight: 700;
  color: #f1f5f9;
  white-space: nowrap;
  letter-spacing: -0.3px;
}

.sidebar-menu {
  border-right: none;
  background: transparent;
  flex: 1;
  padding: 12px 8px;
  overflow-y: auto;
}

.sidebar-menu::-webkit-scrollbar {
  width: 4px;
}

.sidebar-menu::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar-menu::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
}

.sidebar-menu::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.2);
}

.sidebar-menu :deep(.el-menu-item) {
  color: #94a3b8;
  font-size: 13px;
  font-weight: 500;
  border-radius: 10px;
  margin: 4px 4px;
  padding: 10px 16px !important;
  transition: all 0.2s ease;
  position: relative;
}

.sidebar-menu :deep(.el-menu-item:hover) {
  background: rgba(255, 255, 255, 0.08);
  color: #f1f5f9;
  transform: translateX(4px);
}

.sidebar-menu :deep(.el-menu-item.is-active) {
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.2) 0%, rgba(109, 40, 217, 0.15) 100%);
  color: #c4b5fd;
  font-weight: 600;
  box-shadow: 0 4px 12px rgba(139, 92, 246, 0.2);
}

.sidebar-menu :deep(.el-menu-item.is-active::before) {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  background: linear-gradient(180deg, #a78bfa 0%, #8b5cf6 100%);
  border-radius: 0 2px 2px 0;
}

.sidebar-menu :deep(.el-sub-menu__title) {
  color: #94a3b8;
  font-size: 13px;
  font-weight: 500;
  border-radius: 10px;
  margin: 4px 4px;
  padding: 10px 16px !important;
  transition: all 0.2s ease;
}

.sidebar-menu :deep(.el-sub-menu__title:hover) {
  background: rgba(255, 255, 255, 0.08);
  color: #f1f5f9;
}

.sidebar-menu :deep(.el-sub-menu .el-menu) {
  background: transparent;
  padding: 4px 0;
}

.sidebar-menu :deep(.el-sub-menu .el-menu .el-menu-item) {
  padding-left: 40px !important;
  min-width: auto;
  font-size: 12px;
  border-radius: 8px;
}

.sidebar-menu :deep(.el-sub-menu .el-menu .el-menu-item:hover) {
  transform: translateX(2px);
}

.sidebar-menu :deep(.el-sub-menu .el-menu .el-menu-item.is-active) {
  background: rgba(139, 92, 246, 0.12);
  color: #a78bfa;
}

.sidebar-menu :deep(.el-menu--collapse) {
  padding: 12px 4px;
}

.sidebar-menu :deep(.el-menu--collapse .el-menu-item) {
  padding: 12px !important;
  justify-content: center;
}

.sidebar-menu :deep(.el-menu--collapse .el-sub-menu__title) {
  padding: 12px !important;
  justify-content: center;
}

.sidebar-menu :deep(.el-menu-item i) {
  font-size: 16px;
  margin-right: 10px;
  width: 16px;
  text-align: center;
}

.sidebar-menu :deep(.el-sub-menu__title i) {
  font-size: 16px;
  margin-right: 10px;
  width: 16px;
  text-align: center;
}

.sidebar-icon {
  font-size: 16px;
  margin-right: 10px;
  width: 16px;
  text-align: center;
}
</style>
