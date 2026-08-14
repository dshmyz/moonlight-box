import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import PackageInfoSidebar from './PackageInfoSidebar.vue'

vi.mock('@/utils/clipboard', () => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))

const mountIt = (pkg: Record<string, any> = {}, versions: any[] = []) =>
  mount(PackageInfoSidebar, {
    props: {
      pkg: { name: 'commons-lang3', type: 'maven2', ...pkg },
      versions,
      selectedVersion: '',
    },
    global: { plugins: [ElementPlus] },
  })

describe('PackageInfoSidebar', () => {
  // el-descriptions 每一行是一个 <tr>，内含 label / content 两个单元格
  const repoRowHtml = (wrapper: ReturnType<typeof mountIt>) =>
    wrapper
      .findAll('.el-descriptions__label')
      .find((l) => l.text() === '仓库')
      ?.element.parentElement as HTMLElement | undefined

  it('单仓库回退渲染 repository_name', () => {
    const wrapper = mountIt({ repository: 'maven-local' })
    expect(repoRowHtml(wrapper)!.textContent).toContain('maven-local')
  })

  it('跨仓库聚合展示所有仓库（组合仓库在前）', () => {
    const wrapper = mountIt({
      repository: 'maven-local',
      repositories: ['maven-local', 'maven-central-proxy'],
      group_repositories: ['maven-group'],
    })
    const row = repoRowHtml(wrapper)!
    const chips = [...row.querySelectorAll('.repo-chip')].map((c) => c.textContent)
    expect(chips).toEqual(['maven-group', 'maven-local', 'maven-central-proxy'])
    expect(row.querySelector('.repo-chip')!.className).toContain('repo-chip-group')
  })
})
