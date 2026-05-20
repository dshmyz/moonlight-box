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
│   └── frontend.go        # 前端静态文件 embed & 服务
├── internal/              # 核心业务代码（禁止外部 import）
│   ├── adapter/           # 包类型适配器（npm/maven/pypi/go/yum/apt/generic）
│   ├── ai/                # AI 助手（LLM 集成 + 工具调用）
│   ├── cache/             # 缓存管理器 + 各类缓存实现
│   ├── config/            # YAML 配置加载（Viper）
│   ├── database/          # DB 连接 + AutoMigrate + Seed
│   ├── handler/           # HTTP Handler 层（Gin）
│   ├── middleware/        # Gin 中间件（Auth/CORS/Audit）
│   ├── migration/         # 从 Nexus 等迁移数据
│   ├── model/             # GORM 数据模型
│   ├── proxy/             # 代理下载、远程仓库客户端、健康检查
│   ├── repository/        # 数据访问层（GORM 封装）
│   ├── service/           # 业务逻辑层
│   ├── storage/           # 存储后端抽象（Local/S3）
│   └── types/             # 接口定义 + 类型声明
├── web/                   # 前端（Vue 3 + TS + Element Plus）
├── configs/               # 配置文件
├── scripts/               # 运维脚本
├── sql/                   # SQL Schema & 迁移
├── docs/                  # 文档
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
- **适配器模式**: `types.Adapter` 接口统一不同包协议，每个协议一个 adapter 实现
- **策略模式**: Proxy Router 根据仓库类型（Local/Proxy/Virtual）选择不同解析策略
- **工厂模式**: 存储后端（Local/S3）动态创建
- **熔断器模式**: 远程仓库健康检查与熔断保护
- **显式依赖注入**: 所有服务在 `cmd/registry/main.go` 中集中组装，通过 `RouterContext` 传递

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
| `internal/types/` | `Adapter` 接口定义，新增包类型需实现此接口 |
| `internal/config/` | 配置结构体 + 加载逻辑 |
| `internal/database/` | DB 初始化 + AutoMigrate |
| `internal/proxy/` | 代理下载核心逻辑（缓存、远程请求、健康检查） |
| `web/src/` | 前端源码（Vite 构建，输出到 `cmd/registry/dist/`） |

## 注意事项

- **前端构建产物**嵌入到 `cmd/registry/dist/`，运行 `make embed-web` 会先清理再构建
- **日志初始化顺序**: 配置加载 → 临时日志 → DB 初始化 → 正式日志（DB 必须在日志之后初始化）
- **缓存 TTL**: 系统内多处使用 5 分钟 TTL 缓存（Package/Repo/Permission），注意缓存一致性
- **SQLite 默认**: 生产环境建议使用 PostgreSQL，SQLite 仅适合开发/小规模部署
- **GORM AutoMigrate**: 会尝试更新表结构，但不处理列删除或重命名，需要手动 SQL 迁移
- **Batcher 模式**: Download Count 和 Proxy Log 使用批量处理器（定时 flush），`defer Stop()` 确保优雅关闭
- **健康检查**: 远程仓库健康检查配置优先从系统配置读取，其次回退到 YAML 配置
- **前端测试**: 使用 Vitest + Playwright (e2e)，`web/scripts/e2e/` 存放端到端测试
