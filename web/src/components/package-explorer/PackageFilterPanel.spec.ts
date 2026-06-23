import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { flushPromises } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import PackageFilterPanel from './PackageFilterPanel.vue'

vi.mock('@/api/repository', () => ({
  repositoryApi: {
    list: vi.fn().mockResolvedValue([
      { id: 1, name: 'npm-proxy', type: 'proxy', package_type: 'npm' },
      { id: 2, name: 'maven-hosted', type: 'local', package_type: 'maven' },
    ]),
  },
}))

const mountIt = (props: Record<string, any> = {}) => mount(PackageFilterPanel, {
  props: { visible: true, repository: '', version: '', ...props } as any,
  global: { plugins: [ElementPlus] },
})

describe('PackageFilterPanel', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('visible=false 时不渲染面板', () => {
    const wrapper = mountIt({ visible: false })
    expect(wrapper.find('.filter-panel-inline').exists()).toBe(false)
  })

  it('visible=true 时渲染面板', () => {
    const wrapper = mountIt()
    expect(wrapper.find('.filter-panel-inline').exists()).toBe(true)
  })

  it('打开时加载仓库列表', async () => {
    const wrapper = mountIt()
    await flushPromises()
    const options = wrapper.findAllComponents({ name: 'ElOption' })
    expect(options.length).toBeGreaterThanOrEqual(2)
  })

  it('选择仓库触发 update:repository', async () => {
    const wrapper = mountIt()
    await flushPromises()
    await wrapper.findComponent({ name: 'ElSelect' }).vm.$emit('update:modelValue', 'npm-proxy')
    expect(wrapper.emitted('update:repository')?.[0]).toEqual(['npm-proxy'])
  })

  it('输入版本触发 update:version', async () => {
    const wrapper = mountIt()
    await flushPromises()
    const input = wrapper.findComponent({ name: 'ElInput' })
    await input.vm.$emit('update:modelValue', '1.2.*')
    expect(wrapper.emitted('update:version')?.[0]).toEqual(['1.2.*'])
  })

  it('点击重置清空仓库和版本', async () => {
    const wrapper = mountIt({ repository: 'npm-proxy', version: '1.0' })
    await flushPromises()
    await wrapper.find('[data-test="reset"]').trigger('click')
    expect(wrapper.emitted('update:repository')?.[0]).toEqual([''])
    expect(wrapper.emitted('update:version')?.[0]).toEqual([''])
  })

  it('点击应用触发 apply 事件', async () => {
    const wrapper = mountIt()
    await flushPromises()
    await wrapper.find('[data-test="apply"]').trigger('click')
    expect(wrapper.emitted('apply')).toBeTruthy()
  })
})
