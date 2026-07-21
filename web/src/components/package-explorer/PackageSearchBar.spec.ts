import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import PackageSearchBar from './PackageSearchBar.vue'
import { PACKAGE_TYPE_OPTIONS } from '@/constants/package'

const baseQuery = {
  q: '', type: 'all', repository: '', version: '', source: 'all',
  sort: 'updated_at' as const, page: 1, pageSize: 20,
}

const mountIt = (props: Record<string, any> = {}) => mount(PackageSearchBar, {
  props: {
    query: baseQuery,
    recentSearches: [],
    loading: false,
    hasActiveFilter: false,
    viewMode: 'table' as const,
    ...props,
  } as any,
  global: { plugins: [ElementPlus] },
})

describe('PackageSearchBar', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  it('渲染搜索框和类型 chips', () => {
    const wrapper = mountIt()
    expect(wrapper.find('.search-input').exists()).toBe(true)
    // 全部 + 所有类型选项
    expect(wrapper.findAll('.type-chip')).toHaveLength(1 + PACKAGE_TYPE_OPTIONS.length)
  })

  it('输入触发 500ms debounce 后 emit search', async () => {
    const wrapper = mountIt()
    await wrapper.find('.search-input input').setValue('react')
    expect(wrapper.emitted('update:query')).toBeTruthy()

    vi.advanceTimersByTime(499)
    expect(wrapper.emitted('search')).toBeFalsy()

    vi.advanceTimersByTime(1)
    expect(wrapper.emitted('search')).toBeTruthy()
  })

  it('回车立即触发 search 并 emit add-recent', async () => {
    const wrapper = mountIt({ query: { ...baseQuery, q: 'react' } })
    await wrapper.find('.search-input input').trigger('keyup.enter')
    expect(wrapper.emitted('search')).toBeTruthy()
    expect(wrapper.emitted('add-recent')?.[0]).toEqual(['react'])
  })

  it('点击类型 chip 触发 update:query with type', async () => {
    const wrapper = mountIt()
    await wrapper.findAll('.type-chip:not(.type-chip--more)')[1].trigger('click')
    const emitted = wrapper.emitted('update:query')?.[0]?.[0] as any
    expect(emitted.type).toBe('npm')
    expect(emitted.page).toBe(1)
  })

  it('聚焦时显示最近搜索下拉', async () => {
    const wrapper = mountIt({ recentSearches: ['react', 'vue'] })
    await wrapper.findComponent({ name: 'ElInput' }).vm.$emit('focus')
    expect(wrapper.find('.recent-dropdown').exists()).toBe(true)
    expect(wrapper.findAll('.recent-item')).toHaveLength(2)
  })

  it('点击最近搜索项触发 search', async () => {
    const wrapper = mountIt({ recentSearches: ['react'] })
    await wrapper.findComponent({ name: 'ElInput' }).vm.$emit('focus')
    await wrapper.find('.recent-item').trigger('mousedown')
    expect(wrapper.emitted('search')).toBeTruthy()
  })

  it('有激活筛选时筛选按钮显示红点', () => {
    const wrapper = mountIt({
      query: { ...baseQuery, repository: 'main' },
      hasActiveFilter: true,
    })
    // 组件用 el-badge is-dot 实现红点，检查 is-dot 类是否应用
    expect(wrapper.find('.el-badge__content.is-dot').exists()).toBe(true)
  })

  it('清空按钮触发清空并搜索', async () => {
    const wrapper = mountIt({ query: { ...baseQuery, q: 'react' } })
    wrapper.findComponent({ name: 'ElInput' }).vm.$emit('clear')
    const emitted = wrapper.emitted('update:query')?.[0]?.[0] as any
    expect(emitted.q).toBe('')
    expect(wrapper.emitted('search')).toBeTruthy()
  })
})
