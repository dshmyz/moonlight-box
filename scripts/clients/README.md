# 客户端真实集成测试

本目录包含使用官方语言客户端库的真实集成测试脚本，用于验证仓库系统与各种编程语言包管理工具的兼容性。

## 目录说明

### 1. test_go_client.sh
使用官方 `go` 命令测试 Go 模块代理功能。

**测试内容：**
- `go get` - 下载依赖
- `go list -m` - 列出模块信息
- `go mod download` - 下载模块到本地缓存
- GOPROXY 协议端点测试（@v/list, @v/info, @v/mod, @v/zip）

**使用示例：**
```bash
bash scripts/clients/test_go_client.sh http://localhost:9081
```

### 2. test_npm_client.sh
使用官方 `npm` 命令测试 NPM 包管理功能。

**测试内容：**
- `npm install` - 安装依赖包
- `npm view` - 查看包信息
- `npm publish` - 发布包到仓库（需要认证）
- NPM registry API 测试

**使用示例：**
```bash
bash scripts/clients/test_npm_client.sh http://localhost:9081
```

### 3. test_maven_client.sh
使用官方 `mvn` 命令测试 Maven 仓库功能。

**测试内容：**
- `mvn compile` - 编译项目
- `mvn package` - 打包项目
- `mvn deploy` - 部署到仓库（需要配置）
- Maven 元数据和 artifact 下载

**使用示例：**
```bash
bash scripts/clients/test_maven_client.sh http://localhost:9081
```

### 4. test_pypi_client.sh
使用官方 `pip` 命令测试 PyPI 仓库功能。

**测试内容：**
- `pip install` - 安装 Python 包
- PyPI Simple API 测试
- PyPI JSON API 测试
- wheel 包下载和验证

**使用示例：**
```bash
bash scripts/clients/test_pypi_client.sh http://localhost:9081
```

## 前置条件

### Go 测试
- Go 1.16+
- 网络连接（如果测试代理功能）

### NPM 测试
- Node.js 和 npm
- 可选：npm publish 需要有效的认证令牌

### Maven 测试
- Maven 3.x+
- JDK 8+
- 可选：mvn deploy 需要仓库权限配置

### PyPI 测试
- Python 3.7+
- pip
- 可选：twine 用于发布包

## 运行所有客户端测试

```bash
# 运行所有客户端测试
for test in scripts/clients/*.sh; do
    bash "$test" http://localhost:9081
done
```

## 注意事项

1. **认证要求**：某些测试（如 npm publish、mvn deploy）需要有效的认证令牌
2. **网络要求**：代理测试可能需要访问外部上游仓库
3. **清理**：测试脚本会在 /tmp 目录创建临时文件，大部分会自动清理
4. **权限**：确保测试用户有足够的权限执行相关操作

## 测试输出

每个测试脚本都会输出：
- ✅ PASS - 测试通过
- ⚠ WARN - 测试失败或跳过（可能是配置问题）
- ❌ FAIL - 测试明确失败

## 故障排查

### Go 测试失败
- 检查 GOPROXY 环境变量配置
- 确认 Go 版本 >= 1.16
- 验证仓库路径（/repository/ 而非 /repo/）

### NPM 测试失败
- 检查 npm registry 配置
- 确认 Node.js 和 npm 已正确安装
- 验证仓库认证配置

### Maven 测试失败
- 检查 Maven settings.xml 配置
- 确认 Java 版本符合要求
- 验证仓库权限设置

### PyPI 测试失败
- 检查 pip index-url 配置
- 确认 Python 版本 >= 3.7
- 验证 PyPI 代理仓库配置

## 相关文档

- [Maven 生命周期测试](../lifecycle/test_maven_lifecycle.md)
- [NPM 生命周期测试](../lifecycle/test_npm_lifecycle.md)
- [PyPI 生命周期测试](../lifecycle/test_pypi_lifecycle.md)
- [Go 生命周期测试](../lifecycle/test_go_lifecycle.md)
