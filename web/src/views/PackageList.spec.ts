import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import PackageList from './PackageList.vue'
import { packageApi } from '@/api/package'

const push = vi.fn()

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRouter: () => ({ push }),
  }
})

vi.mock('@/api/package', () => ({
  packageApi: {
    search: vi.fn(),
    deletePackage: vi.fn(),
  },
}))

const stubs = {
  UploadPackageDialog: { template: '<div />' },
  VersionDrawer: { template: '<div />' },
}

describe('PackageList', () => {
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
          repository_type: 'proxy',
          download_count: 12,
          updated_at: '2026-05-24T00:00:00Z',
        },
      ],
      total: 1,
    })

    const wrapper = mount(PackageList, {
      global: {
        plugins: [ElementPlus],
        stubs,
      },
    })

    await flushPromises()
    await wrapper.find('.btn-view-detail').trigger('click')

    expect(push).toHaveBeenCalledWith({
      name: 'AdminPackageDetail',
      params: { type: 'npm', name: 'lodash' },
    })
  })

  it('uses package_type from search results when opening package detail', async () => {
    ;(packageApi.search as any).mockResolvedValue({
      list: [
        {
          id: 1,
          name: 'lodash',
          display_name: 'lodash',
          package_type: 'npm',
          repository_type: 'proxy',
          download_count: 12,
          updated_at: '2026-05-24T00:00:00Z',
        },
      ],
      total: 1,
    })

    const wrapper = mount(PackageList, {
      global: {
        plugins: [ElementPlus],
        stubs,
      },
    })

    await flushPromises()
    await wrapper.find('.btn-view-detail').trigger('click')

    expect(push).toHaveBeenCalledWith({
      name: 'AdminPackageDetail',
      params: { type: 'npm', name: 'lodash' },
    })
  })
})
