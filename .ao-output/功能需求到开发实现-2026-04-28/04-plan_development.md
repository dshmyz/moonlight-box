# Development Plan: Moonlight Registry Phase 1 MVP

**Author**: Leo (全栈开发工程师)  **Date**: 2026-04-28  **Status**: Ready for Review
**Based on**: 需求文档 + 架构设计 + UI/UX 设计规范

---

## 1. Component/Service Split（组件/服务拆分方案）

### 1.1 后端服务拆分

```
backend (Go 单体应用)
├── cmd/registry/
│   └── main.go                    # 应用入口: 解析 CLI 参数 (serve/version)
│
├── internal/
│   ├── config/
│   │   ├── config.go              # 配置结构体定义 + Load() 函数 (viper)
│   │   └── defaults.go            # setDefaults() 所有默认值
│   │
│   ├── model/                     # GORM 数据模型 (纯 struct 定义，无业务逻辑)
│   │   ├── base.go                # BaseModel (ID, CreatedAt, UpdatedAt)
│   │   ├── user.go                # User 模型 + 角色关联
│   │   ├── role.go                # Role + Permission + 关联表
│   │   ├── package.go             # Package + PackageVersion
│   │   └── audit.go               # AuditLog
│   │
│   ├── database/
│   │   ├── database.go            # 数据库连接初始化 + WAL 模式设置
│   │   └── migration.go           # GORM AutoMigrate + 种子数据 (admin 用户)
│   │
│   ├── repository/                # 数据访问层 (仅 DB 操作，无业务逻辑)
│   │   ├── user_repo.go           # CreateUser, GetUserByUsername, ListUsers
│   │   ├── role_repo.go           # GetRoles, AssignRoleToUser
│   │   ├── package_repo.go        # CreatePackage, GetPackage, ListPackages, IncrementDownload
│   │   └── audit_repo.go          # CreateAuditLog, ListAuditLogs
│   │
│   ├── storage/
│   │   ├── backend.go             # StorageBackend 接口定义
│   │   └── local_storage.go       # LocalStorage 实现 (basePath + 原子写入)
│   │
│   ├── adapter/                   # 协议适配器层 (每个 adapter 一个文件)
│   │   ├── adapter.go             # PackageAdapter 统一接口
│   │   ├── types.go               # PackageType 常量 + 请求/响应类型
│   │   ├── npm_adapter.go         # NPMAdapter: 实现 PackageAdapter 接口
│   │   └── maven_adapter.go       # MavenAdapter: 实现 PackageAdapter 接口
│   │
│   ├── service/                   # 业务逻辑层 (组合 repository + storage)
│   │   ├── auth_service.go        # Login, GenerateToken, ValidateToken
│   │   ├── package_service.go     # Upload, Download, GetMetadata, ListPackages
│   │   ├── storage_service.go     # 高层存储操作 (委托给 StorageBackend)
│   │   └── audit_service.go       # LogAction (包装 audit_repo)
│   │
│   ├── handler/                   # HTTP 处理器层 (仅解析请求 + 调用 service + 返回响应)
│   │   ├── response.go            # 统一响应格式 (JSONResponse, ErrorResponse)
│   │   ├── auth_handler.go        # POST /api/v1/auth/login
│   │   ├── admin_handler.go       # GET/POST/PUT /api/v1/users/*, /api/v1/stats/*
│   │   └── package_handler.go     # npm/maven 包操作的 HTTP handler
│   │
│   ├── middleware/                # Gin 中间件
│   │   ├── auth.go                # JWT 验证 + 用户信息注入到 context
│   │   ├── rbac.go                # 基于角色的权限检查
│   │   ├── cors.go                # CORS 处理
│   │   ├── recovery.go            # Panic 恢复
│   │   ├── requestid.go           # 请求 ID 生成与注入
│   │   └── ratelimit.go           # Token Bucket 限流
│   │
│   └── util/                      # 工具函数 (纯函数，无状态)
│       ├── hash.go                # bcrypt HashPassword, CheckPassword
│       ├── errors.go              # 业务错误定义 (ErrNotFound, ErrUnauthorized 等)
│       ├── validator.go           # 输入验证辅助函数
│       └── pagination.go          # 分页参数解析
│
├── configs/
│   └── config.example.yaml        # 配置示例
│
├── go.mod
├── go.sum
├── Makefile
└── .gitignore
```

