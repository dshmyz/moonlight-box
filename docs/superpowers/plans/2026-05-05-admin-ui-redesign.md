# 管理后台 UI 改进实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将管理后台从传统深色风格改造为现代简约的浅色风格，提升视觉美观度和用户体验

**架构：** 采用渐进式改造策略，从整体布局开始，逐步优化各个页面和组件。使用 CSS 变量系统统一设计规范，通过组件抽象实现样式复用。

**技术栈：** Vue 3 + TypeScript + Element Plus + CSS Variables

---

## 文件结构

### 将要修改的文件

**布局组件**:
- `web/src/components/layout/AppHeader.vue` - 顶部导航栏，改为白色背景
- `web/src/components/layout/AppSidebar.vue` - 侧边栏，改为浅灰色背景
- `web/src/views/Layout.vue` - 整体布局容器
- `web/src/App.vue` - 全局样式和 CSS 变量定义

**仪表盘页面**:
- `web/src/views/Dashboard.vue` - 仪表盘主页面
- `web/src/components/dashboard/StatCards.vue` - 统计卡片组件
- `web/src/components/dashboard/DownloadChart.vue` - 下载趋势图
- `web/src/components/dashboard/StorageCard.vue` - 存储使用卡片
- `web/src/components/dashboard/TopPackages.vue` - 热门包列表
- `web/src/components/dashboard/ActivityFeed.vue` - 最近活动列表

**功能管理页面**:
- `web/src/views/PackageList.vue` - 包管理列表页
- `web/src/views/RepositoryList.vue` - 仓库管理列表页
- `web/src/components/package/PackageTable.vue` - 包列表表格组件

**基础 UI 组件**:
- `web/src/components/ui/CustomButton.vue` - 按钮组件
- `web/src/components/ui/CustomInput.vue` - 输入框组件
- `web/src/components/ui/CustomSelect.vue` - 下拉选择组件

### 将要创建的文件

**新增组件**:
- `web/src/components/ui/StatCard.vue` - 统计卡片组件（可复用）
- `web/src/components/ui/TypeTag.vue` - 类型标签组件
- `web/src/components/ui/StatusTag.vue` - 状态标签组件
- `web/src/components/ui/GradientCard.vue` - 渐变卡片组件

**样式文件**:
- `web/src/styles/design-tokens.css` - 设计令牌（CSS 变量）

---

## 第一阶段：整体布局改造

### 任务 1：定义全局设计令牌

**文件：**
- 创建：`web/src/styles/design-tokens.css`
- 修改：`web/src/App.vue`

- [ ] **步骤 1：创建设计令牌文件**

创建 `web/src/styles/design-tokens.css`:

```css
:root {
  /* 主色 */
  --color-primary: #3b82f6;
  --color-primary-light: #60a5fa;
  --color-primary-dark: #2563eb;

  /* 功能色 */
  --color-success: #10b981;
  --color-success-light: #4ade80;
  --color-success-dark: #15803d;

  --color-warning: #f59e0b;
  --color-warning-light: #fbbf24;
  --color-warning-dark: #d97706;

  --color-danger: #ef4444;
  --color-danger-light: #f87171;
  --color-danger-dark: #dc2626;

  /* 中性色 */
  --color-bg-page: #f9fafb;
  --color-bg-card: #ffffff;
  --color-bg-sidebar: #fafafa;
  --color-bg-hover: #f3f4f6;
  --color-bg-active: #eff6ff;

  --color-border: #e5e7eb;
  --color-border-light: #f3f4f6;
  --color-border-dark: #d1d5db;

  --color-text-primary: #111827;
  --color-text-secondary: #6b7280;
  --color-text-tertiary: #9ca3af;
  --color-text-inverse: #ffffff;

  /* 圆角 */
  --radius-sm: 4px;
  --radius-md: 6px;
  --radius-lg: 8px;
  --radius-xl: 12px;
  --radius-full: 9999px;

  /* 间距 */
  --spacing-xs: 4px;
  --spacing-sm: 8px;
  --spacing-md: 12px;
  --spacing-lg: 16px;
  --spacing-xl: 20px;
  --spacing-2xl: 24px;

  /* 字体大小 */
  --font-size-xs: 12px;
  --font-size-sm: 13px;
  --font-size-base: 14px;
  --font-size-lg: 16px;
  --font-size-xl: 20px;
  --font-size-2xl: 24px;
  --font-size-3xl: 28px;

  /* 字重 */
  --font-weight-normal: 400;
  --font-weight-medium: 500;
  --font-weight-semibold: 600;
  --font-weight-bold: 700;

  /* 阴影 */
  --shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.05);
  --shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
  --shadow-lg: 0 10px 15px -3px rgba(0, 0, 0, 0.1);

  /* 过渡 */
  --transition-fast: 150ms ease;
  --transition-base: 200ms ease;
  --transition-slow: 300ms ease;
}
```

