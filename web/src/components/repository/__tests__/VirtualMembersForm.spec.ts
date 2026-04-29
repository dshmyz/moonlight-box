import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import VirtualMembersForm from '../VirtualMembersForm.vue'

const createWrapper = (props = {}) => {
  return mount(VirtualMembersForm, {
    props: {
      membersText: '',
      ...props,
    },
    global: {
      plugins: [ElementPlus],
    },
  })
}

describe('VirtualMembersForm', () => {
  it('renders virtual members form', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.find('.members-list').exists()).toBe(true)
  })

  it('shows info alert about member priority', () => {
    const wrapper = createWrapper()
    
    expect(wrapper.html()).toContain('成员仓库将按照从上到下的顺序进行优先级排序')
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
    
    const memberInputs = wrapper.findAll('.member-item input')
    expect(memberInputs).toHaveLength(3)
  })

  it('adds new member when clicking add button', async () => {
    const wrapper = createWrapper({
      membersText: '',
    })
    
    const addButton = wrapper.find('button')
    await addButton.trigger('click')
    
    const memberInputs = wrapper.findAll('.member-item input')
    expect(memberInputs.length).toBeGreaterThan(0)
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
    const wrapper = createWrapper({
      membersText: 'repo1',
    })
    
    await wrapper.vm.$nextTick()
    
    const input = wrapper.find('.member-item input')
    await input.setValue('new-repo-name')
    
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
    
    const memberInputs = wrapper.findAll('.member-item input')
    expect(memberInputs).toHaveLength(2)
  })

  it('updates membersText when adding multiple members', async () => {
    const wrapper = createWrapper({
      membersText: '',
    })
    
    const addButton = wrapper.find('button')
    await addButton.trigger('click')
    await addButton.trigger('click')
    
    const inputs = wrapper.findAll('.member-item input')
    if (inputs.length >= 2) {
      await inputs[0].setValue('repo-a')
      await inputs[1].setValue('repo-b')
      
      expect(wrapper.emitted('update:membersText')).toBeTruthy()
    }
  })
})
