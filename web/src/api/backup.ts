import request from './request'

export interface Backup {
  id: number
  name: string
  description?: string
  size: number
  status: 'pending' | 'creating' | 'completed' | 'failed'
  created_at: string
  created_by?: string
}

export interface BackupCreateRequest {
  name: string
  description?: string
}

export interface BackupConfig {
  enabled: boolean
  interval: string
  time: string
}

export interface BackupConfigUpdateRequest {
  enabled: boolean
  interval: string
  time: string
}

export const backupApi = {
  list() {
    return request.get<{ list: Backup[] }>('/backups')
  },

  create(data: BackupCreateRequest) {
    return request.post<Backup>('/backups', data)
  },

  restore(id: number) {
    return request.post(`/backups/${id}/restore`)
  },

  delete(id: number) {
    return request.delete(`/backups/${id}`)
  },

  getConfig() {
    return request.get<BackupConfig>('/backups/config')
  },

  updateConfig(data: BackupConfigUpdateRequest) {
    return request.put<BackupConfig>('/backups/config', data)
  },
}
