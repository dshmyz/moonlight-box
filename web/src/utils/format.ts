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

const relativeTimeCache = new Map<string, { time: number; result: string }>()
const RELATIVE_TIME_CACHE_TTL = 60000
const MAX_CACHE_SIZE = 500

function cleanupRelativeTimeCache(): void {
  const now = Date.now()
  for (const [key, val] of relativeTimeCache) {
    if (now - val.time > RELATIVE_TIME_CACHE_TTL) {
      relativeTimeCache.delete(key)
    }
  }
}

/**
 * 格式化相对时间（中文）
 * @param timeStr - 时间字符串
 * @returns 中文相对时间字符串
 */
export const formatRelativeTime = (timeStr: string | undefined): string => {
  if (!timeStr) return '-'

  const now = Date.now()

  // 当缓存过大时清理过期条目
  if (relativeTimeCache.size > MAX_CACHE_SIZE) {
    cleanupRelativeTimeCache()
  }

  const cached = relativeTimeCache.get(timeStr)
  if (cached && now - cached.time < RELATIVE_TIME_CACHE_TTL) {
    return cached.result
  }

  const date = new Date(timeStr)
  const diffMs = now - date.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))
  let result: string

  if (diffDays < 0) result = date.toLocaleDateString('zh-CN')
  else if (diffDays === 0) result = '今天'
  else if (diffDays === 1) result = '昨天'
  else if (diffDays < 30) result = `${diffDays} 天前`
  else if (diffDays < 365) result = `${Math.floor(diffDays / 30)} 个月前`
  else result = `${Math.floor(diffDays / 365)} 年前`

  relativeTimeCache.set(timeStr, { time: now, result })
  return result
}

export const formatSize = (bytes: number | undefined): string => {
  if (bytes === undefined || bytes === null) return '-'
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}
