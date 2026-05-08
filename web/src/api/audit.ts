import request from './request'

export interface AuditLog {
  id: number
  user_id: number | null
  action: string
  resource_type: string
  resource_id: number | null
  resource_name: string
  ip_address: string
  user_agent: string
  request_id: string
  response_status: number
  details: string
  duration_ms: number
  created_at: string
}

export interface AuditLogListResponse {
  items: AuditLog[]
  pagination: {
    total: number
    page: number
    page_size: number
    total_pages: number
  }
}

export const auditApi = {
  list(params?: { page?: number; page_size?: number; action?: string; ip_address?: string; resource_type?: string }) {
    return request.get<AuditLogListResponse>('/audit/logs', { params })
  },

  get(id: number) {
    return request.get<AuditLog>(`/audit/logs/${id}`)
  },
}