- [ ] **步骤 2：在 App.vue 中引入设计令牌**

修改 `web/src/App.vue` 的 `<style>` 部分:

```vue
<style>
@import './styles/design-tokens.css';

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, 'Fira Sans', 'Droid Sans', 'Helvetica Neue', sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  color: var(--color-text-primary);
  background-color: var(--color-bg-page);
}

#app {
  width: 100%;
  height: 100vh;
}
</style>
```

- [ ] **步骤 3：Commit 设计令牌**

```bash
git add web/src/styles/design-tokens.css web/src/App.vue
git commit -m "feat: 添加全局设计令牌系统

- 定义颜色、圆角、间距、字体等设计规范
- 使用 CSS 变量实现主题系统
- 为后续 UI 改造奠定基础"
```

---

### 任务 2：改造顶部导航栏

**文件：**
- 修改：`web/src/components/layout/AppHeader.vue`

- [ ] **步骤 1：更新顶部导航栏样式**

修改 `web/src/components/layout/AppHeader.vue` 的 `<style scoped>` 部分:

```vue
<style scoped>
.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg-card);
  padding: 0 var(--spacing-2xl);
  height: 56px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-lg);
}

.collapse-btn {
  cursor: pointer;
  font-size: 18px;
  color: var(--color-text-secondary);
  transition: color var(--transition-fast);
}

.collapse-btn:hover {
  color: var(--color-text-primary);
}

.header-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-lg);
}

.header-link {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  color: var(--color-text-secondary);
  text-decoration: none;
  font-size: var(--font-size-sm);
  transition: color var(--transition-fast);
}

.header-link:hover {
  color: var(--color-primary);
}

.user-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  cursor: pointer;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-lg);
  transition: background-color var(--transition-fast);
}

.user-info:hover {
  background-color: var(--color-bg-hover);
}

.user-avatar {
  background: linear-gradient(135deg, var(--color-primary) 0%, var(--color-primary-dark) 100%);
  color: var(--color-text-inverse);
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
}

.username {
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.arrow-icon {
  font-size: 12px;
  color: var(--color-text-tertiary);
}

:deep(.el-breadcrumb__inner a) {
  color: var(--color-text-secondary);
  text-decoration: none;
}

:deep(.el-breadcrumb__inner a:hover) {
  color: var(--color-primary);
}

:deep(.el-dropdown-menu__item) {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
</style>
```

- [ ] **步骤 2：Commit 顶部导航栏改造**

```bash
git add web/src/components/layout/AppHeader.vue
git commit -m "feat: 改造顶部导航栏为现代简约风格

- 改为白色背景，使用设计令牌
- 优化用户信息展示
- 统一间距和圆角规范"
```

---

### 任务 3：改造侧边栏

**文件：**
- 修改：`web/src/components/layout/AppSidebar.vue`

- [ ] **步骤 1：更新侧边栏样式**

修改 `web/src/components/layout/AppSidebar.vue` 的 `<style scoped>` 部分:

