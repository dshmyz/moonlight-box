# Moonlight Registry - 企业级开源组件仓库设计文档

> **日期**: 2026-04-28
> **状态**: ✅ 已批准
> **版本**: v1.0

---

## 1. 项目概述

### 1.1 目标

构建一个**企业内部私有化的开源组件仓库**，支持多种编程语言的依赖包管理，提供：
- 从远程仓库拉取并缓存包（代理模式）
- 发布和管理私有包（本地仓库）
- 可视化 Web 管理后台
- 自动安全扫描与风险阻断
- 完整的审计日志与权限控制

### 1.2 核心价值

- **统一管理**: 一个系统管理所有语言的依赖包
- **加速构建**: 本地缓存减少网络延迟
- **安全合规**: 自动漏洞扫描 + 风险阻断
- **简单部署**: 单一二进制文件，零外部依赖（默认）
- **完全可控**: 数据完全在企业内部，无隐私风险

### 1.3 支持的包管理器

| # | 包管理器 | 路由前缀 | 用途 | 文件格式 |
|---|---------|---------|------|---------|
| 1 | npm | `/npm` | JavaScript/TypeScript | `.tgz`, `package.json` |
| 2 | Maven | `/maven2` | Java/Kotlin | `.jar`, `.pom` |
| 3 | PyPI | `/pypi` | Python | `.whl`, `.tar.gz` |
| 4 | Go modules | `/go` | Go | `.zip`, `.mod` |
| 5 | NuGet | `/nuget` | .NET/C# | `.nupkg` |
| 6 | YUM/DNF | `/yum` | RHEL/CentOS/Rocky/AlmaLinux | `.rpm` |
| 7 | APT | `/apt` | Debian/Ubuntu | `.deb` |
| 8 | Generic | `/files` | 任意文件 | 任意 |

---

## 2. 架构设计

### 2.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Moonlight Registry                           │
│                      (Go 单体应用 + Vue 前端)                         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    API Gateway Layer                         │    │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────────┐  │    │
│  │  │ /npm/    │ │ /maven/  │ │ /pypi/   │ │ /go/, /yum/, ..│  │    │
│  │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └───────┬────────┘  │    │
│  └────────┼────────────┼────────────┼──────────────┼───────────┘    │
│           │            │            │              │                │
│  ┌────────▼────────────▼────────────▼──────────────▼───────────┐    │
│  │                  Core Engine (Go)                            │    │
│  │                                                             │    │
│  │  ┌─────────────────────────────────────────────────────┐   │    │
│  │  │              Protocol Adapters (8种)                  │   │    │
│  │  │  NPM | Maven | PyPI | Go | NuGet | YUM | APT | Generic│   │    │
│  │  └─────────────────────────────────────────────────────┘   │    │
│  │                                                             │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────┐   │    │
│  │  │ Package      │  │ Storage      │  │ Proxy & Cache  │   │    │
│  │  │ Manager      │  │ Service      │  │ Service        │   │    │
│  │  └──────────────┘  └──────────────┘  └────────────────┘   │    │
│  │                                                             │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────┐   │    │
│  │  │ Auth & RBAC  │  │ Security     │  │ Event & Audit  │   │    │
│  │  │ Service      │  │ Scanner      │  │ Log Service    │   │    │
│  │  └──────────────┘  └──────────────┘  └────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                     Data Layer                               │    │
│  │  ┌─────────────┐  ┌─────────────────────────────────────┐  │    │
│  │  │ PostgreSQL  │  │ Storage Backend                      │  │    │
│  │  │ 或 SQLite   │  │ (本地文件/S3/OSS)                     │  │    │
│  │  │ (可切换)     │  │                                      │  │    │
│  │  └─────────────┘  └─────────────────────────────────────┘  │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                   Web Admin Frontend                          │    │
│  │                 (Vue 3 + TypeScript)                          │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 设计原则

1. **协议适配器模式**: 每种包管理器是独立 adapter，实现统一接口，方便扩展新语言
2. **存储抽象层**: 通过 `StorageBackend` 接口屏蔽底层存储差异（本地/S3）
3. **代理缓存透明化**: 客户端无感知，自动从远程拉取并缓存到本地
4. **安全左移**: 包上传时即触发安全扫描，阻断风险包进入仓库
5. **最小依赖**: 默认仅依赖 SQLite + 本地文件，可选升级 PG + S3

### 2.3 技术选型

#### 后端 (Go)

| 组件 | 技术 | 说明 |
|------|------|------|
| HTTP 框架 | Gin | 高性能、生态好、中间件丰富 |
| ORM | GORM | 功能完善、迁移工具成熟、支持 SQLite/PG |
| 数据库 | **SQLite** (默认) / PostgreSQL (可选) | 零配置起步 / 高并发生产环境 |
| 缓存 | BigCache / sync.Map | Go 内存缓存，零外部依赖 |
| 存储接口 | 自定义 `io.Reader/Writer` 抽象 | 支持本地文件和 S3 协议 |
| 认证 | golang-jwt/jwt5 | 纯 Go JWT 实现 |
| 密码哈希 | x/crypto/bcrypt | 标准库密码哈希 |
| 配置 | viper | 支持 YAML/ENV/命令行 |
| 日志 | zap | 结构化高性能日志 |
| CLI | cobra | 命令行框架 |

#### 前端 (Vue 3)

| 组件 | 技术 | 说明 |
|------|------|------|
| 框架 | Vue 3 + Composition API | 响应式、组合式 API |
| 构建 | Vite | 快速开发体验 |
| 语言 | TypeScript | 类型安全 |
| UI 库 | Element Plus 或 Ant Design Vue | 企业级组件库 |
| 状态管理 | Pinia | Vue 3 官方推荐 |
| HTTP | Axios | HTTP 客户端 |
| 路由 | Vue Router 4 | 路由管理 |
| 图表 | ECharts | 数据可视化 |

---

## 3. 核心模块设计

### 3.1 协议适配器层 (Protocol Adapters)

#### 统一接口定义

