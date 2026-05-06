import request from './request'

export interface CacheStats {
  total_entries: number
  total_size: number
  expired_entries: number
  max_size_gb: number
}

export const cacheApi = {
  getStats() {
    return request.get<CacheStats>('/cache/stats')
  },

  clear() {
    return request.delete('/cache')
  },

  invalidate(data: { pattern: string }) {
    return request.post('/cache/invalidate', data)
  },
}
