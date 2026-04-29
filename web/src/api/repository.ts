import request from './request'

export interface FailureCacheRule {
  status_code?: number
  status_code_range?: [number, number]
  ttl_seconds: number
}

export interface Repository {
  id: number
  name: string
  display_name: string
  description: string
  type: 'local' | 'proxy' | 'virtual'
  package_type: string
  enabled: boolean
  remote_url?: string
  auth_type?: string
  auth_config?: string
  proxy_priority?: number
  timeout_seconds?: number
  max_redirects?: number
  insecure_skip_verify?: boolean
  failure_cache_rules?: FailureCacheRule[]
  cache_enabled?: boolean
  cache_ttl_seconds?: number
  cache_negative_ttl?: number
  cache_max_size_gb?: number
  allow_overwrite?: boolean
  allow_delete?: boolean
  created_at: string
  updated_at: string
  members?: RepositoryGroup[]
}

export interface RepositoryGroup {
  id: number
  virtual_repo_id: number
  member_repo_id: number
  priority: number
  member_repo: Repository
}

export const repositoryApi = {
  list(params?: { package_type?: string; type?: string }) {
    return request.get<{ list: Repository[] }>('/repositories', { params })
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
    return request.get<RepositoryGroup[]>(`/repositories/${name}/members`)
  },

  addMember(name: string, data: { member_name: string; priority: number }) {
    return request.post(`/repositories/${name}/members`, data)
  },

  removeMember(name: string, memberName: string) {
    return request.delete(`/repositories/${name}/members/${memberName}`)
  },
}

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
