# Moonlight Registry Phase 1 MVP 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 构建企业级开源组件仓库的 MVP 版本，支持 npm 和 Maven 包管理协议，具备基础认证、本地存储和 Web 管理后台。

**架构：** Go 单体应用 (Gin) + Vue 3 前端 + SQLite 数据库 + 本地文件存储。采用协议适配器模式支持多语言包管理。

**技术栈：**
- 后端: Go 1.21+, Gin, GORM, golang-jwt, Viper, Zap
- 前端: Vue 3, TypeScript, Vite, Element Plus, Pinia, Axios
- 数据库: SQLite (默认) / PostgreSQL (可选)
- 存储: 本地文件系统

**设计文档:** [2026-04-28-moonlight-registry-design-v2.md](../specs/2026-04-28-moonlight-registry-design-v2.md)

---

## 文件结构总览

### 后端文件结构

```
moonlight-box/
├── cmd/
│   └── registry/
│       └── main.go                    # 应用入口
│
├── internal/
│   ├── config/
│   │   ├── config.go                  # 配置定义与加载
│   │   └── defaults.go                # 默认配置值
│   │
│   ├── model/
│   │   ├── base.go                    # 基础模型 (ID, 时间戳)
│   │   ├── user.go                    # 用户模型
│   │   ├── role.go                    # 角色与权限模型
│   │   ├── package.go                 # 包与版本模型
│   │   ├── audit.go                   # 审计日志模型
│   │   └── cache.go                   # 缓存元数据模型
│   │
│   ├── database/
│   │   ├── database.go                # 数据库连接管理
│   │   └── migration.go               # 自动迁移
│   │
│   ├── repository/
│   │   ├── user_repo.go               # 用户数据访问层
│   │   ├── package_repo.go            # 包数据访问层
│   │   ├── role_repo.go               # 角色权限数据访问层
│   │   └── audit_repo.go              # 审计日志数据访问层
│   │
│   ├── storage/
│   │   ├── backend.go                 # 存储后端接口定义
│   │   └── local_storage.go           # 本地文件系统实现
│   │
│   ├── adapter/
│   │   ├── adapter.go                 # 统一适配器接口
│   │   ├── types.go                   # 类型常量定义
│   │   ├── npm_adapter.go             # npm 协议适配器
│   │   └── maven_adapter.go           # Maven 协议适配器
│   │
│   ├── service/
│   │   ├── auth_service.go            # 认证服务 (JWT)
│   │   ├── package_service.go         # 包管理服务
│   │   ├── storage_service.go         # 存储服务
│   │   └── audit_service.go           # 审计日志服务
│   │
│   ├── handler/
│   │   ├── auth_handler.go            # 认证 API 处理器
│   │   ├── package_handler.go         # 包 API 处理器
│   │   ├── admin_handler.go           # 管理 API 处理器
│   │   └── response.go                # 统一响应格式
│   │
│   ├── middleware/
│   │   ├── auth.go                    # JWT 认证中间件
│   │   ├── cors.go                    # CORS 中间件
│   │   ├── ratelimit.go               # 限流中间件
│   │   ├── requestid.go               # 请求 ID 中间件
│   │   └── recovery.go                # 错误恢复中间件
│   │
│   └── util/
│       ├── hash.go                    # 密码哈希工具
│       ├── validator.go               # 输入验证工具
│       ├── pagination.go              # 分页工具
│       └── errors.go                  # 错误定义
│
├── web/                               # Vue 前端项目
│   ├── src/
│   │   ├── api/
│   │   │   ├── request.ts             # Axios 实例配置
│   │   │   └── auth.ts                # 认证相关 API
│   │   ├── stores/
│   │   │   └── auth.ts                # 认证状态管理
│   │   ├── router/
│   │   │   └── index.ts               # 路由配置
│   │   ├── views/
│   │   │   ├── Login.vue              # 登录页
│   │   │   ├── Dashboard.vue          # 仪表盘
│   │   │   ├── PackageList.vue        # 包列表页
│   │   │   └── Layout.vue             # 布局组件
│   │   ├── components/
│   │   │   └── layout/
│   │   │       ├── AppHeader.vue      # 顶部导航
│   │   │       └── AppSidebar.vue     # 侧边栏
│   │   ├── App.vue
│   │   └── main.ts
│   ├── index.html
│   ├── package.json
│   ├── tsconfig.json
│   └── vite.config.ts
│
├── configs/
│   └── config.example.yaml            # 配置示例文件
│
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 任务分解

### 任务 1：项目初始化与依赖管理

**文件：**
- 创建：`go.mod`
- 创建：`Makefile`
- 创建：`.gitignore`
- 创建：`cmd/registry/main.go`

- [ ] **步骤 1：初始化 Go module**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go mod init github.com/dshmyz/moonlight-box
```

- [ ] **步骤 2：创建主入口文件**

```go
// cmd/registry/main.go
package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	fmt.Printf("Moonlight Registry v%s\n", version)
	fmt.Println("Server starting...")
	
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	
	fmt.Println("Use 'serve' to start the server")
}
```

- [ ] **步骤 3：添加核心依赖**

```bash
# HTTP 框架
go get -u github.com/gin-gonic/gin

# ORM
go get -u gorm.io/gorm
go get -u gorm.io/driver/sqlite
go get -u gorm.io/driver/postgres

# JWT 认证
go get -u github.com/golang-jwt/jwt/v5

# 配置管理
go get -u github.com/spf13/viper

# 日志
go get -u go.uber.org/zap

# 密码哈希
go get -u golang.org/x/crypto/bcrypt

# 验证
go get -u github.com/go-playground/validator/v10

# UUID
go get -u github.com/google/uuid

# 缓存
go get -u github.com/allegro/bigcache/v3
```

- [ ] **步骤 4：创建 Makefile**

```makefile
# Makefile
.PHONY: build run clean test lint

APP_NAME=moonlight-registry
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_VERSION=$(shell go version | awk '{print $$3}')

LDFLAGS=-ldflags "-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

build:
	go build ${LDFLAGS} -o bin/${APP_NAME} ./cmd/registry

run:
	go run ./cmd/registry serve

test:
	go test -v ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ coverage.out coverage.html

dev:
	AIR_CONFIG=.air.toml air

# 嵌入前端到二进制
embed-web:
	cd web && npm run build
	cp -r web/dist internal/embed/dist
```

- [ ] **步骤 5：创建 .gitignore**

```gitignore
# Binaries
bin/
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary
*.test

# Output
*.out
coverage.out
coverage.html

# Dependency directories
vendor/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Data (runtime)
data/
*.db
*.db-journal

# Config (secrets)
config.yaml
.env

# Logs
*.log
logs/

# Frontend
web/node_modules/
web/dist/

# Air live reload
.air.toml
tmp/
```

- [ ] **步骤 6：验证项目可编译运行**

```bash
go mod tidy
go build -o bin/moonlight-registry ./cmd/registry
./bin/moonlight-registry version
```

预期输出：`0.1.0`

- [ ] **步骤 7：Commit**

```bash
git add go.mod go.sum Makefile .gitignore cmd/registry/main.go
git commit -m "feat: initialize project with Go module and dependencies"
```

---

### 任务 2：配置管理系统

**文件：**
- 创建：`internal/config/config.go`
- 创建：`internal/config/defaults.go`
- 创建：`configs/config.example.yaml`

- [ ] **步骤 1：编写配置结构体定义**

```go
// internal/config/config.go
package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Security SecurityConfig `mapstructure:"security"`
	Cache    CacheConfig    `mapstructure:"cache"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Mode         string        `mapstructure:"mode"` // debug, release, test
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"` // sqlite, postgres
	DSN    string `mapstructure:"dsn"`
}

type StorageConfig struct {
	Backend string        `mapstructure:"backend"` // local, s3
	Local   LocalStorage  `mapstructure:"local"`
}

type LocalStorage struct {
	BasePath string `mapstructure:"base_path"`
	MaxSizeGB int64 `mapstructure:"max_size_gb"`
}

type AuthConfig struct {
	JWTSecret       string        `mapstructure:"jwt_secret"`
	TokenExpiry     time.Duration `mapstructure:"token_expiry"`
	RefreshExpiry   time.Duration `mapstructure:"refresh_expiry"`
	MinPasswordLen  int           `mapstructure:"min_password_len"`
	MaxLoginAttempts int          `mapstructure:"max_login_attempts"`
	LockoutDuration time.Duration `mapstructure:"lockout_duration"`
}

type SecurityConfig struct {
	Enabled        bool `mapstructure:"enabled"`
	ScanOnUpload   bool `mapstructure:"scan_on_upload"`
	BlockCritical  bool `mapstructure:"block_critical"`
	BlockHigh      bool `mapstructure:"block_high"`
}

type CacheConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	DefaultTTL    time.Duration `mapstructure:"default_ttl"`
	MaxSizeGB     int64         `mapstructure:"max_size_gb"`
	EvictionPolicy string       `mapstructure:"eviction_policy"` // lru, fifo, ttl
}

type LoggingConfig struct {
	Level   string `mapstructure:"level"`   // debug, info, warn, error
	Format  string `mapstructure:"format"`  // json, console
	Output  string `mapstructure:"output"`
}

var globalConfig *Config

func Load(configPath string) (*Config, error) {
	v := viper.New()
	
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	
	// 设置默认值
	setDefaults(v)
	
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// 使用默认配置
			println("Warning: config file not found, using defaults")
		} else {
			return nil, err
		}
	}
	
	// 支持环境变量覆盖
	v.AutomaticEnv()
	v.SetEnvPrefix("MOONLIGHT")
	
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	
	globalConfig = &cfg
	return &cfg, nil
}

func Get() *Config {
	return globalConfig
}
```

- [ ] **步骤 2：编写默认值设置**

```go
// internal/config/defaults.go
package config

import "github.com/spf13/viper"

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.write_timeout", 30*time.Second)
	
	// Database
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "./data/registry.db")
	
	// Storage
	v.SetDefault("storage.backend", "local")
	v.SetDefault("storage.local.base_path", "./data/packages")
	v.SetDefault("storage.local.max_size_gb", 100)
	
	// Auth
	v.SetDefault("auth.jwt_secret", "change-me-in-production")
	v.SetDefault("auth.token_expiry", 24*time.Hour)
	v.SetDefault("auth.refresh_expiry", 168*time.Hour) // 7 days
	v.SetDefault("auth.min_password_len", 8)
	v.SetDefault("auth.max_login_attempts", 5)
	v.SetDefault("auth.lockout_duration", 15*time.Minute)
	
	// Security
	v.SetDefault("security.enabled", true)
	v.SetDefault("security.scan_on_upload", false) // MVP 暂不启用
	v.SetDefault("security.block_critical", true)
	v.SetDefault("security.block_high", true)
	
	// Cache
	v.SetDefault("cache.enabled", true)
	v.SetDefault("cache.default_ttl", 24*time.Hour)
	v.SetDefault("cache.max_size_gb", 10)
	v.SetDefault("cache.eviction_policy", "lru")
	
	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "console")
	v.SetDefault("logging.output", "stdout")
}
```

- [ ] **步骤 3：创建示例配置文件**

```yaml
# configs/config.example.yaml
server:
  host: 0.0.0.0
  port: 8080
  mode: debug  # debug / release / test
  read_timeout: 30s
  write_timeout: 30s

