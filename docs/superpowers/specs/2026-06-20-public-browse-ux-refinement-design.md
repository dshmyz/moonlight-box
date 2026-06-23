# 公共浏览页 UX 改造设计

## 背景

包管理查询界面统一后，公共端 `/` 直接使用 `PackageExplorer mode="public"`。虽然查询逻辑统一了，但视觉和交互仍偏管理后台：页面缺少门户感，高级筛选以右侧抽屉呈现，只有仓库和版本两个字段却遮挡列表，体验较重。

本设计在不影响管理端核心能力的前提下，改善公共浏览页视觉层次，并把高级筛选改为行内展开面板。

## 目标

1. 公开页面从“管理风列表页”改为“包仓库门户页”。
2. 高级筛选不再使用抽屉，改为搜索区域下方的行内展开面板。
3. 公共端恢复“包 / 仓库”双 Tab，保留仓库展示能力。
4. 查询、分页、排序、URL 同步、快捷键、最近搜索继续复用 `usePackageSearch`，不重复实现业务逻辑。
5. 管理端 `/admin/packages` 行为保持稳定，只接受筛选面板交互改善。

## 非目标

- 不新增新的后端筛选字段。
- 不改变包搜索 API 参数契约。
- 不把公共端 Tab 状态写入 URL。
- 不重做 PackageGrid 卡片数据结构。
- 不改动权限模型、删除逻辑、上传逻辑。

## 视觉方向

公共端采用门户式结构：

```text
Hero 区
  标题 / 副标题 / 大搜索框

包 / 仓库 Tab

包 Tab
  类型 chips / 排序 / 筛选按钮
  行内筛选面板（展开时）
  包网格
  分页

仓库 Tab
  RepositoryShowcase
  RepositoryStatusPanel
```

视觉原型位于：

`/.superpowers/brainstorm/2264-1781931974/layout-compare-standalone.html`

该文件仅用于设计讨论，不应提交到仓库。

## 架构设计

### PublicBrowsePage 独立编排公共端

`web/src/views/PublicBrowsePage.vue` 不再直接渲染完整 `PackageExplorer`，而是编排公共端专属布局。

职责：

- 管理 `activeTab: 'packages' | 'repositories'`
- 渲染公共端 Hero
- 渲染包 / 仓库 Tab
- 在包 Tab 中组合搜索栏、行内筛选、网格、分页、空状态、骨架屏
- 在仓库 Tab 中复用仓库展示组件

公共端仍使用：

```ts
usePackageSearch({
  mode: 'public',
  defaultPageSize: 24,
  pageSizeOptions: [12, 24, 48, 96],
})
```

### PackageExplorer 保持管理端容器职责

`web/src/components/package-explorer/PackageExplorer.vue` 继续服务管理端 `/admin/packages`。

它不再承担公共端视觉布局职责，避免继续增加公共端专用 props 导致组件复杂化。

管理端仍复用：

- `PackageSearchBar`
- `PackageFilterPanel`
- `PackageTable`
- `PackageGrid`
- `PackagePagination`
- `PackageSkeleton`
- `PackageEmptyState`

### 新增公共端轻量组件

#### PublicPackageHero.vue

职责：展示公共端门户标题和搜索区域。

建议位置：

`web/src/components/package-explorer/PublicPackageHero.vue`

输入：

```ts
interface Props {
  total?: number
  loading?: boolean
}
```

插槽：

```text
search 插槽：放置 PackageSearchBar 的 hero variant
```

#### PublicBrowseTabs.vue

职责：公共端包 / 仓库 Tab 切换。

建议位置：

`web/src/components/package-explorer/PublicBrowseTabs.vue`

Props：

```ts
activeTab: 'packages' | 'repositories'
packageCount?: number
```

Emits：

```ts
'update:activeTab': ['packages' | 'repositories']
```

## 行内筛选面板设计

### PackageFilterPanel 改造

`web/src/components/package-explorer/PackageFilterPanel.vue` 从 `el-drawer` 改为普通行内容器。

保留现有 props / emits：

```ts
props:
  visible: boolean
  repository: string
  version: string

emits:
  update:visible
  update:repository
  update:version
  apply
  reset
```

DOM 结构：

```vue
<Transition name="filter-panel">
  <section v-if="visible" class="filter-panel-inline">
    <div class="filter-section">仓库 Select</div>
    <div class="filter-section">版本 Input</div>
    <div class="filter-actions">重置 / 应用</div>
  </section>
</Transition>
```

交互：

- 点击搜索栏中的“筛选”按钮：父组件切换 `showFilter`
- 面板展开在搜索区域下方
- 修改仓库 / 版本：只更新 query 字段
- 点击“应用”：触发 `apply`
- 点击“重置”：清空仓库和版本，并触发 `reset`

当前版本保持“应用后搜索”的语义，不改为即时搜索，以降低行为变化风险。

### 父组件放置位置

