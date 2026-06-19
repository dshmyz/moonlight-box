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

  it('权限字符串格式为 resource:action', () => {
    setActivePinia(createPinia())
    const store = useAuthStore()
    // @ts-expect-error 测试直接注入 user
    store.user = { permissions: [{ resource: 'package', action: 'write' }] }
    const wrapper = mount(defineComponent({
      directives: { permission },
      template: `<div><button v-permission="'package:write'">上传</button></div>`,
    }))
    expect(wrapper.find('button').exists()).toBe(true)
  })
})