```vue
<style scoped>
.app-sidebar {
  background: var(--color-bg-sidebar);
  border-right: 1px solid var(--color-border);
  transition: width var(--transition-slow);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.sidebar-logo {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 0 var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.logo-icon {
  width: 32px;
  height: 32px;
  background: linear-gradient(135deg, var(--color-primary) 0%, var(--color-primary-dark) 100%);
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-text-inverse);
  font-size: 16px;
  font-weight: var(--font-weight-bold);
  flex-shrink: 0;
}

.logo-text {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-bold);
  color: var(--color-text-primary);
  white-space: nowrap;
  letter-spacing: -0.3px;
}

.sidebar-menu {
  border-right: none;
  background: transparent;
  flex: 1;
  padding-top: var(--spacing-sm);
  overflow-y: auto;
}

.sidebar-menu :deep(.el-menu-item) {
  color: var(--color-text-secondary);
  font-size: var(--font-size-base);
  border-radius: var(--radius-lg);
  margin: 0 var(--spacing-sm);
  padding-left: var(--spacing-xl) !important;
}

.sidebar-menu :deep(.el-menu-item:hover) {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.sidebar-menu :deep(.el-menu-item.is-active) {
  background: var(--color-bg-active);
  color: var(--color-primary);
  font-weight: var(--font-weight-medium);
}

.sidebar-menu :deep(.el-menu-item.is-active::before) {
  display: none;
}

.sidebar-menu :deep(.el-sub-menu__title) {
  color: var(--color-text-secondary);
  font-size: var(--font-size-base);
}

.sidebar-menu :deep(.el-sub-menu__title:hover) {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.sidebar-menu :deep(.el-sub-menu .el-menu) {
  background: transparent;
}

.sidebar-menu :deep(.el-sub-menu .el-menu .el-menu-item) {
  padding-left: 52px !important;
  min-width: auto;
}

.sidebar-menu :deep(.el-menu--collapse) {
  padding-top: var(--spacing-sm);
}
</style>
```

- [ ] **步骤 2：Commit 侧边栏改造**

```bash
git add web/src/components/layout/AppSidebar.vue
git commit -m "feat: 改造侧边栏为现代简约风格

- 改为浅灰色背景，更轻盈
- 使用设计令牌统一样式
- 优化选中状态和悬停效果"
```

---

### 任务 4：更新整体布局容器

**文件：**
- 修改：`web/src/views/Layout.vue`

- [ ] **步骤 1：更新布局容器样式**

修改 `web/src/views/Layout.vue` 的 `<style scoped>` 部分:

```vue
<style scoped>
.layout-container {
  height: 100vh;
  display: flex;
  flex-direction: row;
}

.main-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--color-bg-page);
}

.app-main {
  padding: var(--spacing-2xl);
  overflow-y: auto;
  flex: 1;
}
</style>
```

- [ ] **步骤 2：Commit 布局容器改造**

```bash
git add web/src/views/Layout.vue
git commit -m "feat: 更新整体布局容器样式

- 使用设计令牌
- 统一背景色和内边距"
```

---

## 第二阶段：仪表盘页面改造

### 任务 5：创建可复用的统计卡片组件

**文件：**
- 创建：`web/src/components/ui/StatCard.vue`

- [ ] **步骤 1：创建统计卡片组件**

创建 `web/src/components/ui/StatCard.vue`:

