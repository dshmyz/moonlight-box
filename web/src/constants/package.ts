export const PACKAGE_TYPE_COLORS: Record<string, string> = {
  npm: 'primary',
  maven: 'danger',
  pypi: 'success',
  go: 'warning',
  yum: 'info',
  apt: 'info',
  generic: 'info',
}

export const PACKAGE_TYPE_LABELS: Record<string, string> = {
  npm: 'npm',
  maven: 'Maven',
  pypi: 'PyPI',
  go: 'Go',
  yum: 'Yum',
  apt: 'Apt',
  generic: 'Generic',
}

export const getPackageTypeColor = (type: string): string => {
  return PACKAGE_TYPE_COLORS[normalizePackageType(type)] || 'info'
}

export const getPackageTypeLabel = (type: string): string => {
  return PACKAGE_TYPE_LABELS[normalizePackageType(type)] || type
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

export function normalizePackageType(type: string): string {
	if (type === 'maven2') return 'maven'
	return type
}

export const PACKAGE_TYPE_OPTIONS = [
  { value: 'npm', label: 'npm' },
  { value: 'maven', label: 'Maven' },
  { value: 'pypi', label: 'PyPI' },
  { value: 'go', label: 'Go' },
  { value: 'yum', label: 'Yum' },
  { value: 'apt', label: 'Apt' },
  { value: 'generic', label: 'Generic' },
] as const

export function formatPackageName(name: string, pkgType: string): string {
  if (!name) return ''

  const normalizedType = normalizePackageType(pkgType)
  if (normalizedType === 'maven') {
    const separator = name.includes(':') ? ':' : '/'
    const parts = name.split(separator)
    if (parts.length >= 2) {
      return `${parts[0]}:${parts[1]}`
    }
  }

  return name
}
