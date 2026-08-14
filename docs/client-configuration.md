# 客户端配置指南

本文档提供各种包管理器客户端的配置方法，帮助您快速开始使用 Moonlight Registry。

## 目录

- [NPM 配置](#npm-配置)
- [Maven 配置](#maven-配置)
- [PyPI 配置](#pypi-配置)
- [Go 配置](#go-配置)
- [Yum 配置](#yum-配置)
- [APT 配置](#apt-配置)
- [通用仓库配置](#通用仓库配置)

---

## NPM 配置

### 方式一：使用配置文件（推荐）

1. 编辑配置文件 `~/.npmrc`：
```ini
# ~/.npmrc
registry=http://your-registry:9081/repository/npm-virtual/

# 认证信息（通过 Web UI 个人设置 → 访问令牌 获取）
//your-registry:9081/repository/npm-virtual/:_authToken=your-token-here

# 或者使用 Basic 认证
//your-registry:9081/repository/npm-virtual/:username=your-username
//your-registry:9081/repository/npm-virtual/:_password=your-base64-password
```

2. 验证配置：
```bash
npm config list
npm install test-package
```

### 方式二：使用命令行配置

```bash
# 设置仓库地址
npm config set registry http://your-registry:9081/repository/npm-virtual/

# 设置认证信息
npm config set //your-registry:9081/repository/npm-virtual/:_authToken your-token-here

# 验证配置
npm config get registry
```

### 方式三：项目级配置

在项目根目录创建 `.npmrc` 文件：

```ini
# 项目级配置
registry=http://your-registry:9081/repository/npm-virtual/

# 作用域包配置
@mycompany:registry=http://your-registry:9081/repository/npm-local/
```

### 发布包

```bash
# 发布到本地仓库
npm publish --registry=http://your-registry:9081/repository/npm-local/

# 或在 package.json 中配置
{
  "publishConfig": {
    "registry": "http://your-registry:9081/repository/npm-local/"
  }
}
```

### 常见问题

**Q: `npm adduser` 不工作怎么办？**

A: 请使用以下方式获取认证令牌：
1. 通过 Web UI 登录获取令牌
2. 联系管理员获取预配置的 `.npmrc` 文件

**Q: 如何使用代理仓库？**

A: 配置虚拟仓库地址，系统会自动从上游仓库拉取：
```bash
npm config set registry http://your-registry:9081/repository/npm-virtual/
```

---

## Maven 配置

### 方式一：使用 settings.xml（推荐）

编辑 `~/.m2/settings.xml`：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.0.0
                              http://maven.apache.org/xsd/settings-1.0.0.xsd">
  
  <!-- 服务器认证配置 -->
  <servers>
    <server>
      <id>moonlight-releases</id>
      <username>your-username</username>
      <password>your-password</password>
    </server>
    <server>
      <id>moonlight-snapshots</id>
      <username>your-username</username>
      <password>your-password</password>
    </server>
  </servers>

  <!-- 镜像配置 -->
  <mirrors>
    <mirror>
      <id>moonlight-public</id>
      <mirrorOf>central</mirrorOf>
      <name>Moonlight Public</name>
      <url>http://your-registry:9081/repository/maven-virtual/</url>
    </mirror>
  </mirrors>

  <!-- Profile 配置 -->
  <profiles>
    <profile>
      <id>moonlight</id>
      <repositories>
        <repository>
          <id>moonlight-releases</id>
          <url>http://your-registry:9081/repository/maven-virtual/</url>
          <releases>
            <enabled>true</enabled>
          </releases>
          <snapshots>
            <enabled>false</enabled>
          </snapshots>
        </repository>
        <repository>
          <id>moonlight-snapshots</id>
          <url>http://your-registry:9081/repository/maven-virtual/</url>
          <releases>
            <enabled>false</enabled>
          </releases>
          <snapshots>
            <enabled>true</enabled>
          </snapshots>
        </repository>
      </repositories>
    </profile>
  </profiles>

  <!-- 激活 Profile -->
  <activeProfiles>
    <activeProfile>moonlight</activeProfile>
  </activeProfiles>
</settings>
```

### 方式二：项目级 pom.xml 配置

在项目的 `pom.xml` 中添加：

```xml
<project>
  <!-- 仓库配置 -->
  <repositories>
    <repository>
      <id>moonlight-public</id>
      <url>http://your-registry:9081/repository/maven-virtual/</url>
    </repository>
  </repositories>

  <!-- 发布配置 -->
  <distributionManagement>
    <repository>
      <id>moonlight-releases</id>
      <url>http://your-registry:9081/repository/maven-virtual/</url>
    </repository>
    <snapshotRepository>
      <id>moonlight-snapshots</id>
      <url>http://your-registry:9081/repository/maven-virtual/</url>
    </snapshotRepository>
  </distributionManagement>
</project>
```

### 发布包

```bash
# 发布 Release 版本
mvn clean deploy -DskipTests

# 发布 Snapshot 版本
mvn clean deploy -DskipTests
```

### 常见问题

**Q: 如何配置多个仓库？**

A: 使用虚拟仓库或配置多个 repository：
```xml
<repositories>
  <repository>
    <id>moonlight-virtual</id>
    <url>http://your-registry:9081/repository/maven-virtual/</url>
  </repository>
</repositories>
```

**Q: 如何跳过测试发布？**

A: 使用 `-DskipTests` 参数：
```bash
mvn clean deploy -DskipTests
```

---

## PyPI 配置

### 方式一：使用 pip 配置（推荐）

1. 创建配置文件：
```bash
mkdir -p ~/.pip
cat > ~/.pip/pip.conf << 'EOF'
[global]
index-url = http://your-registry:9081/repository/pypi-virtual/simple/
trusted-host = your-registry

[install]
extra-index-url = http://your-registry:9081/repository/pypi-local/simple/
EOF
```

2. 验证配置：
```bash
pip config list
pip install test-package
```

### 方式二：使用环境变量

```bash
# 设置仓库地址
export PIP_INDEX_URL=http://your-registry:9081/repository/pypi-virtual/simple/
export PIP_TRUSTED_HOST=your-registry

# 安装包
pip install test-package
```

### 方式三：使用 .pypirc 配置

1. 创建配置文件：
```bash
cat > ~/.pypirc << 'EOF'
[distutils]
index-servers =
    moonlight

[moonlight]
repository = http://your-registry:9081/repository/pypi-local/
username = your-username
password = your-password
EOF

chmod 600 ~/.pypirc
```

### 发布包

```bash
# 方式一：使用 twine
pip install twine
twine upload --repository moonlight dist/*

# 方式二：使用 setup.py
python setup.py upload -r moonlight
```

### 常见问题

**Q: 为什么上传包失败？**

A: 当前上传端点为 `/upload/`，请确保使用正确的 URL：
```bash
twine upload --repository-url http://your-registry:9081/repository/pypi-local/ dist/*
```

**Q: 如何使用代理仓库？**

A: 配置虚拟仓库地址，系统会自动从上游仓库拉取：
```bash
pip config set global.index-url http://your-registry:9081/repository/pypi-virtual/simple/
```

---

## Go 配置

### 方式一：使用环境变量（推荐）

```bash
# 配置 GOPROXY
export GOPROXY=http://your-registry:9081/go,https://proxy.golang.org,direct

# 配置私有模块
export GOPRIVATE=your-registry,github.com/your-org

# 配置校验和数据库（可选）
export GOSUMDB=off
```

### 方式二：项目级配置

在项目根目录创建 `.env` 文件：

```bash
# .env
GOPROXY=http://your-registry:9081/go,https://proxy.golang.org,direct
GOPRIVATE=your-registry
```

### 使用示例

```bash
# 下载依赖
go get example.com/package@latest

# 下载私有模块
GOPRIVATE=your-registry go get your-registry/module@latest

# 验证配置
go env GOPROXY
```

### 发布包

```bash
# 上传模块（需要认证）
curl -X PUT \
  -H "Authorization: Bearer your-token" \
  -T ./my-module-v1.0.0.zip \
  http://your-registry:9081/go/my-module/v1.0.0.zip
```

### 常见问题

**Q: 为什么下载包时提示校验和错误？**

A: 当前版本不支持校验和数据库，请禁用校验和验证：
```bash
export GOSUMDB=off
```

**Q: 如何配置多个代理？**

A: 使用逗号分隔多个代理地址：
```bash
export GOPROXY=http://your-registry:9081/go,https://proxy.golang.org,direct
```

---

## 通用仓库配置

Generic（通用）仓库支持任意类型的二进制文件和包，适用于 Docker 镜像层、安装包、文档等非标准格式。

### 下载文件

```bash
# 直接通过 HTTP 下载
curl -O http://your-registry:9081/repository/generic-local/path/to/file.zip

# 带认证下载
curl -H "Authorization: Bearer your-token" \
  -O http://your-registry:9081/repository/generic-local/path/to/file.zip
```

### 上传文件

```bash
# 使用 curl 上传
curl -X PUT \
  -H "Authorization: Bearer your-token" \
  -T ./myfile.zip \
  http://your-registry:9081/repository/generic-local/path/to/file.zip
```

---

## Yum 配置

### 方式一：使用配置文件

1. 创建仓库配置文件：
```bash
sudo cat > /etc/yum.repos.d/moonlight.repo << 'EOF'
[moonlight]
name=Moonlight Repository
baseurl=http://your-registry:9081/repository/yum-local/
enabled=1
gpgcheck=0

[moonlight-proxy]
name=Moonlight Proxy Repository
baseurl=http://your-registry:9081/repository/yum-proxy/
enabled=1
gpgcheck=0
EOF
```

2. 更新缓存：
```bash
sudo yum clean all
sudo yum makecache
```

### 使用示例

```bash
# 搜索包
yum search package-name

# 安装包
sudo yum install package-name

# 查看包信息
yum info package-name
```

### 发布包

```bash
# 上传 RPM 包
curl -X PUT \
  -H "Authorization: Bearer your-token" \
  -T ./package-1.0.0.rpm \
  http://your-registry:9081/repository/yum-local/package-1.0.0.rpm
```

---

## APT 配置

### 方式一：使用配置文件

1. 添加仓库密钥（如果需要）：
```bash
wget -qO - http://your-registry:9081/repository/apt/gpg.key | sudo apt-key add -
```

2. 添加仓库源：
```bash
sudo cat > /etc/apt/sources.list.d/moonlight.list << 'EOF'
deb [trusted=yes] http://your-registry:9081/repository/apt stable main
EOF
```

3. 更新索引：
```bash
sudo apt update
```

### 使用示例

```bash
# 搜索包
apt search package-name

# 安装包
sudo apt install package-name

# 查看包信息
apt show package-name
```

### 发布包

```bash
# 上传 DEB 包
curl -X PUT \
  -H "Authorization: Bearer your-token" \
  -T ./package_1.0.0_amd64.deb \
  http://your-registry:9081/repository/apt/pool/main/package_1.0.0_amd64.deb
```

---

## 通用配置建议

### 1. 认证管理

**推荐做法**：
- 使用专用账号，不要使用个人账号
- 定期轮换密码和令牌
- 为不同环境使用不同的凭据

### 2. 网络配置

**内网环境**：
```bash
# 设置代理（如果需要）
export HTTP_PROXY=http://proxy.company.com:8080
export HTTPS_PROXY=http://proxy.company.com:8080

# 设置信任主机
export NO_PROXY=your-registry,localhost,127.0.0.1
```

### 3. 缓存配置

**本地缓存**：
```bash
# NPM 缓存
npm config set cache ~/.npm-cache --global

# Maven 本地仓库
# 在 settings.xml 中配置
<localRepository>~/.m2/repository</localRepository>

# pip 缓存
pip config set global.cache-dir ~/.pip-cache
```

### 4. 安全建议

- ✅ 使用 HTTPS（生产环境）
- ✅ 启用认证
- ✅ 定期更新凭据
- ✅ 限制权限范围
- ❌ 不要在代码中硬编码凭据
- ❌ 不要共享账号

---

## 获取帮助

如果遇到问题：

1. 查看 [常见问题解答](./faq.md)
2. 提交 Issue：https://github.com/dshmyz/moonlight-box/issues

---

**最后更新**：2026-08-12
