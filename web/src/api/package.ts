import request from './request'

export interface Package {
  id: number
  name: string
  type: string
  description: string
  latest_version?: string
  download_count: number
  updated_at: string
  repository_id?: number
  repository_type?: string
  homepage?: string
  license?: string
  created_by?: number
}

export interface SearchResponse {
  list: Package[]
  total: number
  page: number
  page_size: number
  search_time_ms: number
}

export const packageApi = {
  search(params: { q: string; type?: string; scope?: string; sort?: string; page?: number; page_size?: number }) {
    return request.get<SearchResponse>('/packages/search', { params })
  },
}
