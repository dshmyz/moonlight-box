import request from './request'

export type BlockMatchType = 'exact' | 'wildcard' | 'range'

export interface BlockRule {
  id: number
  package_name: string
  version: string
  match_type: BlockMatchType
  package_type: string
  reason: string
  enabled: boolean
  condition_type: string
  condition_op: string
  condition_value: string
  created_by?: number
  created_at: string
  updated_at: string
}

export interface BlockRuleCreateParams {
  package_name: string
  version: string
  match_type: BlockMatchType
  package_type: string
  reason: string
  enabled?: boolean
  condition_type?: string
  condition_op?: string
  condition_value?: string
}

export const blockRuleApi = {
  list(params?: { page?: number; page_size?: number; package_name?: string; package_type?: string; enabled?: string }) {
    return request.get<{ items: BlockRule[]; pagination: { total: number; page: number; page_size: number } }>('/block-rules', { params })
  },

  create(data: BlockRuleCreateParams) {
    return request.post<BlockRule>('/block-rules', data)
  },

  batchImport(data: { rules: BlockRuleCreateParams[] }) {
    return request.post<{ success: number; failed: number; total: number }>('/block-rules/batch-import', data)
  },

  downloadTemplate() {
    return request.get('/block-rules/template', { responseType: 'blob' })
  },

  update(id: number, data: Partial<BlockRule>) {
    return request.put(`/block-rules/${id}`, data)
  },

  delete(id: number) {
    return request.delete(`/block-rules/${id}`)
  },
}
