# 包管理功能设计文档

**日期**: 2026-04-29
**版本**: 1.0
**状态**: 待审查

## 概述

为管理后台实现完整的包管理功能，包括包列表展示、搜索过滤、版本管理等核心功能。

## 目标

1. 提供包列表的双视图展示（表格/卡片）
2. 支持搜索、过滤、排序功能
3. 实现完整的版本管理（废弃、禁用、恢复、删除）
4. 复用现有组件，降低开发成本

## 架构设计

### 组件复用策略

采用**增强现有页面**的方案，复用公开页面的 `PackageDetail.vue` 组件：

- **公开页面**: `/packages/:type/:name` - 只读展示
- **管理后台**: `/admin/packages/:type/:name` - 显示版本管理功能

通过路由判断，在详情页中根据路由前缀决定是否显示管理功能。

### 页面结构

```
管理后台 (/admin)
├── 包管理
│   ├── 列表页 (PackageList.vue)
│   │   ├── 表格视图
│   │   ├── 卡片视图
│   │   ├── 搜索/过滤/排序
│   │   └── 快捷操作（查看版本抽屉）
│   └── 详情页 (PackageDetail.vue - 复用)
│       ├── 包基本信息
│       ├── 版本列表
│       └── 版本管理操作
```

## 功能设计

### 1. 包列表页 (PackageList.vue)

#### 1.1 视图切换

**表格视图**:
- 信息密度高，适合快速浏览
- 列：包名、类型、版本数、下载数、更新时间、操作
- 包名可点击，跳转到详情页

**卡片视图**:
- 视觉效果好，每个包信息突出
- 显示：包名、版本号、描述、类型标签、统计信息
- 底部操作按钮：查看版本、查看详情

#### 1.2 搜索和过滤

**搜索**:
- 支持按包名、描述搜索
- 实时搜索，输入即搜索

**过滤**:
- 按包类型过滤：npm、maven、pypi、go、nuget、yum、apt、generic
- 全部类型选项

**排序**:
- 按下载量（默认）
- 按名称
- 按更新时间

#### 1.3 分页

- 默认每页 20 条
- 支持切换每页条数：10、20、50、100
- 显示总数和当前页码

#### 1.4 快捷操作

**查看版本**:
- 点击按钮，右侧弹出抽屉
- 抽屉内显示版本列表（版本号、状态、下载量、发布时间）
- 底部"查看完整详情"按钮跳转到详情页

**详情**:
- 直接跳转到包详情页

### 2. 包详情页 (PackageDetail.vue - 增强)

#### 2.1 基本信息

- 包名、类型标签
- 最新版本号
- 总下载量
- 描述
- 统计信息：版本数、最后更新时间、发布者、主页链接

#### 2.2 版本列表

**表格展示**:
- 版本号
- 状态（已发布、已废弃、已禁用）
- 下载量
- 发布时间
- 操作按钮

**状态可视化**:
- 已发布：绿色标签，正常显示
- 已废弃：橙色标签，正常显示
- 已禁用：红色标签，版本号划线，背景浅红色

#### 2.3 版本管理操作

**废弃版本**:
- 将版本标记为已废弃
- 用户安装时会收到警告
- 仍可安装使用

**禁用版本**:
- 完全禁用版本
- 用户无法安装该版本
- 适用于严重安全漏洞场景

**恢复版本**:
- 将已废弃或已禁用的版本恢复为已发布状态

**删除版本**:
- 永久删除版本（需二次确认）
- 谨慎操作

**下载版本**:
- 下载特定版本的包文件

## 数据模型

### Package 模型（已存在）

```go
type Package struct {
    BaseModel
    Name           string
    Type           PackageType
    Description    string
    RepositoryID   uint
    RepositoryType RepositoryType
    Homepage       string
    License        string
    DownloadCount  int64
    CreatedBy      uint
    Versions       []PackageVersion
}
```

### PackageVersion 模型（已存在）

```go
type PackageVersion struct {
    ID             uint
    PackageID      uint
    Version        string
    Status         PackageStatus
    StoragePath    string
    SizeBytes      int64
    ChecksumSHA256 string
    ChecksumMD5    string
    PublishedAt    time.Time
    PublishedBy    uint
    Metadata       string
    DownloadCount  int
    Dependencies   []PackageDependency
}
```

### PackageStatus 枚举（已存在）

```go
const (
    StatusDraft      PackageStatus = "draft"
    StatusPublished  PackageStatus = "published"
    StatusDeprecated PackageStatus = "deprecated"
    StatusYanked     PackageStatus = "yanked"  // 对应"已禁用"
)
```

## API 接口

### 已有接口

