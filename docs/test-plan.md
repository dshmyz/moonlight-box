# 制品仓库完整能力测试方案

## 文档信息

- **版本**: v1.0
- **创建日期**: 2026-05-08
- **适用范围**: moonlight-box 制品仓库系统
- **测试目标**: 验证制品仓库是否具备完整的能力，包括 Maven、npm、Go、PyPI 等生态系统的完整支持

---

## 一、测试概述

### 1.1 测试方法论

核心方法：**用各生态系统的原生客户端，模拟真实的构建和依赖拉取场景**，同时辅以接口校验和异常测试。

### 1.2 测试环境要求

#### 本地开发环境
- 服务地址: `http://localhost:9081`
- 管理员账号: `admin / admin123`
- 必需工具: curl, git
- 可选工具: mvn, npm, go, pip, twine, ab (Apache Bench)

#### 生产验证环境
- 服务地址: 根据实际部署配置
- 需要相应的访问凭证
- 建议在预生产环境执行完整测试

### 1.3 快速执行

```bash
# 执行所有测试
./scripts/run_all_tests.sh http://localhost:9081

# 执行特定测试套件
TEST_SUITE=maven ./scripts/run_all_tests.sh http://localhost:9081

# 可用测试套件
# all, basic, maven, npm, go, pypi, proxy, group, performance, exception
```

---

## 二、现有测试覆盖分析

### 2.1 已实现的测试

| 测试类别 | 测试脚本 | 覆盖度 | 说明 |
|---------|---------|--------|------|
| 基础 HTTP 接口 | `test_basic_http.sh` | ✅ 完整 | 上传、下载、删除、校验和 |
| 认证与权限 | `test_auth.sh` | ✅ 完整 | 登录、无效凭证、只读用户、令牌验证 |
| Maven Release | `test_maven_lifecycle.sh` | ✅ 完整 | 项目创建、编译、打包、部署、下载 |
| npm 生命周期 | `test_npm_lifecycle.sh` | ✅ 完整 | 发布、安装、代理 |
| PyPI 生命周期 | `test_pypi_lifecycle.sh` | ✅ 完整 | 构建、上传、安装、代理 |
| 多协议代理 | `test_all_proxy.sh` | ✅ 完整 | NPM/Maven/PyPI/Yum/Go/NuGet 代理回源 |
| npm 仓库管理 | `test-npm-repo.sh` | ✅ 完整 | 仓库 CRUD、代理、虚拟仓库 |
| Go 仓库管理 | `test-go-repo.sh` | ✅ 完整 | 仓库 CRUD、代理路由 |
| E2E 测试 | `tests/e2e/*_e2e_test.go` | ✅ 完整 | Go 语言端到端测试 |
| 单元测试 | `internal/adapter/*_test.go` | ✅ 完整 | 各适配器单元测试 |

### 2.2 新增测试（本次补充）

| 测试类别 | 测试脚本 | 覆盖度 | 说明 |
|---------|---------|--------|------|
| **Maven SNAPSHOT** | `test_maven_snapshot.sh` | 🆕 新增 | SNAPSHOT 版本发布、元数据更新、时间戳验证 |
| **Go 模块生命周期** | `test_go_lifecycle.sh` | 🆕 新增 | GOPROXY 协议、@v/list、.info、.mod、.zip 验证 |
| **仓库组能力** | `test_group_repository.sh` | 🆕 新增 | 组合仓库、搜索顺序、成员管理 |
| **性能与压力** | `test_performance.sh` | 🆕 新增 | 基准测试、大文件、并发、持续负载、内存泄漏 |
| **异常场景** | `test_exception_scenarios.sh` | 🆕 新增 | 空文件、超大文件、并发冲突、路径遍历、特殊字符 |
| **测试执行入口** | `run_all_tests.sh` | 🆕 新增 | 统一测试执行、环境检查、测试套件管理 |

---

## 三、详细测试方案

### 3.1 通用能力测试（所有格式都需验证）

#### 3.1.1 基础 HTTP 接口

**测试脚本**: `test_basic_http.sh`

