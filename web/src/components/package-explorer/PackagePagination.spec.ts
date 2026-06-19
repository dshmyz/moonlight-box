import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import PackagePagination from './PackagePagination.vue'

describe('PackagePagination', () => {
  const mountIt = (props: Record<string, any>) => mount(PackagePagination, {
    props: props as any,
    global: { plugins: [ElementPlus] },
  })

  it('显示总数信息', () => {
    const wrapper = mountIt({ total: 100, page: 1, pageSize: 20, pageSizeOptions: [20, 50, 100] })
    expect(wrapper.text()).toContain('100')
    expect(wrapper.text()).toContain('个包')
  })

  it('页码变化触发 update:page 事件', async () => {
    const wrapper = mountIt({ total: 100, page: 1, pageSize: 20, pageSizeOptions: [20, 50, 100] })
    await wrapper.findComponent({ name: 'ElPagination' }).vm.$emit('current-change', 3)
    expect(wrapper.emitted('update:page')?.[0]).toEqual([3])
  })

  it('每页大小变化触发 update:pageSize 并重置到第1页', async () => {
    const wrapper = mountIt({ total: 100, page: 3, pageSize: 20, pageSizeOptions: [20, 50, 100] })
    await wrapper.findComponent({ name: 'ElPagination' }).vm.$emit('size-change', 50)
    expect(wrapper.emitted('update:pageSize')?.[0]).toEqual([50])
    expect(wrapper.emitted('update:page')?.[0]).toEqual([1])
  })

  it('total 为 0 时不渲染', () => {
    const wrapper = mountIt({ total: 0, page: 1, pageSize: 20, pageSizeOptions: [20, 50, 100] })
    expect(wrapper.find('.pagination-wrapper').exists()).toBe(false)
  })
})
