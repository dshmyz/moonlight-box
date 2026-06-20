import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { permission } from '@/directives/permission'
import { useAuthStore } from '@/stores/auth'
import PackageTable from './PackageTable.vue'

vi.mock('@/api/package', () => ({
  packageApi: {
    deletePackage: vi.fn(),
  },
}))

vi.mock('@/utils/clipboard', () => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))

const samplePackages = [
  { id: 1, name: 'lodash', display_name: 'lodash', type: 'npm', description: 'Utility library', repository_type: 'proxy', versions_count: 3, download_count: 100, updated_at: '2026-06-19T00:00:00Z' },
  { id: 2, name: 'react', display_name: 'react', type: 'npm', description: 'UI library', repository_type: 'local', versions_count: 5, download_count: 500, updated_at: '2026-06-18T00:00:00Z' },
]

// ElTable stub：在 jsdom 中真实 el-table 无法渲染行内容（依赖浏览器 layout API），
// 用 stub 模拟行渲染，保留测试需要的 DOM 结构与事件
const ElTableStub = defineComponent({
  name: 'ElTable',
  props: {
    data: { type: Array, default: () => [] },
    size: { type: String, default: 'default' },
  },
  emits: ['selection-change'],
  setup(props, { slots }) {
    return () => {
      // 收集 el-table-column 子节点
      const children = slots.default?.() || []
      const columns = children
        .filter((v: any) => v && v.type && (v.type as any).name === 'ElTableColumnStub' || (v.type as any).__isTableColumn)
        .map((v: any) => v.props || {})

      // 表头
      const headerCells = columns.map((col: any) => {
        if (col.type === 'selection') {
          return h('th', { class: 'el-table-column--selection' }, '')
        }
        return h('th', {}, col.label || '')
      })

      // 行
      const rows = (props.data || []).map((row: any) => {
        const cells = children.map((vnode: any) => {
          const colProps = vnode.props || {}
          if (colProps.type === 'selection') {
            return h('td', { class: 'el-table-column--selection' })
          }
          // 调用列的 default slot 渲染单元格
          const slotFn = (vnode.children as any)?.default
          const content = slotFn ? slotFn({ row }) : (row[colProps.prop] ?? '')
          return h('td', {}, content)
        })
        return h('tr', { class: 'el-table__row' }, cells)
      })

      return h('div', { class: 'el-table' }, [
        h('div', { class: 'el-table__header-wrapper' }, [
          h('table', { class: 'el-table__header' }, [
            h('thead', [h('tr', headerCells)]),
          ]),
        ]),
        h('div', { class: 'el-table__body-wrapper' }, [
          h('table', { class: 'el-table__body' }, [
            h('tbody', rows),
          ]),
        ]),
      ])
    }
  },
})

const ElTableColumnStub = defineComponent({
  name: 'ElTableColumn',
  props: {
    type: String,
    label: String,
    prop: String,
    width: [String, Number],
    align: String,
    fixed: String,
  },
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const mountIt = (props: Record<string, any> = {}) => {
  const pinia = createPinia()
  setActivePinia(pinia)
  // 注入 admin 用户，使 v-permission 对 package:delete 放行
  const store = useAuthStore()
  // @ts-expect-error 测试直接注入 admin user
  store.user = { roles: ['admin'], permissions: [] }

  return mount(PackageTable, {
    props: {
      packages: samplePackages,
      loading: false,
      mode: 'admin',
      density: 'default',
      selectedIds: [],
      columns: {},
      ...props,
    } as any,
    global: {
      plugins: [ElementPlus, pinia],
      directives: { permission },
      stubs: {
        ElTable: ElTableStub,
        ElTableColumn: ElTableColumnStub,
      },
    },
  })
}

describe('PackageTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('渲染包列表', () => {
    const wrapper = mountIt()
    expect(wrapper.findAll('.el-table__row')).toHaveLength(2)
    expect(wrapper.text()).toContain('lodash')
    expect(wrapper.text()).toContain('react')
  })

  it('admin 模式显示批量选择列', () => {
    const wrapper = mountIt({ mode: 'admin' })
    expect(wrapper.find('.el-table-column--selection').exists()).toBe(true)
  })

  it('public 模式不显示批量选择列和操作列的删除按钮', () => {
    const wrapper = mountIt({ mode: 'public' })
    expect(wrapper.find('.el-table-column--selection').exists()).toBe(false)
    expect(wrapper.find('.btn-delete').exists()).toBe(false)
  })

  it('勾选行触发 update:selectedIds', async () => {
    const wrapper = mountIt({ mode: 'admin' })
    // el-table 的 selection-change 事件
    await wrapper.findComponent({ name: 'ElTable' }).vm.$emit('selection-change', [samplePackages[0]])
    expect(wrapper.emitted('update:selectedIds')?.[0]).toEqual([[1]])
  })

  it('点击包名复制按钮触发复制', async () => {
    const { copyToClipboard } = await import('@/utils/clipboard')
    const wrapper = mountIt({ mode: 'admin' })
    await wrapper.find('.copy-name-btn').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('npm:lodash')
  })

  it('点击查看版本触发 view-versions 事件', async () => {
    const wrapper = mountIt({ mode: 'admin' })
    await wrapper.find('.btn-view-versions').trigger('click')
    expect(wrapper.emitted('view-versions')?.[0]).toEqual([samplePackages[0]])
  })

  it('点击详情触发 view-detail 事件', async () => {
    const wrapper = mountIt({ mode: 'admin' })
    await wrapper.find('.btn-view-detail').trigger('click')
    expect(wrapper.emitted('view-detail')?.[0]).toEqual([samplePackages[0]])
  })

  it('点击删除触发 delete-package 事件', async () => {
    const wrapper = mountIt({ mode: 'admin' })
    await wrapper.find('.btn-delete').trigger('click')
    expect(wrapper.emitted('delete-package')?.[0]).toEqual([samplePackages[0]])
  })

  it('columns 配置控制列显隐', () => {
    const wrapper = mountIt({
      columns: { description: false, source: false, versions: false, downloads: false, updatedAt: false },
    })
    // 隐藏的列不应渲染表头
    const headers = wrapper.findAll('.el-table__header th')
    const headerTexts = headers.map(h => h.text())
    expect(headerTexts.some(t => t.includes('描述'))).toBe(false)
  })

  it('density 传给 el-table 的 size', () => {
    const wrapper = mountIt({ density: 'small' })
    expect(wrapper.findComponent({ name: 'ElTable' }).props('size')).toBe('small')
  })
})
