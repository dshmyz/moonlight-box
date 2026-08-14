# Moonlight Registry 文档中心

欢迎来到 Moonlight Registry 文档中心！本目录包含完整的使用文档和配置指南。

## 📚 文档导航

### 快速开始

- **[客户端配置指南](./client-configuration.md)** - 各包管理器的详细配置方法
- **[常见问题解答](./faq.md)** - 快速解决常见问题

### 功能文档

- **[AI 集成](./ai-integration.md)** - AI 助手功能使用指南
- **[插件开发规范](./plugin-impl-rules.md)** - 协议插件开发规则
- **[协议插件契约](./protocol-plugin-contract.md)** - Plugin ↔ Runtime 接口约定

### 技术文档

- **[版本处理最佳实践](./version-handling.md)** - 包版本管理规范
- **[版本提取最佳实践](./version-extraction-best-practices.md)** - 版本号提取方法

### 设计文档

- **[设计规范](./superpowers/specs/)** - 功能设计文档
- **[实现计划](./superpowers/plans/)** - 开发实现计划

## 🚀 客户端快速配置

### NPM 用户

```bash
# 设置仓库地址
npm config set registry http://your-registry:9081/repository/npm-virtual/

# 设置认证令牌（通过 Web UI 个人设置 → 访问令牌 获取）
npm config set //your-registry:9081/repository/npm-virtual/:_authToken your-token-here

# 验证配置
npm config list
```

### Maven 用户

在 `~/.m2/settings.xml` 中添加：

```xml
<mirrors>
  <mirror>
    <id>moonlight</id>
    <mirrorOf>central</mirrorOf>
    <url>http://your-registry:9081/repository/maven-virtual/</url>
  </mirror>
</mirrors>
```

### PyPI 用户

```bash
# 创建配置文件
mkdir -p ~/.pip
cat > ~/.pip/pip.conf << 'EOF'
[global]
index-url = http://your-registry:9081/repository/pypi-virtual/simple/
trusted-host = your-registry
EOF

# 验证配置
pip config list
```

### Go 用户

```bash
# 设置环境变量
export GOPROXY=http://your-registry:9081/go,https://proxy.golang.org,direct
export GOPRIVATE=your-registry
export GOSUMDB=off

# 验证配置
go env GOPROXY
```

## 📖 支持的包管理器

| 包管理器 | 协议支持 | 配置文档 |
|---------|---------|---------|
| **NPM** | ✅ 完整支持 | [配置指南](./client-configuration.md#npm-配置) |
| **Maven** | ✅ 完整支持 | [配置指南](./client-configuration.md#maven-配置) |
| **PyPI** | ✅ 完整支持 | [配置指南](./client-configuration.md#pypi-配置) |
| **Go** | ✅ 完整支持 | [配置指南](./client-configuration.md#go-配置) |
| **Yum** | ✅ 完整支持 | [配置指南](./client-configuration.md#yum-配置) |
| **APT** | ✅ 完整支持 | [配置指南](./client-configuration.md#apt-配置) |
| **Generic** | ✅ 完整支持 | [配置指南](./client-configuration.md#通用仓库配置) |

### 仓库类型

| 类型 | 说明 | 使用场景 |
|------|------|----------|
| **Hosted（本地仓库）** | 存储内部开发的包 | 发布和托管内部包 |
| **Proxy（代理仓库）** | 代理外部仓库 | 缓存外部包，加速下载 |
| **Group（虚拟仓库）** | 聚合多个仓库 | 统一访问入口，简化配置 |

### 核心功能

- ✅ **包上传/下载** - 支持多种包管理器
- ✅ **代理缓存** - 自动缓存外部包
- ✅ **虚拟仓库** - 统一访问入口
- ✅ **元数据同步** - 预缓存包列表
- ✅ **权限管理** - 细粒度访问控制（RBAC）
- ✅ **审计日志** - 操作记录追踪
- ✅ **Webhook** - 事件通知
- ✅ **AI 助手** - 智能问答与工具调用
- ✅ **MCP Server** - Model Context Protocol 服务端，支持 AI 客户端集成
- ✅ **安全扫描** - 漏洞扫描与阻断规则
- ✅ **数据迁移** - 支持从 Nexus 迁移

## ❓ 常见问题

### Q: 如何获取访问令牌？

**A:** 通过 Web UI 登录后，在"个人设置" → "访问令牌"中生成。

### Q: 如何发布包？

**A:**
- **NPM**: `npm publish --registry=http://your-registry:9081/repository/npm-local/`
- **Maven**: `mvn clean deploy`
- **PyPI**: `twine upload --repository-url http://your-registry:9081/repository/pypi-local/ dist/*`

更多问题请查看 [常见问题解答](./faq.md)。

## 🛠️ 故障排查

```bash
# 检查服务状态
curl http://your-registry:9081/health

# 检查网络连通性
ping your-registry

# 检查端口
telnet your-registry 9081
```

各包管理器详细日志：
- NPM: `npm install --verbose`
- Maven: `mvn clean install -X`
- pip: `pip install --verbose package-name`

## 📞 获取帮助

- 📖 [客户端配置指南](./client-configuration.md)
- ❓ [常见问题解答](./faq.md)
- 🐛 提交 Issue：https://github.com/dshmyz/moonlight-box/issues

---

**最后更新**：2026-08-12
