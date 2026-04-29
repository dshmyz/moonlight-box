<template>
  <el-header class="app-header">
    <div class="header-left">
      <el-icon class="collapse-btn" @click="toggleCollapse">
        <Fold v-if="!isCollapsed" />
        <Expand v-else />
      </el-icon>
      <el-breadcrumb separator="/">
        <el-breadcrumb-item
          v-for="item in breadcrumbs"
          :key="item.path"
        >
          <router-link v-if="item.path" :to="item.path">
            {{ item.title }}
          </router-link>
          <span v-else>{{ item.title }}</span>
        </el-breadcrumb-item>
      </el-breadcrumb>
    </div>

    <div class="header-right">
      <el-tooltip content="浏览公共仓库" placement="bottom">
        <router-link to="/" class="header-link">
          <el-icon><View /></el-icon>
          <span>公共仓库</span>
        </router-link>
      </el-tooltip>
      <el-dropdown trigger="click" @command="handleCommand">
        <span class="user-info">
          <el-avatar :size="30" class="user-avatar">
            {{ userInitial }}
          </el-avatar>
          <span class="username">{{ authStore.user?.display_name || authStore.user?.username || '用户' }}</span>
          <el-icon class="arrow-icon"><ArrowDown /></el-icon>
        </span>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="profile">
              <el-icon><User /></el-icon>
              个人中心
            </el-dropdown-item>
            <el-dropdown-item command="logout" divided>
              <el-icon><SwitchButton /></el-icon>
              退出登录
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </el-header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { Fold, Expand, ArrowDown, User, SwitchButton, View } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'

defineProps<{
  isCollapsed: boolean
}>()

const emit = defineEmits<{
  'toggle-collapse': []
}>()

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const breadcrumbs = computed(() => {
  const breadcrumbMap: Record<string, Array<{ title: string; path?: string }>> = {
    Dashboard: [
      { title: '首页', path: '/admin/dashboard' },
      { title: '仪表盘' },
    ],
    AdminPackages: [
      { title: '首页', path: '/admin/dashboard' },
      { title: '包管理' },
    ],
    Repositories: [
      { title: '首页', path: '/admin/dashboard' },
      { title: '仓库管理' },
    ],
    CacheManagement: [
      { title: '首页', path: '/admin/dashboard' },
      { title: '缓存管理' },
    ],
  }

  const routeName = route.name as string
  return breadcrumbMap[routeName] || [
    { title: '首页', path: '/admin/dashboard' },
    { title: route.meta.title as string || '未知页面' },
  ]
})

const userInitial = computed(() => {
  const name = authStore.user?.display_name || authStore.user?.username || 'U'
  return name.charAt(0).toUpperCase()
})

function toggleCollapse() {
  emit('toggle-collapse')
}

async function handleCommand(command: string) {
  switch (command) {
    case 'logout':
      await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      })
      await authStore.logout()
      router.push('/login')
      break
    case 'profile':
      break
  }
}
</script>

<style scoped>
.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #e8e8e8;
  background: #fff;
  padding: 0 20px;
  height: 56px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.collapse-btn {
  cursor: pointer;
  font-size: 18px;
  color: #666;
  transition: color 0.2s;
}

.collapse-btn:hover {
  color: #303133;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-link {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #606266;
  text-decoration: none;
  font-size: 13px;
  transition: color 0.2s;
}

.header-link:hover {
  color: #409eff;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 6px;
  transition: background-color 0.2s;
}

.user-info:hover {
  background-color: #f5f7fa;
}

.user-avatar {
  background: linear-gradient(135deg, #409eff 0%, #337ecc 100%);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
}

.username {
  font-size: 14px;
  color: #303133;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.arrow-icon {
  font-size: 12px;
  color: #909399;
}

:deep(.el-breadcrumb__inner a) {
  color: #606266;
  text-decoration: none;
}

:deep(.el-breadcrumb__inner a:hover) {
  color: #409eff;
}

:deep(.el-dropdown-menu__item) {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
