# 后端测试指南

本文档介绍如何测试后端仓库功能，包括单元测试和集成测试。

## 测试概览

### 测试覆盖的功能

| 组件 | 测试文件 | 测试用例数 | 状态 |
|------|----------|------------|------|
| NPM 适配器 | `internal/adapter/npm_adapter_test.go` | 17 | ✅ 通过 |
| 仓库服务 | `internal/service/repository_service_test.go` | 20 | ✅ 通过 |
| E2E 集成测试 | `tests/e2e/npm_e2e_test.go` | 25+ | ✅ 通过 |

### NPM 适配器测试

测试 NPM 协议适配器的核心功能：

- **包解析**：测试作用域包 (@scope/package) 和非作用域包 (express)
- **包发布**：测试有效包发布和缺失附件场景
- **包获取**：测试包元数据和特定版本获取
- **包删除**：测试取消发布功能
- **包上传**：测试元数据验证（名称和版本必填）
- **包删除**：测试完整删除流程

### 仓库服务测试

测试仓库管理的 CRUD 操作：

- **创建仓库**：Local、Proxy、Virtual 三种类型
- **读取仓库**：获取单个仓库和列表
- **更新仓库**：修改仓库配置
- **删除仓库**：删除仓库及其关联数据
- **成员管理**：虚拟仓库成员添加/删除/查询
- **认证配置**：Basic Auth、Bearer Token、API Key

## 运行测试

### 前置条件

```bash
# 安装 Go 依赖
cd /Users/gracegaoya/work/project/moonlight-box
go mod tidy
```

### 运行 NPM 适配器测试

```bash
# 运行所有 NPM 适配器测试
go test ./internal/adapter/... -v -run "TestNpmAdapter"

# 运行特定测试
go test ./internal/adapter/... -v -run "TestNpmAdapter_Publish"

# 运行带覆盖率测试
go test ./internal/adapter/... -cover -v
```

### 运行仓库服务测试

```bash
# 运行所有仓库服务测试
go test ./internal/service/... -v -run "TestRepositoryService"

# 运行特定测试
go test ./internal/service/... -v -run "TestRepositoryService_Create"
```

### 运行所有后端测试

```bash
# 运行所有测试
go test ./... -v

# 运行所有测试（简洁模式）
go test ./...

# 运行测试并生成覆盖率报告
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### 运行 E2E 集成测试

E2E 测试需要服务运行在后台，或者使用测试服务器模式：

```bash
# 方式 1：运行 E2E 测试脚本（需要服务已启动）
chmod +x scripts/test-npm-repo.sh
./scripts/test-npm-repo.sh

# 方式 2：运行 Go E2E 测试
go test ./tests/e2e/... -v
```

### 使用环境变量

```bash
# 设置 Registry 服务地址
export REGISTRY_URL=http://localhost:8080

# 运行 shell 测试脚本
./scripts/test-npm-repo.sh
```

## 测试内容详解

### NPM 本地仓库测试

测试 Local 类型仓库的包管理功能：

1. **创建仓库**：创建 npm 本地仓库
2. **发布包**：使用 npm publish 发布包
3. **获取包元数据**：GET /npm/{scope}/{package}
4. **获取特定版本**：GET /npm/{scope}/{package}/{version}
5. **下载 tarball**：GET /npm/-/tarball/{filename}
6. **取消发布**：DELETE /npm/{scope}/{package}/-rev/{revision}
7. **验证删除**：确认包已删除

### 代理仓库测试

测试 Proxy 类型仓库的代理功能：

1. **创建代理仓库**：配置远程 URL（如 https://registry.npmjs.org）
2. **认证配置**：
   - Basic Auth：用户名/密码认证
   - Bearer Token：Token 认证
   - API Key：自定义 Header 认证
3. **缓存配置**：
   - 启用/禁用缓存
   - 设置 TTL
   - 配置最大缓存大小
4. **代理请求**：通过代理仓库请求公共包

### 虚拟仓库测试

测试 Virtual 类型仓库的成员聚合功能：

1. **创建虚拟仓库**：配置成员列表
2. **添加成员**：将本地/代理仓库添加为成员
3. **成员优先级**：设置成员优先级
4. **删除成员**：移除成员关系
5. **成员列表**：获取虚拟仓库的所有成员

### 错误场景测试

测试异常处理：

- 重复仓库名
- 不存在的仓库
- 无效包发布（缺少名称/版本）
- 认证失败
- 缓存配置错误

## 已知问题

### 需要修复的 Bug

1. **DownloadTarball 方法**：
   - 问题：当 tarball 不存在时返回 200 而不是 404
   - 测试：`TestNpmAdapter_DownloadTarball_NotFound`

2. **Unpublish 方法**：
   - 问题：当包不存在时可能返回 500 而不是正确处理
   - 测试：`TestNpmAdapter_Unpublish`

这些问题已在测试中标记，后续需要修复适配器代码。

## 持续集成

在 CI/CD 管道中添加测试步骤：

```yaml
# .github/workflows/test.yml
- name: Run Go Tests
  run: |
    go mod tidy
    go test ./... -v -coverprofile=coverage.out
    go tool cover -html=coverage.out -o coverage.html
```

## 故障排查

### 测试失败

1. **依赖问题**：运行 `go mod tidy` 确保依赖完整
2. **数据库问题**：检查 SQLite 内存数据库是否正常
3. **权限问题**：确保 `/tmp/test-storage` 目录可写

### 常见问题

```bash
# 问题：record not found 警告
# 解决：这是正常现象，测试使用空数据库

# 问题：no such column: name
# 解决：已在 package_repo.go 中修复 DeleteByNameAndVersion 方法
```

## 测试覆盖率目标

- 语句覆盖率：> 80%
- 分支覆盖率：> 70%
- 函数覆盖率：> 75%

当前覆盖率可以通过运行 `go test ./... -cover` 查看。
