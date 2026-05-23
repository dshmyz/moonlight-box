# 目录结构重构设计文档

**日期**：2026-05-24  
**状态**：设计完成，待实现  
**目标**：完全符合 `new3.md` 文档要求的目录结构

---

## 一、背景和目标

### 当前问题

当前项目的目录结构与 `new3.md` 文档要求的架构设计不一致：

1. ❌ 缺少 `core/` 目录层次
2. ❌ `adapter/` 应该是 `plugins/`
3. ❌ `handler/` 应该是 `api/http/`
4. ❌ `types/v2.go` 包含过多职责，应该拆分
5. ❌ 缺少 `worker/` 目录（后台任务）

### 重构目标

**主要目标**：完全符合 `new3.md` 文档的目录结构

**次要目标**：
- 提升代码可读性和可维护性
- 明确各层职责边界
- 优化依赖关系
- 便于未来扩展

### 约束条件

- ✅ 可以暂时破坏功能（开发阶段）
- ✅ 需要修复所有依赖和测试
- ✅ 需要添加文档和注释
- ✅ 同时优化代码结构

---

## 二、目标目录结构

```
internal/
├── core/                          # 核心层（新增）
│   ├── runtime/                   # 仓库运行时
│   │   ├── hosted.go             # HostedRuntime
│   │   ├── proxy.go              # ProxyRuntime
│   │   ├── group.go              # GroupRuntime
│   │   └── interface.go          # RepositoryRuntime 接口
│   ├── graph/                     # 制品图（新增）
│   │   ├── artifact.go           # Artifact 数据结构
│   │   ├── relation.go           # ArtifactRelation
│   │   └── store.go              # ArtifactGraph 存储
│   ├── blob/                      # Blob 存储
│   │   ├── cas.go                # CAS BlobStore
│   │   ├── interface.go          # BlobStore 接口
│   │   └── metadata.go           # Blob 元数据
│   ├── projection/                # 投影引擎
│   │   ├── engine.go             # Projection Engine
│   │   └── interface.go          # Projection 接口
│   ├── transaction/               # 事务管理
│   │   └── upload_session.go     # UploadSession
│   ├── repository/                # 仓库管理
│   │   ├── manager.go            # RepositoryManager
│   │   └── router.go             # RepositoryRouter
│   ├── cache/                     # 缓存层
│   │   ├── policy.go             # CachePolicy
│   │   └── store.go              # Cache Store
│   └── events/                    # 事件系统（新增）
│       └── dispatcher.go         # Event Dispatcher
│
├── plugins/                       # 协议插件层（原 adapter/）
│   ├── maven/                     # Maven 插件
│   │   ├── plugin.go             # MavenPlugin
│   │   ├── metadata.go           # Maven 元数据处理
│   │   ├── snapshot.go           # Snapshot 处理
│   │   └── upload.go             # Maven 上传
│   ├── npm/                       # npm 插件
│   │   ├── plugin.go
│   │   └── metadata.go
│   ├── pypi/                      # PyPI 插件
│   │   ├── plugin.go
│   │   ├── simple.go             # Simple Index
│   │   └── wheel.go              # Wheel 处理
│   ├── oci/                       # OCI/Docker 插件
│   │   ├── plugin.go
│   │   ├── manifest.go
│   │   ├── auth.go
│   │   └── upload.go
│   ├── apt/                       # APT 插件
│   │   └── plugin.go
│   ├── yum/                       # YUM 插件
│   │   └── plugin.go
│   ├── go/                        # Go Module 插件
│   │   └── plugin.go
│   └── raw/                       # Raw 文件插件
│       └── plugin.go
│
├── api/                           # API 层
│   └── http/                      # HTTP API（原 handler/）
│       ├── router.go             # HTTP 路由
│       ├── repository_handler.go
│       ├── package_handler.go
│       └── ...
│
├── worker/                        # 后台任务（新增）
│   ├── projection/                # 投影生成
│   │   └── generator.go
│   └── gc/                        # 垃圾回收
│       └── collector.go
│
├── model/                         # 数据模型（保持）
├── repository/                    # 数据访问层（保持）
├── service/                       # 业务服务层（保持）
├── storage/                       # 存储后端（保持）
├── middleware/                    # 中间件（保持）
├── config/                        # 配置（保持）
├── constants/                     # 常量（保持）
├── errors/                        # 错误（保持）
├── types/                         # 类型定义（精简）
└── util/                          # 工具（保持）
```

---

## 三、文件迁移映射

### 3.1 核心层迁移

