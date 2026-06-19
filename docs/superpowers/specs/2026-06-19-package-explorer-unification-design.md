# 包管理查询界面统一设计

- **日期**: 2026-06-19
- **主题**: 将三套并行的包管理查询界面（PackageList V1、PackageCenterV2、BrowsePage）统一为单一 `PackageExplorer` 组件，补齐操作体验短板
- **状态**: 设计已批准，待编写实现计划

## 背景与动机

项目当前存在三套并行的包查询界面，体验差异显著且存在功能性缺陷：

| 界面 | 路由 | 体验完整度 | 关键问题 |
|------|------|-----------|---------|
| `PackageList.vue` (V1 管理端) | `/admin/packages` | 中 | 无骨架屏/URL同步/快捷键；卡片视图无空状态；删除后页码可能越界；搜索回车与 debounce 可能重复请求 |
| `PackageCenterV2.vue` (V2 管理端) | `/admin/packages-v2` | 低 | **搜索框缺失**（emits 了 search 事件但模板无输入框）；"最近使用"Tab 切换不过滤；排序用原生 `<select>` 风格不一致；侧边栏"最近更新"搜索后不更新 |
| `BrowsePage.vue` (公共浏览页) | `/` | 高 | 有骨架屏/URL同步/键盘快捷键；但"按下载量"排序**实际被硬编码成 `updated_at`**（[BrowsePage.vue:179](file:///Users/gracegaoya/work/project/moonlight-box/web/src/views/BrowsePage.vue#L179) bug） |

**共性短板**：
1. 三套界面各自实现搜索/筛选/排序/分页逻辑，未抽共享 composable
2. 类型选项三处硬编码，已有 `constants/package.ts` 的 `PACKAGE_TYPE_OPTIONS` 却未统一引用
3. 删除按钮未按 `package:delete` 权限控制，对所有 `package:read` 用户可见
4. 无批量操作、无列设置/密度切换、无最近搜索、无快速复制包名
5. V1/V2 路由并存，用户困惑哪个是主力

**目标**：重写一套统一的 `PackageExplorer` 组件，通过 `mode` prop 控制管理操作显隐，三套界面彻底归一；修复全部已知 bug；补齐查询增强能力（高级筛选、批量操作、列设置/密度切换、最近搜索/快速复制）。

## 架构设计

### 统一组件结构

新增目录 `web/src/components/package-explorer/`，包含：

```
web/src/components/package-explorer/
├── PackageExplorer.vue         容器，编排子组件 + mode 透传
├── PackageSearchBar.vue        搜索框 + 最近搜索下拉 + 类型 chips + 高级筛选触发按钮
├── PackageFilterPanel.vue      高级筛选抽屉（仓库/版本/来源/更新时间范围）
├── PackageTable.vue            表格视图（批量选择列 + 列设置 + 密度切换 + 操作列）
├── PackageGrid.vue             卡片网格视图
├── PackagePagination.vue       统一分页器（顶部+底部双位置可配）
├── PackageSkeleton.vue         骨架屏（表格/网格两种形态）
└── PackageEmptyState.vue       空状态（区分"无数据"与"无匹配结果"与"加载失败"）
```

**子组件拆分原则**：每个子组件单一职责，通过 props/emits 与容器通信，可独立测试。容器 `PackageExplorer.vue` 仅做编排，不堆砌逻辑。

### 共享逻辑

```
web/src/composables/
└── usePackageSearch.ts         搜索/筛选/排序/分页/URL同步/快捷键/最近搜索
```

### 路由收敛

| 路由 | 目标 | 备注 |
|------|------|------|
| `/admin/packages` | 新统一管理端（`mode="admin"`） | 保留原路径避免书签失效 |
| `/admin/packages-v2` | 重定向到 `/admin/packages` | 保留 30 天后下线重定向 |
| `/` | 新统一公共页（`mode="public"`） | 保留 `q/type/sort/page/page_size` URL 参数契约 |

**待删除文件**（迁移完成后）：`PackageList.vue`、`PackageCenterV2.vue`、`BrowsePage.vue`，以及 `components/package/`、`components/package-center/`、`components/browse/` 中未被新界面复用的组件。被复用的子组件（如 `VersionDrawer`、`UploadPackageDialog`）迁移到 `package-explorer/` 或保留原位并更新引用。

### mode 行为差异

| 能力 | admin | public |
|------|-------|--------|
| 查询/筛选/排序/分页 | ✅ | ✅ |
| 骨架屏/空状态/URL同步/快捷键 | ✅ | ✅ |
| 表格/网格视图切换 | ✅ | ✅ |
| 删除/上传 | ✅ | ❌ |
| 批量操作 | ✅ | ❌ |
| 列设置/密度切换 | ✅ | ❌（卡片网格无需） |
| 高级筛选（仓库/来源） | ✅ | ✅（只读筛选） |
| 统计概览/侧边栏 | ✅（吸纳 V2） | ❌ |

## usePackageSearch composable

### 职责

管理搜索词、筛选条件、排序、分页、URL 同步、键盘快捷键、最近搜索、加载状态、错误状态、数据获取。组件只负责渲染。

### API 设计

```typescript
// web/src/composables/usePackageSearch.ts
interface UsePackageSearchOptions {
  mode: 'admin' | 'public'
  initialQuery?: Partial<PackageQuery>   // 路由初始参数兜底
  syncUrl?: boolean                       // 是否同步 URL，默认 true
  pageSizeOptions?: number[]              // 默认 [20, 50, 100]
  defaultPageSize?: number                // 默认 20
  recentSearchKey?: string                // localStorage key，区分 admin/public
}

interface PackageQuery {
  q: string
  type: string                            // 'all' | 'npm' | 'maven' | ...
  repository?: string                     // 高级筛选
  version?: string                        // 高级筛选
  source?: 'all' | 'local' | 'proxy'      // 高级筛选
  updatedAtRange?: [string, string]       // 高级筛选，ISO 日期
  sort: 'updated_at' | 'downloads' | 'name'
  page: number
  pageSize: number
}

function usePackageSearch(options: UsePackageSearchOptions) {
  // 状态
  const query = reactive<PackageQuery>({...})
  const packages = ref<Package[]>([])
  const total = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const searchTime = ref(0)               // 耗时 ms，public 模式展示
  const recentSearches = ref<string[]>([]) // 最近 5 个

  // 派生
  const isEmpty = computed(() => !loading.value && packages.value.length === 0)
  const hasActiveFilter = computed(() => /* 高级筛选是否有值 */)

  // 动作
  async function search()                 // 主搜索，带防抖由调用方控制
  async function refresh()                // 保持当前条件重新加载
  function resetFilters()                 // 清空高级筛选
  function setQuery(patch: Partial<PackageQuery>)  // 统一入口，自动重置到第1页

  // URL 同步
  watch(query, () => syncToUrl(), { deep: true })
  onMounted(() => readFromUrl())

  // 键盘快捷键
  onMounted(() => registerShortcuts())    // / 聚焦搜索框、Esc 清空
  onUnmounted(() => unregisterShortcuts())

  // 最近搜索
  function addRecentSearch(term: string)
  function clearRecentSearches()

  return { query, packages, total, loading, error, searchTime,
           recentSearches, isEmpty, hasActiveFilter,
           search, refresh, resetFilters, setQuery,
           addRecentSearch, clearRecentSearches }
}
```

### 关键决策

1. **防抖策略**：搜索框 `@input` 触发 300ms debounce 后调 `setQuery({ q })`；回车立即触发并取消未执行的 debounce（避免 V1 当前的重复请求 bug）。debounce 在搜索栏组件内部用 lodash.debounce 实现，composable 只暴露同步的 `setQuery`。

2. **URL 同步契约**（统一三套界面）：
   - `q` → 搜索词
   - `type` → 包类型
   - `sort` → 排序字段
   - `page` / `page_size` → 分页
   - `repo` / `version` / `source` / `from` / `to` → 高级筛选
   - 用 `router.replace` 避免污染历史，但页码变化用 `router.push` 支持前进后退

3. **排序 bug 修复**：直接传 `query.sort` 给 API，不再做 `downloads → updated_at` 的错误映射。

4. **类型选项统一**：所有 chips/select 引用 `constants/package.ts` 的 `PACKAGE_TYPE_OPTIONS`，不再三处硬编码。

5. **错误处理**：`error` 状态暴露给组件，由 `PackageEmptyState` 区分"加载失败"与"无数据"两种空状态文案。

6. **删除后页码修正**：删除成功后调 `refresh()`，若当前页无数据且非第 1 页，自动 `page--` 重试，避免 V1 当前的越界问题。单删和批量删统一走此逻辑。

7. **快捷键作用域**：`/` 聚焦搜索框、`Esc` 清空搜索词；当焦点在 input/textarea 时不拦截 `/`（避免输入字符被吞）。

## 交互体验与状态反馈

### 搜索栏（PackageSearchBar）

**布局**（单行，响应式 wrap）：
```
[🔍 搜索框（带最近搜索下拉）] [类型 chips: 全部 npm Maven PyPI Go Yum Apt Generic ▾] [筛选] [排序▾] [视图切换 表|网格] [admin: 列设置⚙] [admin: 密度切换]
```

**搜索框行为**：
- placeholder：`搜索包名、描述或标签（按 / 聚焦）`
- `@input` 300ms debounce → `setQuery({ q })`
- `@keyup.enter` → 立即搜索 + 取消未执行 debounce + 记入最近搜索
- 聚焦时下拉显示最近 5 个搜索词（点击直接搜索；右侧"清空历史"按钮）
- 窄屏（<768px）搜索框占满整行，其他控件 wrap 到下一行
- 右侧内嵌清空按钮（`✕`），点击清空 + 立即搜索

**类型 chips**：
- 引用 `PACKAGE_TYPE_OPTIONS`，第一个"全部"
- 超过 6 个时折叠为"全部 + 前 5 个 + 更多 ▾"（解决 V1 当前的窄屏拥挤）
- 选中态用类型对应颜色（已有常量 `PACKAGE_TYPE_COLORS`）
- 点击切换类型 → `setQuery({ type, page: 1 })`

### 高级筛选面板（PackageFilterPanel）

**触发**：搜索栏右侧"筛选"按钮，带红点角标表示有激活的筛选项。

**形态**：右侧抽屉（`el-drawer` direction=rtl），宽度 380px。

**筛选项**：
- 仓库：`el-select` 多选，选项从 `repositoryApi.list()` 动态加载
- 版本：`el-input`，支持精确版本号或通配符（如 `1.2.*`），透传给 API `version` 参数
- 来源：`el-radio-group`（全部/本地/代理）
- 更新时间范围：`el-date-picker` type=daterange，快捷选项"今天/最近7天/最近30天/最近90天"

**底部操作**：
- "重置"：清空所有高级筛选，保留搜索词和类型
- "应用"：关闭抽屉 + 触发搜索

**URL 同步**：`repo` / `version` / `source` / `from` / `to` 参数。

### 批量操作（仅 admin，表格视图）

**多选**：
- 表格首列 `el-table-column type="selection"`
- 表头全选 + 单行勾选
- 选中态在翻页/切视图时清空（避免误删跨页数据）

**操作栏**：
- 选中 ≥1 项时，表格上方浮出操作栏：`已选 N 项 [批量删除] [批量重新扫描] [导出 CSV] [取消选择]`
- 批量删除：`ElMessageBox.confirm` 列出前 5 个包名 + "等 N 个包"，确认后循环调 `deletePackage`，进度条显示已完成/总数，失败项汇总提示
- 批量重新扫描：调安全扫描接口（需后端确认接口形态）
- 导出 CSV：导出当前选中项（或当前查询结果全部，二选一按钮）

**权限控制**：
- 批量删除按钮仅当用户有 `package:delete` 权限时显示（通过 `v-permission` 指令）
- `package:read` 用户只能看不能删

### 列设置与密度切换（仅 admin，表格视图）

**列设置**：
- 搜索栏右侧齿轮按钮 → `el-popover` 弹出列勾选清单
- 可控列：描述 / 来源 / 版本数 / 下载量 / 更新时间 / 操作（包名和类型固定不可隐藏）
- 配置存 localStorage key `package-explorer:columns:<mode>`

**密度切换**：
- 三档：紧凑（`size="small"`）/ 默认（`size="default"`）/ 宽松（`size="large"`）
- 存 localStorage key `package-explorer:density`

### 状态反馈

**加载**：
- 首次加载：`PackageSkeleton`（表格 8 行 / 网格 8 卡片，shimmer 动画）
- 翻页/筛选变化：表格 `v-loading` 遮罩（不替换内容，避免闪烁）
- 搜索框内嵌 loading 图标（请求进行中时旋转）

**空状态**（PackageEmptyState）：
- 三种文案：
  - 无数据（从未有包）：`暂无包` + admin 模式引导"上传第一个包"
  - 无匹配结果：`未找到匹配的包` + "尝试调整搜索词或清空筛选" + "清空所有筛选"按钮
  - 加载失败：`加载失败` + 错误摘要 + "重试"按钮

**搜索耗时**：结果区顶部显示 `找到 N 个包（耗时 X ms）`，public 模式也保留。

**快速复制**：
- 表格包名列：hover 显示复制图标，点击复制 `type:name` 完整标识到剪贴板，`ElMessage.success('已复制')`
- 网格卡片：包名右侧常驻复制图标
- 版本号：版本抽屉中每行版本号 hover 显示复制图标

### 响应式断点

| 断点 | 行为 |
|------|------|
| ≥1200px | 搜索栏单行；admin 显示侧边栏（统计概览 + Top 包） |
| 768-1199px | 搜索栏控件 wrap；admin 侧边栏折叠为顶部统计条 |
| <768px | 搜索框占满；chips 横向滚动；表格切卡片网格视图（强制）；快捷键提示隐藏；分页器简化 |

## 迁移策略

### 阶段 1：新建并存

- 在 `package-explorer/` 下新建全部组件和 composable，不删除旧文件
- 新增路由 `/admin/packages-new`（临时）指向新管理端，`/` 仍指向旧 BrowsePage
- 新增路由 `/browse-new`（临时）指向新公共页
- 验证新界面功能完备、bug 已修复（特别是排序映射、V2 搜索框缺失、V2 Tab 失效）

### 阶段 2：切换

- `/admin/packages` 切到新管理端，旧 `PackageList.vue` 改名 `PackageList.legacy.vue` 保留
- `/admin/packages-v2` 重定向到 `/admin/packages`（不再渲染旧 V2）
- `/` 切到新公共页，旧 `BrowsePage.vue` 改名 `BrowsePage.legacy.vue` 保留
- 临时路由 `/admin/packages-new` 和 `/browse-new` 保留 7 天用于回滚验证

### 阶段 3：清理（7 天后）

- 删除 `PackageList.legacy.vue`、`PackageCenterV2.vue`、`BrowsePage.legacy.vue`
- 删除临时路由
- 删除旧 `components/package/`、`components/package-center/`、`components/browse/` 中未被新界面复用的组件
- 被新界面复用的子组件（如 `VersionDrawer`、`UploadPackageDialog`）保留原位（`components/package/`），仅更新 import 路径指向新容器；不重复迁移以避免 git 历史断裂

**回滚预案**：阶段 2 切换后若发现严重问题，将 `/admin/packages` 和 `/` 路由指回 legacy 文件即可，无需回滚代码。

## 权限控制

### 路由 meta

- `/admin/packages`：`meta.requiresAuth = true, meta.permissions = ['package:read']`
- `/`：无 auth 要求

### 按钮级权限（admin 模式内）

- 删除按钮：`v-permission="'package:delete'"`
- 上传按钮：`v-permission="'package:write'"`
- 批量删除：`v-permission="'package:delete'"`
- 批量重新扫描：`v-permission="'security:scan'"`
- 批量导出 CSV：`v-permission="'package:read'"`（所有 admin 用户可导出）
- 列设置/密度切换：无权限要求（纯 UI 偏好）

### 缺失指令处理

若项目尚无 `v-permission` 指令，本次同步新增 `web/src/directives/permission.ts`，从 Pinia auth store 读取当前用户权限，无权限时 `el.parentNode.removeChild(el)`。注册到 `main.ts`。

## 测试策略

### composable 单元测试（Vitest）

`usePackageSearch.spec.ts`：
- URL 同步：query 变化 → URL 更新；路由初始参数 → query 初始化
- 防抖：连续 input 不触发多次请求
- 删除后页码修正：当前页删空 → 自动回退
- 排序字段透传：`sort: 'downloads'` → API 收到 `downloads`（验证 bug 修复）
- 最近搜索：去重、最多 5 个、localStorage 持久化
- 快捷键：`/` 聚焦、`Esc` 清空、input 焦点时不拦截 `/`

### 组件测试（Vitest + @vue/test-utils）

- `PackageSearchBar`：chips 折叠/展开、最近搜索下拉、清空按钮
- `PackageFilterPanel`：抽屉开关、重置、应用、URL 同步
- `PackageTable`：批量选择、列设置持久化、密度切换、复制按钮
- `PackageEmptyState`：三种文案正确渲染
- `PackageExplorer`：`mode="admin"` 显示管理操作、`mode="public"` 隐藏

### E2E 测试（Playwright，补充到 `web/scripts/e2e/`）

- 管理端：搜索 → 筛选 → 排序 → 翻页 → 批量删除 → 验证页码修正
- 公共端：URL 参数直接访问 → 验证状态恢复 → `/` 快捷键聚焦 → Esc 清空

### 回归验证清单

- [ ] 排序"按下载量"实际按下载量（修复 bug）
- [ ] V2 搜索框缺失问题不再存在（V2 已下线）
- [ ] V2 "最近使用"Tab 失效不再存在
- [ ] 删除后页码不越界
- [ ] 卡片视图有空状态
- [ ] 三套界面 URL 参数契约一致
- [ ] 类型选项单一来源（`PACKAGE_TYPE_OPTIONS`）

## 后端接口确认

本次以前端为主，但有两处需后端配合：

1. **批量删除接口**：当前 `deletePackage` 是单个。批量删除可前端循环调用，但建议后端新增 `POST /packages/batch-delete` 接收 `[{type, name}]` 数组，返回逐项结果。若后端暂不提供，前端循环调用作为降级方案，需处理部分失败。

2. **高级筛选项支持**：API `search` 已支持 `repository/version`，但需确认 `source`（local/proxy）和 `updatedAtRange`（from/to）参数后端是否已实现。若未实现，需后端补充或前端先隐藏这两项。

在 writing-plans 阶段把这两项作为前置依赖列出，前后端并行推进。

## YAGNI 检查（剔除过度设计）

- ❌ 不做搜索建议/自动补全下拉（需后端新增接口，超出本次范围）
- ❌ 不做包收藏/星标（无需求）
- ❌ 不做查询历史持久化到后端（localStorage 已够）
- ❌ 不做表格虚拟滚动（单页最多 100 行，无需）
- ❌ 不做暗色模式适配（本次范围外）
- ✅ 保留：最近搜索、列设置、密度、批量操作、高级筛选、URL 同步、快捷键、骨架屏、空状态、快速复制
