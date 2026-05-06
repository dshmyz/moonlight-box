import request from '@/api/request'

export interface LocalConfig {
  base_path: string
  max_size_gb: number
}

export interface S3Config {
  endpoint: string
  region: string
  access_key_id: string
  secret_access_key: string
  bucket: string
  base_path: string
  max_size_gb: number
  use_ssl: boolean
}

export interface OBSConfig {
  endpoint: string
  access_key_id: string
  secret_access_key: string
  bucket: string
  base_path: string
  max_size_gb: number
}

export interface StorageConfig {
  local: LocalConfig
  s3: S3Config
  obs: OBSConfig
}

export interface StorageBackend {
  id: number
  name: string
  type: string
  description: string
  config: StorageConfig
  is_default: boolean
  status: string
  is_active: boolean
  created_at: string
  updated_at: string
}

export const storageBackendApi = {
  list() {
    return request.get<StorageBackend[]>('/storage-backends')
  },
  getById(id: number) {
    return request.get<StorageBackend>(`/storage-backends/${id}`)
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