**每个文件的职责边界**：
- `model/`: 仅定义 GORM struct 和关联关系
- `repository/`: 仅做数据库 CRUD，不处理业务规则
- `service/`: 编排 repository + storage，处理业务规则和验证
- `handler/`: 解析 HTTP 请求参数，调用 service，格式化 HTTP 响应
- `middleware/`: 请求前置处理（认证/权限/日志）
- `adapter/`: 实现特定包管理协议的 HTTP 路由注册和协议解析

### 1.2 前端组件拆分

```
frontend (Vue 3 SPA)
├── src/
│   ├── api/
│   │   ├── request.ts             # Axios 实例 + 拦截器 (JWT 注入 + 401 处理)
│   │   └── auth.ts                # login, logout, getUserInfo API 函数
│   │
│   ├── stores/
│   │   └── auth.ts                # Pinia store: token, user, roles, login/logout actions
│   │
│   ├── router/
│   │   └── index.ts               # 路由配置 + 导航守卫 (auth check + role check)
│   │
│   ├── views/
│   │   ├── Login.vue              # 登录页: 表单 + 登录逻辑
│   │   ├── Layout.vue             # 布局容器: AppSidebar + AppHeader + RouterView
│   │   ├── Dashboard.vue          # 仪表盘: 统计卡片 + ECharts 图表
│   │   ├── PackageList.vue        # 包列表: 搜索/筛选/表格/分页
│   │   ├── PackageDetail.vue      # 包详情: 基本信息 + 版本列表
│   │   ├── UserManagement.vue     # 用户管理: 列表 + 创建/编辑对话框
│   │   └── AuditLogs.vue          # 审计日志: 时间线/表格
│   │
│   ├── components/
│   │   ├── layout/
│   │   │   ├── AppSidebar.vue     # 侧边栏: 菜单渲染 + 角色过滤
│   │   │   └── AppHeader.vue      # 顶部栏: Logo + 用户菜单
│   │   ├── common/
│   │   │   ├── StatCard.vue       # 统计卡片组件 (icon + value + label)
│   │   │   ├── SearchFilter.vue   # 搜索 + 筛选栏组件
│   │   │   ├── DataTable.vue      # 通用数据表格 (分页 + 排序)
│   │   │   └── EmptyState.vue     # 空状态插图组件
│   │   └── package/
│   │       ├── PackageTable.vue   # 包列表表格行渲染
│   │       ├── VersionList.vue    # 版本列表表格
│   │       └── PackageBadge.vue   # 协议类型标签 (npm/maven 颜色区分)
│   │
│   ├── composables/               # 组合式函数 (可复用的业务逻辑)
│   │   ├── useAuth.ts             # 登录/登出逻辑 + token 管理
│   │   ├── usePagination.ts       # 分页状态管理
│   │   ├── useSearch.ts           # 搜索 + debounce 逻辑
│   │   └── usePackage.ts          # 包数据获取 + 缓存
│   │
│   ├── types/                     # TypeScript 类型定义
│   │   ├── api.ts                 # API 响应类型
│   │   ├── package.ts             # Package, PackageVersion 类型
│   │   ├── user.ts                # User, Role 类型
│   │   └── common.ts              # 通用类型 (Pagination, ApiResponse)
│   │
│   ├── styles/
│   │   ├── variables.scss         # SCSS 变量 (颜色、间距)
│   │   └── global.scss            # 全局样式重置
│   │
│   ├── App.vue
│   └── main.ts
│
├── index.html
├── package.json
├── tsconfig.json
└── vite.config.ts
```

---

## 2. Implementation Details（实现细节）

### 2.1 TypeScript 类型定义

```typescript
// types/api.ts
export interface ApiResponse<T = unknown> {
  code: number;
  message: string;
  data: T;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

// types/package.ts
export type PackageType = 'npm' | 'maven';

export interface Package {
  id: number;
  name: string;
  type: PackageType;
  description: string;
  createdBy: string;
  createdAt: string;
  versionCount: number;
  totalDownloads: number;
}

export interface PackageVersion {
  id: number;
  version: string;
  status: 'published' | 'deprecated' | 'yanked';
  sizeBytes: number;
  checksumSha256: string;
  publishedAt: string;
  downloadCount: number;
}

// types/user.ts
export type UserRole = 'admin' | 'publisher' | 'consumer' | 'viewer';

export interface User {
  id: number;
  username: string;
  email: string;
  displayName: string;
  roles: UserRole[];
  isActive: boolean;
  lastLoginAt: string | null;
  createdAt: string;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
  user: User;
}
```

### 2.2 Go 错误处理策略

