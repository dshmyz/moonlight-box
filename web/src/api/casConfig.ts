import request from '@/api/request'

export interface CASConfig {
  enabled: boolean
  server_url: string
  service_url: string
  login_path: string
  validate_path: string
}

export const casConfigApi = {
  getConfig() {
    return request.get('/cas/config')
  },
  updateConfig(config: CASConfig) {
    return request.put('/cas/config', config)
  },
  deleteConfig() {
    return request.delete('/cas/config')
  },
}

export const casAuthApi = {
  getCASConfig() {
    return request.get('/auth/cas/config')
  },
  casCallback(ticket: string) {
    return request.get('/auth/cas/callback', { params: { ticket } })
  },
}
