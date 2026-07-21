export interface MenuItem {
  index: string
  title: string
  icon: string
  permission?: {
    resource: string
    action: string
  }
  children?: MenuItem[]
}

export const menuConfig: MenuItem[] = [
  {
    index: '/admin/dashboard',
    title: '仪表盘',
    icon: 'fa-solid fa-gauge-high',
  },
  {
    index: 'artifact',
    title: '制品管理',
    icon: 'fa-solid fa-boxes',
    children: [
      {
        index: '/admin/packages',
        title: '包管理',
        icon: 'fa-solid fa-box',
        permission: { resource: 'package', action: 'read' },
      },
      {
        index: '/admin/repositories',
        title: '仓库管理',
        icon: 'fa-solid fa-folder-tree',
        permission: { resource: 'repositories', action: 'read' },
      },
    ],
  },
  {
    index: 'security',
    title: '安全合规',
    icon: 'fa-solid fa-shield-alt',
    children: [
      {
        index: '/admin/security',
        title: '安全中心',
        icon: 'fa-solid fa-shield',
        permission: { resource: 'security', action: 'read' },
      },
      {
        index: '/admin/vuln-rules',
        title: '漏洞规则',
        icon: 'fa-solid fa-list-check',
        permission: { resource: 'security', action: 'read' },
      },
      {
        index: '/admin/block-rules',
        title: '阻断规则',
        icon: 'fa-solid fa-ban',
        permission: { resource: 'block-rules', action: 'read' },
      },
    ],
  },
  {
    index: 'storage',
    title: '存储管理',
    icon: 'fa-solid fa-hard-drive',
    children: [
      {
        index: '/admin/storage',
        title: '存储管理',
        icon: 'fa-solid fa-database',
        permission: { resource: 'storage-backends', action: 'read' },
      },
      {
        index: '/admin/cache',
        title: '缓存管理',
        icon: 'fa-solid fa-memory',
        permission: { resource: 'cache', action: 'read' },
      },
      {
        index: '/admin/files',
        title: '文件浏览',
        icon: 'fa-solid fa-folder-open',
        permission: { resource: 'system', action: 'admin' },
      },
    ],
  },
  {
    index: 'users',
    title: '用户与权限',
    icon: 'fa-solid fa-users',
    children: [
      {
        index: '/admin/users',
        title: '用户管理',
        icon: 'fa-solid fa-user',
        permission: { resource: 'users', action: 'read' },
      },
      {
        index: '/admin/roles',
        title: '角色管理',
        icon: 'fa-solid fa-user-tag',
        permission: { resource: 'users', action: 'read' },
      },
    ],
  },
  {
    index: 'system',
    title: '系统运维',
    icon: 'fa-solid fa-server',
    children: [
      {
        index: '/admin/system-config',
        title: '系统配置',
        icon: 'fa-solid fa-wrench',
        permission: { resource: 'system', action: 'admin' },
      },
      {
        index: '/admin/cas-settings',
        title: 'CAS 设置',
        icon: 'fa-solid fa-key',
        permission: { resource: 'system', action: 'admin' },
      },
      {
        index: '/admin/system-info',
        title: '系统信息',
        icon: 'fa-solid fa-info-circle',
        permission: { resource: 'system', action: 'read' },
      },
      {
        index: '/admin/api-docs',
        title: 'API 文档',
        icon: 'fa-solid fa-book',
      },
      {
        index: '/admin/audit',
        title: '审计日志',
        icon: 'fa-solid fa-file-text',
        permission: { resource: 'audit', action: 'read' },
      },
      {
        index: '/admin/download-logs',
        title: '下载日志',
        icon: 'fa-solid fa-file-lines',
        permission: { resource: 'audit', action: 'read' },
      },
      {
        index: '/admin/backups',
        title: '备份管理',
        icon: 'fa-solid fa-download',
        permission: { resource: 'system', action: 'admin' },
      },
      {
        index: '/admin/webhooks',
        title: 'Webhook 管理',
        icon: 'fa-solid fa-link',
        permission: { resource: 'webhooks', action: 'read' },
      },
      {
        index: '/admin/migration-v2',
        title: '数据迁移',
        icon: 'fa-solid fa-arrow-right-arrow-left',
        permission: { resource: 'system', action: 'admin' },
      },
    ],
  },
]