```go
// util/errors.go - 业务错误定义
var (
    ErrNotFound         = errors.New("resource not found")
    ErrUnauthorized     = errors.New("unauthorized")
    ErrForbidden        = errors.New("insufficient permissions")
    ErrInvalidInput     = errors.New("invalid input")
    ErrPackageExists    = errors.New("package version already exists")
    ErrInvalidToken     = errors.New("invalid or expired token")
    ErrStorageFailed    = errors.New("storage operation failed")
)

// handler/response.go - 统一错误映射
func HandleError(c *gin.Context, err error) {
    var statusCode int
    var message string

    switch {
    case errors.Is(err, util.ErrNotFound):
        statusCode = http.StatusNotFound
        message = "资源不存在"
    case errors.Is(err, util.ErrUnauthorized), errors.Is(err, util.ErrInvalidToken):
        statusCode = http.StatusUnauthorized
        message = "未授权，请重新登录"
    case errors.Is(err, util.ErrForbidden):
        statusCode = http.StatusForbidden
        message = "权限不足"
    case errors.Is(err, util.ErrInvalidInput):
        statusCode = http.StatusBadRequest
        message = err.Error()
    case errors.Is(err, util.ErrPackageExists):
        statusCode = http.StatusConflict
        message = "该版本已存在"
    default:
        statusCode = http.StatusInternalServerError
        message = "服务器内部错误"
        // 记录详细错误日志（不暴露给用户）
        log.Printf("internal error: %v", err)
    }

    c.JSON(statusCode, gin.H{
        "code":    statusCode,
        "message": message,
    })
}
```

**错误处理规则**：
1. 所有错误必须用 `%w` 包装，保留错误链
2. repository 层只返回数据库错误，不返回业务错误
3. service 层转换数据库错误为业务错误
4. handler 层映射业务错误为 HTTP 状态码和用户友好消息
5. 永远不要吞掉错误（空的 catch 或忽略 error return）

### 2.3 前端错误处理策略

```typescript
// api/request.ts - Axios 拦截器
const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('accessToken');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response) {
      const { status, data } = error.response;
      switch (status) {
        case 401:
          // Token 过期，清除并跳转登录
          localStorage.removeItem('accessToken');
          router.push('/login');
          ElMessage.error('登录已过期，请重新登录');
          break;
        case 403:
          ElMessage.error('权限不足');
          break;
        case 404:
          ElMessage.error('资源不存在');
          break;
        case 409:
          ElMessage.warning(data.message || '资源已存在');
          break;
        default:
          ElMessage.error(data.message || '请求失败');
      }
    } else {
      ElMessage.error('网络连接失败');
    }
    return Promise.reject(error);
  }
);
```

### 2.4 测试策略

**后端测试**：

| 层级 | 测试类型 | 工具 | 覆盖目标 |
|------|---------|------|---------|
| repository | 单元测试 | testify + sqlmock | CRUD 正确性 |
| service | 单元测试 | testify + mock | 业务逻辑 + 错误处理 |
| handler | 集成测试 | httptest + gin | HTTP 路由 + 中间件链 |
| adapter | 集成测试 | httptest | 协议兼容性 |

```go
// service/auth_service_test.go 示例
func TestAuthService_Login_Success(t *testing.T) {
    // Arrange
    mockRepo := &mocks.UserRepository{}
    mockRepo.On("GetByUsername", "testuser").Return(&model.User{
        ID:       1,
        Username: "testuser",
        PasswordHash: "$2a$12$...", // bcrypt hash of "password123"
        Roles:    []model.Role{{Name: "admin"}},
    }, nil)

    svc := NewAuthService(mockRepo, []byte("test-secret"))

    // Act
    resp, err := svc.Login("testuser", "password123")

    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, resp.AccessToken)
    assert.Equal(t, "testuser", resp.User.Username)
    mockRepo.AssertExpectations(t)
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
    mockRepo := &mocks.UserRepository{}
    mockRepo.On("GetByUsername", "testuser").Return(&model.User{
        PasswordHash: "$2a$12$...",
    }, nil)

    svc := NewAuthService(mockRepo, []byte("test-secret"))

    resp, err := svc.Login("testuser", "wrongpassword")

    assert.Error(t, err)
    assert.True(t, errors.Is(err, util.ErrUnauthorized))
    assert.Nil(t, resp)
}
```

**前端测试**：

