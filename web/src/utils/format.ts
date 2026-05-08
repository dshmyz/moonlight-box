/**
 * 格式化数字（支持 K、M 单位）
 * @param num - 要格式化的数字
 * @returns 格式化后的字符串
 */
export const formatNumber = (num: number | undefined): string => {
  if (num === undefined || num === null) return '0'
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

/**
 * 格式化日期
 * @param date - 日期字符串
 * @returns 格式化后的日期字符串
 */
export const formatDate = (date: string | undefined): string => {
  if (!date) return '-'
  return new Date(date).toLocaleString('zh-CN')
}

/**
 * 格式化相对时间
 * @param timeStr - 时间字符串
 * @returns 相对时间字符串
 */
export const formatRelativeTime = (timeStr: string | undefined): string => {
  if (!timeStr) return '-'

  const date = new Date(timeStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffDays < 0) return date.toLocaleDateString('zh-CN')
  if (diffDays === 0) return 'Today'
  if (diffDays === 1) return 'Yesterday'
  if (diffDays < 30) return `${diffDays}d ago`
  if (diffDays < 365) return `${Math.floor(diffDays / 30)}mo ago`
  return `${Math.floor(diffDays / 365)}y ago`
}

export const formatSize = (bytes: number | undefined): string => {
  if (bytes === undefined || bytes === null) return '-'
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}