```vue
<template>
  <div :class="['stat-card', `stat-card--${variant}`]">
    <div class="stat-card__header">
      <span class="stat-card__label">{{ label }}</span>
      <span class="stat-card__icon">{{ icon }}</span>
    </div>
    <div class="stat-card__value">{{ formattedValue }}</div>
    <div v-if="trend" class="stat-card__trend">
      <span class="stat-card__trend-icon">{{ trend > 0 ? '↑' : '↓' }}</span>
      <span class="stat-card__trend-text">{{ Math.abs(trend) }}% 较{{ trendPeriod }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  label: string
  value: number | string
  icon?: string
  trend?: number
  trendPeriod?: string
  variant?: 'blue' | 'green' | 'orange' | 'pink'
}

const props = withDefaults(defineProps<Props>(), {
  icon: '',
  trend: 0,
  trendPeriod: '上周',
  variant: 'blue',
})

const formattedValue = computed(() => {
  if (typeof props.value === 'string') return props.value
  if (props.value >= 1000) {
    return (props.value / 1000).toFixed(1) + 'K'
  }
  return props.value.toLocaleString()
})
</script>

<style scoped>
.stat-card {
  padding: var(--spacing-xl);
  border-radius: var(--radius-xl);
  border: 1px solid;
  transition: transform var(--transition-fast), box-shadow var(--transition-fast);
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
}

.stat-card--blue {
  background: linear-gradient(135deg, #eff6ff 0%, #dbeafe 100%);
  border-color: #bfdbfe;
}

.stat-card--blue .stat-card__label {
  color: var(--color-primary);
}

.stat-card--blue .stat-card__value {
  color: #1e40af;
}

.stat-card--blue .stat-card__trend-text {
  color: #60a5fa;
}

.stat-card--green {
  background: linear-gradient(135deg, #f0fdf4 0%, #dcfce7 100%);
  border-color: #bbf7d0;
}

.stat-card--green .stat-card__label {
  color: var(--color-success);
}

.stat-card--green .stat-card__value {
  color: #15803d;
}

.stat-card--green .stat-card__trend-text {
  color: #4ade80;
}

.stat-card--orange {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
  border-color: #fcd34d;
}

.stat-card--orange .stat-card__label {
  color: var(--color-warning);
}

.stat-card--orange .stat-card__value {
  color: #92400e;
}

.stat-card--orange .stat-card__trend-text {
  color: #fbbf24;
}

.stat-card--pink {
  background: linear-gradient(135deg, #fce7f3 0%, #fbcfe8 100%);
  border-color: #f9a8d4;
}

.stat-card--pink .stat-card__label {
  color: #db2777;
}

.stat-card--pink .stat-card__value {
  color: #9f1239;
}

.stat-card--pink .stat-card__trend-text {
  color: #f472b6;
}

.stat-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--spacing-sm);
}

.stat-card__label {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
}

.stat-card__icon {
  font-size: 18px;
}

.stat-card__value {
  font-size: var(--font-size-3xl);
  font-weight: var(--font-weight-bold);
  margin-bottom: var(--spacing-xs);
}

.stat-card__trend {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-xs);
}

.stat-card__trend-icon {
  font-size: 14px;
}
</style>
```

- [ ] **步骤 2：Commit 统计卡片组件**

```bash
git add web/src/components/ui/StatCard.vue
git commit -m "feat: 创建可复用的统计卡片组件

- 支持 4 种颜色变体（蓝、绿、橙、粉）
- 支持趋势指示
- 使用设计令牌
- 添加悬停动画效果"
```

---

### 任务 6：创建类型标签组件

**文件：**
- 创建：`web/src/components/ui/TypeTag.vue`

- [ ] **步骤 1：创建类型标签组件**

创建 `web/src/components/ui/TypeTag.vue`:

```vue
<template>
  <span :class="['type-tag', `type-tag--${type}`]">
    {{ type.toUpperCase() }}
  </span>
</template>

<script setup lang="ts">
interface Props {
  type: 'npm' | 'pypi' | 'maven' | 'nuget' | 'go'
}

defineProps<Props>()
</script>

<style scoped>
.type-tag {
  padding: var(--spacing-xs) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-medium);
  display: inline-block;
}

.type-tag--npm {
  background: #dbeafe;
  color: #1e40ab;
}

.type-tag--pypi {
  background: #dcfce7;
  color: #15803d;
}

.type-tag--maven {
  background: #fef3c7;
  color: #92400e;
}

.type-tag--nuget {
  background: #fce7f3;
  color: #9f1239;
}

.type-tag--go {
  background: #e0e7ff;
  color: #3730a3;
}
</style>
```

- [ ] **步骤 2：Commit 类型标签组件**

```bash
git add web/src/components/ui/TypeTag.vue
git commit -m "feat: 创建类型标签组件

- 支持 NPM、PyPI、Maven、NuGet、Go 五种类型
- 每种类型使用不同颜色
- 使用设计令牌"
```