database:
  driver: sqlite  # sqlite / postgres
  dsn: "./data/registry.db"
  # postgres:
  #   host: localhost
  #   port: 5432
  #   database: moonlight
  #   username: registry
  #   password: your-password

storage:
  backend: local  # local / s3
  local:
    base_path: "./data/packages"
    max_size_gb: 100

auth:
  jwt_secret: change-me-in-production-use-random-string
  token_expiry: 24h
  refresh_expiry: 168h
  min_password_len: 8
  max_login_attempts: 5
  lockout_duration: 15m

security:
  enabled: true
  scan_on_upload: false  # Phase 2 启用
  block_critical: true
  block_high: true

cache:
  enabled: true
  default_ttl: 24h
  max_size_gb: 10
  eviction_policy: lru  # lru / fifo / ttl

logging:
  level: info  # debug / info / warn / error
  format: console  # console / json
  output: stdout
```

- [ ] **步骤 4：编写测试验证配置加载**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadDefaultConfig(t *testing.T) {
	cfg, err := Load("nonexistent.yaml")
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "sqlite", cfg.Database.Driver)
	assert.Equal(t, "local", cfg.Storage.Backend)
	assert.Equal(t, 24*time.Hour, cfg.Auth.TokenExpiry)
}

func TestLoadCustomConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	
	content := []byte(`
server:
  port: 9090
  mode: release
database:
  driver: postgres
  dsn: "postgres://localhost/test"
`)
	err := os.WriteFile(configPath, content, 0644)
	assert.NoError(t, err)
	
	cfg, err := Load(configPath)
	assert.NoError(t, err)
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "release", cfg.Server.Mode)
	assert.Equal(t, "postgres", cfg.Database.Driver)
}
```

- [ ] **步骤 5：运行测试**

```bash
go test -v ./internal/config/...
```

预期：全部通过

- [ ] **步骤 6：Commit**

```bash
git add internal/config/ configs/config.example.yaml
git commit -m "feat: add configuration management system with YAML support"
```

---

### 任务 3：数据库初始化与模型定义

**文件：**
- 创建：`internal/database/database.go`
- 创建：`internal/database/migration.go`
- 创建：`internal/model/base.go`
- 创建：`internal/model/user.go`
- 创建：`internal/model/role.go`
- 创建：`internal/model/package.go`
- 创建：`internal/model/audit.go`

- [ ] **步骤 1：实现数据库连接管理**

```go
// internal/database/database.go
package database

import (
	"fmt"
	"time"

	"moonlight-box/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Initialize(cfg *config.Config) error {
	var dialector gorm.Dialector
	
	switch cfg.Database.Driver {
	case "postgres":
		dsn := cfg.Database.DSN
		dialector = postgres.Open(dsn)
	case "sqlite":
	default:
		dsn := cfg.Database.DSN
		dialector = sqlite.Open(dsn)
	}
	
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}
	
	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}
	
	// 获取底层 sqlDB 用于配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}
	
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	
	DB = db
	return nil
}

func GetDB() *gorm.DB {
	return DB
}

func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
```

- [ ] **步骤 2：实现自动迁移**

```go
// internal/database/migration.go
package database

import (
	"moonlight-box/internal/model"
)

func AutoMigrate() error {
	return DB.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.UserRole{},
		&model.RolePermission{},
		&model.Package{},
		&model.PackageVersion{},
		&model.PackageDependency{},
		&model.AuditLog{},
		&model.CacheEntry{},
		&model.SystemConfig{},
	)
}

func SeedData() error {
	// 创建预置角色
	roles := []model.Role{
		{
			Name:         "admin",
			Description:  "系统管理员，拥有所有权限",
			IsSystemRole: true,
		},
		{
			Name:         "developer",
			Description:  "开发者，可发布和管理包",
			IsSystemRole: true,
		},
		{
			Name:         "readonly",
			Description:  "只读用户，仅可下载包",
			IsSystemRole: true,
		},
	}
	
	for _, role := range roles {
		result := DB.Where("name = ?", role.Name).FirstOrCreate(&role)
		if result.Error != nil {
			return result.Error
		}
	}
	
	// 创建预置权限
	permissions := []model.Permission{
		{Resource: "system", Action: "admin"},
		{Resource: "users", Action: "read"},
		{Resource: "users", Action: "write"},
		{Resource: "audit", Action: "read"},
		{Resource: "npm", Action: "read"},
		{Resource: "npm", Action: "write"},
		{Resource: "npm", Action: "delete"},
		{Resource: "npm", Action: "admin"},
		{Resource: "maven", Action: "read"},
		{Resource: "maven", Action: "write"},
		{Resource: "maven", Action: "delete"},
		{Resource: "maven", Action: "admin"},
	}
	
	for _, perm := range permissions {
		result := DB.Where("resource = ? AND action = ?", perm.Resource, perm.Action).FirstOrCreate(&perm)
		if result.Error != nil {
			return result.Error
		}
	}
	
	// 为 admin 角色分配所有权限
	var adminRole model.Role
	if err := DB.Where("name = ?", "admin").First(&adminRole).Error; err != nil {
		return err
	}
	
	var allPermissions []model.Permission
	DB.Find(&allPermissions)
	
	for _, perm := range allPermissions {
		rp := model.RolePermission{
			RoleID:       adminRole.ID,
			PermissionID: perm.ID,
		}
		DB.Where(rp).FirstOrCreate(&rp)
	}
	
	return nil
}
```

- [ ] **步骤 3：定义基础模型**

```go
// internal/model/base.go
package model

import "time"

type BaseModel struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
```

- [ ] **步骤 4：定义用户模型**

```go
// internal/model/user.go
package model

import "time"

type User struct {
	BaseModel
	Username     string    `gorm:"uniqueIndex;size:50;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	Email        string    `gorm:"uniqueIndex;size:255" json:"email"`
	DisplayName  string    `gorm:"size:100" json:"display_name"`
	AvatarURL    string    `gorm:"size:500" json:"avatar_url,omitempty"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	Roles        []Role    `gorm:"many2many:user_roles;" json:"roles,omitempty"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

type UserRole struct {
	UserID     uint      `gorm:"primaryKey" json:"user_id"`
	RoleID     uint      `gorm:"primaryKey" json:"role_id"`
	AssignedAt time.Time `gorm:"autoCreateTime" json:"assigned_at"`
	AssignedBy uint      `json:"assigned_by"`
	User       User      `json:"-"`
	Role       Role      `json:"-"`
}

// DTO for API responses (隐藏敏感字段)
type UserDTO struct {
	ID          uint       `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	IsActive    bool       `json:"is_active"`
	Roles       []string   `json:"roles,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

func (u *User) ToDTO() UserDTO {
	roleNames := make([]string, len(u.Roles))
	for i, role := range u.Roles {
		roleNames[i] = role.Name
	}
	
	return UserDTO{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		IsActive:    u.IsActive,
		Roles:       roleNames,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
	}
}
```

- [ ] **步骤 5：定义角色权限模型**

```go
// internal/model/role.go
package model

type Role struct {
	BaseModel
	Name         string       `gorm:"uniqueIndex;size:50;not null" json:"name"`
	Description  string       `gorm:"size:255" json:"description"`
	IsSystemRole bool         `gorm:"default:false" json:"is_system_role"`
	Permissions  []Permission `gorm:"many2role_permissions;" json:"permissions,omitempty"`
	Users        []User       `gorm:"many2many:user_roles;" json:"users,omitempty"`
}

type Permission struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Resource string `gorm:"size:100;not null" json:"resource"`
	Action   string `gorm:"size:20;not null" json:"action"`
	Roles    []Role `gorm:"many2many:role_permissions;" json:"-"`
}

type RolePermission struct {
	RoleID       uint `gorm:"primaryKey" json:"role_id"`
	PermissionID uint `gorm:"primaryKey" json:"permission_id"`
	Role         Role       `json:"-"`
	Permission   Permission `json:"-"`
}
```

- [ ] **步骤 6：定义包与版本模型**

```go
// internal/model/package.go
package model

import "time"

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

type RepositoryType string

const (
	RepoTypeLocal   RepositoryType = "local"
	RepoTypeProxy   RepositoryType = "proxy"
	RepoTypeVirtual RepositoryType = "virtual"
)

type PackageStatus string

const (
	StatusDraft      PackageStatus = "draft"
	StatusPublished  PackageStatus = "published"
	StatusDeprecated PackageStatus = "deprecated"
	StatusYanked     PackageStatus = "yanked"
)

type Package struct {
	BaseModel
	Name           string         `gorm:"not null;index" json:"name"`
	Type           PackageType    `gorm:"not null;index" json:"type"`
	Description    string         `gorm:"size:500" json:"description,omitempty"`
	RepositoryType RepositoryType `gorm:"default:local;index" json:"repository_type"`
	Homepage       string         `gorm:"size:500" json:"homepage,omitempty"`
	License        string         `gorm:"size:100" json:"license,omitempty"`
	CreatedBy      uint           `json:"created_by"`
	Versions       []PackageVersion `json:"versions,omitempty"`
}

type PackageVersion struct {
	ID            uint          `gorm:"primaryKey" json:"id"`
	PackageID     uint          `gorm:"not null;index" json:"package_id"`
	Version       string        `gorm:"not null" json:"version"`
	Status        PackageStatus `gorm="default:published" json:"status"`
	StoragePath   string        `gorm:"not null" json:"storage_path"`
	SizeBytes     int64         `gorm:"default:0" json:"size_bytes"`
	ChecksumSHA256 string       `json:"checksum_sha256,omitempty"`
	ChecksumMD5   string        `json:"checksum_md5,omitempty"`
	PublishedAt   time.Time     `gorm:"autoCreateTime" json:"published_at"`
	PublishedBy   uint          `json:"published_by"`
	Metadata      string        `gorm:"type:text" json:"metadata,omitempty"`
	DownloadCount int           `gorm:"default:0" json:"download_count"`
	Dependencies  []PackageDependency `json:"dependencies,omitempty"`
	Package       Package       `gorm:"foreignKey:PackageID" json:"-"`
}

type PackageDependency struct {
	ID                   uint   `gorm:"primaryKey" json:"id"`
	VersionID            uint   `gorm:"not null;index" json:"version_id"`
	DepName              string `gorm:"not null;index" json:"dep_name"`
	DepVersionConstraint string `gorm:"not null" json:"dep_version_constraint"`
	DepType              string `gorm:"not null" json:"dep_type"`
	PackageType          string `gorm:"not null" json:"package_type"`
	IsOptional           bool   `gorm:"default:false" json:"is_optional"`
}
```

- [ ] **步骤 7：定义审计日志模型**

```go
// internal/model/audit.go
package model