```go
type PackageType string

const (
    PackageTypeNPM     PackageType = "npm"
    PackageTypeMaven   PackageType = "maven"
    PackageTypePyPI    PackageType = "pypi"
    PackageTypeGo      PackageType = "go"
    PackageTypeNuGet   PackageType = "nuget"
    PackageTypeYum     PackageType = "yum"
    PackageTypeApt     PackageType = "apt"
    PackageTypeGeneric PackageType = "generic"
)

type PackageIdentity struct {
    Name    string
    Version string
    Type    PackageType
}

type UploadRequest struct {
    Package     io.Reader
    Filename    string
    Size        int64
    Checksum    string
    Metadata    map[string]interface{}
    UploadedBy  uint
}

type PackageContent struct {
    Content     io.ReadCloser
    ContentType string
    Size        int64
    Checksum    string
}

type PackageMeta struct {
    ID          uint
    Name        string
    Type        PackageType
    Description string
    Versions    []PackageVersionInfo
}

type PackageVersionInfo struct {
    Version     string
    PublishedAt time.Time
    Size        int64
    DownloadCount int64
}

// 统一适配器接口
type PackageAdapter interface {
    // 包管理器类型
    Type() PackageType
    
    // 路由前缀
    RoutePrefix() string
    
    // 注册路由到 Gin Router
    RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc)
    
    // 解析包名和版本
    ParsePackagePath(path string) (*PackageIdentity, error)
    
    // 上传/发布包
    Upload(ctx context.Context, req *UploadRequest) (*PackageVersion, error)
    
    // 下载包
    Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error)
    
    // 获取包元数据
    GetMetadata(ctx context.Context, name string) (*PackageMeta, error)
    
    // 删除包
    Delete(ctx context.Context, identity *PackageIdentity) error
    
    // 获取所有版本列表
    ListVersions(ctx context.Context, name string) ([]string, error)
}
```

#### 各适配器实现要点

##### npm Adapter
- 兼容 npm registry REST API 规范
- 处理 `package.json` 解析与验证
- tarball 上传/下载
- scope 包支持 (`@scope/package`)
- 支持 `npm publish` / `npm install` 命令

##### Maven Adapter
- 实现 Maven 2 仓库协议
- 生成/解析 `maven-metadata.xml`
- SNAPSHOT 版本支持
- POM 文件处理
- 分类目录结构: `groupId/artifactId/version/`

##### PyPI Adapter
- 实现 PEP 503 (Simple API) 规范
- 可选实现 PEP 691 (JSON API)
- wheel (.whl) 和 source distribution (.tar.gz) 支持
- 兼容 `pip install` / `twine upload`

##### Go Modules Adapter
- 实现 GOPROXY 协议规范
- 处理 `@v/list`, `.info`, `.mod`, `.zip` 四个端点
- 支持模块路径查询
- 最小版本选择兼容

##### NuGet Adapter
- 实现 NuGet Server v3 (V3) API
- OData 查询支持
- `.nupkg` 和 `.snupkg` (符号包) 处理
- 兼容 `dotnet push` / `nuget push`

##### YUM/DNF Adapter
- RPM 仓库元数据生成 (`repodata/`)
- 支持 `repomd.xml`, `primary.xml.gz`, `filelists.xml.gz`
- 多架构支持 (x86_64, aarch64, noarch)
- GPG 签名支持 (可选)
- 兼容 `yum install` / `dnf install`

##### APT Adapter
- DEB 仓库元数据生成
- `Packages`, `Release`, `Release.gpg` 文件
- 多发行版/架构支持
- 兼容 `apt-get install`

##### Generic/Raw Adapter
- 纯文件存储与下载
- 目录浏览功能
- 版本化目录结构
- 任意文件类型支持

---

### 3.2 存储服务层 (Storage Service)

#### 存储后端接口

```go
type StorageBackend interface {
    // 名称
    Name() string
    
    // 初始化
    Init(config map[string]interface{}) error
    
    // 存储文件
    Put(ctx context.Context, key string, reader io.Reader, size int64) error
    
    // 获取文件
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    
    // 删除文件
    Delete(ctx context.Context, key string) error
    
    // 检查是否存在
    Exists(ctx context.Context, key string) (bool, error)
    
    // 获取文件大小
    Size(ctx context.Context, key string) (int64, error)
    
    // 列出目录内容
    List(ctx context.Context, prefix string) ([]StorageEntry, error)
    
    // 生成临时访问 URL (用于 S3 直传)
    PresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
    
    // 关闭资源
    Close() error
}

type StorageEntry struct {
    Key  string
    IsDir bool
    Size int64
}
```

#### 内置实现

##### LocalStorage (本地文件系统)

```go
type LocalStorage struct {
    basePath string           // 根目录路径
    maxFileSize int64         // 单文件最大限制
}

// 特性:
// - 使用 os 标准库实现
// - 自动创建目录结构
// - 符号链接支持 (用于 latest 版本)
// - 磁盘空间检查
```

##### S3Storage (S3 兼容对象存储)

```go
type S3Storage struct {
    client     *s3.Client
    bucket     string
    region     string
    endpoint   string          // 支持 MinIO/Aliyun OSS 等
}

// 特性:
// - AWS SDK for Go v2
// - 兼容所有 S3 协议存储
// - 分片上传大文件支持
// - Pre-signed URL 生成
```

#### 存储路径规范

```
data/
├── packages/
│   ├── npm/
│   │   └── @scope/
│   │       └── package-name/
│   │           └── 1.0.0/
│   │               ├── package.tgz
│   │               └── package.json
│   ├── maven2/
│   │   └── com/example/
│   │       └── my-lib/
│   │           └── 1.0.0/
│   │               ├── my-lib-1.0.0.jar
│   │               ├── my-lib-1.0.0.pom
│   │               └── maven-metadata.xml
│   ├── pypi/
│   │   └── example-package/
│   │       └── example_package-1.0.0-py3-none-any.whl
│   ├── go/
│   │   └── github.com/user/repo/
│   │       └── @v/
│   │           ├── list
│   │           ├── v1.0.0.info
│   │           ├── v1.0.0.mod
│   │           └── v1.0.0.zip
│   ├── nuget/
│   │   └── package/
│   │       └── 1.0.0/
│   │           └── package.1.0.0.nupkg
│   ├── yum/
│   │   └── repos/
│   │       └── repo-name/
│   │           ├── repodata/
│   │           └── Packages/
│   ├── apt/
│   │   └── pool/
│   │       └── main/
│   │           └── p/pkgname/
│   │               └── pkgname_1.0.0_amd64.deb
│   └── generic/
│       └── tools/
│           └── mytool/
│               └── v1.0.0/
│                   └── mytool-binary
│
├── cache/                       # 远程代理缓存
│   ├── npm-registry/
│   ├── maven-central/
│   └── pypi-org/
│
├── registry.db                 # SQLite 数据库 (或连接 PG)
│
└── security/
    ├── vuln-database.json      # 本地漏洞数据库
    └── scan-reports/           # 扫描报告
```

---

### 3.3 代理缓存服务 (Proxy & Cache Service)

#### 服务设计

```go
type ProxyService struct {
    localStore    StorageBackend
    remoteClient  *http.Client
    cachePolicy   CachePolicy
    cacheStore    CacheStore       // 缓存元数据存储
}

type CachePolicy struct {
    DefaultTTL         time.Duration  // 默认缓存时间 (24h)
    MetadataTTL        time.Duration  // 元数据刷新间隔 (1h)
    MaxSizeBytes       int64          // 最大缓存大小 (100GB)
    EvictionPolicy     string         // LRU / FIFO / TTL
    MaxConcurrentFetch int            // 最大并发拉取数 (10)
}

type CacheEntry struct {
    RemoteURL      string
    LocalKey       string
    ETag           string
    LastModified   string
    ContentType    string
    CachedAt       time.Time
    ExpiresAt      time.Time
    AccessCount    int64
    SizeBytes      int64
}
```