---

### 任务 7：创建状态标签组件

**文件：**
- 创建：`web/src/components/ui/StatusTag.vue`

- [ ] **步骤 1：创建状态标签组件**

创建 `web/src/components/ui/StatusTag.vue`:

```vue
<template>
  <span :class="['status-tag', `status-tag--${status}`]">
    {{ statusText }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  status: 'online' | 'offline' | 'maintenance'
}

const props = defineProps<Props>()

const statusText = computed(() => {
  const map = {
    online: '在线',
    offline: '离线',
    maintenance: '维护中',
  }
  return map[props.status]
})
</script>

<style scoped>
.status-tag {
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-medium);
  display: inline-block;
}

.status-tag--online {
  background: #dcfce7;
  color: #15803d;
}

.status-tag--offline {
  background: #fee2e2;
  color: #991b1b;
}

.status-tag--maintenance {
  background: #fef3c7;
  color: #92400e;
}
</style>
```

- [ ] **步骤 2：Commit 状态标签组件**

```bash
git add web/src/components/ui/StatusTag.vue
git commit -m "feat: 创建状态标签组件

- 支持在线、离线、维护中三种状态
- 每种状态使用不同颜色
- 使用设计令牌"
```

---

### 任务 8：改造仪表盘统计卡片

**文件：**
- 修改：`web/src/components/dashboard/StatCards.vue`

- [ ] **步骤 1：更新统计卡片组件使用新的 StatCard**

修改 `web/src/components/dashboard/StatCards.vue`:

```vue
<template>
  <div class="stat-cards">
    <StatCard
      label="总仓库数"
      :value="stats.repositories?.length || 0"
      icon="📊"
      :trend="2"
      trend-period="本月"
      variant="blue"
    />
    <StatCard
      label="总包数量"
      :value="totalPackages"
      icon="📦"
      :trend="156"
      trend-period="本周"
      variant="green"
    />
    <StatCard
      label="下载量"
      :value="totalDownloads"
      icon="⬇️"
      :trend="12"
      trend-period="上周"
      variant="orange"
    />
    <StatCard
      label="缓存命中率"
      :value="cacheHitRate"
      icon="⚡"
      :trend="3.2"
      trend-period="昨日"
      variant="pink"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import StatCard from '@/components/ui/StatCard.vue'

interface Repository {
  name: string
  package_count?: number
}

interface Stats {
  repositories?: Repository[]
  storage?: {
    total_bytes: number
    used_bytes: number
    usage_percent: number
  }
  cache?: {
    hit_rate: number
    total_entries: number
  }
  downloads_last_7_days?: Array<{ date: string; count: number }>
  top_packages?: Array<{ name: string; downloads: number }>
}

interface Props {
  stats: Stats
}

const props = defineProps<Props>()

const totalPackages = computed(() => {
  return props.stats.repositories?.reduce((sum, repo) => sum + (repo.package_count || 0), 0) || 0
})

const totalDownloads = computed(() => {
  const total = props.stats.downloads_last_7_days?.reduce((sum, day) => sum + day.count, 0) || 0
  if (total >= 1000) {
    return (total / 1000).toFixed(1) + 'K'
  }
  return total.toString()
})

const cacheHitRate = computed(() => {
  return props.stats.cache?.hit_rate?.toFixed(1) + '%' || '0%'
})
</script>

<style scoped>
.stat-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-lg);
  margin-bottom: var(--spacing-2xl);
}

@media (max-width: 1200px) {
  .stat-cards {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 768px) {
  .stat-cards {
    grid-template-columns: 1fr;
  }
}
</style>
```

- [ ] **步骤 2：Commit 统计卡片改造**

```bash
git add web/src/components/dashboard/StatCards.vue
git commit -m "feat: 改造仪表盘统计卡片

- 使用新的 StatCard 组件
- 实现渐变色背景
- 添加趋势指示
- 支持响应式布局"
```

---

### 任务 9：改造仪表盘主页面