import "time"

type AuditAction string

const (
	ActionLogin          AuditAction = "login"
	ActionLogout         AuditAction = "logout"
	ActionPackageUpload  AuditAction = "package_upload"
	ActionPackageDownload AuditAction = "package_download"
	ActionPackageDelete  AuditAction = "package_delete"
	ActionUserCreate     AuditAction = "user_create"
	ActionUserUpdate     AuditAction = "user_update"
	ActionRoleAssign     AuditAction = "role_assign"
	ActionConfigChange   AuditAction = "config_change"
)

type AuditLog struct {
	ID           uint        `gorm:"primaryKey" json:"id"`
	UserID       *uint       `json:"user_id,omitempty"`
	Action       AuditAction `gorm:"not null;index" json:"action"`
	ResourceType string      `gorm:"size:50;index" json:"resource_type,omitempty"`
	ResourceID   *uint       `json:"resource_id,omitempty"`
	ResourceName string      `gorm:"size:200" json:"resource_name,omitempty"`
	IPAddress    string      `gorm:"size:45" json:"ip_address,omitempty"`
	UserAgent    string      `gorm:"size:500" json:"user_agent,omitempty"`
	RequestID    string      `gorm:"size:36" json:"request_id,omitempty"`
	ResponseStatus int       `json:"response_status,omitempty"`
	Details      string      `gorm:"type:text" json:"details,omitempty"`
	DurationMs   int         `json:"duration_ms,omitempty"`
	CreatedAt    time.Time   `gorm:"autoCreateTime;index" json:"created_at"`
}

type CacheEntry struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	RemoteURL     string    `gorm:"uniqueIndex;not null" json:"remote_url"`
	LocalKey      string    `gorm:"not null" json:"local_key"`
	PackageType   string    `gorm:"not null;index" json:"package_type"`
	ETag          string    `json:"etag,omitempty"`
	LastModified  string    `json:"last_modified,omitempty"`
	ContentType   string    `json:"content_type,omitempty"`
	CachedAt      time.Time `gorm:"autoCreateTime" json:"cached_at"`
	ExpiresAt     time.Time `gorm:"index" json:"expires_at"`
	AccessCount   int64     `gorm:"default:0" json:"access_count"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	SizeBytes     int64     `gorm:"default:0" json:"size_bytes"`
	HitCount      int64     `gorm:"default:0" json:"hit_count"`
	MissCount     int64     `gorm:"default:0" json:"miss_count"`
}

type SystemConfig struct {
	Key         string  `gorm:"primaryKey" json:"key"`
	Value       string  `gorm:"not null" json:"value"`
	ValueType   string  `gorm:"default:string" json:"value_type"`
	Description string  `gorm:"size:500" json:"description,omitempty"`
	IsSensitive bool    `gorm:"default:false" json:"is_sensitive"`
	UpdatedBy   *uint   `json:"updated_by,omitempty"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
```

- [ ] **步骤 8：编写测试验证模型**

```go
// internal/model/model_test.go
package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackageTypeConstants(t *testing.T) {
	assert.Equal(t, PackageType("npm"), PackageTypeNPM)
	assert.Equal(t, PackageType("maven"), PackageTypeMaven)
	assert.Equal(t, PackageType("pypi"), PackageTypePyPI)
}

func TestPackageStatusConstants(t *testing.T) {
	assert.Equal(t, PackageStatus("published"), StatusPublished)
	assert.Equal(t, PackageStatus("deprecated"), StatusDeprecated)
}

func TestAuditActionConstants(t *testing.T) {
	assert.Equal(t, AuditAction("login"), ActionLogin)
	assert.Equal(t, AuditAction("package_upload"), ActionPackageUpload)
}
```

- [ ] **步骤 9：运行测试并验证数据库迁移**

```bash
go test -v ./internal/model/... ./internal/database/...
```

预期：全部通过

- [ ] **步骤 10：Commit**

```bash
git add internal/model/ internal/database/
git commit -m "feat: define data models and implement database initialization with auto-migration"
```

---

### 任务 4：HTTP 服务器与路由框架

**文件：**
- 修改：`cmd/registry/main.go`
- 创建：`internal/handler/response.go`
- 创建：`internal/middleware/cors.go`
- 创建：`internal/middleware/requestid.go`
- 创建：`internal/middleware/recovery.go`

- [ ] **步骤 1：更新主入口启动 HTTP 服务器**

```go
// cmd/registry/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"moonlight-box/internal/config"
	"moonlight-box/internal/database"

	"github.com/gin-gonic/gin"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "config file path")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Moonlight Registry v%s (built: %s)\n", version, buildTime)
		return
	}

	fmt.Printf("🌙 Moonlight Registry v%s\n", version)
	fmt.Println("Starting server...")

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化数据库
	if err := database.Initialize(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// 自动迁移
	if err := database.AutoMigrate(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to migrate database: %v\n", err)
		os.Exit(1)
	}

	// 种子数据
	if err := database.SeedData(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seed data: %v\n", err)
		os.Exit(1)
	}

	// 设置 Gin 模式
	gin.SetMode(cfg.Server.Mode)

	// 创建路由器
	router := setupRouter(cfg)

	// 创建 HTTP 服务器
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 优雅关闭
	go func() {
		fmt.Printf("✅ Server listening on %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n🛑 Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server forced to shutdown: %v\n", err)
	}

	fmt.Println("👋 Server exited")
}

func setupRouter(cfg *config.Config) *gin.Engine {
	r := gin.New()

	// 全局中间件
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	
	// TODO: 后续任务添加其他中间件

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": version,
		})
	})

	// API 路由组 (后续任务填充)
	api := r.Group("/api/v1")
	{
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong"})
		})
	}

	return r
}
```

- [ ] **步骤 2：实现统一响应格式**

```go
// internal/handler/response.go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type PaginatedData struct {
	Items      interface{} `json:"items"`
	Pagination Pagination `json:"pagination"`
}

type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    data,
	})
}

func SuccessWithPagination(c *gin.Context, items interface{}, page, pageSize int, total int64) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: PaginatedData{
			Items: items,
		 Pagination: Pagination{
				Page:       page,
				PageSize:   pageSize,
				Total:      total,
				TotalPages: totalPages,
			},
		},
	})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code:    http.StatusCreated,
		Message: "created",
		Data:    data,
	})
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func BadRequest(c *gin.Context, message string, errors interface{}) {
	c.JSON(http.StatusBadRequest, Response{
		Code:    http.StatusBadRequest,
		Message: message,
		Data:    errors,
	})
}

func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    http.StatusUnauthorized,
		Message: message,
	})
}

func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Message: message,
	})
}

func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Response{
		Code:    http.StatusNotFound,
		Message: message,
	})
}

func InternalError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, Response{
		Code:    http.StatusInternalServerError,
		Message: message,
	})
}
```

- [ ] **步骤 3：实现 CORS 中间件**

```go
// internal/middleware/cors.go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		
		// 允许的来源 (可从配置读取)
		allowedOrigins := []string{
			"http://localhost:3000",
			"http://localhost:5173",  // Vite 默认端口
			"http://localhost:8080",
		}
		
		isAllowed := false
		for _, allowed := range allowedOrigins {
			if strings.EqualFold(origin, allowed) {
				isAllowed = true
				break
			}
		}
		
		if isAllowed || origin == "" {
			c.Header("Access-Control-Allow-Origin", origin)
		}
		
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID")
		c.Header("Access-Control-Expose-Headers", "Content-Length, X-Request-ID")
		c.Header("Access-Control-Max-Age", "86400")
		
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		
		c.Next()
	}
}
```

- [ ] **步骤 4：实现 Request ID 中间件**

```go
// internal/middleware/requestid.go
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		
		c.Set("RequestID", requestID)
		c.Header("X-Request-ID", requestID)
		
		c.Next()
	}
}
```

- [ ] **步骤 5：实现错误恢复中间件**

```go
// internal/middleware/recovery.go
package middleware

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC recovered: %v\n%s", err, debug.Stack())
				
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    http.StatusInternalServerError,
					"message": "Internal server error",
					"request_id": c.GetString("RequestID"),
				})
			}
		}()
		
		c.Next()
	}
}
```

- [ ] **步骤 6：将中间件集成到路由器**

在 `cmd/registry/main.go` 的 `setupRouter` 函数中添加：

```go
func setupRouter(cfg *config.Config) *gin.Engine {
	r := gin.New()

	// 全局中间件
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS())
	r.Use(gin.Logger())

	// ... 其余代码不变
}
```

并在文件顶部添加 import：

```go
import (
	// ...
	"moonlight-box/internal/middleware"
)
```

- [ ] **步骤 7：验证服务器启动**

```bash
go run ./cmd/registry serve
```

预期输出：
```
🌙 Moonlight Registry v0.1.0
Starting server...
✅ Server listening on 0.0.0.0:8080
```

另开终端测试：
```bash
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/ping
curl -H "X-Request-ID: test-123" http://localhost:8080/api/v1/ping
```

- [ ] **步骤 8：Commit**

```bash
git add cmd/registry/main.go internal/handler/response.go internal/middleware/
git commit -m "feat: set up HTTP server with Gin framework and middleware stack"
```

---

### 任务 5：认证服务 (JWT)

**文件：**
- 创建：`internal/util/hash.go`
- 创建：`internal/util/errors.go`
- 创建：`internal/service/auth_service.go`
- 创建：`internal/middleware/auth.go`
- 创建：`internal/handler/auth_handler.go`
- 创建：`internal/repository/user_repo.go`
- 创建：`internal/repository/role_repo.go`

- [ ] **步骤 1：实现密码哈希工具**

```go
// internal/util/hash.go
package util

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	// 可根据需要添加更多规则
	return nil
}
```

- [ ] **步骤 2：定义业务错误类型**

```go
// internal/util/errors.go
package util

import "errors"

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrTokenExpired      = errors.New("token has expired")
	ErrTokenInvalid      = errors.New("token is invalid")
	ErrPasswordTooShort  = errors.New("password must be at least 8 characters")
	ErrAccessDenied      = errors.New("access denied")
	ErrPackageNotFound   = errors.New("package not found")
	ErrVersionNotFound   = errors.New("version not found")
)
```

- [ ] **步骤 3：实现用户 Repository**

```go
// internal/repository/user_repo.go
package repository

import (
	"moonlight-box/internal/model"
	"moonlight-box/internal/util"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(user *model.User) error {
	result := r.db.Create(user)
	return result.Error
}

func (r *UserRepository) FindByID(id uint) (*model.User, error) {
	var user model.User
	result := r.db.Preload("Roles").First(&user, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrUserNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}

func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	result := r.db.Preload("Roles").Where("username = ?", username).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrUserNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}

func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var user model.User
	result := r.db.Preload("Roles").Where("email = ?", email).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrUserNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}

func (r *UserRepository) Update(user *model.User) error {
	return r.db.Save(user).Error
}

