import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import PublicBrowsePage from './PublicBrowsePage.vue'
import { packageApi } from '@/api/package'

const push = vi.fn()
const replace = vi.fn()

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRouter: () => ({ push, replace }),
    useRoute: () => ({ query: {} }),
  }
})

vi.mock('@/api/package', () => ({
  packageApi: { search: vi.fn().mockResolvedValue({ items: [], total: 0, search_time_ms: 2 }) },
}))

vi.mock('@/api/repository', () => ({
  repositoryApi: { list: vi.fn().mockResolvedValue([]) },
}))

const mountPage = () => mount(PublicBrowsePage, {
  global: {
    plugins: [ElementPlus],
    stubs: {
      PackageSearchBar: { template: '<div class="package-search-bar-stub" />' },
      PackageFilterPanel: { template: '<div class="filter-panel-inline-stub" />' },
      PackageGrid: { template: '<div class="package-grid-stub" />' },
      PackagePagination: { template: '<div class="pagination-stub" />' },
      PublicPackageHero: { template: '<div class="public-hero-stub"><slot name="search" /></div>' },
      PublicBrowseTabs: { template: '<div class="browse-tabs-stub" />' },
      PackageEmptyState: { template: '<div class="empty-state-stub" />' },
      RepositoryShowcase: { template: '<div class="repos-main-stub" />' },
      RepositoryStatusPanel: { template: '<div class="repos-sidebar-stub" />' },
    },
  },
})

describe('PublicBrowsePage', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('渲染 Hero 组件', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.find('.public-hero-stub').exists()).toBe(true)
  })

  it('默认显示包 Tab 并包含 PackageSearchBar', async () => {
    const wrapper = mountPage()
    await flushPromises()
    // 包 Tab 初始状态使用 v-if="activeTab === 'packages'" 所以应有 PackageSearchBar stub
    expect(wrapper.find('.package-search-bar-stub').exists()).toBe(true)
  })

  it('搜索时调用 packageApi.search', async () => {
    mountPage()
    await flushPromises()
    expect(packageApi.search).toHaveBeenCalled()
  })

  it('仓库 Tab 内容显示', async () => {
    const wrapper = mountPage()
    await flushPromises()
    // 默认 activeTab=packages，仓库 Tab stub 不应该出现
    expect(wrapper.find('.repos-main-stub').exists()).toBe(false)
  })
})