import request from './request'

export interface SystemConfig {
  key: string
  value: string
  value_type: string
  category: string
  description: string
  is_sensitive: boolean
  updated_by?: number
  updated_at: string
}

export interface ConfigUpdateRequest {
  configs: Array<{
    key: string
    value: string
  }>
}

export interface SystemInfo {
  version: string
  go_version: string
  build_time: string
  git_commit: string
  uptime: number
  os: string
  arch: string
  cpu_count: number
  memory_usage: number
  goroutine_count: number
}

export const systemApi = {
  getConfigs() {
    return request.get<{ list: SystemConfig[] }>('/configs')
  },

  updateConfigs(data: ConfigUpdateRequest) {
    return request.post('/configs', data)
  },

  getSystemInfo() {
    return request.get<SystemInfo>('/system/info')
  },
}
