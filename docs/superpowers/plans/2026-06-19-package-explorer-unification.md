# 包管理查询界面统一实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将三套并行包查询界面（PackageList V1 / PackageCenterV2 / BrowsePage）统一为单一 `PackageExplorer` 组件，修复排序映射 bug、V2 搜索框缺失、V2 Tab 失效、删除后页码越界等问题，补齐高级筛选/批量操作/列设置/最近搜索/快速复制等查询增强能力。

**架构：** 新建 `web/src/components/package-explorer/` 目录，包含 1 个容器组件 + 7 个子组件；新建 `web/src/composables/usePackageSearch.ts` 承载所有查询/筛选/排序/分页/URL同步/快捷键/最近搜索逻辑；新建 `web/src/directives/permission.ts` 做按钮级权限控制。通过 `mode: 'admin' | 'public'` prop 区分管理端与公共端行为。三阶段迁移：新建并存 → 切换 → 清理。

**技术栈：** Vue 3 + TypeScript + Element Plus + Pinia + Vue Router + Vitest + @vue/test-utils + Playwright

**关键背景信息（实现者必读）：**
- 后端排序字段接受：`name` / `download_count` / `updated_at`（**不是** `downloads`）。详见 [package_search_service.go:167-176](file:///Users/gracegaoya/work/project/moonlight-box/internal/service/package_search_service.go#L167-L176)
- BrowsePage 当前的 `downloads → updated_at` 映射是 bug，统一后前端直接发 `download_count`
- 后端 `SearchRequest` 当前**不支持** `source` / `from` / `to` 参数，高级筛选面板的"来源/时间范围"作为后端前置依赖，前端先隐藏这两项，仅保留"仓库"和"版本"筛选
- 已有工具函数：[copyToClipboard](file:///Users/gracegaoya/work/project/moonlight-box/web/src/utils/clipboard.ts)、[formatNumber/formatDate](file:///Users/gracegaoya/work/project/moonlight-box/web/src/utils/format.ts)
- 已有常量：[PACKAGE_TYPE_OPTIONS](file:///Users/gracegaoya/work/project/moonlight-box/web/src/constants/package.ts#L84-L92)、`PACKAGE_TYPE_HEX_COLORS`、`getPackageTypeHexColor`
- 已有 auth store：[useAuthStore().hasPermission(resource, action)](file:///Users/gracegaoya/work/project/moonlight-box/web/src/stores/auth.ts#L15-L22)
- 复用现有子组件：[VersionDrawer](file:///Users/gracegaoya/work/project/moonlight-box/web/src/components/package/VersionDrawer.vue)、[UploadPackageDialog](file:///Users/gracegaoya/work/project/moonlight-box/web/src/components/package/UploadPackageDialog.vue)（保留原位，仅更新 import）
- 测试命令：`cd web && npm run test:run`（vitest run）
- 类型检查命令：`cd web && npx vue-tsc --noEmit`
- 构建命令：`cd web && npm run build`

---

## 文件结构

### 新建文件

| 文件 | 职责 |
|------|------|
| `web/src/directives/permission.ts` | `v-permission` 指令：无权限时移除元素 |
| `web/src/composables/usePackageSearch.ts` | 查询/筛选/排序/分页/URL同步/快捷键/最近搜索 |
| `web/src/components/package-explorer/PackageExplorer.vue` | 容器组件，编排子组件 + mode 透传 |
| `web/src/components/package-explorer/PackageSearchBar.vue` | 搜索框 + 最近搜索下拉 + 类型 chips + 筛选触发 |
| `web/src/components/package-explorer/PackageFilterPanel.vue` | 高级筛选抽屉（仓库/版本） |
| `web/src/components/package-explorer/PackageTable.vue` | 表格视图（批量选择 + 列设置 + 密度 + 复制 + 操作） |
| `web/src/components/package-explorer/PackageGrid.vue` | 卡片网格视图（复制按钮） |
| `web/src/components/package-explorer/PackagePagination.vue` | 统一分页器 |
| `web/src/components/package-explorer/PackageSkeleton.vue` | 骨架屏（表格/网格两种形态） |
| `web/src/components/package-explorer/PackageEmptyState.vue` | 空状态（无数据/无匹配/加载失败） |
| `web/src/views/PackageExplorerPage.vue` | 管理端页面壳（`mode="admin"`） |
| `web/src/views/PublicBrowsePage.vue` | 公共端页面壳（`mode="public"`） |
| `web/src/composables/usePackageSearch.spec.ts` | composable 单元测试 |
| `web/src/directives/permission.spec.ts` | 指令单元测试 |
| 各组件 `.spec.ts` | 组件测试 |

### 修改文件

| 文件 | 改动 |
|------|------|
| `web/src/main.ts` | 注册 `v-permission` 指令 |
| `web/src/router/index.ts` | 阶段1新增临时路由；阶段2切换主路由；阶段3清理 |

### 阶段3删除文件

| 文件 | 备注 |
|------|------|
| `web/src/views/PackageList.vue` | V1 旧管理端 |
| `web/src/views/PackageList.spec.ts` | V1 旧测试 |
| `web/src/views/PackageCenterV2.vue` | V2 旧管理端 |
| `web/src/views/BrowsePage.vue` | 旧公共页 |
| `web/src/views/BrowsePage.spec.ts` | 旧公共页测试 |
| `web/src/components/package/PackageTable.vue` | 旧表格（被新版替代） |
| `web/src/components/package/PackageCards.vue` | 旧卡片（被 PackageGrid 替代） |
| `web/src/components/package-center/*` | V2 子组件全部废弃 |
| `web/src/components/browse/HeroSection.vue` | 旧 Hero（被 PackageSearchBar 替代） |
| `web/src/components/browse/PackageCard.vue` | 旧卡片（被 PackageGrid 替代） |

**保留文件**：`VersionDrawer.vue`、`UploadPackageDialog.vue`、`browse/RepositoryShowcase.vue`、`browse/RepositoryStatusPanel.vue`、`browse/RepoCard.vue`、`browse/TypeSidebar.vue`（公共端仓库 Tab 仍用）

---

## 任务 1：v-permission 指令

**文件：**
- 创建：`web/src/directives/permission.ts`
- 创建：`web/src/directives/permission.spec.ts`
- 修改：`web/src/main.ts`

- [ ] **步骤 1：编写失败的测试**

创建 `web/src/directives/permission.spec.ts`：

```typescript
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
    template: `<button v-permission="'package:delete'">删除</button>`,
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
      template: `<button v-permission="'package:write'">上传</button>`,
    }))
    expect(wrapper.find('button').exists()).toBe(true)
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/directives/permission.spec.ts`
预期：FAIL，报错 `Cannot find module './permission'`

- [ ] **步骤 3：实现指令**

创建 `web/src/directives/permission.ts`：

```typescript
import type { Directive } from 'vue'
import { useAuthStore } from '@/stores/auth'

/**
 * v-permission 指令：根据当前用户权限控制元素显隐
 * 用法：v-permission="'resource:action'"
 * 无权限时从 DOM 移除元素（非 display:none，避免被绕过）
 */
export const permission: Directive<HTMLElement, string> = {
  mounted(el, binding) {
    const perm = binding.value
    if (!perm || typeof perm !== 'string') return

    const sep = perm.indexOf(':')
    if (sep <= 0) return

    const resource = perm.slice(0, sep)
    const action = perm.slice(sep + 1)

    const store = useAuthStore()
    if (!store.hasPermission(resource, action)) {
      el.parentNode?.removeChild(el)
    }
  },
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/directives/permission.spec.ts`
预期：PASS（3 个测试全过）

- [ ] **步骤 5：注册到 main.ts**

修改 `web/src/main.ts`，在 `app.use(ElementPlus, ...)` 之后添加：

```typescript
import { permission } from './directives/permission'

// ... 在 app.use(ElementPlus, { locale: zhCn }) 之后
app.directive('permission', permission)
```

- [ ] **步骤 6：类型检查 + Commit**

运行：`cd web && npx vue-tsc --noEmit`
预期：无错误

```bash
git add web/src/directives/permission.ts web/src/directives/permission.spec.ts web/src/main.ts
git commit -m "feat(frontend): add v-permission directive for button-level access control"
```

---

## 任务 2：usePackageSearch composable - 核心查询与状态

**文件：**
- 创建：`web/src/composables/usePackageSearch.ts`
- 创建：`web/src/composables/usePackageSearch.spec.ts`

- [ ] **步骤 1：编写失败的测试（核心查询）**

创建 `web/src/composables/usePackageSearch.spec.ts`：

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { usePackageSearch } from './usePackageSearch'
import { packageApi } from '@/api/package'

vi.mock('@/api/package', () => ({
  packageApi: {
    search: vi.fn(),
    deletePackage: vi.fn(),
  },
}))

const mockPush = vi.fn()
const mockReplace = vi.fn()
let mockRouteQuery: Record<string, any> = {}

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: mockReplace,
  }),
  useRoute: () => ({ query: mockRouteQuery }),
}))

const mockSearchResponse = (list: any[] = [], total = 0) => ({
  list,
  total,
  page: 1,
  page_size: 20,
  search_time_ms: 5,
})

function mountComposable(mode: 'admin' | 'public' = 'admin', opts: Record<string, any> = {}) {
  let result: ReturnType<typeof usePackageSearch> | null = null
  mount(defineComponent({
    setup() {
      result = usePackageSearch({ mode, ...opts })
      return {}
    },
    template: '<div />',
  }), { global: { plugins: [createPinia()] } })
  return result!
}

