# 公共浏览页 UX 改造实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将公共浏览页从"管理风列表页"改为"包仓库门户页"，把高级筛选从抽屉改为行内展开面板。

**架构：** PublicBrowsePage 独立编排公共端布局（Hero + 双 Tab + 行内筛选 + 网格），不再直接渲染完整 PackageExplorer。PackageFilterPanel 从 el-drawer 改为行内折叠面板。PackageSearchBar 增加 variant prop。公共端查询逻辑继续复用 usePackageSearch，不重复实现。

**技术栈：** Vue 3 + TypeScript + Element Plus + Vitest + @vue/test-utils + Playwright

---

### 任务 1：PackageFilterPanel 改为行内面板

**文件：**
- 修改：`web/src/components/package-explorer/PackageFilterPanel.vue`
- 修改：`web/src/components/package-explorer/PackageFilterPanel.spec.ts`

- [ ] **步骤 1：模板从 el-drawer 改为行内容器**

```vue
<template>
  <Transition name="filter-panel">
    <section v-if="visible" class="filter-panel-inline" data-test="filter-panel">
      <div class="filter-row">
        <div class="filter-section">
          <label class="filter-label">仓库</label>
          <el-select
            :model-value="repository"
            placeholder="全部仓库"
            clearable
            filterable
            size="default"
            @update:model-value="(v: string) => emit('update:repository', v || '')"
          >
            <el-option
              v-for="repo in repositories"
              :key="repo.id"
              :label="repo.name"
              :value="repo.name"
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
            placeholder="支持精确版本号或通配符，如 1.2.*"
            clearable
            size="default"
            @update:model-value="(v: string) => emit('update:version', v)"
          />
        </div>

        <div class="filter-actions">
          <el-button data-test="reset" size="default" @click="onReset">重置</el-button>
          <el-button data-test="apply" type="primary" size="default" @click="emit('apply')">应用</el-button>
        </div>
      </div>
    </section>
  </Transition>
</template>
```

- [ ] **步骤 2：脚本保持现有逻辑不变**（props、emits、repositories 加载、onReset）

- [ ] **步骤 3：添加行内面板样式**，替换旧的抽屉样式（`el-drawer` 相关删除，`filter-panel-inline` 用 flex row 布局，`Transition` name 使用折叠动画）

```css
.filter-panel-inline {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 14px;
  padding: 16px 20px;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
}
.filter-row {
  display: flex;
  gap: 16px;
  align-items: flex-end;
  flex-wrap: wrap;
}
.filter-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1;
  min-width: 200px;
}
.filter-label {
  font-size: 12px;
  font-weight: 600;
  color: #475569;
}
.filter-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}
/* 展开/收起动画 */
.filter-panel-enter-active,
.filter-panel-leave-active {
  transition: all 0.25s ease;
}
.filter-panel-enter-from,
.filter-panel-leave-to {
  opacity: 0;
  transform: translateY(-8px);
  max-height: 0;
  margin-bottom: 0;
}
.filter-panel-enter-to,
.filter-panel-leave-from {
  opacity: 1;
  transform: translateY(0);
}
```

- [ ] **步骤 4：更新 PackageFilterPanel.spec.ts**

现有 5 个测试大部分仍然有效，但需调整：
  - 去掉对 el-drawer 的依赖（不再需要 drawer 相关测试）
  - `visible=false` 时 `.filter-panel-inline` 不应存在 → 新增测试
  - 仓库列表加载、update:repository、update:version、重置、应用保持
  
```typescript
it('visible=false 时不渲染面板', () => {
  const wrapper = mount(PackageFilterPanel, {
    props: { visible: false, repository: '', version: '' },
    global: { plugins: [ElementPlus] },
  })
  expect(wrapper.find('.filter-panel-inline').exists()).toBe(false)
})

it('visible=true 时渲染面板', () => {
  const wrapper = mount(PackageFilterPanel, {
    props: { visible: true, repository: '', version: '' },
    global: { plugins: [ElementPlus] },
  })
  expect(wrapper.find('.filter-panel-inline').exists()).toBe(true)
})
```

其余测试（仓库列表加载、update:repository、update:version、重置、应用事件）保持原样，仅调整选择器从 `[data-test="reset"]` 等继续有效即可。

