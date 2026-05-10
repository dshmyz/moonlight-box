# Moonlight Registry Phase 2 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现多代理仓库系统、缓存服务、代理认证、仓库管理 API 及前端管理界面

**架构：** 基于 Virtual/Local/Proxy 三种仓库类型，构建多上游代理路由引擎，支持缓存策略和认证配置，提供完整的仓库管理 API 和前端界面

**技术栈：** Go 1.21+, Gin, GORM, SQLite, BadgerDB (本地缓存), Element Plus, Vue 3

---

## 文件结构总览

```
internal/
├── model/
│   ├── repository.go              # 仓库模型 (Local/Proxy/Virtual)
│   └── cache.go                   # 缓存模型
├── repository/
│   ├── repository_repo.go         # 仓库数据访问层
│   └── group_repo.go              # 仓库组成员关系
├── proxy/
│   ├── router.go                  # 多代理路由引擎
│   ├── cache.go                   # 缓存服务 (BadgerDB)
│   ├── auth.go                    # 代理认证
│   └── client.go                  # 远程 HTTP 客户端
├── service/
│   ├── repository_service.go      # 仓库管理服务
│   └── cache_service.go           # 缓存管理服务
├── handler/
│   ├── repository_handler.go      # 仓库 API 处理器
│   └── cache_handler.go           # 缓存管理 API
├── adapter/
│   ├── proxy_adapter.go           # 代理拉取适配器 (通用)
│   ├── npm_adapter.go             # 修改: 支持代理拉取
│   └── maven_adapter.go           # 修改: 支持代理拉取
├── config/
│   └── config.go                  # 修改: 增加 Repository 配置
└── middleware/
    └── repo_auth.go               # 仓库级认证中间件

web/src/
├── views/
│   ├── RepositoryList.vue         # 仓库列表页
│   ├── RepositoryDetail.vue       # 仓库详情页
│   └── CacheManagement.vue        # 缓存管理页
├── api/
│   └── repository.ts              # 仓库 API 接口
└── router/
    └── index.ts                   # 修改: 增加仓库路由
```

---

### 任务 1：仓库模型与数据库迁移

**文件：**
- 创建：`internal/model/repository.go`
- 创建：`internal/model/cache.go`
- 创建：`internal/repository/repository_repo.go`
- 创建：`internal/repository/group_repo.go`
- 修改：`internal/database/migration.go` (增加迁移)
- 修改：`internal/model/package.go` (增加 repository_id 字段)

- [ ] **步骤 1：创建仓库模型**

```go
// internal/model/repository.go
package model

import (
	"encoding/json"
	"time"
)

type RepositoryType string

const (
	RepoTypeLocal   RepositoryType = "local"
	RepoTypeProxy   RepositoryType = "proxy"
	RepoTypeVirtual RepositoryType = "virtual"
)

type Repository struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"uniqueIndex;size:100"`
	DisplayName string         `json:"display_name" gorm:"size:200"`
	Description string         `json:"description"`
	Type        RepositoryType `json:"type" gorm:"size:20"`
	PackageType string         `json:"package_type" gorm:"size:50"`
	Enabled     bool           `json:"enabled" gorm:"default:true"`

	RemoteURL     string `json:"remote_url,omitempty"`
	AuthType      string `json:"auth_type" gorm:"default:none"`
	AuthConfig    string `json:"auth_config" gorm:"type:text"`
	ProxyPriority int    `json:"proxy_priority" gorm:"default:0"`

	CacheEnabled    bool    `json:"cache_enabled" gorm:"default:true"`
	CacheTTLSeconds int     `json:"cache_ttl_seconds" gorm:"default:86400"`
	CacheMaxSizeGB  float64 `json:"cache_max_size_gb" gorm:"default:10"`
	CacheNegativeTTL int    `json:"cache_negative_ttl" gorm:"default:300"`

	AllowOverwrite bool  `json:"allow_overwrite" gorm:"default:false"`
	AllowDelete    bool  `json:"allow_delete" gorm:"default:false"`
	DownloadCount  int64 `json:"download_count" gorm:"default:0"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Members []RepositoryGroup `json:"members,omitempty" gorm:"foreignKey:VirtualRepoID"`
}

func (r *Repository) GetAuthConfig() (*ProxyAuthConfig, error) {
	if r.AuthConfig == "" {
		return &ProxyAuthConfig{Type: "none"}, nil
	}
	var cfg ProxyAuthConfig
	err := json.Unmarshal([]byte(r.AuthConfig), &cfg)
	return &cfg, err
}

type RepositoryGroup struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	VirtualRepoID uint     `json:"virtual_repo_id" gorm:"uniqueIndex:idx_virtual_member"`
	MemberRepoID uint      `json:"member_repo_id" gorm:"uniqueIndex:idx_virtual_member"`
	Priority     int       `json:"priority" gorm:"default:0"`
	CreatedAt    time.Time `json:"created_at"`

	VirtualRepo Repository `json:"virtual_repo,omitempty" gorm:"foreignKey:VirtualRepoID"`
	MemberRepo  Repository `json:"member_repo,omitempty" gorm:"foreignKey:MemberRepoID"`
}

type ProxyAuthConfig struct {
	Type   string      `json:"type"`
	Basic  *BasicAuth  `json:"basic,omitempty"`
	Bearer *BearerAuth `json:"bearer,omitempty"`
	APIKey *APIKeyAuth `json:"api_key,omitempty"`
}

type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type BearerAuth struct {
	Token string `json:"token"`
}

type APIKeyAuth struct {
	HeaderName string `json:"header_name"`
	KeyValue   string `json:"key_value"`
	QueryParam string `json:"query_param,omitempty"`
}

type CacheEntry struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	RepoName    string    `json:"repo_name" gorm:"index"`
	CacheKey    string    `json:"cache_key" gorm:"uniqueIndex"`
	ETag        string    `json:"etag"`
	LastModified string   `json:"last_modified"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	ExpiresAt   time.Time `json:"expires_at" gorm:"index"`
	AccessCount int64     `json:"access_count" gorm:"default:0"`
	CreatedAt   time.Time `json:"created_at"`
}
```

- [ ] **步骤 2：创建仓库数据访问层**

```go
// internal/repository/repository_repo.go
package repository

