# 软件包中心重新设计实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 重新设计软件包中心（BrowsePage），采用极简网格流布局，实现简洁清爽的视觉风格

**架构：** 单页面组件，通过 CSS Grid 实现响应式布局（≥1200px 3列，≥768px 2列，<768px 紧凑列表）。包卡片组件复用 browse/PackageCard，包管理页面使用独立的 package/PackageCards。

**技术栈：** Vue 3 + Element Plus + CSS Grid + CSS Media Queries

---

## 文件清单

| 文件 | 职责 |
|------|------|
| `web/src/views/BrowsePage.vue` | 主页面：Hero、搜索、筛选、列表容器 |
| `web/src/components/browse/PackageCard.vue` | 包卡片组件（网格模式） |

---

## 实现任务

### 任务 1：简化 BrowsePage Hero 区域

**文件：** `web/src/views/BrowsePage.vue:159-177`

- [ ] **步骤 1：移除 Hero 区域的背景色和边框**

```css
.hero-section {
  background: transparent;
  border: none;
  border-radius: 8px;
  padding: 0 0 20px;
  margin-bottom: 16px;
}
```

- [ ] **步骤 2：简化标题和描述样式**

```css
.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #1f2937;
  margin: 0 0 4px;
  letter-spacing: -0.3px;
}

.page-desc {
  color: #6b7280;
  font-size: 14px;
  margin: 0;
}
```

- [ ] **步骤 3：Commit**

```bash
git add web/src/views/BrowsePage.vue
git commit -m "refactor: simplify hero section to minimal style"
```

---

### 任务 2：简化搜索栏样式

**文件：** `web/src/views/BrowsePage.vue:186-265`

- [ ] **步骤 1：简化搜索框样式**

```css
.search-bar-wrapper {
  margin-bottom: 16px;
}

.search-input {
  width: 100%;
}

.search-input :deep(.el-input__wrapper) {
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  box-shadow: none;
  padding: 0 14px;
  height: 40px;
  background: #fafafa;
  transition: all 0.2s;
}

.search-input :deep(.el-input__wrapper:hover),
.search-input :deep(.el-input__wrapper.is-focus) {
  border-color: #2563eb;
  background: #fff;
}

.search-input :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}

.search-input :deep(.el-input-group__append) {
  background: #2563eb;
  border: none;
  border-radius: 0 8px 8px 0;
  padding: 0 16px;
}

.search-input :deep(.el-input-group__append:hover) {
  background: #1d4ed8;
}
```

- [ ] **步骤 2：Commit**

```bash
git add web/src/views/BrowsePage.vue
git commit -m "refactor: simplify search bar styling"
```

---

### 任务 3：简化筛选栏样式

**文件：** `web/src/views/BrowsePage.vue:300-350`

- [ ] **步骤 1：简化筛选栏，移除背景和边框**

```css
.filter-card {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 0;
  background: transparent;
  border: none;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.filter-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.filter-divider {
  display: none;
}

.filter-label {
  font-size: 13px;
  color: #6b7280;
  white-space: nowrap;
}

.stats-count {
  font-size: 13px;
  color: #6b7280;
  margin-left: auto;
}
```

- [ ] **步骤 2：统一类型筛选按钮样式**

```css
.browse-tabs :deep(.el-radio-button__inner) {
  border-radius: 6px;
  border: 1px solid #e5e7eb;
  background: #fff;
  color: #6b7280;
  font-size: 13px;
  padding: 6px 12px;
  box-shadow: none;
}

.browse-tabs :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  background: #2563eb;
  border-color: #2563eb;
  color: #fff;
}
```

- [ ] **步骤 3：简化排序选择器样式**

```css
.filter-group :deep(.el-select) {
  min-width: 110px;
}

.filter-group :deep(.el-input__wrapper) {
  border-radius: 6px;
  border: 1px solid #e5e7eb;
  background: #fff;
  box-shadow: none;
  padding: 0 10px;
  height: 32px;
}
```

- [ ] **步骤 4：Commit**

```bash
git add web/src/views/BrowsePage.vue
git commit -m "refactor: simplify filter bar styling"
```

---

### 任务 4：实现响应式网格列表

