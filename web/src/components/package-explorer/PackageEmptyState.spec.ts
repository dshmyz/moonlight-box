import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import PackageEmptyState from './PackageEmptyState.vue'

const createWrapper = (props = {}) => {
  const app = createApp({})
  app.use(ElementPlus)

  return mount(PackageEmptyState, {
    props: {
      variant: 'empty',
      mode: 'admin',
      ...props,
    },
    global: {
      plugins: [ElementPlus],
    },
  })
}

describe('PackageEmptyState', () => {
  it('无数据状态显示"暂无包"', () => {
    const wrapper = createWrapper({ variant: 'empty', mode: 'admin' })
    expect(wrapper.text()).toContain('暂无包')
    expect(wrapper.text()).toContain('上传第一个包')
  })

  it('无匹配结果状态显示"未找到匹配的包"', () => {
    const wrapper = createWrapper({ variant: 'no-match', mode: 'admin' })
    expect(wrapper.text()).toContain('未找到匹配的包')
    expect(wrapper.find('[data-test="clear-filters"]').exists()).toBe(true)
  })

  it('加载失败状态显示"加载失败"和重试按钮', () => {
    const wrapper = createWrapper({
      variant: 'error',
      mode: 'admin',
      errorMessage: '网络错误',
    })
    expect(wrapper.text()).toContain('加载失败')
    expect(wrapper.text()).toContain('网络错误')
    expect(wrapper.find('[data-test="retry"]').exists()).toBe(true)
  })

  it('点击重试按钮触发 retry 事件', async () => {
    const wrapper = createWrapper({
      variant: 'error',
      mode: 'admin',
      errorMessage: '网络错误',
    })
    await wrapper.find('[data-test="retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toBeTruthy()
  })

  it('点击清空筛选触发 clear-filters 事件', async () => {
    const wrapper = createWrapper({ variant: 'no-match', mode: 'admin' })
    await wrapper.find('[data-test="clear-filters"]').trigger('click')
    expect(wrapper.emitted('clear-filters')).toBeTruthy()
  })

  it('public 模式无数据状态不显示上传引导', () => {
    const wrapper = createWrapper({ variant: 'empty', mode: 'public' })
    expect(wrapper.text()).not.toContain('上传第一个包')
  })
})
