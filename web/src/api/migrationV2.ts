import request from './request'

// --- Enums ---

export type PlanStatus =
  | 'draft' | 'scanning' | 'prechecking' | 'precheck_failed' | 'ready'
  | 'running' | 'paused' | 'verifying' | 'completed' | 'failed'
  | 'cancelling' | 'cancelled'

export type PlanStage = 'scan' | 'precheck' | 'execute' | 'verify' | 'done'

export type JobKind =
  | 'repo_config' | 'group_membership' | 'permission' | 'role' | 'user'
  | 'artifact_scan' | 'artifact_copy' | 'verify'

export type JobStatus =
  | 'pending' | 'blocked' | 'running' | 'completed' | 'failed' | 'skipped' | 'cancelled'

export type ItemStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped'

export type ConflictSeverity = 'warning' | 'blocking'
export type ConflictKind =
  | 'repo_exists' | 'email_exists' | 'role_exists'
  | 'missing_group_member' | 'source_detail_unavailable'
  | 'target_repository_missing' | 'storage_backend_missing'
  | 'artifact_target_conflict'

export type ConflictPolicy = 'skip' | 'map_existing' | 'rename' | 'use_fallback' | 'create_missing' | 'fail'

export type EventLevel = 'info' | 'warn' | 'error'

// --- DTOs ---

export interface ScopeSelection {
  repo_config: boolean
  hosted_repos: boolean
  proxy_repos: boolean
  group_repos: boolean
  group_memberships: boolean
  privileges: boolean
  roles: boolean
  users: boolean
  user_roles: boolean
  artifacts: boolean
  artifact_repos: string[]
  target_strategy: string
  target_repo_id: number
  target_repo_name: string
}

export interface PlanStats {
  total_repos: number
  synced_repos: number
  skipped_repos: number
  failed_repos: number
  total_users: number
  synced_users: number
  skipped_users: number
  failed_users: number
  total_roles: number
  synced_roles: number
  skipped_roles: number
  total_artifacts: number
  synced_artifacts: number
  skipped_artifacts: number
  failed_artifacts: number
}

export interface MigrationPlan {
  id: number
  name: string
  source_type: string
  source_url: string
  status: PlanStatus
  current_stage: PlanStage
  stats: PlanStats | null
  created_at: string
  updated_at: string
  started_at: string | null
  completed_at: string | null
}

export interface MigrationJob {
  id: number
  plan_id: number
  kind: JobKind
  status: JobStatus
  source_key: string
  target_key: string
  attempt_count: number
  max_attempts: number
  error_code: string
  error_message: string
}

export interface MigrationItem {
  id: number
  plan_id: number
  job_id: number
  kind: string
  source_repository: string
  source_format: string
  source_name: string
  source_version: string
  target_repository: string
  status: ItemStatus
  error_code: string
  error_message: string
}

export interface MigrationConflict {
  id: number
  plan_id: number
  kind: ConflictKind
  severity: ConflictSeverity
  source_key: string
  target_key: string
  message: string
  suggested_policy: ConflictPolicy
  selected_policy: ConflictPolicy | null
  resolved_at: string | null
}

export interface MigrationEvent {
  id: number
  plan_id: number
  job_id: number | null
  item_id: number | null
  level: EventLevel
  event_type: string
  message: string
  created_at: string
}

// --- API ---

export const migrationV2Api = {
  // Source
  testSource(data: { source_type: string; url: string; username: string; password: string }) {
    return request.post('/migration/v2/sources/test', data)
  },

  // Plans
  createPlan(data: {
    name: string
    source_url: string
    username: string
    password: string
    scope: ScopeSelection
  }) {
    return request.post<MigrationPlan>('/migration/v2/plans', data)
  },

  listPlans() {
    return request.get<MigrationPlan[]>('/migration/v2/plans')
  },

  getPlan(id: number) {
    return request.get<MigrationPlan>(`/migration/v2/plans/${id}`)
  },

  deletePlan(id: number) {
    return request.delete(`/migration/v2/plans/${id}`)
  },

  // Stages
  scanPlan(id: number) {
    return request.post(`/migration/v2/plans/${id}/scan`)
  },

  precheckPlan(id: number) {
    return request.post(`/migration/v2/plans/${id}/precheck`)
  },

  applyConflicts(id: number, resolutions: { conflict_id: number; policy: ConflictPolicy }[]) {
    return request.post(`/migration/v2/plans/${id}/conflicts/apply`, { resolutions })
  },

  startPlan(id: number) {
    return request.post(`/migration/v2/plans/${id}/start`)
  },

  pausePlan(id: number) {
    return request.post(`/migration/v2/plans/${id}/pause`)
  },

  resumePlan(id: number) {
    return request.post(`/migration/v2/plans/${id}/resume`)
  },

  cancelPlan(id: number) {
    return request.post(`/migration/v2/plans/${id}/cancel`)
  },

  retryFailed(id: number) {
    return request.post(`/migration/v2/plans/${id}/retry`)
  },

  // Data
  getJobs(planId: number) {
    return request.get<MigrationJob[]>(`/migration/v2/plans/${planId}/jobs`)
  },

  getItems(planId: number, page?: number, pageSize?: number) {
    return request.get(`/migration/v2/plans/${planId}/items`, { params: { page, page_size: pageSize } })
  },

  getConflicts(planId: number) {
    return request.get<MigrationConflict[]>(`/migration/v2/plans/${planId}/conflicts`)
  },

  getEvents(planId: number, limit?: number) {
    return request.get<MigrationEvent[]>(`/migration/v2/plans/${planId}/events`, { params: { limit } })
  },
}
