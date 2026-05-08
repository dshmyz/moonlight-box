import request from './request'

export interface VulnRule {
  id?: number
  package_pattern: string
  package_type: string
  max_version: string
  min_version: string
  cve: string
  severity: string
  cvss: number
  title: string
  description: string
  fixed_version: string
  references: string
  source?: string
  enabled: boolean
  external_id?: string
  synced_at?: string
  created_at?: string
  updated_at?: string
}

export interface VulnDataSource {
  id?: number
  name: string
  type: string
  url: string
  auth_type: string
  auth_token?: string
  enabled: boolean
  sync_cron: string
  last_sync_at?: string
  last_status?: string
  last_error?: string
  created_at?: string
  updated_at?: string
}

export const vulnRuleApi = {
  listRules(params?: { page?: number; page_size?: number; source?: string; severity?: string; pkg_type?: string; keyword?: string }) {
    return request.get<{ items: VulnRule[]; pagination: { total: number; page: number; page_size: number } }>('/security/vuln-rules', { params })
  },

  getRule(id: number) {
    return request.get<VulnRule>(`/security/vuln-rules/${id}`)
  },

  createRule(rule: VulnRule) {
    return request.post<VulnRule>('/security/vuln-rules', rule)
  },

  updateRule(id: number, rule: Partial<VulnRule>) {
    return request.put(`/security/vuln-rules/${id}`, rule)
  },

  deleteRule(id: number) {
    return request.delete(`/security/vuln-rules/${id}`)
  },

  importRules(rules: VulnRule[]) {
    return request.post<{ count: number }>('/security/vuln-rules/import', rules)
  },

  listDataSources() {
    return request.get<VulnDataSource[]>('/security/vuln-sources')
  },

  createDataSource(ds: VulnDataSource) {
    return request.post<VulnDataSource>('/security/vuln-sources', ds)
  },

  updateDataSource(id: number, ds: Partial<VulnDataSource>) {
    return request.put(`/security/vuln-sources/${id}`, ds)
  },

  deleteDataSource(id: number) {
    return request.delete(`/security/vuln-sources/${id}`)
  },

  syncDataSource(id: number) {
    return request.post(`/security/vuln-sources/${id}/sync`)
  },

  syncAllDataSources() {
    return request.post('/security/vuln-sources/sync-all')
  },

  testDataSource(ds: VulnDataSource) {
    return request.post('/security/vuln-sources/test', ds)
  },
}
