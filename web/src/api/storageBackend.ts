import request from '@/api/request'

export interface StorageBackend {
  id: number
  name: string
  type: string
  description: string
  config: Record<string, any>
  is_default: boolean
  status: string
  is_active: boolean
  created_at: string
  updated_at: string
}

export const storageBackendApi = {
  list() {
    return request.get('/storage-backends')
  },
  getById(id: number) {
    return request.get(`/storage-backends/${id}`)
  },
  create(data: Partial<StorageBackend>) {
    return request.post('/storage-backends', data)
  },
  update(id: number, data: Partial<StorageBackend>) {
    return request.put(`/storage-backends/${id}`, data)
  },
  delete(id: number) {
    return request.delete(`/storage-backends/${id}`)
  },
  setDefault(id: number) {
    return request.post(`/storage-backends/${id}/default`)
  },
  testConnection(data: Partial<StorageBackend>) {
    return request.post('/storage-backends/test', data)
  },
}
