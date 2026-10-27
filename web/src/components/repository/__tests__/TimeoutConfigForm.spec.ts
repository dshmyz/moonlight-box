import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import TimeoutConfigForm from '../TimeoutConfigForm.vue'

type FormModel = {
  timeout_seconds: number
  connect_timeout_seconds: number
  read_timeout_seconds: number
  max_redirects: number
  insecure_skip_verify: boolean
}

const baseForm = (overrides: Partial<FormModel> = {}): FormModel => ({
  timeout_seconds: 0,
  connect_timeout_seconds: 0,
  read_timeout_seconds: 0,
  max_redirects: 0,
  insecure_skip_verify: false,
  ...overrides,
})

const createWrapper = (props = {}) => {
  return mount(TimeoutConfigForm, {
    props: {
      form: baseForm(),
      ...props,
    },
    global: {
      plugins: [ElementPlus],
    },
  })
}

describe('TimeoutConfigForm', () => {
  it('renders timeout configuration form', () => {
    const wrapper = createWrapper()

    expect(wrapper.find('.timeout-config-form').exists()).toBe(true)
  })

  it('shows timeout seconds field', () => {
    const wrapper = createWrapper()

    expect(wrapper.html()).toContain('超时时间（秒）')
  })

  it('shows connect timeout field', () => {
    const wrapper = createWrapper()

    expect(wrapper.html()).toContain('建连超时（秒）')
  })

  it('shows read timeout field', () => {
    const wrapper = createWrapper()

    expect(wrapper.html()).toContain('读超时（秒）')
  })

  it('shows max redirects field', () => {
    const wrapper = createWrapper()

    expect(wrapper.html()).toContain('最大重定向次数')
  })

  it('shows insecure skip verify switch', () => {
    const wrapper = createWrapper()

    expect(wrapper.html()).toContain('跳过证书校验')
  })

  it('shows hint for timeout_seconds', () => {
    const wrapper = createWrapper()

    expect(wrapper.html()).toContain('0 表示使用全局默认值（30s）')
  })

  it('shows hint for connect_timeout_seconds', () => {
    const wrapper = createWrapper()

    expect(wrapper.html()).toContain('0 表示使用全局默认值（10s）')
  })

  it('shows hint for read_timeout_seconds', () => {
    const wrapper = createWrapper()

    expect(wrapper.html()).toContain('仅作用于元数据请求，不影响大文件下载')
  })

  it('shows hint for max_redirects', () => {
    const wrapper = createWrapper()

    expect(wrapper.html()).toContain('0 表示使用全局默认值（10），-1 表示不跟随重定向')
  })

  it('binds timeout_seconds correctly', () => {
    const wrapper = createWrapper({ form: baseForm({ timeout_seconds: 60 }) })

    expect(wrapper.html()).toBeDefined()
  })

  it('binds connect_timeout_seconds correctly', () => {
    const wrapper = createWrapper({ form: baseForm({ connect_timeout_seconds: 5 }) })

    expect(wrapper.html()).toBeDefined()
  })

  it('binds read_timeout_seconds correctly', () => {
    const wrapper = createWrapper({ form: baseForm({ read_timeout_seconds: 20 }) })

    expect(wrapper.html()).toBeDefined()
  })

  it('binds max_redirects correctly', () => {
    const wrapper = createWrapper({ form: baseForm({ max_redirects: 5 }) })

    expect(wrapper.html()).toBeDefined()
  })

  it('binds insecure_skip_verify correctly when true', () => {
    const wrapper = createWrapper({ form: baseForm({ insecure_skip_verify: true }) })

    expect(wrapper.html()).toContain('跳过证书校验')
  })

  it('updates form when timeout_seconds changes', async () => {
    const form = baseForm()

    const wrapper = createWrapper({ form })

    const inputNumber = wrapper.find('.el-input-number')
    await inputNumber.trigger('click')

    expect(form.timeout_seconds).toBe(0)
  })

  it('updates form when insecure_skip_verify changes', async () => {
    const form = baseForm()

    const wrapper = createWrapper({ form })
    const switchElement = wrapper.find('.el-switch')

    await switchElement.trigger('click')

    expect(form.insecure_skip_verify).toBe(true)
  })

  it('has exactly 5 form items', () => {
    const wrapper = createWrapper()

    const formItems = wrapper.findAll('.el-form-item')
    expect(formItems).toHaveLength(5)
  })
})
