import request from './request'

export interface FailureCacheRule {
  status_code?: number
  status_code_range?: [number, number]
  ttl_seconds: number
}

export interface BasicAuth {
  username: string
  password: string
}

export interface BearerAuth {
  token: string
}

export interface APIKeyAuth {
  header_name: string
  key_value: string
  query_param?: string
}

export interface ProxyAuthConfig {
  type: string
  basic?: BasicAuth
  bearer?: BearerAuth
  api_key?: APIKeyAuth
}

export interface RepositoryConfig {
  remote_url?: string
  auth_type?: string
  auth?: ProxyAuthConfig
  proxy_priority?: number
  cache_enabled?: boolean
  cache_ttl_seconds?: number
  cache_max_size_gb?: number
  cache_negative_ttl?: number
  timeout_seconds?: number
  max_redirects?: number
  insecure_skip_verify?: boolean
  failure_cache_rules?: string
}

export interface Repository {
  id: number
  name: string
  display_name: string
  description: string
  type: 'local' | 'proxy' | 'virtual'
  package_type: string
  enabled: boolean
  public_visible?: boolean
  config?: RepositoryConfig
  allow_overwrite?: boolean
  allow_delete?: boolean
  created_at: string
  updated_at: string
  members?: RepositoryMember[] | string[]
  url?: string
  storage_backend_id?: number
}

export interface RepositoryMember {
  id: number
  repository_id: number
  member_id: number
  position: number
  member_repo: Repository
}

export interface HealthStatus {
  is_healthy: boolean
  last_check_time: string
  last_check_error?: string
  /** 响应时间，单位：纳秒（Go time.Duration JSON 序列化） */
  response_time: number
  /** 连续失败次数 */
  consecutive_failures: number
  /** HTTP 状态码 */
  status_code?: number
}

export interface CircuitBreakerStats {
  state: string
  success_count: number
  failure_count: number
  last_failure_time?: string
}

export interface RepositoryHealthInfo {
  health_status: HealthStatus
  circuit_breaker?: CircuitBreakerStats
}

export interface RepositoryWithHealth extends Repository {
  health_info?: RepositoryHealthInfo
}

export interface PaginatedResponse<T> {
  items: T[]
  pagination: {
    page: number
    page_size: number
    total: number
    total_pages: number
  }
}

export const repositoryApi = {
  list(params?: { package_type?: string; type?: string; page?: number; page_size?: number }) {
    return request.get<RepositoryWithHealth[] | PaginatedResponse<RepositoryWithHealth>>('/repositories', { params })
  },

  get(name: string) {
    return request.get<Repository>(`/repositories/${name}`)
  },

  create(data: Partial<Repository>) {
    return request.post<Repository>('/repositories', data)
  },

  update(name: string, data: Partial<Repository>) {
    return request.put(`/repositories/${name}`, data)
  },

  delete(name: string) {
    return request.delete(`/repositories/${name}`)
  },

  getMembers(name: string) {
    return request.get<RepositoryMember[]>(`/repositories/${name}/members`)
  },

  addMember(name: string, data: { member_name: string; position: number }) {
    return request.post(`/repositories/${name}/members`, data)
  },

  removeMember(name: string, memberName: string) {
    return request.delete(`/repositories/${name}/members/${memberName}`)
  },

  // 健康检查相关 API
  getAllHealthStatuses() {
    return request.get<{ total: number; items: RepositoryHealthInfo[] }>('/health/repos')
  },

  getHealthStatus(repoId: number) {
    return request.get<RepositoryHealthInfo>(`/health/repos/${repoId}`)
  },

  resetCircuitBreaker(repoId: number) {
    return request.post(`/health/repos/${repoId}/reset`)
  },
}
