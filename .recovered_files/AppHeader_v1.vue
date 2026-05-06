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
          <el-avatar :size="32" class="user-avatar">
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
import { ref, computed } from 'vue'
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
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg-card);
  padding: 0 var(--spacing-2xl);
  height: 56px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-lg);
}

.collapse-btn {
  cursor: pointer;
  font-size: 18px;
  color: var(--color-text-secondary);
  transition: color var(--transition-fast);
}

.collapse-btn:hover {
  color: var(--color-text-primary);
}

.header-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-lg);
}

.header-link {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  color: var(--color-text-secondary);
  text-decoration: none;
  font-size: var(--font-size-sm);
  transition: color var(--transition-fast);
}

.header-link:hover {
  color: var(--color-primary);
}

.user-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  cursor: pointer;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-lg);
  transition: background-color var(--transition-fast);
}

.user-info:hover {
  background-color: var(--color-bg-hover);
}

.user-avatar {
  background: linear-gradient(135deg, var(--color-primary) 0%, var(--color-primary-dark) 100%);
  color: var(--color-text-inverse);
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
}

.username {
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.arrow-icon {
  font-size: 12px;
  color: var(--color-text-tertiary);
}

:deep(.el-breadcrumb__inner a) {
  color: var(--color-text-secondary);
  text-decoration: none;
}

:deep(.el-breadcrumb__inner a:hover) {
  color: var(--color-primary);
}

:deep(.el-dropdown-menu__item) {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
</style>
