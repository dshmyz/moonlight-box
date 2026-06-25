import request from './request'

export interface DownloadLog {
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
  request_id: string
  error_message: string
  created_at: string
}

export interface DownloadStats {
  total_downloads: number
  success_count: number
  failed_count: number
  cached_count: number
  total_bytes: number
}

export interface DownloadLogListResponse {
  items: DownloadLog[]
  pagination: {
    total: number
    page: number
    page_size: number
  }
}

export interface LogCleanupConfig {
  enabled: boolean
  retention_days: number
  interval: string
}

export const downloadLogApi = {
  list(params?: { page?: number; page_size?: number; repository_id?: number; package_type?: string; status?: string; start_date?: string; end_date?: string }) {
    return request.get<DownloadLogListResponse>('/download-logs/logs', { params })
  },

  getStats(params?: { repository_id?: number; start_date?: string; end_date?: string }) {
    return request.get<DownloadStats>('/download-logs/stats', { params })
  },

  getCleanupConfig() {
    return request.get<LogCleanupConfig>('/download-logs/cleanup/config')
  },

  updateCleanupConfig(config: LogCleanupConfig) {
    return request.put<LogCleanupConfig>('/download-logs/cleanup/config', config)
  },

  cleanupNow() {
    return request.post<{ message: string }>('/download-logs/cleanup/now')
  },
}
