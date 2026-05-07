import request from './request'

export interface CacheStats {
  total_items: number
  positive_items: number
  negative_items: number
  total_size: number
  used_bytes: number
  max_bytes: number
  max_items: number
  num_shards: number
  expired_entries: number
  max_size_gb: number
}

export interface CacheListItem {
  key: string
  size: number
  content_type: string
  is_negative: boolean
  expiry: string
  remaining_ttl: number
  is_expired: boolean
}

export interface CacheListResponse {
  items: CacheListItem[]
  total: number
  offset: number
  limit: number
}

export const cacheApi = {
  getStats() {
    return request.get<CacheStats>('/cache/stats')
  },

  list(params: { offset?: number; limit?: number; search?: string }) {
    return request.get<CacheListResponse>('/cache/items', { params })
  },

  deleteItem(key: string) {
    return request.delete(`/cache/items/${encodeURIComponent(key)}`)
  },

  clear() {
    return request.delete('/cache')
  },

  invalidate(data: { pattern: string }) {
    return request.post('/cache/invalidate', data)
  },

  cleanupExpired() {
    return request.delete('/cache/expired')
  },
}