| 组件 | 测试类型 | 工具 | 覆盖目标 |
|------|---------|------|---------|
| Login.vue | 组件测试 | Vitest + Vue Test Utils | 表单验证 + 登录流程 |
| AppSidebar.vue | 单元测试 | Vitest | 角色过滤 + 路由高亮 |
| useAuth.ts | 单元测试 | Vitest + mock | token 管理 + 登出 |
| api/request.ts | 单元测试 | Vitest + msw | 拦截器行为 |

```typescript
// composables/useAuth.spec.ts
import { describe, it, expect, vi } from 'vitest'
import { useAuth } from './useAuth'
import * as authApi from '@/api/auth'

describe('useAuth', () => {
  it('stores token after successful login', async () => {
    vi.spyOn(authApi, 'login').mockResolvedValue({
      accessToken: 'test-token',
      user: { id: 1, username: 'admin', roles: ['admin'] },
    } as any)

    const { login, isAuthenticated } = useAuth()
    await login('admin', 'password')

    expect(isAuthenticated.value).toBe(true)
    expect(localStorage.getItem('accessToken')).toBe('test-token')
  })

  it('clears token on logout', async () => {
    localStorage.setItem('accessToken', 'test-token')
    const { logout, isAuthenticated } = useAuth()
    await logout()

    expect(isAuthenticated.value).toBe(false)
    expect(localStorage.getItem('accessToken')).toBeNull()
  })
})
```

---

## 3. Performance Optimization Checklist（性能优化清单）

### 3.1 后端

- [ ] SQLite 启用 WAL 模式 (`PRAGMA journal_mode=WAL`)
- [ ] 包元数据使用 bigcache 内存缓存（TTL 5 分钟）
- [ ] 包列表查询使用分页（默认 20 条/页），避免全表扫描
- [ ] 数据库索引：`packages(name, type)` 唯一索引、`package_versions(package_id, version)` 唯一索引
- [ ] 文件上传使用流式处理（不一次性读入内存）
- [ ] 代理拉取使用 HTTP/2 连接复用
- [ ] Gin 使用 release 模式运行（生产环境）

### 3.2 前端

- [ ] Vue Router 懒加载：`component: () => import('@/views/Dashboard.vue')`
- [ ] ECharts 按需引入（仅引入用到的图表类型）
- [ ] Element Plus 按需引入（unplugin-vue-components 自动注册）
- [ ] 包列表表格使用虚拟滚动（如果单页 > 100 条）
- [ ] 搜索输入 debounce 300ms，避免频繁 API 调用
- [ ] 统计卡片数字使用 `requestAnimationFrame` 平滑过渡动画
- [ ] 静态资源文件名包含 hash（Vite 默认）

### 3.3 验收标准

| Metric | Target | Measurement |
|--------|--------|-------------|
| 包缓存命中 P99 | < 50ms | wrk 压测 |
| 包上传 P99 | < 500ms | wrk 压测 |
| 前端 FCP | < 300ms | Lighthouse |
| 前端 LCP | < 600ms | Lighthouse |
| 首屏包大小 | < 500KB (gzip) | Webpack Bundle Analyzer |
| API 错误率 | < 0.1% | 日志统计 |

---

## 4. Implementation Order（实现顺序）

### Phase 1.1: 基础骨架（第 1-2 天）
1. 项目初始化 (go.mod, Makefile, .gitignore)
2. 配置系统 (config.go, defaults.go)
3. 数据库连接 + 模型定义 + 自动迁移
4. HTTP 服务器 + 路由骨架 + 中间件链

### Phase 1.2: 认证与权限（第 3-4 天）
5. JWT 认证服务 (hash, token 生成/验证)
6. Auth 中间件 + RBAC 中间件
7. 登录 API + 用户管理 API
8. 种子数据 (admin 用户)

### Phase 1.3: 存储与 npm 适配器（第 5-6 天）
9. StorageBackend 接口 + LocalStorage 实现
10. PackageService (上传/下载/元数据)
11. npm Adapter (publish/install/metadata)

### Phase 1.4: Maven 适配器（第 7 天）
12. Maven Adapter (deploy/install/maven-metadata.xml)

### Phase 1.5: 前端（第 8-12 天）
13. 前端项目初始化 (Vite + TypeScript + Element Plus)
14. 布局组件 (Layout, AppSidebar, AppHeader)
15. 登录页 + 认证流程
16. Dashboard + 包列表页
17. 用户管理页 + 审计日志页

### Phase 1.6: 集成与测试（第 13-14 天）
18. 前后端集成测试
19. 性能压测
20. 文档完善