#### 工作流程

```
客户端请求 → [检查本地缓存]
                │
                ├─ [命中] → 检查 TTL
                │              │
                │              ├─ [未过期] → 直接返回 ✅
                │              │
                │              └─ [已过期] → 返回旧版本 + 异步刷新 ⚠️
                │
                └─ [未命中] → 远程拉取 (带条件请求 If-None-Match)
                                  │
                                  ├─ [304 Not Modified] → 更新 TTL ✅
                                  │
                                  └─ [200 OK] → 存入本地 → 返回给客户端 ✅
                                               (流式传输，不阻塞)
```

#### 远程仓库配置

```yaml
proxy:
  enabled: true
  
  remotes:
    npm:
      url: "https://registry.npmjs.org"
      enabled: true
      timeout: 30s
      
    maven-central:
      url: "https://repo.maven.apache.org/maven2"
      enabled: true
      timeout: 60s
      
    maven-google:
      url: "https://maven.google.com"
      enabled: true
      
    maven-spring:
      url: "https://repo.spring.io/release"
      enabled: false
      
    pypi:
      url: "https://pypi.org/simple"
      enabled: true
      timeout: 30s
      
    go-proxy:
      url: "https://proxy.golang.org"
      enabled: true
      timeout: 60s
      
    nuget-org:
      url: "https://api.nuget.org/v3/index.json"
      enabled: false
      
  cache:
    default_ttl: 24h
    metadata_ttl: 1h
    max_size_gb: 100
    eviction_policy: lru
```

---

### 3.4 安全扫描引擎 (Security Scanner)

#### 扫描引擎设计

```go
type SecurityScanner struct {
    vulnDatabase   *VulnDatabase
    scannerPool    chan *scannerWorker
    scanConfig     ScanConfig
}

type VulnDatabase struct {
    sync.RWMutex
    version        string           // 数据库版本
    lastUpdated    time.Time
    cveList        map[string]*CVE  // CVE ID -> 详情
    packageIndex   map[string][]string  // 包名 -> CVE 列表
}

type CVE struct {
    ID             string
    Severity       string  // critical, high, medium, low, none
    CVSSScore      float64
    Title          string
    Description    string
    AffectedRanges []AffectedRange
    References     []string
    PublishedAt    time.Time
}

type AffectedRange struct {
    PackageName    string
    PackageType    PackageType
    VersionConstraint string  // semver 表达式
    FixedVersion   string
}

type ScanResult struct {
    PackageID      uint
    PackageName    string
    Version        string
    PackageType    PackageType
    Status         ScanStatus
    Vulnerabilities []*VulnFinding
    Summary        ScanSummary
    ScannedAt      time.Time
    ScannerVersion string
}

type VulnFinding struct {
    CVE            *CVE
    DependencyName string
    CurrentVersion string
    FixedVersion   string
    IsDirectDep    bool
}

type ScanSummary struct {
    Total          int
    CriticalCount  int
    HighCount      int
    MediumCount    int
    LowCount       int
    NoneCount      int
}

type ScanStatus string

const (
    ScanStatusPending   ScanStatus = "pending"
    ScanStatusScanning  ScanStatus = "scanning"
    ScanStatusCompleted ScanStatus = "completed"
    ScanStatusFailed    ScanStatus = "failed"
)
```

#### 扫描流程

```
包上传 → [解析依赖]
            │
            ↓
    [遍历依赖树]
            │
            ├─ 对每个依赖:
            │      ├─ 查询漏洞数据库
            │      ├─ 匹配版本范围
            │      └─ 收集漏洞信息
            │
            ↓
    [汇总结果]
            │
            ├─ 生成扫描报告
            ├─ 写入数据库
            │
            ↓
    [应用阻断策略]
            │
            ├─ 有 critical/high?
            │      ├─ block_policy.critical == true → ❌ 阻断上传
            │      └─ block_policy.high == true → ❌ 阻断上传
            │
            └─ 仅 medium/low?
                   ├─ 记录警告 ⚠️
                   └─ 允许通过 ✅
```

#### 安全配置

```yaml
security:
  enabled: true
  
  # 扫描策略
  scan_on_upload: true          # 上传时自动扫描
  scan_schedule: "0 2 * * *"    # 定时全量扫描 (每天凌晨2点)
  
  # 阻断策略
  block_policy:
    critical: true              # 阻断严重漏洞
    high: true                  # 阻断高危漏洞
    medium: false               # 中危仅警告
    low: false                  # 低危仅记录
  
  # 白名单 (豁免扫描的包)
  allowlist:
    patterns:
      - "@internal/*"           # 内部作用域包
      - "company-*"             # 公司前缀包
    packages:
      - "specific-package"      # 特定包名
  
  # 漏洞数据库
  vulnerability_database:
    auto_update: true           # 自动更新
    update_interval: 24h        # 更新间隔
    sources:
      - type: github_advisory
        enabled: true
      - type: osv
        enabled: true
      - type: nvd
        enabled: true
        api_key: ""             # NVD API Key (提高速率限制)
```

---

### 3.5 认证与权限服务 (Auth & RBAC)

#### 用户模型

```go
type User struct {
    ID            uint           `gorm:"primaryKey"`
    Username      string         `gorm:"uniqueIndex;size:50;not null"`
    PasswordHash  string         `gorm:"size:255;not null"`  // bcrypt
    Email         string         `gorm:"uniqueIndex;size:255"`
    DisplayName   string         `gorm:"size:100"`
    AvatarURL     string         `gorm:"size:500"`
    IsActive      bool           `gorm:"default:true"`
    Roles         []Role         `gorm:"many2many:user_roles;"`
    LastLoginAt   *time.Time
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type Role struct {
    ID          uint       `gorm:"primaryKey"`
    Name        string     `gorm:"uniqueIndex;size:50;not null"`
    Description string     `gorm:"size:255"`
    IsSystemRole bool      `gorm:"default:false"`  // 内置角色不可删除
    Permissions []Permission `gorm:"many2role_permissions;"`
    Users       []User     `gorm:"many2many:user_roles;"`
    CreatedAt   time.Time
}

type Permission struct {
    ID       uint   `gorm:"primaryKey"`
    Resource string `gorm:"size:100;not null"`  // 资源: npm:*, maven:read
    Action   string `gorm:"size:20;not null"`   // 操作: read, write, delete, admin
    Roles    []Role `gorm:"many2many:role_permissions;"`
}

// 预置角色
var SystemRoles = []Role{
    {
        Name:        "admin",
        Description: "系统管理员，拥有所有权限",
        IsSystemRole: true,
    },
    {
        Name:        "developer",
        Description: "开发者，可发布和管理包",
        IsSystemRole: true,
    },
    {
        Name:        "readonly",
        Description: "只读用户，仅可下载包",
        IsSystemRole: true,
    },
}

// 预置权限
var SystemPermissions = []Permission{
    // 全局权限
    {Resource: "system", Action: "admin"},
    {Resource: "users", Action: "read"},
    {Resource: "users", Action: "write"},
    {Resource: "audit", Action: "read"},
    
    // npm 权限
    {Resource: "npm", Action: "read"},
    {Resource: "npm", Action: "write"},
    {Resource: "npm", Action: "delete"},
    {Resource: "npm", Action: "admin"},
    
    // maven 权限
    {Resource: "maven", Action: "read"},
    {Resource: "maven", Action: "write"},
    {Resource: "maven", Action: "delete"},
    {Resource: "maven", Action: "admin"},
    
    // ... 其他语言类似
}
```

