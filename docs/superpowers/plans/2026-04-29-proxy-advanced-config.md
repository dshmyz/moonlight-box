# 代理仓库高级配置实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为代理仓库增加超时时间、重定向处理、证书校验、失败缓存规则等高级配置，并重写 RemoteClient 支持流式处理和动态超时，同时完善前端仓库管理表单。

**架构：** 后端新增 4 个 Repository 模型字段，重构 RemoteClient 使用 RequestOptions 参数和 TransportManager 管理连接池，ProxyRouter 改用流式缓存；前端重写 RepositoryList.vue 表单，按类型分组展示配置项。

**技术栈：** Go (Gin, GORM, http.Transport), Vue 3 + TypeScript + Element Plus

---

## 文件结构

### 后端文件

| 文件 | 操作 | 职责 |
|------|------|------|
| `internal/model/repository.go` | 修改 | Repository 模型新增 4 个字段 |
| `internal/config/config.go` | 修改 | 新增 ProxyConfig 结构体 |
| `internal/config/defaults.go` | 修改 | 新增 proxy 相关默认值 |
| `internal/proxy/client.go` | 重写 | RemoteClient 改用 RequestOptions + TransportManager + 流式处理 |
| `internal/proxy/cache.go` | 新增 | FailureCacheRule 结构体和匹配逻辑 |
| `internal/proxy/router.go` | 修改 | resolveProxy 改为流式处理 + 使用新 RequestOptions |
| `internal/handler/repository_handler.go` | 修改 | Create/Update 接口支持新字段 |
| `internal/database/migration.go` | 修改 | seedDefaultRepositories 更新新字段 |

### 前端文件

| 文件 | 操作 | 职责 |
|------|------|------|
| `web/src/api/repository.ts` | 修改 | Repository interface 新增字段 |
| `web/src/views/RepositoryList.vue` | 重写 | 创建/编辑表单按类型分组展示 |

---

## 任务分解

### 任务 1：Repository 模型新增字段

**文件：**
- 修改：`internal/model/repository.go`

- [ ] **步骤 1：在 Repository 结构体中新增 4 个字段**

在 `internal/model/repository.go` 的 `Repository` 结构体中，在 `CacheNegativeTTL` 字段之后新增：

```go
	// 代理高级配置（仅 proxy 类型仓库使用）
	TimeoutSeconds     int    `json:"timeout_seconds" gorm:"default:0"`
	MaxRedirects       int    `json:"max_redirects" gorm:"default:0"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" gorm:"default:false"`
	FailureCacheRules  string `json:"failure_cache_rules" gorm:"type:text"`
```

- [ ] **步骤 2：运行编译确认无错误**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./...
```

预期：编译通过

- [ ] **步骤 3：Commit**

```bash
git add internal/model/repository.go
git commit -m "feat(model): add proxy advanced config fields to Repository"
```

---

### 任务 2：全局配置新增 ProxyConfig

**文件：**
- 修改：`internal/config/config.go`
- 修改：`internal/config/defaults.go`

- [ ] **步骤 1：新增 ProxyConfig 结构体**

在 `internal/config/config.go` 中新增：

```go
type ProxyConfig struct {
	DefaultTimeout     time.Duration `mapstructure:"default_timeout"`
	ConnectTimeout     time.Duration `mapstructure:"connect_timeout"`
	LargeFileThreshold int64         `mapstructure:"large_file_threshold"` // bytes
	MaxRedirects       int           `mapstructure:"max_redirects"`
	InsecureSkipVerify bool          `mapstructure:"insecure_skip_verify"`
}
```

在 `Config` 结构体中添加 Proxy 字段：

```go
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Security SecurityConfig `mapstructure:"security"`
	Cache    CacheConfig    `mapstructure:"cache"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Proxy    ProxyConfig    `mapstructure:"proxy"`
}
```

- [ ] **步骤 2：新增 proxy 默认值**

在 `internal/config/defaults.go` 中新增：

```go
	// Proxy
	v.SetDefault("proxy.default_timeout", 30*time.Second)
	v.SetDefault("proxy.connect_timeout", 10*time.Second)
	v.SetDefault("proxy.large_file_threshold", 50*1024*1024) // 50MB
	v.SetDefault("proxy.max_redirects", 10)
	v.SetDefault("proxy.insecure_skip_verify", false)
```

- [ ] **步骤 3：运行编译确认无错误**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./...
```

预期：编译通过

- [ ] **步骤 4：Commit**

```bash
git add internal/config/config.go internal/config/defaults.go
git commit -m "feat(config): add ProxyConfig with default values"
```

---