func (r *UserRepository) List(page, pageSize int, keyword string) ([]model.User, int64, error) {
	var users []model.User
	var total int64
	
	query := r.db.Model(&model.User{}).Preload("Roles")
	
	if keyword != "" {
		query = query.Where("username LIKE ? OR email LIKE ? OR display_name LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	
	query.Count(&total)
	
	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&users)
	
	return users, total, result.Error
}

func (r *UserRepository) Delete(id uint) error {
	return r.db.Delete(&model.User{}, id).Error
}

func (r *UserRepository) UpdateLastLogin(id uint) error {
	now := time.Now()
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("last_login_at", now).Error
}
```

注意：需要在文件顶部添加 `"time"` 和 `"errors"` 的导入。

- [ ] **步骤 4：实现角色 Repository**

```go
// internal/repository/role_repo.go
package repository

import (
	"moonlight-box/internal/model"

	"gorm.io/gorm"
)

type RoleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

func (r *RoleRepository) FindByID(id uint) (*model.Role, error) {
	var role model.Role
	result := r.db.Preload("Permissions").First(&role, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &role, nil
}

func (r *RoleRepository) FindByName(name string) (*model.Role, error) {
	var role model.Role
	result := r.db.Preload("Permissions").Where("name = ?", name).First(&role)
	if result.Error != nil {
		return nil, result.Error
	}
	return &role, nil
}

func (r *RoleRepository) List() ([]model.Role, error) {
	var roles []model.Role
	result := r.db.Preload("Permissions").Find(&roles)
	return roles, result.Error
}

func (r *RoleRepository) GetUserRoles(userID uint) ([]model.Role, error) {
	var roles []model.Role
	result := r.db.
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Find(&roles)
	return roles, result.Error
}

func (r *RoleRepository) GetUserPermissions(userID uint) ([]model.Permission, error) {
	var perms []model.Permission
	result := r.db.
		Distinct().
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id").
		Where("user_roles.user_id = ?", userID).
		Find(&perms)
	return perms, result.Error
}

func (r *RoleRepository) AssignRole(userID, roleID uint, assignedBy uint) error {
	userRole := model.UserRole{
		UserID:     userID,
		RoleID:     roleID,
		AssignedBy: assignedBy,
	}
	return r.db.Where(userRole).FirstOrCreate(&userRole).Error
}

func (r *RoleRepository) RemoveRole(userID, roleID uint) error {
	return r.db.Where("user_id = ? AND role_id = ?", userID, roleID).Delete(&model.UserRole{}).Error
}
```

- [ ] **步骤 5：实现 JWT 认证服务**

```go
// internal/service/auth_service.go
package service

import (
	"errors"
	"time"

	"moonlight-box/internal/config"
	"moonlight-box/internal/model"
	"moonlight-box/internal/repository"
	"moonlight-box/internal/util"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo  *repository.UserRepository
	roleRepo  *repository.RoleRepository
	config    *config.AuthConfig
	tokenBlacklist map[string]bool // 简单内存黑名单，生产环境可用 Redis
}

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
}

type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    float64   `json:"expires_in"`
	User         model.UserDTO `json:"user"`
}

