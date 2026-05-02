import request from './request'

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
  getRepoConfig(name: string) {
    return request.get<RepoConfigResponse>(`/public/repo/${name}`)
  },
}