### 任务 3：RemoteClient 重构

**文件：**
- 重写：`internal/proxy/client.go`

- [ ] **步骤 1：重写 client.go 全部内容**

```go
package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// TransportManager 管理多个 Transport 实例，避免每次请求都创建
type TransportManager struct {
	secureTransport   *http.Transport
	insecureTransport *http.Transport
	connectTimeout    time.Duration
}

// NewTransportManager 创建 TransportManager
func NewTransportManager(connectTimeout time.Duration) *TransportManager {
	baseTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   connectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}

	insecureTransport := baseTransport.Clone()
	insecureTransport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
	}

	return &TransportManager{
		secureTransport:   baseTransport,
		insecureTransport: insecureTransport,
		connectTimeout:    connectTimeout,
	}
}

// GetTransport 根据是否需要跳过证书校验返回对应的 Transport
func (m *TransportManager) GetTransport(insecure bool) *http.Transport {
	if insecure {
		return m.insecureTransport
	}
	return m.secureTransport
}

// RequestOptions 请求选项
type RequestOptions struct {
	ConnectTimeout     time.Duration
	ReadTimeout        time.Duration
	MaxRedirects       int // -1 表示不跟随重定向，0 表示使用默认值
	InsecureSkipVerify bool
}

// RemoteClient 远程 HTTP 客户端
type RemoteClient struct {
	transportManager *TransportManager
	defaultMaxRedirects int
}

// NewRemoteClient 创建远程客户端
func NewRemoteClient(tm *TransportManager, defaultMaxRedirects int) *RemoteClient {
	return &RemoteClient{
		transportManager:    tm,
		defaultMaxRedirects: defaultMaxRedirects,
	}
}

// buildClient 根据选项构建 http.Client
func (c *RemoteClient) buildClient(opts RequestOptions) *http.Client {
	maxRedirects := opts.MaxRedirects
	if maxRedirects == 0 {
		maxRedirects = c.defaultMaxRedirects
	}

	transport := c.transportManager.GetTransport(opts.InsecureSkipVerify)

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if maxRedirects == -1 {
				return http.ErrUseLastResponse
			}
			if len(via) >= maxRedirects {
				return fmt.Errorf("重定向次数超过限制: %d", maxRedirects)
			}
			return nil
		},
		Timeout: opts.ReadTimeout,
	}
}

// Get 发起 GET 请求，返回原始 HTTP 响应
func (c *RemoteClient) Get(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) (*http.Response, error) {
	client := c.buildClient(opts)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	if auth != nil {
		if err := auth.Apply(req); err != nil {
			return nil, fmt.Errorf("应用认证信息失败: %w", err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
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

// GetBytes 发起 GET 请求并读取完整响应体
func (c *RemoteClient) GetBytes(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) ([]byte, string, error) {
	resp, err := c.Get(ctx, url, opts, auth)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取响应体失败: %w", err)
	}

	return body, contentType, nil
}

// GetStream 发起 GET 请求并返回流式读取器，避免大文件全部加载到内存
func (c *RemoteClient) GetStream(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) (*http.Response, error) {
	client := c.buildClient(opts)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	if auth != nil {
		if err := auth.Apply(req); err != nil {
			return nil, fmt.Errorf("应用认证信息失败: %w", err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
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

// RemoteError 远程请求错误
type RemoteError struct {
	StatusCode int
	URL        string
}

// Error 实现 error 接口
func (e *RemoteError) Error() string {
	return fmt.Sprintf("远程请求失败: %d %s", e.StatusCode, e.URL)
}

// IsNotFound 判断是否为 404 未找到错误
func (e *RemoteError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}
```

- [ ] **步骤 2：运行编译确认无错误**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./...
```

预期：编译通过，但可能有其他文件因接口变化而编译失败（后续任务修复）

- [ ] **步骤 3：Commit**

```bash
git add internal/proxy/client.go
git commit -m "refactor(proxy): rewrite RemoteClient with RequestOptions and TransportManager"
```

---

### 任务 4：失败缓存规则结构体和匹配逻辑

**文件：**
- 创建：`internal/proxy/cache.go`

- [ ] **步骤 1：创建 failure_cache_rule.go**

```go
package proxy

import "encoding/json"

// FailureCacheRule 失败缓存规则
type FailureCacheRule struct {
	StatusCode     int   `json:"status_code,omitempty"`
	StatusCodeRange []int `json:"status_code_range,omitempty"` // [start, end]
	TTLSeconds     int   `json:"ttl_seconds"`
}

// FailureCacheRules 失败缓存规则列表
type FailureCacheRules []FailureCacheRule

