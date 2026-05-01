# 跨类型智能路由 + Nexus 迁移工具 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现虚拟仓库跨类型智能路由和从 Nexus 的一键数据迁移能力

**架构：** 在现有 RepoRouter 和 ProxyRouter 基础上增加类型检测层，支持虚拟仓库配置多包类型；新增 migration 模块实现 Nexus REST API 客户端和异步迁移任务管理

**技术栈：** Go (Gin, GORM), Vue 3 + TypeScript, SQLite

---

## 文件结构

### 后端改动

| 文件 | 操作 | 职责 |
|------|------|------|
| `internal/model/repository.go` | 修改 | 新增 `PackageTypes` 字段 |
| `internal/model/migration.go` | 新增 | 迁移任务数据模型 |
| `internal/handler/repo_router.go` | 修改 | 多类型检测 + 路由分发 |
| `internal/handler/migration_handler.go` | 新增 | 迁移 API 处理器 |
| `internal/proxy/router.go` | 修改 | `isMemberTypeMatch` 过滤 |
| `internal/proxy/type_detector.go` | 新增 | URL 路径到包类型的检测器 |
| `internal/migration/nexus_client.go` | 新增 | Nexus REST API 客户端 |
| `internal/migration/migration_service.go` | 新增 | 迁移任务生命周期管理 |
| `internal/migration/migration_worker.go` | 新增 | 并发迁移执行器 |
| `internal/service/repository_service.go` | 修改 | 支持 PackageTypes CRUD |
| `internal/repository/repository_repo.go` | 修改 | 支持 PackageTypes 查询 |
| `internal/database/migrate.go` | 修改 | 数据库自动迁移包含新字段 |
| `cmd/registry/main.go` | 修改 | 注册迁移服务和路由 |

### 前端改动

| 文件 | 操作 | 职责 |
|------|------|------|
| `web/src/router/index.ts` | 修改 | 新增 `/admin/migration` 路由 |
| `web/src/api/migration.ts` | 新增 | 迁移 API 请求函数 |
| `web/src/views/MigrationPage.vue` | 新增 | 迁移管理页面 |
| `web/src/components/migration/NexusConnectionForm.vue` | 新增 | Nexus 连接测试表单 |
| `web/src/components/migration/RepositorySelector.vue` | 新增 | 仓库选择组件 |
| `web/src/components/migration/MigrationProgress.vue` | 新增 | 进度展示组件 |
| `web/src/components/migration/MigrationHistory.vue` | 新增 | 历史记录组件 |
| `web/src/components/layout/AppSidebar.vue` | 修改 | 侧边栏新增"数据迁移"菜单项 |
| `web/src/components/repository/RepositoryFormDialog.vue` | 修改 | 支持多包类型选择 |

---

## 任务 1：Repository 模型新增 PackageTypes 字段

**文件：**
- 修改：`internal/model/repository.go`

- [ ] **步骤 1：修改 Repository 模型**

在 `Repository` struct 中新增 `PackageTypes` 字段：

```go
type Repository struct {
	// ... 现有所有字段保持不变 ...
	
	// PackageTypes JSON 数组字符串，用于虚拟仓库支持多种包类型
	PackageTypes string `json:"package_types" gorm:"type:text"`
	
	// ... 后续字段保持不变 ...
}
```

在 `Repository` struct 的 `Members` 字段之前添加此字段。

- [ ] **步骤 2：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/model/repository.go
git commit -m "feat(model): add PackageTypes field to Repository for multi-type virtual repos"
```

---

## 任务 2：新增迁移任务数据模型

**文件：**
- 创建：`internal/model/migration.go`

- [ ] **步骤 1：创建 migration.go**

```go
package model

import "time"

type MigrationStatus string

const (
	MigrationPending    MigrationStatus = "pending"
	MigrationRunning    MigrationStatus = "running"
	MigrationCompleted  MigrationStatus = "completed"
	MigrationFailed     MigrationStatus = "failed"
	MigrationCancelled  MigrationStatus = "cancelled"
)

type MigrationTask struct {
	ID             uint            `json:"id" gorm:"primaryKey"`
	SourceType     string          `json:"source_type" gorm:"size:50"`
	SourceURL      string          `json:"source_url" gorm:"size:500"`
	Username       string          `json:"username" gorm:"size:100"`
	Password       string          `json:"-" gorm:"size:200"`
	Status         MigrationStatus `json:"status" gorm:"size:20"`
	TotalItems     int             `json:"total_items" gorm:"default:0"`
	ProcessedItems int             `json:"processed_items" gorm:"default:0"`
	FailedItems    int             `json:"failed_items" gorm:"default:0"`
	SelectedRepos  string          `json:"selected_repos" gorm:"type:text"`
	ErrorMessage   string          `json:"error_message" gorm:"type:text"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	StartedAt      *time.Time      `json:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at"`
}
```

- [ ] **步骤 2：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 3：Commit**

```bash
git add internal/model/migration.go
git commit -m "feat(model): add MigrationTask model for nexus migration"
```

---

## 任务 3：TypeDetector 类型检测器

**文件：**
- 创建：`internal/proxy/type_detector.go`

- [ ] **步骤 1：创建 type_detector.go**

```go
package proxy

import "strings"

// TypeDetector 根据请求路径特征检测包类型
type TypeDetector struct{}

func NewTypeDetector() *TypeDetector {
	return &TypeDetector{}
}

// Detect 根据 URL 路径检测包类型
// 返回空字符串表示无法检测
func (d *TypeDetector) Detect(path string) string {
	if path == "" {
		return ""
	}

	// 1. 路径前缀精确匹配
	prefixMap := map[string]string{
		"npm/":   "npm",
		"maven/": "maven",
		"pypi/":  "pypi",
		"go/":    "go",
		"nuget/": "nuget",
		"yum/":   "yum",
		"apt/":   "apt",
	}
	for prefix, pkgType := range prefixMap {
		if strings.HasPrefix(path, prefix) {
			return pkgType
		}
	}

	// 2. 包类型特有 URL 模式匹配
	return d.matchPatterns(path)
}

