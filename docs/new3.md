通用制品仓库系统最终架构设计（最终收敛版）

⸻

一、系统目标

实现一个：

企业级多协议制品仓库平台

类似：

* Sonatype 的 Nexus
* JFrog Artifactory
* VMware Harbor（OCI 部分）

支持：

协议	Hosted	Proxy	Group
Maven	✅	✅	✅
npm	✅	✅	✅
PyPI	✅	✅	✅
OCI/Docker	✅	✅	✅
Go Module	✅	✅	✅
NuGet	✅	✅	✅
Helm	✅	✅	✅
APT	✅	✅	✅
YUM/DNF	✅	✅	✅
Cargo	✅	✅	✅
Conan	✅	✅	✅
RubyGems	✅	✅	✅
Raw	✅	✅	✅

⸻

二、最终核心思想（最重要）

整个系统本质：

一个：

“基于 Artifact Graph + CAS Blob 的 Repository Runtime Platform”

不是：

文件服务器

也不是：

HTTP 下载站

⸻

三、核心概念（必须理解）

⸻

1. Artifact（制品）

表示：

一个逻辑包对象

例如：

⸻

Maven

foo-1.0.jar
foo-1.0.pom

⸻

OCI

manifest
config
layer

⸻

PyPI

foo-1.0.whl

⸻

Artifact 是：

系统核心逻辑对象

⸻

2. Blob（物理二进制）

Blob：

表示：

真正存储的字节流

例如：

* jar
* tar.gz
* wheel
* OCI layer

⸻

Blob 不关心协议

Blob：

只是：

immutable binary object

⸻

3. CAS（Content Addressable Storage）

CAS：

表示：

基于内容 hash 存储

例如：

sha256:xxxx

作为唯一 key。

⸻

为什么必须 CAS

支持：

* dedupe
* immutable storage
* OCI layer reuse
* GC
* replication

⸻

4. ArtifactGraph（制品图）

这是：

系统真正核心。

表示：

Artifact 之间的关系网络

⸻

例如 Maven

pom -> dependency
snapshot -> timestamp build

⸻

OCI

manifest -> layer
manifest -> config
index -> manifest

⸻

npm

dist-tag -> version

⸻

所以：

协议本质：

都是：

artifact relation graph

⸻

5. Projection（协议视图）

Projection：

表示：

“协议暴露给客户端的视图”

不是：

source-of-truth

⸻

例如：

⸻

PyPI

/simple/foo/

生成 HTML。

⸻

Maven

maven-metadata.xml

⸻

OCI

tags/list

⸻

Projection 是：

protocol-specific view

⸻

6. RepositoryRuntime（仓库运行时）

这是：

仓库行为核心。

负责：

* proxy
* group
* cache
* stale
* refresh
* merge
* consistency

⸻

Runtime 不关心协议

Runtime：

不知道：

* XML
* JSON
* HTML
* OCI

⸻

7. ProtocolPlugin（协议插件）

负责：

协议语义

包括：

* path grammar
* upload/download grammar
* metadata grammar
* projection grammar
* remote protocol

⸻

四、最终架构（最终版）

                    ┌────────────────────┐
                    │       HTTP         │
                    └─────────┬──────────┘
                              │
                    ┌─────────▼──────────┐
                    │    ProtocolPlugin  │
                    │  (protocol layer)  │
                    └─────────┬──────────┘
                              │
                    ┌─────────▼──────────┐
                    │ RepositoryRuntime  │
                    │ (repository layer) │
                    └──────┬─────┬───────┘
                           │     │
          ┌────────────────┘     └────────────────┐
          │                                       │
 ┌────────▼─────────┐                 ┌──────────▼──────────┐
 │ Artifact Graph   │                 │ Projection Engine   │
 └────────┬─────────┘                 └──────────┬──────────┘
          │                                       │
 ┌────────▼─────────┐                 ┌──────────▼──────────┐
 │ BlobStore (CAS)  │                 │ Generated Artifact  │
 └──────────────────┘                 └─────────────────────┘

⸻

五、最终分层职责（最重要）

⸻

1. ProtocolPlugin

解决：

协议如何工作

负责：

* 协议路径解析
* 协议请求处理
* remote registry 协议
* metadata parse/render
* projection render

⸻

2. RepositoryRuntime

解决：

仓库如何工作

负责：

* hosted/proxy/group
* cache policy
* refresh policy
* merge policy
* stale check

⸻

3. ArtifactGraph

解决：

系统保存什么

⸻

4. BlobStore

解决：

二进制怎么存

⸻

六、最终核心接口（最终版）

⸻

ProtocolPlugin

最终：

只保留：

type ProtocolPlugin interface {
    Name() string
    Handle(
        ctx *RequestContext,
        runtime RepositoryRuntime,
    ) error
}

⸻

为什么只保留 Handle

因为：

协议本质：

就是：

request/response grammar

协议：

天然应该：

完整自处理。

⸻

但：

不能绕过 Runtime。

这是关键。

⸻

七、Handle 的职责

Handle：

负责：

⸻

1. 解析协议路径

例如：

⸻

PyPI

/simple/foo/

