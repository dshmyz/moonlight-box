# 测试脚本分类说明

本文档说明 scripts 目录下所有非单元测试脚本的分类结构。

## 目录结构

```
scripts/
├── run_all_tests.sh              # 主测试运行器（统一入口）
├── README.md                     # 本说明文档
├── core/                         # 核心功能测试
│   ├── test_basic_http.sh        # 基础 HTTP 接口测试
│   ├── test_auth.sh              # 认证与权限测试
│   ├── test_group_repository.sh  # 仓库组（Group）能力测试
│   ├── test-npm-repo.sh          # NPM 仓库功能测试
│   └── test-go-repo.sh           # Go 仓库功能测试
├── lifecycle/                    # 各语言包生命周期测试
│   ├── test_maven_lifecycle.sh   # Maven Release 完整生命周期
│   ├── test_maven_snapshot.sh    # Maven SNAPSHOT 版本测试
│   ├── test_npm_lifecycle.sh     # npm 完整生命周期测试
│   ├── test_pypi_lifecycle.sh    # PyPI 完整生命周期测试
│   └── test_go_lifecycle.sh      # Go 模块完整生命周期测试
├── proxy/                        # 代理仓库测试
│   ├── test_all_proxy.sh         # 多协议代理综合测试
│   ├── test-aliyun-proxy.sh      # 阿里云代理仓库版本测试
│   ├── test-all-proxy-versions.sh           # 远程代理版本测试 v1
│   ├── test-all-proxy-versions-v4.sh        # 远程代理版本测试 v4
│   ├── test-proxy-versions.sh               # 远程代理版本测试基础版
│   ├── test-proxy-versions-v2.sh            # 远程代理版本测试 v2
│   ├── test-proxy-versions-v3.sh            # 远程代理版本测试 v3
│   ├── test-final-proxy-versions.sh         # 最终代理版本验证
│   └── test-versions-after-fix.sh           # 修复后代理版本验证
├── performance/                  # 性能测试
│   └── test_performance.sh       # 性能与压力测试（吞吐、并发、内存等）
├── exception/                    # 异常场景测试
│   └── test_exception_scenarios.sh  # 异常场景测试（空文件、大文件、路径遍历等）
├── yum/                          # YUM 仓库专项测试
│   ├── test-yum-download.sh      # YUM RPM 包下载测试
│   └── test-version-fix.sh       # YUM 版本号修复验证
└── debug/                        # 调试和版本验证测试
    ├── test-version-from-scratch.sh   # 从头验证各语言包版本号
    ├── test-version-protocols.sh      # 各语言版本号协议规则验证
    ├── test-npm-debug.sh              # NPM 包下载调试
    └── comprehensive_test.sh          # 综合测试（如果存在）
```

## 快速使用

### 运行所有测试

```bash
# 运行完整测试套件
bash scripts/run_all_tests.sh

# 指定目标地址
bash scripts/run_all_tests.sh http://localhost:9081

# 指定测试套件
TEST_SUITE=basic bash scripts/run_all_tests.sh
TEST_SUITE=maven bash scripts/run_all_tests.sh
TEST_SUITE=npm bash scripts/run_all_tests.sh
TEST_SUITE=go bash scripts/run_all_tests.sh
TEST_SUITE=pypi bash scripts/run_all_tests.sh
TEST_SUITE=proxy bash scripts/run_all_tests.sh
TEST_SUITE=group bash scripts/run_all_tests.sh
TEST_SUITE=performance bash scripts/run_all_tests.sh
TEST_SUITE=exception bash scripts/run_all_tests.sh
```

### 运行单个分类的测试

```bash
# 核心功能
bash scripts/core/test_basic_http.sh
bash scripts/core/test_auth.sh
bash scripts/core/test_group_repository.sh

# 生命周期
bash scripts/lifecycle/test_maven_lifecycle.sh
bash scripts/lifecycle/test_npm_lifecycle.sh

# 代理仓库
bash scripts/proxy/test_all_proxy.sh

# 性能测试
bash scripts/performance/test_performance.sh

# 异常场景
bash scripts/exception/test_exception_scenarios.sh
```

## 测试分类说明

### core/ - 核心功能测试

测试制品仓库的核心能力：
- **test_basic_http.sh**: 验证基础 HTTP 接口可用性（健康检查、版本信息等）
- **test_auth.sh**: 测试认证、授权、Token 生成与验证
- **test_group_repository.sh**: 测试仓库组功能（多仓库聚合、搜索顺序等）
- **test-npm-repo.sh**: NPM 仓库基本功能
- **test-go-repo.sh**: Go 仓库基本功能

### lifecycle/ - 生命周期测试

测试各语言包从创建、发布、安装到卸载的完整生命周期：
- **test_maven_lifecycle.sh**: Maven Release 版本创建、发布、依赖解析
- **test_maven_snapshot.sh**: Maven SNAPSHOT 版本、时间戳、buildNumber 管理
- **test_npm_lifecycle.sh**: npm 包创建、发布、安装、代理缓存
- **test_pypi_lifecycle.sh**: Python 包构建、上传、pip 安装
- **test_go_lifecycle.sh**: Go 模块 .info/.mod/.zip 端点、GOPROXY 代理

### proxy/ - 代理仓库测试

测试远程代理回源、缓存、版本号记录等功能：
- **test_all_proxy.sh**: 综合测试多协议代理（Maven/NPM/PyPI/Go/YUM）
- **test-aliyun-proxy.sh**: 阿里云镜像代理的版本号验证
- **test-proxy-versions*.sh**: 系列版本验证脚本（迭代修复过程中产生）
- **test-final-proxy-versions.sh**: 最终版本验证
- **test-versions-after-fix.sh**: 修复后的回归验证

### performance/ - 性能测试

测试制品仓库的性能指标：
- **test_performance.sh**: 
  - 基准性能（小文件吞吐）
  - 大文件上传/下载性能
  - 并发上传/下载
  - 持续负载测试
  - 内存泄漏检测
  - 连接池复用测试

### exception/ - 异常场景测试

测试制品仓库对各种异常情况的处理能力：
- **test_exception_scenarios.sh**:
  - 空文件上传
  - 超大文件上传（限制测试）
  - 并发上传同一 SNAPSHOT 版本
  - 路径遍历攻击防护
  - 特殊字符文件名
  - 重复上传处理
  - 删除后再访问
  - 上游不可用代理
  - 慢速客户端测试

### yum/ - YUM 仓库专项测试

测试 YUM/RPM 仓库功能：
- **test-yum-download.sh**: YUM RPM 包上传和下载
- **test-version-fix.sh**: YUM 版本号与 release 分离验证

### debug/ - 调试和版本验证

开发调试过程中使用的验证脚本：
- **test-version-from-scratch.sh**: 从零开始验证各语言包版本号
- **test-version-protocols.sh**: 验证版本号是否符合各语言协议规范
- **test-npm-debug.sh**: NPM 包下载和版本号调试
- **comprehensive_test.sh**: 综合测试脚本（如果存在）

## 注意事项

1. **脚本已归位**：所有测试脚本已统一归入 scripts 分类目录下，原始散落的脚本已清理
2. **run_all_tests.sh**：主测试运行器已更新为引用分类目录下的脚本路径
3. **依赖工具**：部分测试需要安装额外工具（mvn、npm、go、pip、ab 等）
4. **服务要求**：所有测试需要制品仓库服务运行在指定地址