| 测试项 | 方法 | 预期结果 | 状态 |
|-------|------|---------|------|
| 上传制品 | PUT | 返回 201 或 204 | ✅ |
| 下载制品 | GET | 返回 200，内容一致 | ✅ |
| 删除制品 | DELETE | 返回 200/204，再次 GET 返回 404 | ✅ |
| 校验和文件 | GET .sha1/.md5 | 返回正确的校验和文本 | ✅ |

**执行命令**:
```bash
./scripts/test_basic_http.sh http://localhost:9081
```

#### 3.1.2 认证与权限

**测试脚本**: `test_auth.sh`

| 测试项 | 场景 | 预期结果 | 状态 |
|-------|------|---------|------|
| 管理员登录 | 正确凭证 | 返回 access_token | ✅ |
| 无效凭证 | 错误用户名/密码 | 返回 401 | ✅ |
| 只读用户上传 | 只读用户尝试上传 | 返回 401/403 | ✅ |
| 只读用户下载 | 只读用户下载 | 返回 200 | ✅ |
| 无令牌访问 | 未认证请求 | 返回 401/403 | ✅ |
| 无效令牌 | 伪造 token | 返回 401/403 | ✅ |

**执行命令**:
```bash
./scripts/test_auth.sh http://localhost:9081
```

#### 3.1.3 代理仓库能力

**测试脚本**: `test_all_proxy.sh`

| 测试项 | 场景 | 预期结果 | 状态 |
|-------|------|---------|------|
| 首次请求 | 请求不存在的制品 | 从远程拉取并缓存，返回成功 | ✅ |
| 二次请求 | 请求已缓存制品 | 快速返回，不再请求远程 | ✅ |
| 远程不可用 | 上游仓库不可用 | 已缓存仍可下载，未缓存返回 502 | ✅ |

**测试的代理源**:
- Maven: 阿里云 Maven 镜像
- npm: npm.taobao.org
- PyPI: tuna.tsinghua.edu.cn
- Go: goproxy.cn
- Yum: 各 Linux 发行版镜像

**执行命令**:
```bash
./scripts/test_all_proxy.sh http://localhost:9081
```

#### 3.1.4 仓库组能力

**测试脚本**: `test_group_repository.sh` (新增)

| 测试项 | 场景 | 预期结果 | 状态 |
|-------|------|---------|------|
| 创建仓库组 | 组合两个托管仓库 | 创建成功 | 🆕 |
| 下载仓库 A 制品 | 通过 group URL 下载 | 成功返回仓库 A 的内容 | 🆕 |
| 下载仓库 B 制品 | 通过 group URL 下载 | 成功返回仓库 B 的内容 | 🆕 |
| 搜索顺序验证 | 制品存在于多个仓库 | 返回第一个匹配的仓库版本 | 🆕 |
| 不存在制品 | 请求不存在的制品 | 返回 404 | 🆕 |
| 成员配置 | 查看仓库组信息 | 包含正确的成员列表 | 🆕 |

**执行命令**:
```bash
./scripts/test_group_repository.sh http://localhost:9081
```

---

### 3.2 Maven 专项测试

#### 3.2.1 Release 版本发布

**测试脚本**: `test_maven_lifecycle.sh`

| 测试项 | 验证点 | 预期结果 | 状态 |
|-------|--------|---------|------|
| 创建测试项目 | pom.xml 配置 | 项目结构完整 | ✅ |
| 编译项目 | mvn compile | 编译成功 | ✅ |
| 打包项目 | mvn package | 生成 JAR 文件 | ✅ |
| 部署 Release | mvn deploy | 部署成功 | ✅ |
| 文件存储 | 检查存储路径 | JAR、POM、SHA1、MD5 存在 | ✅ |
| metadata.xml | 检查元数据 | 包含 version 和 release 标签 | ✅ |
| 下载依赖 | 新项目依赖解析 | 成功下载 | ✅ |

**执行命令**:
```bash
./scripts/test_maven_lifecycle.sh http://localhost:9081
```

#### 3.2.2 SNAPSHOT 版本发布

**测试脚本**: `test_maven_snapshot.sh` (新增)