⸻

OCI

/v2/nginx/manifests/latest

⸻

Maven

/com/foo/bar/1.0/bar-1.0.jar

⸻

2. 识别协议语义

例如：

simple index
manifest request
artifact download
upload session

⸻

3. 调用 Runtime API

例如：

runtime.QueryArtifacts(...)
runtime.GetArtifact(...)
runtime.RenderProjection(...)
runtime.BeginUpload(...)

⸻

4. render 协议响应

例如：

* XML
* JSON
* HTML
* OCI manifest

⸻

八、为什么 Runtime 必须保留

因为：

仓库行为：

必须统一。

⸻

例如：

⸻

proxy cache

应该统一。

⸻

stale policy

应该统一。

⸻

group merge

应该统一。

⸻

consistency

应该统一。

⸻

九、否则会发生什么

如果 plugin 自己做：

cache
proxy
group
refresh

最终：

每个协议都有自己的 repository system

系统会崩。

⸻

十、最终 Runtime 接口

type RepositoryRuntime interface {
    GetArtifact(
        ctx context.Context,
        key ArtifactKey,
    ) (*Artifact, error)
    QueryArtifacts(
        ctx context.Context,
        query ArtifactQuery,
    ) ([]*Artifact, error)
    RenderProjection(
        ctx context.Context,
        query ProjectionQuery,
    ) (*ProjectionResult, error)
    BeginUpload(
        ctx context.Context,
        request UploadRequest,
    ) (UploadSession, error)
    OpenRemote(
        ctx context.Context,
        request RemoteOpenRequest,
    ) (*RemoteResponse, error)
}

⸻

十一、Runtime 类型

⸻

HostedRuntime

本地仓库。

⸻

ProxyRuntime

代理仓库。

支持：

* remote fetch
* cache
* stale refresh

⸻

GroupRuntime

组合仓库。

支持：

* merge
* shadowing
* priority

⸻

不透明上游流（OpenRemote）

`RepositoryRuntime.OpenRemote` 是 Runtime 对 Plugin 开放的受限能力，用于不适合归一化为 Artifact 的 GET/HEAD 上游响应流。

Plugin 只提供仓库内相对路径、方法和请求头，不能读取仓库配置、判断 hosted/proxy/group、构造上游 URL 或自行执行 HTTP。Runtime 统一负责 URL 解析、传输、请求/响应头、缓存与失败策略；Plugin 只按协议写回 Runtime 返回的未读取响应流。

它不替代 `RemoteFetcher`：

* `RemoteFetcher`：Runtime 回调 Plugin 拉取并归一化远端 metadata 为 Artifact，供语义查询、缓存和 merge 使用。
* `OpenRemote`：不解析、不写入 ArtifactGraph，适用于不透明流。

Group 对不透明流按成员优先级返回第一个支持的 proxy 响应，不合并响应体。特别是 group 的 PyPI 根 `/simple/` 只是浏览/探测结果，不保证是完整包目录；`/simple/{package}/` 等包级路径仍走语义 Runtime 查询和既有 merge 策略。

⸻

十二、Artifact 数据结构

type Artifact struct {
    ID string
    RepositoryID string
    Format string
    Kind string
    Coordinates map[string]string
    Properties map[string]string
    Relations []ArtifactRelation
    Blobs []BlobRef
    CreatedAt time.Time
}

⸻

十三、ArtifactRelation

type ArtifactRelation struct {
    Type string
    TargetArtifactID string
}

⸻

十四、BlobStore

type BlobStore interface {
    Put(
        ctx context.Context,
        blob Blob,
    ) (BlobRef, error)
    Get(
        ctx context.Context,
        ref BlobRef,
    ) (io.ReadCloser, error)
    Delete(
        ctx context.Context,
        ref BlobRef,
    ) error
}

⸻

十五、CAS 布局

blobs/
  sha256/
    ab/
      cd/
        abcdef...

⸻

十六、Projection

type ProjectionResult struct {
    Dynamic bool
    Content []byte
    Artifact *Artifact
}

⸻

Dynamic Projection

动态生成：

PyPI simple
npm metadata
docker tags/list

⸻

Materialized Projection

生成后存储：

Packages.gz
repodata.xml

⸻

十七、UploadSession（事务上传）

type UploadSession interface {
    PutBlob(
        ctx context.Context,
        blob Blob,
    ) error
    PutArtifact(
        ctx context.Context,
        artifact *Artifact,
    ) error
    Commit(
        ctx context.Context,
    ) error
    Abort(
        ctx context.Context,
    ) error
}

⸻

为什么必须 UploadSession

因为：

⸻

Maven deploy

需要：

jar
pom
checksum
signature

一起提交。

⸻

OCI push

需要：

layers
config
manifest

manifest 最后提交。

⸻

十八、最终请求流程（核心）

⸻

PyPI Simple 请求

GET /repository/pypi/simple/foo/

⸻

Router

找到：

repo = pypi
plugin = pypi
runtime = proxy

⸻

plugin.Handle()

解析：

simple index request

⸻

runtime.RenderProjection()

⸻

ProxyRuntime

发现：

metadata stale

⸻

