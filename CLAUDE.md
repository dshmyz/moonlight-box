# CLAUDE.md — Moonlight Box 开发指南

## 项目概述

**Moonlight Box**（moonlight-registry）是一个企业级多协议包仓库管理系统，支持 npm、Maven、PyPI、Go Modules、YUM、APT、Generic 等主流包格式的代理、缓存、托管与安全扫描。可替代 Nexus Repository Manager，提供更轻量、更灵活的自建方案。

## 技术栈

| 层 | 技术 |
|---|------|
| **后端** | Go 1.24 + Gin (HTTP) + GORM (ORM) |
| **前端** | Vue 3 + TypeScript + Vite + Element Plus |
| **数据库** | SQLite（默认）/ PostgreSQL |
| **存储** | Local Filesystem / Amazon S3 (aws-sdk-go-v2) |
| **缓存** | 内存缓存（带 TTL），可选 Redis |
| **监控** | Prometheus client_golang |
| **日志** | Logrus + lumberjack（日志轮转） |
| **配置** | Viper + YAML |
| **鉴权** | JWT (golang-jwt) + CAS SSO |

## 目录结构

```
moonlight-box/
├── cmd/registry/          # 主程序入口
│   ├── main.go            # 启动入口：配置加载 → DB 初始化 → DI 组装 → HTTP Server
│   ├── router.go          # 路由注册 + RouterContext 依赖注入
│   ├── runtime_init.go    # 运行时（Runtime/Plugin）组装与初始化
│   └── frontend.go        # 前端静态文件 embed & 服务
├── internal/              # 核心业务代码（禁止外部 import）
│   ├── api/http/          # HTTP Handler 层（Gin）
│   ├── ai/                # AI 助手（LLM 集成 + 工具调用）
│   ├── config/            # YAML 配置加载（Viper）
│   ├── constants/         # 全局常量
│   ├── core/              # 运行时核心
│   │   ├── cache/         # 缓存管理器 + 各类缓存实现（含 MetadataCache）
│   │   └── runtime/       # ProtocolPlugin Runtime：hosted/proxy/group 路由与回源
│   ├── database/          # DB 连接 + AutoMigrate + Seed
│   ├── errors/            # 业务错误定义（ErrNotFound/ErrBlocked 等）
│   ├── job/               # 后台任务
│   ├── metrics/           # Prometheus 指标
│   ├── middleware/        # Gin 中间件（Auth/CORS/RBAC/Recovery 等）
│   ├── migration/         # 从 Nexus 等迁移数据（v2 流水线架构）
│   ├── model/             # GORM 数据模型
│   ├── plugins/           # 协议插件（npm/maven/pypi/go/yum/apt/raw），实现 ProtocolPlugin 接口
│   ├── proxy/             # 代理下载、远程仓库客户端、健康检查、熔断器
│   ├── repository/        # 数据访问层（GORM 封装）
│   ├── response/          # 统一 API 响应封装
│   ├── service/           # 业务逻辑层
│   ├── storage/           # 存储后端抽象（Local/S3/CAS Blob Store）
│   ├── types/             # 通用类型与上下文声明
│   ├── util/              # 通用工具（加密/哈希/日志/Stream 校验等）
│   └── version/           # 版本号解析（npm/maven/pypi/go）
├── web/                   # 前端（Vue 3 + TS + Element Plus）
├── configs/               # 配置文件示例
├── scripts/               # 运维与测试脚本（按 clients/core/debug/lifecycle 等分类）
├── sql/                   # SQL Schema & 迁移脚本
├── docs/                  # 文档（含 superpowers/plans/specs、swagger、design-archive）
├── bin/                   # 编译产物
├── Makefile               # 构建命令
└── go.mod                 # Go 依赖
```

## 关键命令

```bash
# 后端
make build          # 编译到 bin/moonlight-box
make run            # go run ./cmd/registry serve
make test           # go test -v ./...
make test-coverage  # 覆盖率报告
make lint           # golangci-lint
make dev            # air 热重载开发
make clean          # 清理构建产物

# 前端
cd web && npm run dev       # 开发模式
cd web && npm run build     # 构建生产版
cd web && npm test          # 运行测试

# 全栈
make embed-web      # 构建前端并嵌入到 Go 二进制
make swagger        # 从注释生成 OpenAPI 文档
```

## 架构模式

- **分层架构**: Handler → Service → Repository → Model/Storage
- **插件模式**: `ProtocolPlugin` 接口（`internal/plugins/`）统一不同包协议，每个协议一个插件实现；Runtime 通过 `RemoteFetcher` 回调插件做协议相关的远端交互
- **策略模式**: Runtime 根据仓库类型（hosted/proxy/group）选择不同解析策略
- **工厂模式**: 存储后端（Local/S3）动态创建
- **熔断器模式**: 远程仓库健康检查与熔断保护
- **显式依赖注入**: 所有服务在 `cmd/registry/main.go` 中集中组装，通过 `RouterContext` 传递；运行时层在 `runtime_init.go` 中组装

## 编码约定

- **Go**: 显式依赖注入，不使用 DI 框架；`main.go` 负责组装，`router.go` 负责路由
- **错误处理**: 使用 `fmt.Errorf` + `%w` 包装错误，`logrus.WithFields` 记录上下文
- **数据库**: GORM AutoMigrate 管理 schema；Repository 层封装所有 DB 操作
- **缓存**: 统一通过 `CacheManager` 注册，带 TTL 自动清理
- **前端**: Composition API + `<script setup>`；Element Plus 组件库；Pinia 状态管理
- **API 响应**: 统一通过 `response` 包封装（成功/错误/分页格式一致）

