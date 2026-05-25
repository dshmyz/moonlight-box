import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import BrowsePage from './BrowsePage.vue'
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
  packageApi: {
    search: vi.fn(),
  },
}))

vi.mock('@/components/browse/HeroSection.vue', () => ({
  default: { template: '<div />' },
}))

vi.mock('@/components/browse/RepositoryShowcase.vue', () => ({
  default: { template: '<div />' },
}))

describe('BrowsePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('uses format from search results when opening package detail', async () => {
    ;(packageApi.search as any).mockResolvedValue({
      list: [
        {
          id: 1,
          name: 'lodash',
          display_name: 'lodash',
          format: 'npm',
          download_count: 12,
          updated_at: '2026-05-24T00:00:00Z',
        },
      ],
      total: 1,
      search_time_ms: 2,
    })

    const wrapper = mount(BrowsePage, {
      global: {
        plugins: [ElementPlus],
      },
    })

    await flushPromises()
    await wrapper.find('.package-card').trigger('click')

    expect(push).toHaveBeenCalledWith('/packages/npm/lodash')
  })

  it('uses package_type from search results when opening package detail', async () => {
    ;(packageApi.search as any).mockResolvedValue({
      list: [
        {
          id: 1,
          name: 'lodash',
          display_name: 'lodash',
          package_type: 'npm',
          download_count: 12,
          updated_at: '2026-05-24T00:00:00Z',
        },
      ],
      total: 1,
      search_time_ms: 2,
    })

    const wrapper = mount(BrowsePage, {
      global: {
        plugins: [ElementPlus],
      },
    })

    await flushPromises()
    await wrapper.find('.package-card').trigger('click')

    expect(push).toHaveBeenCalledWith('/packages/npm/lodash')
  })
})
