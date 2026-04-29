# Architecture Design Document: Moonlight Registry Phase 1 MVP

**Status**: Approved
**Author**: Ryan (技术架构师)  **Last Updated**: 2026-04-28  **Version**: 1.0
**Stakeholders**: Alex (PM), Maya (设计), Leo (开发)

---

## 1. System Context（系统上下文）

Moonlight Registry 是一个 Go 单体应用，对外提供两类接口：
- **包管理协议接口**：兼容 npm registry / Maven 仓库协议的 HTTP REST API
- **管理后台**：Vue 3 SPA（由后端 serve 静态文件）+ 管理 REST API

```
┌──────────┐     HTTP REST      ┌──────────────────────────────────┐
│ npm cli  │◄──────────────────►│                                  │
│          │     /npm/*         │                                  │
└──────────┘                    │         Moonlight Registry       │
                                │         (Go 单体应用)             │
┌──────────┐     HTTP REST      │                                  │
│ mvn cli  │◄──────────────────►│  ┌─────────┐  ┌───────────────┐  │
│          │   /maven2/*        │  │ npm     │  │ Maven         │  │
└──────────┘                    │  │ adapter │  │ adapter       │  │
                                │  └────┬────┘  └──────┬────────┘  │
┌──────────┐     HTTP REST      │       │              │           │
│ Browser  │◄──────────────────►│  ┌────▼──────────────▼────────┐  │
│          │     / + /api/v1/*  │  │     Core Services          │  │
└──────────┘                    │  │ Auth · Storage · Proxy     │  │
                                │  └────────────┬───────────────┘  │
                                │               │                  │
                                │  ┌────────────▼───────────────┐  │
                                │  │     Data Layer             │  │
                                │  │ SQLite + Local Filesystem  │  │
                                │  └────────────────────────────┘  │
                                └──────────────────────────────────┘

External Dependencies:
- npmjs.org (npm proxy remote, 可选)
- repo.maven.apache.org (Maven proxy remote, 可选)
```

---

## 2. Architectural Goals & Constraints（架构目标与约束）

| Goal | Metric | Target | Constraint |
|------|--------|--------|------------|
| 缓存命中延迟 | P99 延迟 | < 50ms | 本地文件系统读取 |
| 包上传延迟 | P99 延迟 | < 500ms | 本地写入 + DB 记录 |
| API P50 延迟 | 请求延迟 | < 20ms | Gin 路由 + 中间件开销 |
| 并发用户 | 活跃连接 | < 1000 | SQLite WAL 模式 |
| 启动时间 | 服务就绪 | < 5s | 单二进制文件 |
| 内存使用 | 运行时内存 | < 200MB | 无外部服务依赖 |

---

## 3. Non-Goals（不设计的部分）

- ❌ 多节点部署 / 水平扩展 — Phase 1 单实例
- ❌ S3/OSS 存储后端 — Phase 1 仅本地文件
- ❌ PostgreSQL 数据库 — Phase 1 仅 SQLite
- ❌ 供应链安全模块 — Phase 2
- ❌ CAS 内容寻址存储 — Phase 2
- ❌ 多代理仓库 / Proxy 优先级路由 — Phase 2

---

## 4. Architecture Overview（架构概览）

### Component Architecture（组件架构）

