import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PublicBrowseTabs from './PublicBrowseTabs.vue'
import ElementPlus from 'element-plus'

const mountIt = (props: Record<string, any> = {}) => mount(PublicBrowseTabs, {
  props: { activeTab: 'packages', ...props } as any,
  global: { plugins: [ElementPlus] },
})

describe('PublicBrowseTabs', () => {
  it('默认显示两个 Tab', () => {
    const wrapper = mountIt()
    const tabs = wrapper.findAll('.browse-tab')
    expect(tabs).toHaveLength(2)
  })

  it('activeTab=packages 时包 Tab 高亮', () => {
    const wrapper = mountIt({ activeTab: 'packages' })
    const tabs = wrapper.findAll('.browse-tab')
    expect(tabs[0].classes()).toContain('browse-tab--active')
    expect(tabs[1].classes()).not.toContain('browse-tab--active')
  })

  it('activeTab=repositories 时仓库 Tab 高亮', () => {
    const wrapper = mountIt({ activeTab: 'repositories' })
    const tabs = wrapper.findAll('.browse-tab')
    expect(tabs[0].classes()).not.toContain('browse-tab--active')
    expect(tabs[1].classes()).toContain('browse-tab--active')
  })

  it('点击仓库 Tab 触发 update:activeTab', async () => {
    const wrapper = mountIt()
    await wrapper.findAll('.browse-tab')[1].trigger('click')
    expect(wrapper.emitted('update:activeTab')?.[0]).toEqual(['repositories'])
  })

  it('显示包数量统计', () => {
    const wrapper = mountIt({ packageCount: 128 })
    expect(wrapper.text()).toContain('128')
  })

  it('不传 packageCount 时不显示统计', () => {
    const wrapper = mountIt()
    expect(wrapper.find('.tab-count').exists()).toBe(false)
  })
})