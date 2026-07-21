import { describe, expect, it } from 'vitest'
import { getMigratableArtifactRepositories, validateArtifactSelection } from './migrationRepositorySelection'
import type { SourceRepository } from '@/api/migrationV2'

describe('migrationRepositorySelection', () => {
  const repositories: SourceRepository[] = [
    { name: 'npm-hosted', format: 'npm', type: 'hosted' },
    { name: 'maven-proxy', format: 'maven2', type: 'proxy' },
    { name: 'npm-group', format: 'npm', type: 'group' },
    { name: '', format: 'npm', type: 'hosted' },
  ]

  it('returns only named hosted and proxy repositories as artifact candidates', () => {
    expect(getMigratableArtifactRepositories(repositories).map(repo => repo.name)).toEqual(['npm-hosted', 'maven-proxy'])
  })

  it('requires at least one selected artifact repository when artifact migration is enabled', () => {
    expect(validateArtifactSelection(true, [], true)).toBe('请选择至少一个制品仓库')
  })

  it('blocks artifact migration when source repositories failed to load', () => {
    expect(validateArtifactSelection(true, ['npm-hosted'], false)).toBe('源仓库列表加载失败，请重新加载或关闭制品数据')
  })

  it('allows empty selection when artifact migration is disabled', () => {
    expect(validateArtifactSelection(false, [], false)).toBe('')
  })
})
