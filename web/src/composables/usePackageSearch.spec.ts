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

describe('usePackageSearch - 删除后页码修正', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(packageApi.search as any).mockReset()
    ;(packageApi.deletePackage as any).mockReset()
    setActivePinia(createPinia())
    mockRouteQuery = {}
  })

  it('删除后当前页空且非第1页，自动回退一页重试', async () => {
    ;(packageApi.search as any)
      .mockResolvedValueOnce(mockSearchResponse([], 0))
      .mockResolvedValueOnce(mockSearchResponse([
        { id: 2, name: 'pkg2', type: 'npm' },
      ], 1))

    const cs = mountComposable()
    cs.query.page = 3
    await cs.refresh()

    expect(packageApi.search).toHaveBeenCalledTimes(2)
    expect(packageApi.search).toHaveBeenNthCalledWith(1, expect.objectContaining({ page: 3 }))
    expect(packageApi.search).toHaveBeenNthCalledWith(2, expect.objectContaining({ page: 2 }))
    expect(cs.query.page).toBe(2)
    expect(cs.packages.value).toHaveLength(1)
  })

  it('删除后当前页空但已是第1页，不回退', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse([], 0))

    const cs = mountComposable()
    cs.query.page = 1
    await cs.refresh()

    expect(packageApi.search).toHaveBeenCalledTimes(1)
    expect(cs.query.page).toBe(1)
  })

  it('批量删除导致多页变空时连续回退到有数据的页（验证 while 循环）', async () => {
    // 模拟：第5页空 → 第4页空 → 第3页空 → 第2页有数据
    ;(packageApi.search as any)
      .mockResolvedValueOnce(mockSearchResponse([], 0))  // page 5
      .mockResolvedValueOnce(mockSearchResponse([], 0))  // page 4
      .mockResolvedValueOnce(mockSearchResponse([], 0))  // page 3
      .mockResolvedValueOnce(mockSearchResponse([
        { id: 10, name: 'pkg10', type: 'npm' },
      ], 1))  // page 2

    const cs = mountComposable()
    cs.query.page = 5
    await cs.refresh()

    // 应该连续回退 3 次（5→4→3→2），共 4 次 API 调用
    expect(packageApi.search).toHaveBeenCalledTimes(4)
    expect(packageApi.search).toHaveBeenNthCalledWith(1, expect.objectContaining({ page: 5 }))
    expect(packageApi.search).toHaveBeenNthCalledWith(2, expect.objectContaining({ page: 4 }))
    expect(packageApi.search).toHaveBeenNthCalledWith(3, expect.objectContaining({ page: 3 }))
    expect(packageApi.search).toHaveBeenNthCalledWith(4, expect.objectContaining({ page: 2 }))
    expect(cs.query.page).toBe(2)
    expect(cs.packages.value).toHaveLength(1)
  })

  it('所有页都空时回退到第1页停止（不会死循环）', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse([], 0))

    const cs = mountComposable()
    cs.query.page = 5
    await cs.refresh()

    // 从 page 5 回退到 page 1，共 5 次调用
    expect(packageApi.search).toHaveBeenCalledTimes(5)
    expect(cs.query.page).toBe(1)
    expect(cs.packages.value).toEqual([])
  })
})

describe('usePackageSearch - URL 同步', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    mockRouteQuery = {}
    ;(packageApi.search as any).mockReset()
  })

  it('从 URL 读取初始查询参数', async () => {
    mockRouteQuery = { q: 'react', type: 'npm', sort: 'download_count', page: '3' }
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable()
    // onMounted 触发 readFromUrl
    await cs.search()

    expect(cs.query.q).toBe('react')
    expect(cs.query.type).toBe('npm')
    expect(cs.query.sort).toBe('download_count')
    expect(cs.query.page).toBe(3)
  })

  it('setQuery 后用 replace 更新 URL（非分页变化）', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable()
    cs.setQuery({ q: 'vue' })

    expect(mockReplace).toHaveBeenCalledWith(expect.objectContaining({
      query: expect.objectContaining({ q: 'vue' }),
    }))
  })

  it('分页变化用 push 更新 URL（支持前进后退）', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable()
    cs.setQuery({ page: 2 })

    expect(mockPush).toHaveBeenCalledWith(expect.objectContaining({
      query: expect.objectContaining({ page: '2' }),
    }))
  })

  it('syncUrl=false 时不更新 URL', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable('admin', { syncUrl: false })
    cs.setQuery({ q: 'vue' })

    expect(mockPush).not.toHaveBeenCalled()
    expect(mockReplace).not.toHaveBeenCalled()
  })
})