#### JWT 认证流程

```go
type AuthService struct {
    jwtSecret      []byte
    tokenExpiry    time.Duration  // 默认 24h
    refreshExpiry  time.Duration  // 默认 7d
    tokenBlacklist *TokenBlacklist
}

type TokenClaims struct {
    UserID   uint   `json:"uid"`
    Username string `json:"uname"`
    Roles    []string `json:"roles"`
    jwt.RegisteredClaims
}

type TokenBlacklist struct {
    store Store  // Redis/内存/DB
}

// 登录
func (a *AuthService) Login(username, password string) (*AuthResponse, error) {
    // 1. 验证用户名密码
    user, err := a.ValidateCredentials(username, password)
    if err != nil {
        return nil, ErrInvalidCredentials
    }
    
    // 2. 生成 Access Token + Refresh Token
    accessToken, err := a.GenerateToken(user)
    refreshToken, err := a.GenerateRefreshToken(user)
    
    // 3. 更新最后登录时间
    a.UpdateLastLogin(user.ID)
    
    return &AuthResponse{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
        ExpiresIn:    a.tokenExpiry.Seconds(),
        User:         user.ToDTO(),
    }, nil
}

// 中间件: JWT 验证
func (a *AuthService) AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := a.ExtractToken(c)
        
        claims, err := a.ValidateToken(token)
        if err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
            return
        }
        
        // 检查黑名单
        if a.tokenBlacklist.IsBlacklisted(token) {
            c.AbortWithStatusJSON(401, gin.H{"error": "token revoked"})
            return
        }
        
        // 设置用户信息到上下文
        c.Set("userID", claims.UserID)
        c.Set("username", claims.Username)
        c.Set("roles", claims.Roles)
        
        c.Next()
    }
}
```

#### RBAC 权限检查中间件

```go
func RequirePermission(resource, action string) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetUint("userID")
        roles := c.GetStringSlice("roles")
        
        // 检查是否有权限
        hasPermission, err := authService.CheckPermission(userID, resource, action)
        if err != nil || !hasPermission {
            c.AbortWithStatusJSON(403, gin.H{
                "error": "insufficient permissions",
                "required": fmt.Sprintf("%s:%s", resource, action),
            })
            return
        }
        
        c.Next()
    }
}

// 使用示例
packages.GET("", auth.AuthMiddleware(), RequirePermission("npm", "read"), listHandler)
packages.POST("", auth.AuthMiddleware(), RequirePermission("npm", "write"), uploadHandler)
```

---

## 4. 数据模型设计

### 4.1 ER 关系图

```
users ──< user_roles >── roles ──< role_permissions >── permissions
  │                                                    │
  │ created_by                                         │ resource:action
  │                                                    │
  ▼                                                    │
packages <── package_versions ──┐                      │
  │                             │                      │
  │ package_id                  │ version_id           │
  │                             ▼                      │
  │                      scan_results                  │
  │                                                      │
  ├──────────────────────────────────────────────────────┘
  │
  ├─< audit_logs (user_id)
  │
  └─< cache_entries (独立)
```

### 4.2 核心表 DDL

#### packages 表

```sql
CREATE TABLE IF NOT EXISTS packages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN (
        'npm', 'maven', 'pypi', 'go', 'nuget', 
        'yum', 'apt', 'generic'
    )),
    description TEXT DEFAULT '',
    repository_type TEXT DEFAULT('local') CHECK(repository_type IN ('local', 'proxy', 'virtual')),
    homepage TEXT DEFAULT '',
    license TEXT DEFAULT '',
    created_by INTEGER REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(name, type)
);

CREATE INDEX idx_packages_type ON packages(type);
CREATE INDEX idx_packages_repo_type ON packages(repository_type);
CREATE INDEX idx_packages_created_by ON packages(created_by);
CREATE INDEX idx_packages_name ON packages(name);
```

#### package_versions 表

```sql
CREATE TABLE IF NOT EXISTS package_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    package_id INTEGER NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    version TEXT NOT NULL,
    status TEXT DEFAULT('published') CHECK(status IN (
        'draft', 'published', 'deprecated', 'yanked'
    )),
    storage_path TEXT NOT NULL,
    size_bytes BIGINT DEFAULT 0,
    checksum_sha256 TEXT,
    checksum_md5 TEXT,
    published_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    published_by INTEGER REFERENCES users(id),
    metadata JSON DEFAULT '{}',
    download_count INTEGER DEFAULT 0,
    last_downloaded_at DATETIME,
    
    UNIQUE(package_id, version)
);

CREATE INDEX idx_pv_status ON package_versions(status);
CREATE INDEX idx_pv_published ON package_versions(published_at);
CREATE INDEX idx_pv_download_count ON package_versions(download_count DESC);
```

#### users 表

```sql
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email TEXT UNIQUE,
    display_name TEXT DEFAULT '',
    avatar_url TEXT DEFAULT '',
    is_active BOOLEAN DEFAULT TRUE,
    last_login_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

#### roles 表

```sql
CREATE TABLE IF NOT EXISTS roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    is_system_role BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 插入预置角色
INSERT OR IGNORE INTO roles (name, description, is_system_role) VALUES
    ('admin', '系统管理员，拥有所有权限', 1),
    ('developer', '开发者，可发布和管理包', 1),
    ('readonly', '只读用户，仅可下载包', 1);
