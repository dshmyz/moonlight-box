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

export interface DatabasePoolStats {
  status?: string
  max_open_connections: number
  open_connections: number
  in_use: number
  idle: number
  wait_count: number
  wait_duration_ms: number
  max_idle_closed: number
  max_idle_time_closed: number
  max_lifetime_closed: number
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
  database_pool?: DatabasePoolStats
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
