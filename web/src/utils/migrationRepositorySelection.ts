export interface SourceRepository {
  name: string
  format: string
  type: string
  url?: string
}

export function getMigratableArtifactRepositories(repositories: SourceRepository[]): SourceRepository[] {
  return repositories.filter(repo => repo.name && (repo.type === 'hosted' || repo.type === 'proxy'))
}

export function validateArtifactSelection(artifactsEnabled: boolean, selectedRepositories: string[], repositoriesLoaded: boolean): string {
  if (!artifactsEnabled) return ''
  if (!repositoriesLoaded) return '源仓库列表加载失败，请重新加载或关闭制品数据'
  if (selectedRepositories.length === 0) return '请选择至少一个制品仓库'
  return ''
}