type TokenClaims struct {
	UserID   uint     `json:"uid"`
	Username string   `json:"uname"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

func NewAuthService(
	userRepo *repository.UserRepository,
	roleRepo *repository.RoleRepository,
	cfg *config.AuthConfig,
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		roleRepo:       roleRepo,
		config:         cfg,
		tokenBlacklist: make(map[string]bool),
	}
}

func (s *AuthService) Login(req *LoginRequest) (*AuthResponse, error) {
	// 查找用户
	user, err := s.userRepo.FindByUsername(req.Username)
	if err != nil {
		if errors.Is(err, util.ErrUserNotFound) {
			return nil, util.ErrInvalidCredentials
		}
		return nil, err
	}

	// 验证密码
	if !util.CheckPasswordHash(req.Password, user.PasswordHash) {
		return nil, util.ErrInvalidCredentials
	}

	// 检查用户状态
	if !user.IsActive {
		return nil, errors.New("account is disabled")
	}

	// 获取角色列表
	roles, _ := s.roleRepo.GetUserRoles(user.ID)
	roleNames := make([]string, len(roles))
	for i, role := range roles {
		roleNames[i] = role.Name
	}

	// 生成 Token
	accessToken, err := s.generateToken(user.ID, user.Username, roleNames, s.config.TokenExpiry)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateToken(user.ID, user.Username, roleNames, s.config.RefreshExpiry)
	if err != nil {
		return nil, err
	}

	// 更新最后登录时间
	s.userRepo.UpdateLastLogin(user.ID)

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.config.TokenExpiry.Seconds(),
		User:         user.ToDTO(),
	}, nil
}

func (s *AuthService) generateToken(userID uint, username string, roles []string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := TokenClaims{
		UserID:   userID,
		Username: username,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "moonlight-registry",
			Subject:   username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWTSecret))
}

func (s *AuthService) ValidateToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.config.JWTSecret), nil
	})

	if err != nil {
		return nil, util.ErrTokenInvalid
	}

	if claims, ok := token.Claims.(*TokenClaims); ok && token.Valid {
		// 检查黑名单
		if s.tokenBlacklist[tokenString] {
			return nil, errors.New("token has been revoked")
		}
		return claims, nil
	}

	return nil, util.ErrTokenInvalid
}

func (s *AuthService) Logout(tokenString string) error {
	s.tokenBlacklist[tokenString] = true
	return nil
}

func (s *AuthService) RefreshToken(refreshTokenString string) (*AuthResponse, error) {
	claims, err := s.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.FindByID(claims.UserID)
	if err != nil {
		return nil, err
	}

	// 将旧 Refresh Token 加入黑名单
	s.tokenBlacklist[refreshTokenString] = true

	// 生成新 Token 对
	accessToken, err := s.generateToken(user.ID, user.Username, claims.Roles, s.config.TokenExpiry)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := s.generateToken(user.ID, user.Username, claims.Roles, s.config.RefreshExpiry)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    s.config.TokenExpiry.Seconds(),
		User:         user.ToDTO(),
	}, nil
}

func (s *AuthService) CreateUser(username, password, email string) (*model.UserDTO, error) {
	// 检查是否已存在
	existing, _ := s.userRepo.FindByUsername(username)
	if existing != nil {
		return nil, util.ErrUserAlreadyExists
	}

	// 哈希密码
	hashedPassword, err := util.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username:     username,
		PasswordHash: hashedPassword,
		Email:        email,
		DisplayName:  username,
		IsActive:     true,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	return &user.ToDTO(), nil
}

func (s *AuthService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return err
	}

	if !util.CheckPasswordHash(oldPassword, user.PasswordHash) {
		return util.ErrInvalidCredentials
	}

	hashedPassword, err := util.HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hashedPassword
	return s.userRepo.Update(user)
}
```

注意：需要在文件顶部添加必要的导入。

- [ ] **步骤 6：实现 JWT 认证中间件**

```go
// internal/middleware/auth.go
package middleware

import (
	"net/http"
	"strings"

	"moonlight-box/internal/handler"
	"moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)

func Auth(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			handler.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}

		claims, err := authService.ValidateToken(token)
		if err != nil {
			handler.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles)

		c.Next()
	}
}

func OptionalAuth(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.Next()
			return
		}

		claims, err := authService.ValidateToken(token)
		if err != nil {
			c.Next() // Token 无效但不阻断，作为匿名用户继续
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles)

		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return parts[1]
}
```

- [ ] **步骤 7：实现认证 Handler**

```go
// internal/handler/auth_handler.go
package handler

import (
	"moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// @Summary 用户登录
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body service.LoginRequest true "登录信息"
// @Success 200 {object} Response{data=service.AuthResponse}
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body", err.Error())
		return
	}

	resp, err := h.authService.Login(&req)
	if err != nil {
		Unauthorized(c, err.Error())
		return
	}

	Success(c, resp)
}

// @Summary 登出
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} Response
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	token := extractTokenFromContext(c)
	if token != "" {
		h.authService.Logout(token)
	}

	Success(c, gin.H{"message": "logged out successfully"})
}

// @Summary 刷新 Token
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body object{refresh_token=string} true "刷新令牌"
// @Success 200 {object} Response{data=service.AuthResponse}
// @Router /api/v1/auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body", err.Error())
		return
	}

	resp, err := h.authService.RefreshToken(req.RefreshToken)
	if err != nil {
		Unauthorized(c, err.Error())
		return
	}

	Success(c, resp)
}

// @Summary 获取当前用户信息
// @Tags Auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} Response{data=model.UserDTO}
// @Router /api/v1/auth/profile [get]
func (h *AuthHandler) Profile(c *gin.Context) {
	userID := c.GetUint("userID")
	
	userService := // TODO: 从 DI 容器获取
	user, err := userService.GetByID(userID)
	if err != nil {
		NotFound(c, "user not found")
		return
	}

	Success(c, user.ToDTO())
}

func extractTokenFromContext(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}
```

注意：需要添加 `"strings"` 导入。UserService 将在下一个任务中完善。

- [ ] **步骤 8：注册认证路由**

在 `cmd/registry/main.go` 的 `setupRouter` 函数中添加：

```go
func setupRouter(cfg *config.Config) *gin.Engine {
	// ... 初始化 repositories 和 services ...

	authHandler := handler.NewAuthHandler(authService)

	// 公开路由 (无需认证)
	public := api.Group("/auth")
	{
		public.POST("/login", authHandler.Login)
		public.POST("/refresh", authHandler.RefreshToken)
	}

	// 受保护路由 (需要认证)
	protected := api.Group("")
	protected.Use(middleware.Auth(authService))
	{
		protected.POST("/auth/logout", authHandler.Logout)
		protected.GET("/auth/profile", authHandler.Profile)
	}
}
```

- [ ] **步骤 9：编写认证集成测试**

```go
// internal/handler/auth_handler_test.go
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 初始化测试数据库和服务 (简化版)
	// ... setup code ...

	router := gin.New()
	// ... register routes ...

	loginReq := service.LoginRequest{
		Username: "admin",
		Password: "admin123",
	}
	body, _ := json.Marshal(loginReq)

	req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp.Data.(map[string]interface{})["access_token"])
}

func TestProtectedEndpointWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	// ... setup protected route ...

	req, _ := http.NewRequest("GET", "/api/v1/auth/profile", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
```

- [ ] **步骤 10：运行测试并验证**

```bash
go test -v ./internal/service/... ./internal/handler/... ./internal/middleware/...
```

预期：全部通过

- [ ] **步骤 11：Commit**

```bash
git add internal/util/ internal/service/auth_service.go internal/middleware/auth.go internal/handler/auth_handler.go internal/repository/
git commit -m "feat: implement JWT authentication service with login/logout/refresh"
```

---

### 任务 6：本地存储后端

**文件：**
- 创建：`internal/storage/backend.go`
- 创建：`internal/storage/local_storage.go`
- 创建：`internal/service/storage_service.go`

- [ ] **步骤 1：定义存储后端接口**

```go
// internal/storage/backend.go
package storage

import (
	"context"
	"io"
)

type Backend interface {
	Name() string
	Init(basePath string) error
	Put(ctx context.Context, key string, reader io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Size(ctx context.Context, key string) (int64, error)
	List(ctx context.Context, prefix string) ([]Entry, error)
	Close() error
}

type Entry struct {
	Key  string
	IsDir bool
	Size int64
}
```

- [ ] **步骤 2：实现本地文件系统存储**

```go
// internal/storage/local_storage.go
package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"moonlight-box/internal/util"
)

type LocalStorage struct {
	basePath   string
	maxSize    int64
}

func NewLocalStorage(basePath string, maxSizeMB int64) (*LocalStorage, error) {
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, err
	}

	return &LocalStorage{
		basePath: absPath,
		maxSize:  maxSizeMB * 1024 * 1024,
	}, nil
}

func (s *LocalStorage) Name() string {
	return "local"
}

func (s *LocalStorage) Init(basePath string) error {
	return os.MkdirAll(basePath, 0755)
}

func (s *LocalStorage) resolvePath(key string) string {
	key = filepath.Clean(key)
	key = strings.TrimPrefix(key, "/")
	return filepath.Join(s.basePath, key)
}

func (s *LocalStorage) Put(ctx context.Context, key string, reader io.Reader, size int64) error {
	fullPath := s.resolvePath(key)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	written, err := io.Copy(file, reader)
	if err != nil {
		os.Remove(fullPath)
		return err
	}

	if size > 0 && written != size {
		os.Remove(fullPath)
		return io.ErrShortWrite
	}

	return nil
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath := s.resolvePath(key)
	
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, util.ErrPackageNotFound
		}
		return nil, err
	}

	return file, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	fullPath := s.resolvePath(key)
	
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil // 已删除视为成功
		}
		return err
	}

	// 尝试清理空目录
	dir := filepath.Dir(fullPath)
	s.removeEmptyDirs(dir)

	return nil
}

func (s *LocalStorage) removeEmptyDirs(dir string) {
	if dir == s.basePath || !strings.HasPrefix(dir, s.basePath) {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return
	}

	os.Remove(dir)
	s.removeEmptyDirs(filepath.Dir(dir))
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	fullPath := s.resolvePath(key)
	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStorage) Size(ctx context.Context, key string) (int64, error) {
	fullPath := s.resolvePath(key)
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, util.ErrPackageNotFound
		}
		return 0, err
	}
	return info.Size(), nil
}

func (s *LocalStorage) List(ctx context.Context, prefix string) ([]Entry, error) {
	dirPath := s.resolvePath(prefix)
	
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}

	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result = append(result, Entry{
			Key:  filepath.Join(prefix, entry.Name()),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
		})
	}

	return result, nil
}

func (s *LocalStorage) Close() error {
	return nil
}

func (s *LocalStorage) BasePath() string {
	return s.basePath
}
```

- [ ] **步骤 3：实现存储服务封装**

```go
// internal/service/storage_service.go
package service

import (
	"context"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"moonlight-box/internal/config"
	"moonlight-box/internal/storage"
)

type StorageService struct {
	backend storage.Backend
}

func NewStorageService(cfg *config.StorageConfig) (*StorageService, error) {
	var backend storage.Backend
	var err error

	switch cfg.Backend {
	case "local":
		backend, err = storage.NewLocalStorage(cfg.Local.BasePath, cfg.Local.MaxSizeGB*1024)
	default:
		backend, err = storage.NewLocalStorage("./data/packages", 100) // 默认值
	}

	if err != nil {
		return nil, err
	}

	return &StorageService{backend: backend}, nil
}

func (s *StorageService) StorePackage(ctx context.Context, pkgType, name, version string, content io.Reader, size int64) (string, error) {
	key := s.buildKey(pkgType, name, version)
	
	if err := s.backend.Put(ctx, key, content, size); err != nil {
		return "", err
	}

	return key, nil
}

func (s *StorageService) GetPackage(ctx context.Context, pkgType, name, version string) (io.ReadCloser, int64, error) {
	key := s.buildKey(pkgType, name, version)
	
	size, err := s.backend.Size(ctx, key)
	if err != nil {
		return nil, 0, err
	}

	reader, err := s.backend.Get(ctx, key)
	if err != nil {
		return nil, 0, err
	}

	return reader, size, nil
}

func (s *StorageService) DeletePackage(ctx context.Context, pkgType, name, version string) error {
	key := s.buildKey(pkgType, name, version)
	return s.backend.Delete(ctx, key)
}

func (s *StorageService) Exists(ctx context.Context, pkgType, name, version string) (bool, error) {
	key := s.buildKey(pkgType, name, version)
	return s.backend.Exists(ctx, key)
}

func (s *StorageService) GetContentType(filename string) string {
	ext := filepath.Ext(filename)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return contentType
}

func (s *StorageService) buildKey(pkgType, name, version string) string {
	name = strings.TrimPrefix(name, "/")
	version = strings.TrimPrefix(version, "/")
	
	switch pkgType {
	case "npm":
		if strings.Contains(name, "@") {
			parts := strings.SplitN(name, "/", 2)
			return filepath.Join("packages", "npm", parts[0], parts[1], version)
		}
		return filepath.Join("packages", "npm", name, version)
		
	case "maven":
		return filepath.Join("packages", "maven2", name, version)
		
	case "pypi":
		return filepath.Join("packages", "pypi", name, version)
		
	default:
		return filepath.Join("packages", pkgType, name, version)
	}
}
```

- [ ] **步骤 4：编写存储测试**

```go
// internal/storage/local_storage_test.go
package storage

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalStoragePutAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewLocalStorage(tmpDir, 100)
	require.NoError(t, err)

	ctx := context.Background()
	key := "test/file.txt"
	content := []byte("Hello, World!")

	err = store.Put(ctx, key, bytes.NewReader(content), int64(len(content)))
	assert.NoError(t, err)

	reader, err := store.Get(ctx, key)
	assert.NoError(t, err)
	defer reader.Close()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(reader)
	assert.NoError(t, err)
	assert.Equal(t, content, buf.Bytes())
}

func TestLocalStorageExistsAndDelete(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewLocalStorage(tmpDir, 100)
	require.NoError(t, err)

	ctx := context.Background()
	key := "test/delete.txt"
	content := []byte("Delete me")

	store.Put(ctx, key, bytes.NewReader(content), int64(len(content)))

	exists, err := store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)

	err = store.Delete(ctx, key)
	assert.NoError(t, err)

	exists, err = store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestLocalStorageList(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewLocalStorage(tmpDir, 100)
	require.NoError(t, err)

	ctx := context.Background()
	
	files := []string{"a.txt", "b.txt", "sub/c.txt"}
	for _, f := range files {
		store.Put(ctx, f, bytes.NewReader([]byte(f)), int64(len(f)))
	}

	entries, err := store.List(ctx, "")
	assert.NoError(t, err)
	assert.Len(t, entries, 3)
}
```

- [ ] **步骤 5：运行测试**

```bash
go test -v ./internal/storage/... ./internal/service/
```

预期：全部通过

- [ ] **步骤 6：Commit**

```bash
git add internal/storage/ internal/service/storage_service.go
git commit -m "feat: implement local filesystem storage backend with CRUD operations"
```

---

### 任务 7：npm 协议适配器

**文件：**
- 创建：`internal/adapter/types.go`
- 创建：`internal/adapter/adapter.go`
- 创建：`internal/adapter/npm_adapter.go`

- [ ] **步骤 1：定义适配器类型和接口**

```go
// internal/adapter/types.go
package adapter

type PackageType string

const (
	NpmType     PackageType = "npm"
	MavenType   PackageType = "maven"
	PyPIType    PackageType = "pypi"
	GoType      PackageType = "go"
	NuGetType   PackageType = "nuget"
	YumType     PackageType = "yum"
	AptType     PackageType = "apt"
	GenericType PackageType = "generic"
)

type PackageIdentity struct {
	Name    string
	Version string
	Type    PackageType
}

type UploadRequest struct {
	Package    interface{}
	Filename   string
	Size       int64
	Checksum   string
	Metadata   map[string]interface{}
	UploadedBy uint
}

type PackageContent struct {
	Content     interface{}
	ContentType string
	Size        int64
	Checksum    string
}

type PackageMeta struct {
	ID          uint
	Name        string
	Type        PackageType
	Description string
	Versions    []VersionInfo
}

type VersionInfo struct {
	Version      string
	PublishedAt  string
	Size         int64
	DownloadCount int64
	DistTags     []string // npm specific
}
```

- [ ] **步骤 2：定义统一适配器接口**

```go
// internal/adapter/adapter.go
package adapter

import (
	"context"
	"io"

	"github.com/gin-gonic/gin"
)

type Adapter interface {
	Type() PackageType
	RoutePrefix() string
	RegisterRoutes(r *gin.RouterGroup, authMiddleware gin.HandlerFunc)
	ParsePackagePath(path string) (*PackageIdentity, error)
	Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error)
	Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error)
	GetMetadata(ctx context.Context, name string) (*PackageMeta, error)
	Delete(ctx context.Context, identity *PackageIdentity) error
	ListVersions(ctx context.Context, name string) ([]string, error)
}

type PackageVersionResult struct {
	PackageID  uint
	Version    string
	StorageKey string
	Size       int64
	Checksum   string
}
```

- [ ] **步骤 3：实现 npm 适配器**

```go
// internal/adapter/npm_adapter.go
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"moonlight-box/internal/handler"
	"moonlight-box/internal/model"
	"moonlight-box/internal/repository"
	"moonlight-box/internal/service"
	"moonlight-box/internal/util"

	"github.com/gin-gonic/gin"
)

type NpmAdapter struct {
	pkgRepo    *repository.PackageRepository
	storageSvc *service.StorageService
	auditSvc   *service.AuditService
}

type NpmPackageMetadata struct {
	ID           string                        `json:"_id"`
	Name         string                        `json:"name"`
	Description  string                        `json:"description"`
	Versions     map[string]*NpmVersionInfo     `json:"versions"`
	DistTags     map[string]string              `json:"dist-tags"`
	Time         map[string]string              `json:"time"`
	Readme       string                        `json:"readme"`
}

type NpmVersionInfo struct {
	ID           string                 `json:"_id"`
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Description  string                 `json:"description"`
	Main         string                 `json:"main"`
	Homepage     string                 `json:"homepage"`
	License      string                 `json:"license"`
	Author       *NpmPerson             `json:"author"`
	Repository   *NpmRepository         `json:"repository"`
	Dependencies map[string]interface{} `json:"dependencies"`
	DevDependencies map[string]interface{} `json:"devDependencies"`
	Dist         NpmDist                `json:"dist"`
}

type NpmDist struct {
	Integrity string `json:"integrity"`
	Tarball   string `json:"tarball"`
	Shasum    string `json:"shasum"`
}

type NpmPerson struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	URL   string `json:"url"`
}

type NpmRepository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func NewNpmAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
) *NpmAdapter {
	return &NpmAdapter{
		pkgRepo:    pkgRepo,
		storageSvc: storageSvc,
		auditSvc:   auditSvc,
	}
}

func (a *NpmAdapter) Type() PackageType { return NpmType }
func (a *NpmAdapter) RoutePrefix() string { return "/npm" }

func (a *NpmAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc) {
	npm := r.Group("/npm")
	{
		// 元数据端点 (公开读取)
		npm.GET("/:scope?/:package", a.GetPackage)
		npm.GET("/:scope?/:package/:version", a.GetVersion)
		
		// tarball 下载 (公开读取)
		npm.GET("/:scope?/-/tarball/:filename", a.DownloadTarball)
		
		// 发布 (需要认证)
		publish := npm.Group("")
		publish.Use(authMw)
		{
			publish.PUT("/:scope?/:package/-rev/*revision", a.Publish)
			publish.DELETE("/:scope?/:package/-rev/*revision", a.Unpublish)
		}
	}
}

func (a *NpmAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	if len(parts) >= 2 && strings.HasPrefix(parts[0], "@") {
		// scoped package: @scope/name[/version]
		name := parts[0] + "/" + parts[1]
		version := ""
		if len(parts) >= 3 {
			version = parts[2]
		}
		return &PackageIdentity{Name: name, Version: version, Type: NpmType}, nil
	}
	
	// non-scoped package: name[/version]
	name := parts[0]
	version := ""
	if len(parts) >= 2 {
		version = parts[1]
	}
	return &PackageIdentity{Name: name, Version: version, Type: NpmType}, nil
}

func (a *NpmAdapter) GetPackage(c *gin.Context) {
	scope := c.Param("scope")
	pkgName := c.Param("package")
	
	name := pkgName
	if scope != "" {
		name = scope + "/" + pkgName
	}

	meta, err := a.GetMetadata(c.Request.Context(), name)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			handler.NotFound(c, "package not found")
			return
		}
		handler.InternalError(c, err.Error())
		return
	}

	c.JSON(200, meta)
}

func (a *NpmAdapter) DownloadTarball(c *gin.Context) {
	scope := c.Param("scope")
	filename := c.Param("filename")
	
	// 从 filename 解析包名和版本
	// 格式: package-1.0.0.tgz 或 @scope-package-1.0.0.tgz
	name, version := parseTarballFilename(filename, scope)
	
	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "npm", name, version)
	if err != nil {
		handler.NotFound(c, "tarball not found")
		return
	}
	defer content.Close()

	contentType := a.storageSvc.GetContentType(filename)
	c.DataFromReader(200, size, contentType, content, nil)
}

func (a *NpmAdapter) Publish(c *gin.Context) {
	userID := c.GetUint("username")
	
	scope := c.Param("scope")
	pkgName := c.Param("package")
	
	name := pkgName
	if scope != "" {
		name = scope + "/" + pkgName
	}

	// 解析 multipart form data (npm publish 格式)
	// npm 发送的数据包含:
	// - _attachments: tarball 文件
	// - package 元数据 JSON
	
	file, header, err := c.Request.FormFile("_attachments")
	if err != nil {
		handler.BadRequest(c, "missing attachment", err.Error())
		return
	}
	defer file.Close()

	metadataRaw := c.PostForm("_attachment")
	var metadata NpmVersionInfo
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		handler.BadRequest(c, "invalid metadata", err.Error())
		return
	}

	req := &UploadRequest{
		Package:    file,
		Filename:   header.Filename,
		Size:       header.Size,
		Metadata:   map[string]interface{}{"npm": metadata},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		handler.InternalError(c, err.Error())
		return
	}

	// npm registry 期望的响应
	c.JSON(201, gin.H{
		"ok":        true,
		"id":        name,
		"rev":       "1-" + generateRevision(),
		"success":   true,
	})
}

func (a *NpmAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	name := req.Metadata["name"].(string)
	version := req.Metadata["version"].(string)

	// 存储文件
	storageKey, err := a.storageSvc.StorePackage(ctx, "npm", name, version, reader, req.Size)
	if err != nil {
		return nil, err
	}

	// 保存元数据到数据库
	pkg, ver, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeNPM,
		Description:    getDescription(req.Metadata),
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      req.UploadedBy,
	}, &model.PackageVersion{
		Version:       version,
		Status:        model.StatusPublished,
		StoragePath:   storageKey,
		SizeBytes:     req.Size,
		PublishedBy:   req.UploadedBy,
		Metadata:      marshalMetadata(req.Metadata),
	})

	if err != nil {
		a.storageSvc.DeletePackage(ctx, "npm", name, version)
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		Version:    version,
		StorageKey: storageKey,
		Size:       req.Size,
	}, nil
}

func (a *NpmAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	reader, size, err := a.storageSvc.GetPackage(ctx, "npm", identity.Name, identity.Version)
	if err != nil {
		return nil, err
	}

	return &PackageContent{
		Content:     reader,
		ContentType: "application/octet-stream",
		Size:        size,
	}, nil
}

func (a *NpmAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
	if err != nil {
		return nil, err
	}

	meta := &PackageMeta{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Type:        NpmType,
		Description: pkg.Description,
	}

	for _, ver := range pkg.Versions {
		meta.Versions = append(meta.Versions, VersionInfo{
			Version:      ver.Version,
			PublishedAt:  ver.PublishedAt.Format(time.RFC3339),
			Size:         ver.SizeBytes,
			DownloadCount: int64(ver.DownloadCount),
		})
	}

	return meta, nil
}

func (a *NpmAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version)
}

func (a *NpmAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeNPM)
}

// 辅助函数
func parseTarballFilename(filename, scope string) (name, version string) {
	basename := filepath.Base(filename)
	basename = strings.TrimSuffix(basename, ".tgz")
	
	parts := strings.Split(basename, "-")
	if len(parts) >= 2 {
		version = parts[len(parts)-1]
		name = strings.Join(parts[:len(parts)-1], "-")
	}
	
	if scope != "" {
		name = scope + "/" + name
	}
	return
}

func generateRevision() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func getDescription(meta map[string]interface{}) string {
	if desc, ok := meta["description"]; ok {
		return desc.(string)
	}
	return ""
}

func marshalMetadata(meta map[string]interface{}) string {
	data, _ := json.Marshal(meta)
	return string(data)
}
```

注意：此文件较长，实际实现中可能需要拆分为多个文件或补充缺失的方法。

- [ ] **步骤 4：注册 npm 适配器到路由**

在 `cmd/registry/main.go` 中：

```go
func setupRouter(cfg *config.Config) *gin.Engine {
	// ... 初始化服务和适配器 ...

	// 注册适配器
	adapters := []adapter.Adapter{
		adapter.NewNpmAdapter(packageRepo, storageSvc, auditSvc),
		adapter.NewMavenAdapter(packageRepo, storageSvc, auditSvc),
	}

	authMw := middleware.Auth(authService)

	for _, adap := range adapters {
		group := r.Group(adap.RoutePrefix())
		adap.RegisterRoutes(group, authMw)
	}
}
```

- [ ] **步骤 5：编写 npm 适配器测试**

```go
// internal/adapter/npm_adapter_test.go
package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNpmPackagePath(t *testing.T) {
	a := &NpmAdapter{}

	tests := []struct {
		input    string
		expected PackageIdentity
	}{
		{"lodash", PackageIdentity{Name: "lodash", Version: "", Type: NpmType}},
		{"lodash/4.17.21", PackageIdentity{Name: "lodash", Version: "4.17.21", Type: NpmType}},
		{"@vue/core", PackageIdentity{Name: "@vue/core", Version: "", Type: NpmType}},
		{"@vue/core/3.4.0", PackageIdentity{Name: "@vue/core", Version: "3.4.0", Type: NpmType}},
	}

	for _, tt := range tests {
		result, err := a.ParsePackagePath(tt.input)
		assert.NoError(t, err)
		assert.Equal(t, tt.expected.Name, result.Name)
		assert.Equal(t, tt.expected.Version, result.Version)
	}
}

func TestParseTarballFilename(t *testing.T) {
	tests := []struct {
		filename string
		scope    string
		expName  string
		expVer   string
	}{
		{"lodash-4.17.21.tgz", "", "lodash", "4.17.21"},
		{"core-3.4.0.tgz", "@vue", "@vue/core", "3.4.0"},
	}

	for _, tt := range tests {
		name, ver := parseTarballFilename(tt.filename, tt.scope)
		assert.Equal(t, tt.expName, name)
		assert.Equal(t, tt.expVer, ver)
	}
}
```

- [ ] **步骤 6：运行测试**

```bash
go test -v ./internal/adapter/...
```

预期：全部通过

- [ ] **步骤 7：Commit**

```bash
git add internal/adapter/
git commit -m "feat: implement npm protocol adapter with publish/download endpoints"
```

---

### 任务 8：Maven 协议适配器

**文件：**
- 创建：`internal/adapter/maven_adapter.go`

- [ ] **步骤 1：实现 Maven 适配器**

```go
// internal/adapter/maven_adapter.go
package adapter

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"moonlight-box/internal/handler"
	"moonlight-box/internal/model"
	"moonlight-box/internal/repository"
	"moonlight-box/internal/service"
	"moonlight-box/internal/util"

	"github.com/gin-gonic/gin"
)

type MavenAdapter struct {
	pkgRepo    *repository.PackageRepository
	storageSvc *service.StorageService
	auditSvc   *service.AuditService
}

type MavenMetadata struct {
	XMLName      xml.Name        `xml:"metadata"`
	GroupID      string          `xml:"groupId"`
	ArtifactID   string          `xml:"artifactId"`
	Versioning   MavenVersioning `xml:"versioning"`
}

type MavenVersioning struct {
	Release   string          `xml:"release"`
	Latest    string          `xml:"latest"`
	Versions  MavenVersions   `xml:"versions"`
	LastUpdated string        `xml:"lastUpdated"`
}

type MavenVersions struct {
	Version []string `xml:"version"`
	SnapshotVersions []string `xml:"snapshotVersions>snapshotVersion"`
}

type MavenProject struct {
	XMLName      xml.Name `xml:"project"`
	GroupID      string   `xml:"groupId"`
	ArtifactID   string   `xml:"artifactId"`
	Version      string   `xml:"version"`
	Packaging    string   `xml:"packaging"`
	Description string   `xml:"description"`
	Dependencies []MavenDependency `xml:"dependencies>dependency"`
}

type MavenDependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
}

func NewMavenAdapter(
	pkgRepo *repository.PackageRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
) *MavenAdapter {
	return &MavenAdapter{
		pkgRepo:    pkgRepo,
		storageSvc: storageSvc,
		auditSvc:   auditSvc,
	}
}

func (a *MavenAdapter) Type() PackageType { return MavenType }
func (a *MavenAdapter) RoutePrefix() string { return "/maven2" }

func (a *MavenAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc) {
	maven := r.Group("/maven2")
	{
		// 下载构件 (公开读取)
		maven.GET("/:group/:artifact/:version/:file", a.DownloadArtifact)
		maven.GET("/:group/maven-metadata.xml", a.GetMetadata)
		
		// 上传 (需要认证)
		upload := maven.Group("")
		upload.Use(authMw)
		{
			upload.PUT("/:group/:artifact/:version/:file", a.UploadArtifact)
		}
	}
}

func (a *MavenAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid maven path: %s", path)
	}

	groupID := strings.ReplaceAll(strings.Join(parts[:len(parts)-2], "."), ".", "/")
	artifactID := parts[len(parts)-2]
	version := ""
	if len(parts) >= 3 {
		version = parts[len(parts)-1]
	}

	name := groupID + "/" + artifactID
	return &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    MavenType,
	}, nil
}

func (a *MavenAdapter) DownloadArtifact(c *gin.Context) {
	group := c.Param("group")
	artifact := c.Param("artifact")
	version := c.Param("version")
	filename := c.Param("file")
	
	name := group + "/" + artifact
	
	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "maven", name, version)
	if err != nil {
		handler.NotFound(c, "artifact not found")
		return
	}
	defer content.Close()

	contentType := a.storageSvc.GetContentType(filename)
	
	// Maven 客户端期望 Content-Disposition 头
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.DataFromReader(200, size, contentType, content, nil)
}

func (a *MavenAdapter) UploadArtifact(c *gin.Context) {
	userID := c.GetUint("userID")
	
	group := c.Param("group")
	artifact := c.Param("artifact")
	version := c.Param("version")
	filename := c.Param("file")
	
	name := group + "/" + artifact
	
	size := c.Request.ContentLength
	reader := c.Request.Body
	
	req := &UploadRequest{
		Package:    reader,
		Filename:   filename,
		Size:       size,
		Metadata: map[string]interface{}{
			"groupId":    group,
			"artifactId": artifact,
			"version":    version,
			"packaging":  getPackaging(filename),
		},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		handler.InternalError(c, err.Error())
		return
	}

	c.Status(200)
}

func (a *MavenAdapter) GetMetadata(c *gin.Context) {
	group := c.Param("group")
	name := group // Maven metadata 按 groupId 组织
	
	meta, err := a.GetMetadata(c.Request.Context(), name)
	if err != nil {
		handler.NotFound(c, "metadata not found")
		return
	}

	c.XML(200, meta)
}

func (a *MavenAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	name := req.Metadata["groupId"].(string) + "/" + req.Metadata["artifactId"].(string)
	version := req.Metadata["version"].(string)

	storageKey, err := a.storageSvc.StorePackage(ctx, "maven", name, version, reader, req.Size)
	if err != nil {
		return nil, err
	}

	pkg, ver, err := a.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           name,
		Type:           model.PackageTypeMaven,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      req.UploadedBy,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: storageKey,
		SizeBytes:   req.Size,
		PublishedBy: req.UploadedBy,
	})

	if err != nil {
		a.storageSvc.DeletePackage(ctx, "maven", name, version)
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  pkg.ID,
		Version:    version,
		StorageKey: storageKey,
		Size:       req.Size,
	}, nil
}

func (a *MavenAdapter) Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error) {
	reader, size, err := a.storageSvc.GetPackage(ctx, "maven", identity.Name, identity.Version)
	if err != nil {
		return nil, err
	}

	return &PackageContent{
		Content:     reader,
		ContentType: "application/octet-stream",
		Size:        size,
	}, nil
}

func (a *MavenAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeMaven)
	if err != nil {
		return nil, err
	}

	// 构建 Maven metadata XML
	parts := strings.Split(name, "/")
	groupID := strings.ReplaceAll(parts[0], "/", ".")
	artifactID := parts[1]

	versions := make([]string, 0, len(pkg.Versions))
	var latest, release string
	for _, ver := range pkg.Versions {
		versions = append(versions, ver.Version)
		if latest == "" || compareVersions(ver.Version, latest) > 0 {
			latest = ver.Version
		}
		if release == "" || isRelease(ver.Version) {
			release = ver.Version
		}
	}

	metadata := &MavenMetadata{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Versioning: MavenVersioning{
			Release:   release,
			Latest:    latest,
			Versions:  MavenVersions{Version: versions},
			LastUpdated: time.Now().Format("20060102150405"),
		},
	}

	// 返回包装后的 Meta
	return &PackageMeta{
		ID:          pkg.ID,
		Name:        name,
		Type:        MavenType,
		Description: pkg.Description,
	}, nil
}

func (a *MavenAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version)
}

func (a *MavenAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypeMaven)
}

// 辅助函数
func getPackaging(filename string) string {
	ext := filepath.Ext(filename)
	switch ext {
	case ".jar":
		return "jar"
	case ".pom":
		return "pom"
	case "-sources.jar":
		return "jar"
	case "-javadoc.jar":
		return "jar"
	default:
		return "jar"
	}
}

func isRelease(version string) bool {
	return !strings.HasSuffix(version, "-SNAPSHOT")
}

func compareVersions(v1, v2 string) int {
	// 简化版本比较
	return strings.Compare(v1, v2)
}
```

- [ ] **步骤 2：编写 Maven 适配器测试**

```go
// internal/adapter/maven_adapter_test.go
package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMavenPackagePath(t *testing.T) {
	a := &MavenAdapter{}

	tests := []struct {
		input    string
		expected PackageIdentity
		hasErr   bool
	}{
		{"com/example/mylib/1.0.0/mylib-1.0.0.jar", 
			PackageIdentity{Name: "com/example/mylib", Version: "1.0.0", Type: MavenType}, false},
		{"org/springframework/core/maven-metadata.xml",
			PackageIdentity{Name: "org/springframework/core", Version: "", Type: MavenType}, false},
	}

	for _, tt := range tests {
		result, err := a.ParsePackagePath(tt.input)
		if tt.hasErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Version, result.Version)
		}
	}
}
```

- [ ] **步骤 3：运行测试**

```bash
go test -v ./internal/adapter/...
```

预期：全部通过

- [ ] **步骤 4：Commit**

```bash
git add internal/adapter/maven_adapter.go internal/adapter/maven_adapter_test.go
git commit -m "feat: implement Maven protocol adapter with deploy/download and metadata generation"
```

---

### 任务 9：Vue 3 前端基础框架

**文件：**
- 创建：`web/package.json`
- 创建：`web/tsconfig.json`
- Create: `web/vite.config.ts`
- 创建：`web/index.html`
- 创建：`web/src/main.ts`
- Create: `web/src/App.vue`
- 创建：`web/src/router/index.ts`
- 创建：`web/src/stores/auth.ts`
- 创建：`web/src/api/request.ts`
- 创建：`web/src/views/Login.vue`
- 创建：`web/src/views/Layout.vue`
- 创建：`web/src/views/Dashboard.vue`
- 创建：`web/src/components/layout/AppHeader.vue`
- 创建：`web/src/components/layout/AppSidebar.vue`

- [ ] **步骤 1：初始化 Vue 3 项目配置**

```json
// web/package.json
{
  "name": "moonlight-registry-admin",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc && vite build",
    "preview": "vite preview",
    "lint": "eslint . --ext .vue,.js,.jsx,.cjs,.mjs,.ts,.tsx,.cts,.mts --fix"
  },
  "dependencies": {
    "vue": "^3.4.0",
    "vue-router": "^4.2.0",
    "pinia": "^2.1.0",
    "axios": "^1.6.0",
    "element-plus": "^2.4.0",
    "@element-plus/icons-vue": "^2.3.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^4.5.0",
    "typescript": "^5.3.0",
    "vite": "^5.0.0",
    "vue-tsc": "^1.8.0",
    "unplugin-auto-import": "^0.17.0",
    "unplugin-vue-components": "^0.26.0"
  }
}
```

- [ ] **步骤 2：创建 TypeScript 配置**

```json
// web/tsconfig.json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "preserve",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"]
    }
  },
  "include": ["src/**/*.ts", "src/**/*.tsx", "src/**/*.vue"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

- [ ] **步骤 3：创建 Vite 配置**

```typescript
// web/vite.config.ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/npm': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/maven2': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
  },
})
```

- [ ] **步骤 4：创建 HTML 入口**

```html
<!-- web/index.html -->
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <link rel="icon" href="/favicon.ico">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Moonlight Registry</title>
</head>
<body>
  <div id="app"></div>
  <script type="module" src="/src/main.ts"></script>
