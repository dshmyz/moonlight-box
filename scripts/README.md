# Scripts 目录结构说明

本目录包含 Moonlight Box Registry 的所有测试脚本和工具脚本。

## 目录结构

```
scripts/
├── clients/              # 🔗 真实客户端集成测试
│   ├── README.md       # 客户端测试说明
│   ├── test_go_client.sh       # Go 客户端测试（go get, go mod）
│   ├── test_npm_client.sh      # NPM 客户端测试（npm install）
│   ├── test_maven_client.sh     # Maven 客户端测试（mvn deploy）
│   └── test_pypi_client.sh      # PyPI 客户端测试（pip install）
│
├── lifecycle/           # 🔄 完整生命周期测试
│   ├── test_go_lifecycle.sh           # Go 模块生命周期
│   ├── test_npm_lifecycle.sh          # NPM 包生命周期
│   ├── test_maven_lifecycle.sh        # Maven 项目生命周期
│   ├── test_maven_snapshot.sh         # Maven SNAPSHOT 版本
│   └── test_pypi_lifecycle.sh         # PyPI 包生命周期
│
├── core/               # ⚙️ 核心功能测试
│   ├── test_basic_http.sh             # 基础 HTTP 操作
│   ├── test_group_repository.sh      # 仓库组功能
│   ├── test-npm-repo.sh              # NPM 仓库功能
│   ├── test-go-repo.sh               # Go 仓库功能
│   ├── test_auth.sh                  # 认证功能
│   ├── test_block.sh                 # 阻断规则功能测试
│   ├── test_virtual_repo_members.sh  # 虚拟仓库成员测试
│   └── verify_report.sh              # 功能验证脚本
│
├── proxy/              # 🌐 代理仓库测试
│   ├── test_all_proxy.sh              # 多协议代理综合测试
│   ├── test-aliyun-proxy.sh           # 阿里云代理测试
│   ├── test-proxy-versions.sh         # 代理版本测试
│   ├── test-proxy-versions-v2.sh      # 代理版本测试 v2
│   ├── test-proxy-versions-v3.sh      # 代理版本测试 v3
│   ├── test-all-proxy-versions.sh     # 所有代理版本测试
│   ├── test-all-proxy-versions-v4.sh  # 所有代理版本测试 v4
│   ├── test-final-proxy-versions.sh   # 最终代理版本测试
│   └── test-versions-after-fix.sh     # 修复后版本测试
│
├── performance/        # 🚀 性能测试
│   └── test_performance.sh           # 性能与压力测试
│
├── exception/         # ⚠️ 异常场景测试
│   └── test_exception_scenarios.sh   # 异常场景处理
│
├── yum/               # 📦 YUM 仓库测试
│   ├── test-yum-download.sh           # YUM 下载测试
│   └── test-version-fix.sh            # YUM 版本修复测试
│
├── debug/             # 🔍 调试和诊断
│   ├── comprehensive_test.sh          # 综合测试
│   ├── test-npm-debug.sh             # NPM 调试
│   ├── test-version-from-scratch.sh   # 从头版本测试
│   ├── test-version-protocols.sh      # 版本协议测试
│   ├── debug-block-rules.sh           # 阻断规则调试工具
│   ├── diagnose_maven.sh              # Maven 问题诊断工具
│   └── test_maven_local_download.sh   # Maven 本地下载诊断
│
├── run_all_tests.sh  # 📋 主测试入口脚本（运行所有测试）
└── README.md         # 本说明文件
```

## 测试类型说明

### 1. 🔗 clients/ - 真实客户端集成测试

使用官方语言客户端库进行的真实集成测试，验证仓库系统与各种包管理工具的兼容性。

**特点：**
- 使用真实的命令行工具（go, npm, mvn, pip）
- 测试实际的包管理操作
- 验证端到端的工作流程

**运行方式：**
```bash
# 运行所有客户端测试
for test in scripts/clients/*.sh; do
    bash "$test" http://localhost:9081
done

# 单独运行
bash scripts/clients/test_go_client.sh http://localhost:9081
bash scripts/clients/test_npm_client.sh http://localhost:9081
bash scripts/clients/test_maven_client.sh http://localhost:9081
bash scripts/clients/test_pypi_client.sh http://localhost:9081
```

### 2. 🔄 lifecycle/ - 完整生命周期测试

测试各协议包的完整生命周期：从创建、发布、下载到删除的全流程。