- [ ] **步骤 5：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackageFilterPanel.spec.ts`
预期：PASS

- [ ] **步骤 6：Commit**

```bash
git add web/src/components/package-explorer/PackageFilterPanel.vue web/src/components/package-explorer/PackageFilterPanel.spec.ts
git commit -m "refactor(frontend): change PackageFilterPanel from drawer to inline panel"
```

---

### 任务 2：PackageSearchBar 增加 variant prop

**文件：**
- 修改：`web/src/components/package-explorer/PackageSearchBar.vue`
- 修改：`web/src/components/package-explorer/PackageSearchBar.spec.ts`

- [ ] **步骤 1：添加 props 中的 variant**

```typescript
const props = defineProps<{
  query: PackageQuery
  recentSearches: string[]
  loading: boolean
  hasActiveFilter: boolean
  viewMode: 'table' | 'grid'
  variant?: 'default' | 'hero'  // 新增
}>()
```

- [ ] **步骤 2：模板根元素添加 dynamic class**

```diff
- <div class="package-search-bar">
+ <div class="package-search-bar" :class="{ 'package-search-bar--hero': variant === 'hero' }">
```

- [ ] **步骤 3：添加 hero 样式**

```css
.package-search-bar--hero {
  flex-direction: column;
  align-items: stretch;
  gap: 0;
  padding: 0;
  background: transparent;
  border: none;
}
.package-search-bar--hero .search-wrapper {
  max-width: 600px;
  margin: 0 auto;
}
.package-search-bar--hero .search-input {
  /* 大搜索框已在 Hero 中控制宽度 */
}
.package-search-bar--hero .type-chips {
  justify-content: center;
  margin-top: 16px;
}
.package-search-bar--hero .toolbar-actions {
  /* 在 hero 模式中隐藏排序和视图切换，由下方 PackageSearchBar 第二次实例化负责 */
  display: none;
}
```

注意：PublicBrowsePage 中 Hero 里的 `PackageSearchBar variant="hero"` 用于搜索框和类型 chips。排序/筛选/视图切换在 Hero 下方的第二个 `PackageSearchBar variant="default"` 中。

- [ ] **步骤 4：更新 PackageSearchBar.spec.ts**

新增测试：

```typescript
it('hero variant 添加 hero class', () => {
  const wrapper = mountIt({ variant: 'hero' })
  expect(wrapper.find('.package-search-bar--hero').exists()).toBe(true)
})

it('default variant 不添加 hero class', () => {
  const wrapper = mountIt()
  expect(wrapper.find('.package-search-bar--hero').exists()).toBe(false)
})
```

- [ ] **步骤 5：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackageSearchBar.spec.ts`
预期：PASS（9 个测试）

- [ ] **步骤 6：Commit**

```bash
git add web/src/components/package-explorer/PackageSearchBar.vue web/src/components/package-explorer/PackageSearchBar.spec.ts
git commit -m "feat(frontend): add variant prop to PackageSearchBar for hero mode"
```

---

### 任务 3：创建 PublicPackageHero 组件

**文件：**
- 创建：`web/src/components/package-explorer/PublicPackageHero.vue`

- [ ] **步骤 1：创建组件模板**

```vue
<template>
  <div class="public-hero">
    <div class="hero-content">
      <h1 class="hero-title">Moonlight Box</h1>
      <p class="hero-subtitle">企业级多协议包仓库 · 搜索和浏览软件包</p>
      <div class="hero-search">
        <slot name="search" />
      </div>
      <div v-if="total !== undefined && !loading" class="hero-stats">
        <span class="stat-badge">{{ formatNumber(total) }} 个包可搜索</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { formatNumber } from '@/utils/format'

defineProps<{
  total?: number
  loading?: boolean
}>()
</script>

<style scoped>
.public-hero {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  padding: 48px 24px 40px;
  text-align: center;
  color: #fff;
}
.hero-content {
  max-width: 720px;
  margin: 0 auto;
}
.hero-title {
  font-size: 32px;
  font-weight: 800;
  margin: 0 0 8px;
  letter-spacing: -0.5px;
}
.hero-subtitle {
  font-size: 15px;
  opacity: 0.86;
  margin: 0 0 24px;
  line-height: 1.5;
}
.hero-search {
  max-width: 560px;
  margin: 0 auto;
}
.hero-stats {
  margin-top: 16px;
  font-size: 13px;
  opacity: 0.8;
}
.stat-badge {
  display: inline-block;
  padding: 4px 14px;
  background: rgba(255, 255, 255, 0.15);
  border-radius: 999px;
  backdrop-filter: blur(4px);
}
</style>
```

- [ ] **步骤 2：运行类型检查**