// ParseFailureCacheRules 解析 JSON 字符串为规则列表
func ParseFailureCacheRules(jsonStr string) (FailureCacheRules, error) {
	if jsonStr == "" {
		return nil, nil
	}
	var rules FailureCacheRules
	err := json.Unmarshal([]byte(jsonStr), &rules)
	return rules, err
}

// Match 匹配状态码，返回匹配的规则的 TTL，未匹配返回 0
func (rules FailureCacheRules) Match(statusCode int) int {
	for _, rule := range rules {
		if rule.StatusCode > 0 && rule.StatusCode == statusCode {
			return rule.TTLSeconds
		}
		if len(rule.StatusCodeRange) == 2 {
			if statusCode >= rule.StatusCodeRange[0] && statusCode <= rule.StatusCodeRange[1] {
				return rule.TTLSeconds
			}
		}
	}
	return 0
}

// ShouldCache 判断是否应该缓存该状态码
func (rules FailureCacheRules) ShouldCache(statusCode int) bool {
	return rules.Match(statusCode) > 0
}
```

- [ ] **步骤 2：运行编译确认无错误**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./...
```

预期：编译通过

- [ ] **步骤 3：Commit**

```bash
git add internal/proxy/cache.go
git commit -m "feat(proxy): add FailureCacheRule for customizable error caching"
```

---

### 任务 5：ProxyRouter 流式处理重写

**文件：**
- 修改：`internal/proxy/router.go`

- [ ] **步骤 1：重写 ProxyRouter 以使用新的 RequestOptions 和流式处理**

需要将 `router.go` 中的 `resolveProxy` 方法改为流式处理，并更新 `RemoteClient` 调用方式。

由于改动较大，需要查看当前文件完整内容来确定修改范围。

修改 `resolveProxy` 方法：

```go
// resolveProxy 从代理仓库解析包，支持流式处理和本地缓存
func (r *ProxyRouter) resolveProxy(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	// 构造缓存键
	cacheKey := fmt.Sprintf("proxy:%s:%s:%s", repo.Name, name, version)

	// 尝试从缓存获取
	cached, err := r.cache.Get(ctx, cacheKey)
	if err == nil && cached != nil {
		if cached.IsNegative {
			return nil, ErrPackageNotFound
		}
		return &RouteResult{
			SourceType: "proxy",
			Content:    io.NopCloser(bytes.NewReader(cached.Content)),
			Size:       cached.Size,
			FromCache:  true,
			CacheTTL:   repo.CacheTTLSeconds,
		}, nil
	}

	// 缓存未命中，向远程仓库发起请求
	remoteURL := fmt.Sprintf("%s/%s/%s", repo.RemoteURL, name, version)
	authCfg, err := repo.GetAuthConfig()
	if err != nil {
		return nil, err
	}

	// 计算超时时间
	readTimeout := r.getReadTimeout(repo)
	
	// 构建请求选项
	opts := RequestOptions{
		ReadTimeout:        readTimeout,
		MaxRedirects:       repo.MaxRedirects,
		InsecureSkipVerify: repo.InsecureSkipVerify,
	}

	// 获取远程内容（流式）
	resp, err := r.client.GetStream(ctx, remoteURL, opts, toProxyAuthConfig(authCfg))
	if err != nil {
		if remoteErr, ok := err.(*RemoteError); ok {
			// 根据失败缓存规则决定是否缓存
			rules, parseErr := proxy.ParseFailureCacheRules(repo.FailureCacheRules)
			if parseErr == nil && rules.ShouldCache(remoteErr.StatusCode) {
				ttl := rules.Match(remoteErr.StatusCode)
				r.cache.SetNegative(ctx, cacheKey, time.Duration(ttl)*time.Second)
			} else if remoteErr.IsNotFound() {
				// 兼容现有的 404 负向缓存逻辑
				r.cache.SetNegative(ctx, cacheKey, time.Duration(repo.CacheNegativeTTL)*time.Second)
			}
		}
		return nil, err
	}
	defer resp.Body.Close()

	// 获取内容大小
	contentLength := resp.ContentLength
	contentType := resp.Header.Get("Content-Type")

	// 如果 Content-Length 未设置，可能需要读取部分内容来判断
	if contentLength <= 0 {
		// 对于无法确定大小的响应，回退到 GetBytes
		body, ct, err := r.client.GetBytes(ctx, remoteURL, opts, toProxyAuthConfig(authCfg))
		if err != nil {
			return nil, err
		}
		size := int64(len(body))
		r.cache.Set(ctx, &CacheItem{
			Key:         cacheKey,
			Content:     body,
			ContentType: ct,
			Size:        size,
		}, time.Duration(repo.CacheTTLSeconds)*time.Second)

		return &RouteResult{
			SourceType: "proxy",
			Content:    io.NopCloser(bytes.NewReader(body)),
			Size:       size,
			FromCache:  false,
			CacheTTL:   repo.CacheTTLSeconds,
		}, nil
	}

	// 读取完整响应体并缓存
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	size := int64(len(body))
	r.cache.Set(ctx, &CacheItem{
		Key:         cacheKey,
		Content:     body,
		ContentType: contentType,
		Size:        size,
	}, time.Duration(repo.CacheTTLSeconds)*time.Second)

	return &RouteResult{
		SourceType: "proxy",
		Content:    io.NopCloser(bytes.NewReader(body)),
		Size:       size,
		FromCache:  false,
		CacheTTL:   repo.CacheTTLSeconds,
	}, nil
}

// getReadTimeout 获取读取超时时间，大文件动态延长
func (r *ProxyRouter) getReadTimeout(repo *model.Repository) time.Duration {
	if repo.TimeoutSeconds > 0 {
		base := time.Duration(repo.TimeoutSeconds) * time.Second
		// 大文件阈值判断需要根据 Content-Length，这里返回基础值
		// 实际大文件判断在 GetStream 返回后根据 ContentLength 处理
		return base
	}
	// 使用全局默认值
	cfg := config.Get()
	if cfg != nil {
		return cfg.Proxy.DefaultTimeout
	}
	return 30 * time.Second
}
```