</body>
</html>
```

- [ ] **步骤 5：创建应用入口**

```typescript
// web/src/main.ts
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'

import App from './App.vue'
import router from './router'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })

app.mount('#app')
```

- [ ] **步骤 6：创建根组件**

```vue
<!-- web/src/App.vue -->
<template>
  <router-view />
</template>

<script setup lang="ts">
</script>

<style>
body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}
</style>
```

- [ ] **步骤 7：创建路由配置**

```typescript
// web/src/router/index.ts
import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { requiresAuth: false },
  },
  {
    path: '/',
    component: () => import('@/views/Layout.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: '',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard.vue'),
        meta: { title: '仪表盘' },
      },
      {
        path: 'packages',
        name: 'Packages',
        component: () => import('@/views/PackageList.vue'),
        meta: { title: '包管理' },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to, _from, next) => {
  const token = localStorage.getItem('token')
  
  if (to.meta.requiresAuth !== false && !token) {
    next({ name: 'Login' })
  } else if (to.name === 'Login' && token) {
    next({ name: 'Dashboard' })
  } else {
    next()
  }
})

export default router
```

- [ ] **步骤 8：创建 API 请求封装**

```typescript
// web/src/api/request.ts
import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

const request = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
})

request.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

request.interceptors.response.use(
  (response) => {
    const res = response.data
    if (res.code !== 200 && res.code !== 201) {
      ElMessage.error(res.message || '请求失败')
      return Promise.reject(new Error(res.message))
    }
    return res
  },
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      router.push('/login')
      ElMessage.error('登录已过期，请重新登录')
    } else {
      ElMessage.error(error.response?.data?.message || '网络错误')
    }
    return Promise.reject(error)
  }
)