运行：`cd web && npx vue-tsc --noEmit`
预期：无错误

- [ ] **步骤 3：Commit**

```bash
git add web/src/components/package-explorer/PublicPackageHero.vue
git commit -m "feat(frontend): create PublicPackageHero component for public portal header"
```

---

### 任务 4：创建 PublicBrowseTabs 组件

**文件：**
- 创建：`web/src/components/package-explorer/PublicBrowseTabs.vue`
- 创建：`web/src/components/package-explorer/PublicBrowseTabs.spec.ts`

- [ ] **步骤 1：编写测试**

```typescript
// PublicBrowseTabs.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PublicBrowseTabs from './PublicBrowseTabs.vue'

const mountIt = (props: Record<string, any> = {}) => mount(PublicBrowseTabs, {
  props: { activeTab: 'packages', ...props },
})

describe('PublicBrowseTabs', () => {
  it('默认显示两个 Tab', () => {
    const wrapper = mountIt()
    const tabs = wrapper.findAll('.browse-tab')
    expect(tabs).toHaveLength(2)
  })

  it('activeTab=packages 时包 Tab 高亮', () => {
    const wrapper = mountIt({ activeTab: 'packages' })
    const tabs = wrapper.findAll('.browse-tab')
    expect(tabs[0].classes()).toContain('browse-tab--active')
    expect(tabs[1].classes()).not.toContain('browse-tab--active')
  })

  it('点击仓库 Tab 触发 update:activeTab', async () => {
    const wrapper = mountIt()
    await wrapper.findAll('.browse-tab')[1].trigger('click')
    expect(wrapper.emitted('update:activeTab')?.[0]).toEqual(['repositories'])
  })

  it('显示包数量统计', () => {
    const wrapper = mountIt({ packageCount: 128 })
    expect(wrapper.text()).toContain('128')
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/components/package-explorer/PublicBrowseTabs.spec.ts`
预期：FAIL（组件尚未创建）

- [ ] **步骤 3：创建组件**

```vue
<template>
  <div class="browse-tabs">
    <button
      class="browse-tab"
      :class="{ 'browse-tab--active': activeTab === 'packages' }"
      @click="$emit('update:activeTab', 'packages')"
    >
      <el-icon><Box /></el-icon>
      包
      <span v-if="packageCount !== undefined" class="tab-count">{{ packageCount }}</span>
    </button>
    <button
      class="browse-tab"
      :class="{ 'browse-tab--active': activeTab === 'repositories' }"
      @click="$emit('update:activeTab', 'repositories')"
    >
      <el-icon><FolderOpened /></el-icon>
      仓库
    </button>
  </div>
</template>

<script setup lang="ts">
import { Box, FolderOpened } from '@element-plus/icons-vue'

defineProps<{
  activeTab: 'packages' | 'repositories'
  packageCount?: number
}>()

defineEmits<{
  'update:activeTab': ['packages' | 'repositories']
}>()
</script>

<style scoped>
.browse-tabs {
  display: flex;
  gap: 0;
  padding: 0 24px;
  border-bottom: 2px solid #e2e8f0;
  background: #fff;
}
.browse-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 13px 20px;
  font-size: 14px;
  font-weight: 600;
  color: #64748b;
  border: none;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  background: transparent;
  cursor: pointer;
  transition: all 0.2s;
}
.browse-tab:hover { color: #4f46e5; }
.browse-tab--active {
  color: #4f46e5;
  border-bottom-color: #4f46e5;
}
.tab-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  font-size: 11px;
  font-weight: 700;
  background: #eef2ff;
  color: #4f46e5;
  border-radius: 999px;
}
</style>
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PublicBrowseTabs.spec.ts`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add web/src/components/package-explorer/PublicBrowseTabs.vue web/src/components/package-explorer/PublicBrowseTabs.spec.ts
git commit -m "feat(frontend): create PublicBrowseTabs component for package/repo tab switching"
```

---

### 任务 5：PublicBrowsePage 重写为独立编排

**文件：**
- 修改：`web/src/views/PublicBrowsePage.vue`
- 创建：`web/src/views/PublicBrowsePage.spec.ts`

- [ ] **步骤 1：编写测试**

```typescript
// PublicBrowsePage.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import PublicBrowsePage from './PublicBrowsePage.vue'
import { packageApi } from '@/api/package'
import { repositoryApi } from '@/api/repository'

const push = vi.fn()
const replace = vi.fn()

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRouter: () => ({ push, replace }),
    useRoute: () => ({ query: {} }),
  }
})