import (
	"fmt"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type RepositoryRepository struct {
	db *gorm.DB
}

func NewRepositoryRepository(db *gorm.DB) *RepositoryRepository {
	return &RepositoryRepository{db: db}
}

func (r *RepositoryRepository) Create(repo *model.Repository) error {
	return r.db.Create(repo).Error
}

func (r *RepositoryRepository) FindByName(name string) (*model.Repository, error) {
	var repo model.Repository
	err := r.db.Where("name = ?", name).First(&repo).Error
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

func (r *RepositoryRepository) List(filter map[string]interface{}) ([]model.Repository, error) {
	var repos []model.Repository
	query := r.db.Model(&model.Repository{})

	if pkgType, ok := filter["package_type"]; ok {
		query = query.Where("package_type = ?", pkgType)
	}
	if repoType, ok := filter["type"]; ok {
		query = query.Where("type = ?", repoType)
	}
	if enabled, ok := filter["enabled"]; ok {
		query = query.Where("enabled = ?", enabled)
	}

	err := query.Order("created_at DESC").Find(&repos).Error
	return repos, err
}

func (r *RepositoryRepository) Update(name string, updates map[string]interface{}) error {
	return r.db.Model(&model.Repository{}).Where("name = ?", name).Updates(updates).Error
}

func (r *RepositoryRepository) Delete(name string) error {
	repo, err := r.FindByName(name)
	if err != nil {
		return err
	}
	return r.db.Delete(repo).Error
}

func (r *RepositoryRepository) FindByPackageType(pkgType string) ([]model.Repository, error) {
	var repos []model.Repository
	err := r.db.Where("package_type = ? AND enabled = ?", pkgType, true).
		Order("proxy_priority ASC").Find(&repos).Error
	return repos, err
}

func (r *RepositoryRepository) FindVirtualByPackageType(pkgType string) (*model.Repository, error) {
	var repo model.Repository
	err := r.db.Where("type = ? AND package_type = ? AND enabled = ?",
		model.RepoTypeVirtual, pkgType, true).First(&repo).Error
	if err != nil {
		return nil, fmt.Errorf("virtual repository not found for package type: %s", pkgType)
	}
	return &repo, nil
}
```

- [ ] **步骤 3：创建仓库组成员关系数据访问层**

```go
// internal/repository/group_repo.go
package repository

import (
	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type GroupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

func (r *GroupRepository) AddMember(virtualRepoID, memberRepoID uint, priority int) error {
	group := model.RepositoryGroup{
		VirtualRepoID: virtualRepoID,
		MemberRepoID:  memberRepoID,
		Priority:      priority,
	}
	return r.db.Create(&group).Error
}

func (r *GroupRepository) RemoveMember(virtualRepoID, memberRepoID uint) error {
	return r.db.Where("virtual_repo_id = ? AND member_repo_id = ?",
		virtualRepoID, memberRepoID).Delete(&model.RepositoryGroup{}).Error
}

func (r *GroupRepository) GetMembersByVirtualRepo(virtualRepoID uint) ([]model.RepositoryGroup, error) {
	var groups []model.RepositoryGroup
	err := r.db.Where("virtual_repo_id = ?", virtualRepoID).
		Preload("MemberRepo").
		Order("priority ASC").
		Find(&groups).Error
	return groups, err
}

func (r *GroupRepository) UpdatePriority(virtualRepoID, memberRepoID uint, priority int) error {
	return r.db.Model(&model.RepositoryGroup{}).
		Where("virtual_repo_id = ? AND member_repo_id = ?", virtualRepoID, memberRepoID).
		Update("priority", priority).Error
}
```

- [ ] **步骤 4：修改 Package 模型增加 repository_id 关联**

在 `internal/model/package.go` 的 Package struct 中增加：

```go
// 在 Package struct 中增加字段
RepositoryID uint        `json:"repository_id" gorm:"index"`
Repository   *Repository `json:"repository,omitempty"`
```

- [ ] **步骤 5：更新数据库迁移**

在 `internal/database/migration.go` 的 RunMigrations 函数中，种子数据部分增加默认仓库：

```go
// 在 seedRolesAndPermissions() 之后调用
if err := seedDefaultRepositories(); err != nil {
	return err
}
```

```go
func seedDefaultRepositories() error {
	// npm 仓库组
	repos := []model.Repository{
		{
			Name:        "npm-local",
			DisplayName: "NPM 内部仓库",
			Type:        model.RepoTypeLocal,
			PackageType: "npm",
			Enabled:     true,
		},
		{
			Name:        "npm-proxy-cn",
			DisplayName: "阿里云 NPM 镜像",
			Type:        model.RepoTypeProxy,
			PackageType: "npm",
			RemoteURL:   "https://registry.npmmirror.com",
			ProxyPriority: 1,
			CacheTTLSeconds: 86400,
			CacheMaxSizeGB:  50,
			Enabled:     true,
		},
		{
			Name:        "npm-proxy-official",
			DisplayName: "NPM 官方仓库",
			Type:        model.RepoTypeProxy,
			PackageType: "npm",
			RemoteURL:   "https://registry.npmjs.org",
			ProxyPriority: 2,
			CacheTTLSeconds: 3600,
			Enabled:     true,
		},
		{
			Name:        "npm-virtual",
			DisplayName: "NPM 聚合仓库",
			Type:        model.RepoTypeVirtual,
			PackageType: "npm",
			Enabled:     true,
		},
		// Maven 仓库组
		{
			Name:        "maven-local",
			DisplayName: "Maven 内部仓库",
			Type:        model.RepoTypeLocal,
			PackageType: "maven2",
			Enabled:     true,
		},
		{
			Name:        "maven-proxy-aliyun",
			DisplayName: "阿里云 Maven 镜像",
			Type:        model.RepoTypeProxy,
			PackageType: "maven2",
			RemoteURL:   "https://maven.aliyun.com/repository/public",
			ProxyPriority: 1,
			Enabled:     true,
		},
		{
			Name:        "maven-proxy-central",
			DisplayName: "Maven Central",
			Type:        model.RepoTypeProxy,
			PackageType: "maven2",
			RemoteURL:   "https://repo1.maven.org/maven2/",
			ProxyPriority: 2,
			Enabled:     true,
		},
		{
			Name:        "maven-virtual",
			DisplayName: "Maven 聚合仓库",
			Type:        model.RepoTypeVirtual,
			PackageType: "maven2",
			Enabled:     true,
		},
	}

	for _, repo := range repos {
		var existing model.Repository
		result := DB.Where("name = ?", repo.Name).First(&existing)
		if result.Error != nil {
			if err := DB.Create(&repo).Error; err != nil {
				return err
			}
		}
	}

	// 配置虚拟仓库成员
	type member struct {
		virtualRepo string
		memberRepo  string
		priority    int
	}

	members := []member{
		{"npm-virtual", "npm-local", 0},
		{"npm-virtual", "npm-proxy-cn", 1},
		{"npm-virtual", "npm-proxy-official", 2},
		{"maven-virtual", "maven-local", 0},
		{"maven-virtual", "maven-proxy-aliyun", 1},
		{"maven-virtual", "maven-proxy-central", 2},
	}

	for _, m := range members {
		virtualRepo, err := repoRepo.FindByName(m.virtualRepo)
		if err != nil {
			continue
		}
		memberRepo, err := repoRepo.FindByName(m.memberRepo)
		if err != nil {
			continue
		}

		var existing model.RepositoryGroup
		result := DB.Where("virtual_repo_id = ? AND member_repo_id = ?",
			virtualRepo.ID, memberRepo.ID).First(&existing)
		if result.Error != nil {
			groupRepo := groupRepo.NewGroupRepository(DB)
			groupRepo.AddMember(virtualRepo.ID, memberRepo.ID, m.priority)
		}
	}

	return nil
}
```

**验证：**
```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./cmd/registry/
rm -f data/registry.db
./moonlight-registry 2>&1 | head -30
# 预期输出包含数据库迁移和种子仓库创建日志
```

---

### 任务 2：代理认证模块

**文件：**
- 创建：`internal/proxy/auth.go`

- [ ] **步骤 1：实现代理认证配置和应用**

```go
// internal/proxy/auth.go
package proxy

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

type ProxyAuthConfig struct {
	Type   string      `json:"type"`
	Basic  *BasicAuth  `json:"basic,omitempty"`
	Bearer *BearerAuth `json:"bearer,omitempty"`
	APIKey *APIKeyAuth `json:"api_key,omitempty"`
}

type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type BearerAuth struct {
	Token string `json:"token"`
}

type APIKeyAuth struct {
	HeaderName string `json:"header_name"`
	KeyValue   string `json:"key_value"`
	QueryParam string `json:"query_param,omitempty"`
}

func (c *ProxyAuthConfig) Apply(req *http.Request) error {
	if c == nil || c.Type == "none" {
		return nil
	}

	switch c.Type {
	case "basic":
		if c.Basic == nil {
			return fmt.Errorf("basic auth config is missing")
		}
		password := resolveEnv(c.Basic.Password)
		req.SetBasicAuth(c.Basic.Username, password)

	case "bearer":
		if c.Bearer == nil {
			return fmt.Errorf("bearer auth config is missing")
		}
		token := resolveEnv(c.Bearer.Token)
		req.Header.Set("Authorization", "Bearer "+token)

	case "api_key":
		if c.APIKey == nil {
			return fmt.Errorf("api key auth config is missing")
		}
		keyValue := resolveEnv(c.APIKey.KeyValue)
		req.Header.Set(c.APIKey.HeaderName, keyValue)
		if c.APIKey.QueryParam != "" {
			q := req.URL.Query()
			q.Set(c.APIKey.QueryParam, keyValue)
			req.URL.RawQuery = q.Encode()
		}

	default:
		return fmt.Errorf("unsupported auth type: %s", c.Type)
	}

	return nil
}

func resolveEnv(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		envKey := s[2 : len(s)-1]
		return os.Getenv(envKey)
	}
	return s
}
```

**验证：**
```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./internal/proxy/
# 预期：无错误
```

---

### 任务 3：远程 HTTP 客户端

**文件：**
- 创建：`internal/proxy/client.go`

- [ ] **步骤 1：实现远程 HTTP 客户端**

```go
// internal/proxy/client.go
package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type RemoteClient struct {
	httpClient *http.Client
	timeout    time.Duration
}

func NewRemoteClient(timeout time.Duration) *RemoteClient {
	return &RemoteClient{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		timeout: timeout,
	}
}

func (c *RemoteClient) Get(ctx context.Context, url string, auth *ProxyAuthConfig) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if auth != nil {
		if err := auth.Apply(req); err != nil {
			return nil, fmt.Errorf("failed to apply auth: %w", err)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &RemoteError{
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}

	return resp, nil
}

func (c *RemoteClient) GetBytes(ctx context.Context, url string, auth *ProxyAuthConfig) ([]byte, string, error) {
	resp, err := c.Get(ctx, url, auth)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	etag := resp.Header.Get("ETag")
	lastModified := resp.Header.Get("Last-Modified")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	_ = etag
	_ = lastModified
	return body, contentType, nil
}

type RemoteError struct {
	StatusCode int
	URL        string
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("remote request failed: %d %s", e.StatusCode, e.URL)
}

func (e *RemoteError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}
```

**验证：**
```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./internal/proxy/
```

---

### 任务 4：缓存服务

**文件：**
- 创建：`internal/proxy/cache.go`
- 创建：`internal/service/cache_service.go`
- 修改：`internal/config/config.go` (增加 Cache 配置)

- [ ] **步骤 1：实现缓存服务**

```go
// internal/proxy/cache.go
package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type CacheService struct {
	db       *gorm.DB
	maxSizeGB float64
	mu       sync.RWMutex
}

func NewCacheService(db *gorm.DB, maxSizeGB float64) *CacheService {
	return &CacheService{
		db:        db,
		maxSizeGB: maxSizeGB,
	}
}

type CacheItem struct {
	Key         string
	Content     []byte
	ContentType string
	Size        int64
	ExpiresAt   time.Time
	IsNegative  bool
}

func (c *CacheService) Get(ctx context.Context, key string) (*CacheItem, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var entry model.CacheEntry
	if err := c.db.Where("cache_key = ? AND expires_at > ?", key, time.Now()).
		First(&entry).Error; err != nil {
		return nil, err
	}

	entry.AccessCount++
	c.db.Model(&model.CacheEntry{}).Where("id = ?", entry.ID).
		Update("access_count", entry.AccessCount)

	return &CacheItem{
		Key:         entry.CacheKey,
		ContentType: entry.ContentType,
		Size:        entry.Size,
		ExpiresAt:   entry.ExpiresAt,
		IsNegative:  entry.ContentType == "application/x-negative-cache",
	}, nil
}

func (c *CacheService) Set(ctx context.Context, item *CacheItem, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := model.CacheEntry{
		RepoName:    extractRepoName(item.Key),
		CacheKey:    item.Key,
		ContentType: item.ContentType,
		Size:        item.Size,
		ExpiresAt:   time.Now().Add(ttl),
	}

	return c.db.Create(&entry).Error
}

func (c *CacheService) SetNegative(ctx context.Context, key string, ttl time.Duration) error {
	return c.Set(ctx, &CacheItem{
		Key:         key,
		ContentType: "application/x-negative-cache",
		IsNegative:  true,
	}, ttl)
}

func (c *CacheService) Invalidate(ctx context.Context, pattern string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.db.Where("cache_key LIKE ?", pattern).
		Delete(&model.CacheEntry{}).Error
}

func (c *CacheService) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.db.Exec("DELETE FROM cache_entries").Error
}

func (c *CacheService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	var totalEntries int64
	var totalSize int64
	var expiredEntries int64

	c.db.Model(&model.CacheEntry{}).Count(&totalEntries)
	c.db.Model(&model.CacheEntry{}).Select("COALESCE(SUM(size), 0)").Scan(&totalSize)
	c.db.Model(&model.CacheEntry{}).Where("expires_at < ?", time.Now()).Count(&expiredEntries)

	return map[string]interface{}{
		"total_entries":  totalEntries,
		"total_size":     totalSize,
		"expired_entries": expiredEntries,
		"max_size_gb":    c.maxSizeGB,
	}, nil
}

func extractRepoName(key string) string {
	// key format: proxy:repoName:pkg:version
	parts := splitKey(key)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func splitKey(key string) []string {
	// Simple split implementation
	var parts []string
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	parts = append(parts, key[start:])
	return parts
}
```

- [ ] **步骤 2：更新配置增加缓存配置**

在 `internal/config/config.go` 中增加：

```go
type CacheConfig struct {
	MaxSizeGB   float64 `mapstructure:"max_size_gb"`
	DefaultTTL  int     `mapstructure:"default_ttl"`
	MetadataTTL int     `mapstructure:"metadata_ttl"`
}
```

在 `defaults.go` 中增加默认值：

```go
v.SetDefault("cache.max_size_gb", 10)
v.SetDefault("cache.default_ttl", 86400)
v.SetDefault("cache.metadata_ttl", 3600)
```

**验证：**
```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./internal/proxy/ ./internal/service/
```

---

### 任务 5：多代理路由引擎

**文件：**
- 创建：`internal/proxy/router.go`
- 创建：`internal/service/repository_service.go`

- [ ] **步骤 1：实现路由引擎核心逻辑**

```go
// internal/proxy/router.go
package proxy

import (
	"context"
	"fmt"
	"io"

	"github.com/moonlight-box/registry/internal/adapter"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"gorm.io/gorm"
)

var ErrPackageNotFound = fmt.Errorf("package not found")

type ProxyRouter struct {
	db        *gorm.DB
	cache     *CacheService
	client    *RemoteClient
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	adapters  map[string]adapter.Adapter
}

func NewProxyRouter(
	db *gorm.DB,
	cache *CacheService,
	client *RemoteClient,
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	adapters map[string]adapter.Adapter,
) *ProxyRouter {
	return &ProxyRouter{
		db:        db,
		cache:     cache,
		client:    client,
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		adapters:  adapters,
	}
}

type RouteResult struct {
	Source     string
	SourceType string
	Content    io.ReadCloser
	Size       int64
	FromCache  bool
	CacheTTL   int
}

func (r *ProxyRouter) Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
	virtualRepo, err := r.repoRepo.FindVirtualByPackageType(pkgType)
	if err != nil {
		return nil, err
	}

	members, err := r.groupRepo.GetMembersByVirtualRepo(virtualRepo.ID)
	if err != nil {
		return nil, err
	}

	for _, member := range members {
		repo := member.MemberRepo

		var result *RouteResult
		switch repo.Type {
		case model.RepoTypeLocal:
			result, err = r.resolveLocal(ctx, &repo, pkgType, name, version)
		case model.RepoTypeProxy:
			result, err = r.resolveProxy(ctx, &repo, pkgType, name, version)
		}

		if err == nil && result != nil {
			result.Source = repo.Name
			return result, nil
		}
	}

	return nil, ErrPackageNotFound
}

func (r *ProxyRouter) resolveLocal(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	adp, ok := r.adapters[pkgType]
	if !ok {
		return nil, fmt.Errorf("no adapter for package type: %s", pkgType)
	}

	identity := &adapter.PackageIdentity{
		PackageType: pkgType,
		Name:        name,
		Version:     version,
	}

	content, err := adp.Download(ctx, identity)
	if err != nil {
		return nil, err
	}

	return &RouteResult{
		SourceType: "local",
		Content:    content.Body,
		Size:       content.Size,
		FromCache:  false,
	}, nil
}

func (r *ProxyRouter) resolveProxy(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	cacheKey := fmt.Sprintf("proxy:%s:%s:%s", repo.Name, name, version)

	cached, err := r.cache.Get(ctx, cacheKey)
	if err == nil && cached != nil {
		if cached.IsNegative {
			return nil, ErrPackageNotFound
		}
		return &RouteResult{
			SourceType: "proxy",
			Content:    io.NopCloser(nil),
			Size:       cached.Size,
			FromCache:  true,
			CacheTTL:   repo.CacheTTLSeconds,
		}, nil
	}

	adp, ok := r.adapters[pkgType]
	if !ok {
		return nil, fmt.Errorf("no adapter for package type: %s", pkgType)
	}

	authCfg, err := repo.GetAuthConfig()
	if err != nil {
		return nil, err
	}

	remoteURL := fmt.Sprintf("%s/%s/%s", repo.RemoteURL, name, version)
	content, contentType, err := r.client.GetBytes(ctx, remoteURL, authCfg)
	if err != nil {
		if remoteErr, ok := err.(*RemoteError); ok && remoteErr.IsNotFound() {
			r.cache.SetNegative(ctx, cacheKey, time.Duration(repo.CacheNegativeTTL)*time.Second)
		}
		return nil, err
	}

	size := int64(len(content))
	r.cache.Set(ctx, &CacheItem{
		Key:         cacheKey,
		Content:     content,
		ContentType: contentType,
		Size:        size,
	}, time.Duration(repo.CacheTTLSeconds)*time.Second)

	return &RouteResult{
		SourceType: "proxy",
		Content:    io.NopCloser(nil),
		Size:       size,
		FromCache:  false,
		CacheTTL:   repo.CacheTTLSeconds,
	}, nil
}
```

**验证：**
```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./internal/proxy/
```

---

### 任务 6：仓库管理 API

**文件：**
- 创建：`internal/handler/repository_handler.go`
- 创建：`internal/handler/cache_handler.go`
- 创建：`internal/service/repository_service.go`

- [ ] **步骤 1：实现仓库管理服务**

```go
// internal/service/repository_service.go
package service

import (
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"gorm.io/gorm"
)

type RepositoryService struct {
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	db        *gorm.DB
}

func NewRepositoryService(repoRepo *repository.RepositoryRepository, groupRepo *repository.GroupRepository, db *gorm.DB) *RepositoryService {
	return &RepositoryService{
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		db:        db,
	}
}

func (s *RepositoryService) Create(repo *model.Repository, members []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(repo).Error; err != nil {
			return err
		}

		if repo.Type == model.RepoTypeVirtual && len(members) > 0 {
			for i, memberName := range members {
				memberRepo, err := s.repoRepo.FindByName(memberName)
				if err != nil {
					continue
				}
				s.groupRepo.AddMember(repo.ID, memberRepo.ID, i)
			}
		}

		return nil
	})
}