```

#### permissions 表

```sql
CREATE TABLE IF NOT EXISTS permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    resource TEXT NOT NULL,
    action TEXT NOT NULL CHECK(action IN ('read', 'write', 'delete', 'admin')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(resource, action)
);
```

#### user_roles 关联表

```sql
CREATE TABLE IF NOT EXISTS user_roles (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    assigned_by INTEGER REFERENCES users(id),
    
    PRIMARY KEY(user_id, role_id)
);
```

#### role_permissions 关联表

```sql
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id INTEGER NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    
    PRIMARY KEY(role_id, permission_id)
);
```

#### package_dependencies 表

```sql
CREATE TABLE IF NOT EXISTS package_dependencies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version_id INTEGER NOT NULL REFERENCES package_versions(id) ON DELETE CASCADE,
    dep_name TEXT NOT NULL,
    dep_version_constraint TEXT NOT NULL,
    dep_type TEXT NOT NULL,  -- runtime, dev, optional, peer
    package_type TEXT NOT NULL,
    is_optional BOOLEAN DEFAULT FALSE,
    
    UNIQUE(version_id, dep_name, dep_type)
);

CREATE INDEX idx_pd_version ON package_dependencies(version_id);
CREATE INDEX idx_pd_dep_name ON package_dependencies(dep_name);
```

#### scan_results 表

```sql
CREATE TABLE IF NOT EXISTS scan_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version_id INTEGER NOT NULL UNIQUE REFERENCES package_versions(id),
    scan_status TEXT DEFAULT('pending') CHECK(scan_status IN (
        'pending', 'scanning', 'completed', 'failed'
    )),
    scanner_version TEXT,
    total_vulnerabilities INTEGER DEFAULT 0,
    critical_count INTEGER DEFAULT 0,
    high_count INTEGER DEFAULT 0,
    medium_count INTEGER DEFAULT 0,
    low_count INTEGER DEFAULT 0,
    scanned_at DATETIME,
    report_path TEXT,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

#### scan_vulnerabilities 表 (详细漏洞记录)

```sql
CREATE TABLE IF NOT EXISTS scan_vulnerabilities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_result_id INTEGER NOT NULL REFERENCES scan_results(id) ON DELETE CASCADE,
    cve_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    cvss_score REAL,
    dependency_name TEXT NOT NULL,
    current_version TEXT NOT NULL,
    fixed_version TEXT,
    is_direct_dep BOOLEAN DEFAULT FALSE,
    title TEXT,
    description TEXT,
    references JSON DEFAULT '[]'
);

CREATE INDEX idx_sv_scan_result ON scan_vulnerabilities(scan_result_id);
CREATE INDEX idx_sv_cve ON scan_vulnerabilities(cve_id);
CREATE INDEX idx_sv_severity ON scan_vulnerabilities(severity);
```

#### audit_logs 表

```sql
CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER REFERENCES users(id),
    action TEXT NOT NULL CHECK(action IN (
        'login', 'logout',
        'package_upload', 'package_download', 'package_delete',
        'package_publish', 'package_deprecate',
        'user_create', 'user_update', 'user_delete',
        'role_assign', 'config_change',
        'scan_trigger', 'block_apply'
    )),
    resource_type TEXT,
    resource_id INTEGER,
    resource_name TEXT,
    ip_address TEXT,
    user_agent TEXT,
    request_id TEXT,
    response_status INTEGER,
    details JSON DEFAULT '{}',
    duration_ms INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_user ON audit_logs(user_id);
CREATE INDEX idx_audit_action ON audit_logs(action);
CREATE INDEX idx_audit_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_created ON audit_logs(created_at);
-- 保留最近 90 天数据，定期清理
```

#### cache_entries 表

```sql
CREATE TABLE IF NOT EXISTS cache_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    remote_url TEXT NOT NULL UNIQUE,
    local_key TEXT NOT NULL,
    package_type TEXT NOT NULL,
    etag TEXT,
    last_modified TEXT,
    content_type TEXT,
    cached_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    access_count INTEGER DEFAULT 0,
    last_accessed_at DATETIME,
    size_bytes BIGINT DEFAULT 0,
    hit_count INTEGER DEFAULT 0,
    miss_count INTEGER DEFAULT 0
);

CREATE INDEX idx_cache_package ON cache_entries(package_type);
CREATE INDEX idx_cache_expires ON cache_entries(expires_at);
CREATE INDEX idx_cache_access ON cache_entries(last_accessed_at DESC);
```

#### system_config 表 (键值配置)

```sql
CREATE TABLE IF NOT EXISTS system_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    value_type TEXT DEFAULT('string') CHECK(value_type IN (
        'string', 'int', 'bool', 'json'
    )),
    description TEXT DEFAULT '',
    is_sensitive BOOLEAN DEFAULT FALSE,  // 是否敏感 (密码等)
    updated_by INTEGER REFERENCES users(id),
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 初始配置
INSERT OR IGNORE INTO system_config (key, value, value_type, description) VALUES
    ('site_name', 'Moonlight Registry', 'string', '站点名称'),
    ('allow_registration', 'false', 'bool', '允许自助注册'),
    ('max_upload_size_mb', '500', 'int', '最大上传大小(MB)'),
    ('session_timeout_hours', '24', 'int', '会话超时时间(小时)'),
    ('security_scan_enabled', 'true', 'bool', '启用安全扫描');
```

---

## 5. API 接口设计

### 5.1 认证相关

```
POST   /api/v1/auth/login         # 用户登录
POST   /api/v1/auth/logout        # 登出 (Token 黑名单)
POST   /api/v1/auth/refresh       # 刷新 Token
GET    /api/v1/auth/profile       # 当前用户信息
PUT    /api/v1/auth/password      # 修改密码
GET    /api/v1/auth/sessions       # 活跃会话列表
DELETE /api/v1/auth/sessions/:id   # 注销指定会话
```

### 5.2 包管理 API (各协议兼容)

#### npm 协议

```
GET    /npm/                              # 包搜索
GET    /npm/:package                      # 包元数据
GET    /npm/:package/:version             # 版本元数据
PUT    /npm/:package/-rev/:revision       # 发布包 (npm publish)
DELETE /npm/:package/-rev/:revision       # 删除包 (npm unpublish)
GET    /npm/:package/-/tarball/:filename  # 下载 tarball
```

#### Maven 协议

```
GET    /maven2/:group/:artifact/:version/:file  # 下载构件
PUT    /maven2/:group/:artifact/:version/:file  # 上传构件 (deploy)
GET    /maven2/:group/maven-metadata.xml        # 元数据
```

#### PyPI 协议

```
GET    /pypi/simple/                          # 包索引 (PEP 503)
GET    /pypi/simple/:package/                 # 包文件列表
POST   /pypi/upload                           # 上传包 (twine upload)
GET    /pypi/:package/:version/json           # JSON API (PEP 691)
```

#### Go Modules 协议

```
GET    /go/:module/@v/list                    # 版本列表
GET    /go/:module/@v/:version.info           # 版本信息
GET    /go/:module/@v/:version.mod            # go.mod 内容
GET    /go/:module/@v/:version.zip            # 源码 zip
GET    /go/:module@latest                     # 最新版本
```

#### YUM/DNF 协议

