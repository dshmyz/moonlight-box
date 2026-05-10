# 常见问题解答 (FAQ)

本文档收集了使用 Moonlight Registry 过程中常见的问题和解决方案。

---

## 目录

- [通用问题](#通用问题)
- [NPM 相关](#npm-相关)
- [Maven 相关](#maven-相关)
- [PyPI 相关](#pypi-相关)
- [Go 相关](#go-相关)
- [NuGet 相关](#nuget-相关)
- [Yum/APT 相关](#yumapt-相关)
- [认证与权限](#认证与权限)
- [性能与优化](#性能与优化)
- [故障排查](#故障排查)

---

## 通用问题

### Q: 如何获取访问令牌？

**A:** 有以下几种方式：

1. **通过 Web UI**：
   - 登录 Web UI
   - 进入"个人设置" → "访问令牌"
   - 点击"生成新令牌"

2. **通过 API**：
```bash
curl -X POST http://your-registry:9081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"your-username","password":"your-password"}'
```

3. **联系管理员**：
   - 管理员可以为您预生成配置文件

---

### Q: 本地仓库和代理仓库有什么区别？

**A:**

| 类型 | 说明 | 用途 |
|------|------|------|
| **本地仓库** | 存储内部开发的包 | 发布和托管内部包 |
| **代理仓库** | 代理外部仓库（如 npmjs.org） | 缓存外部包，加速下载 |
| **虚拟仓库** | 聚合多个仓库 | 统一访问入口，简化配置 |

**推荐配置**：
- 开发环境：使用虚拟仓库（自动从本地或代理获取）
- 发布包：直接发布到本地仓库

---

### Q: 如何配置多个仓库？

**A:** 使用虚拟仓库作为统一入口：

```bash
# NPM 示例
npm config set registry http://your-registry:9081/repo/npm-virtual/

# Maven 示例
<mirror>
  <id>moonlight-virtual</id>
  <mirrorOf>*</mirrorOf>
  <url>http://your-registry:9081/repo/maven-virtual/</url>
</mirror>
```

虚拟仓库会自动从配置的本地仓库和代理仓库中查找包。

---

### Q: 为什么有些包下载很慢？

**A:** 可能的原因：

1. **首次下载**：代理仓库需要从上游下载，首次较慢
2. **网络问题**：检查网络连接和代理设置
3. **上游限制**：某些仓库有速率限制

**解决方案**：
- 使用元数据同步功能预缓存包列表
- 配置本地缓存加速重复下载
- 联系管理员启用 CDN 加速

---

## NPM 相关

### Q: 为什么 `npm adduser` 不工作？

**A:** 当前版本暂不支持 `npm adduser` 命令。

**替代方案**：
1. 通过 Web UI 登录获取令牌
2. 手动配置 `.npmrc` 文件：
```ini
//your-registry:9081/repo/npm-virtual/:_authToken=your-token-here
```
3. 联系管理员获取预配置文件

---

### Q: 如何发布作用域包？

**A:** 配置作用域仓库：

```ini
# .npmrc
@mycompany:registry=http://your-registry:9081/repo/npm-local/
```

或使用命令行：
```bash
npm publish --registry=http://your-registry:9081/repo/npm-local/
```

---

### Q: `npm install` 报错 404 Not Found？

**A:** 可能的原因：

1. **仓库地址错误**：
```bash
# 检查配置
npm config get registry

# 应该是
http://your-registry:9081/repo/npm-virtual/
```

2. **包不存在**：
   - 检查包名拼写
   - 确认包已发布到仓库

3. **认证失败**：
```bash
# 检查认证配置
npm config list
```

---

### Q: 如何删除已发布的包版本？

**A:** 使用 `npm unpublish` 命令：

```bash
# 删除特定版本
npm unpublish package-name@1.0.0 --registry=http://your-registry:9081/repo/npm-local/

# 删除整个包（谨慎使用）
npm unpublish package-name --all --registry=http://your-registry:9081/repo/npm-local/
```

**注意**：需要相应权限。

---

## Maven 相关

### Q: 如何配置 Maven 镜像所有仓库？

**A:** 在 `settings.xml` 中配置：

```xml
<mirrors>
  <mirror>
    <id>moonlight-all</id>
    <mirrorOf>*</mirrorOf>
    <name>Moonlight All Repositories</name>
    <url>http://your-registry:9081/repo/maven-virtual/</url>
  </mirror>
</mirrors>
```

**注意**：`*` 会镜像所有仓库，包括插件仓库。

---

### Q: Maven 发布包失败，提示 401 Unauthorized？

**A:** 检查以下配置：

1. **settings.xml 中的 server 配置**：
```xml
<server>
  <id>moonlight-releases</id>
  <username>your-username</username>
  <password>your-password</password>
</server>
```

2. **pom.xml 中的 repository id 匹配**：
```xml
<distributionManagement>
  <repository>
    <id>moonlight-releases</id>  <!-- 必须与 settings.xml 中的 id 一致 -->
    <url>http://your-registry:9081/repo/maven-local/</url>
  </repository>
</distributionManagement>
```

---

### Q: 如何下载 SNAPSHOT 版本？

**A:** 确保 repository 配置启用了 snapshots：

```xml
<repository>
  <id>moonlight-snapshots</id>
  <url>http://your-registry:9081/repo/maven-local/</url>
  <snapshots>
    <enabled>true</enabled>
    <updatePolicy>always</updatePolicy>
  </snapshots>
</repository>
```

---

### Q: Maven 依赖解析冲突怎么办？

**A:** 使用依赖管理：

```xml
<dependencyManagement>
  <dependencies>
    <dependency>
      <groupId>com.example</groupId>
      <artifactId>problematic-lib</artifactId>
      <version>1.0.0</version>
    </dependency>
  </dependencies>
</dependencyManagement>
```

或使用 `mvn dependency:tree` 查看依赖树。

---

## PyPI 相关

### Q: `pip install` 报错 "Cannot fetch index base URL"？

**A:** 检查配置：

1. **pip.conf 配置正确**：
```ini
[global]
index-url = http://your-registry:9081/repo/pypi-virtual/simple/
trusted-host = your-registry
```

2. **网络可达**：
```bash
curl -I http://your-registry:9081/repo/pypi-virtual/simple/
```

---

### Q: 使用 twine 上传包失败？

**A:** 当前上传端点为 `/pypi/upload/`，请使用：

```bash
twine upload --repository-url http://your-registry:9081/pypi/upload/ dist/*
```

或在 `~/.pypirc` 中配置：
```ini
[moonlight]
repository = http://your-registry:9081/pypi/upload/
username = your-username
password = your-password
```

然后：
```bash
twine upload --repository moonlight dist/*
```

---

### Q: 如何安装特定版本的包？

**A:** 使用版本指定语法：

```bash
# 精确版本
pip install package==1.0.0

# 版本范围
pip install "package>=1.0.0,<2.0.0"

# 最小版本
pip install "package>=1.0.0"
```

---

### Q: pip 缓存在哪里？

**A:** 默认缓存位置：

- **Linux/Mac**: `~/.cache/pip`
- **Windows**: `%LocalAppData%\pip\Cache`

自定义缓存目录：
```bash
pip config set global.cache-dir ~/.pip-cache
```

清除缓存：
```bash
pip cache purge
```

---

## Go 相关

### Q: `go get` 报错 "checksum mismatch"？

**A:** 当前版本不支持校验和数据库，请禁用：

```bash
export GOSUMDB=off
```

或配置项目级：
```bash
go env -w GOSUMDB=off
```

---

### Q: 如何下载私有模块？

**A:** 配置 GOPRIVATE：

```bash
# 配置私有模块前缀
export GOPRIVATE=your-registry,github.com/your-org

# 下载私有模块
go get your-registry/module@latest
```

---

### Q: Go 模块代理配置不生效？

**A:** 检查配置：

```bash
# 查看当前配置
go env GOPROXY

# 设置配置
go env -w GOPROXY=http://your-registry:9081/go,https://proxy.golang.org,direct

# 验证
go env GOPROXY
```

---

### Q: 如何发布 Go 模块？

**A:** 当前版本需要手动上传：

```bash
# 打包模块
go mod tidy
zip -r mymodule-v1.0.0.zip .

# 上传
curl -X PUT \
  -H "Authorization: Bearer your-token" \
  -F "module=@mymodule-v1.0.0.zip" \
  http://your-registry:9081/go/mymodule/v1.0.0/upload
```

---

## NuGet 相关

### Q: Visual Studio 中搜索包失败？

**A:** 当前版本搜索功能有限，建议：

1. **使用包列表浏览**：
```powershell
nuget list -Source moonlight
```

2. **直接安装已知包**：
```powershell
Install-Package PackageName -Source moonlight
```

---

### Q: NuGet push 报错 403 Forbidden？

**A:** 检查：

1. **API Key 正确**：
```powershell
nuget push MyPackage.1.0.0.nupkg \
  -Source http://your-registry:9081/nuget/v3/index.json \
  -ApiKey your-api-key
```

2. **用户有发布权限**：
   - 联系管理员确认权限

---

### Q: 如何配置 NuGet 包源优先级？

**A:** 在 `NuGet.Config` 中：

```xml
<packageSources>
  <clear />
  <add key="moonlight" value="http://your-registry:9081/nuget/v3/index.json" />
  <add key="nuget.org" value="https://api.nuget.org/v3/index.json" />
</packageSources>
```

NuGet 会按顺序尝试包源。

---

## Yum/APT 相关

### Q: Yum 更新缓存失败？

**A:** 清除并重建缓存：

```bash
sudo yum clean all
sudo yum makecache
```

---

### Q: APT 报错 "NO_PUBKEY"？

**A:** 使用信任配置：

```bash
# 方法一：信任仓库（推荐）
deb [trusted=yes] http://your-registry:9081/apt stable main

# 方法二：添加 GPG 密钥
wget -qO - http://your-registry:9081/apt/gpg.key | sudo apt-key add -
```

---

### Q: 如何上传 RPM/DEB 包？

**A:** 使用 curl 上传：

```bash
# RPM 包
curl -X POST \
  -H "Authorization: Bearer your-token" \
  -F "file=@package-1.0.0.rpm" \
  http://your-registry:9081/yum/local/upload

# DEB 包
curl -X POST \
  -H "Authorization: Bearer your-token" \
  -F "file=@package_1.0.0_amd64.deb" \
  http://your-registry:9081/apt/upload
```

---

## 认证与权限

### Q: 忘记密码怎么办？

**A:** 联系管理员重置密码。管理员可以通过以下方式：

1. **Web UI**：用户管理 → 重置密码
2. **API**：
```bash
curl -X PUT http://your-registry:9081/api/v1/users/:id/reset-password \
  -H "Authorization: Bearer admin-token" \
  -H "Content-Type: application/json" \
  -d '{"new_password":"newpass123"}'
```

---

### Q: 如何创建只读账号？

**A:** 管理员可以：

1. 创建用户
2. 分配只读角色
3. 限制仓库访问权限

具体操作请参考权限管理文档。

---

### Q: Token 过期了怎么办？

**A:** 重新生成 Token：

1. 登录 Web UI
2. 进入"个人设置" → "访问令牌"
3. 点击"刷新令牌"

或通过 API：
```bash
curl -X POST http://your-registry:9081/api/v1/auth/refresh \
  -H "Authorization: Bearer your-refresh-token"
```

---

## 性能与优化

### Q: 如何加速包下载？

**A:** 优化建议：

1. **启用本地缓存**：
```bash
# NPM
npm config set cache ~/.npm-cache --global

# pip
pip config set global.cache-dir ~/.pip-cache
```

2. **使用虚拟仓库**：统一入口，自动选择最快源

3. **预缓存常用包**：联系管理员启用元数据同步

---

### Q: 仓库占用空间太大怎么办？

**A:** 联系管理员：

1. 配置清理策略
2. 删除未使用的包版本
3. 启用存储配额

---

### Q: 如何查看下载统计？

**A:** 通过 Web UI：

1. 进入"仪表板"
2. 查看"下载统计"
3. 查看"热门包排行"

或通过 API：
```bash
curl http://your-registry:9081/api/v1/dashboard/stats \
  -H "Authorization: Bearer your-token"
```

---

## 故障排查

### Q: 如何查看详细日志？

**A:** 各客户端启用详细日志：

**NPM**:
```bash
npm install --verbose
```

**Maven**:
```bash
mvn clean install -X
```

**pip**:
```bash
pip install --verbose package-name
```

**Go**:
```bash
GOPROXY=http://your-registry:9081/go go get -v -x package@latest
```

---

### Q: 连接超时怎么办？

**A:** 检查：

1. **网络连通性**：
```bash
ping your-registry
curl -I http://your-registry:9081/health
```

2. **防火墙设置**：确保端口 9081 开放

3. **代理配置**：检查代理设置是否正确

---

### Q: 如何报告 Bug？

**A:** 收集以下信息：

1. 客户端版本（`npm --version`、`mvn --version` 等）
2. 配置文件内容（去除敏感信息）
3. 错误日志
4. 复现步骤

然后：
1. 提交 Issue：https://github.com/your-org/moonlight-box/issues
2. 或联系管理员：admin@company.com

---

### Q: 服务不可用怎么办？

**A:** 检查服务状态：

```bash
# 健康检查
curl http://your-registry:9081/health

# 查看服务状态
systemctl status moonlight-registry

# 查看日志
tail -f /var/log/moonlight-registry/registry.log
```

如果服务确实不可用，联系运维团队。

---

## 获取更多帮助

如果以上 FAQ 无法解决您的问题：

1. 📖 查看 [客户端配置指南](./client-configuration.md)
2. 💬 联系管理员：admin@company.com
3. 🐛 提交 Issue：https://github.com/your-org/moonlight-box/issues
4. 📚 查看官方文档：https://docs.moonlight-box.io

---

**最后更新**：2026-05-03
