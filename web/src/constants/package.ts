export const PACKAGE_TYPE_COLORS: Record<string, string> = {
  npm: 'primary',
  maven: 'danger',
  pypi: 'success',
  go: 'warning',
  yum: 'info',
  apt: 'info',
  generic: 'info',
}

export const PACKAGE_TYPE_HEX_COLORS: Record<string, string> = {
  npm: '#cb3837',
  maven: '#e65100',
  pypi: '#3775a9',
  go: '#00add8',
  yum: '#2e6da4',
  apt: '#d70a53',
  generic: '#64748b',
}

export const PACKAGE_TYPE_HEX_COLORS_RGB: Record<string, string> = {
  npm: '203, 56, 55',
  maven: '230, 81, 0',
  pypi: '55, 117, 169',
  go: '0, 173, 216',
  yum: '46, 109, 164',
  apt: '215, 10, 83',
  generic: '100, 116, 139',
}

export const getPackageTypeHexColor = (type: string): string => {
  return PACKAGE_TYPE_HEX_COLORS[normalizePackageType(type)] || PACKAGE_TYPE_HEX_COLORS.generic
}

export const getPackageTypeHexColorRGB = (type: string): string => {
  return PACKAGE_TYPE_HEX_COLORS_RGB[normalizePackageType(type)] || PACKAGE_TYPE_HEX_COLORS_RGB.generic
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