```
GET    /yum/:repo/repodata/repomd.xml         # 主元数据
GET    /yum/:repo/repodata/*                  # 元数据文件
GET    /yum/:repo/Packages/*                  # RPM 包下载
POST   /yum/:repo/upload                     # 上传 RPM
POST   /yum/:repo/regenerate                 # 重新生成元数据
```

#### 其他协议 (NuGet, APT, Generic)

```
# NuGet
GET    /nuget/v3/index.json
GET    /nuget/v3-flatcontainer/:id/:ver/:id.nupkg
PUT    /nuget/v2/package

# APT
GET    /apt/dists/:dist/Release
GET    /apt/pool/:component/:prefix/:filename
POST   /apt/upload

# Generic
GET    /files/*                               # 下载任意文件
POST   /files/upload                          # 上传文件
GET    /files/browse/*                        # 浏览目录
```

### 5.3 管理 API

```
# ========== 包管理 ==========
GET    /api/v1/packages                      # 包列表 (分页/搜索/筛选)
GET    /api/v1/packages/:id                  # 包详情
DELETE /api/v1/packages/:id                  # 删除包
PATCH  /api/v1/packages/:id/deprecate        # 弃用包
GET    /api/v1/packages/:id/versions         # 版本列表
GET    /api/v1/packages/:id/versions/:vid    # 版本详情
DELETE /api/v1/packages/:id/versions/:vid    # 删除版本

# ========== 安全扫描 ==========
GET    /api/v1/packages/:id/scan             # 扫描结果
POST   /api/v1/packages/:id/scan/trigger     # 手动触发扫描
GET    /api/v1/security/dashboard            # 安全概览仪表盘
GET    /api/v1/security/vulnerabilities       # 漏洞列表
GET    /api/v1/security/statistics           # 统计数据

# ========== 用户管理 ==========
GET    /api/v1/users                          # 用户列表
POST   /api/v1/users                          # 创建用户
GET    /api/v1/users/:id                      # 用户详情
PUT    /api/v1/users/:id                      # 更新用户
DELETE /api/v1/users/:id                      # 删除用户
PATCH  /api/v1/users/:id/roles                # 分配角色
PATCH  /api/v1/users/:id/status               # 启用/禁用

# ========== 角色权限 ==========
GET    /api/v1/roles                           # 角色列表
POST   /api/v1/roles                           # 创建角色
PUT    /api/v1/roles/:id                       # 更新角色
DELETE /api/v1/roles/:id                       # 删除角色 (非内置)
GET    /api/v1/roles/:id/permissions           # 角色权限
PUT    /api/v1/roles/:id/permissions           # 更新权限

# ========== 缓存管理 ==========
GET    /api/v1/cache/stats                    # 缓存统计
GET    /api/v1/cache/entries                  # 缓存条目列表
DELETE /api/v1/cache/entries/:id              # 清除条目
POST   /api/v1/cache/prefetch                 # 预热缓存
DELETE /api/v1/cache/clear                    # 清空全部缓存
GET    /api/v1/cache/remotes                  # 远程仓库状态

# ========== 审计日志 ==========
GET    /api/v1/audit/logs                     # 日志列表 (筛选/分页)
GET    /api/v1/audit/logs/:id                 # 日志详情
GET    /api/v1/audit/export                   # 导出 (CSV/JSON)
GET    /api/v1/audit/statistics               # 统计

# ========== 系统配置 ==========
GET    /api/v1/config                          # 获取配置
PUT    /api/v1/config                          # 更新配置
POST   /api/v1/config/reset                   # 重置为默认

# ========== 系统监控 ==========
GET    /api/v1/system/health                  # 健康检查
GET    /api/v1/system/stats                   # 系统统计
GET    /api/v1/system/version                 # 版本信息
GET    /api/v1/system/info                    # 系统信息
```

### 5.4 API 响应格式

#### 成功响应

```json
{
    "code": 200,
    "message": "success",
    "data": { ... }
}
```

#### 分页响应

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "items": [...],
        "pagination": {
            "page": 1,
            "page_size": 20,
            "total": 100,
            "total_pages": 5
        }
    }
}
```

#### 错误响应

```json
{
    "code": 400,
    "message": "Validation failed",
    "errors": [
        {
            "field": "version",
            "message": "Invalid semver format"
        }
    ]
}
```

---

## 6. Web 管理后台设计

### 6.1 页面结构

```
/src/views/
├── dashboard/
│   ├── Overview.vue           # 总览仪表盘
│   ├── Activity.vue           # 最近活动
│   └── Security.vue           # 安全状态概览
│
├── packages/
│   ├── PackageList.vue        # 包列表
│   ├── PackageDetail.vue      # 包详情
│   ├── PackageVersions.vue    # 版本管理
│   └── PackageUpload.vue      # 上传界面
│
├── security/
│   ├── Dashboard.vue          # 安全概览
│   ├── Vulnerabilities.vue    # 漏洞列表
│   ├── Policies.vue           # 阻断策略
│   └── Reports.vue            # 扫描报告
│
├── users/
│   ├── UserList.vue           # 用户列表
│   ├── UserDetail.vue         # 用户详情
│   └── RoleManagement.vue     # 角色权限
│
├── cache/
│   ├── Stats.vue              # 缓存统计
│   ├── Entries.vue            # 缓存条目
│   └── Settings.vue           # 远程仓库配置
│
├── audit/
│   ├── Logs.vue               # 日志查看
│   └── Export.vue             # 导出功能
│
└── settings/
    ├── General.vue            # 基本设置
    ├── Storage.vue            # 存储配置
    ├── Proxy.vue              # 代理配置
    ├── Security.vue           # 安全设置
    └── About.vue              # 关于/版本
```

### 6.2 核心组件

```
/src/components/
├── layout/
│   ├── AppHeader.vue          # 顶部导航栏
│   ├── AppSidebar.vue         # 侧边菜单
│   ├── AppBreadcrumb.vue      # 面包屑
│   └── AppFooter.vue          # 页脚
│
├── packages/
│   ├── PackageCard.vue        # 包卡片
│   ├── VersionBadge.vue       # 版本标签
│   ├── DependencyGraph.vue    # 依赖关系图
│   ├── UploadDropzone.vue     # 拖拽上传区
│   └── MetadataViewer.vue    # 元数据查看器
│
├── security/
│   ├── VulnTable.vue          # 漏洞表格
│   ├── SeverityBadge.vue      # 严重程度标签
│   ├── ScanProgress.vue       # 扫描进度
│   └── RiskGauge.vue          # 风险仪表盘
│
└── common/
    ├── DataTable.vue          # 通用表格
    ├── SearchInput.vue        # 搜索框
    ├── StatusBadge.vue        # 状态标签
    ├── ConfirmDialog.vue      # 确认对话框
    ├── Pagination.vue         # 分页器
    └── StatCard.vue           # 统计卡片
