import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import PackageExplorer from './PackageExplorer.vue'
import { packageApi } from '@/api/package'
import { permission } from '@/directives/permission'
import { useAuthStore } from '@/stores/auth'

vi.mock('@/api/package', () => ({
  packageApi: {
    search: vi.fn(),
    deletePackage: vi.fn(),
    getVersions: vi.fn(),
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useRoute: () => ({ query: {} }),
  createRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    beforeEach: vi.fn(),
    afterEach: vi.fn(),
  }),
  createWebHistory: () => ({}),
}))

const mockPackages = [
  { id: 1, name: 'lodash', display_name: 'lodash', type: 'npm', download_count: 100, updated_at: '2026-06-19T00:00:00Z' },
  { id: 2, name: 'react', display_name: 'react', type: 'npm', download_count: 500, updated_at: '2026-06-18T00:00:00Z' },
]

// 上传对话框和版本抽屉属于子组件的下游依赖，本测试只验证 PackageExplorer 容器行为，
// 用 stub 隔离以避免其内部副作用（如真实 HTTP、DOM API）干扰容器逻辑断言。
const mountIt = (props: Record<string, any> = {}) => {
  const pinia = createPinia()
  setActivePinia(pinia)
  // 注入 admin 用户，使 v-permission 对 package:write / package:delete 放行
  const store = useAuthStore()
  // @ts-expect-error 测试注入 admin user
  store.user = { roles: ['admin'], permissions: [] }
  return mount(PackageExplorer, {
    props: { mode: 'admin', ...props } as any,
    global: {
      plugins: [ElementPlus, pinia],
      directives: { permission },
      stubs: {
        UploadPackageDialog: true,
        VersionDrawer: true,
      },
    },
  })
}

describe('PackageExplorer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(packageApi.search as any).mockResolvedValue({ list: mockPackages, total: 2, search_time_ms: 5 })
  })

  it('admin 模式显示上传按钮', async () => {
    const wrapper = mountIt({ mode: 'admin' })
    await flushPromises()
    expect(wrapper.find('.upload-btn').exists()).toBe(true)
  })

  it('public 模式不显示上传按钮', async () => {
    const wrapper = mountIt({ mode: 'public' })
    await flushPromises()
    expect(wrapper.find('.upload-btn').exists()).toBe(false)
  })

  it('admin 模式默认表格视图', async () => {
    const wrapper = mountIt({ mode: 'admin' })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'PackageTable' }).exists()).toBe(true)
  })

  it('首次加载显示骨架屏', async () => {
    ;(packageApi.search as any).mockImplementation(() => new Promise(() => {}))
    const wrapper = mountIt({ mode: 'admin' })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'PackageSkeleton' }).exists()).toBe(true)
  })

  it('加载完成无数据显示空状态', async () => {
    ;(packageApi.search as any).mockResolvedValue({ list: [], total: 0, search_time_ms: 0 })
    const wrapper = mountIt({ mode: 'admin' })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'PackageEmptyState' }).exists()).toBe(true)
  })

  it('切换视图为 grid 时显示 PackageGrid', async () => {
    const wrapper = mountIt({ mode: 'admin' })
    await flushPromises()
    await wrapper.findComponent({ name: 'PackageSearchBar' }).vm.$emit('update:viewMode', 'grid')
    expect(wrapper.findComponent({ name: 'PackageGrid' }).exists()).toBe(true)
  })

  it('搜索耗时显示在结果区', async () => {
    const wrapper = mountIt({ mode: 'admin' })
    await flushPromises()
    expect(wrapper.text()).toContain('5ms')
  })
})