| 测试项 | 验证点 | 预期结果 | 状态 |
|-------|--------|---------|------|
| 创建 SNAPSHOT 项目 | version=1.0-SNAPSHOT | 项目配置正确 | 🆕 |
| 第一次部署 | mvn deploy | 部署成功 | 🆕 |
| 时间戳格式 | 文件路径包含时间戳 | `1.0-20230501.123456-1.jar` | 🆕 |
| maven-metadata.xml | 检查元数据 | 包含 `<snapshot>` 标签 | 🆕 |
| timestamp 字段 | 检查时间戳 | 格式: `20230501.123456` | 🆕 |
| buildNumber 字段 | 检查构建号 | `<buildNumber>1</buildNumber>` | 🆕 |
| 第二次部署 | 更新 SNAPSHOT | buildNumber 增加 | 🆕 |
| 下载 SNAPSHOT | 依赖解析 | 成功下载最新版本 | 🆕 |
| 校验和文件 | .sha1 和 .md5 | 可访问且正确 | 🆕 |

**执行命令**:
```bash
./scripts/test_maven_snapshot.sh http://localhost:9081
```

#### 3.2.3 代理仓库下载

**测试脚本**: `test_maven_lifecycle.sh` (已有)

| 测试项 | 验证点 | 预期结果 | 状态 |
|-------|--------|---------|------|
| 从代理下载 | guava 32.1.3-jre | 成功下载 | ✅ |
| 缓存验证 | 第二次下载 | 速度明显提升 | ✅ |

---

### 3.3 npm 专项测试

**测试脚本**: `test_npm_lifecycle.sh`

| 测试项 | 验证点 | 预期结果 | 状态 |
|-------|--------|---------|------|
| 创建测试包 | package.json | 包配置正确 | ✅ |
| 配置 registry | npm set registry | 配置成功 | ✅ |
| 发布包 | npm publish | 发布成功 | ✅ |
| 元数据存储 | 检查 package.json | 元数据正确 | ✅ |
| tarball 存储 | 检查 .tgz 文件 | 附件存在 | ✅ |
| 安装包 | npm install | 成功安装 | ✅ |
| 代理功能 | lodash 安装 | 首次拉取，二次缓存 | ✅ |

**执行命令**:
```bash
./scripts/test_npm_lifecycle.sh http://localhost:9081
```

---

### 3.4 Go 模块专项测试

**测试脚本**: `test_go_lifecycle.sh` (新增)

| 测试项 | 验证点 | 预期结果 | 状态 |
|-------|--------|---------|------|
| 创建测试模块 | go.mod | 模块配置正确 | 🆕 |
| @v/list 端点 | GET /@v/list | 返回版本列表 | 🆕 |
| .info 文件 | GET /@v/v1.8.4.info | 包含 Version 和 Time | 🆕 |
| .mod 文件 | GET /@v/v1.8.4.mod | 包含 module 声明 | 🆕 |
| .zip 文件 | GET /@v/v1.8.4.zip | 有效 zip 格式 | 🆕 |
| 代理模式 | go get 公共模块 | 成功缓存 | 🆕 |
| 缓存验证 | 二次请求 | 响应时间 < 500ms | 🆕 |
| 存储结构 | 检查目录 | 符合 GOPROXY 协议 | 🆕 |
| sumdb 验证 | 校验和匹配 | 不因校验失败而报错 | 🆕 |

**GOPROXY 协议要求**:
```
/{module}/@v/list              - 版本列表
/{module}/@v/{version}.info    - 版本信息
/{module}/@v/{version}.mod     - 模块依赖
/{module}/@v/{version}.zip     - 源码包
```

**执行命令**:
```bash
./scripts/test_go_lifecycle.sh http://localhost:9081
```

---

### 3.5 PyPI 专项测试

**测试脚本**: `test_pypi_lifecycle.sh`

| 测试项 | 验证点 | 预期结果 | 状态 |
|-------|--------|---------|------|
| 创建 Python 包 | setup.py | 包配置正确 | ✅ |
| 构建包 | python setup.py sdist bdist_wheel | 生成 wheel 和 sdist | ✅ |
| 上传包 | twine upload | 上传成功 | ✅ |
| Simple Index | /simple/{package}/ | HTML 列表符合 PEP 503 | ✅ |
| HTML 格式 | `<a href>` 链接 | 包含版本链接和 sha256 哈希 | ✅ |
| 安装包 | pip install | 成功解析并安装 | ✅ |
| 代理功能 | requests 安装 | 首次拉取，二次缓存 | ✅ |

