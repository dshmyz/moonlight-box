# Moonlight Registry 文档中心

欢迎来到 Moonlight Registry 文档中心！本目录包含完整的使用文档和配置指南。

## 📚 文档导航

### 快速开始

- **[客户端配置指南](./client-configuration.md)** - 各包管理器的详细配置方法
- **[配置文件模板](./templates/)** - 开箱即用的配置文件模板
- **[常见问题解答](./faq.md)** - 快速解决常见问题

### 功能文档

- **[AI 集成](./ai-integration.md)** - AI 助手功能使用指南
- **[元数据同步](./superpowers/specs/2026-05-02-metadata-sync-design.md)** - 代理仓库元数据同步

### 技术文档

- **[版本处理最佳实践](./version-handling.md)** - 包版本管理规范
- **[版本提取最佳实践](./version-extraction-best-practices.md)** - 版本号提取方法

### 设计文档

- **[设计规范](./superpowers/specs/)** - 功能设计文档
- **[实现计划](./superpowers/plans/)** - 开发实现计划

## 🚀 快速配置

### NPM 用户

```bash
# 1. 下载配置模板
curl -o ~/.npmrc https://your-registry/docs/templates/.npmrc

# 2. 编辑配置，填入令牌
vi ~/.npmrc

# 3. 验证配置
npm config list
```

### Maven 用户

```bash
# 1. 下载配置模板
curl -o ~/.m2/settings.xml https://your-registry/docs/templates/settings.xml

# 2. 编辑配置，填入认证信息
vi ~/.m2/settings.xml

# 3. 验证配置
mvn help:effective-settings
```

### PyPI 用户

```bash
# 1. 创建配置目录
mkdir -p ~/.pip

# 2. 下载配置模板
curl -o ~/.pip/pip.conf https://your-registry/docs/templates/pip.conf

# 3. 验证配置
pip config list
```

### Go 用户

```bash
# 1. 设置环境变量
export GOPROXY=http://your-registry:9081/go,https://proxy.golang.org,direct
export GOPRIVATE=your-registry
export GOSUMDB=off

# 2. 添加到 shell 配置
echo 'export GOPROXY=http://your-registry:9081/go,https://proxy.golang.org,direct' >> ~/.bashrc
echo 'export GOPRIVATE=your-registry' >> ~/.bashrc
echo 'export GOSUMDB=off' >> ~/.bashrc

# 3. 验证配置
go env GOPROXY
```

## 📖 详细指南

### 支持的包管理器

| 包管理器 | 协议支持 | 配置文档 | 模板文件 |
|---------|---------|---------|---------|
| **NPM** | ✅ 完整支持 | [配置指南](./client-configuration.md#npm-配置) | [.npmrc](./templates/.npmrc) |
| **Maven** | ✅ 完整支持 | [配置指南](./client-configuration.md#maven-配置) | [settings.xml](./templates/settings.xml) |
| **PyPI** | ✅ 完整支持 | [配置指南](./client-configuration.md#pypi-配置) | [pip.conf](./templates/pip.conf) |
| **Go** | ✅ 完整支持 | [配置指南](./client-configuration.md#go-配置) | [go-env.sh](./templates/go-env.sh) |
| **NuGet** | ✅ 完整支持 | [配置指南](./client-configuration.md#nuget-配置) | [NuGet.Config](./templates/NuGet.Config) |
| **Yum** | ✅ 完整支持 | [配置指南](./client-configuration.md#yum-配置) | [moonlight.repo](./templates/moonlight.repo) |
| **APT** | ✅ 完整支持 | [配置指南](./client-configuration.md#apt-配置) | [moonlight.list](./templates/moonlight.list) |

### 仓库类型

| 类型 | 说明 | 使用场景 |
|------|------|----------|
| **本地仓库** | 存储内部开发的包 | 发布和托管内部包 |
| **代理仓库** | 代理外部仓库 | 缓存外部包，加速下载 |
| **虚拟仓库** | 聚合多个仓库 | 统一访问入口，简化配置 |

### 核心功能

- ✅ **包上传/下载** - 支持多种包管理器
- ✅ **代理缓存** - 自动缓存外部包
- ✅ **虚拟仓库** - 统一访问入口
- ✅ **元数据同步** - 预缓存包列表
- ✅ **权限管理** - 细粒度访问控制
- ✅ **审计日志** - 操作记录追踪
- ✅ **Webhook** - 事件通知
- ✅ **AI 助手** - 智能问答

## 🔧 配置模板

所有配置文件模板位于 [templates/](./templates/) 目录：

```
templates/
├── .npmrc              # NPM 配置
├── settings.xml        # Maven 配置
├── pip.conf            # pip 配置
├── .pypirc             # PyPI 上传配置
├── NuGet.Config        # NuGet 配置
├── moonlight.repo      # Yum 仓库配置
├── moonlight.list      # APT 仓库配置
├── go-env.sh           # Go 环境变量
└── README.md           # 模板使用说明
```

## ❓ 常见问题

### Q: 如何获取访问令牌？

**A:** 通过 Web UI 登录后，在"个人设置" → "访问令牌"中生成。

### Q: `npm adduser` 不工作怎么办？

**A:** 当前版本暂不支持该命令，请手动配置 `.npmrc` 文件或联系管理员获取预配置文件。

### Q: 如何发布包？

**A:** 
- **NPM**: `npm publish --registry=http://your-registry:9081/repo/npm-local/`
- **Maven**: `mvn clean deploy`
- **PyPI**: `twine upload --repository-url http://your-registry:9081/pypi/upload/ dist/*`

更多问题请查看 [常见问题解答](./faq.md)。

## 🛠️ 故障排查

### 连接问题

```bash
# 检查服务状态
curl http://your-registry:9081/health

# 检查网络连通性
ping your-registry

# 检查端口
telnet your-registry 9081
```

### 认证问题

```bash
# NPM - 检查配置
npm config list

# Maven - 检查配置
mvn help:effective-settings

# pip - 检查配置
pip config list
```

### 详细日志

```bash
# NPM 详细日志
npm install --verbose

# Maven 详细日志
mvn clean install -X

# pip 详细日志
pip install --verbose package-name
```

## 📞 获取帮助

- 📖 [客户端配置指南](./client-configuration.md)
- ❓ [常见问题解答](./faq.md)
- 📧 联系管理员：admin@company.com
- 🐛 提交 Issue：https://github.com/your-org/moonlight-box/issues

## 📝 文档贡献

如果您发现文档有误或需要补充，欢迎：

1. 提交 Issue 反馈
2. 提交 Pull Request 改进
3. 联系维护团队

---

**文档版本**：1.0.0  
**最后更新**：2026-05-03  
**维护团队**：Moonlight Registry Team
