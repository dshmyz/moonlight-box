import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
    requiresAuth?: boolean
    permission?: {
      resource: string
      action: string
    }
  }
}

const routes: RouteRecordRaw[] = [
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
        path: 'browse-new',
        name: 'BrowseNew',
        component: () => import('@/views/PublicBrowsePage.vue'),
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
      {
        path: 'help',
        name: 'PublicHelp',
        component: () => import('@/views/PublicHelp.vue'),
        meta: { title: '帮助中心' },
      },
      {
        path: 'about',
        name: 'About',
        component: () => import('@/views/About.vue'),
        meta: { title: '关于' },
      },
    ],
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { requiresAuth: false },
  },
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
        meta: { title: '包管理', permission: { resource: 'package', action: 'read' } },
      },
      {
        path: 'packages-v2',
        name: 'AdminPackagesV2',
        component: () => import('@/views/PackageCenterV2.vue'),
        meta: { title: '软件包中心', permission: { resource: 'package', action: 'read' } },
      },
      {
        path: 'packages-new',
        name: 'AdminPackagesNew',
        component: () => import('@/views/PackageExplorerPage.vue'),
        meta: { title: '包管理（新）', permission: { resource: 'package', action: 'read' } },
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
        meta: { title: '仓库管理', permission: { resource: 'repositories', action: 'read' } },
      },
      {
        path: 'repositories/:name',
        name: 'RepositoryDetail',
        component: () => import('@/views/RepositoryDetail.vue'),
        meta: { title: '仓库详情', permission: { resource: 'repositories', action: 'read' } },
      },
      {
        path: 'block-rules',
        name: 'BlockRules',
        component: () => import('@/views/BlockRuleList.vue'),
        meta: { title: '阻断规则', permission: { resource: 'block-rules', action: 'read' } },
      },
      {
        path: 'cache',
        name: 'CacheManagement',
        component: () => import('@/views/CacheManagement.vue'),
        meta: { title: '缓存管理', permission: { resource: 'cache', action: 'read' } },
      },
      {
        path: 'storage',
        name: 'StorageManagement',
        component: () => import('@/views/StorageManagement.vue'),
        meta: { title: '存储管理', permission: { resource: 'storage-backends', action: 'read' } },
      },
      {
        path: 'files',
        name: 'FileBrowser',
        component: () => import('@/views/FileBrowser.vue'),
        meta: { title: '文件浏览', permission: { resource: 'system', action: 'admin' } },
      },
      {
        path: 'security',
        name: 'SecurityCenter',
        component: () => import('@/views/SecurityCenter.vue'),
        meta: { title: '安全中心', permission: { resource: 'security', action: 'read' } },
      },
      {
        path: 'vuln-rules',
        name: 'VulnRuleManagement',
        component: () => import('@/views/VulnRuleManagement.vue'),
        meta: { title: '漏洞规则', permission: { resource: 'security', action: 'read' } },
      },
      {
        path: 'users',
        name: 'UserManagement',
        component: () => import('@/views/UserManagement.vue'),
        meta: { title: '用户管理', permission: { resource: 'users', action: 'read' } },
      },
      {
        path: 'audit',
        name: 'AuditLogs',
        component: () => import('@/views/AuditLogs.vue'),
        meta: { title: '审计日志', permission: { resource: 'audit', action: 'read' } },
      },
      {
        path: 'download-logs',
        name: 'DownloadLogs',
        component: () => import('@/views/DownloadLogs.vue'),
        meta: { title: '下载日志', permission: { resource: 'audit', action: 'read' } },
      },
      {
        path: 'roles',
        name: 'RoleManagement',
        component: () => import('@/views/RoleManagement.vue'),
        meta: { title: '角色管理', permission: { resource: 'users', action: 'read' } },
      },
      {
        path: 'profile',
        name: 'ProfileSettings',
        component: () => import('@/views/ProfileSettings.vue'),
        meta: { title: '个人设置' },
      },
      {
        path: 'help',
        name: 'HelpCenter',
        component: () => import('@/views/HelpCenter.vue'),
        meta: { title: '帮助中心' },
      },
      {
        path: 'docs/:doc',
        name: 'DocsViewer',
        component: () => import('@/views/DocsViewer.vue'),
        meta: { title: '文档查看' },
      },
      {
        path: 'backups',
        name: 'BackupManagement',
        component: () => import('@/views/BackupManagement.vue'),
        meta: { title: '备份管理', permission: { resource: 'system', action: 'admin' } },
      },
      {
        path: 'webhooks',
        name: 'WebhookManagement',
        component: () => import('@/views/WebhookManagement.vue'),
        meta: { title: 'Webhook 管理', permission: { resource: 'webhooks', action: 'read' } },
      },
      {
        path: 'migration-v2',
        name: 'MigrationV2',
        component: () => import('@/views/MigrationWizardPage.vue'),
        meta: { title: '数据迁移', permission: { resource: 'system', action: 'admin' } },
      },
      {
        path: 'system-config',
        name: 'SystemConfig',
        component: () => import('@/views/SystemConfig.vue'),
        meta: { title: '系统配置', permission: { resource: 'system', action: 'admin' } },
      },
      {
        path: 'system-info',
        name: 'SystemInfo',
        component: () => import('@/views/SystemInfo.vue'),
        meta: { title: '系统信息', permission: { resource: 'system', action: 'read' } },
      },
      {
        path: 'api-docs',
        name: 'APIDocs',
        component: () => import('@/views/APIDocs.vue'),
        meta: { title: 'API 文档' },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to, _from, next) => {
  const authStore = useAuthStore()
  const token = localStorage.getItem('token')

  if (to.meta.requiresAuth === true && !token) {
    next({ name: 'Login', query: { redirect: to.fullPath } })
    return
  }

  if (to.name === 'Login' && token) {
    next({ name: 'Dashboard' })
    return
  }

  if (to.meta.permission && token) {
    if (!authStore.user) {
      try {
        await authStore.fetchProfile()
      } catch {
        next({ name: 'Login', query: { redirect: to.fullPath } })
        return
      }
    }

    const { resource, action } = to.meta.permission
    if (!authStore.hasPermission(resource, action)) {
      next({ name: 'Dashboard' })
      return
    }
  }

  next()
})

export default router
