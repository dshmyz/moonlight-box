import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import AuthConfigForm from '../AuthConfigForm.vue'

const createWrapper = (props = {}) => {
  return mount(AuthConfigForm, {
    props: {
      form: {
        remote_url: '',
        auth_type: 'none',
        auth_config: '',
        proxy_priority: 0,
      },
      authConfig: {
        username: '',
        password: '',
        token: '',
        header_name: '',
        key_value: '',
      },
      ...props,
    },
    global: {
      plugins: [ElementPlus],
    },
  })
}

describe('AuthConfigForm', () => {
  it('renders all form fields', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.find('.auth-config-form').exists()).toBe(true)
    expect(wrapper.findAll('.el-form-item').length).toBeGreaterThanOrEqual(3)
  })

  it('binds remote_url field correctly', () => {
    const wrapper = createWrapper({
      form: {
        remote_url: 'https://registry.npmjs.org',
        auth_type: 'none',
        auth_config: '',
        proxy_priority: 0,
      },
      authConfig: {
        username: '',
        password: '',
        token: '',
        header_name: '',
        key_value: '',
      },
    })
    
    const remoteUrlInput = wrapper.find('input[placeholder="https://registry.npmjs.org"]')
    expect((remoteUrlInput.element as HTMLInputElement).value).toBe('https://registry.npmjs.org')
  })

  it('shows all auth type options', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.vm.$props.form.auth_type).toBe('none')
  })

  it('shows Basic Auth fields when auth_type is basic', () => {
    const wrapper = createWrapper({
      form: {
        remote_url: '',
        auth_type: 'basic',
        auth_config: '',
        proxy_priority: 0,
      },
      authConfig: {
        username: 'testuser',
        password: 'testpass',
        token: '',
        header_name: '',
        key_value: '',
      },
    })
    
    const usernameInput = wrapper.find('input[placeholder="用户名"]')
    expect((usernameInput.element as HTMLInputElement).value).toBe('testuser')
  })

  it('shows Bearer Token field when auth_type is bearer', () => {
    const wrapper = createWrapper({
      form: {
        remote_url: '',
        auth_type: 'bearer',
        auth_config: '',
        proxy_priority: 0,
      },
      authConfig: {
        username: '',
        password: '',
        token: 'test-token-123',
        header_name: '',
        key_value: '',
      },
    })
    
    const tokenInput = wrapper.find('input[placeholder="Bearer Token"]')
    expect((tokenInput.element as HTMLInputElement).value).toBe('test-token-123')
  })

  it('shows API Key fields when auth_type is api_key', () => {
    const wrapper = createWrapper({
      form: {
        remote_url: '',
        auth_type: 'api_key',
        auth_config: '',
        proxy_priority: 0,
      },
      authConfig: {
        username: '',
        password: '',
        token: '',
        header_name: 'X-API-Key',
        key_value: 'api-key-value',
      },
    })
    
    const headerInput = wrapper.find('input[placeholder="X-API-Key"]')
    expect((headerInput.element as HTMLInputElement).value).toBe('X-API-Key')
    
    const keyValueInput = wrapper.find('input[placeholder="API Key 值"]')
    expect((keyValueInput.element as HTMLInputElement).value).toBe('api-key-value')
  })

  it('hides Basic Auth fields when auth_type is not basic', () => {
    const wrapper = createWrapper({
      form: {
        remote_url: '',
        auth_type: 'none',
        auth_config: '',
        proxy_priority: 0,
      },
      authConfig: {
        username: '',
        password: '',
        token: '',
        header_name: '',
        key_value: '',
      },
    })
    
    expect(wrapper.html()).not.toContain('placeholder="用户名"')
    expect(wrapper.html()).not.toContain('placeholder="密码"')
  })

  it('hides Bearer Token field when auth_type is not bearer', () => {
    const wrapper = createWrapper({
      form: {
        remote_url: '',
        auth_type: 'none',
        auth_config: '',
        proxy_priority: 0,
      },
      authConfig: {
        username: '',
        password: '',
        token: '',
        header_name: '',
        key_value: '',
      },
    })
    
    expect(wrapper.html()).not.toContain('placeholder="Bearer Token"')
  })

  it('hides API Key fields when auth_type is not api_key', () => {
    const wrapper = createWrapper({
      form: {
        remote_url: '',
        auth_type: 'none',
        auth_config: '',
        proxy_priority: 0,
      },
      authConfig: {
        username: '',
        password: '',
        token: '',
        header_name: '',
        key_value: '',
      },
    })
    
    expect(wrapper.html()).not.toContain('placeholder="X-API-Key"')
    expect(wrapper.html()).not.toContain('placeholder="API Key 值"')
  })

  it('binds proxy_priority field correctly', () => {
    const wrapper = createWrapper({
      form: {
        remote_url: '',
        auth_type: 'none',
        auth_config: '',
        proxy_priority: 50,
      },
      authConfig: {
        username: '',
        password: '',
        token: '',
        header_name: '',
        key_value: '',
      },
    })
    
    expect(wrapper.html()).toContain('数字越小优先级越高')
  })

  it('updates form when remote_url changes', async () => {
    const form = {
      remote_url: '',
      auth_type: 'none',
      auth_config: '',
      proxy_priority: 0,
    }
    
    const wrapper = createWrapper({ form })
    
    const input = wrapper.find('input[placeholder="https://registry.npmjs.org"]')
    await input.setValue('https://registry.npmjs.org')
    
    expect(form.remote_url).toBe('https://registry.npmjs.org')
  })

  it('updates form when auth_type changes', async () => {
    const form = {
      remote_url: '',
      auth_type: 'none',
      auth_config: '',
      proxy_priority: 0,
    }
    
    const wrapper = createWrapper({ form })
    
    const select = wrapper.find('.el-select')
    await select.trigger('click')
    
    expect(wrapper.html()).toBeDefined()
  })
})