**PEP 503 要求**:
```html
<!DOCTYPE html>
<html>
  <body>
    <a href="../../packages/test-package-1.0.0.tar.gz#sha256=...">test-package-1.0.0.tar.gz</a>
    <a href="../../packages/test_package-1.0.0-py3-none-any.whl#sha256=...">test_package-1.0.0-py3-none-any.whl</a>
  </body>
</html>
```

**执行命令**:
```bash
./scripts/test_pypi_lifecycle.sh http://localhost:9081
```

---

### 3.6 性能与压力测试

**测试脚本**: `test_performance.sh` (新增)

#### 3.6.1 基准性能测试

| 测试项 | 指标 | 预期结果 | 状态 |
|-------|------|---------|------|
| 吞吐量 | Requests/sec | > 100 req/s | 🆕 |
| P50 响应时间 | 50th percentile | < 100ms | 🆕 |
| P90 响应时间 | 90th percentile | < 200ms | 🆕 |
| P95 响应时间 | 95th percentile | < 500ms | 🆕 |
| P99 响应时间 | 99th percentile | < 1000ms | 🆕 |
| 失败请求 | Failed requests | 0 | 🆕 |

**执行命令**:
```bash
./scripts/test_performance.sh http://localhost:9081
```

#### 3.6.2 大文件上传/下载

| 测试项 | 场景 | 预期结果 | 状态 |
|-------|------|---------|------|
| 100MB 上传 | 上传大文件 | 成功，监控内存占用 | 🆕 |
| 100MB 下载 | 下载大文件 | 成功，内容一致 | 🆕 |
| 上传速度 | MB/s | > 10MB/s (本地) | 🆕 |
| 下载速度 | MB/s | > 50MB/s (本地) | 🆕 |

#### 3.6.3 并发测试

| 测试项 | 场景 | 预期结果 | 状态 |
|-------|------|---------|------|
| 并发上传 | 10 个客户端同时上传 | 全部成功，无数据丢失 | 🆕 |
| 并发下载 | 10 个客户端同时下载 | 全部成功，响应正常 | 🆕 |
| 持续负载 | 30 秒持续请求 | 无失败，性能稳定 | 🆕 |

#### 3.6.4 内存泄漏检测

| 测试项 | 方法 | 预期结果 | 状态 |
|-------|------|---------|------|
| 内存增长 | 50 次请求前后对比 | 增长 < 10MB | 🆕 |
| 连接池 | Keep-Alive 请求数 | > 0（连接复用） | 🆕 |

---

### 3.7 异常场景测试

**测试脚本**: `test_exception_scenarios.sh` (新增)

| 测试项 | 场景 | 预期结果 | 状态 |
|-------|------|---------|------|
| 空文件上传 | 上传 0 字节文件 | 正常处理或拒绝 | 🆕 |
| 超大文件上传 | 500MB 文件 | 返回 413 或成功 | 🆕 |
| 并发 SNAPSHOT | 10 个客户端上传同一 SNAPSHOT | maven-metadata.xml 最终一致 | 🆕 |
| 路径遍历攻击 | `../../../etc/passwd` | 返回 400/403/404 | 🆕 |
| 特殊字符文件名 | 空格、%、+ 等 | 正常处理 | 🆕 |
| 重复上传 | 同一制品上传两次 | 覆盖或返回 409 | 🆕 |
| 删除后访问 | 删除后再次 GET | 返回 404 | 🆕 |
| 重复删除 | 删除不存在的制品 | 返回 404 | 🆕 |
| 上游不可用 | 代理远程仓库不可用 | 返回 502 或友好错误 | 🆕 |
| 请求头注入 | 自定义请求头 | 正常处理 | 🆕 |
| 慢速客户端 | --limit-rate 1K | 不破坏客户端体验 | 🆕 |

**执行命令**:
```bash
./scripts/test_exception_scenarios.sh http://localhost:9081
```

---

## 四、测试执行指南

