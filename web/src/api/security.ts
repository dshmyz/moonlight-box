import request from './request'

export interface Vulnerability {
  cve_id: string
  severity: string
  cvss_score: number
  title: string
  dependency_name: string
  current_version: string
  fixed_version?: string
  references: string
}

export interface SecurityStats {
  total_scans: number
  critical: number
  high: number
  medium: number
  low: number
}

export interface ScanResult {
  id: number
  version_id: number
  scan_status: string
  total_vulnerabilities: number
  critical_count: number
  high_count: number
  medium_count: number
  low_count: number
  scanned_at: string
}

export interface SecurityDashboard {
  recent_vulnerabilities: ScanResult[]
}

export interface VulnerabilityListResponse {
  items: Vulnerability[]
  pagination: {
    total: number
    page: number
    page_size: number
  }
}

export const securityApi = {
  getStatistics() {
    return request.get<SecurityStats>('/security/statistics')
  },

  listVulnerabilities(params?: { page?: number; page_size?: number; severity?: string; pkg_type?: string }) {
    return request.get<VulnerabilityListResponse>('/security/vulnerabilities', { params })
  },

  getDashboard() {
    return request.get<SecurityDashboard>('/security/dashboard')
  },

  getScanResult(packageId: number) {
    return request.get(`/security/packages/${packageId}/scan`)
  },

  triggerFullScan() {
    return request.post('/security/scan/full')
  },

  triggerScan(packageId: number) {
    return request.post(`/security/packages/${packageId}/scan/trigger`)
  },

  blockByCVE(cve: string) {
    return request.post(`/security/block/${cve}`)
  },
}
