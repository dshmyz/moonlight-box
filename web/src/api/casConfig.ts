import request from '@/api/request'
import type { AuthResponse } from '@/api/auth'

export interface CASConfig {
  enabled: boolean
  server_url: string
  service_url: string
  login_path: string
  validate_path: string
}

export const casAuthApi = {
  getCASConfig() {
    return request.get<{ enabled: boolean; login_url: string }>('/auth/cas/config')
  },
  casCallback(ticket: string) {
    return request.get<AuthResponse>('/auth/cas/callback', { params: { ticket } })
  },
}

export const casConfigApi = {
  getConfig() {
    return request.get<CASConfig>('/cas/config')
  },
  updateConfig(config: CASConfig) {
    return request.put<{ message: string }>('/cas/config', config)
  },
  testConnection() {
    return request.post<{ message: string }>('/cas/config/test')
  },
}