vi.mock('@/api/package', () => ({
  packageApi: { search: vi.fn().mockResolvedValue({ items: [], total: 0, search_time_ms: 2 }) },
}))

vi.mock('@/api/repository', () => ({
  repositoryApi: { list: vi.fn().mockResolvedValue([]) },
}))

const mountPage = () => mount(PublicBrowsePage, {
  global: {
    plugins: [ElementPlus],
    stubs: {
      PackageSearchBar: { template: '<div class="package-search-bar"><input /></div>' },
      PackageFilterPanel: { template: '<div class="filter-panel-inline" />' },
      PackageGrid: { template: '<div class="package-card" />' },
      PackagePagination: { template: '<div class="el-pagination" />' },
      PublicPackageHero: { template: '<div class="public-hero"><slot name="search" /></div>' },
      PublicBrowseTabs: { template: '<div class="browse-tabs"><button class="browse-tab--active" /></div>' },
      RepositoryShowcase: { template: '<div />' },
    },
  },
})

describe('PublicBrowsePage', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('渲染 Hero', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.find('.public-hero').exists()).toBe(true)
  })

  it('默认显示包 Tab', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.find('.package-card').exists()).toBe(true)
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web && npx vitest run src/views/PublicBrowsePage.spec.ts`
预期：FAIL（页面尚未重写）

- [ ] **步骤 3：重写 PublicBrowsePage.vue**

```vue
<template>
  <div class="public-browse">
    <!-- Hero -->
    <PublicPackageHero :total="total" :loading="loading">
      <template #search>
        <PackageSearchBar
          variant="hero"
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
          @open-filter="showFilter = !showFilter"
        />
      </template>
    </PublicPackageHero>

    <!-- 双 Tab -->
    <PublicBrowseTabs
      v-model:active-tab="activeTab"
      :package-count="total"
    />

    <!-- 包 Tab -->
    <template v-if="activeTab === 'packages'">
      <div class="packages-section">
        <!-- 搜索栏（第二行：排序/筛选/视图切换） -->
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
          @open-filter="showFilter = !showFilter"
        />

        <!-- 行内筛选面板 -->
        <PackageFilterPanel
          v-show="showFilter"
          :visible="showFilter"
          v-model:repository="query.repository"
          v-model:version="query.version"
          @apply="onFilterApply"
          @reset="onFilterApply"
        />

        <!-- 统计 -->
        <div v-if="searchTime > 0 && !loading && packages.length > 0" class="search-stats">
          找到 {{ total }} 个包（耗时 {{ searchTime }}ms）
        </div>

        <!-- 骨架屏 -->
        <div v-if="loading && packages.length === 0" class="grid-container">
          <div v-for="i in 6" :key="i" class="skeleton-card" />
        </div>

        <!-- 空状态 -->
        <PackageEmptyState
          v-else-if="!loading && packages.length === 0"
          :variant="emptyVariant"
          mode="public"
          :error-message="error || ''"
          @clear-filters="onClearFilters"
          @retry="search"
        />

        <!-- 网格 -->
        <div v-else class="grid-container">
          <PackageGrid
            :packages="packages"
            mode="public"
            @view-detail="onViewDetail"
            @view-versions="onViewVersions"
          />
        </div>

        <!-- 分页 -->
        <div class="pagination-wrapper">
          <PackagePagination
            :total="total"
            :page="query.page"
            :page-size="query.pageSize"
            :page-size-options="pageSizeOptions"
            @update:page="onPageChange"
            @update:page-size="onPageSizeChange"
          />
        </div>
      </div>
    </template>

    <!-- 仓库 Tab -->
    <template v-else>
      <div class="repos-section">
        <div class="repos-main">
          <RepositoryShowcase />
        </div>
        <aside class="repos-sidebar">
          <RepositoryStatusPanel />
        </aside>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { usePackageSearch, type PackageQuery } from '@/composables/usePackageSearch'
import type { Package } from '@/api/package'
import PublicPackageHero from '@/components/package-explorer/PublicPackageHero.vue'
import PublicBrowseTabs from '@/components/package-explorer/PublicBrowseTabs.vue'
import PackageSearchBar from '@/components/package-explorer/PackageSearchBar.vue'
import PackageFilterPanel from '@/components/package-explorer/PackageFilterPanel.vue'
import PackageGrid from '@/components/package-explorer/PackageGrid.vue'
import PackagePagination from '@/components/package-explorer/PackagePagination.vue'
import PackageEmptyState from '@/components/package-explorer/PackageEmptyState.vue'
import RepositoryShowcase from '@/components/browse/RepositoryShowcase.vue'
import RepositoryStatusPanel from '@/components/browse/RepositoryStatusPanel.vue'

