# NPM搜索API功能设计

## 概述

为NPM适配器添加搜索API功能，支持用户搜索本地仓库中的NPM包。

## 背景

当前NPM适配器已经实现了核心功能：
- 包上传/下载
- 元数据管理
- 代理支持
- Webhook支持

但缺少以下重要功能：
1. **搜索API**：用户无法搜索包，影响用户体验
2. **包发现**：用户需要知道确切的包名才能访问

## 目标

- 实现NPM标准搜索API（`/-/v1/search`）
- 支持包名和描述搜索
- 返回JSON格式的搜索结果
- 仅搜索本地仓库

## 非目标

- 搜索代理仓库
- 高级搜索功能（分页、排序、过滤）
- 搜索建议
- 搜索历史
- 全文搜索引擎集成

## 设计细节

### 架构设计

**核心思路**：在NPM适配器中添加搜索端点，查询数据库中的NPM包，使用SQL LIKE进行模糊匹配。

**处理流程**：
```
请求搜索
   ↓
解析查询参数
   ↓
验证参数
   ↓
查询数据库
   ↓
格式化结果
   ↓
返回JSON
```

### API端点

**端点**：
```
GET /npm/-/v1/search
```

**请求参数**：
- `text`：搜索关键词（必需）
- `size`：返回结果数量（可选，默认20，最大100）
- `from`：偏移量（可选，默认0）

**请求示例**：
```bash
# 基础搜索
GET /npm/-/v1/search?text=express

# 分页搜索
GET /npm/-/v1/search?text=react&size=10&from=0

# 描述搜索
GET /npm/-/v1/search?text=http%20client
```

### 响应格式

**成功响应**（200 OK）：
```json
{
  "objects": [
    {
      "package": {
        "name": "express",
        "version": "4.18.2",
        "description": "Fast, unopinionated, minimalist web framework",
        "date": "2026-05-03T12:00:00Z"
      },
      "score": {
        "detail": {
          "quality": 1.0,
          "popularity": 1.0,
          "maintenance": 1.0
        },
        "final": 1.0
      }
    }
  ],
  "total": 100,
  "time": "Mon May 03 2026 12:00:00 GMT+0000"
}
```

**错误响应**（400 Bad Request）：
```json
{
  "error": "invalid search parameters",
  "message": "text parameter is required"
}
```

### 数据结构

**1. 搜索请求结构**
```go
type NpmSearchRequest struct {
    Text string `form:"text" binding:"required"`
    Size int    `form:"size" binding:"omitempty,min=1,max=100"`
    From int    `form:"from" binding:"omitempty,min=0"`
}
```

**2. 搜索响应结构**
```go
type NpmSearchResponse struct {
    Objects []NpmSearchObject `json:"objects"`
    Total   int               `json:"total"`
    Time    string            `json:"time"`
}

type NpmSearchObject struct {
    Package NpmSearchPackage `json:"package"`
    Score   NpmSearchScore   `json:"score"`
}

type NpmSearchPackage struct {
    Name        string `json:"name"`
    Version     string `json:"version"`
    Description string `json:"description,omitempty"`
    Date        string `json:"date"`
}

type NpmSearchScore struct {
    Detail NpmSearchScoreDetail `json:"detail"`
    Final  float64              `json:"final"`
}

type NpmSearchScoreDetail struct {
    Quality      float64 `json:"quality"`
    Popularity   float64 `json:"popularity"`
    Maintenance  float64 `json:"maintenance"`
}
```

### 实现方法

#### 1. 搜索处理方法

```go
func (a *NpmAdapter) HandleSearch(c *gin.Context) {
    var req NpmSearchRequest
    if err := c.ShouldBindQuery(&req); err != nil {
        response.BadRequest(c, "invalid search parameters", err.Error())
        return
    }

    // 设置默认值
    if req.Size == 0 {
        req.Size = 20
    }

    // 查询数据库
    packages, total, err := a.searchPackages(c.Request.Context(), req.Text, req.Size, req.From)
    if err != nil {
        response.InternalError(c, "search failed")
        return
    }

    // 格式化响应
    resp := a.formatSearchResponse(packages, total)
    c.JSON(200, resp)
}
```

#### 2. 数据库查询方法

```go
func (a *NpmAdapter) searchPackages(ctx context.Context, query string, size, from int) ([]model.Package, int, error) {
    var packages []model.Package
    var total int64

    // 构建查询
    searchTerm := "%" + query + "%"
    db := a.pkgRepo.DB().Model(&model.Package{}).
        Where("type = ?", model.PackageTypeNPM).
        Where("name LIKE ? OR description LIKE ?", searchTerm, searchTerm)

    // 获取总数
    db.Count(&total)

    // 获取结果
    err := db.Preload("Versions").
        Order("updated_at DESC").
        Offset(from).
        Limit(size).
        Find(&packages).Error

    return packages, int(total), err
}
```