## 重要文件

| 文件 | 职责 |
|------|------|
| `cmd/registry/main.go` | 启动入口，所有服务的 DI 组装点 |
| `cmd/registry/router.go` | 路由注册 + RouterContext 定义 |
| `cmd/registry/runtime_init.go` | 运行时层（Runtime/Plugin）组装与初始化 |
| `internal/plugins/` | `ProtocolPlugin` 接口实现，新增包类型需实现此接口 |
| `internal/core/runtime/` | Runtime 层：hosted/proxy/group 行为、回源策略、缓存决策 |
| `internal/config/` | 配置结构体 + 加载逻辑 |
| `internal/database/` | DB 初始化 + AutoMigrate |
| `internal/proxy/` | 代理下载核心逻辑（缓存、远程请求、健康检查） |
| `web/src/` | 前端源码（Vite 构建，输出到 `cmd/registry/front/`） |

## 注意事项

- **前端构建产物**嵌入到 `cmd/registry/front/`，运行 `make embed-web` 会先清理再构建

## 架构红线（绝对不可破坏）

**文件**: `docs/new3.md` 定义了最终架构。以下红线在重构或添加功能时不可违反：

### 分层边界

| 层 | 职责 | 不可做的事 |
|-----|------|-----------|
| **ProtocolPlugin** | 协议语法：路径解析、请求路由、metadata 解析/渲染、projection 渲染 | **不可**在 Handle 中直接调 HTTP 访问上游。**不可**判断仓库类型（Type=="proxy"/"virtual"）。**不可**做缓存/回源策略决策。**不可**自建 `http.Client`，必须使用 `SetHTTPClient()` 注入的客户端 |
| **RepositoryRuntime** | 仓库行为：hosted/proxy/group、缓存策略、回源时机、stale 判断、merge 策略 | 不可感知协议格式（XML/JSON/HTML） |
| **RemoteFetcher** | Plugin 实现此接口，Runtime 回调它来**拉取远端数据+归一化为 Artifact** | RemoteFetcher 中的 HTTP 调用是唯一合法例外——因为 Runtime 通过它回调 Plugin 做协议相关的远端交互 |

### 请求流程（唯一合法路径）

```
Plugin.Handle()
  → 解析协议路径、识别请求语义
  → runtime.QueryArtifacts(query)       ← 必须带 RemotePath
  → runtime.GetArtifact(key)
  → runtime.RenderProjection(query)
  → runtime.BeginUpload(...)
  → render 协议响应（JSON/XML/HTML）
  ❌ 不可: http.Get() / proxyUpstream / UpstreamBaseURL
```

### 回源流程

```
Plugin 调用 QueryArtifacts(RemotePath=...)
  → ProxyRuntime 发现 metadata store 为空
  → ProxyRuntime 回调 plugin.FetchRemote(remoteURL, path)
    → Plugin 拉取远端、按协议解析、返回 []*Artifact
  → ProxyRuntime 缓存到 metadata store
  → 返回 artifacts
  → Plugin render
```

### 检查清单

新增协议或修改现有协议时，确保:
- [ ] Plugin 的 Handle 方法中没有任何 `http.Get`/`http.Post` 调用
- [ ] Plugin 的 Handle 方法中没有 `ctx.Repository.Type == "proxy"` 判断
- [ ] Plugin 的 Handle 方法中没有 `*GroupRuntime`/`*ProxyRuntime` 类型断言
- [ ] 需要回源的能力通过实现 `RemoteFetcher` 接口提供
- [ ] QueryArtifacts 调用包含 `RemotePath` 字段
- [ ] Plugin 内部不自建 `http.Client`，必须使用 `SetHTTPClient()` 注入的客户端（来自 `proxy.TransportManager`，含 DNS 映射、TLS 配置和连接池）
- [ ] 新增 Plugin 时，`main.go` 中必须调用 `plugin.SetHTTPClient(pluginHTTPClient)`
- [ ] Runtime 层改动不影响任何插件的协议语义
- **日志初始化顺序**: 配置加载 → 临时日志 → DB 初始化 → 正式日志（DB 必须在日志之后初始化）
- **缓存 TTL**: 系统内多处使用 5 分钟 TTL 缓存（Package/Repo/Permission），注意缓存一致性
- **SQLite 默认**: 生产环境建议使用 PostgreSQL，SQLite 仅适合开发/小规模部署
- **GORM AutoMigrate**: 会尝试更新表结构，但不处理列删除或重命名，需要手动 SQL 迁移
- **Batcher 模式**: Download Count 和 Proxy Log 使用批量处理器（定时 flush），`defer Stop()` 确保优雅关闭
- **健康检查**: 远程仓库健康检查配置优先从系统配置读取，其次回退到 YAML 配置
- **前端测试**: 使用 Vitest + Playwright (e2e)，端到端测试配置见 `web/playwright.config.ts`

## Agent skills

### Issue tracker

Issues tracked on GitHub Issues (`dshmyz/moonlight-box`). External PRs are NOT a triage surface. See `docs/agents/issue-tracker.md`.

### Triage labels

Default label names: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout: `CONTEXT.md` + `docs/adr/` at repo root. See `docs/agents/domain.md`.
