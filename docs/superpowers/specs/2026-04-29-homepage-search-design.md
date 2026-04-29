# Moonlight Registry 首页与搜索功能设计

> **目标：** 实现 P0 功能 - 全局搜索 + 仓库状态卡片，提升用户体验和系统可观测性
> **架构：** 独立搜索 API + 聚合 Dashboard API，前端组件化拆分
> **技术栈：** Go 1.21+, Gin, GORM, Vue 3, Element Plus

---

## 一、后端 API 设计

### 1.1 包搜索 API

```
GET /api/v1/packages/search
```

**请求参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `q` | string | 是 | 搜索关键词 |
| `type` | string | 否 | 包类型过滤（npm/maven2/pypi） |
| `scope` | string | 否 | 搜索范围（name/description/all），默认 name |
| `sort` | string | 否 | 排序方式（downloads/name/updated_at），默认 downloads |
| `page` | int | 否 | 页码，默认 1 |
| `page_size` | int | 否 | 每页数量，默认 20 |

**返回格式：**

```json
{
  "code": 0,
  "data": {
    "list": [
      {
        "id": 1,
        "name": "lodash",
        "type": "npm",
        "description": "A modern JavaScript utility library",
        "latest_version": "4.17.21",
        "download_count": 50000000,
        "updated_at": "2024-01-15T10:30:00Z"
      }
    ],
    "total": 123,
    "page": 1,
    "page_size": 20,
    "search_time_ms": 12
  }
}
```

### 1.2 Dashboard 统计 API

```
GET /api/v1/dashboard/stats
```

**返回格式：**

```json
{
  "code": 0,
  "data": {
    "repositories": [
      {
        "name": "npm-local",
        "type": "local",
        "package_type": "npm",
        "status": "healthy",
        "package_count": 3456,
        "download_count_today": 234,
        "storage_bytes": 5584384000
      }
    ],
    "storage": {
      "total_bytes": 2469606195200,
      "used_bytes": 1931392139264,
      "usage_percent": 78.2
    },
    "cache": {
      "hit_rate": 94.2,
      "total_entries": 8901
    },
    "downloads_last_7_days": [1200, 1450, 1100, 1800, 1650, 2100, 1900]
  }
}
```

---

## 二、后端文件结构

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `internal/handler/package_search_handler.go` | 包搜索 API 处理器 |
| 新建 | `internal/service/package_search_service.go` | 包搜索业务逻辑 |
| 新建 | `internal/handler/dashboard_handler.go` | Dashboard API 处理器 |
| 新建 | `internal/service/dashboard_service.go` | Dashboard 聚合统计服务 |
| 修改 | `cmd/registry/main.go` | 注册搜索和 Dashboard 路由 |
| 修改 | `internal/model/package.go` | Package 增加 `download_count` 字段 |

---

## 三、前端文件结构

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `web/src/api/package.ts` | 包搜索 API 接口 |
| 新建 | `web/src/api/dashboard.ts` | Dashboard API 接口 |
| 新建 | `web/src/components/dashboard/RepoStatusCard.vue` | 仓库状态卡片 |
| 新建 | `web/src/components/dashboard/StorageCard.vue` | 存储容量卡片 |
| 新建 | `web/src/components/dashboard/DownloadChart.vue` | 下载趋势图 |
| 新建 | `web/src/components/dashboard/ActivityFeed.vue` | 最近活动 |
| 修改 | `web/src/views/Dashboard.vue` | 重写管理后台首页 |
| 修改 | `web/src/views/BrowsePage.vue` | 对接真实搜索 API |

---

## 四、搜索实现逻辑

### 4.1 后端搜索查询

```go
func (s *PackageSearchService) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
    query := s.db.Model(&model.Package{})
    
    // 根据 scope 构建搜索条件
    switch req.Scope {
    case "name":
        query = query.Where("name LIKE ?", "%"+req.Query+"%")
    case "description":
        query = query.Where("description LIKE ?", "%"+req.Query+"%")
    case "all":
        query = query.Where("name LIKE ? OR description LIKE ?", 
            "%"+req.Query+"%", "%"+req.Query+"%")
    default:
        query = query.Where("name LIKE ?", "%"+req.Query+"%")
    }
    
    // 包类型过滤
    if req.Type != "" {
        query = query.Where("type = ?", req.Type)
    }
    
    // 排序
    switch req.Sort {
    case "downloads":
        query = query.Order("download_count DESC")
    case "name":
        query = query.Order("name ASC")
    case "updated_at":
        query = query.Order("updated_at DESC")
    default:
        query = query.Order("download_count DESC")
    }
    
    // 分页
    var total int64
    query.Count(&total)
    
    var packages []model.Package
    offset := (req.Page - 1) * req.PageSize
    query.Offset(offset).Limit(req.PageSize).Find(&packages)
    
    return &SearchResult{
        List:   packages,
        Total:  total,
        Page:   req.Page,
        PageSize: req.PageSize,
    }, nil
}
```

---

## 五、前端组件设计

### 5.1 仓库状态卡片（RepoStatusCard.vue）

```
┌─────────────────────────────┐
│ npm-local          🟢 健康   │
│                             │
│ 3,456 包    ⬇ 234 今日下载  │
│ 5.2 GB 存储                  │
│                             │
│ [查看详情] [编辑]            │
└─────────────────────────────┘
```

**Props：**
- `repo` - 仓库数据对象
- `compact` - 是否紧凑模式

**状态颜色映射：**
- `healthy` → 绿色 (#67c23a)
- `syncing` → 橙色 (#e6a23c)
- `error` → 红色 (#f56c6c)
- `unknown` → 灰色 (#909399)

### 5.2 存储容量卡片（StorageCard.vue）

- 使用 `el-progress` 显示使用百分比
- 显示已用/总量（自动格式化为 GB/TB）
- 预计满载时间（按当前增长率计算）

### 5.3 下载趋势图（DownloadChart.vue）

- 使用简单的柱状图或折线图
- 显示最近 7 天的下载量
- 可选：使用 ECharts 如果项目已引入

### 5.4 最近活动（ActivityFeed.vue）

- 使用 `el-timeline` 显示
- 每条活动显示：时间、用户、操作类型、描述
- 最多显示 20 条

---

## 六、搜索 API 路由注册

在 `cmd/registry/main.go` 中：

```go
// 包管理（公开）
packages := r.Group("/api/v1/packages")
{
    packages.GET("/search", packageSearchHandler.Search)
}
```

---

## 七、实现顺序

1. **后端** - Package 模型增加 `download_count` 字段 + 数据库迁移
2. **后端** - 实现包搜索服务和 Handler
3. **后端** - 实现 Dashboard 统计服务和 Handler
4. **后端** - 注册路由
5. **前端** - 创建 API 接口文件
6. **前端** - 创建 Dashboard 子组件
7. **前端** - 重写 Dashboard.vue
8. **前端** - 修改 BrowsePage.vue 对接搜索 API
9. **测试** - API 测试 + 前端验证

---

## 规格自检

1. **占位符扫描** - 无 TODO 或待定内容
2. **内部一致性** - API 路径、字段名称一致
3. **范围检查** - 聚焦 P0 功能，可被一个实现计划覆盖
4. **模糊性检查** - 搜索 scope 和 sort 参数已明确定义
