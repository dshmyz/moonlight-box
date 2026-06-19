import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { permission } from './permission'
import { useAuthStore } from '@/stores/auth'

const wrap = (hasPerm: boolean) => {
  setActivePinia(createPinia())
  const store = useAuthStore()
  // @ts-expect-error 测试直接注入 user
  store.user = { permissions: hasPerm ? [{ resource: 'package', action: 'delete' }] : [] }

  return mount(defineComponent({
    directives: { permission },
    template: `<div><button v-permission="'package:delete'">删除</button></div>`,
  }))
}

describe('v-permission', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('有权限时保留元素', () => {
    const wrapper = wrap(true)
    expect(wrapper.find('button').exists()).toBe(true)
  })

  it('无权限时从 DOM 移除元素', () => {
    const wrapper = wrap(false)
    expect(wrapper.find('button').exists()).toBe(false)
  })

  it('非法权限字符串格式时保留元素（fail-open）', () => {
    setActivePinia(createPinia())
    const store = useAuthStore()
    // @ts-expect-error 测试直接注入 user
    store.user = { permissions: [] }

    // 空字符串
    const w1 = mount(defineComponent({
      directives: { permission },
      template: `<div><button v-permission="''">x</button></div>`,
    }))
    expect(w1.find('button').exists()).toBe(true)

    // 无冒号
    const w2 = mount(defineComponent({
      directives: { permission },
      template: `<div><button v-permission="'package'">x</button></div>`,
    }))
    expect(w2.find('button').exists()).toBe(true)

    // action 为空（"package:"）
    const w3 = mount(defineComponent({
      directives: { permission },
      template: `<div><button v-permission="'package:'">x</button></div>`,
    }))
    expect(w3.find('button').exists()).toBe(true)
  })

  it('admin 用户拥有所有权限', () => {
    setActivePinia(createPinia())
    const store = useAuthStore()
    // @ts-expect-error 测试直接注入 user，roles 包含 admin 使 isAdmin computed 为 true
    store.user = { roles: ['admin'], permissions: [] }
    const wrapper = mount(defineComponent({
      directives: { permission },
      template: `<div><button v-permission="'package:delete'">删除</button></div>`,
    }))
    expect(wrapper.find('button').exists()).toBe(true)
  })
})
