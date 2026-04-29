import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import PermissionsConfigForm from '../PermissionsConfigForm.vue'

const createWrapper = (props = {}) => {
  return mount(PermissionsConfigForm, {
    props: {
      form: {
        allow_overwrite: false,
        allow_delete: false,
      },
      ...props,
    },
    global: {
      plugins: [ElementPlus],
    },
  })
}

describe('PermissionsConfigForm', () => {
  it('renders permissions configuration form', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.find('.permissions-config-form').exists()).toBe(true)
  })

  it('shows allow_overwrite switch', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.html()).toContain('允许覆盖')
  })

  it('shows allow_delete switch', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.html()).toContain('允许删除')
  })

  it('binds allow_overwrite correctly when true', () => {
    const wrapper = createWrapper({
      form: {
        allow_overwrite: true,
        allow_delete: false,
      },
    })
    
    expect(wrapper.html()).toContain('允许覆盖')
  })

  it('binds allow_delete correctly when true', () => {
    const wrapper = createWrapper({
      form: {
        allow_overwrite: false,
        allow_delete: true,
      },
    })
    
    expect(wrapper.html()).toContain('允许删除')
  })

  it('updates form when allow_overwrite changes', async () => {
    const form = {
      allow_overwrite: false,
      allow_delete: false,
    }
    
    const wrapper = createWrapper({ form })
    const switchElement = wrapper.findAll('.el-switch')[0]
    
    await switchElement.trigger('click')
    
    expect(form.allow_overwrite).toBe(true)
  })

  it('updates form when allow_delete changes', async () => {
    const form = {
      allow_overwrite: false,
      allow_delete: false,
    }
    
    const wrapper = createWrapper({ form })
    const switchElement = wrapper.findAll('.el-switch')[1]
    
    await switchElement.trigger('click')
    
    expect(form.allow_delete).toBe(true)
  })

  it('has exactly 2 form items', () => {
    const wrapper = createWrapper()
    
    const formItems = wrapper.findAll('.el-form-item')
    expect(formItems).toHaveLength(2)
  })
})