**文件：** `web/src/views/BrowsePage.vue:340-360`

- [ ] **步骤 1：实现网格布局**

```css
.package-list {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

@media (max-width: 1199px) {
  .package-list {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 767px) {
  .package-list {
    display: flex;
    flex-direction: column;
    gap: 0;
  }
}
```

- [ ] **步骤 2：Commit**

```bash
git add web/src/views/BrowsePage.vue
git commit -m "feat: implement responsive grid layout for package list"
```

---

### 任务 5：重新设计 PackageCard 极简风格

**文件：** `web/src/components/browse/PackageCard.vue`

- [ ] **步骤 1：重写极简卡片样式**

```css
.package-card {
  background: #fff;
  border-radius: 8px;
  padding: 20px;
  cursor: pointer;
  transition: all 0.2s;
}

.package-card:hover {
  background: #fafbfc;
}

.card-top {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.card-top :deep(.el-tag) {
  border-radius: 4px;
  font-size: 11px;
  padding: 0 6px;
  height: 20px;
  line-height: 18px;
}

.package-name {
  font-size: 15px;
  font-weight: 500;
  color: #1f2937;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.package-desc {
  color: #6b7280;
  font-size: 13px;
  margin: 0 0 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-bottom {
  display: flex;
  gap: 16px;
  align-items: center;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #9ca3af;
}

.meta-item .el-icon {
  font-size: 14px;
}
```

- [ ] **步骤 2：添加小屏列表样式（父容器传递）**

在小屏下（<768px），卡片样式需要变为紧凑列表：

```css
@media (max-width: 767px) {
  .package-card {
    border-radius: 0;
    padding: 12px 0;
    border-bottom: 1px solid #f3f4f6;
  }

  .package-card:hover {
    background: #fafbfc;
  }

  .package-card:last-child {
    border-bottom: none;
  }
}
```

- [ ] **步骤 3：Commit**

```bash
git add web/src/components/browse/PackageCard.vue
git commit -m "refactor: redesign PackageCard to minimalist style"
```

---

### 任务 6：添加紧凑列表模式（小屏适配）

**文件：** `web/src/views/BrowsePage.vue`

- [ ] **步骤 1：在响应式布局下切换卡片样式**

由于 CSS 无法直接修改子组件样式，需要在 BrowsePage 中为小屏添加额外样式覆盖：

```css
@media (max-width: 767px) {
  .package-list .package-card {
    border-radius: 0;
    padding: 12px 0;
    border-bottom: 1px solid #f3f4f6;
    background: transparent;
  }

  .package-list .package-card:hover {
    background: #fafbfc;
  }

  .package-list .package-card:last-child {
    border-bottom: none;
  }
}
```

- [ ] **步骤 2：Commit**

```bash
git add web/src/views/BrowsePage.vue
git commit -m "feat: add compact list style for mobile view"
```

---

### 任务 7：验证构建

- [ ] **步骤 1：运行构建验证**

```bash
cd web && npm run build 2>&1 | grep -E "(error|Error|✓ built)"
```

预期输出：`✓ built in X.XXs`

- [ ] **步骤 2：Commit**

```bash
git add -A
git commit -m "chore: verify build passes"
```

---

## 规格覆盖度自检

| 规格需求 | 对应任务 |
|----------|----------|
| Hero 区域无边框无背景 | 任务 1 |
| 搜索框与内容区视觉融合 | 任务 2 |
| 筛选栏轻量化，无多余装饰 | 任务 3 |
| 大屏显示 2-3 列网格卡片 | 任务 4 |
| 小屏自动切换为紧凑列表 | 任务 4, 6 |
| hover 状态仅背景变化，无阴影 | 任务 5 |
| 整体风格与首页/仪表盘一致 | 所有任务 |

---

## 执行交接

计划已完成并保存到 `docs/superpowers/plans/2026-05-05-browse-page-redesign.md`。两种执行方式：

**1. 子代理驱动（推荐）** - 每个任务调度一个新的子代理，任务间进行审查，快速迭代

**2. 内联执行** - 在当前会话中使用 executing-plans 执行任务，批量执行并设有检查点

选哪种方式？