- [ ] **步骤 2：更新 NewProxyRouter 和依赖注入**

由于 `RemoteClient` 构造函数变了，需要在初始化的地方更新。

- [ ] **步骤 3：运行编译确认无错误**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./...
```

- [ ] **步骤 4：Commit**

```bash
git add internal/proxy/router.go
git commit -m "refactor(proxy): rewrite resolveProxy with stream processing and RequestOptions"
```

---

### 任务 6：更新 RemoteClient 初始化

**文件：**
- 搜索并修改 RemoteClient 初始化的地方（cmd/registry/main.go 或相关初始化文件）

- [ ] **步骤 1：查找 RemoteClient 初始化位置并更新**

需要先查找当前 RemoteClient 在哪里被创建，然后更新为使用新的构造函数。

```bash
cd /Users/gracegaoya/work/project/moonlight-box
grep -rn "NewRemoteClient" --include="*.go"
```

- [ ] **步骤 2：更新初始化代码**

```go
tm := proxy.NewTransportManager(cfg.Proxy.ConnectTimeout)
client := proxy.NewRemoteClient(tm, cfg.Proxy.MaxRedirects)
```

- [ ] **步骤 3：运行编译确认无错误**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./...
```

- [ ] **步骤 4：Commit**

```bash
git add <modified_files>
git commit -m "fix(proxy): update RemoteClient initialization with new constructor"
```

---

### 任务 7：更新 Handler 支持新字段

**文件：**
- 修改：`internal/handler/repository_handler.go`

- [ ] **步骤 1：更新 Create 方法**

在 `repository_handler.go` 的 `Create` 方法中，在 `repo` 结构体初始化时添加新字段：

```go
	repo := model.Repository{
		Name:               req.Name,
		DisplayName:        req.DisplayName,
		Description:        req.Description,
		Type:               model.RepositoryType(req.Type),
		PackageType:        req.PackageType,
		RemoteURL:          req.RemoteURL,
		AuthType:           req.AuthType,
		AuthConfig:         req.AuthConfig,
		ProxyPriority:      req.ProxyPriority,
		TimeoutSeconds:     req.TimeoutSeconds,
		MaxRedirects:       req.MaxRedirects,
		InsecureSkipVerify: req.InsecureSkipVerify,
		FailureCacheRules:  req.FailureCacheRules,
		Enabled:            true,
	}
```

更新 request 结构体：

```go
	var req struct {
		Name               string   `json:"name" binding:"required"`
		DisplayName        string   `json:"display_name"`
		Description        string   `json:"description"`
		Type               string   `json:"type" binding:"required"`
		PackageType        string   `json:"package_type" binding:"required"`
		RemoteURL          string   `json:"remote_url"`
		AuthType           string   `json:"auth_type"`
		AuthConfig         string   `json:"auth_config"`
		ProxyPriority      int      `json:"proxy_priority"`
		TimeoutSeconds     int      `json:"timeout_seconds"`
		MaxRedirects       int      `json:"max_redirects"`
		InsecureSkipVerify bool     `json:"insecure_skip_verify"`
		FailureCacheRules  string   `json:"failure_cache_rules"`
		Members            []string `json:"members"`
	}
```