### 4.1 环境准备

#### 安装依赖工具

```bash
# macOS
brew install maven node go python twine httpd

# Ubuntu/Debian
sudo apt-get install maven nodejs npm golang python3-pip apache2-utils
```

#### 启动服务

```bash
# 开发模式
make run

# 或
go run ./cmd/registry serve
```

### 4.2 执行测试

#### 方式一: 执行所有测试

```bash
./scripts/run_all_tests.sh http://localhost:9081
```

#### 方式二: 执行特定测试套件

```bash
# 基础测试
TEST_SUITE=basic ./scripts/run_all_tests.sh http://localhost:9081

# Maven 测试
TEST_SUITE=maven ./scripts/run_all_tests.sh http://localhost:9081

# 性能测试
TEST_SUITE=performance ./scripts/run_all_tests.sh http://localhost:9081
```

#### 方式三: 执行单个测试脚本

```bash
./scripts/test_maven_snapshot.sh http://localhost:9081
```

### 4.3 测试输出

每个测试脚本都会输出:
- ✓ PASS: 测试通过
- ✗ FAIL: 测试失败
- ⚠ WARN: 警告（非关键问题）
- ℹ INFO: 信息提示

最终输出测试汇总:
```
============================================
 测试汇总
============================================
  通过: 15
  失败: 2
  总计: 17

⚠ 部分测试失败! ❌
```

---

## 五、测试报告模板

### 5.1 测试执行报告

```
制品仓库测试报告
================

执行时间: 2026-05-08 14:30:00
测试环境: 本地开发环境 (localhost:9081)
测试版本: v1.0.0

测试摘要:
- 总测试数: 150
- 通过: 145
- 失败: 5
- 跳过: 0
- 通过率: 96.7%

失败用例:
1. [性能] P99 响应时间 >= 1000ms (实际: 1250ms)
2. [异常] 超大文件上传未返回 413
3. ...

性能指标:
- 吞吐量: 150 req/s
- P50: 45ms
- P90: 120ms
- P95: 180ms
- P99: 950ms

风险评估:
- 高风险: 无
- 中风险: P99 响应时间略高，建议优化
- 低风险: 超大文件限制未配置

建议:
1. 配置文件上传大小限制
2. 优化大文件下载性能
3. ...
```

### 5.2 生产部署检查清单

- [ ] 所有基础测试通过
- [ ] 所有生命周期测试通过
- [ ] 性能指标满足 SLA
- [ ] 异常场景处理正确
- [ ] 认证与权限验证通过
- [ ] 代理仓库功能正常
- [ ] 无内存泄漏
- [ ] 并发测试通过

---

## 六、自动化集成

### 6.1 CI/CD 集成

```yaml
# .github/workflows/test.yml
name: Artifact Repository Tests

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Setup Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    
    - name: Setup Maven
      uses: stCarolas/setup-maven@v4.5
      with:
        maven-version: '3.8.6'
    
    - name: Setup Node.js
      uses: actions/setup-node@v3
      with:
        node-version: '18'
    
    - name: Setup Python
      uses: actions/setup-python@v4
      with:
        python-version: '3.11'
    
    - name: Start Service
      run: |
        go build -o moonlight-box ./cmd/registry
        ./moonlight-box serve &
        sleep 5
    
    - name: Run Basic Tests
      run: ./scripts/run_all_tests.sh http://localhost:9081
      env:
        TEST_SUITE: basic
    
    - name: Run Maven Tests
      run: ./scripts/run_all_tests.sh http://localhost:9081
      env:
        TEST_SUITE: maven
    
    - name: Run npm Tests
      run: ./scripts/run_all_tests.sh http://localhost:9081
      env:
        TEST_SUITE: npm
    
    - name: Run Go Tests
      run: ./scripts/run_all_tests.sh http://localhost:9081
      env:
        TEST_SUITE: go
    
    - name: Run PyPI Tests
      run: ./scripts/run_all_tests.sh http://localhost:9081
      env:
        TEST_SUITE: pypi
```

### 6.2 Docker Compose 测试环境

