# 协议插件实现规则手册

> 每种包格式的**协议规范** + **实现细节**，供重构时对照参考。
> 最后更新: 2026-05-27

---

## 目录

1. [架构约束（通用）](#1-架构约束通用)
2. [接口契约](#2-接口契约)
3. [npm](#3-npm)
4. [PyPI](#4-pypi)
5. [Maven](#5-maven)
6. [Go Modules](#6-go-modules)
7. [YUM](#7-yum)
8. [APT](#8-apt)
9. [Generic (Raw)](#9-generic-raw)
10. [通用检查清单](#10-通用检查清单)

---

## 1. 架构约束（通用）

### 1.1 四条红线

| 红线 | 说明 |
|------|------|
| **禁止直接 HTTP 调用** | `Handle()` 中不得出现 `http.Get` / `http.Post` 等调用。唯一的合法 HTTP 调用在 `FetchRemote()` 中，由 Runtime 回调 |
| **禁止判断仓库类型** | 不得出现 `ctx.Repository.Type == "proxy"` / `ctx.Repository.Type == "virtual"` 等判断 |
| **禁止 Runtime 类型断言** | 不得出现 `*GroupRuntime` / `*ProxyRuntime` / `*HostedRuntime` 类型断言 |
| **禁止自建 HTTP 客户端** | Plugin 内部**不得** `&http.Client{}` 或 `http.DefaultClient`。所有 HTTP 请求必须使用通过 `SetHTTPClient()` 注入的 client，该 client 由 `main.go` 从 `proxy.TransportManager` 获取，包含 DNS 映射、TLS 配置和连接池 |

### 1.2 请求流程（唯一合法路径）

```
Plugin.Handle(ctx, repoRuntime)
  → 解析路径、识别请求语义
  → repoRuntime.QueryArtifacts(query)   ← 必须带 RemotePath
  → repoRuntime.GetArtifact(key)
  → repoRuntime.BeginUpload(...)
  → 渲染协议响应（JSON/XML/HTML）
```

### 1.3 回源流程

```
Plugin 调用 QueryArtifacts(RemotePath=...)
  → ProxyRuntime 发现 metadata store 为空
  → ProxyRuntime 回调 plugin.FetchRemote(remoteURL, path)
    → Plugin 拉取远端、按协议解析、返回 []*Artifact
  → ProxyRuntime 缓存到 metadata store
  → 返回 artifacts
  → Plugin 渲染响应
```

### 1.4 Context 传递

所有 `context.Background()` 调用必须替换为 `ctx.Request.Context()`，确保超时和取消信号正确传播。

---

## 2. 接口契约

### 2.1 ProtocolPlugin

```go
type ProtocolPlugin interface {
    Name() string
    Handle(ctx *RequestContext, repoRuntime RepositoryRuntime) error
}
```

### 2.2 RemoteFetcher

```go
type RemoteFetcher interface {
    FetchRemote(ctx context.Context, remoteURL, path string) ([]*Artifact, error)
}
```

所有 7 个插件均实现此接口。`FetchRemote` 中的 HTTP 调用是唯一合法的例外。

### 2.3 RepositoryRuntime

```go
type RepositoryRuntime interface {
    GetArtifact(ctx context.Context, key ArtifactKey) (*ArtifactWithContent, error)
    QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
    BeginUpload(ctx context.Context, req UploadRequest) (UploadSession, error)
    DeleteArtifact(ctx context.Context, key ArtifactKey) error
}
```

### 2.4 ArtifactKey 坐标约定

Coordinates 是 `map[string]string`，不同格式使用不同 key：

| 格式 | 核心坐标 key |
|------|-------------|
| npm | `name`, `version`, `path` |
| PyPI | `package`, `version`, `filename`, `path` |
| Maven | `group`, `artifact`, `version`, `filename`, `path`, `name`(=group:artifact) |
| Go | `module`, `version`, `path`, `ext` |
| YUM | `file`, `filename`, `name`, `version`, `type`, `href`, `path` |
| APT | `package`, `name`, `version`, `filename`, `file` |
| Generic | `name`, `path` |

---

## 3. npm

### 3.1 协议规范

**规范来源**: [CommonJS Registry API](https://github.com/npm/registry/blob/main/docs/REGISTRY-API.md)、npm CLI 文档

**Registry 基础 URL**: `https://registry.npmjs.org`（官方）

#### 3.1.1 包元数据

```
GET /{package}
```

返回完整包元数据 JSON，结构如下：

```json
{
  "name": "express",
  "dist-tags": {
    "latest": "4.18.2"
  },
  "versions": {
    "4.18.2": {
      "name": "express",
      "version": "4.18.2",
      "dist": {
        "tarball": "https://registry.npmjs.org/express/-/express-4.18.2.tgz",
        "shasum": "...",
        "integrity": "sha512-..."
      },
      "dependencies": { ... },
      "devDependencies": { ... },
      "scripts": { ... }
    }
  }
}
```

**Scoped 包**: `@scope/name` 格式，URL 中需要编码为 `%40scope%2Fname`

#### 3.1.2 Tarball 下载

```
GET /{package}/-/{package}-{version}.tgz
```

返回 gzip 压缩的 tar 归档文件，包含包的源码。

- **路径格式**: `/{name}/-/{name}-{version}.tgz`
- **Scoped 包**: `/@scope/name/-/name-{version}.tgz`（scope 部分不包含在 tarball 文件名中）
- **Content-Type**: `application/octet-stream`

#### 3.1.3 包发布

```
PUT /{package}
```

请求体是完整的包元数据 JSON（包含 `_attachments` 字段，内嵌 tarball 的 base64 编码）。

```json
{
  "name": "my-package",
  "version": "1.0.0",
  "dist-tags": { "latest": "1.0.0" },
  "_attachments": {
    "my-package-1.0.0.tgz": {
      "content_type": "application/octet-stream",
      "data": "<base64-encoded-tarball>"
    }
  }
}
```

#### 3.1.4 dist-tags

```
GET /-/package/{package}/dist-tags
PUT /-/package/{package}/dist-tags/{tag}
DELETE /-/package/{package}/dist-tags/{tag}
```

dist-tags 是版本标签映射，如 `{"latest": "1.0.0", "beta": "2.0.0-beta.1"}`

#### 3.1.5 搜索

```
GET /-/v1/search?text={query}&size={limit}
```

返回搜索结果 JSON。

#### 3.1.6 认证

```
PUT /-/user/org.couchdb.user:{username}
```

npm v1 token 认证，或通过 `Authorization: Bearer {token}` 头。

#### 3.1.7 内部端点

| 端点 | 说明 |
|------|------|
| `GET /-/all` | 全量包列表（couchdb 风格） |
| `GET /-/npm/ping` | 心跳 `{"ok": true}` |
| `POST /-/npm/v1/security/advisories/bulk` | 安全审计 |
| `POST /-/npm/v1/security/audits/quick` | 安全审计快速模式 |

### 3.2 实现要点

#### 路由规则

| 路径模式 | 方法 | 语义 |
|----------|------|------|
| `-/all` 或 `/-all` | GET | 全量包列表 |
| `-/npm/ping` | GET/POST | 心跳检测 |
| `-/npm/v1/security/advisories/bulk` | POST | 安全审计（返回空 `{}`） |
| `-/npm/v1/security/audits/quick` | POST | 安全审计（返回空 `{}`） |
| `{scope}/{name}/-/{tarball}` | GET | Tarball 下载 |
| `{name}` 或 `{scope}/{name}` | GET | 包元数据（版本列表 + dist-tags） |
| `{name}` 或 `{scope}/{name}` | PUT | 包发布 |

#### FetchRemote 实现

```
FetchRemote(ctx, remoteURL, path)
  → URL 编码包名: @scope/pkg → %40scope%2Fpkg
  → GET {remoteURL}/{encodedName}
  → 解析 JSON: raw["versions"] → map[string]interface{}
  → 每个版本生成一个 Artifact{Format:"npm", Kind:"version", Coordinates:{name, version}}
```

#### ArtifactKey 结构

**Tarball 下载**:
```go
ArtifactKey{
    Format: "npm",
    Coordinates: map[string]string{
        "name":    packageName,     // 如 "express" 或 "@scope/pkg"
        "version": version,         // 如 "4.18.2"
        "path":    packageName+"/-",// 固定后缀
    },
    Filename: "express-4.18.2.tgz",
}
```

**包元数据查询**:
```go
ArtifactQuery{
    Format:     "npm",
    Coordinates: map[string]string{"name": packageName},
    RemotePath: packageName,  // 必须！供 FetchRemote 回源
}
```

#### 特殊行为

- **Scoped 包**: `@scope/name` 格式，URL 编码使用 `url.PathEscape`
- **dist-tags**: 从版本列表中用 semver 排序，取最新版本作为 `latest`
- **tarball URL**: 拼接为 `{repoBase}/{name}/-/{name}-{version}.tgz`
- **repoBaseURL**: 支持反向代理（`X-Forwarded-Proto` / `X-Forwarded-Host` 头）
- **Upload**: 接收完整 npm metadata JSON，存储为 blob，版本从 metadata 中提取
- **Content-Type**: 元数据返回 `application/json`，tarball 返回 `application/octet-stream`

#### 重构要点

- `repoBaseURL()` 函数在 `plugin.go` 中定义，拼接仓库基础 URL
- tarball 路径解析通过 `strings.Split(path, "/-/")` 实现
- 版本从 filename 中反推: `strings.TrimSuffix(strings.TrimPrefix(filename, name+"-"), ".tgz")`

---

## 4. PyPI

### 4.1 协议规范

**规范来源**: [PEP 503 — Simple Repository API](https://peps.python.org/pep-0503/)、[PEP 691 — JSON-based Simple API](https://peps.python.org/pep-0691/)、[PEP 700](https://peps.python.org/pep-0700/)、[PEP 427 — Wheel](https://peps.python.org/pep-0427/)、[PEP 625](https://peps.python.org/pep-0625/)

#### 4.1.1 Simple Index（包名列表）

```
GET /simple/
```

返回仓库中所有包名的 HTML 页面：

```html
<!DOCTYPE html>
<html>
<head><title>Simple Index</title></head>
<body>
  <a href="requests/">requests</a><br>
  <a href="flask/">flask</a><br>
</body>
</html>
```

**JSON 格式** (PEP 691，通过 `Accept: application/vnd.pypi.simple.v1+json`):

```json
{
  "meta": { "api-version": "1.0" },
  "projects": [
    { "name": "requests", "url": "requests/" },
    { "name": "flask", "url": "flask/" }
  ]
}
```

#### 4.1.2 包文件列表

```
GET /simple/{package}/
```

返回指定包的所有发行文件链接：

```html
<!DOCTYPE html>
<html>
<head><title>Links for requests</title></head>
<body>
<h1>Links for requests</h1>
<a href="../../packages/62/35/.../requests-2.28.0.tar.gz#sha256=abc123">requests-2.28.0.tar.gz</a><br>
<a href="../../packages/8a/.../requests-2.28.0-py3-none-any.whl#sha256=def456">requests-2.28.0-py3-none-any.whl</a><br>
</body>
</html>
```

**JSON 格式** (PEP 691):

```json
{
  "meta": { "api-version": "1.0" },
  "files": [
    {
      "url": "../../packages/62/35/.../requests-2.28.0.tar.gz",
      "filename": "requests-2.28.0.tar.gz",
      "hashes": { "sha256": "abc123" },
      "requires-python": ">=3.7"
    }
  ]
}
```

#### 4.1.3 包名规范化 (PEP 503)

包名在 URL 中必须规范化：
- 全部转为小写
- 将 `_`、`.`、`-` 统一替换为 `-`

例如: `My_Package` → `my-package`

正则: `re.sub(r"[-_.]+", "-", name).lower()`

#### 4.1.4 文件名规范

**Wheel 文件名** (PEP 427):
```
{distribution}-{version}(-{build tag})?-{python tag}-{abi tag}-{platform tag}.whl
```
示例: `requests-2.28.0-py3-none-any.whl`

必须至少包含 5 个 `-` 分隔的段。

**Sdist 文件名** (PEP 625):
```
{distribution}-{version}.tar.gz
{distribution}-{version}.tar.bz2
{distribution}-{version}.zip
```
示例: `requests-2.28.0.tar.gz`

#### 4.1.5 哈希验证

链接中可以包含哈希片段: `#sha256=abc123def456`

支持的算法: `sha256`、`sha384`、`md5`（不推荐）

#### 4.1.6 PyPI JSON API（非 Simple API）

```
GET /pypi/{package}/json
GET /pypi/{package}/{version}/json
```

返回:
```json
{
  "info": {
    "name": "requests",
    "version": "2.28.0",
    "summary": "Python HTTP for Humans.",
    "requires_python": ">=3.7"
  },
  "releases": {
    "2.28.0": [
      {
        "filename": "requests-2.28.0.tar.gz",
        "url": "https://files.pythonhosted.org/packages/...",
        "digests": { "sha256": "..." },
        "size": 123456
      }
    ]
  }
}
```

#### 4.1.7 文件上传 (Warehouse/legacy)

```
POST /legacy/
Content-Type: multipart/form-data
```

表单字段: `:action=file_upload`、`content`（文件）、`name`、`version` 等。

Moonlight 通过 PUT `/packages/{hash}/{filename}` 简化实现。

### 4.2 实现要点

#### 路由规则

| 路径模式 | 方法 | 语义 |
|----------|------|------|
| `simple` 或 `simple/` | GET | Simple Index（包名列表） |
| `simple/{package}/` | GET | 包文件列表 |
| `packages/{hash}/{filename}` | GET | 文件下载 |
| `packages/{hash}/{filename}` | PUT | 文件上传 |
| `packages/{filename}.sha256` | GET | SHA256 校验值 |
| `pypi/{package}/json` | GET | JSON API |
| `pypi/{package}/{version}/json` | GET | 指定版本 JSON API |

#### FetchRemote 实现

```
FetchRemote(ctx, remoteURL, path)
  → 判断请求类型:
    - isSimpleIndexRequest(path): GET {remoteURL}/simple/ → parseSimpleIndex
    - isPackageListRequest(path): GET {remoteURL}/simple/{pkg}/ → parsePackageList

parseSimpleIndex(body):
  → 正则: <a href="([^"]+)/">([^<]+)</a>
  → 每个匹配生成 Artifact{Format:"pypi", Kind:"package-index", Coordinates:{name, package}}

parsePackageList(packageName, body):
  → 正则: <a href="[^"]*/packages/(([^"#]+))(?:#[^"]*)?[^>]*>([^<]+)</a>
  → 校验文件名: isValidWheelFilename 或 isValidSdistFilename
  → 提取 version: extractVersionFromFilename
  → 每个文件生成 Artifact{Format:"pypi", Kind:"package-file", Coordinates:{name, package, version, filename, path}, Properties:{remote_path}}
```

#### ArtifactKey 结构

**文件下载**:
```go
ArtifactKey{
    Format: "pypi",
    Coordinates: map[string]string{
        "package":  packageName,  // normalize 后: lowercase, _ → -
        "version":  version,
        "filename": filename,
        "path":     dir,          // 如 "packages/62/35/..."
    },
    Filename: filename,
}
```

**Simple Index 查询**:
```go
ArtifactQuery{
    Format:     "pypi",
    RemotePath: path,  // "simple" 或 "simple/"
}
```

**包文件列表查询**:
```go
ArtifactQuery{
    Format:     "pypi",
    Coordinates: map[string]string{"package": packageName},
    RemotePath: path,  // "simple/{package}/"
}
```

#### 特殊行为

- **包名规范化**: `normalizePackageName(name)` → `strings.ToLower(strings.ReplaceAll(name, "_", "-"))`
- **Wheel 文件名校验** (PEP 427): `{name}-{version}(-{build})?-{python}-{abi}-{platform}.whl`，至少 5 段
- **Sdist 文件名校验** (PEP 625): `{name}-{version}.tar.gz` / `.tar.bz2` / `.zip`
- **Simple Index 内容协商**: `Accept: application/vnd.pypi.simple` 或 `application/json` → JSON 格式；否则 HTML
- **JSON API**: 返回 `{info: {name, version}, releases: {version: [files]}}`
- **Simple Index JSON 格式**: `{meta: {api-version: "1.0"}, projects: [{name, url}]}`
- **包文件列表 JSON 格式**: `{meta: {api-version: "1.0"}, files: [{url, filename, hashes}]}`
- **SHA256 校验**: 从 `artifact.BlobRefs[0].Digest` 获取
- **路径安全校验**: `validatePyPIPath()` 禁止 `..`、`~`、`$`、反引号等危险字符

#### 重构要点

- `extractPackageNameFromFilename()` 和 `extractVersionFromFilename()` 区分 wheel 和 sdist 格式
- Wheel 文件名用 `-` 分隔，第一段是包名，第二段是版本
- Sdist 文件名用 `-` 分隔，最后一段是版本（注意 `tar.gz` 后缀要先去除）
- Simple Index HTML 中的链接格式: `<a href="{name}/">{name}</a>`
- 包文件列表 HTML 中的链接: `<a href="../../packages/{remote_path}">{filename}</a>`

---

## 5. Maven

### 5.1 协议规范

**规范来源**: [Apache Maven Repository Layout](https://maven.apache.org/repositories/layout.html)、[Apache Maven POM](https://maven.apache.org/pom.html)、[Sonatype Repository Specification](https://maven.apache.org/repository/layout.html)

#### 5.1.1 仓库目录布局

Maven 仓库采用严格的目录结构：

```
/
├── {groupId as path}/
│   └── {artifactId}/
│       ├── maven-metadata.xml          ← artifact 级元数据
│       ├── {version}/
│       │   ├── maven-metadata.xml      ← version 级元数据（SNAPSHOT 用）
│       │   ├── {artifactId}-{version}.pom
│       │   ├── {artifactId}-{version}.jar
│       │   ├── {artifactId}-{version}-sources.jar
│       │   ├── {artifactId}-{version}-javadoc.jar
│       │   └── {artifactId}-{version}.{classifier}.{ext}
│       └── ...
```

**示例**:
```
com/google/guava/guava/
├── maven-metadata.xml
├── 31.1-jre/
│   ├── guava-31.1-jre.pom
│   ├── guava-31.1-jre.jar
│   └── guava-31.1-jre-sources.jar
└── 31.1-android/
    ├── guava-31.1-android.pom
    └── guava-31.1-android.jar
```

#### 5.1.2 Artifact 坐标

Maven artifact 由三元组唯一标识：
- **groupId**: 组织标识，如 `com.google.guava`
- **artifactId**: 构件标识，如 `guava`
- **version**: 版本号，如 `31.1-jre`

在仓库路径中: `{groupId.replace('.','/')}/{artifactId}/{version}/`

#### 5.1.3 maven-metadata.xml（Artifact 级）

位于 `{groupId as path}/{artifactId}/maven-metadata.xml`：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <groupId>com.google.guava</groupId>
  <artifactId>guava</artifactId>
  <versioning>
    <latest>31.1-jre</latest>
    <release>31.1-jre</release>
    <versions>
      <version>30.0-jre</version>
      <version>31.0-jre</version>
      <version>31.1-jre</version>
      <version>31.1-android</version>
    </versions>
    <lastUpdated>20220301000000</lastUpdated>
  </versioning>
</metadata>
```

**字段说明**:
- `latest`: 仓库中最新版本（按部署时间）
- `release`: 最新非 SNAPSHOT 版本
- `versions`: 所有可用版本列表
- `lastUpdated`: 最后更新时间，格式 `yyyyMMddHHmmss`（UTC）

#### 5.1.4 maven-metadata.xml（Version 级 / SNAPSHOT）

位于 `{groupId as path}/{artifactId}/{version}/maven-metadata.xml`，用于 SNAPSHOT 版本：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <groupId>com.example</groupId>
  <artifactId>my-app</artifactId>
  <version>1.0-SNAPSHOT</version>
  <versioning>
    <snapshot>
      <timestamp>20220301.120000</timestamp>
      <buildNumber>1</buildNumber>
    </snapshot>
    <lastUpdated>20220301120000</lastUpdated>
    <snapshotVersions>
      <snapshotVersion>
        <extension>jar</extension>
        <value>1.0-20220301.120000-1</value>
        <updated>20220301120000</updated>
      </snapshotVersion>
      <snapshotVersion>
        <extension>pom</extension>
        <value>1.0-20220301.120000-1</value>
        <updated>20220301120000</updated>
      </snapshotVersion>
      <snapshotVersion>
        <extension>jar</extension>
        <classifier>sources</classifier>
        <value>1.0-20220301.120000-1</value>
        <updated>20220301120000</updated>
      </snapshotVersion>
    </snapshotVersions>
  </versioning>
</metadata>
```

**字段说明**:
- `snapshot.timestamp`: 快照时间戳 `yyyyMMdd.HHmmss`
- `snapshot.buildNumber`: 构建编号（递增整数）
- `snapshotVersions`: 每个 extension/classifier 组合的实际版本号

#### 5.1.5 文件下载

```
GET /{groupId as path}/{artifactId}/{version}/{filename}
```

直接返回文件内容。常见文件类型:
- `.pom` — POM 文件（XML）
- `.jar` — Java Archive
- `.war` — Web Archive
- `-sources.jar` — 源码包
- `-javadoc.jar` — 文档包
- `.aar` — Android Archive
- `.module` — Gradle Module Metadata

#### 5.1.6 文件上传

```
PUT /{groupId as path}/{artifactId}/{version}/{filename}
Content-Type: application/octet-stream
```

直接上传文件内容到指定路径。通常伴随上传 `.pom`、`.sha1`、`.md5` 等。

#### 5.1.7 文件删除

```
DELETE /{groupId as path}/{artifactId}/{version}/{filename}
```

删除指定文件或整个版本目录。

#### 5.1.8 搜索

```
GET /solrsearch/select?q=g:{groupId}+AND+a:{artifactId}&rows=20&wt=json
```

Maven Central 使用 Solr 提供搜索。私有仓库可能使用不同实现。

### 5.2 实现要点

#### 路由规则

| 路径模式 | 方法 | 语义 |
|----------|------|------|
| `{group}/{artifact}/maven-metadata.xml` | GET | Artifact 级元数据 |
| `{group}/{artifact}/{version}/maven-metadata.xml` | GET | Version 级元数据（SNAPSHOT） |
| `{group}/{artifact}/{version}/{file}` | GET | 构件下载 |
| `{group}/{artifact}/{version}/{file}` | PUT | 构件上传 |
| `{group}/{artifact}/{version}/{file}` | DELETE | 构件删除 |

#### FetchRemote 实现

```
FetchRemote(ctx, remoteURL, path)
  → 对 maven-metadata.xml 请求:
    → GET {remoteURL}/{path}
    → XML 解析 → mavenMetadata 结构体
    → 提取 versions 列表
    → 每个版本生成 Artifact{Format:"maven", Kind:"version", Coordinates:{group, artifact, version, base_version?}}
  → 对其他路径:
    → parseMavenPath(path) 提取坐标
    → 返回单个 Artifact{Format:"maven", Kind:"artifact", Coordinates, Properties:{filename, extension}}
```

#### ArtifactKey 结构

**路径解析** (`parseMavenPath`):
```
路径: com/google/guava/guava/31.1/guava-31.1.jar
parts: [com, google, guava, guava, 31.1, guava-31.1.jar]
parts[-1] = filename = "guava-31.1.jar"
parts[-2] = version  = "31.1"
parts[-3] = artifact = "guava"
parts[:-3] = group   = "com.google" (用 . 连接)
```

```go
ArtifactKey{
    Format: "maven",
    Coordinates: map[string]string{
        "name":     "com.google:guava",  // group:artifact
        "group":    "com.google",
        "artifact": "guava",
        "version":  "31.1",
        "filename": "guava-31.1.jar",
        "path":     "com/google/guava/guava/31.1",  // 去掉 filename
    },
    Filename:  "guava-31.1.jar",
    Extension: ".jar",
}
```

#### Metadata 解析

**路径中的版本识别**: `mavenVersionPattern = regexp.MustCompile("^\d|^SNAPSHOT")`

**metadata 路径解析** (handleMetadata):
```
路径: com/google/guava/guava/maven-metadata.xml
parts = [com, google, guava, guava, maven-metadata.xml]  // 去掉最后一段
parts = [com, google, guava, guava]
最后一段如果匹配版本模式 → 是 version 级 metadata
否则 → 是 artifact 级 metadata
```

**metadata XML 结构**:
```xml
<metadata>
  <groupId>com.google</groupId>
  <artifactId>guava</artifactId>
  <versioning>
    <latest>31.1-jre</latest>
    <release>31.1-jre</release>
    <versions>
      <version>30.0-jre</version>
      <version>31.1-jre</version>
    </versions>
    <lastUpdated>20220301000000</lastUpdated>
    <!-- SNAPSHOT 专用 -->
    <snapshot><timestamp>20220301.000000</timestamp><buildNumber>1</buildNumber></snapshot>
    <snapshotVersions>
      <snapshotVersion><extension>jar</extension><value>31.1-SNAPSHOT</value></snapshotVersion>
    </snapshotVersions>
  </versioning>
</metadata>
```

#### 特殊行为

- **Metadata 降级**: QueryArtifacts 返回空时，尝试 GetArtifact 获取缓存的 maven-metadata.xml 文件
- **SNAPSHOT 支持**: 版本包含 "SNAPSHOT" 时，生成 `<snapshot>` 和 `<snapshotVersions>` 节点
- **lastUpdated 格式**: `20060102150405`（UTC）
- **Extension 提取**: 从 filename 或 coordinates 中的 `extension` 属性获取，默认 `"jar"`
- **Classifier**: 从 properties 或 coordinates 中的 `classifier` 属性获取
- **版本排序**: `sort.Strings(versions)`，latest 取排序后的最后一个

#### 重构要点

- `parseMavenPath` 的路径解析假设 `{group}/{artifact}/{version}/{filename}` 格式
- metadata 路径需要特殊处理：最后一段可能是版本号（数字开头或 SNAPSHOT）
- SNAPSHOT 版本的 metadata 比普通版本复杂得多，需要生成 snapshotVersions
- Delete 操作支持 `ErrReadOnly` 错误码

---

## 6. Go Modules

### 6.1 协议规范

**规范来源**: [Go Module Reference — Module Proxy](https://go.dev/ref/mod#module-proxy)、[Go Module Mirror](https://go.dev/ref/mod#module-proxy)、[pkg.go.dev](https://pkg.go.dev/golang.org/x/mod/)

**Proxy 基础 URL**: `https://proxy.golang.org`（官方）

#### 6.1.1 版本发现

**查询最新版本**:
```
GET /{module}/@latest
```

返回 JSON:
```json
{
  "Version": "v1.2.3",
  "Time": "2023-01-15T10:30:00Z",
  "Origin": { ... }
}
```

- `Version`: 最新版本号（语义化版本，必须以 `v` 开头）
- `Time`: 该版本的创建时间（RFC 3339 格式）

**查询版本列表**:
```
GET /{module}/@v/list
```

返回纯文本，每行一个版本号:
```
v1.0.0
v1.1.0
v1.2.0
v1.2.3
```

- 不保证排序
- 不包含 `v` 前缀的版本和非语义化版本也会列出
- 不包含 retract 的版本

#### 6.1.2 版本元数据

```
GET /{module}/@v/{version}.info
```

返回 JSON:
```json
{
  "Version": "v1.2.3",
  "Time": "2023-01-15T10:30:00Z"
}
```

#### 6.1.3 go.mod 文件

```
GET /{module}/@v/{version}.mod
```

返回该版本的 `go.mod` 文件原始内容（纯文本）。

```
module github.com/gin-gonic/gin

go 1.20

require (
    github.com/go-playground/validator/v10 v10.14.0
    ...
)
```

#### 6.1.4 源码 zip

```
GET /{module}/@v/{version}.zip
```

返回 zip 格式的源码归档。内部目录结构:

```
{module}@{version}/
├── go.mod
├── go.sum
├── main.go
├── subpkg/
│   └── util.go
└── ...
```

- 顶层目录名为 `{module}@{version}`
- 不包含 `.git` 等 VCS 目录
- 不包含 `vendor/` 目录

#### 6.1.5 ziphash

```
GET /{module}/@v/{version}.ziphash
```

返回 zip 的 SHA-256 哈希（格式同 `go.sum`）:
```
h1:abc123def456...=
```

#### 6.1.6 URL 编码规则

模块路径中的大写字母需要编码:

| 字符 | 编码 |
|------|------|
| `A` | `!a` |
| `B` | `!b` |
| ... | ... |
| `Z` | `!z` |

示例: `github.com/Azure/azure-sdk-go` → `github.com/!azure/azure-sdk-go`

版本中的大写字母同理。

#### 6.1.7 错误响应

- **404**: 模块或版本不存在
- **410**: 模块已被 retract
- **其他 4xx/5xx**: 代理错误

Go 命令行会按 `GOPROXY` 设置依次尝试多个代理，最后 fallback 到 `direct`（直接从 VCS 拉取）。

#### 6.1.8 GOFLAGS 和 GONOSUMCHECK

- `GONOSUMDB`: 不向 sum DB 校验的模块列表
- `GONOSUMCHECK`: 不校验 checksum 的模块列表
- `GOFLAGS`: 传递给 go 命令的默认参数

### 6.2 实现要点

#### 路由规则

| 路径模式 | 方法 | 语义 |
|----------|------|------|
| `{module}/@latest` | GET | 最新版本信息 |
| `{module}/@v/list` | GET | 版本列表 |
| `{module}/@v/{version}.info` | GET | 版本信息 JSON |
| `{module}/@v/{version}.mod` | GET | go.mod 内容 |
| `{module}/@v/{version}.zip` | GET | 模块源码 zip |

#### FetchRemote 实现

```
FetchRemote(ctx, remoteURL, path)
  → 判断路径后缀:
    - /@v/list: GET {remoteURL}/{path}
      → bufio.Scanner 逐行读取版本号
      → 每行生成 Artifact{Format:"go", Kind:"version", Coordinates:{module, version}}
    - /@latest: GET {remoteURL}/{path}
      → JSON decode → {Version, Time}
      → 生成 Artifact{Format:"go", Kind:"version", Coordinates:{module, version}, Properties:{time}}
    - 其他: splitModulePath 提取 module 和 filename
      → 返回 Artifact{Format:"go", Kind:"module-file", Coordinates:{module, path}, Properties:{filename}}
```

#### ArtifactKey 结构

**模块下载**:
```go
ArtifactKey{
    Format: "go",
    Coordinates: map[string]string{
        "name":    modulePath,           // 如 "github.com/gin-gonic/gin"
        "module":  modulePath,
        "version": cleanVersion,         // 如 "v1.9.1"
        "path":    modulePath+"/@v",     // 固定后缀
        "ext":     fileType,             // "info" / "mod" / "zip"
    },
    Filename: "v1.9.1.mod",
}
```

**版本列表查询**:
```go
ArtifactQuery{
    Format:     "go",
    RemotePath: path,  // "{module}/@v/list"
    Coordinates: map[string]string{"module": modulePath},
}
```

#### 特殊行为

- **.info 文件动态生成**: 不存储 blob，直接输出 `{"Version":"v1.9.1","Time":"2023-01-01T00:00:00Z"}`
- **.mod 和 .zip**: 从存储中读取 blob 流式返回
- **Content-Type**: `.info` → `application/json`，`.mod` → `text/plain`，`.zip` → `application/zip`
- **splitModulePath**: 通过 `/@v/` 或 `/@latest` 分隔符拆分 module 路径和文件名

#### 重构要点

- Go 模块路径可以包含任意层级（如 `golang.org/x/tools/go/analysis`），不能简单 split
- `.info` 文件是特殊处理——不走存储，直接生成 JSON
- 版本格式遵循 semver（`v1.2.3`），但 FetchRemote 返回的版本可能没有 `v` 前缀

---

## 7. YUM

### 7.1 协议规范

**规范来源**: [createrepo_c](https://github.com/rpm-software-management/createrepo_c)、[DNF Documentation](https://dnf.readthedocs.io/)、[Librepo](https://github.com/rpm-software-management/librepo)

> YUM/DNF 仓库元数据没有独立的 RFC 规范文档，de facto 标准由 createrepo/createrepo_c 实现定义。

#### 7.1.1 仓库目录布局

```
/
├── repodata/
│   ├── repomd.xml                 ← 元数据索引（入口文件）
│   ├── <hash>-primary.xml.gz      ← 包元数据（压缩）
│   ├── <hash>-filelists.xml.gz    ← 文件列表（压缩）
│   ├── <hash>-other.xml.gz        ← 变更日志（压缩）
│   ├── <hash>-comps.xml           ← 包组定义
│   └── <hash>-updateinfo.xml.gz   ← 安全更新信息
├── Packages/
│   ├── nginx-1.20.1-1.el8.x86_64.rpm
│   ├── openssl-1.1.1k-2.el8.x86_64.rpm
│   └── ...
```

#### 7.1.2 repomd.xml

仓库入口文件，包含所有元数据文件的引用：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo">
  <revision>1677600000</revision>

  <data type="primary">
    <checksum type="sha256">abc123...</checksum>
    <location href="repodata/abc123-primary.xml.gz"/>
    <timestamp>1677600000</timestamp>
    <size>123456</size>
    <open-size>987654</open-size>
    <open-checksum type="sha256">def456...</open-checksum>
  </data>

  <data type="filelists">
    <checksum type="sha256">ghi789...</checksum>
    <location href="repodata/ghi789-filelists.xml.gz"/>
    <timestamp>1677600000</timestamp>
    <size>234567</size>
  </data>

  <data type="other">
    <checksum type="sha256">jkl012...</checksum>
    <location href="repodata/jkl012-other.xml.gz"/>
    <timestamp>1677600000</timestamp>
    <size>345678</size>
  </data>

  <data type="group">
    <checksum type="sha256">mno345...</checksum>
    <location href="repodata/mno345-comps.xml"/>
    <timestamp>1677600000</timestamp>
    <size>45678</size>
  </data>
</repomd>
```

**`<data>` 属性**:
- `type`: 元数据类型 — `primary`、`filelists`、`other`、`group`、`updateinfo`、`prestodelta`
- `checksum`: 文件校验值
- `location href`: 文件相对路径
- `timestamp`: Unix 时间戳
- `size`: 压缩后大小
- `open-size`: 解压后大小
- `open-checksum`: 解压后校验值

#### 7.1.3 primary.xml

核心包元数据，描述每个 RPM 包的信息：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common"
          xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <package type="rpm">
    <name>nginx</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="1.20.1" rel="1.el8"/>
    <checksum type="sha256" pkgid="YES">abc123...</checksum>
    <summary>High performance HTTP server</summary>
    <description>Nginx is a web server...</description>
    <packager>Fedora Project</packager>
    <url>http://nginx.org</url>
    <time file="1677600000" build="1677500000"/>
    <size package="1234567" installed="3456789" archive="4567890"/>
    <location href="Packages/nginx-1.20.1-1.el8.x86_64.rpm"/>
    <format>
      <rpm:license>BSD</rpm:license>
      <rpm:vendor>Fedora Project</rpm:vendor>
      <rpm:group>System Environment/Daemons</rpm:group>
      <rpm:buildhost>build.example.com</rpm:buildhost>
      <rpm:sourcerpm>nginx-1.20.1-1.el8.src.rpm</rpm:sourcerpm>
      <rpm:header-range start="0" end="1234"/>
      <rpm:provides>
        <rpm:entry name="nginx" flags="EQ" epoch="0" ver="1.20.1" rel="1.el8"/>
        <rpm:entry name="webserver"/>
      </rpm:provides>
      <rpm:requires>
        <rpm:entry name="openssl-libs"/>
        <rpm:entry name="pcre2" flags="GE" epoch="0" ver="10.32"/>
      </rpm:requires>
      <rpm:conflicts>
        <rpm:entry name="httpd"/>
      </rpm:conflicts>
    </format>
  </package>
</metadata>
```

**关键字段**:
- `name`: 包名
- `arch`: 架构（x86_64、aarch64、noarch 等）
- `version`: epoch、ver（主版本）、rel（发行号）
- `checksum`: 包文件的 SHA256 校验值
- `location href`: RPM 文件的相对路径
- `rpm:requires`: 依赖列表
- `rpm:provides`: 提供的虚拟包
- `rpm:conflicts`: 冲突包

#### 7.1.4 RPM 文件名规范

```
{name}-{version}-{release}.{arch}.rpm
```

示例: `nginx-1.20.1-1.el8.x86_64.rpm`

- `version`: 上游版本号
- `release`: 包维护者递增的发行号
- `arch`: 目标架构

#### 7.1.5 仓库更新流程

1. 下载 `repomd.xml` 获取元数据引用
2. 下载 `primary.xml.gz`（解压后解析）获取包列表
3. 根据 `location href` 下载 RPM 文件
4. 校验 `checksum`

#### 7.1.6 Delta RPM (prestodelta)

用于增量更新，减少下载量：

```xml
<prestodelta>
  <delta>
    <oldpackage name="nginx" epoch="0" ver="1.20.0" rel="1.el8"/>
    <newpackage name="nginx" epoch="0" ver="1.20.1" rel="1.el8" arch="x86_64"/>
    <filename>drpms/nginx-1.20.0-1.el8_1.20.1-1.el8.x86_64.drpm</filename>
    <sequence>abc123-def456</sequence>
    <size>12345</size>
  </delta>
</prestodelta>
```

### 7.2 实现要点

#### 路由规则

| 路径模式 | 方法 | 语义 |
|----------|------|------|
| `repodata/repomd.xml` | GET | 仓库元数据 |
| `repodata/*primary.xml*` | GET | 包索引 |
| `*.rpm` | GET | RPM 包下载 |

#### FetchRemote 实现

```
FetchRemote(ctx, remoteURL, path)
  → 对 repomd.xml 请求:
    → GET {remoteURL}/{path}
    → XML 解析 → repomdXML 结构体
    → 生成 repomd.xml 自身的 Artifact
    → 每个 <data> 生成 Artifact{Format:"yum", Kind:"metadata-ref", Coordinates:{file, type, href}}
  → 对其他路径:
    → 返回 Artifact{Format:"yum", Kind:"file", Coordinates:{file, filename, path}, Properties:{filename}}
```

#### ArtifactKey 结构

**repomd.xml**:
```go
ArtifactKey{
    Format: "yum",
    Coordinates: map[string]string{"file": "repomd.xml"},
    Filename: "repomd.xml",
}
```

**primary.xml**:
```go
ArtifactKey{
    Format: "yum",
    Coordinates: map[string]string{"file": filename},  // 如 "primary.xml.gz"
    Filename: filename,
}
```

**RPM 包**:
```go
ArtifactKey{
    Format: "yum",
    Coordinates: map[string]string{"filename": filename},
    Filename: filename,
}
```

#### 特殊行为

- **三级缓存**: handleRepomd 按优先级尝试: GetArtifact → 内存缓存 → QueryArtifacts → 动态渲染
- **内存缓存**: `cache.MemoryCache`，5 分钟 TTL，key 格式 `"yum:repomd:{repoID}:{path}"`
- **repomd.xml 动态渲染**: 从 QueryArtifacts 结果构建 XML，包含 `<data type="primary"><location href="..."/></data>`
- **primary.xml 动态渲染**: 从 QueryArtifacts 结果构建 XML，包含 `<package type="rpm"><name>...</name><version ver="..."/></package>`
- **Content-Type**: metadata 和 primary 返回 `application/xml`，RPM 返回 `application/x-rpm`

#### 重构要点

- `isPrimaryRequest` 用 `strings.Contains` 而非精确匹配，因为 primary.xml 文件名可能带压缩后缀（如 `primary.xml.gz`）
- repomd.xml 的 `<data>` type 字段在动态渲染时固定为 `"primary"`，这是简化处理
- RPM 包只支持 GET，不支持 PUT/DELETE

---

## 8. APT

### 8.1 协议规范

**规范来源**: [Debian Policy Manual](https://www.debian.org/doc/debian-policy/)、[Debian Repository Format](https://wiki.debian.org/DebianRepository/Format)、`man 5 sources.list`

#### 8.1.1 仓库目录布局

```
/
├── dists/
│   └── {codename}/                    ← 如 "jammy"、"bullseye"
│       ├── InRelease                  ← 内联签名的 Release 文件
│       ├── Release                    ← Release 文件
│       ├── Release.gpg                ← Release 的 GPG 分离签名
│       └── {component}/               ← 如 "main"、"contrib"、"non-free"
│           └── binary-{arch}/         ← 如 "binary-amd64"、"binary-arm64"
│               ├── Packages           ← 包索引（纯文本）
│               ├── Packages.gz        ← 包索引（gzip 压缩）
│               └── Packages.xz        ← 包索引（xz 压缩）
├── pool/
│   └── {component}/
│       └── {letter}/
│           └── {source}/              ← 如 "nginx"
│               ├── nginx_1.18.0-6.1_amd64.deb
│               ├── nginx_1.18.0-6.1.dsc
│               └── nginx_1.18.0-6.1.tar.xz
```

#### 8.1.2 Release 文件

描述仓库中所有索引文件的校验信息：

```
Origin: Ubuntu
Label: Ubuntu
Suite: jammy
Codename: jammy
Date: Thu, 21 Apr 2022 10:00:00 UTC
Architectures: amd64 arm64 i386
Components: main restricted universe multiverse
Description: Ubuntu 22.04 LTS
SHA256:
 abc123... 12345 main/binary-amd64/Packages
 def456... 23456 main/binary-amd64/Packages.gz
 ghi789... 34567 main/binary-amd64/Release
 jkl012... 45678 main/binary-amd64/Packages.xz
```

**字段说明**:
- `Origin`: 仓库来源
- `Suite`: 发行套件名（如 `stable`、`jammy-updates`）
- `Codename`: 代号（如 `jammy`、`bullseye`）
- `Architectures`: 支持的架构列表
- `Components`: 组件列表
- `SHA256` / `SHA1` / `MD5`: 各索引文件的校验值、大小和路径

#### 8.1.3 InRelease 文件

`Release` 文件的内联签名版本（clearsigned），签名嵌入同一文件：

```
-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

Origin: Ubuntu
Label: Ubuntu
Suite: jammy
Codename: jammy
...
SHA256:
 abc123... 12345 main/binary-amd64/Packages
 ...
-----BEGIN PGP SIGNATURE-----

iQEzBAABCAAdFiEE...
-----END PGP SIGNATURE-----
```

#### 8.1.4 Packages 文件

每个包一条记录，记录间用空行分隔。每行格式 `Key: Value`：

```
Package: nginx
Version: 1.18.0-6.1
Architecture: amd64
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Installed-Size: 1234
Depends: libc6 (>= 2.17), libpcre3, libssl1.1 (>= 1.1.0), zlib1g (>= 1:1.1.4)
Conflicts: nginx-common
Replaces: nginx-common
Provides: httpd, http-server
Section: httpd
Priority: optional
Filename: pool/main/n/nginx/nginx_1.18.0-6.1_amd64.deb
Size: 567890
MD5sum: abc123def456
SHA256: ghi789jkl012mno345pqr678stu901vwx234yza567
Description: small, powerful, scalable web/proxy server
 Nginx is a web server that can also be used as a reverse proxy,
 load balancer, and HTTP cache.
```

**关键字段**:
- `Package`: 包名
- `Version`: 版本号（Debian 版本格式: `[epoch:]upstream_version[-debian_revision]`）
- `Architecture`: 架构
- `Depends`: 依赖列表（可以有版本约束、替代依赖）
- `Filename`: `.deb` 文件的相对路径（相对于仓库根目录）
- `Size`: 文件大小（字节）
- `SHA256` / `MD5sum`: 文件校验值
- `Provides`: 提供的虚拟包
- `Conflicts` / `Replaces` / `Breaks`: 冲突/替换/破坏关系

#### 8.1.5 .deb 文件名规范

```
{name}_{version}_{arch}.deb
```

示例: `nginx_1.18.0-6.1_amd64.deb`

- `name`: 包名
- `version`: Debian 版本格式
- `arch`: 架构（amd64、arm64、all 等）

#### 8.1.6 sources.list 格式

APT 源配置文件 `/etc/apt/sources.list`:

```
deb http://archive.ubuntu.com/ubuntu/ jammy main restricted
deb http://archive.ubuntu.com/ubuntu/ jammy-updates main restricted
deb http://security.ubuntu.com/ubuntu/ jammy-security main restricted
deb-src http://archive.ubuntu.com/ubuntu/ jammy main restricted
```

格式: `deb[-src] {url} {codename} {component...}`

- `deb`: 二进制包
- `deb-src`: 源码包
- `{url}`: 仓库根 URL
- `{codename}`: 发行代号
- `{component}`: 组件（main、contrib 等）

#### 8.1.7 APT 更新流程

1. 下载 `InRelease`（或 `Release` + `Release.gpg`）
2. 验证 GPG 签名
3. 根据 `SHA256` 校验列表下载 `Packages.gz`
4. 解压 `Packages.gz`，解析包列表
5. 对比本地已安装包，计算需要更新的包
6. 下载并安装 `.deb` 文件

### 8.2 实现要点

#### 路由规则

| 路径模式 | 方法 | 语义 |
|----------|------|------|
| `*InRelease` | GET | 签名的 Release 文件 |
| `*Release` | GET | Release 文件 |
| `*Release.gpg` | GET | GPG 签名 |
| `*Packages*` | GET | 包索引 |
| `*.deb` | GET | Debian 包下载 |

#### FetchRemote 实现

```
FetchRemote(ctx, remoteURL, path)
  → 对 Packages 请求:
    → GET {remoteURL}/{path}
    → parsePackagesIndex(content): 逐行解析 key-value 对
    → 每个包生成 Artifact{Format:"apt", Kind:"package", Coordinates:{package, name, version, filename}, Properties:{filename, remote_path}}
  → 对 InRelease/Release:
    → 返回 Artifact{Format:"apt", Kind:"release", Coordinates:{file}, Properties:{filename}}
  → 对其他路径:
    → 返回 Artifact{Format:"apt", Kind:"package", Coordinates:{filename}, Properties:{filename}}
```

#### Packages Index 解析

```
parsePackagesIndex(content):
  → 逐行处理，空行分隔不同包
  → 每行格式: "Key: Value"
  → 提取字段: Package, Version, Filename
  → Package 字段值作为 package 坐标
  → Filename 的 basename 作为 filename 坐标
  → 完整 Filename 作为 remote_path 属性
```

#### ArtifactKey 结构

**InRelease/Release**:
```go
ArtifactKey{
    Format: "apt",
    Coordinates: map[string]string{"file": filename},  // 如 "InRelease"
    Filename: filename,
}
```

**Packages 索引**:
```go
ArtifactKey{
    Format: "apt",
    Coordinates: map[string]string{"file": filename},  // 如 "Packages"
    Filename: filename,
}
```

**.deb 包**:
```go
ArtifactKey{
    Format: "apt",
    Coordinates: map[string]string{"filename": filename},
    Filename: filename,
}
```

#### 特殊行为

- **三级缓存**: handlePackages 按优先级: GetArtifact → 内存缓存 → QueryArtifacts → 动态渲染
- **内存缓存**: `cache.MemoryCache`，5 分钟 TTL，key 格式 `"apt:packages:{repoID}:{path}"`
- **Packages 动态渲染**: 从 QueryArtifacts 结果构建 Debian Packages 格式文本
- **Content-Type**: Packages 返回 `text/plain; charset=utf-8`，InRelease 返回 `text/plain`，.deb 返回 `application/vnd.debian.binary-package`
- **路径判断**: `isPackagesRequest` 用 `strings.Contains(path, "Packages")`，匹配 Packages.gz 等变体

#### 重构要点

- Packages 文件格式是纯文本 key-value，用空行分隔记录
- Filename 字段可能是完整路径（如 `pool/main/n/nginx/nginx_1.18.0-6.1_amd64.deb`），需要 `filepath.Base` 提取文件名
- APT 只支持 GET，不支持 PUT/DELETE（.deb 包）

---

## 9. Generic (Raw)

### 9.1 协议规范

Generic 格式没有标准协议规范，它是通用的文件存储协议，通常由 HTTP 静态文件服务器（如 nginx、Apache、MinIO）提供。

#### 9.1.1 行为约定

- **GET**: 下载文件，返回文件内容和对应的 Content-Type
- **PUT**: 上传文件，将请求体存储为文件
- **DELETE**: 删除文件
- **目录浏览**: 部分服务器支持 HTML 目录列表

#### 9.1.2 nginx 自动目录列表

nginx 的 `autoindex on` 配置生成的目录列表 HTML：

```html
<html>
<head><title>Index of /packages/</title></head>
<body>
<h1>Index of /packages/</h1>
<hr><pre><a href="../">../</a>
<a href="v1.0.0/">v1.0.0/</a>                          21-Jan-2024 10:00  -
<a href="v2.0.0/">v2.0.0/</a>                          15-Mar-2024 14:30  -
<a href="release-notes.txt">release-notes.txt</a>            15-Mar-2024 14:30  1234
</pre><hr></body>
</html>
```

特征:
- `href` 属性包含文件/目录链接
- 目录以 `/` 结尾
- 包含 `../` 父目录链接
- 包含文件大小和时间戳

#### 9.1.3 Apache 自动目录列表

Apache 的 `Options +Indexes` 生成的 HTML：

```html
<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html>
<head><title>Index of /packages</title></head>
<body>
<h1>Index of /packages</h1>
<table>
<tr><td><img src="/icons/back.gif" alt="[PARENTDIR]"></td>
    <td><a href="/">Parent Directory</a></td></tr>
<tr><td><img src="/icons/folder.gif" alt="[DIR]"></td>
    <td><a href="v1.0.0/">v1.0.0/</a></td></tr>
<tr><td><img src="/icons/text.gif" alt="[TXT]"></td>
    <td><a href="readme.txt">readme.txt</a></td><td>1234</td></tr>
</table>
</body></html>
```

#### 9.1.4 MinIO/S3 目录列表

MinIO 和 S3 的 ListObjects API 返回 XML：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Name>my-bucket</Name>
  <Prefix>packages/</Prefix>
  <Contents>
    <Key>packages/readme.txt</Key>
    <Size>1234</Size>
    <LastModified>2024-01-15T10:00:00Z</LastModified>
  </Contents>
  <CommonPrefixes>
    <Prefix>packages/v1.0.0/</Prefix>
  </CommonPrefixes>
</ListBucketResult>
```

### 9.2 实现要点

#### 路由规则

| 路径模式 | 方法 | 语义 |
|----------|------|------|
| `{任意路径}` | GET | 文件下载 |
| `{任意路径}` | PUT | 文件上传 |
| `{任意路径}` | DELETE | 文件删除 |

#### FetchRemote 实现

```
FetchRemote(ctx, remoteURL, path)
  → GET {remoteURL}/{path}
  → 如果 Content-Type 是 text/html:
    → parseDirectoryListing: 解析 href="..." 链接
    → 目录链接 → Artifact{Kind:"directory"}
    → 文件链接 → Artifact{Kind:"file"}
    → 跳过 "../"、"/"、查询字符串、绝对 URL
  → 否则（直接文件）:
    → 返回 Artifact{Format:"generic", Kind:"file", Coordinates:{name, path}, Properties:{filename}}
```

#### ArtifactKey 结构

```go
ArtifactKey{
    Format: "generic",
    Coordinates: map[string]string{
        "name": filename,        // 如 "readme.txt"
        "path": dir,             // 如 "docs/v1"，根目录时为 ""
    },
    Filename:  filename,
    Extension: filepath.Ext(filename),  // 如 ".txt"
}
```

#### 特殊行为

- **路径解析**: `filepath.Base(path)` 提取文件名，`filepath.Dir(path)` 提取目录
- **Content-Type 检测**: 根据扩展名映射 `.txt` → `text/plain`，`.json` → `application/json`，`.xml` → `application/xml`，`.zip` → `application/zip`，默认 `application/octet-stream`
- **目录列表解析**: 非正则方式，通过 `strings.Split(html, "href=")` 逐段提取链接
- **Upload**: 直接存储 blob，坐标只有 `name` 和 `path`

#### 重构要点

- Generic 是最简单的格式，没有复杂的元数据结构
- 目录路径用 `.` 表示根目录，需要转换为空字符串
- `dir == "."` 的判断在多处出现，确保根目录正确处理

---

## 10. 通用检查清单

### 10.1 新增/修改 Plugin 时

- [ ] `Handle()` 中无 `http.Get` / `http.Post` 调用
- [ ] `Handle()` 中无 `ctx.Repository.Type == "proxy"` 判断
- [ ] `Handle()` 中无 `*GroupRuntime` / `*ProxyRuntime` 类型断言
- [ ] 实现 `RemoteFetcher` 接口（FetchRemote 方法）
- [ ] `QueryArtifacts` 调用包含 `RemotePath` 字段
- [ ] 所有 HTTP 调用使用 `ctx.Request.Context()` 而非 `context.Background()`
- [ ] **不自建 `http.Client`**：`NewXxxPlugin()` 中的 `httpClient` 仅作零值占位，实际使用的是 `SetHTTPClient()` 注入的 client
- [ ] `SetHTTPClient` 方法存在且正确注入
- [ ] `Name()` 返回值与 `main.go` 中注册的 key 一致

### 10.2 DI 注册时 (`main.go`)

- [ ] `plugin.SetHTTPClient(pluginHTTPClient)` — 从 `proxy.TransportManager` 获取的共享 client
- [ ] `repositoryRouter.RegisterPlugin("format", plugin)` 注册
- [ ] `fetchers` map 中包含该格式的 RemoteFetcher
- [ ] `initRepoRuntimes` 中正确传递 fetcher 和 httpClient

### 10.3 ArtifactKey 唯一性

- [ ] Coordinates 包含足够字段确保同仓库内唯一
- [ ] 不同格式的 Coordinates key 不冲突
- [ ] `ArtifactKey.String()` 正确序列化（Coordinates 按 key 排序）

### 10.4 上传流程

```go
session, err := repoRuntime.BeginUpload(ctx, UploadRequest{...})
blobRef, err := session.PutBlob(ctx, body)
artifact := &Artifact{...}
err := session.PutArtifact(ctx, artifact)
err := session.Commit(ctx)
// 任何步骤失败都要 session.Abort(ctx)
```

### 10.5 错误处理

- `ErrNotFound` → HTTP 404
- `ErrReadOnly` → HTTP 405 Method Not Allowed
- `ErrBlocked` → 直接返回 error（由 Router 层处理）
- 其他错误 → HTTP 500
