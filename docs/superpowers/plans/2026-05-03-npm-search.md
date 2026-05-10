# NPM搜索API实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为NPM适配器添加搜索API功能，支持用户搜索本地仓库中的NPM包。

**架构：** 在NPM适配器中添加搜索端点，查询数据库中的NPM包，使用SQL LIKE进行模糊匹配，返回JSON格式的搜索结果。

**技术栈：** Go、Gin、GORM

---

## 文件结构

**修改文件：**
- `internal/adapter/npm_adapter.go` - NPM适配器主文件
  - 添加搜索数据结构
  - 添加搜索处理方法
  - 添加数据库查询方法
  - 添加响应格式化方法
  - 修改路由注册

---

## 任务 1：添加搜索数据结构

**文件：**
- 修改：`internal/adapter/npm_adapter.go`（在现有数据结构后添加）

- [ ] **步骤 1：添加搜索请求数据结构**

在文件中的数据结构部分（NpmRepository结构体后）添加：

```go
type NpmSearchRequest struct {
	Text string `form:"text" binding:"required"`
	Size int    `form:"size" binding:"omitempty,min=1,max=100"`
	From int    `form:"from" binding:"omitempty,min=0"`
}

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

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/npm_adapter.go
git commit -m "feat(npm): add search data structures"
```

---

## 任务 2：添加数据库查询方法

**文件：**
- 修改：`internal/adapter/npm_adapter.go`（在文件末尾添加）

- [ ] **步骤 1：添加searchPackages方法**

在文件末尾添加数据库查询方法：

```go
func (a *NpmAdapter) searchPackages(ctx context.Context, query string, size, from int) ([]model.Package, int, error) {
	var packages []model.Package
	var total int64

	searchTerm := "%" + query + "%"
	db := a.pkgRepo.DB().Model(&model.Package{}).
		Where("type = ?", model.PackageTypeNPM).
		Where("name LIKE ? OR description LIKE ?", searchTerm, searchTerm)

	db.Count(&total)

	err := db.Preload("Versions").
		Order("updated_at DESC").
		Offset(from).
		Limit(size).
		Find(&packages).Error

	return packages, int(total), err
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/npm_adapter.go
git commit -m "feat(npm): add searchPackages method for database queries"
```

---

## 任务 3：添加响应格式化方法

**文件：**
- 修改：`internal/adapter/npm_adapter.go`（在searchPackages方法后添加）

- [ ] **步骤 1：添加formatSearchResponse方法**

在searchPackages方法后添加响应格式化方法：

```go
func (a *NpmAdapter) formatSearchResponse(packages []model.Package, total int) *NpmSearchResponse {
	objects := make([]NpmSearchObject, 0, len(packages))

	for _, pkg := range packages {
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
					Quality:      1.0,
					Popularity:   1.0,
					Maintenance:  1.0,
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

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/npm_adapter.go
git commit -m "feat(npm): add formatSearchResponse method"
```

---

## 任务 4：添加搜索处理方法

**文件：**
- 修改：`internal/adapter/npm_adapter.go`（在formatSearchResponse方法后添加）

- [ ] **步骤 1：添加HandleSearch方法**

在formatSearchResponse方法后添加搜索处理方法：

```go
func (a *NpmAdapter) HandleSearch(c *gin.Context) {
	var req NpmSearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid search parameters", err.Error())
		return
	}

	if req.Size == 0 {
		req.Size = 20
	}

	packages, total, err := a.searchPackages(c.Request.Context(), req.Text, req.Size, req.From)
	if err != nil {
		response.InternalError(c, "search failed")
		return
	}

	resp := a.formatSearchResponse(packages, total)
	c.JSON(200, resp)
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/npm_adapter.go
git commit -m "feat(npm): add HandleSearch method for search endpoint"
```

---

## 任务 5：注册搜索路由

**文件：**
- 修改：`internal/adapter/npm_adapter.go`（RegisterRoutes方法）

- [ ] **步骤 1：在RegisterRoutes方法中添加搜索路由**

找到RegisterRoutes方法，添加搜索端点：

```go
func (a *NpmAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
	npm := r.Group("/npm")
	
	// 搜索端点（公开访问）
	npm.GET("/-/v1/search", a.HandleSearch)
	
	// 其他现有路由...
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/npm_adapter.go
git commit -m "feat(npm): register search endpoint in RegisterRoutes"
```

---

## 任务 6：测试搜索功能

**文件：**
- 无需修改文件，进行集成测试

- [ ] **步骤 1：启动后端服务**

运行：`go run cmd/registry/main.go`
预期：服务启动成功，监听9081端口

- [ ] **步骤 2：上传测试包**

运行：
```bash
# 先登录获取token
curl -X POST "http://localhost:9081/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# 创建测试包
mkdir -p test-package
echo '{"name":"test-package","version":"1.0.0","description":"A test package for search"}' > test-package/package.json
tar -czf test-package-1.0.0.tgz test-package

# 上传测试包
curl -X PUT "http://localhost:9081/repo/npm-local/test-package" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @test-package-1.0.0.tgz
```
预期：返回200状态码

- [ ] **步骤 3：测试基础搜索**

运行：
```bash
curl "http://localhost:9081/npm/-/v1/search?text=test"
```
预期：返回包含"test-package"的搜索结果

- [ ] **步骤 4：测试描述搜索**

运行：
```bash
curl "http://localhost:9081/npm/-/v1/search?text=search"
```
预期：返回描述包含"search"的包

- [ ] **步骤 5：测试分页**

运行：
```bash
curl "http://localhost:9081/npm/-/v1/search?text=test&size=5&from=0"
```
预期：返回最多5个结果

---

## 任务 7：最终验证和提交

**文件：**
- 无需修改代码文件

- [ ] **步骤 1：运行完整测试流程**

执行完整的测试流程：
1. 上传多个测试包
2. 搜索包名
3. 搜索描述
4. 测试分页
5. 验证所有功能正常

- [ ] **步骤 2：验证数据库查询**

检查搜索是否正确查询数据库：
```bash
sqlite3 ./data/registry.db "SELECT name, description FROM packages WHERE type = 'npm' LIMIT 5;"
```

- [ ] **步骤 3：最终Commit**

```bash
git add -A
git commit -m "feat(npm): complete search API implementation with tests"
```

---

## 注意事项

1. **性能考虑**：
   - 使用索引优化查询
   - 限制返回结果数量
   - 使用预加载减少数据库查询

2. **错误处理**：
   - 验证输入参数
   - 处理数据库错误
   - 返回明确的错误信息

3. **兼容性**：
   - 符合NPM标准搜索API规范
   - 支持基础搜索功能
   - 不影响现有功能

4. **测试覆盖**：
   - 测试基础搜索
   - 测试分页功能
   - 测试边界条件