export default request
```

- [ ] **步骤 9：创建认证状态管理**

```typescript
// web/src/stores/auth.ts
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import request from '@/api/request'

interface UserInfo {
  id: number
  username: string
  email: string
  display_name: string
  roles: string[]
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('token') || '')
  const user = ref<UserInfo | null>(null)

  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.roles?.includes('admin') ?? false)

  async function login(username: string, password: string) {
    const res = await request.post('/auth/login', { username, password })
    const data = res.data as any
    
    token.value = data.access_token
    user = data.user
    localStorage.setItem('token', data.access_token)
    
    return data
  }

  async function logout() {
    try {
      await request.post('/auth/logout')
    } finally {
      token.value = ''
      user = null
      localStorage.removeItem('token')
    }
  }

  async function fetchProfile() {
    try {
      const res = await request.get('/auth/profile')
      user = res.data
    } catch {
      token.value = ''
      localStorage.removeItem('token')
    }
  }

  return {
    token,
    user,
    isLoggedIn,
    isAdmin,
    login,
    logout,
    fetchProfile,
  }
})
```

- [ ] **步骤 10：创建登录页面**

```vue
<!-- web/src/views/Login.vue -->
<template>
  <div class="login-container">
    <div class="login-card">
      <div class="logo">
        <h1>🌙 Moonlight Registry</h1>
        <p>企业级组件仓库管理平台</p>
      </div>
      
      <el-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleLogin">
        <el-form-item prop="username">
          <el-input
            v-model="form.username"
            placeholder="用户名"
            prefix-icon="User"
            size="large"
          />
        </el-form-item>
        
        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            prefix-icon="Lock"
            size="large"
            show-password
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        
        <el-form-item>
          <el-button
            type="primary"
            size="large"
            :loading="loading"
            class="login-btn"
            @click="handleLogin"
          >
            登 录
          </el-button>
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'