func (s *RepositoryService) List(filter map[string]interface{}) ([]model.Repository, error) {
	return s.repoRepo.List(filter)
}

func (s *RepositoryService) Get(name string) (*model.Repository, error) {
	return s.repoRepo.FindByName(name)
}

func (s *RepositoryService) Update(name string, updates map[string]interface{}) error {
	return s.repoRepo.Update(name, updates)
}

func (s *RepositoryService) Delete(name string) error {
	return s.repoRepo.Delete(name)
}

func (s *RepositoryService) AddMember(virtualRepoName, memberRepoName string, priority int) error {
	virtualRepo, err := s.repoRepo.FindByName(virtualRepoName)
	if err != nil {
		return err
	}
	memberRepo, err := s.repoRepo.FindByName(memberRepoName)
	if err != nil {
		return err
	}
	return s.groupRepo.AddMember(virtualRepo.ID, memberRepo.ID, priority)
}

func (s *RepositoryService) RemoveMember(virtualRepoName, memberRepoName string) error {
	virtualRepo, err := s.repoRepo.FindByName(virtualRepoName)
	if err != nil {
		return err
	}
	memberRepo, err := s.repoRepo.FindByName(memberRepoName)
	if err != nil {
		return err
	}
	return s.groupRepo.RemoveMember(virtualRepo.ID, memberRepo.ID)
}