| 原位置 | 新位置 | 说明 |
|--------|--------|------|
| `types/v2.go` (RepositoryRuntime 接口) | `core/runtime/interface.go` | 核心运行时接口 |
| `types/v2.go` (HostedRuntime) | `core/runtime/hosted.go` | Hosted 运行时实现 |
| `types/v2.go` (ProxyRuntime) | `core/runtime/proxy.go` | Proxy 运行时实现 |
| `types/v2.go` (GroupRuntime) | `core/runtime/group.go` | Group 运行时实现 |
| `types/v2.go` (Artifact 结构) | `core/graph/artifact.go` | Artifact 数据结构 |
| `types/v2.go` (ArtifactRelation) | `core/graph/relation.go` | 制品关系 |
| `types/v2.go` (BlobStore 接口) | `core/blob/interface.go` | Blob 存储接口 |
| `storage/cas_blob_store.go` | `core/blob/cas.go` | CAS 实现 |
| `types/v2.go` (ProjectionResult) | `core/projection/interface.go` | 投影接口 |
| `types/v2.go` (UploadSession) | `core/transaction/upload_session.go` | 上传事务 |
| `types/v2.go` (RepositoryRouter) | `core/repository/router.go` | 仓库路由 |
| `types/v2.go` (RepositoryManager) | `core/repository/manager.go` | 仓库管理器 |
| `cache/` | `core/cache/` | 缓存层整体迁移 |

### 3.2 插件层迁移

| 原位置 | 新位置 | 说明 |
|--------|--------|------|
| `adapter/maven_v2.go` | `plugins/maven/plugin.go` | Maven 插件 |
| `adapter/npm_v2.go` | `plugins/npm/plugin.go` | npm 插件 |
| `adapter/pypi_v2.go` | `plugins/pypi/plugin.go` | PyPI 插件 |
| `adapter/apt_v2.go` | `plugins/apt/plugin.go` | APT 插件 |
| `adapter/yum_v2.go` | `plugins/yum/plugin.go` | YUM 插件 |
| `adapter/go_v2.go` | `plugins/go/plugin.go` | Go 插件 |
| `adapter/generic_v2.go` | `plugins/raw/plugin.go` | Raw 插件 |

### 3.3 API 层迁移

| 原位置 | 新位置 | 说明 |
|--------|--------|------|
| `handler/` | `api/http/` | 所有 handler 文件 |

### 3.4 新增目录

| 新位置 | 内容 | 说明 |
|--------|------|------|
| `core/events/` | 事件系统 | 新增事件分发机制 |
| `worker/projection/` | 投影生成器 | 后台投影生成任务 |
| `worker/gc/` | GC 收集器 | Blob 垃圾回收 |

---

## 四、Import 路径更新

### 4.1 路径变更规则

**基础路径**：`github.com/moonlight-box/registry/internal`

| 原路径 | 新路径 | 影响范围 |
|--------|--------|----------|
| `.../internal/types` | `.../internal/core/runtime` | RepositoryRuntime 相关 |
| `.../internal/types` | `.../internal/core/graph` | Artifact 相关 |
| `.../internal/types` | `.../internal/core/blob` | BlobStore 相关 |
| `.../internal/adapter` | `.../internal/plugins/maven` | Maven 插件 |
| `.../internal/adapter` | `.../internal/plugins/npm` | npm 插件 |
| `.../internal/adapter` | `.../internal/plugins/pypi` | PyPI 插件 |
| `.../internal/handler` | `.../internal/api/http` | HTTP handlers |
| `.../internal/cache` | `.../internal/core/cache` | 缓存相关 |

### 4.2 批量更新命令

```bash
# 更新 types 引用
find . -name "*.go" -exec sed -i '' 's|github.com/moonlight-box/registry/internal/types|github.com/moonlight-box/registry/internal/core/runtime|g' {} +

# 更新 adapter 引用
find . -name "*.go" -exec sed -i '' 's|github.com/moonlight-box/registry/internal/adapter|github.com/moonlight-box/registry/internal/plugins|g' {} +

# 更新 handler 引用
find . -name "*.go" -exec sed -i '' 's|github.com/moonlight-box/registry/internal/handler|github.com/moonlight-box/registry/internal/api/http|g' {} +

# 更新 cache 引用
find . -name "*.go" -exec sed -i '' 's|github.com/moonlight-box/registry/internal/cache|github.com/moonlight-box/registry/internal/core/cache|g' {} +
```

---

## 五、执行步骤

### 阶段一：准备工作

**步骤 1：创建新目录结构**
```bash
# 创建 core 层目录
mkdir -p internal/core/{runtime,graph,blob,projection,transaction,repository,cache,events}

# 创建 plugins 层目录
mkdir -p internal/plugins/{maven,npm,pypi,oci,apt,yum,go,raw}

# 创建 api 层目录
mkdir -p internal/api/http

# 创建 worker 层目录
mkdir -p internal/worker/{projection,gc}
```

**步骤 2：备份关键文件**
```bash
# 创建备份分支
git checkout -b backup-before-refactor
git add -A
git commit -m "backup: 保存重构前状态"
git checkout main
```

### 阶段二：核心层迁移

**步骤 3：拆分 types/v2.go**
- 提取 RepositoryRuntime 接口 → `core/runtime/interface.go`
- 提取 HostedRuntime → `core/runtime/hosted.go`
- 提取 ProxyRuntime → `core/runtime/proxy.go`
- 提取 GroupRuntime → `core/runtime/group.go`
- 提取 Artifact 相关 → `core/graph/artifact.go`
- 提取 BlobStore 接口 → `core/blob/interface.go`
- 提取 UploadSession → `core/transaction/upload_session.go`
- 提取 RepositoryRouter → `core/repository/router.go`

