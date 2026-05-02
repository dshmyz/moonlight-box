import request from './request'

export const cacheApi = {
  getStats() {
    return request.get('/cache/stats')
  },

  clear() {
    return request.delete('/cache')
  },

  invalidate(data: { pattern: string }) {
    return request.post('/cache/invalidate', data)
  },
}