**覆盖范围：**
- 包创建和配置
- 版本发布和管理
- 元数据生成
- 代理缓存和更新
- 包删除和清理

### 3. ⚙️ core/ - 核心功能测试

测试仓库系统的核心功能，包括仓库管理、权限控制、仓库组等。

**覆盖范围：**
- 基础 HTTP 操作（GET, PUT, DELETE）
- 认证和授权
- 仓库 CRUD 操作
- 仓库组配置和访问控制

### 4. 🌐 proxy/ - 代理仓库测试

测试代理仓库的回源、缓存、更新等代理相关功能。

**覆盖范围：**
- 多协议代理（NPM, Maven, PyPI, Go, YUM）
- 上游仓库配置
- 缓存策略
- 版本同步

### 5. 🚀 performance/ - 性能测试

测试系统在大并发、大文件等场景下的性能表现。

**覆盖范围：**
- 并发上传/下载
- 大文件处理
- 响应时间基准
- 吞吐量和负载测试

### 6. ⚠️ exception/ - 异常场景测试

测试系统在异常情况下的容错和恢复能力。

**覆盖范围：**
- 空文件上传
- 超大文件处理
- 路径遍历防护
- 并发冲突
- 删除后访问

### 7. 📦 yum/ - YUM 仓库测试

专门针对 YUM/DNF 仓库的测试。

**覆盖范围：**
- YUM 仓库元数据
- RPM 包下载
- YUM 配置和验证

### 8. 🔍 debug/ - 调试和诊断

用于问题诊断和调试的脚本，通常包含更详细的日志输出。

**用途：**
- 问题定位
- 协议分析
- 详细日志收集
- Maven 配置诊断
- 阻断规则调试

## 运行指南

### 快速运行所有测试

```bash
cd /path/to/moonlight-box
bash scripts/run_all_tests.sh http://localhost:9081
```

### 分阶段运行

```bash
# 第一阶段：核心功能
bash scripts/core/test_basic_http.sh http://localhost:9081

# 第二阶段：客户端测试
bash scripts/clients/test_go_client.sh http://localhost:9081

# 第三阶段：代理测试
bash scripts/proxy/test_all_proxy.sh http://localhost:9081
```

### 按协议运行

```bash
# Go 相关
bash scripts/clients/test_go_client.sh http://localhost:9081
bash scripts/lifecycle/test_go_lifecycle.sh http://localhost:9081

# NPM 相关
bash scripts/clients/test_npm_client.sh http://localhost:9081
bash scripts/lifecycle/test_npm_lifecycle.sh http://localhost:9081

# Maven 相关
bash scripts/clients/test_maven_client.sh http://localhost:9081
bash scripts/lifecycle/test_maven_lifecycle.sh http://localhost:9081

# PyPI 相关
bash scripts/clients/test_pypi_client.sh http://localhost:9081
bash scripts/lifecycle/test_pypi_lifecycle.sh http://localhost:9081
```

## 前置条件

### 必需
- 服务已启动（`./bin/registry serve`）
- 数据库已初始化
- 管理员账户（admin/admin123）

### 可选
- Go 1.16+（运行 Go 客户端测试）
- Node.js 和 npm（运行 NPM 客户端测试）
- Maven 3.x+ 和 JDK 8+（运行 Maven 客户端测试）
- Python 3.7+ 和 pip（运行 PyPI 客户端测试）
- Apache Bench（运行性能基准测试）

## 测试结果说明

```
✅ PASS - 测试通过
⚠ WARN - 测试失败或跳过（可能是配置问题或环境限制）
❌ FAIL - 测试明确失败（需要调查和修复）
```

## 故障排查

### 服务未启动
```bash
# 检查服务状态
curl http://localhost:9081/api/v1/health

# 启动服务
./bin/registry serve
```

### 认证失败
```bash
# 检查认证配置
curl -X POST http://localhost:9081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

### 端口被占用
```bash
# 查找占用端口的进程
lsof -ti:9081

# 停止占用进程
kill -9 <PID>

# 或使用另一个端口启动（修改 configs/config.yaml 的 server.port，或用环境变量覆盖）
MOONLIGHT_SERVER_PORT=9082 ./bin/moonlight-box
```

## 相关文档

- [项目 README](../../README.md)
- [API 文档](../docs/api.md)
- [部署指南](../docs/deployment.md)