1. **搜索包**
   - `GET /api/v1/packages/search`
   - 参数：q, type, scope, sort, page, page_size
   - 返回：包列表、总数、分页信息

2. **查看版本列表**
   - `GET /api/v1/packages/:type/:name/versions`
   - 返回：包信息和版本列表

3. **废弃版本**
   - `POST /api/v1/packages/versions/:id/deprecate`
   - 参数：reason（可选）

4. **恢复版本**
   - `POST /api/v1/packages/versions/:id/restore`

5. **禁用版本**
   - `POST /api/v1/packages/versions/:id/yank`
   - 参数：reason（可选）

6. **删除版本**
   - `DELETE /api/v1/packages/versions/:id`

### 需要新增的接口

无需新增接口，所有后端 API 已实现。

## 前端实现

### 文件结构

```
web/src/
├── views/
│   ├── PackageList.vue (增强)
│   └── PackageDetail.vue (增强)
├── components/
│   ├── package/
│   │   ├── PackageTable.vue (新建)
│   │   ├── PackageCards.vue (新建)
│   │   ├── VersionDrawer.vue (新建)
│   │   └── VersionTable.vue (新建)
│   └── package-detail/ (已存在)
├── api/
│   └── package.ts (已存在)
└── router/
    └── index.ts (修改)
```

### 组件说明

#### PackageTable.vue
- 表格视图组件
- 接收包列表数据
- 发出事件：view-versions、view-detail

#### PackageCards.vue
- 卡片视图组件
- 接收包列表数据
- 发出事件：view-versions、view-detail

#### VersionDrawer.vue
- 版本抽屉组件
- 显示版本列表
- 发出事件：view-detail

#### VersionTable.vue
- 版本表格组件
- 显示版本列表和管理操作
- 发出事件：deprecate、disable、restore、delete、download

### 路由配置

```typescript
{
  path: '/admin/packages',
  name: 'AdminPackages',
  component: () => import('@/views/PackageList.vue'),
  meta: { title: '包管理' },
},
{
  path: '/admin/packages/:type/:name',
  name: 'AdminPackageDetail',
  component: () => import('@/views/PackageDetail.vue'),
  meta: { title: '包详情', isAdmin: true },
}
```

## 实现计划

### 阶段 1: 基础列表展示
1. 增强 PackageList.vue，调用搜索 API
2. 实现表格视图
3. 实现搜索、过滤、排序
4. 实现分页

### 阶段 2: 视图切换和快捷操作
1. 实现卡片视图
2. 添加视图切换功能
3. 实现 VersionDrawer 组件
4. 添加快捷操作按钮

### 阶段 3: 版本管理
1. 增强 PackageDetail.vue，添加 isAdmin 判断
2. 实现 VersionTable 组件
3. 实现版本管理操作（废弃、禁用、恢复、删除）
4. 添加操作确认对话框

## 测试计划

### 单元测试
- 组件渲染测试
- 事件触发测试
- 数据格式化测试

### 集成测试
- API 调用测试
- 路由跳转测试
- 权限控制测试

### E2E 测试
- 完整用户流程测试
- 视图切换测试
- 版本管理操作测试

## 风险和缓解

### 风险 1: 详情页复用导致代码复杂
**缓解**: 通过计算属性判断是否为管理后台，条件渲染管理功能组件

### 风险 2: 版本管理操作误操作
**缓解**: 
- 删除操作需二次确认
- 禁用操作显示警告信息
- 提供恢复功能

### 风险 3: 大量包数据导致性能问题
**缓解**: 
- 后端分页
- 虚拟滚动（如果需要）
- 懒加载版本列表

## 成功标准

1. ✅ 包列表正确展示，支持搜索、过滤、排序
2. ✅ 表格和卡片视图切换正常
3. ✅ 版本抽屉正常显示
4. ✅ 详情页正确判断管理后台，显示版本管理功能
5. ✅ 版本管理操作（废弃、禁用、恢复、删除）正常工作
6. ✅ 所有操作有适当的确认和反馈

## 附录

### 参考文档
- [npm 版本管理最佳实践](https://docs.npmjs.com/cli/v9/commands/npm-deprecate)
- [PyPI 版本管理](https://packaging.python.org/en/latest/guides/modernize-setup-py-deprecated/)

### 设计原型
- 表格视图: `.superpowers/brainstorm/57232-1777473280/package-list-table.html`
- 卡片视图: `.superpowers/brainstorm/57232-1777473280/package-list-card.html`
- 详情页: `.superpowers/brainstorm/57232-1777473280/package-detail-v2.html`
- 版本抽屉: `.superpowers/brainstorm/57232-1777473280/package-version-drawer.html`