**步骤 4：迁移 storage/cas_blob_store.go**
- 移动到 `core/blob/cas.go`
- 更新 package 声明为 `blob`
- 更新内部 import

**步骤 5：迁移 cache/ 目录**
- 整体移动到 `core/cache/`
- 更新所有 package 声明

### 阶段三：插件层迁移

**步骤 6：迁移 adapter/ 到 plugins/**
- `adapter/maven_v2.go` → `plugins/maven/plugin.go`
- `adapter/npm_v2.go` → `plugins/npm/plugin.go`
- `adapter/pypi_v2.go` → `plugins/pypi/plugin.go`
- `adapter/apt_v2.go` → `plugins/apt/plugin.go`
- `adapter/yum_v2.go` → `plugins/yum/plugin.go`
- `adapter/go_v2.go` → `plugins/go/plugin.go`
- `adapter/generic_v2.go` → `plugins/raw/plugin.go`
- 更新所有 package 声明

### 阶段四：API 层迁移

**步骤 7：迁移 handler/ 到 api/http/**
- 移动所有 handler 文件到 `api/http/`
- 更新 package 声明为 `http`
- 更新内部 import

### 阶段五：修复和验证

**步骤 8：批量更新 import 路径**
- 使用 sed 批量替换所有 import 路径

**步骤 9：修复编译错误**
```bash
# 编译检查
go build ./...

# 运行测试
go test ./...
```

**步骤 10：验证和清理**
- 验证所有测试通过
- 清理旧目录（如果确认无误）
- 更新文档

---

## 六、验证检查清单

### 编译验证
- [ ] `go build ./...` 无错误
- [ ] `go vet ./...` 无警告
- [ ] `golint ./...` 无严重问题

### 测试验证
- [ ] 所有单元测试通过
- [ ] 所有集成测试通过
- [ ] 覆盖率保持不变或提升

### 功能验证
- [ ] Maven 仓库功能正常
- [ ] npm 仓库功能正常
- [ ] PyPI 仓库功能正常
- [ ] 代理功能正常
- [ ] 上传功能正常

### 文档验证
- [ ] README 更新
- [ ] 架构图更新
- [ ] API 文档更新

---

## 七、风险评估和回滚策略

### 7.1 主要风险点

**风险 1：Import 循环依赖** 🔴 高风险
- **问题**：拆分 `types/v2.go` 可能导致循环依赖
- **影响**：编译失败
- **缓解**：先拆分接口，再拆分实现，使用依赖注入解耦

**风险 2：测试失败** 🟡 中风险
- **问题**：import 路径变更导致测试找不到文件
- **影响**：CI/CD 失败
- **缓解**：批量更新测试文件 import，本地先运行所有测试

**风险 3：运行时错误** 🟡 中风险
- **问题**：反射、序列化等依赖包路径的场景
- **影响**：运行时 panic 或数据错误
- **缓解**：检查所有使用反射的代码，增加集成测试覆盖

**风险 4：第三方依赖** 🟢 低风险
- **问题**：外部包可能依赖旧路径
- **影响**：外部集成失败
- **缓解**：检查所有外部依赖，提供兼容层

### 7.2 回滚策略

**策略 1：Git 分支回滚**（推荐）
```bash
git checkout backup-before-refactor
git checkout -b main-restored
git branch -D main
git branch -m main-restored main
```

**策略 2：选择性回滚**
```bash
git checkout backup-before-refactor -- internal/types/v2.go
git checkout backup-before-refactor -- internal/adapter/
```

**策略 3：分阶段提交**
```bash
git commit -m "refactor: 阶段1 - 创建新目录结构"
git commit -m "refactor: 阶段2 - 核心层迁移"
# 如果某阶段失败，只回滚该阶段
git reset --hard HEAD~1
```

---

## 八、成功标准

### 必须满足
1. ✅ `go build ./...` 无错误
2. ✅ `go test ./...` 全部通过
3. ✅ 核心功能（上传/下载/代理）正常工作
4. ✅ 目录结构完全符合 `new3.md` 要求

### 期望满足
1. ✅ 代码可读性提升
2. ✅ 依赖关系更清晰
3. ✅ 测试覆盖率保持或提升
4. ✅ 文档完整更新

---

## 九、后续优化建议

重构完成后，建议进行以下优化：

1. **接口隔离**：进一步拆分大接口，提高灵活性
2. **依赖注入**：使用 DI 框架管理依赖关系
3. **错误处理**：统一错误处理机制
4. **日志规范**：统一日志格式和级别
5. **性能优化**：优化热点代码路径
6. **文档补充**：补充架构说明和开发指南

---

## 附录：参考资料

- 架构设计文档：`docs/new3.md`
- 原始目录结构：`internal/`
- Go 项目布局建议：https://github.com/golang-standards/project-layout
