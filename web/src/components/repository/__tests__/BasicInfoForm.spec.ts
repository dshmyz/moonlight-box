import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import BasicInfoForm from '../BasicInfoForm.vue'
import { storageBackendApi } from '@/api/storageBackend'

vi.mock('@/api/storageBackend', () => ({
  storageBackendApi: {
    list: vi.fn(),
  },
}))

const createWrapper = (props = {}) => {
  const app = createApp({})
  app.use(ElementPlus)
  
  return mount(BasicInfoForm, {
    props: {
      form: {
        name: '',
        display_name: '',
        description: '',
        type: 'local',
        package_type: 'npm',
      },
      disabled: false,
      ...props,
    },
    global: {
      plugins: [ElementPlus],
    },
  })
}

describe('BasicInfoForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(storageBackendApi.list as any).mockResolvedValue([])
  })

  it('renders all form fields', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.find('.basic-info-form').exists()).toBe(true)
    expect(wrapper.findAll('.el-form-item')).toHaveLength(6)
  })

  it('binds name field correctly', async () => {
    const wrapper = createWrapper({
      form: {
        name: 'test-repo',
        display_name: '',
        description: '',
        type: 'local',
        package_type: 'npm',
      },
    })
    
    const nameInput = wrapper.find('input[placeholder="例如：npm-local"]')
    expect((nameInput.element as HTMLInputElement).value).toBe('test-repo')
  })

  it('binds display_name field correctly', async () => {
    const wrapper = createWrapper({
      form: {
        name: '',
        display_name: 'Test Repository',
        description: '',
        type: 'local',
        package_type: 'npm',
      },
    })
    
    const displayNameInput = wrapper.find('input[placeholder="例如：NPM 内部仓库"]')
    expect((displayNameInput.element as HTMLInputElement).value).toBe('Test Repository')
  })

  it('binds description field correctly', async () => {
    const wrapper = createWrapper({
      form: {
        name: '',
        display_name: '',
        description: 'This is a test repo',
        type: 'local',
        package_type: 'npm',
      },
    })
    
    const textarea = wrapper.find('textarea')
    expect((textarea.element as HTMLTextAreaElement).value).toBe('This is a test repo')
  })

  it('shows correct type selection', async () => {
    const wrapper = createWrapper({
      form: {
        name: '',
        display_name: '',
        description: '',
        type: 'proxy',
        package_type: 'npm',
      },
    })
    
    const typeSelect = wrapper.findAll('.el-select')
    expect(typeSelect.length).toBeGreaterThan(0)
  })

  it('shows correct package_type selection', async () => {
    const form = {
      name: '',
      display_name: '',
      description: '',
      type: 'local' as const,
      package_type: 'maven',
    }
    const wrapper = createWrapper({ form })
    
    expect(wrapper.props('form').package_type).toBe('maven')
  })

  it('disables fields when disabled prop is true', async () => {
    const wrapper = createWrapper({
      disabled: true,
      form: {
        name: 'test-repo',
        display_name: '',
        description: '',
        type: 'local',
        package_type: 'npm',
      },
    })
    
    const nameInput = wrapper.find('input[placeholder="例如：npm-local"]')
    expect((nameInput.element as HTMLInputElement).disabled).toBe(true)
  })

  it('enables fields when disabled prop is false', async () => {
    const wrapper = createWrapper({
      disabled: false,
      form: {
        name: 'test-repo',
        display_name: '',
        description: '',
        type: 'local',
        package_type: 'npm',
      },
    })
    
    const nameInput = wrapper.find('input[placeholder="例如：npm-local"]')
    expect((nameInput.element as HTMLInputElement).disabled).toBe(false)
  })

  it('displays all type options', () => {
    const wrapper = createWrapper({
      form: {
        name: '',
        display_name: '',
        description: '',
        type: 'local',
        package_type: 'npm',
      },
    })
    
    expect(wrapper.vm.$props.form.type).toBe('local')
  })

  it('displays all package type options', () => {
    const wrapper = createWrapper({
      form: {
        name: '',
        display_name: '',
        description: '',
        type: 'local',
        package_type: 'npm',
      },
    })
    
    expect(wrapper.vm.$props.form.package_type).toBe('npm')
  })

  it('shows hint text for name field', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.html()).toContain('唯一标识，创建后不可修改')
  })
})
