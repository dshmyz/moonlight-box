import request from './request'

export interface CacheStats {
  total_items?: number
  positive_items?: number
  negative_items?: number
  active_items?: number
  expired_items?: number
  expired_entries?: number
  total_size?: number
  used_bytes?: number
  max_bytes?: number
  max_items?: number
  num_shards?: number
  ttl_seconds?: number
  max_size_gb?: number
}

export interface CacheListItem {
  key: string
  size: number
  content_type: string
  is_negative: boolean
  expiry: string
  remaining_ttl: number
  is_expired: boolean
  source_cache?: string
  cache_type?: string
}

export interface CacheListResponse {
  items: CacheListItem[]
  total: number
  offset: number
  limit: number
}

export interface CleanupExpiredResponse {
  cleaned: number
}

export interface CacheRegistryInfo {
  name: string
  type: string
  description: string
}

export const cacheApi = {
  listCaches() {
    return request.get<CacheRegistryInfo[]>('/cache/caches')
  },

  getStats(name?: string) {
    const url = name ? `/cache/stats/${name}` : '/cache/stats'
    return request.get<CacheStats | Record<string, CacheStats>>(url)
  },

  list(params: { offset?: number; limit?: number; search?: string; cacheName?: string }) {
    const { cacheName, ...rest } = params
    const url = cacheName ? `/cache/caches/${cacheName}/items` : '/cache/items'
    return request.get<CacheListResponse>(url, { params: rest })
  },

  deleteItem(key: string, cacheName?: string) {
    const url = cacheName ? `/cache/caches/${cacheName}/items/${encodeURIComponent(key)}` : `/cache/items/${encodeURIComponent(key)}`
    return request.delete(url)
  },

  clear(name?: string) {
    const url = name ? `/cache/caches/${name}` : '/cache'
    return request.delete(url)
  },

  invalidate(data: { name: string; pattern: string }) {
    return request.post('/cache/invalidate', data)
  },

  cleanupExpired(name?: string) {
    const url = name ? `/cache/caches/${name}/expired` : '/cache/expired'
    return request.delete<CleanupExpiredResponse>(url)
  },
}
