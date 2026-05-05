import request from './request'

export interface ProxyDownloadLog {
  id: number
  repository_id: number
  repository?: {
    id: number
    name: string
    display_name: string
  }
  package_type: string
  package_name: string
  version: string
  filename: string
  remote_url: string
  status: string
  status_code: number
  size_bytes: number
  duration_ms: number
  from_cache: boolean
  ip_address: string
  user_agent: string
  user_id: number
  error_message: string
  created_at: string
}

export interface ProxyDownloadStats {
  total_downloads: number
  success_count: number
  failed_count: number
  cached_count: number
  total_bytes: number
}

export interface ProxyDownloadLogListResponse {
  items: ProxyDownloadLog[]
  pagination: {
    total: number
    page: number
    page_size: number
  }
}

export const proxyDownloadApi = {
  list(params?: { page?: number; page_size?: number; repository_id?: number; package_type?: string; status?: string; start_date?: string; end_date?: string }) {
    return request.get<ProxyDownloadLogListResponse>('/proxy-downloads/logs', { params })
  },

  getStats(params?: { repository_id?: number; start_date?: string; end_date?: string }) {
    return request.get<ProxyDownloadStats>('/proxy-downloads/stats', { params })
  },
}
