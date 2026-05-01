import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  // Public routes (no login required)
  {
    path: '/',
    component: () => import('@/views/PublicLayout.vue'),
    children: [
      {
        path: '',
        name: 'Browse',
        component: () => import('@/views/BrowsePage.vue'),
        meta: { title: '浏览仓库' },
      },
      {
        path: 'packages/:type/:name',
        name: 'PackageDetail',
        component: () => import('@/views/PackageDetail.vue'),
        meta: { title: '包详情' },
      },
      {
        path: 'repo/:name',
        name: 'RepoConfig',
        component: () => import('@/views/RepoConfigPage.vue'),
        meta: { title: '仓库配置' },
      },
    ],
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { requiresAuth: false },
  },
  // Admin routes (login required)
  {
    path: '/admin',
    component: () => import('@/views/Layout.vue'),
    meta: { requiresAuth: true },
    redirect: '/admin/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard.vue'),
        meta: { title: '仪表盘' },
      },
      {
        path: 'packages',
        name: 'AdminPackages',
        component: () => import('@/views/PackageList.vue'),
        meta: { title: '包管理' },
      },
      {
        path: 'packages/:type/:name',
        name: 'AdminPackageDetail',
        component: () => import('@/views/PackageDetail.vue'),
        meta: { title: '包详情' },
      },
      {
        path: 'repositories',
        name: 'Repositories',
        component: () => import('@/views/RepositoryList.vue'),
        meta: { title: '仓库管理' },
      },
      {
        path: 'block-rules',
        name: 'BlockRules',
        component: () => import('@/views/BlockRuleList.vue'),
        meta: { title: '阻断规则' },
      },
      {
        path: 'cache',
        name: 'CacheManagement',
        component: () => import('@/views/CacheManagement.vue'),
        meta: { title: '缓存管理' },
      },
      {
        path: 'storage',
        name: 'StorageManagement',
        component: () => import('@/views/StorageManagement.vue'),
        meta: { title: '存储管理' },
      },
      {
        path: 'files',
        name: 'FileBrowser',
        component: () => import('@/views/FileBrowser.vue'),
        meta: { title: '文件浏览' },
      },
      {
        path: 'cas-settings',
        name: 'CASSettings',
        component: () => import('@/views/CASSettings.vue'),
        meta: { title: 'CAS 设置' },
      },
      {
        path: 'security',
        name: 'SecurityCenter',
        component: () => import('@/views/SecurityCenter.vue'),
        meta: { title: '安全中心' },
      },
      {
        path: 'users',
        name: 'UserManagement',
        component: () => import('@/views/UserManagement.vue'),
        meta: { title: '用户管理' },
      },
      {
        path: 'audit',
        name: 'AuditLogs',
        component: () => import('@/views/AuditLogs.vue'),
        meta: { title: '审计日志' },
      },
      {
        path: 'backups',
        name: 'BackupManagement',
        component: () => import('@/views/BackupManagement.vue'),
        meta: { title: '备份管理' },
      },
      {
        path: 'webhooks',
        name: 'WebhookManagement',
        component: () => import('@/views/WebhookManagement.vue'),
        meta: { title: 'Webhook 管理' },
      },
      {
        path: 'migration',
        name: 'Migration',
        component: () => import('@/views/MigrationPage.vue'),
        meta: { title: '数据迁移' },
      },
      {
        path: 'system-config',
        name: 'SystemConfig',
        component: () => import('@/views/SystemConfig.vue'),
        meta: { title: '系统配置' },
      },
      {
        path: 'system-info',
        name: 'SystemInfo',
        component: () => import('@/views/SystemInfo.vue'),
        meta: { title: '系统信息' },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to, _from, next) => {
  const token = localStorage.getItem('token')

  if (to.meta.requiresAuth === true && !token) {
    next({ name: 'Login', query: { redirect: to.fullPath } })
  } else if (to.name === 'Login' && token) {
    next({ name: 'Dashboard' })
  } else {
    next()
  }
})

export default router
