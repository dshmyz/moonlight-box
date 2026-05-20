import request from './request'

export interface PublicRepoListItem {
  name: string
  display_name: string
  description: string
  type: 'local' | 'proxy' | 'virtual'
  package_type: string
  enabled: boolean
  remote_url?: string
  registry_url: string
}

export interface RepoConfigResponse {
  name: string
  display_name: string
  description: string
  type: 'local' | 'proxy' | 'virtual'
  package_type: string
  enabled: boolean
  remote_url?: string
  registry_url: string
  config_guide: ConfigStep[]
}

export interface ConfigStep {
  title: string
  description: string
  command: string
  language: string
}

export const publicRepoApi = {
  list(params?: { package_type?: string; type?: string }) {
    return request.get<PublicRepoListItem[]>('/public/repositories', { params })
  },

  getRepoConfig(name: string) {
    return request.get<RepoConfigResponse>(`/public/repo/${name}`)
  },
}