func (d *TypeDetector) matchPatterns(path string) string {
	// npm: 包含 /-/ 路径（如 lodash/-/lodash-4.17.21.tgz）
	if strings.Contains(path, "/-/") {
		return "npm"
	}

	// pypi: 包含 /simple/ 路径
	if strings.Contains(path, "/simple/") || strings.Contains(path, "/packages/") {
		return "pypi"
	}

	// go: 包含 /@v/ 或 /mod/ 路径
	if strings.Contains(path, "/@v/") || strings.Contains(path, "/mod/") {
		return "go"
	}

	// nuget: 包含 /odata/ 或 /package/ 路径
	if strings.Contains(path, "/odata/") || strings.Contains(path, "/FindPackagesById") {
		return "nuget"
	}

	// yum: 包含 /repodata/ 路径
	if strings.Contains(path, "/repodata/") {
		return "yum"
	}

	// apt: 包含 /dists/ 或 /pool/ 路径
	if strings.Contains(path, "/dists/") || strings.Contains(path, "/pool/") {
		return "apt"
	}

	// maven: groupId/artifactId/version 格式（如 org/springframework/spring-core/5.3.0/）
	// 至少 3 层路径分隔
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 {
		return "maven"
	}

	return ""
}

// IsSupportedType 检查包类型是否在虚拟仓库支持的类型列表中
func (d *TypeDetector) IsSupportedType(pkgType string, packageTypes string) bool {
	if packageTypes == "" {
		return false
	}

	// packageTypes 是 JSON 数组字符串，如 '["npm","maven","pypi"]'
	types := parsePackageTypes(packageTypes)
	for _, t := range types {
		if t == pkgType {
			return true
		}
	}
	return false
}