Runtime 内部触发回源

plugin callback

⸻

Plugin 拉取远程 simple

https://pypi.org/simple/foo/

⸻

Plugin normalize

HTML：

→

Artifact Graph

⸻

Runtime 更新 graph

⸻

Plugin render HTML

⸻

返回客户端

⸻

PyPI 根索引不透明流（例外路径）

请求：

GET /repository/pypi/simple/

Plugin 识别为根索引浏览请求后：

```
plugin.Handle()
  → runtime.OpenRemote("simple/", GET, requestHeaders)
  → ProxyRuntime 在 Runtime 内部解析上游 URL 并打开响应流
  → Plugin 透传状态、端到端响应头和 body
```

这条路径不经过 `RemoteFetcher`、ArtifactGraph 或 metadata cache，也不在 Plugin 中执行 HTTP。对于 group，Runtime 按成员优先级选择第一个支持的 proxy 响应，不合并多个根索引；客户端不能将该响应视为完整 catalog。包级 simple 路径继续使用上文的语义查询/回源流程。

⸻

十九、OCI Pull 流程

⸻

请求：

GET /v2/nginx/manifests/latest

⸻

plugin.Handle()

解析：

image=nginx
tag=latest

⸻

runtime.GetArtifact()

⸻

ProxyRuntime

发现：

manifest miss

⸻

Runtime 回调 Plugin

拉取：

remote manifest

⸻

normalize

⸻

graph.Upsert()

⸻

BlobStore.Put()

⸻

plugin.Render()

输出：

OCI manifest

⸻

二十、Proxy 回源机制（关键）

很多协议：

都有：

metadata request
artifact request

两种生命周期。

⸻

metadata

可能：

TTL：

5m
30m
1h

⸻

artifact/blob

通常 immutable。

⸻

所以：

Runtime：

统一：

stale policy

而：

Plugin：

负责：

remote protocol

⸻

二十一、最终 Gin 路由

router.Any(
    "/repository/:repo/*path",
    handler.Handle,
)
router.Any(
    "/v2/*path",
    handler.HandleOCI,
)

⸻

二十二、统一入口 Handler

func Handle(c *gin.Context) {
    repo := resolveRepository(c)
    plugin := pluginManager.Get(repo.Format)
    runtime := runtimeFactory.Get(repo)
    plugin.Handle(
        NewRequestContext(c),
        runtime,
    )
}

⸻

二十三、最终目录结构

internal/
    core/
        runtime/
        graph/
        blob/
        projection/
        transaction/
        repository/
        cache/
        events/
    plugins/
        maven/
        npm/
        pypi/
        oci/
        apt/
        yum/
    api/
        http/
    worker/
        projection/
        gc/

⸻

二十四、最终 Plugin 结构（推荐）

⸻

Maven

plugins/maven/
    plugin.go
    metadata.go
    snapshot.go
    upload.go

⸻

OCI

plugins/oci/
    plugin.go
    manifest.go
    auth.go
    upload.go

⸻

PyPI

plugins/pypi/
    plugin.go
    simple.go
    wheel.go

⸻

二十五、最终数据库设计

⸻

repositories

CREATE TABLE repositories (
    id BIGSERIAL PRIMARY KEY,
    name TEXT UNIQUE,
    format TEXT,
    type TEXT,
    config JSONB,
    created_at TIMESTAMP
);

⸻

artifacts

CREATE TABLE artifacts (
    id BIGSERIAL PRIMARY KEY,
    repository_id BIGINT,
    format TEXT,
    kind TEXT,
    coordinates JSONB,
    properties JSONB,
    created_at TIMESTAMP
);

⸻

artifact_relations

CREATE TABLE artifact_relations (
    source_id BIGINT,
    target_id BIGINT,
    relation_type TEXT
);

⸻

blobs

CREATE TABLE blobs (
    id BIGSERIAL PRIMARY KEY,
    algorithm TEXT,
    digest TEXT UNIQUE,
    size BIGINT,
    storage_path TEXT,
    created_at TIMESTAMP
);

⸻

artifact_blobs

CREATE TABLE artifact_blobs (
    artifact_id BIGINT,
    blob_id BIGINT,
    role TEXT,
    position INTEGER
);

⸻

二十六、最终最重要原则（核心）

⸻

1. 协议复杂性必须封装

进入：

ProtocolPlugin

⸻

2. 仓库行为必须统一

进入：

RepositoryRuntime

⸻

3. Metadata 必须 graph 化

因为：

协议本质：

是 artifact graph。

⸻

4. Blob 必须 immutable CAS

这是工业标准。

⸻

5. Projection 不是 source-of-truth

Projection：

只是：

protocol view

⸻

二十七、最终真正边界（最重要）

⸻

ProtocolPlugin

解决：

协议如何表达

⸻

RepositoryRuntime

解决：

仓库如何运行

⸻

ArtifactGraph

解决：

系统保存什么

⸻

BlobStore

解决：

数据如何存储

⸻

二十八、最终一句话（真正本质）

整个系统最终本质：

一个：

“基于 Artifact Graph + CAS Blob 的多协议 Repository Runtime Platform”
