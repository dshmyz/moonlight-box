import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { usePackageSearch } from './usePackageSearch'
import { packageApi } from '@/api/package'

vi.mock('@/api/package', () => ({
  packageApi: {
    search: vi.fn(),
    deletePackage: vi.fn(),
  },
}))

const mockPush = vi.fn()
const mockReplace = vi.fn()
let mockRouteQuery: Record<string, any> = {}

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: mockReplace,
  }),
  useRoute: () => ({ query: mockRouteQuery }),
}))

const mockSearchResponse = (list: any[] = [], total = 0) => ({
  list,
  total,
  page: 1,
  page_size: 20,
  search_time_ms: 5,
})

function mountComposable(mode: 'admin' | 'public' = 'admin', opts: Record<string, any> = {}) {
  let result: ReturnType<typeof usePackageSearch> | null = null
  mount(defineComponent({
    setup() {
      result = usePackageSearch({ mode, ...opts })
      return {}
    },
    template: '<div />',
  }), { global: { plugins: [createPinia()] } })
  return result!
}

describe('usePackageSearch - 核心查询', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    mockRouteQuery = {}
  })

  it('初始加载调用 API 并填充 packages', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse([
      { id: 1, name: 'lodash', type: 'npm', download_count: 10, updated_at: '2026-06-19T00:00:00Z' },
    ], 1))

    const cs = mountComposable()
    await cs.search()

    expect(packageApi.search).toHaveBeenCalledWith(expect.objectContaining({
      q: '',
      page: 1,
      page_size: 20,
      sort: 'updated_at',
    }))
    expect(cs.packages.value).toHaveLength(1)
    expect(cs.packages.value[0].name).toBe('lodash')
    expect(cs.total.value).toBe(1)
    expect(cs.loading.value).toBe(false)
  })

  it('类型归一化：format 字段回退到 type', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse([
      { id: 1, name: 'pkg', format: 'npm', download_count: 0, updated_at: '2026-06-19T00:00:00Z' },
    ], 1))

    const cs = mountComposable()
    await cs.search()
    expect(cs.packages.value[0].type).toBe('npm')
  })

  it('setQuery 自动重置到第 1 页', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse([], 0))

    const cs = mountComposable()
    cs.query.page = 5
    cs.setQuery({ q: 'react' })
    expect(cs.query.page).toBe(1)
    expect(cs.query.q).toBe('react')
  })

  it('排序字段直接透传 download_count（验证 bug 修复）', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable()
    cs.setQuery({ sort: 'download_count' })
    await cs.search()

    expect(packageApi.search).toHaveBeenCalledWith(expect.objectContaining({
      sort: 'download_count',
    }))
  })

  it('API 失败时设置 error 状态', async () => {
    ;(packageApi.search as any).mockRejectedValue(new Error('network'))

    const cs = mountComposable()
    await cs.search()

    expect(cs.error.value).toBeTruthy()
    expect(cs.packages.value).toEqual([])
  })
})
