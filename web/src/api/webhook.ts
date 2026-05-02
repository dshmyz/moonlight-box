import request from './request'

export interface Webhook {
  id: number
  name: string
  url: string
  secret?: string
  events: string
  status: string
  created_at: string
  updated_at: string
  repository?: string
  package_type?: string
  last_triggered?: string
  failure_count?: number
  created_by?: number
}

export interface WebhookDelivery {
  id: number
  webhook_id: number
  event: string
  response_code: number
  duration: number
  success: boolean
  error?: string
  payload?: string
  created_at: string
}

export interface WebhookCreateRequest {
  name: string
  url: string
  secret?: string
  events: string[]
  enabled?: boolean
}

export interface WebhookUpdateRequest {
  name?: string
  url?: string
  secret?: string
  events?: string[]
  enabled?: boolean
}

export const webhookApi = {
  list() {
    return request.get<{ items: Webhook[]; pagination: { page: number; page_size: number; total: number; total_pages: number } }>('/webhooks')
  },

  create(data: WebhookCreateRequest) {
    return request.post<Webhook>('/webhooks', data)
  },

  update(id: number, data: WebhookUpdateRequest) {
    return request.put<Webhook>(`/webhooks/${id}`, data)
  },

  delete(id: number) {
    return request.delete(`/webhooks/${id}`)
  },

  test(id: number) {
    return request.post<{ success: boolean; message: string }>(`/webhooks/${id}/test`)
  },

  getDeliveries(id: number) {
    return request.get<{ items: WebhookDelivery[]; pagination: { page: number; page_size: number; total: number; total_pages: number } }>(`/webhooks/${id}/deliveries`)
  },
}
