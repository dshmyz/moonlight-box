# 客户端配置指南

本文档详细介绍了如何配置各种包管理器以使用 Moonlight Registry。

## NPM 配置

NPM 是 Node.js 的包管理器，用于安装、发布和管理 JavaScript 包。

### 方式一：配置文件

通过编辑 `~/.npmrc` 文件进行配置，适合长期使用。

#### 步骤

1. 创建或编辑 `~/.npmrc` 文件
2. 添加以下内容：

```
registry=https://your-moonlight-domain/repo/npm-virtual/
//your-moonlight-domain/repo/npm-virtual/:_authToken=YOUR_TOKEN_HERE
```

3. 保存文件并验证配置：

```bash
npm config get registry
npm info express
```

### 方式二：项目级配置

在项目根目录创建 `.npmrc` 文件，仅对当前项目生效。

### 发布包

配置完成后，使用以下命令发布包：

```bash
npm publish
```

## Maven 配置

Maven 是 Java 项目的构建和依赖管理工具。

### 配置 settings.xml

编辑 `~/.m2/settings.xml` 文件：

```xml
<settings>
  <servers>
    <server>
      <id>moonlight</id>
      <username>YOUR_USERNAME</username>
      <password>YOUR_PASSWORD</password>
    </server>
  </servers>
  <mirrors>
    <mirror>
      <id>moonlight</id>
      <mirrorOf>central</mirrorOf>
      <url>https://your-moonlight-domain/repo/maven-virtual/</url>
    </mirror>
  </mirrors>
</settings>
```

### 验证配置

```bash
mvn help:effective-settings
mvn dependency:resolve
```

## PyPI 配置

PyPI 是 Python 的包索引和依赖管理工具。

### 配置 pip

创建 `~/.pip/pip.conf` 文件（Linux/macOS）或 `%APPDATA%\pip\pip.ini`（Windows）：

```ini
[global]
index-url = https://your-moonlight-domain/repo/pypi-virtual/simple/
trusted-host = your-moonlight-domain
```

### 验证配置

```bash
pip config list
pip install requests
```

## Go 配置

Go modules 是 Go 语言的依赖管理系统。

### 配置环境变量

设置以下环境变量：

```bash
export GOPROXY=https://your-moonlight-domain/go,https://proxy.golang.org,direct
export GOPRIVATE=your-moonlight-domain
export GOSUMDB=off
```

### 验证配置

```bash
go env GOPROXY
go get github.com/gin-gonic/gin
```

## NuGet 配置

NuGet 是 .NET 的包管理器。

### 方式一：命令行配置

```bash
nuget sources add -name moonlight -source https://your-moonlight-domain/nuget/v3/index.json
nuget sources update -name moonlight -username YOUR_USERNAME -password YOUR_PASSWORD
```

### 方式二：配置文件

创建 `NuGet.Config` 文件：

```xml
<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <add key="moonlight" value="https://your-moonlight-domain/nuget/v3/index.json" />
  </packageSources>
</configuration>
```

### 验证配置

```bash
nuget list -Source moonlight
dotnet add package Newtonsoft.Json
```

## Yum 配置

Yum 是 CentOS/RHEL 系统的包管理器。

### 配置仓库文件

创建 `/etc/yum.repos.d/moonlight.repo` 文件：

```ini
[moonlight]
name=Moonlight Registry
baseurl=https://your-moonlight-domain/repo/yum-virtual/$basearch/
enabled=1
gpgcheck=0
```

### 验证配置

```bash
yum repolist
yum install nginx
```

## APT 配置

APT 是 Debian/Ubuntu 系统的包管理器。

### 配置软件源

创建 `/etc/apt/sources.list.d/moonlight.list` 文件：

```
deb https://your-moonlight-domain/repo/apt-virtual/ stable main
```

### 添加 GPG 密钥（可选）

```bash
curl -fsSL https://your-moonlight-domain/gpg.key | apt-key add -
```

### 验证配置

```bash
apt update
apt install nginx
```

## 常见问题

### 认证失败

- 检查用户名和密码是否正确
- 确认网络连接正常
- 查看服务器日志获取详细错误信息

### 包下载缓慢

- 检查网络带宽
- 确认服务器资源充足
- 考虑配置本地缓存

### 发布包失败

- 确认有写入权限
- 检查包版本是否已存在
- 验证包格式是否符合规范