```

### 6.3 主要页面描述

#### 📊 仪表盘 (Dashboard)

- **总览卡片**: 包总数、版本数、今日下载量、存储使用量
- **活动时间线**: 最近的上传/下载/扫描操作
- **安全状态**: 漏洞统计、待处理的高危问题
- **热门包 Top 10**: 下载量排行
- **存储使用趋势**: 近 7 天/30 天图表

#### 📦 包管理 (Packages)

- **列表页**: 搜索、按类型/状态筛选、排序、批量操作
- **详情页**: 基本信息、版本列表、依赖关系、下载统计、扫描结果
- **上传页**: 拖拽上传、进度显示、自动扫描开关
- **版本管理**: 版本对比、弃用/恢复、删除

#### 🔒 安全中心 (Security)

- **概览页**: 漏洞分布饼图、严重等级趋势、处理率
- **漏洞列表**: 按严重程度/包名/状态筛选、批量修复建议
- **策略配置**: 阻断规则调整、白名单管理
- **扫描报告**: 详细报告查看、导出 PDF

#### 👥 用户管理 (Users)

- **用户列表**: 搜索、状态筛选、角色筛选
- **用户详情**: 基本信息、角色分配、操作历史
- **角色管理**: 角色 CRUD、权限矩阵编辑

#### 💾 缓存管理 (Cache)

- **统计面板**: 命中率、存储占用、条目数量
- **条目列表**: 按类型/TTL/大小筛选、手动清除
- **远程仓库**: 连接状态、同步配置

#### 📋 审计日志 (Audit)

- **日志查看器**: 时间线视图、表格视图切换
- **高级筛选**: 用户/操作/资源/时间范围/IP
- **导出**: CSV/JSON 格式，支持自定义字段

---

## 7. 安全设计

### 7.1 安全防护层级

```
┌─────────────────────────────────────────────┐
│              应用层安全                      │
│                                             │
│  • 输入验证 (参数校验、类型检查)              │
│  • SQL 注入防护 (GORM 参数化查询)            │
│  • XSS 防护 (输出转义、CSP 头)               │
│  • CSRF 保护 (SameSite Cookie + Token)      │
│  • 文件上传校验 (Magic Number + 大小限制)    │
│                                             │
├─────────────────────────────────────────────┤
│              认证授权层                       │
│                                             │
│  • JWT 无状态认证                            │
│  • RBAC 细粒度权限                           │
│  • Token 黑名单机制                          │
│  • 登录失败次数限制                          │
│  • 密码强度策略                              │
│                                             │
├─────────────────────────────────────────────┤
│              数据安全                         │
│                                             │
│  • bcrypt 密码哈希 (cost=12)                 │
│  • AES-256-GCM 敏感字段加密                  │
│  • HMAC-SHA256 JWT 签名                      │
│  • HTTPS 强制跳转                            │
│  • 安全头 (HSTS, X-Frame-Options 等)        │
│                                             │
├─────────────────────────────────────────────┤
│              基础设施安全                     │
│                                             │
│  • Rate Limiting (IP/用户级别)               │
│  • CORS 白名单配置                           │
│  • 请求日志记录                              │
│  • 定期依赖更新                              │
│  • 安全漏洞扫描                              │
│                                             │
└─────────────────────────────────────────────┘
```

### 7.2 安全配置示例

```yaml
server:
  tls:
    enabled: true
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
  
  security_headers:
    x_frame_options: "DENY"
    x_content_type_options: "nosniff"
    x_xss_protection: "1; mode=block"
    strict_transport_security: "max-age=31536000; includeSubDomains"
    content_security_policy: "default-src 'self'"
  
  cors:
    allowed_origins:
      - "https://admin.example.com"
    allowed_methods: ["GET", "POST", "PUT", "DELETE"]
    allowed_headers: ["Authorization", "Content-Type"]
    max_age: 86400
  
  rate_limiting:
    enabled: true
    global_rps: 1000           # 全局每秒请求数
    login_rpm: 10             # 登录每分钟尝试次数
    upload_rpm: 30            # 上传每分钟次数
    api_rph: 1000             # API 每小时请求数
```

---

## 8. 部署方案

### 8.1 开发环境启动

```bash
# 克隆项目
git clone https://github.com/yourorg/moonlight-box.git
cd moonlight-box

# 一键启动 (默认 SQLite + 本地存储)
./moonlight-registry serve

# 或指定配置
./moonlight-registry serve --config config.yaml
```

### 8.2 生产部署 (Docker)

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o moonlight-registry .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/moonlight-registry .
COPY --from=builder /app/web/dist ./web/dist  # 嵌入前端
EXPOSE 8080
CMD ["./moonlight-registry", "serve"]
```

```yaml
# docker-compose.yml
version: '3.8'

services:
  registry:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data          # 持久化数据
      - ./config.yaml:/app/config.yaml:ro
    environment:
      - MOONLIGHT_ENV=production
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/api/v1/system/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  # 可选: PostgreSQL 替代 SQLite
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: moonlight
      POSTGRES_USER: registry
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    ports:
      - "5432:5432"

volumes:
  pgdata:
```

### 8.3 配置文件示例

```yaml
# config.yaml
server:
  host: 0.0.0.0
  port: 8080
  mode: release  # debug / release

database:
  driver: sqlite       # sqlite / postgres
  dsn: "./data/registry.db"
  # postgres:
  #   host: localhost
  #   port: 5432
  #   database: moonlight
  #   username: registry
  #   password: ${DB_PASSWORD}

storage:
  backend: local       # local / s3
  local:
    base_path: "./data/packages"
    max_size_gb: 1000
  s3:
    endpoint: "${S3_ENDPOINT}"
    bucket: "${S3_BUCKET}"
    region: "${S3_REGION}"
    access_key: "${AWS_ACCESS_KEY_ID}"
    secret_key: "${AWS_SECRET_ACCESS_KEY}"

auth:
  jwt_secret: "${JWT_SECRET}"
  token_expiry: 24h
  refresh_expiry: 168h
  password_min_length: 8
  max_login_attempts: 5
  lockout_duration: 15m

security:
  enabled: true
  scan_on_upload: true
  block_critical: true
  block_high: true
  vuln_db_auto_update: true

cache:
  enabled: true
  default_ttl: 24h
  max_size_gb: 100
  eviction_policy: lru

logging:
  level: info
  format: json
  output: "./data/logs/registry.log"
```

---

## 9. 项目结构

### 9.1 后端目录结构

