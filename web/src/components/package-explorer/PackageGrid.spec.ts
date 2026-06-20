import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import { permission } from '@/directives/permission'
import { useAuthStore } from '@/stores/auth'
import PackageGrid from './PackageGrid.vue'

vi.mock('@/utils/clipboard', () => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))

const samplePackages = [
  { id: 1, name: 'lodash', display_name: 'lodash', type: 'npm', description: 'Utility library', latest_version: '4.17.21', download_count: 100, updated_at: '2026-06-19T00:00:00Z' },
  { id: 2, name: 'react', display_name: 'react', type: 'npm', description: 'UI library', latest_version: '18.0.0', download_count: 500, updated_at: '2026-06-18T00:00:00Z' },
]

const mountIt = (props: Record<string, any> = {}) => {
  const pinia = createPinia()
  setActivePinia(pinia)
  // 注入 admin 用户，使 v-permission 对 package:delete 放行
  const store = useAuthStore()
  // @ts-expect-error 测试直接注入 admin user
  store.user = { roles: ['admin'], permissions: [] }

  return mount(PackageGrid, {
    props: {
      packages: samplePackages,
      mode: 'admin',
      ...props,
    } as any,
    global: {
      plugins: [ElementPlus, pinia],
      directives: { permission },
    },
  })
}

describe('PackageGrid', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('渲染卡片网格', () => {
    const wrapper = mountIt()
    expect(wrapper.findAll('.package-card')).toHaveLength(2)
  })

  it('卡片显示包名、版本、下载量', () => {
    const wrapper = mountIt()
    const card = wrapper.find('.package-card')
    expect(card.text()).toContain('lodash')
    expect(card.text()).toContain('4.17.21')
    expect(card.text()).toContain('100')
  })

  it('点击卡片触发 view-detail', async () => {
    const wrapper = mountIt()
    await wrapper.find('.package-card').trigger('click')
    expect(wrapper.emitted('view-detail')?.[0]).toEqual([samplePackages[0]])
  })

  it('复制按钮触发复制 type:name', async () => {
    const { copyToClipboard } = await import('@/utils/clipboard')
    const wrapper = mountIt()
    await wrapper.find('.copy-name-btn').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('npm:lodash')
  })

  it('admin 模式显示操作按钮', () => {
    const wrapper = mountIt({ mode: 'admin' })
    expect(wrapper.find('.btn-view-versions').exists()).toBe(true)
    expect(wrapper.find('.btn-delete').exists()).toBe(true)
  })

  it('public 模式不显示操作按钮', () => {
    const wrapper = mountIt({ mode: 'public' })
    expect(wrapper.find('.btn-view-versions').exists()).toBe(false)
    expect(wrapper.find('.btn-delete').exists()).toBe(false)
  })
})
