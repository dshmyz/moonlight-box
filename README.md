# Moonlight Box

[English](README.en.md) | 简体中文

企业级多协议包仓库管理系统，支持多种包格式的代理、缓存、托管与安全扫描。

## 特性

- **多协议支持：** 内置 npm、Maven、PyPI、Go Modules、NuGet、YUM、APT 等主流包仓库适配器
- **智能代理路由：** 支持本地仓库与远程仓库的智能路由，自动回源与缓存加速
- **AI 智能助手：** 集成大语言模型，支持日志查询、数据库查询、包信息查询、安全分析等工具调用
- **安全扫描：** 包上传自动扫描，支持拦截高危漏洞包
- **权限管理：** 基于角色的访问控制（RBAC），支持 CAS 单点登录
- **多存储后端：** 支持本地文件系统与 Amazon S3 对象存储
- **多数据库支持：** SQLite（默认）与 PostgreSQL
- **可观测性：** Prometheus 指标采集，结构化日志输出
- **数据迁移：** 支持从 Nexus Repository Manager 迁移数据

## 快速开始

### 环境要求

- Go >= 1.26
- Node.js >= 20（前端构建）
- SQLite 或 PostgreSQL

### 安装

```bash
# 克隆项目
git clone https://github.com/moonlight-box/moonlight-box.git
cd moonlight-box

# 编译
make build

# 或直接运行
make run
```

### 配置

复制示例配置文件并根据实际情况修改：

```bash
cp configs/config.example.yaml configs/config.yaml
```

核心配置项说明：

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `server.port` | 服务端口 | 9081 |
| `database.driver` | 数据库驱动（sqlite / postgres） | sqlite |
| `storage.backend` | 存储后端（local / s3） | local |
| `ai.enabled` | 是否启用 AI 助手 | false |
| `cache.enabled` | 是否启用缓存 | true |
| `security.enabled` | 是否启用安全扫描 | true |

### 启动

```bash
# 使用默认配置启动
./bin/moonlight-box

# 指定配置文件
./bin/moonlight-box -config configs/config.yaml

# 查看版本
./bin/moonlight-box -version
```

启动后访问 `http://localhost:9081` 即可使用。

## 架构设计

### 核心模块

```
┌─────────────────────────────────────────────────────────────┐
│                        Moonlight Box                         │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │  Adapter  │  │  Proxy   │  │   AI     │  │ Security │   │
│  │  Layer    │  │  Router  │  │ Service  │  │  Scanner │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
│         │              │             │             │        │
│  ┌──────┴──────────────┴─────────────┴─────────────┴──────┐ │
│  │                  Core Services                          │ │
│  │  (Auth / RBAC / Cache / Storage / Migration)           │ │
│  └────────────────────────────────────────────────────────┘ │
│                            │                                │
│  ┌─────────────────────────┴──────────────────────────────┐ │
│  │              Data Layer                                 │ │
│  │  (SQLite / PostgreSQL)  +  (Local FS / S3)             │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### 适配器架构

系统通过 Adapter 接口统一不同包仓库的协议差异：

- **npm 适配器：** 支持 npm registry 协议
- **Maven 适配器：** 支持 Maven 仓库协议
- **PyPI 适配器：** 支持 Python Package Index 协议
- **Go Modules 适配器：** 支持 Go 模块代理协议
- **NuGet 适配器：** 支持 NuGet OData 协议
- **YUM/APT 适配器：** 支持 Linux 包管理器协议
- **通用适配器：** 支持自定义包格式

### 代理路由

Proxy Router 负责将包请求智能路由到正确的来源：

1. **本地优先：** 优先从本地仓库查找
2. **代理回源：** 本地未命中时自动从配置的远程仓库拉取
3. **缓存加速：** 拉取结果自动缓存，减少重复请求
4. **健康检查：** 定期检测远程仓库可用性

## AI 助手

系统内置 AI 智能助手，支持以下工具：

| 工具 | 说明 |
|------|------|
| `log_query` | 查询系统日志 |
| `db_query` | 执行数据库查询 |
| `package_info` | 查询包信息与依赖关系 |
| `security_analysis` | 安全漏洞分析 |
| `code_generator` | 代码生成 |

配置示例：

```yaml
ai:
  enabled: true
  provider: "chatglm"  # chatglm, qwen, custom
  base_url: "http://localhost:8000/v1"
  api_key: "your-api-key"
  model: "chatglm3-6b"
```

## 开发指南

### 常用命令

```bash
# 编译
make build

# 运行
make run

# 运行测试
make test

# 生成测试覆盖率报告
make test-coverage

# 代码检查
make lint

# 清理构建产物
make clean

# 开发模式（热重载）
make dev

# 构建前端并嵌入
make embed-web
```

### 项目结构

```
├── cmd/registry/          # 主程序入口
├── configs/               # 配置文件
├── internal/
│   ├── adapter/           # 包仓库适配器层
│   ├── ai/                # AI 服务
│   ├── config/            # 配置管理
│   ├── database/          # 数据库初始化与迁移
│   ├── handler/           # HTTP 处理器
│   ├── middleware/        # 中间件（鉴权、日志、限流等）
│   ├── migration/         # 数据迁移服务
│   ├── model/             # 数据模型
│   ├── proxy/             # 代理路由与缓存
│   ├── repository/        # 数据访问层
│   ├── response/          # 统一响应格式
│   └── service/           # 业务服务层
├── web/                   # 前端项目（Vue 3 + TypeScript）
└── Makefile
```

### 添加新的包适配器

实现 `Adapter` 接口即可添加新的包类型支持：

```go
type Adapter interface {
    Type() types.PackageType
    RoutePrefix() string
    RegisterRoutes(r *gin.RouterGroup, ...)
    ParsePackagePath(path string) (*types.PackageIdentity, error)
    Upload(ctx context.Context, req *types.UploadRequest) (*types.PackageVersionResult, error)
    Download(ctx context.Context, identity *types.PackageIdentity) (*types.PackageContent, error)
    GetMetadata(ctx context.Context, name string) (*types.PackageMeta, error)
    Delete(ctx context.Context, identity *types.PackageIdentity) error
    ListVersions(ctx context.Context, name string) ([]string, error)
}
```

## 许可证

[MIT](./LICENSE)