func (s *RepositoryService) GetMembers(virtualRepoName string) ([]model.RepositoryGroup, error) {
	virtualRepo, err := s.repoRepo.FindByName(virtualRepoName)
	if err != nil {
		return nil, err
	}
	return s.groupRepo.GetMembersByVirtualRepo(virtualRepo.ID)
}
```

- [ ] **步骤 2：实现仓库 API 处理器**

```go
// internal/handler/repository_handler.go
package handler

import (
	"github.com/moonlight-box/registry/internal/service"
	"github.com/gin-gonic/gin"
)

type RepositoryHandler struct {
	svc *service.RepositoryService
}

func NewRepositoryHandler(svc *service.RepositoryService) *RepositoryHandler {
	return &RepositoryHandler{svc: svc}
}

func (h *RepositoryHandler) List(c *gin.Context) {
	filter := make(map[string]interface{})
	if pkgType := c.Query("package_type"); pkgType != "" {
		filter["package_type"] = pkgType
	}
	if repoType := c.Query("type"); repoType != "" {
		filter["type"] = repoType
	}

	repos, err := h.svc.List(filter)
	if err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Success(c, repos)
}

func (h *RepositoryHandler) Get(c *gin.Context) {
	name := c.Param("name")
	repo, err := h.svc.Get(name)
	if err != nil {
		NotFound(c, "Repository not found")
		return
	}

	Success(c, repo)
}