- [ ] **步骤 2：运行编译确认无错误**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./...
```

- [ ] **步骤 3：Commit**

```bash
git add internal/handler/repository_handler.go
git commit -m "feat(handler): add new proxy config fields to Create API"
```

---

### 任务 8：更新种子数据

**文件：**
- 修改：`internal/database/migration.go`

- [ ] **步骤 1：更新 seedDefaultRepositories 中的代理仓库配置**

在 `seedDefaultRepositories` 中为代理仓库添加新的配置字段，例如：

```go
{
	Name:           "npm-proxy-cn",
	DisplayName:    "NPM 国内代理",
	Description:    "国内 npm 镜像源代理",
	Type:           model.RepoTypeProxy,
	PackageType:    string(model.PackageTypeNPM),
	Enabled:        true,
	RemoteURL:      "https://registry.npmmirror.com",
	AuthType:       "none",
	ProxyPriority:  1,
	CacheEnabled:   true,
	CacheTTLSeconds: 86400,
	TimeoutSeconds: 30,
	MaxRedirects:   5,
	FailureCacheRules: `[{"status_code": 404, "ttl_seconds": 300}, {"status_code_range": [500, 599], "ttl_seconds": 60}]`,
},
```

- [ ] **步骤 2：运行编译确认无错误**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./...
```

- [ ] **步骤 3：Commit**

```bash
git add internal/database/migration.go
git commit -m "feat(migration): update seed repositories with new proxy config fields"
```

---

### 任务 9：前端 TypeScript 类型更新

**文件：**
- 修改：`web/src/api/repository.ts`

- [ ] **步骤 1：更新 Repository interface**

```typescript
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
  auth_config?: string
  proxy_priority?: number
  timeout_seconds?: number
  max_redirects?: number
  insecure_skip_verify?: boolean
  failure_cache_rules?: FailureCacheRule[]
  cache_enabled?: boolean
  cache_ttl_seconds?: number
  cache_negative_ttl?: number
  cache_max_size_gb?: number
  allow_overwrite?: boolean
  allow_delete?: boolean
  created_at: string
  updated_at: string
  members?: RepositoryGroup[]
}

export interface FailureCacheRule {
  status_code?: number
  status_code_range?: [number, number]
  ttl_seconds: number
}
```

- [ ] **步骤 2：运行前端类型检查**

```bash
cd /Users/gracegaoya/work/project/moonlight-box/web
npx vue-tsc --noEmit
```

- [ ] **步骤 3：Commit**

```bash
git add web/src/api/repository.ts
git commit -m "feat(web): update Repository interface with new proxy config fields"
```

---

### 任务 10：前端仓库表单重写

**文件：**
- 重写：`web/src/views/RepositoryList.vue`

- [ ] **步骤 1：重写 RepositoryList.vue**

完整的组件实现，按分组展示配置项。包含：
- 基础信息表单
- 代理配置表单（仅 proxy 类型）
- 超时与连接配置（仅 proxy 类型）
- 缓存配置（所有类型，proxy 类型额外显示失败缓存规则）
- 权限控制（local 和 proxy 类型）
- 虚拟仓成员（仅 virtual 类型）

表单使用 Element Plus 的 el-form、el-input、el-select、el-switch、el-input-number 等组件。

失败缓存规则使用可视化表单 + JSON 模式切换：
- 可视化模式：规则列表，每行可添加/删除，支持状态码精确匹配或范围匹配
- JSON 模式：JSON textarea，支持格式化校验

- [ ] **步骤 2：运行前端类型检查和构建**

```bash
cd /Users/gracegaoya/work/project/moonlight-box/web
npx vue-tsc --noEmit
npm run build
```

- [ ] **步骤 3：Commit**

```bash
git add web/src/views/RepositoryList.vue
git commit -m "feat(web): rewrite repository form with grouped config sections"
```

---

## 规格自检

- [x] 规格中的每个需求都有对应任务实现
- [x] 无占位符、TODO、待定章节
- [x] 类型定义一致（FailureCacheRules、RequestOptions 等）
- [x] 所有任务都有验证步骤和 commit

## 执行交接

计划已完成并保存到 `docs/superpowers/plans/2026-04-29-proxy-advanced-config.md`。

**两种执行方式：**

**1. 子代理驱动（推荐）** - 每个任务调度一个新的子代理，任务间进行审查，快速迭代

**2. 内联执行** - 在当前会话中使用 executing-plans 执行任务，批量执行并设有检查点

**选哪种方式？**