#### 3. 格式化响应方法

```go
func (a *NpmAdapter) formatSearchResponse(packages []model.Package, total int) *NpmSearchResponse {
    objects := make([]NpmSearchObject, 0, len(packages))

    for _, pkg := range packages {
        // 获取最新版本
        var latestVersion string
        var updatedAt string
        if len(pkg.Versions) > 0 {
            latestVersion = pkg.Versions[0].Version
            updatedAt = pkg.Versions[0].UpdatedAt.Format(time.RFC3339)
        }

        objects = append(objects, NpmSearchObject{
            Package: NpmSearchPackage{
                Name:        pkg.Name,
                Version:     latestVersion,
                Description: pkg.Description,
                Date:        updatedAt,
            },
            Score: NpmSearchScore{
                Detail: NpmSearchScoreDetail{
                    Quality:     1.0,
                    Popularity:  1.0,
                    Maintenance: 1.0,
                },
                Final: 1.0,
            },
        })
    }

    return &NpmSearchResponse{
        Objects: objects,
        Total:   total,
        Time:    time.Now().Format("Mon Jan 02 2006 15:04:05 GMT-0700"),
    }
}
```

#### 4. 注册路由

```go
func (a *NpmAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
    npm := r.Group("/npm")
    
    // 搜索端点（公开访问）
    npm.GET("/-/v1/search", a.HandleSearch)
    
    // 其他路由...
}
```

### 测试场景

#### 1. 基础搜索测试
```bash
# 搜索包名
GET /npm/-/v1/search?text=express
预期：返回包含"express"的包列表

# 搜索描述
GET /npm/-/v1/search?text=http%20client
预期：返回描述包含"http client"的包列表
```

#### 2. 分页测试
```bash
# 第一页
GET /npm/-/v1/search?text=react&size=10&from=0
预期：返回前10个结果

# 第二页
GET /npm/-/v1/search?text=react&size=10&from=10
预期：返回第11-20个结果
```

#### 3. 边界测试
```bash
# 空搜索
GET /npm/-/v1/search?text=
预期：返回400错误

# 超大size
GET /npm/-/v1/search?text=test&size=1000
预期：自动限制为100

# 负数from
GET /npm/-/v1/search?text=test&from=-1
预期：返回400错误
```

#### 4. 性能测试
```bash
# 大量结果
GET /npm/-/v1/search?text=package
预期：响应时间<500ms

# 复杂查询
GET /npm/-/v1/search?text=very%20long%20search%20query%20with%20multiple%20words
预期：响应时间<500ms
```

## 性能考虑

### 数据库查询优化
- 使用索引：确保 `name` 和 `description` 字段有索引
- LIKE查询：使用 `LIKE '%query%'` 进行模糊匹配
- 分页：使用 `OFFSET` 和 `LIMIT` 进行分页

### 响应时间
- 目标：<500ms
- 优化：预加载版本信息，减少数据库查询次数

### 并发处理
- 支持并发请求
- 无状态设计，易于扩展

## 安全考虑

### 输入验证
- 验证 `text` 参数不为空
- 限制 `size` 最大为100
- 验证 `from` 为非负整数

### SQL注入防护
- 使用参数化查询
- 不直接拼接SQL语句

### 权限控制
- 搜索端点公开访问
- 只返回公开的包信息

## 向后兼容性

- 新增功能，不影响现有功能
- 符合NPM标准搜索API规范
- 现有API保持不变

## 测试计划

### 单元测试
- 搜索请求验证测试
- 数据库查询测试
- 响应格式化测试

### 集成测试
- 基础搜索功能测试
- 分页功能测试
- 边界条件测试

### 性能测试
- 大量结果搜索测试
- 并发请求测试
- 响应时间测试

## 实现计划

1. **添加数据结构**
   - 定义搜索请求和响应结构

2. **实现搜索方法**
   - 添加 HandleSearch 方法
   - 添加 searchPackages 方法
   - 添加 formatSearchResponse 方法

3. **注册路由**
   - 在 RegisterRoutes 中添加搜索端点

4. **测试验证**
   - 基础搜索测试
   - 分页测试
   - 边界测试

## 未来扩展

- 支持搜索代理仓库
- 添加分页和排序功能
- 实现搜索建议
- 集成全文搜索引擎
- 添加搜索历史
- 支持高级查询语法

## 参考资料

- [npm search API documentation](https://github.com/npm/registry/blob/master/docs/REGISTRY-API.md)
- [npm search endpoint specification](https://github.com/npm/registry/blob/master/docs/responses/package-search.md)