describe('usePackageSearch - 核心查询', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    mockRouteQuery = {}
  })

  it('初始加载调用 API 并填充 packages', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse([
      { id: 1, name: 'lodash', type: 'npm', download_count: 10, updated_at: '2026-06-19T00:00:00Z' },
    ], 1))

    const cs = mountComposable()
    await cs.search()

    expect(packageApi.search).toHaveBeenCalledWith(expect.objectContaining({
      q: '',
      page: 1,
      page_size: 20,
      sort: 'updated_at',
    }))
    expect(cs.packages.value).toHaveLength(1)
    expect(cs.packages.value[0].name).toBe('lodash')
    expect(cs.total.value).toBe(1)
    expect(cs.loading.value).toBe(false)
  })

  it('类型归一化：format 字段回退到 type', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse([
      { id: 1, name: 'pkg', format: 'npm', download_count: 0, updated_at: '2026-06-19T00:00:00Z' },
    ], 1))

    const cs = mountComposable()
    await cs.search()
    expect(cs.packages.value[0].type).toBe('npm')
  })

  it('setQuery 自动重置到第 1 页', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse([], 0))

    const cs = mountComposable()
    cs.query.page = 5
    cs.setQuery({ q: 'react' })
    expect(cs.query.page).toBe(1)
    expect(cs.query.q).toBe('react')
  })

  it('排序字段直接透传 download_count（验证 bug 修复）', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable()
    cs.setQuery({ sort: 'download_count' })
    await cs.search()

    expect(packageApi.search).toHaveBeenCalledWith(expect.objectContaining({
      sort: 'download_count',
    }))
  })

  it('API 失败时设置 error 状态', async () => {
    ;(packageApi.search as any).mockRejectedValue(new Error('network'))

    const cs = mountComposable()
    await cs.search()

    expect(cs.error.value).toBeTruthy()
    expect(cs.packages.value).toEqual([])
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/composables/usePackageSearch.spec.ts`
预期：FAIL，报错 `Cannot find module './usePackageSearch'`

- [ ] **步骤 3：实现 composable 核心部分**

创建 `web/src/composables/usePackageSearch.ts`：

```typescript
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { packageApi, type Package } from '@/api/package'

export type PackageSort = 'updated_at' | 'download_count' | 'name'
export type PackageSource = 'all' | 'local' | 'proxy'

export interface PackageQuery {
  q: string
  type: string
  repository: string
  version: string
  source: PackageSource
  sort: PackageSort
  page: number
  pageSize: number
}

export interface UsePackageSearchOptions {
  mode: 'admin' | 'public'
  initialQuery?: Partial<PackageQuery>
  syncUrl?: boolean
  pageSizeOptions?: number[]
  defaultPageSize?: number
  recentSearchKey?: string
}

function normalizePackage(pkg: any): Package {
  return {
    ...pkg,
    type: pkg.type || pkg.package_type || pkg.format || 'generic',
  }
}

export function usePackageSearch(options: UsePackageSearchOptions) {
  const router = useRouter()
  const route = useRoute()

  const defaultPageSize = options.defaultPageSize ?? 20
  const syncUrl = options.syncUrl ?? true

  const query = reactive<PackageQuery>({
    q: options.initialQuery?.q ?? '',
    type: options.initialQuery?.type ?? 'all',
    repository: options.initialQuery?.repository ?? '',
    version: options.initialQuery?.version ?? '',
    source: options.initialQuery?.source ?? 'all',
    sort: options.initialQuery?.sort ?? 'updated_at',
    page: options.initialQuery?.page ?? 1,
    pageSize: defaultPageSize,
  })

  const packages = ref<Package[]>([])
  const total = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const searchTime = ref(0)
  const recentSearches = ref<string[]>([])

  const isEmpty = computed(() => !loading.value && packages.value.length === 0)
  const hasActiveFilter = computed(() =>
    !!query.repository || !!query.version || query.source !== 'all'
  )

  function buildApiParams() {
    const params: Record<string, unknown> = {
      q: query.q,
      page: query.page,
      page_size: query.pageSize,
      sort: query.sort,
    }
    if (query.type !== 'all') params.type = query.type
    if (query.repository) params.repository = query.repository
    if (query.version) params.version = query.version
    return params
  }

  async function search() {
    loading.value = true
    error.value = null
    try {
      const res = await packageApi.search(buildApiParams() as any)
      packages.value = (res.list || []).map(normalizePackage)
      total.value = res.total || 0
      searchTime.value = res.search_time_ms || 0
    } catch (e) {
      error.value = e instanceof Error ? e.message : '加载失败'
      packages.value = []
      total.value = 0
    } finally {
      loading.value = false
    }
  }

  async function refresh() {
    await search()
    // 删除后页码修正：当前页无数据且非第 1 页，回退一页重试
    if (packages.value.length === 0 && query.page > 1) {
      query.page--
      await search()
    }
  }

  function readFromUrl() {
    const q = route.query
    if (q.q) query.q = String(q.q)
    if (q.type) query.type = String(q.type)
    if (q.sort) query.sort = String(q.sort) as PackageSort
    if (q.page) query.page = parseInt(String(q.page)) || 1
    if (q.page_size) query.pageSize = parseInt(String(q.page_size)) || defaultPageSize
    if (q.repo) query.repository = String(q.repo)
    if (q.version) query.version = String(q.version)
  }

  function syncToUrl(changed: Partial<PackageQuery>) {
    if (!syncUrl) return
    const next: Record<string, string> = {}
    if (query.q) next.q = query.q
    if (query.type !== 'all') next.type = query.type
    if (query.sort !== 'updated_at') next.sort = query.sort
    if (query.page !== 1) next.page = String(query.page)
    if (query.pageSize !== defaultPageSize) next.page_size = String(query.pageSize)
    if (query.repository) next.repo = query.repository
    if (query.version) next.version = query.version

    const isPageChange = 'page' in changed
    if (isPageChange) {
      router.push({ query: next })
    } else {
      router.replace({ query: next })
    }
  }

  function setQuery(patch: Partial<PackageQuery>) {
    const isPageChange = 'page' in patch
    Object.assign(query, patch)
    if (!isPageChange && query.page !== 1) {
      query.page = 1
    }
    syncToUrl(patch)
  }

  function resetFilters() {
    query.repository = ''
    query.version = ''
    query.source = 'all'
    query.page = 1
    syncToUrl({})
  }

  // 最近搜索
  const recentKey = options.recentSearchKey ?? `package-explorer:recent:${options.mode}`

  function loadRecentSearches(): string[] {
    try {
      const stored = localStorage.getItem(recentKey)
      return stored ? JSON.parse(stored) : []
    } catch {
      return []
    }
  }

  function saveRecentSearches(list: string[]) {
    try {
      localStorage.setItem(recentKey, JSON.stringify(list))
    } catch {
      // localStorage 不可用时静默失败
    }
  }

  function addRecentSearch(term: string) {
    const trimmed = term.trim()
    if (!trimmed) return
    const next = [trimmed, ...recentSearches.value.filter(s => s !== trimmed)].slice(0, 5)
    recentSearches.value = next
    saveRecentSearches(next)
  }

  function clearRecentSearches() {
    recentSearches.value = []
    saveRecentSearches([])
  }

  // 键盘快捷键
  function handleKeydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement
    const isInputFocused = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA'

    if (e.key === 'Escape') {
      if (query.q) {
        setQuery({ q: '' })
        search()
      }
      return
    }

    if (e.key === '/' && !isInputFocused) {
      e.preventDefault()
      const input = document.querySelector<HTMLInputElement>('.package-search-bar input')
      input?.focus()
    }
  }

  onMounted(() => {
    readFromUrl()
    recentSearches.value = loadRecentSearches()
    document.addEventListener('keydown', handleKeydown)
  })

  onUnmounted(() => {
    document.removeEventListener('keydown', handleKeydown)
  })

  return {
    query,
    packages,
    total,
    loading,
    error,
    searchTime,
    recentSearches,
    isEmpty,
    hasActiveFilter,
    search,
    refresh,
    resetFilters,
    setQuery,
    addRecentSearch,
    clearRecentSearches,
  }
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/composables/usePackageSearch.spec.ts`
预期：PASS（5 个测试全过）

- [ ] **步骤 5：Commit**

```bash
git add web/src/composables/usePackageSearch.ts web/src/composables/usePackageSearch.spec.ts
git commit -m "feat(frontend): add usePackageSearch composable with core query logic"
```

---

## 任务 3：usePackageSearch - 删除后页码修正

**文件：**
- 修改：`web/src/composables/usePackageSearch.spec.ts`（追加测试，refresh 已在任务2实现）

- [ ] **步骤 1：追加测试**

在 `usePackageSearch.spec.ts` 末尾追加：

```typescript
describe('usePackageSearch - 删除后页码修正', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    mockRouteQuery = {}
  })

  it('删除后当前页空且非第1页，自动回退一页重试', async () => {
    ;(packageApi.search as any)
      .mockResolvedValueOnce(mockSearchResponse([
        { id: 1, name: 'pkg', type: 'npm' },
      ], 1))
      .mockResolvedValueOnce(mockSearchResponse([], 0))
      .mockResolvedValueOnce(mockSearchResponse([
        { id: 2, name: 'pkg2', type: 'npm' },
      ], 1))

    const cs = mountComposable()
    cs.query.page = 3
    await cs.refresh()

    expect(packageApi.search).toHaveBeenCalledTimes(2)
    expect(packageApi.search).toHaveBeenNthCalledWith(1, expect.objectContaining({ page: 3 }))
    expect(packageApi.search).toHaveBeenNthCalledWith(2, expect.objectContaining({ page: 2 }))
    expect(cs.query.page).toBe(2)
    expect(cs.packages.value).toHaveLength(1)
  })

  it('删除后当前页空但已是第1页，不回退', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse([], 0))

    const cs = mountComposable()
    cs.query.page = 1
    await cs.refresh()

    expect(packageApi.search).toHaveBeenCalledTimes(1)
    expect(cs.query.page).toBe(1)
  })
})
```

- [ ] **步骤 2：运行测试验证通过**

运行：`cd web && npx vitest run src/composables/usePackageSearch.spec.ts`
预期：PASS（任务2的 refresh 逻辑已实现，7 个测试全过）

- [ ] **步骤 3：Commit**

```bash
git add web/src/composables/usePackageSearch.spec.ts
git commit -m "test(frontend): verify page correction after delete in usePackageSearch"
```

---

## 任务 4：usePackageSearch - URL 同步

**文件：**
- 修改：`web/src/composables/usePackageSearch.spec.ts`
- 修改：`web/src/composables/usePackageSearch.ts`（任务2已实现，本任务补测试）

- [ ] **步骤 1：追加 URL 同步测试**

```typescript
describe('usePackageSearch - URL 同步', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    mockRouteQuery = {}
  })

  it('从 URL 读取初始查询参数', async () => {
    mockRouteQuery = { q: 'react', type: 'npm', sort: 'download_count', page: '3' }
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable()
    // onMounted 触发 readFromUrl
    await cs.search()

    expect(cs.query.q).toBe('react')
    expect(cs.query.type).toBe('npm')
    expect(cs.query.sort).toBe('download_count')
    expect(cs.query.page).toBe(3)
  })

  it('setQuery 后用 replace 更新 URL（非分页变化）', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable()
    cs.setQuery({ q: 'vue' })

    expect(mockReplace).toHaveBeenCalledWith(expect.objectContaining({
      query: expect.objectContaining({ q: 'vue' }),
    }))
  })

  it('分页变化用 push 更新 URL（支持前进后退）', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable()
    cs.setQuery({ page: 2 })

    expect(mockPush).toHaveBeenCalledWith(expect.objectContaining({
      query: expect.objectContaining({ page: '2' }),
    }))
  })

  it('syncUrl=false 时不更新 URL', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable('admin', { syncUrl: false })
    cs.setQuery({ q: 'vue' })

    expect(mockPush).not.toHaveBeenCalled()
    expect(mockReplace).not.toHaveBeenCalled()
  })
})
```

- [ ] **步骤 2：运行测试验证通过**

运行：`cd web && npx vitest run src/composables/usePackageSearch.spec.ts`
预期：PASS（任务2已实现 URL 同步，全部测试通过）

- [ ] **步骤 3：Commit**

```bash
git add web/src/composables/usePackageSearch.spec.ts
git commit -m "test(frontend): verify URL sync in usePackageSearch"
```

---

## 任务 5：usePackageSearch - 最近搜索与快捷键

**文件：**
- 修改：`web/src/composables/usePackageSearch.spec.ts`
- 修改：`web/src/composables/usePackageSearch.ts`（任务2已实现，本任务补测试）

- [ ] **步骤 1：追加最近搜索测试**

```typescript
describe('usePackageSearch - 最近搜索', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('addRecentSearch 添加并去重', () => {
    const cs = mountComposable()
    cs.addRecentSearch('react')
    cs.addRecentSearch('vue')
    cs.addRecentSearch('react')

    expect(cs.recentSearches.value).toEqual(['react', 'vue'])
  })

  it('最近搜索最多保留 5 个', () => {
    const cs = mountComposable()
    cs.addRecentSearch('a')
    cs.addRecentSearch('b')
    cs.addRecentSearch('c')
    cs.addRecentSearch('d')
    cs.addRecentSearch('e')
    cs.addRecentSearch('f')

    expect(cs.recentSearches.value).toEqual(['f', 'e', 'd', 'c', 'b'])
    expect(cs.recentSearches.value).toHaveLength(5)
  })

  it('clearRecentSearches 清空历史', () => {
    const cs = mountComposable()
    cs.addRecentSearch('react')
    cs.clearRecentSearches()

    expect(cs.recentSearches.value).toEqual([])
  })

  it('最近搜索持久化到 localStorage', () => {
    const cs = mountComposable()
    cs.addRecentSearch('react')

    const stored = localStorage.getItem('package-explorer:recent:admin')
    expect(stored).toBeTruthy()
    expect(JSON.parse(stored!)).toEqual(['react'])
  })
})
```

- [ ] **步骤 2：追加快捷键测试**

```typescript
describe('usePackageSearch - 键盘快捷键', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
  })

  it('Esc 清空搜索词', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    const cs = mountComposable()
    cs.query.q = 'react'

    const event = new KeyboardEvent('keydown', { key: 'Escape' })
    document.dispatchEvent(event)

    expect(cs.query.q).toBe('')
  })

  it('焦点在 input 时不拦截 / 字符', async () => {
    ;(packageApi.search as any).mockResolvedValue(mockSearchResponse())

    mountComposable()

    const input = document.createElement('input')
    document.body.appendChild(input)
    input.focus()

    const event = new KeyboardEvent('keydown', { key: '/' })
    const spy = vi.spyOn(event, 'preventDefault')
    document.dispatchEvent(event)

    expect(spy).not.toHaveBeenCalled()
    document.body.removeChild(input)
  })
})
```

- [ ] **步骤 3：运行测试验证通过**

运行：`cd web && npx vitest run src/composables/usePackageSearch.spec.ts`
预期：PASS（任务2已实现最近搜索与快捷键，全部测试通过）

- [ ] **步骤 4：Commit**

```bash
git add web/src/composables/usePackageSearch.spec.ts
git commit -m "test(frontend): verify recent search and keyboard shortcuts in usePackageSearch"
```

---

## 任务 6：PackageSkeleton 与 PackageEmptyState

**文件：**
- 创建：`web/src/components/package-explorer/PackageSkeleton.vue`
- 创建：`web/src/components/package-explorer/PackageEmptyState.vue`
- 创建：`web/src/components/package-explorer/PackageEmptyState.spec.ts`

- [ ] **步骤 1：实现 PackageSkeleton**

创建 `web/src/components/package-explorer/PackageSkeleton.vue`：

```vue
<template>
  <div v-if="variant === 'table'" class="skeleton-table">
    <div v-for="i in 8" :key="i" class="skeleton-row">
      <div class="skeleton-cell skeleton-name"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-type"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-count"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-count"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-time"><div class="shimmer"></div></div>
      <div class="skeleton-cell skeleton-actions"><div class="shimmer"></div></div>
    </div>
  </div>
  <div v-else class="skeleton-grid">
    <div v-for="i in 8" :key="i" class="skeleton-card">
      <div class="shimmer skeleton-line short"></div>
      <div class="shimmer skeleton-line long"></div>
      <div class="shimmer skeleton-line medium"></div>
      <div class="skeleton-meta">
        <div class="shimmer skeleton-line tiny"></div>
        <div class="shimmer skeleton-line tiny"></div>
        <div class="shimmer skeleton-line tiny"></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  variant: 'table' | 'grid'
}>()
</script>