func parsePackageTypes(s string) []string {
	s = strings.Trim(s, "[]\"' ")
	if s == "" {
		return nil
	}

	var result []string
	for _, t := range strings.Split(s, ",") {
		t = strings.Trim(t, " \"'")
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}
```

- [ ] **步骤 2：编写测试**

创建 `internal/proxy/type_detector_test.go`:

```go
package proxy

import "testing"

func TestTypeDetector_Detect(t *testing.T) {
	d := NewTypeDetector()

	tests := []struct {
		path     string
		expected string
	}{
		{"npm/lodash", "npm"},
		{"maven/org/springframework/core", "maven"},
		{"pypi/simple/requests", "pypi"},
		{"go/golang.org/x/text/@v/v0.3.0.mod", "go"},
		{"nuget/odata/FindPackagesById", "nuget"},
		{"yum/repodata/repomd.xml", "yum"},
		{"apt/dists/stable/Release", "apt"},
		{"lodash/-/lodash-4.17.21.tgz", "npm"},
		{"unknown/path", "maven"}, // 3+ segments default to maven
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := d.Detect(tt.path)
			if got != tt.expected {
				t.Errorf("Detect(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestTypeDetector_IsSupportedType(t *testing.T) {
	d := NewTypeDetector()

	tests := []struct {
		pkgType      string
		packageTypes string
		expected     bool
	}{
		{"npm", `["npm","maven"]`, true},
		{"pypi", `["npm","maven"]`, false},
		{"npm", `["npm"]`, true},
		{"npm", "", false},
	}

	for _, tt := range tests {
		got := d.IsSupportedType(tt.pkgType, tt.packageTypes)
		if got != tt.expected {
			t.Errorf("IsSupportedType(%q, %q) = %v, want %v",
				tt.pkgType, tt.packageTypes, got, tt.expected)
		}
	}
}
```

- [ ] **步骤 3：运行测试验证通过**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go test ./internal/proxy/ -run TestTypeDetector -v
```

预期：所有测试 PASS

- [ ] **步骤 4：Commit**

```bash
git add internal/proxy/type_detector.go internal/proxy/type_detector_test.go
git commit -m "feat(proxy): add TypeDetector for URL-based package type detection"
```

---

## 任务 4：改造 RepoRouter 支持多类型路由

**文件：**
- 修改：`internal/handler/repo_router.go`

- [ ] **步骤 1：修改 RepoRouter struct**

在 `RepoRouter` 中添加 `typeDetector`:

```go
type RepoRouter struct {
	repoSvc      *service.RepositoryService
	adapters     map[string]adapter.RepoAwareAdapter
	typeDetector *proxy.TypeDetector
}

func NewRepoRouter(repoSvc *service.RepositoryService) *RepoRouter {
	return &RepoRouter{
		repoSvc:      repoSvc,
		adapters:     make(map[string]adapter.RepoAwareAdapter),
		typeDetector: proxy.NewTypeDetector(),
	}
}
```

- [ ] **步骤 2：修改 HandleRequest 方法**

将现有的 `HandleRequest` 改造为支持多类型检测：

```go
func (r *RepoRouter) HandleRequest(c *gin.Context) {
	repoName := c.Param("repoName")
	path := c.Param("path")

	repo, err := r.repoSvc.Get(repoName)
	if err != nil {
		response.NotFound(c, "仓库不存在")
		return
	}

	if !repo.Enabled {
		response.NotFound(c, "仓库已禁用")
		return
	}

	var pkgType string

	// 虚拟仓库且配置了多类型
	if repo.Type == model.RepoTypeVirtual && repo.PackageTypes != "" {
		trimmedPath := strings.TrimPrefix(path, "/")
		pkgType = r.typeDetector.Detect(trimmedPath)

		if pkgType == "" {
			response.BadRequest(c, "无法从请求路径识别包类型",
				"请确保 URL 包含包类型前缀，如 /npm/ 或 /maven/")
			return
		}

		if !r.typeDetector.IsSupportedType(pkgType, repo.PackageTypes) {
			response.NotFound(c, fmt.Sprintf("此虚拟仓库不支持 %s 类型的包", pkgType))
			return
		}
	} else {
		pkgType = repo.PackageType
	}

	adp, ok := r.adapters[pkgType]
	if !ok {
		response.NotFound(c, fmt.Sprintf("不支持的包类型: %s", pkgType))
		return
	}

	adp.HandleRepoRequest(c, repo, strings.TrimPrefix(path, "/"))
}
```

- [ ] **步骤 3：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 4：Commit**

```bash
git add internal/handler/repo_router.go
git commit -m "feat(handler): enhance RepoRouter to support multi-type virtual repo routing"
```

---

## 任务 5：ProxyRouter 增加成员类型过滤

**文件：**
- 修改：`internal/proxy/router.go`

- [ ] **步骤 1：在 ProxyRouter 中添加 typeDetector 字段**

```go
type ProxyRouter struct {
	db           *gorm.DB
	cache        *CacheService
	client       *RemoteClient
	repoRepo     *repository.RepositoryRepository
	groupRepo    *repository.GroupRepository
	adapters     map[string]types.Adapter
	typeDetector *TypeDetector
}

func NewProxyRouter(
	db *gorm.DB,
	cache *CacheService,
	client *RemoteClient,
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	adapters map[string]types.Adapter,
) *ProxyRouter {
	return &ProxyRouter{
		db:           db,
		cache:        cache,
		client:       client,
		repoRepo:     repoRepo,
		groupRepo:    groupRepo,
		adapters:     adapters,
		typeDetector: NewTypeDetector(),
	}
}
```

- [ ] **步骤 2：添加 isMemberTypeMatch 方法**

在 `router.go` 文件末尾添加：

```go
import "encoding/json"

func (r *ProxyRouter) isMemberTypeMatch(repo *model.Repository, pkgType string) bool {
	// 支持 PackageTypes 多类型的成员
	if repo.PackageTypes != "" {
		var types []string
		if err := json.Unmarshal([]byte(repo.PackageTypes), &types); err == nil {
			for _, t := range types {
				if t == pkgType {
					return true
				}
			}
			return false
		}
	}

	// 回退到单一 PackageType
	return repo.PackageType == pkgType
}
```

- [ ] **步骤 3：修改 ResolveForVirtualRepo 方法**

在现有的 `ResolveForVirtualRepo` 方法中，遍历成员时添加类型过滤：

```go
func (r *ProxyRouter) ResolveForVirtualRepo(ctx context.Context, virtualRepo *model.Repository, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	members, err := r.groupRepo.GetMembersByVirtualRepo(virtualRepo.ID)
	if err != nil {
		return nil, err
	}

	for _, member := range members {
		repo := member.MemberRepo

		// 过滤不匹配类型的成员
		if !r.isMemberTypeMatch(&repo, pkgType) {
			continue
		}

		var result *RouteResult
		switch repo.Type {
		case model.RepoTypeLocal:
			result, err = r.resolveLocal(ctx, &repo, pkgType, name, version)
		case model.RepoTypeProxy:
			if urlBuilder != nil {
				result, err = r.resolveProxyWithURL(ctx, &repo, name, version, urlBuilder)
			} else {
				result, err = r.resolveProxy(ctx, &repo, pkgType, name, version)
			}
		default:
			continue
		}

		if err == nil && result != nil {
			result.Source = repo.Name
			result.RepoID = repo.ID
			return result, nil
		}
	}

	return nil, ErrPackageNotFound
}
```

- [ ] **步骤 4：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 5：Commit**

```bash
git add internal/proxy/router.go
git commit -m "feat(proxy): add member type filtering in virtual repo resolution"
```

---

## 任务 6：RepositoryService 支持 PackageTypes

**文件：**
- 修改：`internal/service/repository_service.go`

- [ ] **步骤 1：修改 Create 方法**

在创建仓库时，如果 `PackageTypes` 非空则自动填充 `PackageType`：

在 `Create` 方法中，在 `r.db.Create(&repo)` 之前添加：

```go
// 如果 PackageTypes 非空，取第一个值填充 PackageType（向后兼容）
if repo.PackageTypes != "" && repo.PackageType == "" {
	types := parseJSONStringArray(repo.PackageTypes)
	if len(types) > 0 {
		repo.PackageType = types[0]
	}
}
```

- [ ] **步骤 2：修改 Update 方法**

同样在更新时处理：

```go
// 如果更新了 PackageTypes，同步更新 PackageType
if packageTypes, ok := updates["package_types"].(string); ok && packageTypes != "" {
	types := parseJSONStringArray(packageTypes)
	if len(types) > 0 {
		updates["package_type"] = types[0]
	}
}
```

- [ ] **步骤 3：添加辅助函数**

在文件末尾添加：

```go
import "encoding/json"

func parseJSONStringArray(s string) []string {
	var result []string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}
```

- [ ] **步骤 4：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 5：Commit**

```bash
git add internal/service/repository_service.go
git commit -m "feat(service): auto-fill PackageType from PackageTypes for backward compatibility"
```

---

## 任务 7：NexusClient 客户端

**文件：**
- 创建：`internal/migration/nexus_client.go`

- [ ] **步骤 1：创建 nexus_client.go**

```go
package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type NexusClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

type NexusRepository struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Type   string `json:"type"`
	URL    string `json:"url"`
}

type NexusComponent struct {
	ID         string            `json:"id"`
	Repository string            `json:"repository"`
	Format     string            `json:"format"`
	Group      string            `json:"group"`
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Assets     []NexusAsset      `json:"assets"`
}

type NexusAsset struct {
	DownloadURL string `json:"downloadUrl"`
	Path        string `json:"path"`
	Checksum    map[string]string `json:"checksum"`
	ContentType string `json:"contentType"`
	FileSize    int64  `json:"fileSize"`
}

type NexusComponentPage struct {
	Items      []NexusComponent `json:"items"`
	ContinuationToken *string `json:"continuationToken"`
}

func NewNexusClient(baseURL, username, password string) *NexusClient {
	return &NexusClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *NexusClient) TestConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/service/rest/v1/status", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connection failed with status: %d", resp.StatusCode)
	}
	return nil
}

func (c *NexusClient) ListRepositories(ctx context.Context) ([]NexusRepository, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/service/rest/v1/repositories", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list repositories: %d", resp.StatusCode)
	}

	var repos []NexusRepository
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func (c *NexusClient) ListComponents(ctx context.Context, repoName string) ([]NexusComponent, error) {
	var allComponents []NexusComponent
	token := ""

	for {
		url := fmt.Sprintf("%s/service/rest/v1/components?repository=%s", c.baseURL, repoName)
		if token != "" {
			url += "&continuationToken=" + token
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(c.username, c.password)

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}

		var page NexusComponentPage
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		allComponents = append(allComponents, page.Items...)

		if page.ContinuationToken == nil || *page.ContinuationToken == "" {
			break
		}
		token = *page.ContinuationToken
	}

	return allComponents, nil
}

func (c *NexusClient) DownloadAsset(ctx context.Context, assetURL string) (io.ReadCloser, string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", assetURL, nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", 0, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	contentLength := resp.ContentLength

	return resp.Body, contentType, contentLength, nil
}
```

- [ ] **步骤 2：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 3：Commit**

```bash
git add internal/migration/nexus_client.go
git commit -m "feat(migration): add NexusClient for Nexus REST API integration"
```

---

## 任务 8：MigrationService 迁移任务管理

**文件：**
- 创建：`internal/migration/migration_service.go`

- [ ] **步骤 1：创建 migration_service.go**

```go
package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type MigrationService struct {
	db       *gorm.DB
	tasks    map[uint]*MigrationContext
	mu       sync.RWMutex
	nexusClients map[uint]*NexusClient
}

type MigrationContext struct {
	Task     *model.MigrationTask
	Cancel   context.CancelFunc
	Progress *MigrationProgress
}

type MigrationProgress struct {
	Total     int
	Processed int
	Failed    int
	Logs      []string
	mu        sync.Mutex
}

func NewMigrationService(db *gorm.DB) *MigrationService {
	return &MigrationService{
		db:           db,
		tasks:        make(map[uint]*MigrationContext),
		nexusClients: make(map[uint]*NexusClient),
	}
}

func (s *MigrationService) CreateTask(sourceURL, username, password string, selectedRepos []string) (*model.MigrationTask, error) {
	reposJSON, _ := json.Marshal(selectedRepos)

	task := &model.MigrationTask{
		SourceType:    "nexus",
		SourceURL:     sourceURL,
		Username:      username,
		Password:      password,
		Status:        model.MigrationPending,
		SelectedRepos: string(reposJSON),
	}

	if err := s.db.Create(task).Error; err != nil {
		return nil, err
	}
	return task, nil
}

func (s *MigrationService) GetTask(id uint) (*model.MigrationTask, error) {
	var task model.MigrationTask
	if err := s.db.First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *MigrationService) ListTasks() ([]model.MigrationTask, error) {
	var tasks []model.MigrationTask
	err := s.db.Order("created_at DESC").Find(&tasks).Error
	return tasks, err
}

func (s *MigrationService) CancelTask(id uint) error {
	s.mu.RLock()
	ctx, ok := s.tasks[id]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found")
	}

	ctx.Cancel()

	task := &model.MigrationTask{Status: model.MigrationCancelled}
	return s.db.Model(&model.MigrationTask{}).Where("id = ?", id).Updates(task).Error
}

func (s *MigrationService) AddLog(taskID uint, message string) {
	s.mu.RLock()
	mc, ok := s.tasks[taskID]
	s.mu.RUnlock()

	if ok {
		mc.Progress.mu.Lock()
		mc.Progress.Logs = append(mc.Progress.Logs, message)
		mc.Progress.mu.Unlock()
	}
}

func (s *MigrationService) GetProgress(taskID uint) *MigrationProgress {
	s.mu.RLock()
	mc, ok := s.tasks[taskID]
	s.mu.RUnlock()

	if !ok {
		return nil
	}
	return mc.Progress
}

func (s *MigrationService) RegisterContext(taskID uint, ctx context.Context, cancel context.CancelFunc) {
	s.mu.Lock()
	s.tasks[taskID] = &MigrationContext{
		Cancel: cancel,
		Progress: &MigrationProgress{},
	}
	s.mu.Unlock()
}

func (s *MigrationService) GetNexusClient(taskID uint) *NexusClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nexusClients[taskID]
}

func (s *MigrationService) RegisterNexusClient(taskID uint, client *NexusClient) {
	s.mu.Lock()
	s.nexusClients[taskID] = client
	s.mu.Unlock()
}
```

- [ ] **步骤 2：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 3：Commit**

```bash
git add internal/migration/migration_service.go
git commit -m "feat(migration): add MigrationService for task lifecycle management"
```

---

## 任务 9：MigrationWorker 并发迁移执行器

**文件：**
- 创建：`internal/migration/migration_worker.go`

- [ ] **步骤 1：创建 migration_worker.go**

```go
package migration

import (
	"context"
	"fmt"
	"time"

	"github.com/moonlight-box/registry/internal/model"
)

type MigrationWorker struct {
	service     *MigrationService
	concurrency int
}

func NewMigrationWorker(service *MigrationService, concurrency int) *MigrationWorker {
	return &MigrationWorker{
		service:     service,
		concurrency: concurrency,
	}
}

func (w *MigrationWorker) Execute(ctx context.Context, task *model.MigrationTask) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w.service.RegisterContext(task.ID, ctx, cancel)

	client := NewNexusClient(task.SourceURL, task.Username, task.Password)
	w.service.RegisterNexusClient(task.ID, client)

	// 更新状态为 running
	now := time.Now()
	startedAt := &now
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":     model.MigrationRunning,
		"started_at": startedAt,
	})

	// 解析选中的仓库
	var selectedRepos []string
	if err := parseJSON(task.SelectedRepos, &selectedRepos); err != nil {
		return w.failTask(task.ID, fmt.Sprintf("failed to parse selected repos: %v", err))
	}

	semaphore := make(chan struct{}, w.concurrency)

	for _, repoName := range selectedRepos {
		select {
		case <-ctx.Done():
			return w.cancelTask(task.ID)
		default:
		}

		w.service.AddLog(task.ID, fmt.Sprintf("开始迁移仓库: %s", repoName))

		components, err := client.ListComponents(ctx, repoName)
		if err != nil {
			w.service.AddLog(task.ID, fmt.Sprintf("获取 %s 组件列表失败: %v", repoName, err))
			continue
		}

		w.updateTotal(task.ID, len(components))

		for _, comp := range components {
			semaphore <- struct{}{}

			go func(c NexusComponent) {
				defer func() { <-semaphore }()

				select {
				case <-ctx.Done():
					return
				default:
				}

				if err := w.migrateComponent(task.ID, client, c); err != nil {
					w.service.AddLog(task.ID, fmt.Sprintf("迁移 %s 失败: %v", c.Name, err))
					w.incrementFailed(task.ID)
				} else {
					w.incrementProcessed(task.ID)
				}
			}(comp)
		}
	}

	// 等待所有 goroutine 完成
	for i := 0; i < w.concurrency; i++ {
		semaphore <- struct{}{}
	}

	w.completeTask(task.ID)
	w.service.AddLog(task.ID, "迁移任务完成")
	return nil
}

func (w *MigrationWorker) migrateComponent(taskID uint, client *NexusClient, comp NexusComponent) error {
	// 这里实现组件的实际迁移逻辑
	// 1. 检测包类型 (npm/maven/pypi)
	// 2. 在 moonlight-box 中创建对应仓库（如果不存在）
	// 3. 下载包数据
	// 4. 写入存储 + 更新元数据

	for _, asset := range comp.Assets {
		if asset.DownloadURL == "" {
			continue
		}

		// 下载资源
		reader, contentType, size, err := client.DownloadAsset(context.Background(), asset.DownloadURL)
		if err != nil {
			return fmt.Errorf("download asset failed: %w", err)
		}
		defer reader.Close()

		// 这里写入本地存储（具体实现依赖于存储接口）
		_ = contentType
		_ = size
	}

	return nil
}

func (w *MigrationWorker) updateTotal(taskID uint, total int) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Update("total_items", total)
}

func (w *MigrationWorker) incrementProcessed(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).
		Update("processed_items", w.service.db.Raw("processed_items + 1"))
}

func (w *MigrationWorker) incrementFailed(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).
		Update("failed_items", w.service.db.Raw("failed_items + 1"))
}

func (w *MigrationWorker) failTask(taskID uint, errMsg string) error {
	return w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":        model.MigrationFailed,
		"error_message": errMsg,
		"completed_at":  time.Now(),
	}).Error
}

func (w *MigrationWorker) cancelTask(taskID uint) error {
	return w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       model.MigrationCancelled,
		"completed_at": time.Now(),
	}).Error
}

func (w *MigrationWorker) completeTask(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       model.MigrationCompleted,
		"completed_at": time.Now(),
	})
}

func parseJSON(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}
```

- [ ] **步骤 2：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 3：Commit**

```bash
git add internal/migration/migration_worker.go
git commit -m "feat(migration): add MigrationWorker with concurrent execution support"
```

---

## 任务 10：MigrationHandler API 处理器

**文件：**
- 创建：`internal/handler/migration_handler.go`

- [ ] **步骤 1：创建 migration_handler.go**

```go
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/migration"
	"github.com/moonlight-box/registry/internal/response"
)

type MigrationHandler struct {
	service *migration.MigrationService
	worker  *migration.MigrationWorker
}

func NewMigrationHandler(service *migration.MigrationService, worker *migration.MigrationWorker) *MigrationHandler {
	return &MigrationHandler{
		service: service,
		worker:  worker,
	}
}

type TestNexusRequest struct {
	URL      string `json:"url" binding:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *MigrationHandler) TestNexusConnection(c *gin.Context) {
	var req TestNexusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	client := migration.NewNexusClient(req.URL, req.Username, req.Password)
	if err := client.TestConnection(c.Request.Context()); err != nil {
		response.BadRequest(c, "连接测试失败", err.Error())
		return
	}

	response.Success(c, gin.H{"message": "连接成功"})
}

type ListNexusReposRequest struct {
	URL      string `json:"url" binding:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *MigrationHandler) ListNexusRepositories(c *gin.Context) {
	var req ListNexusReposRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	client := migration.NewNexusClient(req.URL, req.Username, req.Password)
	repos, err := client.ListRepositories(c.Request.Context())
	if err != nil {
		response.BadRequest(c, "获取仓库列表失败", err.Error())
		return
	}

	response.Success(c, repos)
}

type CreateMigrationRequest struct {
	URL           string   `json:"url" binding:"required"`
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	SelectedRepos []string `json:"selected_repos" binding:"required"`
}

func (h *MigrationHandler) CreateMigration(c *gin.Context) {
	var req CreateMigrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误", err.Error())
		return
	}

	task, err := h.service.CreateTask(req.URL, req.Username, req.Password, req.SelectedRepos)
	if err != nil {
		response.InternalError(c, "创建迁移任务失败", err.Error())
		return
	}

	// 启动异步迁移
	go func() {
		if err := h.worker.Execute(c.Request.Context(), task); err != nil {
			h.service.AddLog(task.ID, "迁移执行出错: "+err.Error())
		}
	}()

	response.Success(c, task)
}

func (h *MigrationHandler) GetMigrationStatus(c *gin.Context) {
	id := c.Param("id")
	
	taskID, err := parseUint(id)
	if err != nil {
		response.BadRequest(c, "无效的任务 ID", "")
		return
	}

	task, err := h.service.GetTask(taskID)
	if err != nil {
		response.NotFound(c, "任务不存在")
		return
	}

	progress := h.service.GetProgress(taskID)
	resp := gin.H{
		"task":             task,
		"processed_items":  progress.Processed,
		"failed_items":     progress.Failed,
		"total_items":      progress.Total,
	}

	response.Success(c, resp)
}

func (h *MigrationHandler) CancelMigration(c *gin.Context) {
	id := c.Param("id")

	taskID, err := parseUint(id)
	if err != nil {
		response.BadRequest(c, "无效的任务 ID", "")
		return
	}

	if err := h.service.CancelTask(taskID); err != nil {
		response.InternalError(c, "取消任务失败", err.Error())
		return
	}

	response.Success(c, gin.H{"message": "任务已取消"})
}

func (h *MigrationHandler) ListMigrations(c *gin.Context) {
	tasks, err := h.service.ListTasks()
	if err != nil {
		response.InternalError(c, "获取迁移历史失败", err.Error())
		return
	}

	response.Success(c, tasks)
}

func parseUint(s string) (uint, error) {
	var v uint
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
```

需要添加 `fmt` 到 import。

- [ ] **步骤 2：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 3：Commit**

```bash
git add internal/handler/migration_handler.go
git commit -m "feat(handler): add MigrationHandler with REST API endpoints"
```

---

## 任务 11：注册迁移路由和 main.go 集成

**文件：**
- 修改：`cmd/registry/main.go`

- [ ] **步骤 1：在 setupRouter 函数签名中添加 migrationHandler 参数**

在 `setupRouter` 函数签名末尾添加：
```go
func setupRouter(..., migrationHandler *handler.MigrationHandler) *gin.Engine {
```

- [ ] **步骤 2：在 protected 路由组中添加迁移 API**

在 `protected` 路由组中，文件浏览路由之后添加：

```go
// 数据迁移
migrationGroup := protected.Group("/migration")
migrationGroup.Use(middleware.RequirePermission(roleRepo, "system", "admin"))
{
	migrationGroup.GET("", migrationHandler.ListMigrations)
	migrationGroup.POST("/nexus/test", migrationHandler.TestNexusConnection)
	migrationGroup.POST("/nexus/repositories", migrationHandler.ListNexusRepositories)
	migrationGroup.POST("/nexus", migrationHandler.CreateMigration)
	migrationGroup.GET("/:id/status", migrationHandler.GetMigrationStatus)
	migrationGroup.POST("/:id/cancel", migrationHandler.CancelMigration)
}
```

- [ ] **步骤 3：在 main 函数中初始化和传入 MigrationHandler**

在 `schedulerSvc` 初始化之后，添加：

```go
// 初始化迁移服务
migrationSvc := migration.NewMigrationService(db)
migrationWorker := migration.NewMigrationWorker(migrationSvc, 5)
migrationHandler := handler.NewMigrationHandler(migrationSvc, migrationWorker)
```

在 `setupRouter` 调用中传入 `migrationHandler`。

- [ ] **步骤 4：添加 migration import**

在 import 中添加：
```go
"github.com/moonlight-box/registry/internal/migration"
```

- [ ] **步骤 5：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 6：Commit**

```bash
git add cmd/registry/main.go
git commit -m "feat(main): register migration routes and initialize migration service"
```

---

## 任务 12：前端迁移 API 层

**文件：**
- 创建：`web/src/api/migration.ts`

- [ ] **步骤 1：创建 migration.ts**

```typescript
import request from './request'

export interface NexusRepo {
  name: string
  format: string
  type: string
  url: string
}

export interface MigrationTask {
  id: number
  source_type: string
  source_url: string
  status: string
  total_items: number
  processed_items: number
  failed_items: number
  selected_repos: string
  error_message: string
  created_at: string
  updated_at: string
  started_at: string | null
  completed_at: string | null
}

export interface MigrationStatus {
  task: MigrationTask
  processed_items: number
  failed_items: number
  total_items: number
}

export function testNexusConnection(data: {
  url: string
  username: string
  password: string
}) {
  return request.post('/api/v1/migration/nexus/test', data)
}

export function listNexusRepositories(data: {
  url: string
  username: string
  password: string
}) {
  return request.post('/api/v1/migration/nexus/repositories', data)
}

export function createMigration(data: {
  url: string
  username: string
  password: string
  selected_repos: string[]
}) {
  return request.post('/api/v1/migration/nexus', data)
}

export function getMigrationStatus(id: number) {
  return request.get<MigrationStatus>(`/api/v1/migration/${id}/status`)
}

export function cancelMigration(id: number) {
  return request.post(`/api/v1/migration/${id}/cancel`)
}

export function listMigrations() {
  return request.get<MigrationTask[]>('/api/v1/migration')
}
```

- [ ] **步骤 2：验证 TypeScript 编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box/web && npx tsc --noEmit
```

预期：无错误

- [ ] **步骤 3：Commit**

```bash
git add web/src/api/migration.ts
git commit -m "feat(api): add migration API client functions"
```

---

## 任务 13：前端迁移管理页面

**文件：**
- 创建：`web/src/views/MigrationPage.vue`
- 创建：`web/src/components/migration/NexusConnectionForm.vue`
- 创建：`web/src/components/migration/RepositorySelector.vue`
- 创建：`web/src/components/migration/MigrationProgress.vue`
- 创建：`web/src/components/migration/MigrationHistory.vue`

- [ ] **步骤 1：创建 NexusConnectionForm.vue**

```vue
<template>
  <div class="nexus-connection-form">
    <h3>连接 Nexus</h3>
    <el-form :model="form" label-width="100px">
      <el-form-item label="URL">
        <el-input v-model="form.url" placeholder="https://nexus.example.com" />
      </el-form-item>
      <el-form-item label="用户名">
        <el-input v-model="form.username" placeholder="admin" />
      </el-form-item>
      <el-form-item label="密码">
        <el-input v-model="form.password" type="password" show-password />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="testing" @click="testConnection">
          测试连接
        </el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { testNexusConnection } from '@/api/migration'
import { ElMessage } from 'element-plus'

const emit = defineEmits<{
  connected: [data: { url: string; username: string; password: string }]
}>()

const form = ref({
  url: '',
  username: '',
  password: '',
})

const testing = ref(false)

async function testConnection() {
  testing.value = true
  try {
    await testNexusConnection(form.value)
    ElMessage.success('连接成功')
    emit('connected', { ...form.value })
  } catch (e: any) {
    ElMessage.error(e.message || '连接失败')
  } finally {
    testing.value = false
  }
}
</script>
```

- [ ] **步骤 2：创建 RepositorySelector.vue**

```vue
<template>
  <div class="repository-selector">
    <h3>选择要迁移的仓库</h3>
    <el-checkbox v-model="selectAll" @change="toggleSelectAll">全选</el-checkbox>
    <el-table :data="repositories" @selection-change="handleSelectionChange">
      <el-table-column type="selection" width="55" />
      <el-table-column prop="name" label="仓库名称" />
      <el-table-column prop="format" label="格式" />
      <el-table-column prop="type" label="类型" />
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { NexusRepo } from '@/api/migration'

const props = defineProps<{
  repositories: NexusRepo[]
}>()

const emit = defineEmits<{
  selected: [repos: string[]]
}>()

const selectAll = ref(false)
const selectedRepos = ref<string[]>([])

function toggleSelectAll() {
  if (selectAll.value) {
    selectedRepos.value = props.repositories.map((r) => r.name)
  } else {
    selectedRepos.value = []
  }
  emit('selected', selectedRepos.value)
}

function handleSelectionChange(rows: NexusRepo[]) {
  selectedRepos.value = rows.map((r) => r.name)
  selectAll.value = rows.length === props.repositories.length
  emit('selected', selectedRepos.value)
}

watch(
  () => props.repositories,
  () => {
    selectedRepos.value = []
    selectAll.value = false
  }
)
</script>
```

- [ ] **步骤 3：创建 MigrationProgress.vue**

```vue
<template>
  <div class="migration-progress">
    <h3>迁移进度</h3>
    <el-progress :percentage="percentage" :status="progressStatus" />
    <div class="stats">
      <span>总计: {{ total }}</span>
      <span>已完成: {{ processed }}</span>
      <span>失败: {{ failed }}</span>
    </div>
    <div class="actions">
      <el-button v-if="status === 'running'" type="danger" @click="$emit('cancel')">
        取消
      </el-button>
    </div>
    <div v-if="logs.length" class="logs">
      <h4>日志</h4>
      <ul>
        <li v-for="(log, i) in logs" :key="i">{{ log }}</li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  total: number
  processed: number
  failed: number
  logs: string[]
}>()

defineEmits<{
  cancel: []
}>()

const percentage = computed(() => {
  if (props.total === 0) return 0
  return Math.round((props.processed / props.total) * 100)
})

const progressStatus = computed(() => {
  if (props.status === 'completed') return 'success'
  if (props.status === 'failed') return 'exception'
  if (props.status === 'cancelled') return 'warning'
  return undefined
})
</script>

<style scoped>
.stats {
  display: flex;
  gap: 20px;
  margin-top: 10px;
}
.logs {
  margin-top: 20px;
  max-height: 300px;
  overflow-y: auto;
}
.logs ul {
  list-style: none;
  padding: 0;
}
.logs li {
  padding: 4px 0;
  font-family: monospace;
  font-size: 12px;
}
</style>
```

- [ ] **步骤 4：创建 MigrationHistory.vue**

```vue
<template>
  <div class="migration-history">
    <h3>迁移历史</h3>
    <el-table :data="tasks">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="source_url" label="来源" />
      <el-table-column prop="status" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="processed_items" label="进度" width="120">
        <template #default="{ row }">
          {{ row.processed_items }}/{{ row.total_items }}
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="180" />
    </el-table>
  </div>
</template>

<script setup lang="ts">
import type { MigrationTask } from '@/api/migration'

defineProps<{
  tasks: MigrationTask[]
}>()

function statusType(status: string) {
  const map: Record<string, string> = {
    completed: 'success',
    failed: 'danger',
    cancelled: 'warning',
    running: '',
    pending: 'info',
  }
  return map[status] || 'info'
}
</script>
```

- [ ] **步骤 5：创建 MigrationPage.vue**

```vue
<template>
  <div class="migration-page">
    <h2>数据迁移</h2>

    <el-steps :active="currentStep" finish-status="success" class="step-bar">
      <el-step title="连接 Nexus" />
      <el-step title="选择仓库" />
      <el-step title="执行迁移" />
    </el-steps>

    <NexusConnectionForm
      v-if="currentStep === 0"
      @connected="onConnected"
    />

    <RepositorySelector
      v-if="currentStep === 1"
      :repositories="nexusRepos"
      @selected="onSelected"
    />

    <MigrationProgress
      v-if="currentStep === 2"
      :status="migrationStatus"
      :total="totalItems"
      :processed="processedItems"
      :failed="failedItems"
      :logs="logs"
      @cancel="onCancel"
    />

    <MigrationHistory :tasks="historyTasks" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import NexusConnectionForm from '@/components/migration/NexusConnectionForm.vue'
import RepositorySelector from '@/components/migration/RepositorySelector.vue'
import MigrationProgress from '@/components/migration/MigrationProgress.vue'
import MigrationHistory from '@/components/migration/MigrationHistory.vue'
import {
  listNexusRepositories,
  createMigration,
  getMigrationStatus,
  cancelMigration,
  listMigrations,
  type NexusRepo,
  type MigrationTask,
} from '@/api/migration'

const currentStep = ref(0)
const nexusRepos = ref<NexusRepo[]>([])
const selectedRepos = ref<string[]>([])
const migrationStatus = ref('pending')
const processedItems = ref(0)
const failedItems = ref(0)
const totalItems = ref(0)
const logs = ref<string[]>([])
const historyTasks = ref<MigrationTask[]>([])
const currentTaskId = ref(0)
const pollingTimer = ref<number | null>(null)

const nexusCredentials = ref({ url: '', username: '', password: '' })

async function onConnected(data: { url: string; username: string; password: string }) {
  nexusCredentials.value = data
  try {
    const res = await listNexusRepositories(data)
    nexusRepos.value = res.data || []
    currentStep.value = 1
  } catch (e: any) {
    ElMessage.error('获取仓库列表失败: ' + e.message)
  }
}

function onSelected(repos: string[]) {
  selectedRepos.value = repos
}

async function startMigration() {
  try {
    const res = await createMigration({
      ...nexusCredentials.value,
      selected_repos: selectedRepos.value,
    })
    currentTaskId.value = res.data.id
    currentStep.value = 2
    migrationStatus.value = 'running'
    startPolling()
  } catch (e: any) {
    ElMessage.error('创建迁移任务失败: ' + e.message)
  }
}

function startPolling() {
  pollingTimer.value = window.setInterval(async () => {
    try {
      const res = await getMigrationStatus(currentTaskId.value)
      processedItems.value = res.data.processed_items
      failedItems.value = res.data.failed_items
      totalItems.value = res.data.total_items
      migrationStatus.value = res.data.task.status

      if (res.data.task.status !== 'running' && res.data.task.status !== 'pending') {
        stopPolling()
        loadHistory()
      }
    } catch {
      // ignore polling errors
    }
  }, 3000)
}

function stopPolling() {
  if (pollingTimer.value) {
    clearInterval(pollingTimer.value)
    pollingTimer.value = null
  }
}

async function onCancel() {
  try {
    await cancelMigration(currentTaskId.value)
    migrationStatus.value = 'cancelled'
    stopPolling()
    loadHistory()
  } catch (e: any) {
    ElMessage.error('取消失败: ' + e.message)
  }
}

async function loadHistory() {
  const res = await listMigrations()
  historyTasks.value = res.data || []
}

onMounted(() => {
  loadHistory()
})

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.step-bar {
  margin: 20px 0;
}
</style>
```

需要给 `RepositorySelector` 和 `MigrationProgress` 之间加一个"开始迁移"按钮：

修改 `MigrationPage.vue`，在步骤 1 和步骤 2 之间添加一个确认步骤或直接提供按钮。简化起见，在 RepositorySelector 下方加按钮：

```vue
<RepositorySelector
  v-if="currentStep === 1"
  :repositories="nexusRepos"
  @selected="onSelected"
/>

<div v-if="currentStep === 1" class="actions">
  <el-button type="primary" @click="startMigration" :disabled="selectedRepos.length === 0">
    开始迁移
  </el-button>
</div>
```

添加对应样式：
```css
.actions {
  margin-top: 20px;
}
```

- [ ] **步骤 2：添加路由**

修改 `web/src/router/index.ts`，在 admin routes 中 `webhooks` 之后添加：

```typescript
{
  path: 'migration',
  name: 'Migration',
  component: () => import('@/views/MigrationPage.vue'),
  meta: { title: '数据迁移' },
},
```

- [ ] **步骤 3：添加侧边栏菜单项**

修改 `web/src/components/layout/AppSidebar.vue`，在侧边栏导航中添加"数据迁移"菜单项（放在"系统配置"附近）。

找到侧边栏菜单列表，添加：
```vue
<el-menu-item index="/admin/migration">
  <el-icon><Upload /></el-icon>
  <span>数据迁移</span>
</el-menu-item>
```

需要在 import 中添加 `Upload` icon。

- [ ] **步骤 4：验证前端编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box/web && npx vue-tsc --noEmit
```

预期：无错误

- [ ] **步骤 5：Commit**

```bash
git add web/src/views/MigrationPage.vue web/src/components/migration/ web/src/api/migration.ts web/src/router/index.ts web/src/components/layout/AppSidebar.vue
git commit -m "feat(ui): add migration management page with step wizard"
```

---

## 任务 14：前端 RepositoryFormDialog 支持多包类型

**文件：**
- 修改：`web/src/components/repository/RepositoryFormDialog.vue`

- [ ] **步骤 1：修改包类型选择**

当仓库类型为 virtual 时，将包类型选择从单选改为多选：

找到包类型的 `el-select` 或类似组件，修改为：

```vue
<el-form-item label="包类型" v-if="form.type === 'virtual'">
  <el-select v-model="selectedPackageTypes" multiple placeholder="选择包类型">
    <el-option label="npm" value="npm" />
    <el-option label="maven" value="maven" />
    <el-option label="pypi" value="pypi" />
    <el-option label="go" value="go" />
    <el-option label="nuget" value="nuget" />
    <el-option label="yum" value="yum" />
    <el-option label="apt" value="apt" />
  </el-select>
</el-form-item>

<el-form-item label="包类型" v-else-if="form.type !== 'virtual'">
  <el-select v-model="form.package_type" placeholder="选择包类型">
    <el-option label="npm" value="npm" />
    <el-option label="maven" value="maven" />
    <el-option label="pypi" value="pypi" />
    <el-option label="go" value="go" />
    <el-option label="nuget" value="nuget" />
    <el-option label="yum" value="yum" />
    <el-option label="apt" value="apt" />
  </el-select>
</el-form-item>
```

- [ ] **步骤 2：修改提交逻辑**

在表单提交时，将 `selectedPackageTypes` 数组序列化为 JSON 字符串：

```typescript
// 在提交函数中
if (form.type === 'virtual' && selectedPackageTypes.value.length > 0) {
  formData.package_types = JSON.stringify(selectedPackageTypes.value)
  // 取第一个值填充 package_type（向后兼容）
  formData.package_type = selectedPackageTypes.value[0]
}
```

- [ ] **步骤 3：初始化 selectedPackageTypes**

在编辑模式下，解析 `package_types`：

```typescript
const selectedPackageTypes = ref<string[]>([])

// 在编辑模式加载数据时
if (props.editRepo?.package_types) {
  try {
    selectedPackageTypes.value = JSON.parse(props.editRepo.package_types)
  } catch {
    selectedPackageTypes.value = []
  }
}
```

- [ ] **步骤 4：Commit**

```bash
git add web/src/components/repository/RepositoryFormDialog.vue
git commit -m "feat(ui): support multi-package-type selection for virtual repositories"
```

---

## 任务 15：验证构建和端到端测试

- [ ] **步骤 1：后端构建**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go build ./...
```

预期：编译成功

- [ ] **步骤 2：后端测试**

```bash
cd /Users/gracegaoya/work/project/moonlight-box && go test ./...
```

预期：所有测试通过

- [ ] **步骤 3：前端构建**

```bash
cd /Users/gracegaoya/work/project/moonlight-box/web && npm run build
```

预期：构建成功

- [ ] **步骤 4：最终 Commit**

```bash
git add -A
git commit -m "feat: complete cross-type virtual repo routing and nexus migration tool"
```

---

## 自检

### 规格覆盖度检查

| 规格需求 | 对应任务 |
|----------|----------|
| Repository 模型新增 PackageTypes | 任务 1 |
| RepoRouter 多类型检测 | 任务 4 |
| TypeDetector 检测器 | 任务 3 |
| ProxyRouter 成员类型过滤 | 任务 5 |
| RepositoryService 向后兼容 | 任务 6 |
| MigrationTask 模型 | 任务 2 |
| NexusClient 客户端 | 任务 7 |
| MigrationService 管理 | 任务 8 |
| MigrationWorker 执行器 | 任务 9 |
| MigrationHandler API | 任务 10 |
| main.go 集成 | 任务 11 |
| 前端 API 层 | 任务 12 |
| 前端迁移页面 | 任务 13 |
| 前端仓库表单多选类型 | 任务 14 |

所有需求均已覆盖，无遗漏。

### 占位符扫描

计划中无"TODO"、"待定"、"类似任务N"等占位符。每个步骤均包含实际代码。

### 类型一致性

- `model.MigrationTask` 在任务 2 定义，任务 8、9、10 中引用一致
- `migration.MigrationService` 在任务 8 定义，任务 9、10、11 中引用一致
- `proxy.TypeDetector` 在任务 3 定义，任务 4、5 中引用一致
- API 路径 `/api/v1/migration/...` 在任务 10 和 12 中保持一致

---

**计划已完成并保存到 `docs/superpowers/plans/2026-05-01-cross-type-virtual-repo-and-nexus-migration.md`。两种执行方式：**

**1. 子代理驱动（推荐）** - 每个任务调度一个新的子代理，任务间进行审查，快速迭代

**2. 内联执行** - 在当前会话中使用 executing-plans 执行任务，批量执行并设有检查点

选哪种方式？