const {
  query, packages, total, loading, error, searchTime,
  recentSearches, hasActiveFilter,
  search, refresh, resetFilters, setQuery,
  addRecentSearch, clearRecentSearches,
} = usePackageSearch({
  mode: 'public',
  defaultPageSize: 24,
  pageSizeOptions: [12, 24, 48, 96],
})

const pageSizeOptions = [12, 24, 48, 96]
const activeTab = ref<'packages' | 'repositories'>('packages')
const showFilter = ref(false)
const viewMode = ref<'table' | 'grid'>('grid')
const router = useRouter()

const emptyVariant = computed<'empty' | 'no-match' | 'error'>(() => {
  if (error.value) return 'error'
  if (query.q || hasActiveFilter.value) return 'no-match'
  return 'empty'
})

function onQueryUpdate(patch: Partial<PackageQuery>) {
  setQuery(patch)
}

function onPageChange(p: number) {
  setQuery({ page: p })
  search()
}

function onPageSizeChange(size: number) {
  setQuery({ pageSize: size, page: 1 })
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

function onViewDetail(pkg: Package) {
  router.push({ name: 'PackageDetail', params: { type: pkg.type, name: pkg.name } })
}

function onViewVersions(pkg: Package) {
  // 公共端暂不实现版本抽屉
}

onMounted(() => {
  search()
})
</script>

<style scoped>
.public-browse {
  min-height: 100vh;
  background: #f8fafc;
}
.packages-section {
  padding: 16px 24px 32px;
  max-width: 1400px;
  margin: 0 auto;
}
.search-stats {
  font-size: 13px;
  color: #64748b;
  margin: 8px 0 16px;
}
.grid-container {
  padding: 0;
}
.skeleton-card {
  height: 140px;
  background: linear-gradient(90deg, #f1f5f9 25%, #e2e8f0 50%, #f1f5f9 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: 12px;
  margin-bottom: 16px;
}
@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
.pagination-wrapper {
  margin-top: 16px;
}
.repos-section {
  display: flex;
  gap: 24px;
  padding: 24px;
  max-width: 1400px;
  margin: 0 auto;
}
.repos-main { flex: 1; min-width: 0; }
.repos-sidebar { width: 320px; flex-shrink: 0; }
</style>
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web && npx vitest run src/views/PublicBrowsePage.spec.ts`
预期：PASS

- [ ] **步骤 5：运行完整测试**

运行：`cd web && npx vitest run`
预期：所有测试通过（17+ 个文件）

- [ ] **步骤 6：类型检查**

运行：`cd web && npx vue-tsc --noEmit`
预期：无错误

- [ ] **步骤 7：Commit**

```bash
git add web/src/views/PublicBrowsePage.vue web/src/views/PublicBrowsePage.spec.ts
git commit -m "feat(frontend): rewrite PublicBrowsePage with hero, tabs, and inline filter panel"
```

---

### 任务 6：PackageExplorer 调整筛选面板位置

**文件：**
- 修改：`web/src/components/package-explorer/PackageExplorer.vue`
- 修改：`web/src/components/package-explorer/PackageExplorer.spec.ts`

- [ ] **步骤 1：更新 PackageExplorer 模板**，将 PackageFilterPanel 从抽屉位置移动到搜索栏下方

在 `PackageSearchBar` 之后、`PackageFilterPanel` 之前，调整位置：

```diff
    <!-- 搜索栏 -->
    <PackageSearchBar ... @open-filter="showFilter = true" />

+   <!-- 行内筛选面板 -->
+   <PackageFilterPanel
+     v-show="showFilter"
+     :visible="showFilter"
+     v-model:repository="query.repository"
+     v-model:version="query.version"
+     @apply="onFilterApply"
+     @reset="onFilterApply"
+   />

    <!-- 批量操作栏（admin + 选中后） -->
    ...
```

删除旧的抽屉用法：
```diff
-   <!-- 筛选抽屉 -->
-   <PackageFilterPanel
-     v-model:visible="showFilter"
-     v-model:repository="query.repository"
-     v-model:version="query.version"
-     @apply="onFilterApply"
-   />
```

- [ ] **步骤 2：更新 PackageExplorer.spec.ts**

验证筛选面板现在在搜索栏下方渲染，而非抽屉。由于测试使用了 stub，主要是确认组件仍正确传递 props：

```typescript
// 验证筛选面板 props 传递（在已有测试中补充）
it('传递筛选面板状态', async () => {
  const wrapper = mount(PackageExplorer as any, {
    props: { mode: 'admin' },
    global: {
      plugins: [ElementPlus, pinia],
      directives: { permission },
      stubs: {
        PackageSearchBar: { template: '<div class="stub-search-bar" />' },
        PackageFilterPanel: { template: '<div class="stub-filter-panel" />' },
        PackageTable: { template: '<div />' },
        PackageEmptyState: { template: '<div />' },
        PackagePagination: { template: '<div />' },
        VersionDrawer: { template: '<div />' },
        UploadPackageDialog: { template: '<div />' },
      },
    },
  })
  // 筛选面板 stub 存在
  expect(wrapper.find('.stub-filter-panel').exists()).toBe(true)
})
```

- [ ] **步骤 3：运行测试验证通过**

运行：`cd web && npx vitest run src/components/package-explorer/PackageExplorer.spec.ts`
预期：PASS

- [ ] **步骤 4：完整测试**

运行：`cd web && npx vitest run`
预期：全部 PASS

- [ ] **步骤 5：Commit**

```bash
git add web/src/components/package-explorer/PackageExplorer.vue web/src/components/package-explorer/PackageExplorer.spec.ts
git commit -m "refactor(frontend): move PackageFilterPanel from drawer to inline in PackageExplorer"
```

---

### 任务 7：更新 E2E 测试与 .gitignore

**文件：**
- 修改：`web/e2e/package-explorer.spec.ts`
- 修改：`.gitignore`

- [ ] **步骤 1：更新公共端 E2E 测试断言**

修改 `web/e2e/package-explorer.spec.ts` 中公共端测试部分：

```typescript
test.describe('PackageExplorer 公共端', () => {
  test('URL 参数直接访问恢复状态', async ({ page }) => {
    await page.goto('/?q=react&type=npm&sort=download_count&page=2&page_size=24')
    await page.waitForLoadState('networkidle')

    // 验证 Hero 存在
    await expect(page.locator('.public-hero')).toBeVisible()
    // 验证搜索框内容（Hero 中的搜索框）
    await expect(page.locator('.package-search-bar input').first()).toHaveValue('react')
    // 验证类型选中（Hero 下方的 chips）
    await expect(page.locator('.type-chip--active')).toContainText('npm')
  })

  test('包/仓库 Tab 切换', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')

    // 默认在包 Tab
    await expect(page.locator('.browse-tab--active')).toContainText('包')

    // 点击仓库 Tab
    await page.click('.browse-tab:last-child')
    await expect(page.locator('.repos-section')).toBeVisible()
  })

  test('筛选面板行内展开', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')

    // 点击筛选按钮
    await page.click('.el-badge .el-button')
    await expect(page.locator('.filter-panel-inline')).toBeVisible()
  })

  test('/ 快捷键聚焦，Esc 清空', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')

    await page.keyboard.press('/')
    await expect(page.locator('.package-search-bar input').first()).toBeFocused()

    await page.fill('.package-search-bar input', 'test')
    await page.keyboard.press('Escape')
    await expect(page.locator('.package-search-bar input').first()).toHaveValue('')
  })
})
```

注意：管理端测试部分保持不动（重复提交筛选面板体验）。

- [ ] **步骤 2：添加 .superpowers/ 到 .gitignore**

```bash
# Brainstorming visual companion files
.superpowers/
```

添加到 `.gitignore` 末尾。

- [ ] **步骤 3：类型检查**

运行：`cd web && npx vue-tsc --noEmit`
预期：无错误

- [ ] **步骤 4：Commit**

```bash
git add web/e2e/package-explorer.spec.ts .gitignore
git commit -m "test(frontend): update E2E for public browse and add .superpowers to gitignore"
```

---

### 任务 8：最终验证

- [ ] **步骤 1：完整测试套件**

运行：`cd web && npx vitest run`
预期：全部 PASS

- [ ] **步骤 2：类型检查**

运行：`cd web && npx vue-tsc --noEmit`
预期：无错误

- [ ] **步骤 3：确认已完成的 commit 列表**

```bash
git log --oneline -8
```

预期展示 7 个有意义 commit（加上可能已有的 commit）。

---