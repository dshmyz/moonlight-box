import request from './request'

export interface Package {
  id: number
  name: string
  display_name: string
  type: string
  package_type?: string
  format?: string
  description: string
  latest_version?: string
  download_count: number
  updated_at: string
  repository_id?: number
  repository_type?: string
  repository_name?: string
  repository_group_name?: string
  repositories?: string[]
  group_repositories?: string[]
  homepage?: string
  license?: string
  created_by?: number
  versions_count?: number
}

export interface SearchResponse {
  list: Package[]
  total: number
  page: number
  page_size: number
  search_time_ms: number
}

export type PackageFileType = 'primary' | 'pom' | 'sources' | 'javadoc' | 'metadata' | 'other'

export interface PackageFile {
  id: number
  version_id: number
  filename: string
  file_type: PackageFileType
  storage_path?: string
  path?: string
  remote_path?: string
  size_bytes: number
  checksum_sha256?: string
  checksum_md5?: string
  download_count: number
  download_url?: string
  qualifiers?: Record<string, unknown>
  attributes?: Record<string, unknown>
  metadata?: Record<string, unknown>
}

export interface PackageVersion {
  id: number
  package_id: number
  repository_id?: number
  version: string
  name?: string
  namespace?: string
  identity_key?: string
  status: string
  storage_path: string
  published_at: string
  published_by: number
  license?: string
  metadata?: Record<string, unknown> | string
  attributes?: Record<string, unknown>
  qualifiers?: Record<string, unknown>
  download_count: number
  size_bytes?: number
  checksum_sha256?: string
  checksum_md5?: string
  trigger_ip?: string
  files?: PackageFile[]
  file_count?: number
  dependencies?: Array<{
    id: number
    version_id: number
    dep_name: string
    dep_version_constraint: string
    dep_type: string
    package_type: string
    is_optional: boolean
  }>
  files_downloaded?: boolean
}

export interface VersionListResponse {
  package_name: string
  type: string
  versions: PackageVersion[]
}

type PackageDeleteTarget = Pick<Package, 'id' | 'name'> & Partial<Pick<Package, 'type' | 'package_type' | 'format' | 'repository_id'>>
export interface PackageVersionTarget {
  type: string
  name: string
  version: string
  repository_id?: number
}

function packageVersionTargetPayload(target: PackageVersionTarget, extra?: Record<string, unknown>) {
  return {
    repository_id: target.repository_id,
    name: target.name,
    version: target.version,
    ...extra,
  }
}

export const packageApi = {
  search(params: { q?: string; type?: string; name?: string; version?: string; repository?: string; sort?: string; page?: number; page_size?: number }) {
    return request.get<SearchResponse>('/packages/search', { params })
  },

  getVersions(type: string, name: string) {
    return request.get<VersionListResponse>(`/packages/${type}/versions`, { params: { name } })
  },

  getVersionFiles(type: string, name: string, version: string, repositoryId?: number) {
    return request.get<{ files: PackageFile[] }>(`/packages/${type}/versions/files`, {
      params: { name, version, repository_id: repositoryId },
    })
  },

  deprecateVersion(versionId: number, reason: string) {
    return request.post(`/packages/versions/${versionId}/deprecate`, { reason })
  },

  deprecatePackageVersion(target: PackageVersionTarget, reason: string) {
    return request.post(`/packages/${target.type}/versions/deprecate`, packageVersionTargetPayload(target, { reason }))
  },

  restoreVersion(versionId: number) {
    return request.post(`/packages/versions/${versionId}/restore`)
  },

  restorePackageVersion(target: PackageVersionTarget) {
    return request.post(`/packages/${target.type}/versions/restore`, packageVersionTargetPayload(target))
  },

  yankVersion(versionId: number) {
    return request.post(`/packages/versions/${versionId}/yank`)
  },

  yankPackageVersion(target: PackageVersionTarget) {
    return request.post(`/packages/${target.type}/versions/yank`, packageVersionTargetPayload(target))
  },

  deleteVersion(versionId: number) {
    return request.delete(`/packages/versions/${versionId}`)
  },

  deletePackageVersion(target: PackageVersionTarget) {
    return request.delete(`/packages/${target.type}/versions`, {
      params: {
        repository_id: target.repository_id,
        name: target.name,
        version: target.version,
      },
    })
  },

  deletePackage(pkg: number | PackageDeleteTarget) {
    if (typeof pkg === 'number') {
      return request.delete(`/packages/by-id/${pkg}`)
    }
    return request.delete(`/packages/by-id/${pkg.id || 0}`, {
      params: {
        type: pkg.type || pkg.package_type || pkg.format,
        name: pkg.name,
        repository_id: pkg.repository_id,
      },
    })
  },
}
