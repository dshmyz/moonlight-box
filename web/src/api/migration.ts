import request from './request'

export interface NexusRepo {
  name: string
  format: string
  type: string
  url: string
}

export interface MigrationTask {
  id: number
  source_type: string
  source_url: string
  status: string
  total_items: number
  processed_items: number
  failed_items: number
  selected_repos: string
  error_message: string
  target_repository_id: number
  target_repository: string
  worker_count: number
  max_retries: number
  batch_size: number
  created_at: string
  updated_at: string
  started_at: string | null
  completed_at: string | null
}

export interface MigrationStatus {
  task: MigrationTask
  processed_items: number
  failed_items: number
  total_items: number
}

export function testNexusConnection(data: {
  url: string
  username: string
  password: string
}) {
  return request.post('/migration/nexus/test', data)
}

export function listNexusRepositories(data: {
  url: string
  username: string
  password: string
}) {
  return request.post('/migration/nexus/repositories', data)
}

export function createMigration(data: {
  url: string
  username: string
  password: string
  selected_repos: string[]
  target_repository_id?: number
  target_repository?: string
  worker_count?: number
  max_retries?: number
  batch_size?: number
}) {
  return request.post('/migration/nexus', data)
}

export function getMigrationStatus(id: number) {
  return request.get<MigrationStatus>(`/migration/${id}/status`)
}

export function cancelMigration(id: number) {
  return request.post(`/migration/${id}/cancel`)
}

export function retryFailedMigration(id: number) {
  return request.post(`/migration/${id}/retry`)
}

export function listMigrations() {
  return request.get<MigrationTask[]>('/migration')
}

export function listMigrationItems(taskId: number, page?: number, pageSize?: number) {
  return request.get('/migration/' + taskId + '/items', {
    params: { page, page_size: pageSize }
  })
}