**文件：**
- 修改：`web/src/views/Dashboard.vue`

- [ ] **步骤 1：更新仪表盘页面样式**

修改 `web/src/views/Dashboard.vue` 的 `<style scoped>` 部分:

```vue
<style scoped>
.dashboard {
  min-height: 100%;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-semibold);
  margin: 0;
  color: var(--color-text-primary);
}

.section-title {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-lg);
}

.repo-grid {
  margin-bottom: var(--spacing-sm);
}
</style>
```

- [ ] **步骤 2：Commit 仪表盘页面改造**

```bash
git add web/src/views/Dashboard.vue
git commit -m "feat: 改造仪表盘主页面样式

- 使用设计令牌
- 统一字体大小和间距"
```

---

## 第三阶段：功能管理页面改造

### 任务 10：改造包管理页面

**文件：**
- 修改：`web/src/views/PackageList.vue`

- [ ] **步骤 1：更新包管理页面样式**

修改 `web/src/views/PackageList.vue` 的 `<style scoped>` 部分（如果文件存在）或添加样式:

```vue
<style scoped>
.package-list {
  min-height: 100%;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-semibold);
  margin: 0;
  color: var(--color-text-primary);
}

.search-bar {
  background: var(--color-bg-card);
  padding: var(--spacing-lg);
  border-radius: var(--radius-xl);
  border: 1px solid var(--color-border);
  margin-bottom: var(--spacing-lg);
}

.search-input {
  flex: 1;
}

.search-input :deep(.el-input__wrapper) {
  border-radius: var(--radius-lg);
}

.table-container {
  background: var(--color-bg-card);
  border-radius: var(--radius-xl);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

.pagination {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: var(--spacing-lg);
  padding: 0 var(--spacing-xs);
  font-size: var(--font-size-base);
  color: var(--color-text-secondary);
}
</style>
```

- [ ] **步骤 2：Commit 包管理页面改造**

```bash
git add web/src/views/PackageList.vue
git commit -m "feat: 改造包管理页面样式

- 使用设计令牌
- 优化搜索栏和表格样式
- 统一间距和圆角"
```

---

### 任务 11：改造仓库管理页面

**文件：**
- 修改：`web/src/views/RepositoryList.vue`

- [ ] **步骤 1：更新仓库管理页面为卡片布局**

修改 `web/src/views/RepositoryList.vue`，添加卡片布局样式:

```vue
<style scoped>
.repository-list {
  min-height: 100%;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-2xl);
}

.page-header h2 {
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-semibold);
  margin: 0;
  color: var(--color-text-primary);
}

.search-bar {
  background: var(--color-bg-card);
  padding: var(--spacing-lg);
  border-radius: var(--radius-xl);
  border: 1px solid var(--color-border);
  margin-bottom: var(--spacing-lg);
}

.repo-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: var(--spacing-lg);
}

.repo-card {
  background: var(--color-bg-card);
  padding: var(--spacing-xl);
  border-radius: var(--radius-xl);
  border: 1px solid var(--color-border);
  transition: transform var(--transition-fast), box-shadow var(--transition-fast);
}

.repo-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
}

.repo-card__header {
  display: flex;
  justify-content: space-between;
  align-items: start;
  margin-bottom: var(--spacing-md);
}

.repo-card__info {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.repo-card__icon {
  width: 48px;
  height: 48px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
}

.repo-card__icon--npm {
  background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%);
}

.repo-card__icon--pypi {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
}

.repo-card__icon--maven {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%);
}

.repo-card__name {
  font-weight: var(--font-weight-semibold);
  font-size: var(--font-size-lg);
  color: var(--color-text-primary);
}

.repo-card__desc {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-top: var(--spacing-xs);
}

.repo-card__stats {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.repo-card__stat-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
  margin-bottom: var(--spacing-xs);
}

.repo-card__stat-value {
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}

.repo-card__actions {
  display: flex;
  gap: var(--spacing-sm);
}

.repo-card__button {
  flex: 1;
  padding: var(--spacing-sm);
  border: 1px solid var(--color-border-dark);
  background: var(--color-bg-card);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.repo-card__button:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}
</style>
```

