import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import VirtualMembersForm from '../VirtualMembersForm.vue'
import { repositoryApi } from '@/api/repository'

vi.mock('@/api/repository', () => ({
  repositoryApi: {
    list: vi.fn(),
  },
}))

const mockRepos = [
  { name: 'repo1', display_name: '仓库1', type: 'local', package_type: 'group' },
  { name: 'repo2', display_name: '仓库2', type: 'proxy', package_type: 'group' },
  { name: 'repo3', display_name: '', type: 'local', package_type: 'group' },
]

const createWrapper = (props = {}) => {
  return mount(VirtualMembersForm, {
    props: {
      membersText: '',
      packageType: 'group',
      ...props,
    },
    global: {
      plugins: [ElementPlus],
    },
  })
}

describe('VirtualMembersForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders virtual members form', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.find('.members-list').exists()).toBe(true)
  })

  it('shows info alert about member priority', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.html()).toContain('拖拽成员可调整优先级顺序，排在越前面优先级越高')
  })

  it('shows add member button', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.html()).toContain('添加成员')
  })

  it('parses members from text on init', async () => {
    const wrapper = createWrapper({
      membersText: 'repo1\nrepo2\nrepo3',
    })
    
    await wrapper.vm.$nextTick()
    
    const memberSelects = wrapper.findAll('.member-item .el-select')
    expect(memberSelects).toHaveLength(3)
  })

  it('adds new member when clicking add button', async () => {
    ;(repositoryApi.list as Mock).mockResolvedValue(mockRepos)

    const wrapper = createWrapper({
      membersText: '',
    })

    // Wait for async data load to complete
    await new Promise(resolve => setTimeout(resolve, 50))
    await wrapper.vm.$nextTick()

    const addButton = wrapper.find('button')
    await addButton.trigger('click')
    await wrapper.vm.$nextTick()

    const memberSelects = wrapper.findAll('.member-item .el-select')
    expect(memberSelects.length).toBeGreaterThan(0)
  })

  it('removes member when clicking delete button', async () => {
    const wrapper = createWrapper({
      membersText: 'repo1\nrepo2',
    })
    
    await wrapper.vm.$nextTick()
    
    const deleteButton = wrapper.find('.el-button--danger')
    await deleteButton.trigger('click')
    
    expect(wrapper.emitted('update:membersText')).toBeTruthy()
  })

  it('emits update:membersText when members change', async () => {
    ;(repositoryApi.list as Mock).mockResolvedValue(mockRepos)
    
    const wrapper = createWrapper({
      membersText: 'repo1',
    })
    
    await wrapper.vm.$nextTick()
    await new Promise(resolve => setTimeout(resolve, 10))
    
    const selectComponent = wrapper.findComponent({ name: 'ElSelect' })
    await selectComponent.vm.$emit('change', 'repo2')
    
    expect(wrapper.emitted('update:membersText')).toBeTruthy()
  })

  it('displays member index numbers', async () => {
    const wrapper = createWrapper({
      membersText: 'repo1\nrepo2',
    })
    
    await wrapper.vm.$nextTick()
    
    const indexes = wrapper.findAll('.member-index')
    expect(indexes).toHaveLength(2)
    expect(indexes[0].text()).toBe('1')
    expect(indexes[1].text()).toBe('2')
  })

  it('filters out empty member names', async () => {
    const wrapper = createWrapper({
      membersText: 'repo1\n\nrepo2',
    })
    
    await wrapper.vm.$nextTick()
    
    const memberSelects = wrapper.findAll('.member-item .el-select')
    expect(memberSelects).toHaveLength(2)
  })

  it('updates membersText when adding multiple members', async () => {
    ;(repositoryApi.list as Mock).mockResolvedValue(mockRepos)

    const wrapper = createWrapper({
      membersText: '',
    })

    // Wait for async data load to complete
    await new Promise(resolve => setTimeout(resolve, 50))
    await wrapper.vm.$nextTick()

    const addButton = wrapper.find('button')
    await addButton.trigger('click')
    await wrapper.vm.$nextTick()
    await addButton.trigger('click')
    await wrapper.vm.$nextTick()

    const selects = wrapper.findAll('.member-item .el-select')
    expect(selects.length).toBe(2)
  })

  it('loads available repos on mount', async () => {
    ;(repositoryApi.list as Mock).mockResolvedValue(mockRepos)
    
    createWrapper()
    
    expect(repositoryApi.list).toHaveBeenCalled()
  })

  it('disables add button when no repos available', async () => {
    ;(repositoryApi.list as Mock).mockResolvedValue([])
    
    const wrapper = createWrapper()
    
    await wrapper.vm.$nextTick()
    await new Promise(resolve => setTimeout(resolve, 10))
    
    const addButton = wrapper.find('button')
    expect(addButton.attributes('disabled')).toBeDefined()
  })

  it('disables add button when loading', async () => {
    ;(repositoryApi.list as Mock).mockImplementation(() => {
      return new Promise(resolve => setTimeout(() => resolve(mockRepos), 100))
    })
    
    const wrapper = createWrapper()
    
    const addButton = wrapper.find('button')
    expect(addButton.attributes('disabled')).toBeDefined()
  })
})
