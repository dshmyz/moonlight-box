import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import VersionTable from './VersionTable.vue'
import type { PackageVersion } from '@/api/package'

const versions: PackageVersion[] = [
  {
    id: 1,
    package_id: 1,
    version: '1.0.0',
    status: 'published',
    storage_path: '',
    published_at: '2026-06-12T00:00:00Z',
    published_by: 1,
    download_count: 0,
    files_downloaded: true,
    files: [
      {
        id: 10,
        version_id: 1,
        filename: 'lib-1.0.0.jar',
        file_type: 'primary' as const,
        remote_path: 'com/example/lib/1.0.0/lib-1.0.0.jar',
        size_bytes: 123,
        download_count: 0,
        download_url: '/repository/maven-proxy/com/example/lib/1.0.0/lib-1.0.0.jar',
      },
    ],
  },
]

const tableStub = defineComponent({
  props: ['data'],
  setup(props, { slots }) {
    return () => h('div', (props.data || []).map((row: unknown) =>
      h('div', { class: 'test-row' }, (slots.default?.() || []).map((vnode) =>
        h(vnode.type as any, { ...(vnode.props || {}), row }, vnode.children as any)
      ))
    ))
  },
})

const tableColumnStub = defineComponent({
  props: ['row'],
  setup(props, { slots }) {
    return () => h('div', slots.default?.({ row: props.row }))
  },
})

function mountVersionTable(inputVersions = versions) {
  return mount(VersionTable, {
    props: {
      versions: inputVersions,
      selectedVersion: '1.0.0',
    },
    global: {
      stubs: {
        ElCard: { template: '<div><slot name="header" /><slot /></div>' },
        ElTable: tableStub,
        ElTableColumn: tableColumnStub,
        ElRadioGroup: { template: '<div><slot /></div>' },
        ElRadioButton: { template: '<button><slot /></button>' },
        ElTag: { template: '<span><slot /></span>' },
        ElTooltip: { template: '<span><slot /></span>' },
        ElIcon: { template: '<span><slot /></span>' },
        ElButton: { template: '<button><slot /></button>' },
        ElPagination: { template: '<div />' },
      },
    },
  })
}

describe('VersionTable', () => {
  it('renders package files as direct download links', () => {
    const wrapper = mountVersionTable()

    const link = wrapper.get('a.file-download-link')
    expect(link.text()).toContain('lib-1.0.0.jar')
    expect(link.attributes('href')).toBe('/repository/maven-proxy/com/example/lib/1.0.0/lib-1.0.0.jar')
    expect(link.attributes('download')).toBe('lib-1.0.0.jar')
    expect(wrapper.emitted('download')).toBeUndefined()
  })

  it('defaults to files marked default_visible when present', () => {
    const wrapper = mountVersionTable([
      {
        ...versions[0],
        files: [
          {
            id: 10,
            version_id: 1,
            filename: 'lib-current.jar',
            file_type: 'primary' as const,
            remote_path: 'lib-current.jar',
            size_bytes: 123,
            download_count: 0,
            download_url: '/repository/test/lib-current.jar',
            attributes: { default_visible: 'true' },
          },
          {
            id: 11,
            version_id: 1,
            filename: 'lib-old.jar',
            file_type: 'primary' as const,
            remote_path: 'lib-old.jar',
            size_bytes: 123,
            download_count: 0,
            download_url: '/repository/test/lib-old.jar',
          },
        ],
      },
    ])

    expect(wrapper.text()).toContain('lib-current.jar')
    expect(wrapper.text()).not.toContain('lib-old.jar')
    expect(wrapper.text()).toContain('更多文件')
  })

  it('shows hidden files after clicking more files', async () => {
    const wrapper = mountVersionTable([
      {
        ...versions[0],
        files: [
          {
            id: 10,
            version_id: 1,
            filename: 'lib-current.jar',
            file_type: 'primary' as const,
            remote_path: 'lib-current.jar',
            size_bytes: 123,
            download_count: 0,
            download_url: '/repository/test/lib-current.jar',
            attributes: { default_visible: 'true' },
          },
          {
            id: 11,
            version_id: 1,
            filename: 'lib-old.jar',
            file_type: 'primary' as const,
            remote_path: 'lib-old.jar',
            size_bytes: 123,
            download_count: 0,
            download_url: '/repository/test/lib-old.jar',
          },
        ],
      },
    ])

    await wrapper.get('.more-files-hint').trigger('click')

    expect(wrapper.text()).toContain('lib-current.jar')
    expect(wrapper.text()).toContain('lib-old.jar')
  })
})