- [ ] **步骤 2：Commit 仓库管理页面改造**

```bash
git add web/src/views/RepositoryList.vue
git commit -m "feat: 改造仓库管理页面为卡片布局

- 实现响应式卡片网格布局
- 添加渐变色图标背景
- 优化状态展示
- 添加悬停动画效果"
```

---

## 第四阶段：基础组件优化

### 任务 12：优化按钮组件

**文件：**
- 修改：`web/src/components/ui/CustomButton.vue`

- [ ] **步骤 1：更新按钮组件样式**

修改 `web/src/components/ui/CustomButton.vue` 的样式部分:

```vue
<style scoped>
.custom-button {
  padding: var(--spacing-sm) var(--spacing-lg);
  border-radius: var(--radius-lg);
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-medium);
  cursor: pointer;
  transition: all var(--transition-fast);
  border: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
}

.custom-button--primary {
  background: var(--color-primary);
  color: var(--color-text-inverse);
}

.custom-button--primary:hover {
  background: var(--color-primary-dark);
  transform: translateY(-1px);
  box-shadow: var(--shadow-md);
}

.custom-button--secondary {
  background: var(--color-bg-card);
  color: var(--color-text-secondary);
  border: 1px solid var(--color-border-dark);
}

.custom-button--secondary:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.custom-button--danger {
  background: var(--color-danger);
  color: var(--color-text-inverse);
}

.custom-button--danger:hover {
  background: var(--color-danger-dark);
  transform: translateY(-1px);
  box-shadow: var(--shadow-md);
}

.custom-button--small {
  padding: var(--spacing-xs) var(--spacing-md);
  font-size: var(--font-size-sm);
  border-radius: var(--radius-md);
}

.custom-button--mini {
  padding: 6px var(--spacing-sm);
  font-size: var(--font-size-xs);
  border-radius: var(--radius-sm);
}
</style>
```

- [ ] **步骤 2：Commit 按钮组件优化**

```bash
git add web/src/components/ui/CustomButton.vue
git commit -m "feat: 优化按钮组件样式

- 使用设计令牌
- 添加悬停动画效果
- 统一尺寸规范"
```

---

### 任务 13：优化输入框组件

**文件：**
- 修改：`web/src/components/ui/CustomInput.vue`

- [ ] **步骤 1：更新输入框组件样式**

修改 `web/src/components/ui/CustomInput.vue` 的样式部分:

```vue
<style scoped>
.custom-input {
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border-dark);
  border-radius: var(--radius-lg);
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
  background: var(--color-bg-card);
  transition: all var(--transition-fast);
}

.custom-input:hover {
  border-color: var(--color-text-tertiary);
}

.custom-input:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.custom-input::placeholder {
  color: var(--color-text-tertiary);
}

.custom-input:disabled {
  background: var(--color-bg-page);
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}
</style>
```

- [ ] **步骤 2：Commit 输入框组件优化**

```bash
git add web/src/components/ui/CustomInput.vue
git commit -m "feat: 优化输入框组件样式

- 使用设计令牌
- 添加聚焦状态样式
- 优化禁用状态"
```

---

### 任务 14：优化下拉选择组件

**文件：**
- 修改：`web/src/components/ui/CustomSelect.vue`

- [ ] **步骤 1：更新下拉选择组件样式**

修改 `web/src/components/ui/CustomSelect.vue` 的样式部分:

```vue
<style scoped>
.custom-select {
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border-dark);
  border-radius: var(--radius-lg);
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
  background: var(--color-bg-card);
  cursor: pointer;
  transition: all var(--transition-fast);
  appearance: none;
  background-image: url("data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' fill='none' viewBox='0 0 20 20'%3e%3cpath stroke='%236b7280' stroke-linecap='round' stroke-linejoin='round' stroke-width='1.5' d='M6 8l4 4 4-4'/%3e%3c/svg%3e");
  background-position: right var(--spacing-sm) center;
  background-repeat: no-repeat;
  background-size: 1.5em 1.5em;
  padding-right: 2.5rem;
}

.custom-select:hover {
  border-color: var(--color-text-tertiary);
}

.custom-select:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.custom-select:disabled {
  background-color: var(--color-bg-page);
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}
</style>
```