管理端：

```text
PackageSearchBar
PackageFilterPanel（展开时）
BatchBar
ContentPanel
```

公共端：

```text
Hero
Tabs
PackageSearchBar
PackageFilterPanel（展开时）
PackageGrid / Empty / Skeleton
Pagination
```

## PackageSearchBar variant 设计

为 `PackageSearchBar` 增加：

```ts
variant?: 'default' | 'hero'
```

默认值：`default`。

行为不变，仅影响样式：

- `default`：保持现有管理端紧凑工具栏样式
- `hero`：搜索框更大、更居中，按钮和 chips 更适合公共端门户视觉

公共端 Hero 里使用：

```vue
<PackageSearchBar variant="hero" ... />
```

保留 `.package-search-bar input` 选择器，避免破坏现有 E2E。

## 公共端数据流

### 包 Tab

包 Tab 使用 `usePackageSearch` 暴露的状态：

- `query`
- `packages`
- `total`
- `loading`
- `error`
- `searchTime`
- `recentSearches`
- `hasActiveFilter`

操作映射：

- 搜索：`setQuery` + `search`
- 类型：`setQuery({ type, page: 1 })` + `search`
- 排序：`setQuery({ sort, page: 1 })` + `search`
- 分页：`setQuery({ page })` + `search`
- 每页数量：`setQuery({ pageSize, page: 1 })` + `search`
- 清空筛选：`resetFilters` + `search`

### 仓库 Tab

仓库 Tab 不触发包搜索。

渲染：

- `RepositoryShowcase`
- `RepositoryStatusPanel`

### URL 同步

继续由 `usePackageSearch` 负责：

- `q`
- `type`
- `sort`
- `page`
- `page_size`
- `repo`
- `version`

`activeTab` 不写入 URL。

原因：Tab 是公共端展示状态，不属于包查询条件；引入 `tab` 会扩大 URL 契约和测试范围，当前收益不足。

## 测试设计

### PackageFilterPanel.spec.ts

调整为行内面板测试：

- `visible=false` 时不渲染面板
- `visible=true` 时渲染仓库和版本字段
- 点击应用 emit `apply`
- 点击重置 emit `update:repository('')`、`update:version('')`、`reset`
- 仓库列表加载仍可工作

### PackageSearchBar.spec.ts

新增：

- `variant="hero"` 时添加 hero 样式 class
- 默认 variant 不改变现有结构
- 筛选按钮仍 emit `open-filter`
- `.package-search-bar input` 仍存在

### PublicBrowsePage.spec.ts

新增或调整：

- 默认展示包 Tab
- 切换到仓库 Tab 后显示仓库相关组件
- 包 Tab 中渲染 `PackageSearchBar`、`PackageFilterPanel`、`PackageGrid`、`PackagePagination`
- URL 参数恢复仍由 `usePackageSearch` 生效

### E2E

更新公共端断言：

- `/` 显示 Hero
- 默认在包 Tab
- 点击筛选后显示行内筛选面板
- 点击仓库 Tab 后显示仓库内容
- `/` 快捷键仍聚焦 `.package-search-bar input`
- `Esc` 仍清空搜索词

## 迁移策略

一次性改造公共端与筛选面板：

1. 新增公共端轻量组件。
2. 改造 `PackageFilterPanel` 为行内面板。
3. `PackageExplorer` 管理端将筛选面板放到搜索栏下方。
4. `PublicBrowsePage` 改为专属公共端编排。
5. 调整单元测试和 E2E。

不新增临时路由。

## 风险与控制

### 风险 1：PackageSearchBar variant 影响管理端

控制：`variant` 默认 `default`；管理端不传该 prop，现有结构和样式保持。

### 风险 2：筛选面板从 drawer 改为行内后影响管理端布局

控制：面板只在 `showFilter=true` 时展开，位置在搜索栏下方；不会遮挡表格，也不改变批量操作逻辑。

### 风险 3：公共端独立编排导致逻辑重复

控制：只重复 UI 编排，不重复查询逻辑；查询仍由 `usePackageSearch` 统一。

### 风险 4：视觉原型文件被误提交

控制：实现阶段应确认 `.superpowers/` 已加入 `.gitignore`，或者避免将 `.superpowers/` 纳入 git add。

## 验收标准

1. `/` 第一屏显示公共端 Hero，而不是直接显示管理风搜索工具栏。
2. `/` 默认显示包 Tab，可切换到仓库 Tab。
3. 高级筛选点击后在搜索区域下方行内展开，不再打开右侧抽屉。
4. 包搜索、类型筛选、排序、分页、URL 同步保持可用。
5. 管理端 `/admin/packages` 仍可搜索、筛选、分页、查看详情、查看版本、删除、批量操作。
6. `npx vue-tsc --noEmit` 通过。
7. `npx vitest run` 通过。
8. E2E 测试中的公共端选择器更新并可被 Playwright 解析。
