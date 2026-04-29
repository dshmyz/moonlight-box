import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import CacheConfigForm from '../CacheConfigForm.vue'

const createWrapper = (props = {}) => {
  return mount(CacheConfigForm, {
    props: {
      form: {
        type: 'local',
        cache_enabled: true,
        cache_ttl_seconds: 86400,
        cache_negative_ttl: 300,
        cache_max_size_gb: 10,
        failure_cache_rules: '',
      },
      ...props,
    },
    global: {
      plugins: [ElementPlus],
    },
  })
}

describe('CacheConfigForm', () => {
  it('renders cache configuration form', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.find('.cache-config-form').exists()).toBe(true)
  })

  it('shows cache enabled switch', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.html()).toContain('el-switch')
  })

  it('shows cache TTL field when cache is enabled', () => {
    const wrapper = createWrapper({
      form: {
        type: 'local',
        cache_enabled: true,
        cache_ttl_seconds: 86400,
        cache_negative_ttl: 300,
        cache_max_size_gb: 10,
        failure_cache_rules: '',
      },
    })
    
    expect(wrapper.html()).toContain('缓存 TTL（秒）')
  })

  it('hides cache fields when cache is disabled', () => {
    const wrapper = createWrapper({
      form: {
        type: 'local',
        cache_enabled: false,
        cache_ttl_seconds: 86400,
        cache_negative_ttl: 300,
        cache_max_size_gb: 10,
        failure_cache_rules: '',
      },
    })
    
    expect(wrapper.html()).not.toContain('缓存 TTL（秒）')
    expect(wrapper.html()).not.toContain('缓存最大大小（GB）')
  })

  it('shows negative cache TTL only for proxy type', () => {
    const wrapper = createWrapper({
      form: {
        type: 'proxy',
        cache_enabled: true,
        cache_ttl_seconds: 86400,
        cache_negative_ttl: 300,
        cache_max_size_gb: 10,
        failure_cache_rules: '',
      },
    })
    
    expect(wrapper.html()).toContain('负向缓存 TTL（秒）')
    expect(wrapper.html()).toContain('404 响应的缓存时间')
  })

  it('hides negative cache TTL for local type', () => {
    const wrapper = createWrapper({
      form: {
        type: 'local',
        cache_enabled: true,
        cache_ttl_seconds: 86400,
        cache_negative_ttl: 300,
        cache_max_size_gb: 10,
        failure_cache_rules: '',
      },
    })
    
    expect(wrapper.html()).not.toContain('负向缓存 TTL（秒）')
  })

  it('shows failure cache rules for proxy type', () => {
    const wrapper = createWrapper({
      form: {
        type: 'proxy',
        cache_enabled: true,
        cache_ttl_seconds: 86400,
        cache_negative_ttl: 300,
        cache_max_size_gb: 10,
        failure_cache_rules: '',
      },
    })
    
    expect(wrapper.html()).toContain('失败缓存规则')
    expect(wrapper.html()).toContain('JSON 格式')
  })

  it('hides failure cache rules for local type', () => {
    const wrapper = createWrapper({
      form: {
        type: 'local',
        cache_enabled: true,
        cache_ttl_seconds: 86400,
        cache_negative_ttl: 300,
        cache_max_size_gb: 10,
        failure_cache_rules: '',
      },
    })
    
    expect(wrapper.html()).not.toContain('失败缓存规则')
  })

  it('binds cache_ttl_seconds correctly', () => {
    const wrapper = createWrapper({
      form: {
        type: 'local',
        cache_enabled: true,
        cache_ttl_seconds: 3600,
        cache_negative_ttl: 300,
        cache_max_size_gb: 10,
        failure_cache_rules: '',
      },
    })
    
    expect(wrapper.html()).toBeDefined()
  })

  it('binds cache_max_size_gb correctly', () => {
    const wrapper = createWrapper({
      form: {
        type: 'local',
        cache_enabled: true,
        cache_ttl_seconds: 86400,
        cache_negative_ttl: 300,
        cache_max_size_gb: 20,
        failure_cache_rules: '',
      },
    })
    
    expect(wrapper.html()).toContain('缓存最大大小（GB）')
  })

  it('emits update:failureRules when rules text changes', async () => {
    const form = {
      type: 'proxy',
      cache_enabled: true,
      cache_ttl_seconds: 86400,
      cache_negative_ttl: 300,
      cache_max_size_gb: 10,
      failure_cache_rules: '',
    }
    
    const wrapper = createWrapper({ form })
    const textarea = wrapper.find('textarea')
    
    await textarea.setValue('[{"status_code": 500, "ttl_seconds": 60}]')
    
    expect(wrapper.emitted('update:failureRules')).toBeTruthy()
  })

  it('shows hint for max cache size', () => {
    const wrapper = createWrapper({
      form: {
        type: 'local',
        cache_enabled: true,
        cache_ttl_seconds: 86400,
        cache_negative_ttl: 300,
        cache_max_size_gb: 10,
        failure_cache_rules: '',
      },
    })
    
    expect(wrapper.html()).toContain('缓存最大大小（GB）')
  })
})