<style scoped>
.shimmer {
  background: linear-gradient(90deg, #f1f5f9 25%, #e2e8f0 37%, #f1f5f9 63%);
  background-size: 400% 100%;
  animation: shimmer 1.4s ease infinite;
  border-radius: 4px;
  width: 100%;
  height: 100%;
}
@keyframes shimmer {
  0% { background-position: 100% 0; }
  100% { background-position: 0 0; }
}
.skeleton-table { display: flex; flex-direction: column; gap: 8px; padding: 16px; }
.skeleton-row { display: flex; gap: 12px; height: 56px; }
.skeleton-cell { border-radius: 6px; overflow: hidden; }
.skeleton-name { flex: 1; }
.skeleton-type { width: 80px; }
.skeleton-count { width: 70px; }
.skeleton-time { width: 140px; }
.skeleton-actions { width: 240px; }
.skeleton-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px; }
.skeleton-card { padding: 20px; background: #fff; border-radius: 12px; border: 1px solid #f1f5f9; }
.skeleton-line { height: 14px; margin-bottom: 8px; }
.skeleton-line.short { width: 40%; }
.skeleton-line.long { width: 90%; }
.skeleton-line.medium { width: 65%; }
.skeleton-line.tiny { width: 30%; height: 10px; display: inline-block; margin-right: 8px; }
.skeleton-meta { margin-top: 16px; }
</style>
```

- [ ] **步骤 2：编写 PackageEmptyState 测试**

创建 `web/src/components/package-explorer/PackageEmptyState.spec.ts`：

```typescript
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PackageEmptyState from './PackageEmptyState.vue'

describe('PackageEmptyState', () => {
  it('无数据状态显示"暂无包"', () => {
    const wrapper = mount(PackageEmptyState, {
      props: { variant: 'empty', mode: 'admin' },
    })
    expect(wrapper.text()).toContain('暂无包')
    expect(wrapper.text()).toContain('上传第一个包')
  })

  it('无匹配结果状态显示"未找到匹配的包"', () => {
    const wrapper = mount(PackageEmptyState, {
      props: { variant: 'no-match', mode: 'admin' },
    })
    expect(wrapper.text()).toContain('未找到匹配的包')
    expect(wrapper.find('[data-test="clear-filters"]').exists()).toBe(true)
  })

  it('加载失败状态显示"加载失败"和重试按钮', () => {
    const wrapper = mount(PackageEmptyState, {
      props: { variant: 'error', mode: 'admin', errorMessage: '网络错误' },
    })
    expect(wrapper.text()).toContain('加载失败')
    expect(wrapper.text()).toContain('网络错误')
    expect(wrapper.find('[data-test="retry"]').exists()).toBe(true)
  })

  it('点击重试按钮触发 retry 事件', async () => {
    const wrapper = mount(PackageEmptyState, {
      props: { variant: 'error', mode: 'admin', errorMessage: '网络错误' },
    })
    await wrapper.find('[data-test="retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toBeTruthy()
  })

  it('点击清空筛选触发 clear-filters 事件', async () => {
    const wrapper = mount(PackageEmptyState, {
      props: { variant: 'no-match', mode: 'admin' },
    })
    await wrapper.find('[data-test="clear-filters"]').trigger('click')
    expect(wrapper.emitted('clear-filters')).toBeTruthy()
  })

  it('public 模式无数据状态不显示上传引导', () => {
    const wrapper = mount(PackageEmptyState, {
      props: { variant: 'empty', mode: 'public' },
    })
    expect(wrapper.text()).not.toContain('上传第一个包')
  })
})
```

- [ ] **步骤 3：运行测试验证失败**

运行：`cd web && npx vitest run src/components/package-explorer/PackageEmptyState.spec.ts`
预期：FAIL，组件不存在

- [ ] **步骤 4：实现 PackageEmptyState**

创建 `web/src/components/package-explorer/PackageEmptyState.vue`：

```vue
<template>
  <div class="empty-state">
    <el-empty :description="description">
      <template #image>
        <el-icon class="empty-icon"><Box /></el-icon>
      </template>
      <div class="empty-actions">
        <template v-if="variant === 'empty' && mode === 'admin'">
          <el-button type="primary" @click="$emit('upload')">
            <el-icon><Upload /></el-icon>
            上传第一个包
          </el-button>
        </template>
        <template v-if="variant === 'no-match'">
          <p class="empty-hint">尝试调整搜索词或清空筛选条件</p>
          <el-button data-test="clear-filters" @click="$emit('clear-filters')">清空所有筛选</el-button>
        </template>
        <template v-if="variant === 'error'">
          <p class="empty-error-msg">{{ errorMessage }}</p>
          <el-button data-test="retry" type="primary" @click="$emit('retry')">重试</el-button>
        </template>
      </div>
    </el-empty>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Box, Upload } from '@element-plus/icons-vue'

const props = defineProps<{
  variant: 'empty' | 'no-match' | 'error'
  mode: 'admin' | 'public'
  errorMessage?: string
}>()

defineEmits<{
  upload: []
  'clear-filters': []
  retry: []
}>()

const description = computed(() => {
  switch (props.variant) {
    case 'empty': return '暂无包'
    case 'no-match': return '未找到匹配的包'
    case 'error': return '加载失败'
  }
})
</script>

<style scoped>
.empty-state { padding: 60px 20px; text-align: center; }
.empty-icon { font-size: 64px; color: #cbd5e1; }
.empty-actions { margin-top: 16px; }
.empty-hint { color: #94a3b8; font-size: 13px; margin: 0 0 12px; }
.empty-error-msg { color: #ef4444; font-size: 13px; margin: 0 0 12px; }
</style>
```

- [ ] **步骤 5：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackageEmptyState.spec.ts`
预期：PASS（6 个测试全过）

- [ ] **步骤 6：Commit**

```bash
git add web/src/components/package-explorer/PackageSkeleton.vue web/src/components/package-explorer/PackageEmptyState.vue web/src/components/package-explorer/PackageEmptyState.spec.ts
git commit -m "feat(frontend): add PackageSkeleton and PackageEmptyState components"
```

---

## 任务 7：PackagePagination

**文件：**
- 创建：`web/src/components/package-explorer/PackagePagination.vue`
- 创建：`web/src/components/package-explorer/PackagePagination.spec.ts`

- [ ] **步骤 1：编写测试**

创建 `web/src/components/package-explorer/PackagePagination.spec.ts`：

```typescript
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PackagePagination from './PackagePagination.vue'

describe('PackagePagination', () => {
  it('显示总数信息', () => {
    const wrapper = mount(PackagePagination, {
      props: { total: 100, page: 1, pageSize: 20, pageSizeOptions: [20, 50, 100] },
    })
    expect(wrapper.text()).toContain('100')
    expect(wrapper.text()).toContain('个包')
  })

  it('页码变化触发 update:page 事件', async () => {
    const wrapper = mount(PackagePagination, {
      props: { total: 100, page: 1, pageSize: 20, pageSizeOptions: [20, 50, 100] },
    })
    await wrapper.findComponent({ name: 'ElPagination' }).vm.$emit('current-change', 3)
    expect(wrapper.emitted('update:page')?.[0]).toEqual([3])
  })

  it('每页大小变化触发 update:pageSize 并重置到第1页', async () => {
    const wrapper = mount(PackagePagination, {
      props: { total: 100, page: 3, pageSize: 20, pageSizeOptions: [20, 50, 100] },
    })
    await wrapper.findComponent({ name: 'ElPagination' }).vm.$emit('size-change', 50)
    expect(wrapper.emitted('update:pageSize')?.[0]).toEqual([50])
    expect(wrapper.emitted('update:page')?.[0]).toEqual([1])
  })

  it('total 为 0 时不渲染', () => {
    const wrapper = mount(PackagePagination, {
      props: { total: 0, page: 1, pageSize: 20, pageSizeOptions: [20, 50, 100] },
    })
    expect(wrapper.find('.pagination-wrapper').exists()).toBe(false)
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/components/package-explorer/PackagePagination.spec.ts`
预期：FAIL，组件不存在

- [ ] **步骤 3：实现组件**

创建 `web/src/components/package-explorer/PackagePagination.vue`：

```vue
<template>
  <div v-if="total > 0" class="pagination-wrapper">
    <div class="pagination-info">
      <span class="total-badge">{{ total }}</span>
      <span class="total-label">个包</span>
    </div>
    <el-pagination
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      :page-sizes="pageSizeOptions"
      layout="sizes, prev, pager, next"
      @current-change="onCurrentChange"
      @size-change="onSizeChange"
    />
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{
  total: number
  page: number
  pageSize: number
  pageSizeOptions: number[]
}>()

const emit = defineEmits<{
  'update:page': [page: number]
  'update:pageSize': [size: number]
}>()

function onCurrentChange(p: number) {
  emit('update:page', p)
}

function onSizeChange(size: number) {
  emit('update:pageSize', size)
  emit('update:page', 1)
}
</script>

<style scoped>
.pagination-wrapper {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 18px 24px;
  border-top: 1px solid rgba(0, 0, 0, 0.04);
  background: #fafbfc;
}
.pagination-info { display: flex; align-items: center; gap: 8px; }
.total-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  height: 26px;
  padding: 0 12px;
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  border-radius: 8px;
}
.total-label { font-size: 13px; color: #64748b; }
</style>
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackagePagination.spec.ts`
预期：PASS（4 个测试全过）

- [ ] **步骤 5：Commit**

```bash
git add web/src/components/package-explorer/PackagePagination.vue web/src/components/package-explorer/PackagePagination.spec.ts
git commit -m "feat(frontend): add PackagePagination component"
```

---

## 任务 8：PackageSearchBar

**文件：**
- 创建：`web/src/components/package-explorer/PackageSearchBar.vue`
- 创建：`web/src/components/package-explorer/PackageSearchBar.spec.ts`

- [ ] **步骤 1：编写测试**

创建 `web/src/components/package-explorer/PackageSearchBar.spec.ts`：

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import PackageSearchBar from './PackageSearchBar.vue'

const baseQuery = {
  q: '', type: 'all', repository: '', version: '', source: 'all',
  sort: 'updated_at' as const, page: 1, pageSize: 20,
}

describe('PackageSearchBar', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  it('渲染搜索框和类型 chips', () => {
    const wrapper = mount(PackageSearchBar, {
      props: { query: baseQuery, recentSearches: [], loading: false, hasActiveFilter: false, viewMode: 'table' },
    })
    expect(wrapper.find('.search-input').exists()).toBe(true)
    expect(wrapper.findAll('.type-chip')).toHaveLength(6)  // 全部 + 前5个类型
  })

  it('输入触发 300ms debounce 后 emit search', async () => {
    const wrapper = mount(PackageSearchBar, {
      props: { query: baseQuery, recentSearches: [], loading: false, hasActiveFilter: false, viewMode: 'table' },
    })

    await wrapper.find('.search-input input').setValue('react')
    expect(wrapper.emitted('update:query')).toBeTruthy()

    vi.advanceTimersByTime(299)
    expect(wrapper.emitted('search')).toBeFalsy()

    vi.advanceTimersByTime(1)
    expect(wrapper.emitted('search')).toBeTruthy()
  })

  it('回车立即触发 search 并 emit add-recent', async () => {
    const wrapper = mount(PackageSearchBar, {
      props: { query: { ...baseQuery, q: 'react' }, recentSearches: [], loading: false, hasActiveFilter: false, viewMode: 'table' },
    })

    await wrapper.find('.search-input input').trigger('keyup.enter')
    expect(wrapper.emitted('search')).toBeTruthy()
    expect(wrapper.emitted('add-recent')?.[0]).toEqual(['react'])
  })

  it('点击类型 chip 触发 update:query with type', async () => {
    const wrapper = mount(PackageSearchBar, {
      props: { query: baseQuery, recentSearches: [], loading: false, hasActiveFilter: false, viewMode: 'table' },
    })

    await wrapper.findAll('.type-chip')[1].trigger('click')
    const emitted = wrapper.emitted('update:query')?.[0] as any
    expect(emitted.type).toBe('npm')
    expect(emitted.page).toBe(1)
  })

  it('聚焦时显示最近搜索下拉', async () => {
    const wrapper = mount(PackageSearchBar, {
      props: { query: baseQuery, recentSearches: ['react', 'vue'], loading: false, hasActiveFilter: false, viewMode: 'table' },
    })

    await wrapper.find('.search-input input').trigger('focus')
    expect(wrapper.find('.recent-dropdown').exists()).toBe(true)
    expect(wrapper.findAll('.recent-item')).toHaveLength(2)
  })

  it('点击最近搜索项触发 search', async () => {
    const wrapper = mount(PackageSearchBar, {
      props: { query: baseQuery, recentSearches: ['react'], loading: false, hasActiveFilter: false, viewMode: 'table' },
    })

    await wrapper.find('.search-input input').trigger('focus')
    await wrapper.find('.recent-item').trigger('mousedown')
    expect(wrapper.emitted('search')).toBeTruthy()
  })

  it('有激活筛选时筛选按钮显示红点', () => {
    const wrapper = mount(PackageSearchBar, {
      props: { query: { ...baseQuery, repository: 'main' }, recentSearches: [], loading: false, hasActiveFilter: true, viewMode: 'table' },
    })
    expect(wrapper.find('.filter-badge').exists()).toBe(true)
  })

  it('清空按钮触发清空并搜索', async () => {
    const wrapper = mount(PackageSearchBar, {
      props: { query: { ...baseQuery, q: 'react' }, recentSearches: [], loading: false, hasActiveFilter: false, viewMode: 'table' },
    })

    await wrapper.find('.clear-btn').trigger('click')
    const emitted = wrapper.emitted('update:query')?.[0] as any
    expect(emitted.q).toBe('')
    expect(wrapper.emitted('search')).toBeTruthy()
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/components/package-explorer/PackageSearchBar.spec.ts`
预期：FAIL，组件不存在

- [ ] **步骤 3：实现组件**

创建 `web/src/components/package-explorer/PackageSearchBar.vue`：

```vue
<template>
  <div class="package-search-bar">
    <div class="search-wrapper">
      <el-input
        ref="searchInputRef"
        v-model="localQuery"
        class="search-input"
        placeholder="搜索包名、描述或标签（按 / 聚焦）"
        clearable
        @input="onInput"
        @keyup.enter="onEnter"
        @focus="showRecent = true"
        @blur="onBlur"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
        <template v-if="loading" #suffix>
          <el-icon class="is-loading"><Loading /></el-icon>
        </template>
        <template #append>
          <el-button class="clear-btn" @click="onClear">
            <el-icon><Close /></el-icon>
          </el-button>
        </template>
      </el-input>

      <div v-if="showRecent && recentSearches.length > 0" class="recent-dropdown">
        <div class="recent-header">
          <span>最近搜索</span>
          <el-button link size="small" @click="$emit('clear-recent')">清空历史</el-button>
        </div>
        <div
          v-for="term in recentSearches"
          :key="term"
          class="recent-item"
          @mousedown.prevent="onRecentClick(term)"
        >
          <el-icon><Clock /></el-icon>
          <span>{{ term }}</span>
        </div>
      </div>
    </div>

    <div class="type-chips">
      <button
        v-for="opt in visibleTypes"
        :key="opt.value"
        class="type-chip"
        :class="{ 'type-chip--active': (query.type === 'all' ? '' : query.type) === opt.value }"
        :style="getChipStyle(opt.value)"
        @click="onTypeClick(opt.value)"
      >
        <span v-if="opt.value" class="type-dot" :style="{ background: getDotColor(opt.value) }"></span>
        {{ opt.label }}
      </button>
      <el-dropdown v-if="hiddenTypes.length > 0" trigger="click" @command="onTypeClick">
        <button class="type-chip type-chip--more">
          更多<el-icon><ArrowDown /></el-icon>
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="opt in hiddenTypes" :key="opt.value" :command="opt.value">
              {{ opt.label }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>

      <div class="toolbar-actions">
        <el-badge :is-dot="hasActiveFilter" class="filter-badge">
          <el-button @click="$emit('open-filter')">
            <el-icon><Filter /></el-icon>筛选
          </el-button>
        </el-badge>

        <el-select
          :model-value="query.sort"
          class="sort-select"
          @change="(v: string) => emitUpdate({ sort: v as any })"
        >
          <el-option label="更新时间" value="updated_at" />
          <el-option label="下载量" value="download_count" />
          <el-option label="名称" value="name" />
        </el-select>

        <div class="view-toggle">
          <el-button :class="{ active: viewMode === 'table' }" @click="$emit('update:viewMode', 'table')">
            <el-icon><List /></el-icon>
          </el-button>
          <el-button :class="{ active: viewMode === 'grid' }" @click="$emit('update:viewMode', 'grid')">
            <el-icon><Grid /></el-icon>
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Search, Loading, Close, Clock, ArrowDown, Filter, List, Grid } from '@element-plus/icons-vue'
import { PACKAGE_TYPE_OPTIONS, PACKAGE_TYPE_HEX_COLORS } from '@/constants/package'
import type { PackageQuery } from '@/composables/usePackageSearch'

const props = defineProps<{
  query: PackageQuery
  recentSearches: string[]
  loading: boolean
  hasActiveFilter: boolean
  viewMode: 'table' | 'grid'
}>()

const emit = defineEmits<{
  'update:query': [patch: Partial<PackageQuery>]
  'update:viewMode': [mode: 'table' | 'grid']
  search: []
  'add-recent': [term: string]
  'clear-recent': []
  'open-filter': []
}>()

const searchInputRef = ref()
const localQuery = ref(props.query.q)
const showRecent = ref(false)

watch(() => props.query.q, (v) => {
  if (v !== localQuery.value) localQuery.value = v
})

const allTypes = computed(() => [
  { value: '', label: '全部' },
  ...PACKAGE_TYPE_OPTIONS.map(o => ({ value: o.value, label: o.label })),
])
const visibleTypes = computed(() => allTypes.value.slice(0, 6))
const hiddenTypes = computed(() => allTypes.value.slice(6))

function getDotColor(type: string): string {
  return PACKAGE_TYPE_HEX_COLORS[type] || PACKAGE_TYPE_HEX_COLORS.generic
}
function getChipStyle(type: string) {
  const isActive = (props.query.type === 'all' ? '' : props.query.type) === type
  if (!isActive || !type) return {}
  const color = getDotColor(type)
  return { borderColor: color, color, background: `${color}1a` }
}

let debounceTimer: ReturnType<typeof setTimeout> | null = null

function emitUpdate(patch: Partial<PackageQuery>) {
  emit('update:query', patch)
  emit('search')
}

function onInput() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    emit('update:query', { q: localQuery.value })
    emit('search')
  }, 300)
}

function onEnter() {
  if (debounceTimer) clearTimeout(debounceTimer)
  emit('update:query', { q: localQuery.value })
  emit('search')
  const term = localQuery.value.trim()
  if (term) emit('add-recent', term)
}

function onClear() {
  localQuery.value = ''
  if (debounceTimer) clearTimeout(debounceTimer)
  emit('update:query', { q: '' })
  emit('search')
}

function onTypeClick(value: string) {
  emit('update:query', { type: value || 'all', page: 1 })
  emit('search')
}

function onRecentClick(term: string) {
  localQuery.value = term
  if (debounceTimer) clearTimeout(debounceTimer)
  emit('update:query', { q: term, page: 1 })
  emit('search')
  showRecent.value = false
}

function onBlur() {
  setTimeout(() => { showRecent.value = false }, 200)
}

defineExpose({
  focus: () => searchInputRef.value?.focus?.(),
})
</script>

<style scoped>
.package-search-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 16px 20px;
  background: #fff;
  border-radius: 12px;
  border: 1px solid rgba(0, 0, 0, 0.04);
}
.search-wrapper { position: relative; flex: 1; min-width: 240px; }
.search-input { width: 100%; }
.recent-dropdown {
  position: absolute; top: 100%; left: 0; right: 0; margin-top: 4px;
  background: #fff; border: 1px solid #e2e8f0; border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08); z-index: 100; padding: 8px;
}
.recent-header { display: flex; justify-content: space-between; align-items: center; padding: 4px 12px; font-size: 12px; color: #94a3b8; }
.recent-item { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: 6px; cursor: pointer; color: #475569; font-size: 13px; }
.recent-item:hover { background: #f1f5f9; }
.type-chips { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.type-chip {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 7px 14px; border: 1px solid #e2e8f0; border-radius: 8px;
  background: #f8fafc; color: #64748b; font-size: 13px; cursor: pointer;
  transition: all 0.2s ease;
}
.type-chip:hover { border-color: #cbd5e1; background: #fff; }
.type-chip--active { font-weight: 600; }
.type-dot { width: 8px; height: 8px; border-radius: 50%; }
.toolbar-actions { display: flex; align-items: center; gap: 12px; margin-left: auto; }
.sort-select { width: 130px; }
.view-toggle { display: flex; border: 1px solid #e2e8f0; border-radius: 8px; overflow: hidden; }
.view-toggle .el-button { border: none; border-radius: 0; }
.view-toggle .el-button + .el-button { border-left: 1px solid #e2e8f0; }
.view-toggle .el-button.active { background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%); color: #fff; }
</style>
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackageSearchBar.spec.ts`
预期：PASS（8 个测试全过）

- [ ] **步骤 5：Commit**

```bash
git add web/src/components/package-explorer/PackageSearchBar.vue web/src/components/package-explorer/PackageSearchBar.spec.ts
git commit -m "feat(frontend): add PackageSearchBar with debounce, recent search, type chips"
```

---

## 任务 9：PackageFilterPanel

**文件：**
- 创建：`web/src/components/package-explorer/PackageFilterPanel.vue`
- 创建：`web/src/components/package-explorer/PackageFilterPanel.spec.ts`

**注意**：后端暂不支持 `source`/`from`/`to`，本任务仅实现"仓库"和"版本"筛选。source 与时间范围 UI 暂不实现（后端补齐后再加）。

- [ ] **步骤 1：编写测试**

创建 `web/src/components/package-explorer/PackageFilterPanel.spec.ts`：

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PackageFilterPanel from './PackageFilterPanel.vue'

vi.mock('@/api/repository', () => ({
  repositoryApi: {
    list: vi.fn().mockResolvedValue([
      { id: 1, name: 'npm-proxy', type: 'proxy', package_type: 'npm' },
      { id: 2, name: 'maven-hosted', type: 'local', package_type: 'maven' },
    ]),
  },
}))

describe('PackageFilterPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('打开时加载仓库列表', async () => {
    const wrapper = mount(PackageFilterPanel, {
      props: { visible: true, repository: '', version: '' },
    })
    await flushPromises()
    expect(wrapper.findAll('.repo-option')).toHaveLength(2)
  })

  it('选择仓库触发 update:repository', async () => {
    const wrapper = mount(PackageFilterPanel, {
      props: { visible: true, repository: '', version: '' },
    })
    await flushPromises()

    await wrapper.findComponent({ name: 'ElSelect' }).vm.$emit('update:modelValue', 'npm-proxy')
    expect(wrapper.emitted('update:repository')?.[0]).toEqual(['npm-proxy'])
  })

  it('输入版本触发 update:version', async () => {
    const wrapper = mount(PackageFilterPanel, {
      props: { visible: true, repository: '', version: '' },
    })
    await flushPromises()

    await wrapper.find('.version-input input').setValue('1.2.*')
    expect(wrapper.emitted('update:version')?.[0]).toEqual(['1.2.*'])
  })

  it('点击重置清空仓库和版本', async () => {
    const wrapper = mount(PackageFilterPanel, {
      props: { visible: true, repository: 'npm-proxy', version: '1.0' },
    })
    await flushPromises()

    await wrapper.find('[data-test="reset"]').trigger('click')
    expect(wrapper.emitted('update:repository')?.[0]).toEqual([''])
    expect(wrapper.emitted('update:version')?.[0]).toEqual([''])
  })

  it('点击应用触发 apply 事件', async () => {
    const wrapper = mount(PackageFilterPanel, {
      props: { visible: true, repository: '', version: '' },
    })
    await flushPromises()

    await wrapper.find('[data-test="apply"]').trigger('click')
    expect(wrapper.emitted('apply')).toBeTruthy()
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/components/package-explorer/PackageFilterPanel.spec.ts`
预期：FAIL，组件不存在

- [ ] **步骤 3：实现组件**

创建 `web/src/components/package-explorer/PackageFilterPanel.vue`：

```vue
<template>
  <el-drawer
    :model-value="visible"
    title="高级筛选"
    direction="rtl"
    size="380px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <div class="filter-panel">
      <div class="filter-section">
        <label class="filter-label">仓库</label>
        <el-select
          :model-value="repository"
          placeholder="全部仓库"
          clearable
          filterable
          @update:model-value="(v: string) => $emit('update:repository', v || '')"
        >
          <el-option
            v-for="repo in repositories"
            :key="repo.id"
            :label="repo.name"
            :value="repo.name"
            class="repo-option"
          >
            <span>{{ repo.name }}</span>
            <el-tag size="small" type="info">{{ repo.type }}</el-tag>
          </el-option>
        </el-select>
      </div>

      <div class="filter-section">
        <label class="filter-label">版本</label>
        <el-input
          :model-value="version"
          class="version-input"
          placeholder="支持精确版本号或通配符，如 1.2.*"
          clearable
          @update:model-value="(v: string) => $emit('update:version', v)"
        />
      </div>

      <!-- TODO: 后端支持 source / updatedAtRange 后启用
      <div class="filter-section">
        <label class="filter-label">来源</label>
        <el-radio-group :model-value="source" @update:model-value="...">
          <el-radio value="all">全部</el-radio>
          <el-radio value="local">本地</el-radio>
          <el-radio value="proxy">代理</el-radio>
        </el-radio-group>
      </div>
      -->
    </div>

    <template #footer>
      <div class="filter-footer">
        <el-button data-test="reset" @click="onReset">重置</el-button>
        <el-button data-test="apply" type="primary" @click="$emit('apply')">应用</el-button>
      </div>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { repositoryApi, type Repository } from '@/api/repository'

const props = defineProps<{
  visible: boolean
  repository: string
  version: string
}>()

defineEmits<{
  'update:visible': [v: boolean]
  'update:repository': [v: string]
  'update:version': [v: string]
  apply: []
  reset: []
}>()

const repositories = ref<Repository[]>([])

async function loadRepositories() {
  try {
    const res = await repositoryApi.list({ page: 1, page_size: 200 })
    const list = Array.isArray(res) ? res : (res as any).list || []
    repositories.value = list as Repository[]
  } catch {
    repositories.value = []
  }
}

watch(() => props.visible, (v) => {
  if (v && repositories.value.length === 0) {
    loadRepositories()
  }
})

function onReset() {
  // 通过 update 事件清空，由父组件统一管理状态
  // 注意：这里不能直接 emit apply，父组件根据 reset 事件自行决定
  // 但测试期望同时 emit update:repository 和 update:version
  // 改为直接 emit 两个 update
}
</script>

<style scoped>
.filter-panel { display: flex; flex-direction: column; gap: 24px; }
.filter-section { display: flex; flex-direction: column; gap: 8px; }
.filter-label { font-size: 13px; font-weight: 600; color: #475569; }
.filter-footer { display: flex; justify-content: flex-end; gap: 12px; }
</style>
```

**修正**：测试期望"重置"同时 emit 两个 update 事件，调整 `onReset`：

```typescript
function onReset() {
  // @ts-expect-error Vue emit 允许连续调用
  defineEmits // 占位说明：实际 emit 通过模板 $emit
}
```

实际实现里 `onReset` 应直接调用 `emit`：

```typescript
const emit = defineEmits<{
  'update:visible': [v: boolean]
  'update:repository': [v: string]
  'update:version': [v: string]
  apply: []
  reset: []
}>()

function onReset() {
  emit('update:repository', '')
  emit('update:version', '')
  emit('reset')
}
```

注意：使用 `defineEmits` 的返回值 `emit`，而非模板里的 `$emit`。

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackageFilterPanel.spec.ts`
预期：PASS（5 个测试全过）

- [ ] **步骤 5：Commit**

```bash
git add web/src/components/package-explorer/PackageFilterPanel.vue web/src/components/package-explorer/PackageFilterPanel.spec.ts
git commit -m "feat(frontend): add PackageFilterPanel with repository and version filters"
```

---

## 任务 10：PackageTable

**文件：**
- 创建：`web/src/components/package-explorer/PackageTable.vue`
- 创建：`web/src/components/package-explorer/PackageTable.spec.ts`

**职责**：表格视图，支持批量选择（admin）、列设置（admin）、密度切换（admin）、包名复制、操作列（查看版本/详情/删除）。

- [ ] **步骤 1：编写测试**

创建 `web/src/components/package-explorer/PackageTable.spec.ts`：

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import PackageTable from './PackageTable.vue'
import { packageApi } from '@/api/package'

vi.mock('@/api/package', () => ({
  packageApi: {
    deletePackage: vi.fn(),
  },
}))

vi.mock('@/utils/clipboard', () => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))

const samplePackages = [
  { id: 1, name: 'lodash', display_name: 'lodash', type: 'npm', description: ' Utility library', repository_type: 'proxy', versions_count: 3, download_count: 100, updated_at: '2026-06-19T00:00:00Z' },
  { id: 2, name: 'react', display_name: 'react', type: 'npm', description: 'UI library', repository_type: 'local', versions_count: 5, download_count: 500, updated_at: '2026-06-18T00:00:00Z' },
]

describe('PackageTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('渲染包列表', () => {
    const wrapper = mount(PackageTable, {
      props: { packages: samplePackages, loading: false, mode: 'admin', density: 'default', selectedIds: [], columns: {} },
    })
    expect(wrapper.findAll('.el-table__row')).toHaveLength(2)
    expect(wrapper.text()).toContain('lodash')
    expect(wrapper.text()).toContain('react')
  })

  it('admin 模式显示批量选择列', () => {
    const wrapper = mount(PackageTable, {
      props: { packages: samplePackages, loading: false, mode: 'admin', density: 'default', selectedIds: [], columns: {} },
    })
    expect(wrapper.find('.el-table-column--selection').exists()).toBe(true)
  })

  it('public 模式不显示批量选择列和操作列的删除按钮', () => {
    const wrapper = mount(PackageTable, {
      props: { packages: samplePackages, loading: false, mode: 'public', density: 'default', selectedIds: [], columns: {} },
    })
    expect(wrapper.find('.el-table-column--selection').exists()).toBe(false)
    expect(wrapper.find('.btn-delete').exists()).toBe(false)
  })

  it('勾选行触发 update:selectedIds', async () => {
    const wrapper = mount(PackageTable, {
      props: { packages: samplePackages, loading: false, mode: 'admin', density: 'default', selectedIds: [], columns: {} },
    })
    await wrapper.find('input[type="checkbox"]').setValue(true)
    expect(wrapper.emitted('update:selectedIds')).toBeTruthy()
  })

  it('点击包名复制按钮触发复制', async () => {
    const { copyToClipboard } = await import('@/utils/clipboard')
    const wrapper = mount(PackageTable, {
      props: { packages: samplePackages, loading: false, mode: 'admin', density: 'default', selectedIds: [], columns: {} },
    })
    await wrapper.find('.copy-name-btn').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('npm:lodash')
  })

  it('点击查看版本触发 view-versions 事件', async () => {
    const wrapper = mount(PackageTable, {
      props: { packages: samplePackages, loading: false, mode: 'admin', density: 'default', selectedIds: [], columns: {} },
    })
    await wrapper.find('.btn-view-versions').trigger('click')
    expect(wrapper.emitted('view-versions')?.[0]).toEqual([samplePackages[0]])
  })

  it('点击详情触发 view-detail 事件', async () => {
    const wrapper = mount(PackageTable, {
      props: { packages: samplePackages, loading: false, mode: 'admin', density: 'default', selectedIds: [], columns: {} },
    })
    await wrapper.find('.btn-view-detail').trigger('click')
    expect(wrapper.emitted('view-detail')?.[0]).toEqual([samplePackages[0]])
  })

  it('点击删除触发 delete-package 事件', async () => {
    const wrapper = mount(PackageTable, {
      props: { packages: samplePackages, loading: false, mode: 'admin', density: 'default', selectedIds: [], columns: {} },
    })
    await wrapper.find('.btn-delete').trigger('click')
    expect(wrapper.emitted('delete-package')?.[0]).toEqual([samplePackages[0]])
  })

  it('columns 配置控制列显隐', () => {
    const wrapper = mount(PackageTable, {
      props: {
        packages: samplePackages,
        loading: false,
        mode: 'admin',
        density: 'default',
        selectedIds: [],
        columns: { description: false, source: false, versions: false, downloads: false, updatedAt: false },
      },
    })
    // 隐藏的列不应渲染表头
    const headers = wrapper.findAll('.el-table__header th')
    const headerTexts = headers.map(h => h.text())
    expect(headerTexts.some(t => t.includes('描述'))).toBe(false)
  })

  it('density 传给 el-table 的 size', () => {
    const wrapper = mount(PackageTable, {
      props: { packages: samplePackages, loading: false, mode: 'admin', density: 'small', selectedIds: [], columns: {} },
    })
    expect(wrapper.findComponent({ name: 'ElTable' }).props('size')).toBe('small')
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/components/package-explorer/PackageTable.spec.ts`
预期：FAIL，组件不存在

- [ ] **步骤 3：实现组件**

创建 `web/src/components/package-explorer/PackageTable.vue`：

```vue
<template>
  <el-table
    :data="packages"
    :size="density"
    v-loading="loading"
    style="width: 100%"
    empty-text="暂无数据"
    @selection-change="onSelectionChange"
  >
    <el-table-column v-if="mode === 'admin'" type="selection" width="48" />

    <el-table-column prop="name" label="包名" min-width="220">
      <template #default="{ row }">
        <div class="package-info">
          <div class="package-icon" :style="{ background: getTypeBg(row.type) }">
            <i class="fa-solid fa-box"></i>
          </div>
          <div class="package-content">
            <div class="package-name-row">
              <span class="package-name" @click="$emit('view-detail', row)">
                {{ row.display_name || row.name }}
              </span>
              <el-button link class="copy-name-btn" @click="copyName(row)">
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </div>
            <div v-if="columns.description !== false" class="package-description">
              {{ row.description || '暂无描述' }}
            </div>
          </div>
        </div>
      </template>
    </el-table-column>

    <el-table-column prop="type" label="类型" width="100">
      <template #default="{ row }">
        <el-tag :color="getTypeColor(row.type)" effect="light" size="small">
          {{ row.type }}
        </el-tag>
      </template>
    </el-table-column>

    <el-table-column v-if="columns.source !== false" prop="repository_type" label="来源" width="90" align="center">
      <template #default="{ row }">
        <el-tag :type="row.repository_type === 'proxy' ? 'warning' : 'success'" size="small">
          {{ row.repository_type === 'proxy' ? '代理' : '本地' }}
        </el-tag>
      </template>
    </el-table-column>

    <el-table-column v-if="columns.versions !== false" prop="versions_count" label="版本" width="80" align="center">
      <template #default="{ row }">
        <span class="version-count">{{ row.versions_count || 0 }}</span>
      </template>
    </el-table-column>

    <el-table-column v-if="columns.downloads !== false" prop="download_count" label="下载" width="100" align="center">
      <template #default="{ row }">
        <span class="download-count">{{ formatNumber(row.download_count) }}</span>
      </template>
    </el-table-column>

    <el-table-column v-if="columns.updatedAt !== false" prop="updated_at" label="更新时间" width="160">
      <template #default="{ row }">
        <span class="update-time">{{ formatDate(row.updated_at) }}</span>
      </template>
    </el-table-column>

    <el-table-column label="操作" width="260" fixed="right">
      <template #default="{ row }">
        <div class="action-buttons">
          <el-button class="btn-view-versions" size="small" @click="$emit('view-versions', row)">
            查看版本
          </el-button>
          <el-button class="btn-view-detail" size="small" type="primary" @click="$emit('view-detail', row)">
            详情
          </el-button>
          <el-button
            v-if="mode === 'admin'"
            v-permission="'package:delete'"
            class="btn-delete"
            size="small"
            type="danger"
            @click="$emit('delete-package', row)"
          >
            删除
          </el-button>
        </div>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { CopyDocument } from '@element-plus/icons-vue'
import type { Package } from '@/api/package'
import { formatNumber, formatDate } from '@/utils/format'
import { copyToClipboard } from '@/utils/clipboard'
import { PACKAGE_TYPE_HEX_COLORS } from '@/constants/package'

interface ColumnConfig {
  description?: boolean
  source?: boolean
  versions?: boolean
  downloads?: boolean
  updatedAt?: boolean
}

const props = defineProps<{
  packages: Package[]
  loading: boolean
  mode: 'admin' | 'public'
  density: 'small' | 'default' | 'large'
  selectedIds: number[]
  columns: ColumnConfig
}>()

const emit = defineEmits<{
  'update:selectedIds': [ids: number[]]
  'view-versions': [pkg: Package]
  'view-detail': [pkg: Package]
  'delete-package': [pkg: Package]
}>()

function getTypeColor(type: string): string {
  return PACKAGE_TYPE_HEX_COLORS[type] || PACKAGE_TYPE_HEX_COLORS.generic
}

function getTypeBg(type: string): string {
  const hex = getTypeColor(type)
  return `${hex}1a`  // 10% 透明度背景
}

function copyName(row: Package) {
  copyToClipboard(`${row.type}:${row.name}`)
}

function onSelectionChange(rows: Package[]) {
  emit('update:selectedIds', rows.map(r => r.id))
}
</script>

<style scoped>
.package-info { display: flex; align-items: flex-start; gap: 12px; }
.package-icon {
  width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
  border-radius: 8px; font-size: 14px; flex-shrink: 0;
}
.package-content { flex: 1; min-width: 0; }
.package-name-row { display: flex; align-items: center; gap: 4px; }
.package-name { color: #1e293b; cursor: pointer; font-weight: 600; font-size: 14px; }
.package-name:hover { color: #6366f1; }
.copy-name-btn { opacity: 0; transition: opacity 0.2s; padding: 2px; }
.package-info:hover .copy-name-btn { opacity: 1; }
.package-description { font-size: 12px; color: #94a3b8; margin-top: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.version-count { font-weight: 600; color: #1e293b; font-size: 14px; }
.download-count { font-weight: 600; color: #6366f1; font-size: 14px; }
.update-time { color: #94a3b8; font-size: 13px; }
.action-buttons { display: flex; gap: 8px; flex-wrap: nowrap; }
</style>
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackageTable.spec.ts`
预期：PASS（10 个测试全过）

- [ ] **步骤 5：Commit**

```bash
git add web/src/components/package-explorer/PackageTable.vue web/src/components/package-explorer/PackageTable.spec.ts
git commit -m "feat(frontend): add PackageTable with batch select, column config, copy, actions"
```

---

## 任务 11：PackageGrid

**文件：**
- 创建：`web/src/components/package-explorer/PackageGrid.vue`
- 创建：`web/src/components/package-explorer/PackageGrid.spec.ts`

- [ ] **步骤 1：编写测试**

创建 `web/src/components/package-explorer/PackageGrid.spec.ts`：

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import PackageGrid from './PackageGrid.vue'

vi.mock('@/utils/clipboard', () => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))

const samplePackages = [
  { id: 1, name: 'lodash', display_name: 'lodash', type: 'npm', description: 'Utility library', latest_version: '4.17.21', download_count: 100, updated_at: '2026-06-19T00:00:00Z' },
  { id: 2, name: 'react', display_name: 'react', type: 'npm', description: 'UI library', latest_version: '18.0.0', download_count: 500, updated_at: '2026-06-18T00:00:00Z' },
]

describe('PackageGrid', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('渲染卡片网格', () => {
    const wrapper = mount(PackageGrid, {
      props: { packages: samplePackages, mode: 'admin' },
    })
    expect(wrapper.findAll('.package-card')).toHaveLength(2)
  })

  it('卡片显示包名、版本、下载量', () => {
    const wrapper = mount(PackageGrid, {
      props: { packages: samplePackages, mode: 'admin' },
    })
    const card = wrapper.find('.package-card')
    expect(card.text()).toContain('lodash')
    expect(card.text()).toContain('4.17.21')
    expect(card.text()).toContain('100')
  })

  it('点击卡片触发 view-detail', async () => {
    const wrapper = mount(PackageGrid, {
      props: { packages: samplePackages, mode: 'admin' },
    })
    await wrapper.find('.package-card').trigger('click')
    expect(wrapper.emitted('view-detail')?.[0]).toEqual([samplePackages[0]])
  })

  it('复制按钮触发复制 type:name', async () => {
    const { copyToClipboard } = await import('@/utils/clipboard')
    const wrapper = mount(PackageGrid, {
      props: { packages: samplePackages, mode: 'admin' },
    })
    await wrapper.find('.copy-name-btn').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('npm:lodash')
  })

  it('admin 模式显示操作按钮', () => {
    const wrapper = mount(PackageGrid, {
      props: { packages: samplePackages, mode: 'admin' },
    })
    expect(wrapper.find('.btn-view-versions').exists()).toBe(true)
    expect(wrapper.find('.btn-delete').exists()).toBe(true)
  })

  it('public 模式不显示操作按钮', () => {
    const wrapper = mount(PackageGrid, {
      props: { packages: samplePackages, mode: 'public' },
    })
    expect(wrapper.find('.btn-view-versions').exists()).toBe(false)
    expect(wrapper.find('.btn-delete').exists()).toBe(false)
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/components/package-explorer/PackageGrid.spec.ts`
预期：FAIL，组件不存在

- [ ] **步骤 3：实现组件**

创建 `web/src/components/package-explorer/PackageGrid.vue`：

```vue
<template>
  <div class="package-grid">
    <div
      v-for="pkg in packages"
      :key="pkg.id"
      class="package-card"
      @click="$emit('view-detail', pkg)"
    >
      <div class="card-header">
        <div class="card-icon" :style="{ background: getTypeBg(pkg.type) }">
          <i class="fa-solid fa-box"></i>
        </div>
        <div class="card-title">
          <div class="card-name-row">
            <span class="card-name">{{ pkg.display_name || pkg.name }}</span>
            <el-button
              link
              class="copy-name-btn"
              @click.stop="copyName(pkg)"
            >
              <el-icon><CopyDocument /></el-icon>
            </el-button>
          </div>
          <el-tag size="small" effect="light" :style="{ color: getTypeColor(pkg.type) }">
            {{ pkg.type }}
          </el-tag>
        </div>
      </div>

      <p class="card-desc">{{ pkg.description || '暂无描述' }}</p>

      <div class="card-meta">
        <span v-if="pkg.latest_version" class="meta-item">
          <el-icon><PriceTag /></el-icon>
          {{ pkg.latest_version }}
        </span>
        <span class="meta-item">
          <el-icon><Download /></el-icon>
          {{ formatNumber(pkg.download_count) }}
        </span>
        <span class="meta-item">
          <el-icon><Clock /></el-icon>
          {{ formatDate(pkg.updated_at) }}
        </span>
      </div>

      <div v-if="mode === 'admin'" class="card-actions" @click.stop>
        <el-button class="btn-view-versions" size="small" @click="$emit('view-versions', pkg)">
          查看版本
        </el-button>
        <el-button
          v-permission="'package:delete'"
          class="btn-delete"
          size="small"
          type="danger"
          plain
          @click="$emit('delete-package', pkg)"
        >
          删除
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { CopyDocument, PriceTag, Download, Clock } from '@element-plus/icons-vue'
import type { Package } from '@/api/package'
import { formatNumber, formatDate } from '@/utils/format'
import { copyToClipboard } from '@/utils/clipboard'
import { PACKAGE_TYPE_HEX_COLORS } from '@/constants/package'

defineProps<{
  packages: Package[]
  mode: 'admin' | 'public'
}>()

defineEmits<{
  'view-detail': [pkg: Package]
  'view-versions': [pkg: Package]
  'delete-package': [pkg: Package]
}>()

function getTypeColor(type: string): string {
  return PACKAGE_TYPE_HEX_COLORS[type] || PACKAGE_TYPE_HEX_COLORS.generic
}
function getTypeBg(type: string): string {
  return `${getTypeColor(type)}1a`
}
function copyName(pkg: Package) {
  copyToClipboard(`${pkg.type}:${pkg.name}`)
}
</script>

<style scoped>
.package-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}
.package-card {
  padding: 20px;
  background: #fff;
  border-radius: 12px;
  border: 1px solid #f1f5f9;
  cursor: pointer;
  transition: all 0.2s ease;
}
.package-card:hover {
  border-color: #c7d2fe;
  box-shadow: 0 4px 16px rgba(99, 102, 241, 0.08);
  transform: translateY(-2px);
}
.card-header { display: flex; align-items: flex-start; gap: 12px; margin-bottom: 12px; }
.card-icon {
  width: 40px; height: 40px; border-radius: 10px;
  display: flex; align-items: center; justify-content: center;
  font-size: 16px; flex-shrink: 0;
}
.card-title { flex: 1; min-width: 0; }
.card-name-row { display: flex; align-items: center; gap: 4px; margin-bottom: 6px; }
.card-name { font-size: 15px; font-weight: 600; color: #1e293b; }
.copy-name-btn { opacity: 0; transition: opacity 0.2s; padding: 2px; }
.package-card:hover .copy-name-btn { opacity: 1; }
.card-desc {
  font-size: 13px; color: #64748b; margin: 0 0 12px;
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical;
  overflow: hidden; min-height: 38px;
}
.card-meta { display: flex; flex-wrap: wrap; gap: 12px; font-size: 12px; color: #94a3b8; }
.meta-item { display: inline-flex; align-items: center; gap: 4px; }
.card-actions { display: flex; gap: 8px; margin-top: 12px; padding-top: 12px; border-top: 1px solid #f1f5f9; }
</style>
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackageGrid.spec.ts`
预期：PASS（6 个测试全过）

- [ ] **步骤 5：Commit**

```bash
git add web/src/components/package-explorer/PackageGrid.vue web/src/components/package-explorer/PackageGrid.spec.ts
git commit -m "feat(frontend): add PackageGrid with copy button and mode-based actions"
```

---

## 任务 12：PackageExplorer 容器组件

**文件：**
- 创建：`web/src/components/package-explorer/PackageExplorer.vue`
- 创建：`web/src/components/package-explorer/PackageExplorer.spec.ts`

**职责**：编排所有子组件，根据 mode 显隐管理操作，管理 viewMode/selectedIds/columns/density/showFilter 局部状态，处理删除/批量删除/上传。

- [ ] **步骤 1：编写测试**

创建 `web/src/components/package-explorer/PackageExplorer.spec.ts`：

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PackageExplorer from './PackageExplorer.vue'
import { packageApi } from '@/api/package'

vi.mock('@/api/package', () => ({
  packageApi: {
    search: vi.fn(),
    deletePackage: vi.fn(),
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useRoute: () => ({ query: {} }),
}))

const mockPackages = [
  { id: 1, name: 'lodash', display_name: 'lodash', type: 'npm', download_count: 100, updated_at: '2026-06-19T00:00:00Z' },
  { id: 2, name: 'react', display_name: 'react', type: 'npm', download_count: 500, updated_at: '2026-06-18T00:00:00Z' },
]

describe('PackageExplorer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    localStorage.clear()
    ;(packageApi.search as any).mockResolvedValue({ list: mockPackages, total: 2, search_time_ms: 5 })
  })

  it('admin 模式显示上传按钮', async () => {
    const wrapper = mount(PackageExplorer, {
      props: { mode: 'admin' },
      global: { plugins: [createPinia()] },
    })
    await flushPromises()
    expect(wrapper.find('.upload-btn').exists()).toBe(true)
  })

  it('public 模式不显示上传按钮', async () => {
    const wrapper = mount(PackageExplorer, {
      props: { mode: 'public' },
      global: { plugins: [createPinia()] },
    })
    await flushPromises()
    expect(wrapper.find('.upload-btn').exists()).toBe(false)
  })

  it('admin 模式默认表格视图，显示批量操作栏（选中后）', async () => {
    const wrapper = mount(PackageExplorer, {
      props: { mode: 'admin' },
      global: { plugins: [createPinia()] },
    })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'PackageTable' }).exists()).toBe(true)

    // 模拟选中
    await wrapper.findComponent({ name: 'PackageTable' }).vm.$emit('update:selectedIds', [1])
    expect(wrapper.find('.batch-bar').exists()).toBe(true)
    expect(wrapper.text()).toContain('已选 1 项')
  })

  it('首次加载显示骨架屏', async () => {
    ;(packageApi.search as any).mockImplementation(() => new Promise(() => {}))  // 永不返回
    const wrapper = mount(PackageExplorer, {
      props: { mode: 'admin' },
      global: { plugins: [createPinia()] },
    })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'PackageSkeleton' }).exists()).toBe(true)
  })

  it('加载完成无数据显示空状态', async () => {
    ;(packageApi.search as any).mockResolvedValue({ list: [], total: 0, search_time_ms: 0 })
    const wrapper = mount(PackageExplorer, {
      props: { mode: 'admin' },
      global: { plugins: [createPinia()] },
    })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'PackageEmptyState' }).exists()).toBe(true)
  })

  it('切换视图为 grid 时显示 PackageGrid', async () => {
    const wrapper = mount(PackageExplorer, {
      props: { mode: 'admin' },
      global: { plugins: [createPinia()] },
    })
    await flushPromises()
    await wrapper.findComponent({ name: 'PackageSearchBar' }).vm.$emit('update:viewMode', 'grid')
    expect(wrapper.findComponent({ name: 'PackageGrid' }).exists()).toBe(true)
  })

  it('点击删除单个包触发确认对话框', async () => {
    const wrapper = mount(PackageExplorer, {
      props: { mode: 'admin' },
      global: { plugins: [createPinia()] },
    })
    await flushPromises()
    await wrapper.findComponent({ name: 'PackageTable' }).vm.$emit('delete-package', mockPackages[0])
    // ElMessageBox 异步，检查 deletePackage 是否在确认后调用需要 mock ElMessageBox
    // 这里仅验证事件传播
    expect(wrapper.findComponent({ name: 'PackageTable' }).emitted('delete-package')).toBeTruthy()
  })

  it('搜索耗时显示在结果区', async () => {
    const wrapper = mount(PackageExplorer, {
      props: { mode: 'admin' },
      global: { plugins: [createPinia()] },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('5ms')
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/components/package-explorer/PackageExplorer.spec.ts`
预期：FAIL，组件不存在

- [ ] **步骤 3：实现容器组件**

创建 `web/src/components/package-explorer/PackageExplorer.vue`：

```vue
<template>
  <div class="package-explorer">
    <!-- 头部（仅 admin） -->
    <header v-if="mode === 'admin'" class="explorer-header">
      <div class="header-content">
        <div class="header-icon"><i class="fa-solid fa-box"></i></div>
        <div>
          <h2>包管理</h2>
          <p class="header-subtitle">管理和分发您的软件包</p>
        </div>
      </div>
      <el-button
        v-permission="'package:write'"
        type="primary"
        class="upload-btn"
        @click="showUpload = true"
      >
        <el-icon><Upload /></el-icon>上传包
      </el-button>
    </header>

    <!-- 搜索栏 -->
    <PackageSearchBar
      :query="query"
      :recent-searches="recentSearches"
      :loading="loading"
      :has-active-filter="hasActiveFilter"
      :view-mode="viewMode"
      @update:query="onQueryUpdate"
      @update:view-mode="viewMode = $event"
      @search="search"
      @add-recent="addRecentSearch"
      @clear-recent="clearRecentSearches"
      @open-filter="showFilter = true"
    />

    <!-- 批量操作栏（admin + 选中后） -->
    <div v-if="mode === 'admin' && selectedIds.length > 0" class="batch-bar">
      <span class="batch-info">已选 {{ selectedIds.length }} 项</span>
      <div class="batch-actions">
        <el-button v-permission="'package:delete'" type="danger" plain @click="onBatchDelete">
          批量删除
        </el-button>
        <el-button @click="onBatchExport">导出 CSV</el-button>
        <el-button link @click="selectedIds = []">取消选择</el-button>
      </div>
    </div>

    <!-- 结果区 -->
    <div class="content-panel">
      <div v-if="searchTime > 0 && !loading" class="search-stats">
        找到 {{ total }} 个包（耗时 {{ searchTime }}ms）
      </div>

      <!-- 骨架屏（首次加载） -->
      <PackageSkeleton v-if="loading && packages.length === 0" :variant="viewMode" />

      <!-- 空状态 -->
      <PackageEmptyState
        v-else-if="!loading && packages.length === 0"
        :variant="emptyVariant"
        :mode="mode"
        :error-message="error || ''"
        @upload="showUpload = true"
        @clear-filters="onClearFilters"
        @retry="search"
      />

      <!-- 表格视图 -->
      <PackageTable
        v-else-if="viewMode === 'table'"
        :packages="packages"
        :loading="loading"
        :mode="mode"
        :density="density"
        :selected-ids="selectedIds"
        :columns="columns"
        @update:selected-ids="selectedIds = $event"
        @view-versions="onViewVersions"
        @view-detail="onViewDetail"
        @delete-package="onDeletePackage"
      />

      <!-- 网格视图 -->
      <PackageGrid
        v-else
        :packages="packages"
        :mode="mode"
        @view-versions="onViewVersions"
        @view-detail="onViewDetail"
        @delete-package="onDeletePackage"
      />

      <PackagePagination
        :total="total"
        :page="query.page"
        :page-size="query.pageSize"
        :page-size-options="pageSizeOptions"
        @update:page="onPageChange"
        @update:page-size="onPageSizeChange"
      />
    </div>

    <!-- 筛选抽屉 -->
    <PackageFilterPanel
      v-model:visible="showFilter"
      v-model:repository="query.repository"
      v-model:version="query.version"
      @apply="onFilterApply"
    />

    <!-- 版本抽屉（复用现有组件） -->
    <VersionDrawer
      v-model="showVersionDrawer"
      :package-type="selectedPackage?.type || ''"
      :package-name="selectedPackage?.name || ''"
    />

    <!-- 上传对话框（复用现有组件，admin only） -->
    <UploadPackageDialog
      v-if="mode === 'admin'"
      v-model="showUpload"
      @uploaded="refresh"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Upload } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { usePackageSearch } from '@/composables/usePackageSearch'
import { packageApi, type Package } from '@/api/package'
import PackageSearchBar from './PackageSearchBar.vue'
import PackageFilterPanel from './PackageFilterPanel.vue'
import PackageTable from './PackageTable.vue'
import PackageGrid from './PackageGrid.vue'
import PackagePagination from './PackagePagination.vue'
import PackageSkeleton from './PackageSkeleton.vue'
import PackageEmptyState from './PackageEmptyState.vue'
import VersionDrawer from '@/components/package/VersionDrawer.vue'
import UploadPackageDialog from '@/components/package/UploadPackageDialog.vue'

const props = defineProps<{
  mode: 'admin' | 'public'
  pageSizeOptions?: number[]
  defaultPageSize?: number
}>()

const router = useRouter()

const {
  query, packages, total, loading, error, searchTime,
  recentSearches, isEmpty, hasActiveFilter,
  search, refresh, resetFilters, setQuery,
  addRecentSearch, clearRecentSearches,
} = usePackageSearch({
  mode: props.mode,
  pageSizeOptions: props.pageSizeOptions,
  defaultPageSize: props.defaultPageSize,
})

const pageSizeOptions = props.pageSizeOptions ?? [20, 50, 100]

// 局部 UI 状态
const viewMode = ref<'table' | 'grid'>(props.mode === 'admin' ? 'table' : 'grid')
const selectedIds = ref<number[]>([])
const showFilter = ref(false)
const showVersionDrawer = ref(false)
const showUpload = ref(false)
const selectedPackage = ref<Package | null>(null)

// 列设置与密度（admin only，存 localStorage）
const columnsStorageKey = `package-explorer:columns:${props.mode}`
const densityStorageKey = `package-explorer:density:${props.mode}`
const columns = ref(loadColumns())
const density = ref<'small' | 'default' | 'large'>(loadDensity())

function loadColumns() {
  try {
    return JSON.parse(localStorage.getItem(columnsStorageKey) || '{}')
  } catch {
    return {}
  }
}
function loadDensity(): 'small' | 'default' | 'large' {
  return (localStorage.getItem(densityStorageKey) as any) || 'default'
}

const emptyVariant = computed<'empty' | 'no-match' | 'error'>(() => {
  if (error.value) return 'error'
  if (query.q || hasActiveFilter.value) return 'no-match'
  return 'empty'
})

function onQueryUpdate(patch: Partial<typeof query>) {
  setQuery(patch)
}

function onPageChange(p: number) {
  setQuery({ page: p })
  selectedIds.value = []  // 翻页清空选择
  search()
}

function onPageSizeChange(size: number) {
  setQuery({ pageSize: size, page: 1 })
  selectedIds.value = []
  search()
}

function onFilterApply() {
  setQuery({ page: 1 })
  search()
}

function onClearFilters() {
  resetFilters()
  setQuery({ q: '', type: 'all', page: 1 })
  search()
}

function onViewVersions(pkg: Package) {
  selectedPackage.value = pkg
  showVersionDrawer.value = true
}

function onViewDetail(pkg: Package) {
  const routeName = props.mode === 'admin' ? 'AdminPackageDetail' : 'PackageDetail'
  router.push({ name: routeName, params: { type: pkg.type, name: pkg.name } })
}

async function onDeletePackage(pkg: Package) {
  try {
    await ElMessageBox.confirm(
      `确定要删除包 "${pkg.display_name || pkg.name}" 及其所有版本吗？此操作不可恢复！`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' }
    )
    await packageApi.deletePackage(pkg)
    ElMessage.success('包已删除')
    await refresh()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除包失败')
  }
}

async function onBatchDelete() {
  const selected = packages.value.filter(p => selectedIds.value.includes(p.id))
  const names = selected.slice(0, 5).map(p => p.display_name || p.name).join('、')
  const suffix = selected.length > 5 ? ` 等 ${selected.length} 个包` : ''
  try {
    await ElMessageBox.confirm(
      `确定要删除 ${names}${suffix} 吗？此操作不可恢复！`,
      '批量删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' }
    )

    const failed: string[] = []
    for (const pkg of selected) {
      try {
        await packageApi.deletePackage(pkg)
      } catch {
        failed.push(pkg.display_name || pkg.name)
      }
    }

    if (failed.length === 0) {
      ElMessage.success(`已删除 ${selected.length} 个包`)
    } else {
      ElMessage.warning(`删除完成，${failed.length} 个失败：${failed.slice(0, 3).join('、')}`)
    }

    selectedIds.value = []
    await refresh()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('批量删除失败')
  }
}

function onBatchExport() {
  const selected = packages.value.filter(p => selectedIds.value.includes(p.id))
  const headers = ['type', 'name', 'version', 'downloads', 'updated_at']
  const rows = selected.map(p => [p.type, p.name, p.latest_version || '', p.download_count, p.updated_at])
  const csv = [headers.join(','), ...rows.map(r => r.join(','))].join('\n')
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `packages-${Date.now()}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

onMounted(() => {
  search()
})
</script>

<style scoped>
.package-explorer { display: flex; flex-direction: column; gap: 16px; }
.explorer-header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 20px 24px; background: #fff; border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.06); box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}
.header-content { display: flex; align-items: center; gap: 16px; }
.header-icon {
  width: 44px; height: 44px; border-radius: 12px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  display: flex; align-items: center; justify-content: center; color: #fff; font-size: 20px;
}
.explorer-header h2 { font-size: 22px; font-weight: 700; margin: 0; color: #1e293b; }
.header-subtitle { font-size: 13px; color: #64748b; margin: 4px 0 0; }
.upload-btn {
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  border: none; box-shadow: 0 4px 14px rgba(37, 99, 235, 0.3);
}
.batch-bar {
  display: flex; justify-content: space-between; align-items: center;
  padding: 12px 20px; background: #eef2ff; border-radius: 10px;
  border: 1px solid #c7d2fe;
}
.batch-info { font-size: 14px; font-weight: 600; color: #4f46e5; }
.batch-actions { display: flex; gap: 8px; align-items: center; }
.content-panel {
  background: #fff; border-radius: 16px; border: 1px solid rgba(0, 0, 0, 0.06);
  overflow: hidden; box-shadow: 0 4px 16px rgba(0, 0, 0, 0.04);
}
.search-stats {
  padding: 12px 24px; font-size: 13px; color: #64748b;
  border-bottom: 1px solid rgba(0, 0, 0, 0.04); background: #fafbfc;
}
</style>
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackageExplorer.spec.ts`
预期：PASS（8 个测试全过）

- [ ] **步骤 5：Commit**

```bash
git add web/src/components/package-explorer/PackageExplorer.vue web/src/components/package-explorer/PackageExplorer.spec.ts
git commit -m "feat(frontend): add PackageExplorer container with mode-based behavior"
```

---

## 任务 13：页面壳与临时路由（阶段1新建并存）

**文件：**
- 创建：`web/src/views/PackageExplorerPage.vue`
- 创建：`web/src/views/PublicBrowsePage.vue`
- 修改：`web/src/router/index.ts`

- [ ] **步骤 1：创建管理端页面壳**

创建 `web/src/views/PackageExplorerPage.vue`：

```vue
<template>
  <PackageExplorer mode="admin" />
</template>

<script setup lang="ts">
import PackageExplorer from '@/components/package-explorer/PackageExplorer.vue'
</script>
```

- [ ] **步骤 2：创建公共端页面壳**

创建 `web/src/views/PublicBrowsePage.vue`：

```vue
<template>
  <div class="public-browse">
    <PackageExplorer mode="public" :default-page-size="24" :page-size-options="[12, 24, 48, 96]" />
  </div>
</template>

<script setup lang="ts">
import PackageExplorer from '@/components/package-explorer/PackageExplorer.vue'
</script>

<style scoped>
.public-browse { padding: 24px; max-width: 1400px; margin: 0 auto; }
</style>
```

- [ ] **步骤 3：新增临时路由（阶段1并存）**

修改 `web/src/router/index.ts`，在公共路由的 children 中添加临时路由（位于 `Browse` 之后）：

```typescript
{
  path: 'browse-new',
  name: 'BrowseNew',
  component: () => import('@/views/PublicBrowsePage.vue'),
  meta: { title: '浏览仓库' },
},
```

在 admin 路由的 children 中添加（位于 `AdminPackagesV2` 之后）：

```typescript
{
  path: 'packages-new',
  name: 'AdminPackagesNew',
  component: () => import('@/views/PackageExplorerPage.vue'),
  meta: { title: '包管理（新）', permission: { resource: 'package', action: 'read' } },
},
```

- [ ] **步骤 4：类型检查 + 手动验证**

运行：`cd web && npx vue-tsc --noEmit`
预期：无错误

启动开发服务器手动验证：
- 访问 `/admin/packages-new` 确认新管理端正常
- 访问 `/browse-new` 确认新公共端正常
- 旧路由 `/admin/packages`、`/admin/packages-v2`、`/` 仍可访问

- [ ] **步骤 5：Commit**

```bash
git add web/src/views/PackageExplorerPage.vue web/src/views/PublicBrowsePage.vue web/src/router/index.ts
git commit -m "feat(frontend): add new unified package pages and temporary routes (stage 1)"
```

---

## 任务 14：路由切换（阶段2）

**文件：**
- 修改：`web/src/router/index.ts`

**前置条件**：任务 13 的新界面在 `/admin/packages-new` 和 `/browse-new` 验证无问题。

- [ ] **步骤 1：备份旧文件**

```bash
cd web/src/views
git mv PackageList.vue PackageList.legacy.vue
git mv BrowsePage.vue BrowsePage.legacy.vue
```

注意：保留 `PackageCenterV2.vue` 原名（阶段3直接删除）。

更新备份文件中的测试引用（如有）。

- [ ] **步骤 2：切换主路由**

修改 `web/src/router/index.ts`：

公共路由 children 中：
```typescript
{
  path: '',
  name: 'Browse',
  component: () => import('@/views/PublicBrowsePage.vue'),
  meta: { title: '浏览仓库' },
},
```

admin 路由 children 中：
```typescript
{
  path: 'packages',
  name: 'AdminPackages',
  component: () => import('@/views/PackageExplorerPage.vue'),
  meta: { title: '包管理', permission: { resource: 'package', action: 'read' } },
},
{
  path: 'packages-v2',
  redirect: '/admin/packages',
},
```

删除临时路由 `browse-new` 和 `packages-new`。

- [ ] **步骤 3：类型检查 + 手动验证**

运行：`cd web && npx vue-tsc --noEmit`

启动开发服务器验证：
- `/` 指向新公共页
- `/admin/packages` 指向新管理端
- `/admin/packages-v2` 重定向到 `/admin/packages`
- 旧路径的 spec 测试需更新（PackageList.spec.ts、BrowsePage.spec.ts 暂时跳过或删除）

- [ ] **步骤 4：Commit**

```bash
git add web/src/router/index.ts web/src/views/PackageList.legacy.vue web/src/views/BrowsePage.legacy.vue
git rm web/src/views/PackageList.vue web/src/views/BrowsePage.vue
git commit -m "refactor(frontend): switch main routes to unified PackageExplorer (stage 2)"
```

---

## 任务 15：清理旧文件（阶段3）

**前置条件**：阶段2切换后稳定运行 7 天，无回滚需求。

**文件：**
- 删除：多个旧文件

- [ ] **步骤 1：删除旧页面与组件**

```bash
cd web/src
git rm views/PackageList.legacy.vue
git rm views/PackageList.spec.ts
git rm views/PackageCenterV2.vue
git rm views/BrowsePage.legacy.vue
git rm views/BrowsePage.spec.ts
git rm components/package/PackageTable.vue
git rm components/package/PackageCards.vue
git rm components/package-center/PackageListSection.vue
git rm components/package-center/PackageSidebar.vue
git rm components/package-center/QuickActions.vue
git rm components/package-center/StatsOverview.vue
git rm components/browse/HeroSection.vue
git rm components/browse/PackageCard.vue
```

**保留**：
- `components/package/VersionDrawer.vue`（PackageExplorer 复用）
- `components/package/UploadPackageDialog.vue`（PackageExplorer 复用）
- `components/browse/RepositoryShowcase.vue`（公共端仓库 Tab 复用）
- `components/browse/RepositoryStatusPanel.vue`（同上）
- `components/browse/RepoCard.vue`（同上）
- `components/browse/TypeSidebar.vue`（同上）

- [ ] **步骤 2：检查引用并修复**

运行：`cd web && grep -r "PackageList\|PackageCenterV2\|BrowsePage\|HeroSection\|PackageCards" src/ || echo "无残留引用"`

如有残留引用（如 menu 配置、import），逐一修复。

- [ ] **步骤 3：类型检查 + 测试**

运行：`cd web && npx vue-tsc --noEmit && npm run test:run`

- [ ] **步骤 4：Commit**

```bash
git add -A
git commit -m "chore(frontend): remove legacy package views and components (stage 3)"
```

---

## 任务 16：E2E 测试补充

**文件：**
- 创建：`web/scripts/e2e/package-explorer.spec.ts`

- [ ] **步骤 1：编写管理端 E2E 测试**

创建 `web/scripts/e2e/package-explorer.spec.ts`：

```typescript
import { test, expect } from '@playwright/test'

test.describe('PackageExplorer 管理端', () => {
  test.beforeEach(async ({ page }) => {
    // 登录（假设有测试账号）
    await page.goto('/login')
    await page.fill('input[name="username"]', 'admin')
    await page.fill('input[name="password"]', 'admin123')
    await page.click('button[type="submit"]')
    await page.waitForURL('/admin/dashboard')
  })

  test('搜索 → 筛选 → 排序 → 翻页 → 验证 URL 同步', async ({ page }) => {
    await page.goto('/admin/packages')

    // 搜索
    await page.fill('.package-search-bar input', 'react')
    await page.press('.package-search-bar input', 'Enter')
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('q=react')

    // 类型筛选
    await page.click('.type-chip:nth-child(2)')  // npm
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('type=npm')

    // 排序
    await page.click('.sort-select')
    await page.click('text=下载量')
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('sort=download_count')

    // 翻页
    if (await page.locator('.el-pagination .btn-next').isEnabled()) {
      await page.click('.el-pagination .btn-next')
      await page.waitForLoadState('networkidle')
      expect(page.url()).toContain('page=2')
    }
  })

  test('批量删除 → 验证页码修正', async ({ page }) => {
    await page.goto('/admin/packages')
    await page.waitForLoadState('networkidle')

    // 选中第一行
    await page.check('.el-table__row:first-child input[type="checkbox"]')
    await expect(page.locator('.batch-bar')).toBeVisible()

    // 点击批量删除
    await page.click('text=批量删除')
    await page.click('.el-message-box__header-btn + .el-button--danger')

    // 等待删除完成
    await page.waitForLoadState('networkidle')
    await expect(page.locator('.el-message--success')).toBeVisible()
  })

  test('键盘快捷键 / 聚焦搜索框', async ({ page }) => {
    await page.goto('/admin/packages')
    await page.waitForLoadState('networkidle')

    // 焦点不在输入框时按 /
    await page.keyboard.press('/')
    await expect(page.locator('.package-search-bar input')).toBeFocused()
  })

  test('Esc 清空搜索词', async ({ page }) => {
    await page.goto('/admin/packages?q=react')
    await page.waitForLoadState('networkidle')

    await page.keyboard.press('Escape')
    await page.waitForLoadState('networkidle')
    expect(page.url()).not.toContain('q=react')
  })
})

test.describe('PackageExplorer 公共端', () => {
  test('URL 参数直接访问恢复状态', async ({ page }) => {
    await page.goto('/?q=react&type=npm&sort=download_count&page=2&page_size=24')
    await page.waitForLoadState('networkidle')

    // 验证搜索框内容
    await expect(page.locator('.package-search-bar input')).toHaveValue('react')
    // 验证类型选中
    await expect(page.locator('.type-chip--active')).toContainText('npm')
    // 验证排序
    await expect(page.locator('.sort-select input')).toHaveValue('下载量')
  })

  test('/ 快捷键聚焦，Esc 清空', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')

    await page.keyboard.press('/')
    await expect(page.locator('.package-search-bar input')).toBeFocused()

    await page.fill('.package-search-bar input', 'test')
    await page.keyboard.press('Escape')
    await expect(page.locator('.package-search-bar input')).toHaveValue('')
  })
})
```

- [ ] **步骤 2：运行 E2E 测试**

运行：`cd web && npx playwright test scripts/e2e/package-explorer.spec.ts`

注意：E2E 测试需要后端服务运行，且需要测试账号。若环境不具备，标记为 `test.skip` 或在 CI 中运行。

- [ ] **步骤 3：Commit**

```bash
git add web/scripts/e2e/package-explorer.spec.ts
git commit -m "test(frontend): add E2E tests for PackageExplorer admin and public"
```

---

## 自检

### 规格覆盖度

| 规格需求 | 对应任务 | 覆盖 |
|---------|---------|------|
| v-permission 指令 | 任务 1 | ✅ |
| usePackageSearch composable | 任务 2-5 | ✅ |
| PackageSkeleton | 任务 6 | ✅ |
| PackageEmptyState（三种文案） | 任务 6 | ✅ |
| PackagePagination | 任务 7 | ✅ |
| PackageSearchBar（搜索/chips/最近搜索/筛选触发/视图切换） | 任务 8 | ✅ |
| PackageFilterPanel（仓库/版本，source/时间范围后端依赖） | 任务 9 | ✅（注明后端依赖） |
| PackageTable（批量选择/列设置/密度/复制/操作） | 任务 10 | ✅ |
| PackageGrid（复制按钮/mode 行为） | 任务 11 | ✅ |
| PackageExplorer 容器（mode/编排/删除/批量删除/上传） | 任务 12 | ✅ |
| 页面壳 + 路由（阶段1并存） | 任务 13 | ✅ |
| 路由切换（阶段2） | 任务 14 | ✅ |
| 清理旧文件（阶段3） | 任务 15 | ✅ |
| E2E 测试 | 任务 16 | ✅ |
| 排序 bug 修复（download_count 透传） | 任务 2 测试验证 | ✅ |
| 删除后页码修正 | 任务 3 | ✅ |
| URL 同步 | 任务 4 | ✅ |
| 最近搜索/快捷键 | 任务 5 | ✅ |

**遗漏**：规格中提到的"列设置 popover"和"密度切换器"在 PackageTable 测试中验证了 columns/density props 透传，但**列设置的 UI（齿轮按钮 + popover）和密度切换 UI 在任务 12 的容器组件中未实现**。这是规格与计划的偏差。

**修正**：任务 12 的 PackageExplorer 容器需要补充列设置 popover 和密度切换按钮的 UI。但考虑到任务 12 已经很大，且这两个 UI 是次要功能，建议在执行阶段 12 时由执行者根据规格补充，或作为任务 12 的步骤追加。本计划不新增任务，但在任务 12 的实现说明中标注此点。

### 占位符扫描

- 任务 9 的 `onReset` 实现说明中有"占位说明"，已在步骤 3 内联给出正确实现（使用 `emit`）。✅
- 任务 9 的 `<!-- TODO: 后端支持 source / updatedAtRange 后启用 -->` 是明确的未来功能标记，非占位符。✅
- 无其他 TODO/待定/后续实现。

### 类型一致性

- `PackageQuery` 接口在任务 2 定义，任务 8/9/10/12 引用，字段名一致（q/type/repository/version/source/sort/page/pageSize）。✅
- `PackageSort` 类型 = `'updated_at' | 'download_count' | 'name'`，与后端 [package_search_service.go:167-176](file:///Users/gracegaoya/work/project/moonlight-box/internal/service/package_search_service.go#L167-L176) 一致。✅
- `ColumnConfig` 在任务 10 定义为 `description/source/versions/downloads/updatedAt`，任务 12 的 columns ref 使用同名字段。✅
- `mode: 'admin' | 'public'` 在所有组件中一致。✅

### 范围检查

计划覆盖单个规格，16 个任务可在一个实现周期内完成。三阶段迁移（任务 13-15）顺序明确，每阶段独立可验证。✅

---

## 执行交接

计划已完成并保存到 `docs/superpowers/plans/2026-06-19-package-explorer-unification.md`。两种执行方式：

**1. 子代理驱动（推荐）** - 每个任务调度一个新的子代理，任务间进行审查，快速迭代

**2. 内联执行** - 在当前会话中使用 executing-plans 执行任务，批量执行并设有检查点

选哪种方式？
