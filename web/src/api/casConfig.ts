import request from '@/api/request'
import type { AuthResponse } from '@/api/auth'

export const casAuthApi = {
  getCASConfig() {
    return request.get<{ enabled: boolean; server_url: string; service_url: string }>('/auth/cas/config')
  },
  casCallback(ticket: string) {
    return request.get<AuthResponse>('/auth/cas/callback', { params: { ticket } })
  },
}
