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
  package_types?: string
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
  metadata_sync_enabled?: boolean
  metadata_sync_interval?: number
  sync_mode?: 'metadata_only' | 'full'
  last_metadata_sync_at?: string
  last_sync_status?: string
}

export interface RepositoryGroup {
  id: number
  virtual_repo_id: number
  member_repo_id: number
  priority: number
  member_repo: Repository
}

export interface SyncConfig {
  metadata_sync_enabled: boolean
  metadata_sync_interval: number
  sync_mode: 'metadata_only' | 'full'
}

export interface SyncTask {
  id: number
  repository_id: number
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  started_at: string
  completed_at?: string
  total_packages: number
  synced_packages: number
  failed_packages: number
  skipped_packages: number
  error_message: string
  trigger_type: 'manual' | 'scheduled'
  triggered_by?: number
}

export const repositoryApi = {
  list(params?: { package_type?: string; type?: string }) {
    return request.get<Repository[]>('/repositories', { params })
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

  // 元数据同步相关 API
  triggerSync(name: string) {
    return request.post<SyncTask>(`/repositories/${name}/sync`)
  },

  getSyncHistory(name: string) {
    return request.get<SyncTask[]>(`/repositories/${name}/sync/history`)
  },

  getSyncTaskStatus(taskId: string) {
    return request.get<SyncTask>(`/sync-tasks/${taskId}`)
  },

  updateSyncConfig(name: string, config: SyncConfig) {
    return request.put(`/repositories/${name}/sync-config`, config)
  },

  cancelSyncTask(taskId: string) {
    return request.post(`/sync-tasks/${taskId}/cancel`)
  },
}