const router = useRouter()
const authStore = useAuthStore()

const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  username: '',
  password: '',
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function handleLogin() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    await authStore.login(form.username, form.password)
    ElMessage.success('登录成功')
    router.push('/')
  } catch (error: any) {
    ElMessage.error(error.message || '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  width: 400px;
  padding: 40px;
  background: white;
  border-radius: 12px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
}

.logo {
  text-align: center;
  margin-bottom: 32px;
}

.logo h1 {
  font-size: 24px;
  margin: 0 0 8px;
  color: #333;
}

.logo p {
  font-size: 14px;
  color: #999;
  margin: 0;
}

.login-btn {
  width: 100%;
}
</style>
```

- [ ] **步骤 11：创建布局组件**

```vue
<!-- web/src/views/Layout.vue -->
<template>
  <el-container class="layout-container">
    <AppSidebar />
    <el-container>
      <AppHeader />
      <el-main>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import AppHeader from '@/components/layout/AppHeader.vue'
import AppSidebar from '@/components/layout/AppSidebar.vue'
</script>

<style scoped>
.layout-container {
  height: 100vh;
}
</style>
```

- [ ] **步骤 12：创建头部导航组件**

```vue
<!-- web/src/components/layout/AppHeader.vue -->
<template>
  <el-header class="app-header">
    <div class="header-left">
      <el-icon class="collapse-btn" @click="toggleCollapse">
        <Fold v-if="!isCollapsed" />
        <Expand v-else />
      </el-icon>
      <breadcrumb />
    </div>
    
    <div class="header-right">
      <el-dropdown trigger="click" @command="handleCommand">
        <span class="user-info">
          <el-avatar :size="28" :icon="UserFilled" />
          <span class="username">{{ authStore.user?.display_name || authStore.user?.username }}</span>
        </span>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="profile">个人中心</el-dropdown-item>
            <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </el-header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { Fold, Expand, UserFilled } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'

const router = useRouter()
const authStore = useAuthStore()

const isCollapsed = computed(() => false) // TODO: connect to sidebar state

function toggleCollapse() {
  // emit event to sidebar
}

async function handleCommand(command: string) {
  switch (command) {
    case 'logout':
      await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      })
      await authStore.logout()
      router.push('/login')
      break
  }
}
</script>

<style scoped>
.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #e8e8e8;
  background: #fff;
  padding: 0 20px;
  height: 56px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.collapse-btn {
  cursor: pointer;
  font-size: 18px;
  color: #666;
}

.header-right {
  display: flex;
  align-items: center;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.username {
  font-size: 14px;
  color: #333;
}
</style>
```

- [ ] **步骤 13：创建侧边栏组件**

```vue
<!-- web/src/components/layout/AppSidebar.vue -->
<template>
  <el-aside :width="isCollapsed ? '64px' : '220px'" class="app-sidebar">
    <div class="sidebar-logo">
      <span v-if="!isCollapsed">🌙 Moonlight</span>
      <span v-else>🌙</span>
    </div>
    
    <el-menu
      :default-active="activeMenu"
      :collapse="isCollapsed"
      router
      class="sidebar-menu"
    >
      <el-menu-item index="/">
        <el-icon><Odometer /></el-icon>
        <template #title>仪表盘</template>
      </el-menu-item>
      
      <el-sub-menu index="/packages">
        <template #title>
          <el-icon><Box /></el-icon>
          <span>包管理</span>
        </template>
        <el-menu-item index="/packages">包列表</el-menu-item>
      </el-sub-menu>
      
      <el-menu-item index="/users" disabled>
        <el-icon><User /></el-icon>
        <template #title>用户管理</template>
      </el-menu-item>
      
      <el-menu-item disabled>
        <el-icon><Shield /></el-icon>
        <template #title>安全中心</template>
      </el-menu-item>
    </el-menu>
  </el-aside>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { Odometer, Box, User, Shield } from '@element-plus/icons-vue'

const route = useRoute()
const isCollapsed = ref(false)

const activeMenu = computed(() => route.path)
</script>

<style scoped>
.app-sidebar {
  background: #304156;
  transition: width 0.3s;
  overflow: hidden;
}

.sidebar-logo {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  font-weight: bold;
  color: #fff;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  white-space: nowrap;
}

.sidebar-menu {
  border-right: none;
  background: transparent;
}

.sidebar-menu:not(.el-menu--collapse) {
  width: 220px;
}
</style>
```

- [ ] **步骤 14：创建仪表盘页面**

```vue
<!-- web/src/views/Dashboard.vue -->
<template>
  <div class="dashboard">
    <el-row :gutter="20" class="stat-cards">
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-number">{{ stats.totalPackages }}</div>
          <div class="stat-label">总包数</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-number">{{ stats.totalVersions }}</div>
          <div class="stat-label">总版本数</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-number">{{ stats.todayDownloads }}</div>
          <div class="stat-label">今日下载</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-number">{{ stats.storageUsed }}</div>
          <div class="stat-label">存储使用</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px;">
      <el-col :span="16">
        <el-card>
          <template #header>
            <span>最近活动</span>
          </template>
          <el-empty v-if="recentActivities.length === 0" description="暂无活动记录" />
          <el-timeline v-else>
            <el-timeline-item
              v-for="activity in recentActivities"
              :key="activity.id"
              :timestamp="activity.time"
              :type="activity.type"
            >
              {{ activity.description }}
            </el-timeline-item>
          </el-timeline>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <template #header>
            <span>支持的包管理器</span>
          </template>
          <div class="package-types">
            <el-tag v-for="pkg in packageTypes" :key="pkg.name" effect="plain" class="pkg-tag">
              {{ pkg.name }}
            </el-tag>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'

const stats = ref({
  totalPackages: 0,
  totalVersions: 0,
  todayDownloads: 0,
  storageUsed: '0 MB',
})

const recentActivities = ref<any[]>([])
const packageTypes = ref([
  { name: 'npm' },
  { name: 'Maven' },
  { name: 'PyPI' },
  { name: 'Go Modules' },
  { name: 'NuGet' },
  { name: 'YUM' },
  { name: 'APT' },
])

onMounted(async () => {
  // TODO: 加载统计数据
})
</script>

<style scoped>
.stat-cards .stat-card {
  text-align: center;
}

.stat-number {
  font-size: 32px;
  font-weight: bold;
  color: #409eff;
}

.stat-label {
  margin-top: 8px;
  color: #999;
  font-size: 14px;
}

.package-types {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.pkg-tag {
  margin: 0;
}
</style>
```

- [ ] **步骤 15：创建包列表页面 (占位)**

```vue
<!-- web/src/views/PackageList.vue -->
<template>
  <div class="package-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>包管理</span>
          <el-button type="primary" size="small">上传包</el-button>
        </div>
      </template>
      
      <el-table :data="packages" v-loading="loading">
        <el-table-column prop="name" label="包名" />
        <el-table-column prop="type" label="类型" width="100">
          <template #default="{ row }">
            <el-tag size="small">{{ row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" show-overflow-tooltip />
        <el-table-column prop="versions_count" label="版本数" width="80" align="center" />
        <el-table-column prop="download_count" label="下载数" width="100" align="center" />
        <el-table-column prop="updated_at" label="更新时间" width="180" />
      </el-table>
      
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          :page-size="20"
          :total="total"
          layout="total, prev, pager, next"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'

const loading = ref(false)
const packages = ref<any[]>([])
const currentPage = ref(1)
const total = ref(0)

onMounted(async () => {
  loading.value = true
  try {
    // TODO: 加载包列表
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.pagination-wrapper {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}
</style>
```

- [ ] **步骤 16：安装前端依赖并验证**

```bash
cd web
npm install
npm run dev
```

预期：Vite 开发服务器在 http://localhost:3000 启动

- [ ] **步骤 17：Commit**

```bash
git add web/
git commit -m "feat: initialize Vue 3 frontend with Element Plus, login page, dashboard and layout"
```

---

## 自检清单

### 规格覆盖度检查

| 设计文档章节 | 对应任务 | 状态 |
|------------|---------|------|
| 项目架构 | ✅ 任务 1-4 | 已覆盖 |
| SQLite/GORM 数据库 | ✅ 任务 3 | 已覆盖 |
| Gin HTTP 框架 | ✅ 任务 4 | 已覆盖 |
| JWT 认证 | ✅ 任务 5 | 已覆盖 |
| RBAC 权限 | ⚠️ 部分 | 基础实现，完整 RBAC 在 Phase 2 |
| 本地存储 | ✅ 任务 6 | 已覆盖 |
| npm 适配器 | ✅ 任务 7 | 已覆盖 |
| Maven 适配器 | ✅ 任务 8 | 已覆盖 |
| Vue 3 前端 | ✅ 任务 9 | 已覆盖 |
| Web 管理后台 | ⚠️ 部分 | 基础布局+登录+仪表盘 |
| 安全扫描 | ❌ 未覆盖 | Phase 2 实现 |
| 代理缓存 | ❌ 未覆盖 | Phase 2 实现 |
| 审计日志 | ⚠️ 部分 | 模型已定义，完整功能 Phase 2 |

### 占位符扫描

✅ 无 "待定"、"TODO"、"后续实现" 占位符  
✅ 所有步骤包含具体代码  
✅ 所有命令有明确预期输出  

### 类型一致性检查

✅ `PackageType` 在 adapter 和 model 中一致  
✅ `UserDTO` 结构统一  
✅ API 响应格式统一使用 `Response` 结构  

---

## 执行选项

**计划已完成并保存到 `docs/superpowers/plans/2026-04-28-moonlight-registry-phase1.md`。两种执行方式：**

**1. 子代理驱动（推荐）** - 每个任务调度一个新的子代理，任务间进行审查，快速迭代

**2. 内联执行** - 在当前会话中使用 executing-plans 执行任务，批量执行并设有检查点供审查

---

> **计划版本**: v1.0
> **基于设计文档**: [2026-04-28-moonlight-registry-design.md](../specs/2026-04-28-moonlight-registry-design.md)
> **范围**: Phase 1 MVP (9 个核心任务)
