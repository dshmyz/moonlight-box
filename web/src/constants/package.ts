/**
 * 包类型颜色映射
 */
export const PACKAGE_TYPE_COLORS: Record<string, string> = {
  npm: 'primary',
  maven: 'danger',
  maven2: 'danger',
  pypi: 'success',
  go: 'warning',
  nuget: 'info',
  yum: 'info',
  apt: 'info',
  generic: 'info',
}

/**
 * 包类型标签映射
 */
export const PACKAGE_TYPE_LABELS: Record<string, string> = {
  npm: 'npm',
  maven: 'Maven',
  maven2: 'Maven',
  pypi: 'PyPI',
  go: 'Go',
  nuget: 'NuGet',
  yum: 'Yum',
  apt: 'Apt',
  generic: 'Generic',
}

/**
 * 获取包类型颜色
 * @param type - 包类型
 * @returns Element Plus Tag 类型
 */
export const getPackageTypeColor = (type: string): string => {
  // 处理 maven 和 maven2 的映射
  const normalizedType = type === 'maven' ? 'maven2' : type
  return PACKAGE_TYPE_COLORS[normalizedType] || 'info'
}

/**
 * 获取包类型标签
 * @param type - 包类型
 * @returns 包类型显示标签
 */
export const getPackageTypeLabel = (type: string): string => {
  const normalizedType = type === 'maven' ? 'maven2' : type
  return PACKAGE_TYPE_LABELS[normalizedType] || type
}

export const VERSION_STATUS_COLORS: Record<string, string> = {
  published: 'success',
  deprecated: 'warning',
  yanked: 'danger',
  draft: 'info',
}

export const VERSION_STATUS_LABELS: Record<string, string> = {
  published: '已发布',
  deprecated: '已弃用',
  yanked: '已撤回',
  draft: '草稿',
}

export const getVersionStatusColor = (status: string): string => {
  return VERSION_STATUS_COLORS[status] || 'info'
}

export const getVersionStatusLabel = (status: string): string => {
  return VERSION_STATUS_LABELS[status] || status
}

export const normalizePackageType = (type: string): string => {
  return type === 'maven' ? 'maven2' : type
}