```
moonlight-box/
├── cmd/
│   └── registry/
│       └── main.go              # 入口点
│
├── internal/
│   ├── config/
│   │   ├── config.go            # 配置加载
│   │   └── defaults.go          # 默认值
│   │
│   ├── model/
│   │   ├── user.go
│   │   ├── package.go
│   │   ├── role.go
│   │   ├── audit.go
│   │   └── cache.go
│   │
│   ├── handler/
│   │   ├── auth.go
│   │   ├── package.go
│   │   ├── admin.go
│   │   ├── user.go
│   │   ├── security.go
│   │   └── cache.go
│   │
│   ├── service/
│   │   ├── auth_service.go
│   │   ├── package_service.go
│   │   ├── storage_service.go
│   │   ├── proxy_service.go
│   │   ├── scan_service.go
│   │   └── audit_service.go
│   │
│   ├── adapter/
│   │   ├── adapter.go           # 接口定义
│   │   ├── npm_adapter.go
│   │   ├── maven_adapter.go
│   │   ├── pypi_adapter.go
│   │   ├── go_adapter.go
│   │   ├── nuget_adapter.go
│   │   ├── yum_adapter.go
│   │   ├── apt_adapter.go
│   │   └── generic_adapter.go
│   │
│   ├── storage/
│   │   ├── backend.go          # 接口定义
│   │   ├── local_storage.go
│   │   └── s3_storage.go
│   │
│   ├── middleware/
│   │   ├── auth.go
│   │   ├── rbac.go
│   │   ├── ratelimit.go
│   │   ├── cors.go
│   │   ├── requestid.go
│   │   └── recovery.go
│   │
│   ├── repository/
│   │   ├── user_repo.go
│   │   ├── package_repo.go
│   │   ├── audit_repo.go
│   │   └── cache_repo.go
│   │
│   └── util/
│       ├── hash.go
│       ├── validator.go
│       ├── response.go
│       └── pagination.go
│
├── web/                         # Vue 前端
│   ├── src/
│   ├── public/
│   ├── package.json
│   └── vite.config.ts
│
├── scripts/
│   ├── build.sh
│   ├── embed-web.sh            # 将前端嵌入二进制
│   └── generate-vulndb.sh      # 生成漏洞数据库
│
├── configs/
│   └── config.example.yaml
│
├── migrations/                  # 数据库迁移
│   ├── 001_init.sql
│   └── ...
│
├── docs/
│   ├── api.md
│   └── deployment.md
│
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
├── docker-compose.yml
└── README.md
```

### 9.2 前端目录结构

```
web/
├── src/
│   ├── assets/                 # 静态资源
│   ├── components/             # 公共组件
│   │   ├── layout/
│   │   ├── common/
│   │   ├── packages/
│   │   └── security/
│   │
│   ├── views/                  # 页面视图
│   │   ├── dashboard/
│   │   ├── packages/
│   │   ├── security/
│   │   ├── users/
│   │   ├── cache/
│   │   ├── audit/
│   │   └── settings/
│   │
│   ├── composables/            # 组合式函数
│   │   ├── useAuth.ts
│   │   ├── usePackage.ts
│   │   ├── useSecurity.ts
│   │   └── ...
│   │
│   ├── stores/                 # Pinia 状态
│   │   ├── auth.ts
│   │   ├── app.ts
│   │   └── ...
│   │
│   ├── api/                    # API 封装
│   │   ├── request.ts
│   │   ├── modules/
│   │   └── types.ts
│   │
│   ├── router/
│   │   └── index.ts
│   │
│   ├── utils/
│   ├── styles/
│   ├── App.vue
│   └── main.ts
│
├── public/
├── index.html
├── tsconfig.json
├── vite.config.ts
└── package.json
```

---

## 10. 开发路线图

### Phase 1: MVP (最小可行产品) ⭐ 第一优先级

**目标**: 能够运行的基本包仓库，支持 npm + Maven

- [ ] 项目初始化 (Go module + 目录结构)
- [ ] SQLite/GORM 数据库初始化与迁移
- [ ] Gin HTTP 服务器基础框架
- [ ] JWT 认证中间件
- [ ] 用户管理 CRUD (seed admin 用户)
- [ ] 本地文件存储后端
- [ ] **npm Adapter** (基本发布/下载)
- [ ] **Maven Adapter** (基本上传/下载)
- [ ] Web 管理后台基础框架 (登录/包列表)
- [ ] 基础审计日志

### Phase 2: 核心完善

**目标**: 完整的多语言支持 + 代理缓存

- [ ] **PyPI Adapter**
- [ ] **Go Modules Adapter**
- [ ] **NuGet Adapter**
- [ ] **YUM/DNF Adapter**
- [ ] **APT Adapter**
- [ ] **Generic Adapter**
- [ ] 代理缓存服务 (远程仓库拉取)
- [ ] 缓存管理 API 与 UI
- [ ] RBAC 权限系统完善
- [ ] 包版本管理 (弃用/恢复/yank)
- [ ] 依赖解析与展示

### Phase 3: 安全与增强

**目标**: 生产就绪的安全特性

- [ ] 安全扫描引擎
- [ ] 本地漏洞数据库集成
- [ ] 风险阻断策略
- [ ] 扫描报告生成
- [ ] 安全仪表盘
- [ ] S3 存储后端支持
- [ ] PostgreSQL 后端支持
- [ ] Rate Limiting 增强
- [ ] API 文档 (Swagger/OpenAPI)

### Phase 4: 企业特性

**目标**: 企业级功能完善

- [ ] LDAP/AD 集成
- [ ] OAuth2/SSO 支持
- [ ] 高级审计报表
- [ ] 通知系统 (邮件/Webhook)
- [ ] 统计分析面板
- [ ] 多租户支持 (可选)
- [ ] HA 高可用部署方案
- [ ] 性能优化与压测

---

## 11. 附录

### 11.1 相关项目参考

| 项目 | 说明 | 参考 |
|------|------|------|
| Sonatype Nexus | 企业级仓库管理器 | Java 实现，功能全面 |
| JFrog Artifactory | 通用制品仓库 | 商业产品，功能强大 |
| Verdaccio | 轻量级 npm 私有仓库 | Node.js，npm 专用 |
| Harbor | Docker 镜像仓库 | Go 实现，容器专用 |
| GitLab Package Registry | GitLab 集成仓库 | Ruby/Go，GitLab 生态 |
| Athens | Go modules proxy | Go 实现，Go 专用 |
| DevPI | Python 包仓库 | Python 实现，PyPI 专用 |

### 11.2 术语表

| 术语 | 定义 |
|------|------|
| Registry | 制品/包仓库 |
| Adapter | 协议适配器，将不同包管理器协议转换为统一接口 |
| Proxy | 代理模式，转发请求到远程仓库并缓存 |
| Artifact | 制品/产物，指编译后的包文件 |
| Repodata | RPM/YUM 仓库元数据 |
| Tarball | npm 包压缩格式 (.tgz) |
| Wheel | Python 二进制分发格式 (.whl) |
| RBAC | 基于角色的访问控制 |
| JWT | JSON Web Token，无状态认证令牌 |
| CVE | Common Vulnerabilities and Exposures，通用漏洞披露 |

---

> **文档版本**: v1.0
> **最后更新**: 2026-04-28
> **审批状态**: ✅ 已批准
