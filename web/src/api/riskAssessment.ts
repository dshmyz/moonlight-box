import request, { rawRequest } from './request'

export interface RiskHit {
  repo?: string
  format?: string
  version?: string
  dependent_name?: string
  constraint?: string
}

export interface RiskAssessmentItem {
  id: number
  assessment_id: number
  name: string
  version: string
  format?: string
  severity?: string
  reason?: string
  cve?: string
  matched: boolean
  artifact_hit?: RiskHit[]
  dependency_hit?: RiskHit[]
  suggestion?: string
  disposition?: '' | 'removed' | 'blocked' | 'ignored'
  disposition_note?: string
  disposed_by?: number
  disposed_at?: string | null
  created_at: string
}

export interface RiskAssessment {
  id: number
  file_name: string
  total_items: number
  matched_items: number
  artifact_hits: number
  dependency_hits: number
  created_by?: number
  created_at: string
  items?: RiskAssessmentItem[]
}

export interface RiskComponent {
  name: string
  version: string
  format?: string
  severity?: string
  reason?: string
  cve?: string
}

export const riskAssessmentApi = {
  upload(file: File) {
    const formData = new FormData()
    formData.append('file', file)
    return request.post<RiskAssessment>('/security/risk-assessments/upload', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 120000,
    })
  },

  aiParse(file: File) {
    const formData = new FormData()
    formData.append('file', file)
    return request.post<{ file_name: string; components: RiskComponent[]; count: number }>(
      '/security/risk-assessments/ai-parse',
      formData,
      {
        headers: { 'Content-Type': 'multipart/form-data' },
        timeout: 120000,
      },
    )
  },

  analyze(fileName: string, components: RiskComponent[]) {
    return request.post<RiskAssessment>('/security/risk-assessments/analyze', { file_name: fileName, components })
  },

  aiReport(id: number) {
    return request.post<{ report: string }>(`/security/risk-assessments/${id}/ai-report`, {}, { timeout: 120000 })
  },

  createBlockRules(id: number, itemIds: number[]) {
    return request.post<{ count: number }>(`/security/risk-assessments/${id}/block-rules`, { item_ids: itemIds })
  },

  dispose(itemId: number, action: 'remove' | 'block' | 'ignore' | 'reset', note?: string) {
    return request.post<RiskAssessmentItem>('/security/risk-assessments/dispose', {
      item_id: itemId,
      action,
      note: note || '',
    })
  },

  disposeStats(id: number) {
    return request.get<{ total: number; disposed: number; removed: number; blocked: number; ignored: number }>(
      `/security/risk-assessments/${id}/dispose-stats`,
    )
  },

  list(params?: { page?: number; page_size?: number }) {
    return request.get<{ items: RiskAssessment[]; pagination: { total: number; page: number; page_size: number } }>(
      '/security/risk-assessments',
      { params },
    )
  },

  get(id: number) {
    return request.get<RiskAssessment>(`/security/risk-assessments/${id}`)
  },

  downloadTemplate() {
    return rawRequest.get('/security/risk-assessments/template', { responseType: 'blob' })
  },

  exportFile(id: number) {
    return rawRequest.get(`/security/risk-assessments/${id}/export`, { responseType: 'blob' })
  },

  remove(id: number) {
    return request.delete(`/security/risk-assessments/${id}`)
  },
}