- [ ] **步骤 2：Commit 下拉选择组件优化**

```bash
git add web/src/components/ui/CustomSelect.vue
git commit -m "feat: 优化下拉选择组件样式

- 使用设计令牌
- 添加自定义下拉箭头
- 优化聚焦和禁用状态"
```

---

## 第五阶段：测试和优化

### 任务 15：验证设计规范一致性

**文件：**
- 无需创建或修改文件

- [ ] **步骤 1：检查所有页面是否使用设计令牌**

运行以下命令检查是否还有硬编码的颜色值:

```bash
cd web/src && grep -r "#[0-9a-fA-F]\{3,6\}" --include="*.vue" --include="*.css" | grep -v "design-tokens.css" | grep -v "linear-gradient" | grep -v "data:image"
```

预期：应该没有输出或只有渐变色相关的输出

- [ ] **步骤 2：检查响应式布局**

在浏览器中测试以下断点:
- 1920px (桌面)
- 1440px (笔记本)
- 1024px (平板横屏)
- 768px (平板竖屏)
- 375px (手机)

预期：所有页面在这些断点下都应该正常显示

- [ ] **步骤 3：检查交互效果**

测试以下交互:
- 按钮悬停效果
- 卡片悬停效果
- 输入框聚焦效果
- 页面过渡动画

预期：所有交互都应该流畅自然

---

### 任务 16：性能优化

**文件：**
- 无需创建或修改文件

- [ ] **步骤 1：检查 CSS 文件大小**

运行以下命令:

```bash
cd web && npm run build && ls -lh dist/assets/*.css
```

预期：CSS 文件总大小应该小于 100KB

- [ ] **步骤 2：检查页面加载时间**

使用 Chrome DevTools 的 Lighthouse 工具测试首页加载时间。

预期：Performance 分数应该大于 90

- [ ] **步骤 3：优化不必要的重渲染**

检查 Vue DevTools，确保没有不必要的组件重渲染。

---

## 验收清单

### 视觉验收

- [ ] 整体风格符合现代简约设计
- [ ] 配色方案统一协调
- [ ] 圆角和间距规范一致
- [ ] 字体层级清晰
- [ ] 所有页面使用设计令牌

### 功能验收

- [ ] 所有页面布局正确
- [ ] 数据展示准确
- [ ] 交互功能正常
- [ ] 响应式适配良好

### 性能验收

- [ ] 页面加载速度 < 2s
- [ ] 交互响应流畅
- [ ] 无明显卡顿
- [ ] CSS 文件大小合理

### 兼容性验收

- [ ] Chrome 最新版测试通过
- [ ] Firefox 最新版测试通过
- [ ] Safari 最新版测试通过
- [ ] Edge 最新版测试通过

---

## 实施建议

1. **渐进式实施**: 按照阶段顺序实施，每个阶段完成后进行测试
2. **频繁提交**: 每个任务完成后立即提交，便于回滚
3. **测试驱动**: 在修改组件前先确保现有功能正常
4. **响应式优先**: 确保移动端体验良好
5. **性能监控**: 定期检查性能指标

---

## 风险应对

### 技术风险

**风险**: Element Plus 组件样式覆盖困难
**应对**: 使用 `:deep()` 选择器，必要时使用 `!important`

**风险**: 渐变色在某些浏览器显示不一致
**应对**: 使用标准的 CSS 渐变语法，测试主流浏览器

### 时间风险

**风险**: 实施时间可能超出预期
**应对**: 优先实现核心功能，次要功能可后续迭代

---

**计划编写**: AI Assistant
**基于文档**: docs/superpowers/specs/2026-05-05-admin-ui-redesign.md
**下一步**: 选择执行方式（子代理驱动或内联执行）
