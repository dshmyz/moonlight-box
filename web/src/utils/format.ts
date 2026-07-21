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
  if (!date || date === '') return '-'
  const d = new Date(date)
  if (isNaN(d.getTime())) return '-' // 检查是否是有效日期
  return d.toLocaleString('zh-CN')
}

export const formatSize = (bytes: number | undefined): string => {
  if (bytes === undefined || bytes === null) return '-'
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}