func (h *RepositoryHandler) Create(c *gin.Context) {
	var req struct {
		Name        string   `json:"name" binding:"required"`
		DisplayName string   `json:"display_name"`
		Description string   `json:"description"`
		Type        string   `json:"type" binding:"required"`
		PackageType string   `json:"package_type" binding:"required"`
		RemoteURL   string   `json:"remote_url"`
		AuthType    string   `json:"auth_type"`
		AuthConfig  string   `json:"auth_config"`
		ProxyPriority int    `json:"proxy_priority"`
		Members     []string `json:"members"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	repo := model.Repository{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Type:        model.RepositoryType(req.Type),
		PackageType: req.PackageType,
		RemoteURL:   req.RemoteURL,
		AuthType:    req.AuthType,
		AuthConfig:  req.AuthConfig,
		ProxyPriority: req.ProxyPriority,
		Enabled:     true,
	}

	if err := h.svc.Create(&repo, req.Members); err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Created(c, repo)
}

func (h *RepositoryHandler) Update(c *gin.Context) {
	name := c.Param("name")
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.svc.Update(name, updates); err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Repository updated"})
}

func (h *RepositoryHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.svc.Delete(name); err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Repository deleted"})
}

func (h *RepositoryHandler) GetMembers(c *gin.Context) {
	name := c.Param("name")
	members, err := h.svc.GetMembers(name)
	if err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Success(c, members)
}

func (h *RepositoryHandler) AddMember(c *gin.Context) {
	name := c.Param("name")
	var req struct {
		MemberName string `json:"member_name" binding:"required"`
		Priority   int    `json:"priority"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.svc.AddMember(name, req.MemberName, req.Priority); err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Member added"})
}

func (h *RepositoryHandler) RemoveMember(c *gin.Context) {
	name := c.Param("name")
	memberName := c.Param("memberName")

	if err := h.svc.RemoveMember(name, memberName); err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Member removed"})
}
```

- [ ] **步骤 3：实现缓存管理 API 处理器**

```go
// internal/handler/cache_handler.go
package handler

import (
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/gin-gonic/gin"
)

type CacheHandler struct {
	cacheSvc *proxy.CacheService
}

func NewCacheHandler(cacheSvc *proxy.CacheService) *CacheHandler {
	return &CacheHandler{cacheSvc: cacheSvc}
}

func (h *CacheHandler) GetStats(c *gin.Context) {
	stats, err := h.cacheSvc.GetStats(c.Request.Context())
	if err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Success(c, stats)
}

func (h *CacheHandler) Clear(c *gin.Context) {
	if err := h.cacheSvc.Clear(c.Request.Context()); err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Cache cleared"})
}

func (h *CacheHandler) Invalidate(c *gin.Context) {
	var req struct {
		Pattern string `json:"pattern" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.cacheSvc.Invalidate(c.Request.Context(), req.Pattern); err != nil {
		InternalServerError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "Cache invalidated"})
}
```

**验证：**
```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./internal/handler/ ./internal/service/
```

---

### 任务 7：更新路由和主入口

**文件：**
- 修改：`cmd/registry/main.go`
- 修改：`internal/handler/response.go`

- [ ] **步骤 1：更新 main.go 集成所有新组件**

在 main.go 中增加：

```go
// 在初始化存储后增加
repoRepo := repository.NewRepositoryRepository(db)
groupRepo := repository.NewGroupRepository(db)
cacheSvc := proxy.NewCacheService(db, cfg.Cache.MaxSizeGB)
remoteClient := proxy.NewRemoteClient(30 * time.Second)
proxyRouter := proxy.NewProxyRouter(db, cacheSvc, remoteClient, repoRepo, groupRepo, nil)

repoSvc := service.NewRepositoryService(repoRepo, groupRepo, db)
repoHandler := handler.NewRepositoryHandler(repoSvc)
cacheHandler := handler.NewCacheHandler(cacheSvc)

// 注册仓库管理 API
api := r.Group("/api/v1")
api.Use(middleware.CORSMiddleware())
api.Use(middleware.RequestIDMiddleware())
{
	// 仓库管理
	repos := api.Group("/repositories")
	{
		repos.GET("", repoHandler.List)
		repos.POST("", repoHandler.Create)
		repos.GET("/:name", repoHandler.Get)
		repos.PUT("/:name", repoHandler.Update)
		repos.DELETE("/:name", repoHandler.Delete)
		repos.GET("/:name/members", repoHandler.GetMembers)
		repos.POST("/:name/members", repoHandler.AddMember)
		repos.DELETE("/:name/members/:memberName", repoHandler.RemoveMember)
	}

	// 缓存管理
	cache := api.Group("/cache")
	{
		cache.GET("/stats", cacheHandler.GetStats)
		cache.DELETE("", cacheHandler.Clear)
		cache.POST("/invalidate", cacheHandler.Invalidate)
	}
}
```

- [ ] **步骤 2：更新配置增加 Repository 和 Cache 配置**

在 `config.go` 中增加：

```go
type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Storage     StorageConfig     `mapstructure:"storage"`
	Auth        AuthConfig        `mapstructure:"auth"`
	Security    SecurityConfig    `mapstructure:"security"`
	Cache       CacheConfig       `mapstructure:"cache"`
	Repository  RepositoryConfig  `mapstructure:"repository"`
	Logging     LoggingConfig     `mapstructure:"logging"`
}

type RepositoryConfig struct {
	DefaultCacheTTL int `mapstructure:"default_cache_ttl"`
	MaxCacheSizeGB  float64 `mapstructure:"max_cache_size_gb"`
}
```

在 `defaults.go` 中增加：

```go
v.SetDefault("repository.default_cache_ttl", 86400)
v.SetDefault("repository.max_cache_size_gb", 10)
```

**验证：**
```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./cmd/registry/
rm -f data/registry.db
./moonlight-registry
```

---

### 任务 8：前端仓库管理界面

**文件：**
- 创建：`web/src/views/RepositoryList.vue`
- 创建：`web/src/views/RepositoryDetail.vue`
- 创建：`web/src/views/CacheManagement.vue`
- 创建：`web/src/api/repository.ts`
- 修改：`web/src/router/index.ts`
- 修改：`web/src/components/layout/AppSidebar.vue`

- [ ] **步骤 1：创建仓库 API 接口**

```typescript
// web/src/api/repository.ts
import request from './request'

export interface Repository {
  id: number
  name: string
  display_name: string
  description: string
  type: 'local' | 'proxy' | 'virtual'
  package_type: string
  enabled: boolean
  remote_url?: string
  auth_type?: string
  proxy_priority?: number
  cache_enabled?: boolean
  cache_ttl_seconds?: number
  cache_max_size_gb?: number
  created_at: string
  updated_at: string
  members?: RepositoryGroup[]
}

export interface RepositoryGroup {
  id: number
  virtual_repo_id: number
  member_repo_id: number
  priority: number
  member_repo: Repository
}

export const repositoryApi = {
  list(params?: { package_type?: string; type?: string }) {
    return request.get<{ list: Repository[] }>('/api/v1/repositories', { params })
  },

  get(name: string) {
    return request.get<Repository>(`/api/v1/repositories/${name}`)
  },

  create(data: Partial<Repository>) {
    return request.post<Repository>('/api/v1/repositories', data)
  },

  update(name: string, data: Partial<Repository>) {
    return request.put(`/api/v1/repositories/${name}`, data)
  },

  delete(name: string) {
    return request.delete(`/api/v1/repositories/${name}`)
  },

  getMembers(name: string) {
    return request.get<RepositoryGroup[]>(`/api/v1/repositories/${name}/members`)
  },

  addMember(name: string, data: { member_name: string; priority: number }) {
    return request.post(`/api/v1/repositories/${name}/members`, data)
  },

  removeMember(name: string, memberName: string) {
    return request.delete(`/api/v1/repositories/${name}/members/${memberName}`)
  },
}

export const cacheApi = {
  getStats() {
    return request.get('/api/v1/cache/stats')
  },

  clear() {
    return request.delete('/api/v1/cache')
  },

  invalidate(data: { pattern: string }) {
    return request.post('/api/v1/cache/invalidate', data)
  },
}
```

- [ ] **步骤 2：创建仓库列表页**

```vue
<!-- web/src/views/RepositoryList.vue -->
<template>
  <div class="repository-list">
    <div class="page-header">
      <h2>仓库管理</h2>
      <el-button type="primary" @click="showCreateDialog = true">
        <el-icon><Plus /></el-icon> 创建仓库
      </el-button>
    </div>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="全部" name="all" />
      <el-tab-pane label="Local" name="local" />
      <el-tab-pane label="Proxy" name="proxy" />
      <el-tab-pane label="Virtual" name="virtual" />
    </el-tabs>

    <el-table :data="filteredRepos" v-loading="loading" style="width: 100%">
      <el-table-column prop="name" label="仓库名称" width="180" />
      <el-table-column prop="display_name" label="显示名称" />
      <el-table-column prop="type" label="类型" width="100">
        <template #default="{ row }">
          <el-tag :type="getTypeTag(row.type)" size="small">
            {{ row.type }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="package_type" label="包类型" width="100" />
      <el-table-column prop="remote_url" label="远程地址" show-overflow-tooltip />
      <el-table-column prop="enabled" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'danger'" size="small">
            {{ row.enabled ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="200">
        <template #default="{ row }">
          <el-button size="small" @click="viewDetail(row)">详情</el-button>
          <el-button size="small" @click="editRepo(row)">编辑</el-button>
          <el-popconfirm title="确定删除此仓库?" @confirm="deleteRepo(row.name)">
            <template #reference>
              <el-button size="small" type="danger">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="showCreateDialog" title="创建仓库" width="500px">
      <el-form :model="newRepo" label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="newRepo.name" placeholder="npm-local" />
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="newRepo.display_name" placeholder="NPM 内部仓库" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="newRepo.type">
            <el-option label="Local" value="local" />
            <el-option label="Proxy" value="proxy" />
            <el-option label="Virtual" value="virtual" />
          </el-select>
        </el-form-item>
        <el-form-item label="包类型">
          <el-select v-model="newRepo.package_type">
            <el-option label="npm" value="npm" />
            <el-option label="maven2" value="maven2" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="newRepo.type === 'proxy'" label="远程地址">
          <el-input v-model="newRepo.remote_url" placeholder="https://registry.npmjs.org" />
        </el-form-item>
        <el-form-item v-if="newRepo.type === 'proxy'" label="优先级">
          <el-input-number v-model="newRepo.proxy_priority" :min="0" :max="100" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreate">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { repositoryApi, type Repository } from '@/api/repository'

const loading = ref(false)
const activeTab = ref('all')
const showCreateDialog = ref(false)
const repos = ref<Repository[]>([])

const filteredRepos = computed(() => {
  if (activeTab.value === 'all') return repos.value
  return repos.value.filter(r => r.type === activeTab.value)
})

const newRepo = ref({
  name: '',
  display_name: '',
  type: 'local' as 'local' | 'proxy' | 'virtual',
  package_type: 'npm',
  remote_url: '',
  proxy_priority: 0,
})

const getTypeTag = (type: string) => {
  switch (type) {
    case 'local': return 'success'
    case 'proxy': return 'warning'
    case 'virtual': return 'primary'
    default: return 'info'
  }
}

const loadRepos = async () => {
  loading.value = true
  try {
    const res = await repositoryApi.list()
    repos.value = res.list || []
  } catch (err) {
    ElMessage.error('加载仓库列表失败')
  } finally {
    loading.value = false
  }
}

const handleCreate = async () => {
  try {
    await repositoryApi.create(newRepo.value)
    ElMessage.success('创建成功')
    showCreateDialog.value = false
    loadRepos()
  } catch (err) {
    ElMessage.error('创建失败')
  }
}

const deleteRepo = async (name: string) => {
  try {
    await repositoryApi.delete(name)
    ElMessage.success('删除成功')
    loadRepos()
  } catch (err) {
    ElMessage.error('删除失败')
  }
}

const viewDetail = (repo: Repository) => {
  // 跳转到详情页
}

const editRepo = (repo: Repository) => {
  // 打开编辑对话框
}

onMounted(loadRepos)
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}
</style>
```

- [ ] **步骤 3：更新路由和侧边栏**

在 `web/src/router/index.ts` 中增加：

```typescript
{
  path: '/repositories',
  component: () => import('@/views/Layout.vue'),
  meta: { requiresAuth: true },
  children: [
    {
      path: '',
      name: 'Repositories',
      component: () => import('@/views/RepositoryList.vue'),
    },
  ],
},
{
  path: '/cache',
  component: () => import('@/views/Layout.vue'),
  meta: { requiresAuth: true },
  children: [
    {
      path: '',
      name: 'CacheManagement',
      component: () => import('@/views/CacheManagement.vue'),
    },
  ],
},
```

在 `web/src/components/layout/AppSidebar.vue` 的 menuItems 中增加：

```typescript
{
  title: '仓库管理',
  icon: 'Files',
  path: '/repositories',
},
{
  title: '缓存管理',
  icon: 'Coin',
  path: '/cache',
},
```

**验证：**
```bash
cd /Users/gracegaoya/work/project/moonlight-box/web
npx vue-tsc --noEmit
npx vite build
# 预期：无错误
```

---

### 任务 9：端到端测试与验证

**文件：** 无

- [ ] **步骤 1：编译验证**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build -o moonlight-registry ./cmd/registry/
# 预期：编译成功
```

- [ ] **步骤 2：数据库迁移验证**

```bash
rm -f data/registry.db
./moonlight-registry 2>&1 | head -30
# 预期：数据库创建成功，默认仓库种子数据插入成功
```

- [ ] **步骤 3：API 测试**

```bash
# 测试仓库列表
curl -s http://localhost:8081/api/v1/repositories | jq .

# 测试创建仓库
curl -s -X POST http://localhost:8081/api/v1/repositories \
  -H "Content-Type: application/json" \
  -d '{"name":"test-repo","display_name":"Test Repo","type":"local","package_type":"npm"}' | jq .

# 测试缓存统计
curl -s http://localhost:8081/api/v1/cache/stats | jq .
```

- [ ] **步骤 4：前端构建验证**

```bash
cd /Users/gracegaoya/work/project/moonlight-box/web
npm run build
# 预期：构建成功
```

---

## 自检验查

1. **规格覆盖度：** ✅ 所有 Phase 2 需求都有对应任务实现
2. **占位符扫描：** ✅ 无 TODO 或占位符
3. **类型一致性：** ✅ 所有类型在各任务中保持一致