```yaml
# docker-compose.test.yml
version: '3.8'

services:
  registry:
    build: .
    ports:
      - "9081:9081"
    environment:
      - DB_PATH=/data/registry.db
      - STORAGE_PATH=/data/packages
    volumes:
      - test-data:/data
  
  test-runner:
    image: alpine:latest
    depends_on:
      - registry
    volumes:
      - ./scripts:/scripts
    command: >
      sh -c "
        apk add --no-cache curl bash maven nodejs npm go python3 py3-pip;
        /scripts/run_all_tests.sh http://registry:9081
      "

volumes:
  test-data:
```

---

## 七、常见问题

### 7.1 Maven 测试失败

**问题**: `mvn deploy` 返回 401

**解决**: 检查 `settings.xml` 中的用户名和密码是否正确

**问题**: SNAPSHOT 版本未更新时间戳

**解决**: 确保仓库配置允许 SNAPSHOT 更新

### 7.2 npm 测试失败

**问题**: `npm publish` 返回 403

**解决**: 检查 `.npmrc` 中的认证配置

**问题**: 包安装失败

**解决**: 确认 registry URL 配置正确

### 7.3 Go 测试失败

**问题**: `go get` 返回 410 Gone

**解决**: 配置 `GONOSUMCHECK` 和 `GOINSECURE`

**问题**: 模块未找到

**解决**: 确认 GOPROXY 配置正确

### 7.4 PyPI 测试失败

**问题**: `twine upload` 返回 400

**解决**: 检查包格式是否符合 PyPI 规范

**问题**: `pip install` 解析失败

**解决**: 确认 Simple Index HTML 符合 PEP 503

---

## 八、测试维护

### 8.1 添加新测试

1. 在 `scripts/` 目录创建新的测试脚本
2. 遵循现有脚本的格式和输出规范
3. 在 `run_all_tests.sh` 中添加测试套件
4. 更新本文档

### 8.2 测试脚本规范

```bash
#!/bin/bash

set -e

BASE_URL="${1:-http://localhost:9081}"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 计数器
PASS_COUNT=0
FAIL_COUNT=0

# 辅助函数
pass() {
    echo -e "  ${GREEN}✓ PASS${NC} $1"
    PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
    echo -e "  ${RED}✗ FAIL${NC} $1"
    FAIL_COUNT=$((FAIL_COUNT + 1))
}

# 测试逻辑...

# 输出汇总
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  总计: $((PASS_COUNT + FAIL_COUNT))"

if [ $FAIL_COUNT -eq 0 ]; then
    exit 0
else
    exit 1
fi
```

---

## 九、附录

### 9.1 测试脚本清单

| 脚本 | 用途 | 依赖 |
|-----|------|------|
| `run_all_tests.sh` | 统一测试执行入口 | curl |
| `test_basic_http.sh` | 基础 HTTP 接口测试 | curl |
| `test_auth.sh` | 认证与权限测试 | curl |
| `test_maven_lifecycle.sh` | Maven Release 测试 | mvn |
| `test_maven_snapshot.sh` | Maven SNAPSHOT 测试 | mvn |
| `test_npm_lifecycle.sh` | npm 生命周期测试 | npm |
| `test_go_lifecycle.sh` | Go 模块生命周期测试 | go |
| `test_pypi_lifecycle.sh` | PyPI 生命周期测试 | pip, twine |
| `test_all_proxy.sh` | 多协议代理测试 | curl |
| `test_group_repository.sh` | 仓库组能力测试 | curl |
| `test_performance.sh` | 性能与压力测试 | curl, ab |
| `test_exception_scenarios.sh` | 异常场景测试 | curl |

### 9.2 参考文档

- [Maven Repository Manager API](https://maven.apache.org/)
- [npm Registry API](https://github.com/npm/registry)
- [Go Proxy Protocol](https://go.dev/ref/mod#goproxy-protocol)
- [PEP 503 - Simple Repository API](https://www.python.org/dev/peps/pep-0503/)
- [Apache Bench Documentation](https://httpd.apache.org/docs/2.4/programs/ab.html)

---

## 十、版本历史

| 版本 | 日期 | 变更 |
|-----|------|------|
| v1.0 | 2026-05-08 | 初始版本，包含完整测试方案和新增测试脚本 |