```
┌─────────────────────────────────────────────────────────────┐
│                      HTTP Server (Gin)                       │
│  ┌──────────────┐ ┌──────────────┐ ┌─────────────────────┐  │
│  │ npm Routes   │ │ Maven Routes │ │ Admin API Routes    │  │
│  │ /npm/*       │ │ /maven2/*    │ │ /api/v1/*           │  │
│  └──────┬───────┘ └──────┬───────┘ └──────────┬──────────┘  │
│         │                │                     │             │
│  ┌──────▼────────────────▼─────────────────────▼──────────┐  │
│  │                   Middleware Layer                      │  │
│  │  Recovery → RequestID → CORS → Auth → RateLimit → RBAC │  │
│  └──────────────────────┬─────────────────────────────────┘  │
│                          │                                    │
│  ┌───────────────────────▼───────────────────────────────┐   │
│  │                  Protocol Adapters                      │   │
│  │  ┌─────────────┐  ┌─────────────┐                     │   │
│  │  │ NPMAdapter  │  │ MavenAdapter│  统一 PackageAdapter│   │
│  │  │             │  │             │  接口               │   │
│  │  └──────┬──────┘  └──────┬──────┘                     │   │
│  └────────┼─────────────────┼─────────────────────────────┘   │
│           │               │                                    │
│  ┌────────▼───────────────▼───────────────────────────────┐   │
│  │                    Core Services                        │   │
│  │  ┌────────────┐ ┌────────────┐ ┌──────────────────┐   │   │
│  │  │ PackageSvc │ │ StorageSvc │ │ ProxySvc (可选)  │   │   │
│  │  └─────┬──────┘ └─────┬──────┘ └────────┬─────────┘   │   │
│  │        │              │                 │             │   │
│  │  ┌─────▼──────┐ ┌─────▼──────┐ ┌───────▼─────────┐   │   │
│  │  │ AuthSvc    │ │ AuditSvc   │ │ CacheSvc        │   │   │
│  │  └────────────┘ └────────────┘ └─────────────────┘   │   │
│  └───────────────────────────────────────────────────────┘   │
│                                                                │
│  ┌───────────────────────────────────────────────────────┐   │
│  │                   Data Access Layer                     │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │   │
│  │  │ UserRepo │ │ PkgRepo  │ │ RoleRepo │ │AuditRepo │  │   │
│  │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘  │   │
│  └───────┼────────────┼────────────┼────────────┼─────────┘   │
│          │            │            │            │             │
│  ┌───────▼────────────▼────────────▼────────────▼─────────┐   │
│  │              SQLite (GORM ORM)                          │   │
│  └───────────────────────────────────────────────────────┘   │
│                                                                │
│  ┌───────────────────────────────────────────────────────┐   │
│  │              Local Filesystem Storage                   │   │
│  │  data/packages/{npm,maven2}/{package}/{version}/       │   │
│  └───────────────────────────────────────────────────────┘   │
│                                                                │
│  ┌───────────────────────────────────────────────────────┐   │
│  │              Vue 3 SPA (静态文件 serve)                  │   │
│  │  / → index.html + dist/*                              │   │
│  └───────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## 5. Technical Stack & Rationale（技术栈与选型理由）

### 后端

| Layer | Technology | Why | Alternative Considered |
|-------|-----------|-----|----------------------|
| HTTP 框架 | Go + Gin | 高性能路由、中间件生态成熟、团队熟悉 | Chi（更轻量但生态弱）、Echo（功能类似但社区小） |
| ORM | GORM | 自动迁移、关联查询方便、支持 SQLite/PG | database/sql + sqlx（更灵活但开发量大） |
| 数据库 | SQLite | 零配置、单文件部署、WAL 模式足够支撑 Phase 1 规模 | PostgreSQL（过度工程化，Phase 2 升级路径） |
| JWT | golang-jwt/jwt5 | 纯 Go、维护活跃、API 简洁 | dgrijalva/jwt-go（已废弃） |
| 配置 | Viper | YAML/ENV/CLI 多源支持、热重载 | koanf（更轻量但功能少） |
| 日志 | Zap | 结构化 JSON 日志、高性能 | logrus（性能不如 Zap） |
| 密码哈希 | bcrypt | 行业标准、抗暴力破解 | Argon2（更安全但依赖 CGO） |

### 前端

| Layer | Technology | Why | Alternative Considered |
|-------|-----------|-----|----------------------|
| 框架 | Vue 3 + Composition API | 响应式、组合式 API 便于逻辑复用 | React（学习曲线陡、状态管理复杂） |
| 构建 | Vite | HMR 快速、开发体验好 | Webpack（配置复杂、构建慢） |
| UI 库 | Element Plus | 企业级组件丰富、Vue 3 原生 | Ant Design Vue（组件样式偏阿里风格） |
| 状态管理 | Pinia | Vue 3 官方推荐、TypeScript 友好 | Vuex（Vue 3 已不推荐） |
| HTTP | Axios | 拦截器、取消请求、自动 JSON 转换 | fetch API（缺少拦截器机制） |
| 路由 | Vue Router 4 | 官方路由、懒加载支持 | 无替代方案 |
| 图表 | ECharts | 功能丰富、中文文档完善 | Chart.js（功能有限） |

---

## 6. Data Flow & State Management（数据流与状态管理）

### 核心数据流

**包发布流程**：
```
npm publish / mvn deploy
  → HTTP POST /npm/* 或 /maven2/*
    → Auth Middleware (JWT 验证)
      → RBAC Middleware (检查 write 权限)
        → NPMAdapter.ParsePackagePath (解析包名/版本)
          → PackageService.Upload
            → StorageBackend.Put (写入本地文件)
              → GORM: 创建 Package + PackageVersion 记录
                → AuditService.Log (记录操作)
                  → 返回成功响应
```

**包拉取流程**：
```
npm install / mvn install
  → HTTP GET /npm/* 或 /maven2/*
    → Auth Middleware (JWT 验证，如果配置了认证)
      → NPMAdapter.ParsePackagePath
        → PackageService.Download
          → StorageBackend.Get (读取本地文件)
            → GORM: 更新 download_count
              → AuditService.Log (记录操作)
                → 流式返回文件内容
```

**代理缓存流程**（缓存未命中）：
```
GET /npm/react
  → 检查本地缓存
    → 未命中
      → ProxyService.FetchFromRemote(npmjs.org)
        → HTTP GET 远程仓库
          → StorageBackend.Put (缓存到本地)
            → GORM: 创建缓存记录
              → 流式返回给客户端
```

### 状态管理策略

**前端**：Pinia 全局状态 + 组件局部状态
- `authStore`: JWT Token、用户信息、角色列表
- `packageStore`: 包列表、筛选条件、分页状态
- `systemStore`: 系统配置、统计数据

**后端**：无状态服务 + SQLite 持久化 + Go 内存缓存
- `bigcache`: 热点包元数据缓存（TTL 5 分钟）
- `sync.Map`: 并发安全的上传锁（防重复上传）

---

## 7. Performance & Scalability（性能与扩展）

### 性能基线

| Scenario | Metric | Baseline | Target |
|----------|--------|----------|--------|
| 包缓存命中 (GET) | P50 延迟 | 10ms | < 5ms |
| 包缓存命中 (GET) | P99 延迟 | 50ms | < 30ms |
| 包上传 (POST) | P99 延迟 | 300ms | < 200ms |
| 包列表查询 | 响应时间 | 200ms | < 100ms |
| 前端 FCP | 首屏渲染 | 500ms | < 300ms |
| 前端 LCP | 最大内容渲染 | 1s | < 600ms |
| 内存使用 | 运行时 | 100MB | < 80MB |

### 扩展策略

**垂直扩展**：SQLite WAL 模式支持 1000 并发读连接，写入串行化。Phase 1 用户规模 < 100 活跃用户，SQLite 完全够用。

**升级路径**：当写入 QPS > 100 或数据量 > 50GB 时：
1. 迁移到 PostgreSQL（GORM 已支持双驱动）
2. 存储后端切换到 S3（StorageBackend 接口已定义）
3. 服务层保持无状态，可通过负载均衡扩展

**缓存策略**：
- 内存缓存：热点包元数据（包名、版本列表）
- 文件系统：包文件本身（天然缓存）
- 浏览器：静态资源（Vite build 文件名含 hash）

---

## 8. Security & Reliability（安全与可靠性）

### 安全设计

| Layer | Mechanism | Detail |
|-------|-----------|--------|
| 传输层 | HTTPS（生产环境）| TLS 1.3，由反向代理（Nginx）处理 |
| 认证 | JWT (HS256) | Token 过期 24h，Refresh Token 7d |
| 密码 | bcrypt (cost 12) | 存储哈希，永不存明文 |
| 授权 | RBAC | 资源:操作 粒度（npm:read, maven:write） |
| 输入验证 | go-playground/validator | 所有外部输入验证 + 清理 |
| SQL 注入 | GORM 参数化查询 | 不拼接 SQL 字符串 |
| 文件路径 | 路径净化 | 防目录遍历攻击 |

### 可靠性设计

| Aspect | Strategy | Detail |
|--------|----------|--------|
| 错误恢复 | Gin Recovery | Panic 捕获 + 500 响应 |
| 请求追踪 | Request ID | 每个请求唯一 ID，贯穿日志 |
| 日志 | Zap 结构化日志 | 错误级别 + 堆栈追踪 |
| 数据库 | WAL 模式 | SQLite 崩溃恢复 |
| 文件写入 | 原子写入 | 先写 .tmp 再 rename |
| 限流 | Token Bucket | 保护 API 免受暴力请求 |

---

## 9. Open Questions（待解决问题）

- [ ] npm search API 是否在 Phase 1 实现？ — Owner: Alex — Deadline: 开发启动前
  - **建议**：Phase 1 不做，搜索功能通过管理后台的包列表筛选替代
- [ ] Maven SNAPSHOT 版本更新策略？ — Owner: Ryan — Deadline: 开发启动前
  - **建议**：Phase 1 不支持 SNAPSHOT，简化实现，Phase 2 加上
- [ ] 前端是否内嵌到 Go 二进制？ — Owner: Leo — Deadline: 开发启动前
  - **建议**：Phase 1 分开部署（前端 dev server + 后端），生产用 `embed.FS` 内嵌

---

## 10. Appendix（附录）

- [v1.0 设计文档](../docs/superpowers/specs/2026-04-28-moonlight-registry-design.md)
- [v2.0 增强设计](../docs/superpowers/specs/2026-04-28-moonlight-registry-design-v2.md)
- [Phase 1 实现计划](../docs/superpowers/plans/2026-04-28-moonlight-registry-phase1.md)