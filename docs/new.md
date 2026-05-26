好，我们现在已经有：

* RepositoryRouter
* PathResolver
* Adapter
* RepositoryChain
* ProxyNode
* BlobStore(CAS)

接下来真正该进入：

“工业级细化”

也就是：

真正能落地开发的系统设计

我们现在往：

Nexus/Artifactory 级别

继续推进。

这次重点细化：

⸻

一、真正的请求生命周期

之前还比较抽象。

现在细化到：

一个 HTTP 请求到底怎么走。

⸻

二、完整请求链路（最终版）

以：

GET /repositories/maven-central/com/foo/demo/1.0/demo.jar

为例。

真正链路：

Gin Engine
    ↓
AccessLog Middleware
    ↓
Auth Middleware
    ↓
RepositoryRouter
    ↓
PathResolver
    ↓
RepositoryManager
    ↓
Build RepositoryChain
    ↓
Get Adapter
    ↓
Adapter Parse Request
    ↓
Build ArtifactKey
    ↓
RepositoryChain.Fetch
    ↓
Hosted/Proxy Node
    ↓
Metadata Lookup
    ↓
Blob Lookup
    ↓
Remote Fetch(if needed)
    ↓
Streaming Response

这里：

每层职责必须绝对清晰。

⸻

三、真正核心对象

很多系统失败：

因为对象边界混乱。

我们重新定义核心对象。

⸻

四、ArtifactKey（超级核心）

之前没细化够。

真正系统：

ArtifactKey 才是核心。

⸻

五、ArtifactKey 真正设计

type ArtifactKey struct {
    RepositoryID string
    Format string
    Coordinates map[string]string
    Filename string
    Extension string
}

⸻

六、为什么它重要

因为：

整个系统最终都围绕：

ArtifactKey

运转。

包括：

* metadata
* blob
* cache
* remote fetch
* search
* ACL

⸻

七、真正协议差异

Maven

{
  "group": "com.foo",
  "artifact": "demo",
  "version": "1.0.0"
}

Docker

{
  "image": "nginx",
  "tag": "latest"
}

Go

{
  "module": "github.com/gin-gonic/gin",
  "version": "v1.0.0"
}

最终：

都统一成 ArtifactKey。

⸻

八、真正 Adapter 作用

Adapter：

本质：

Request → ArtifactKey

这是最重要的理解。

不是：

处理上传下载

⸻

九、真正 MavenAdapter

func (m *MavenAdapter) Handle(
    ctx *RequestContext,
) error {
    path := ctx.RepositoryPath
    gav := ParseMavenGAV(path)
    key := ArtifactKey{
        RepositoryID: ctx.Repository.ID,
        Format: "maven",
        Coordinates: map[string]string{
            "group": gav.Group,
            "artifact": gav.Artifact,
            "version": gav.Version,
        },
        Filename: gav.FileName,
        Extension: gav.Extension,
    }
    switch ctx.Request.Method {
    case http.MethodGet:
        return m.handleDownload(ctx, key)
    case http.MethodPut:
        return m.handleUpload(ctx, key)
    }
    return ErrMethodNotAllowed
}

现在：

adapter 非常纯。

⸻

十、RepositoryChain 真正细化

之前：

Fetch()

还太简单。

真正需要：

⸻

十一、RepositoryChain 接口

type RepositoryChain interface {
    Fetch(
        ctx context.Context,
        key ArtifactKey,
    ) (*Artifact, error)
    Put(
        ctx context.Context,
        artifact *Artifact,
    ) error
    Delete(
        ctx context.Context,
        key ArtifactKey,
    ) error
}

⸻

十二、为什么必须统一 Put/Delete

因为：

protocol ≠ repository topology

例如：

Docker upload

仍然可能：

group repo
    ↓
hosted repo

⸻

十三、真正的 GroupRepo 规则

这是个大坑。

Group Repository：

GET

支持：

按顺序查找

PUT

只能：

路由到 hosted

所以：

GroupRepo 其实有：

WritableRepository

配置。

⸻

十四、真正 GroupNode

type GroupNode struct {
    members []RepositoryNode
    writable RepositoryNode
}

⸻

十五、真正 Fetch 逻辑

func (g *GroupNode) Fetch(
    ctx context.Context,
    key ArtifactKey,
) (*Artifact, error) {
    for _, node := range g.members {
        artifact, err := node.Fetch(ctx, key)
        if err == nil {
            return artifact, nil
        }
    }
    return nil, ErrNotFound
}

⸻

十六、真正 Put 逻辑

func (g *GroupNode) Put(
    ctx context.Context,
    artifact *Artifact,
) error {
    if g.writable == nil {
        return ErrReadOnly
    }
    return g.writable.Put(
        ctx,
        artifact,
    )
}

⸻

十七、ProxyNode 真正复杂点

之前：

只是 fetch。

实际上：

proxy node 要维护状态机。

⸻

十八、Remote Artifact State

type RemoteArtifactState int
const (
    StateMissing RemoteArtifactState = iota
    StateFetching
    StateReady
    StateCorrupted
)

⸻

十九、为什么？

因为：

回源可能失败。

例如：

* 下载中断
* checksum mismatch
* partial blob
* remote timeout

必须避免：

脏缓存

⸻

二十、BlobStore 真正实现

很多人会犯错：

blob store == 文件系统

不是。

真正成熟 BlobStore：

append-only CAS

⸻

二十一、真正 BlobStore 接口

type BlobStore interface {
    Put(
        reader io.Reader,
    ) (BlobRef, error)
    Open(
        ref BlobRef,
    ) (io.ReadCloser, error)
    Stat(
        ref BlobRef,
    ) (*BlobMetadata, error)
    Delete(
        ref BlobRef,
    ) error
}

⸻

二十二、BlobRef

type BlobRef struct {
    Algorithm string
    Digest string
    Size int64
}

⸻

二十三、为什么必须 CAS

因为：

Docker layer dedupe

比如：

ubuntu
nginx
redis

大量 layer 相同。

CAS 是必须。

⸻

二十四、真正 MetadataStore

之前不够细。

⸻

二十五、真正 Metadata

type ArtifactMetadata struct {
    ID string
    RepositoryID string
    Format string
    Coordinates map[string]string
    BlobRefs []BlobRef
    Properties map[string]string
    CreatedAt time.Time
    UpdatedAt time.Time
}

⸻

二十六、为什么 BlobRefs 是数组

因为：

Docker manifest

一个 artifact：

引用多个 layer。

⸻

二十七、真正 RemoteClient

注意：

RemoteClient 也必须协议化。

不要：

Download(url)

应该：

type RemoteClient interface {
    FetchMetadata(
        ctx context.Context,
        key ArtifactKey,
    ) (*RemoteMetadata, error)
    FetchBlob(
        ctx context.Context,
        key ArtifactKey,
    ) (io.ReadCloser, error)
}

⸻

二十八、为什么要拆 metadata/blob

因为：

Maven snapshot

只更新 metadata。

⸻

二十九、真正缓存系统

真正系统：

metadata cache

和

blob cache

完全分离。

⸻

三十、为什么？

因为：

metadata 更新频率 >> blob

例如：

maven-metadata.xml

经常变。

jar 基本不变。

⸻

三十一、真正 CachePolicy

type CachePolicy struct {
    MetadataTTL time.Duration
    BlobTTL time.Duration
    NegativeTTL time.Duration
    MaxBlobSize int64
    SnapshotRefresh bool
}

⸻

三十二、真正 RepositoryManager

Repository 不能每次 DB 查。

应该：

内存缓存。

⸻

三十三、RepositoryManager

type RepositoryManager interface {
    Get(id string) *Repository
    Reload() error
}

内部：

sync.Map

或者：

atomic.Value

⸻

三十四、真正 ACL

ACL 不应该放 adapter。

应该：

RepositoryRouter 层。

⸻

三十五、真正权限模型

type Permission struct {
    RepositoryID string
    Actions []Action
}

⸻

三十六、Action

const (
    ActionRead
    ActionWrite
    ActionDelete
    ActionAdmin
)

⸻

三十七、真正 Upload Session（Docker 巨坑）

Docker upload：

不是一次请求。

是：

POST upload
PATCH upload
PUT complete

所以：

必须 UploadSessionManager。

⸻

三十八、UploadSession

type UploadSession struct {
    ID string
    RepositoryID string
    TempBlob string
    Offset int64
    StartedAt time.Time
}

⸻

三十九、真正 Background Jobs

成熟系统一定有：

Async Jobs

包括：

* cleanup
* GC
* stale refresh
* blob verify
* metadata rebuild

⸻

四十、真正 GC（很关键）

CAS 系统：

blob 不知道谁引用。

所以：

必须引用计数

或者

mark-sweep

⸻

四十一、推荐：

metadata 引用 blob

GC：

scan metadata
mark blob
delete unreferenced

⸻

四十二、真正最终架构（工业版）

                Gin Engine
                     ↓
              HTTP Middleware
                     ↓
             RepositoryRouter
                     ↓
            RepositoryResolver
                     ↓
              RepositoryManager
                     ↓
             Protocol Adapter
                     ↓
             ArtifactKey Builder
                     ↓
              RepositoryChain
                     ↓
     Hosted / Group / Proxy Node
                     ↓
       MetadataStore / BlobStore
                     ↓
            RemoteClient Layer
                     ↓
               External Registry

这时候：

你其实已经在设计：

一个真正企业级 Artifact Repository。


这个是整个系统真正的“地基”。

很多仓库系统最后崩掉：

不是协议。

而是：

数据模型没设计好。

尤其：

* Maven
* Docker
* npm
* Go module

这些协议：

元数据模型完全不同。

所以：

不能设计成：

name/version/path

这种简单模型。

一定会炸。

⸻

一、先明确核心原则（非常重要）

真正成熟设计：

artifact metadata

和

blob storage

必须彻底分离。

即：

Artifact
    ↓ references
Blob

而不是：

artifact == file

这是最重要原则。

⸻

二、真正核心实体

最终你会有：

Repository
Artifact
Blob
Tag/Version
Property
Relationship

⸻

三、Repository 表

这个比较简单。

repositories

CREATE TABLE repositories (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(255) UNIQUE NOT NULL,
    format          VARCHAR(64) NOT NULL,
    type            VARCHAR(32) NOT NULL,
    config          JSONB,
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL
);

⸻

四、repository_members（group repo）

CREATE TABLE repository_members (
    repository_id   BIGINT NOT NULL,
    member_id       BIGINT NOT NULL,
    position        INT NOT NULL,
    PRIMARY KEY(repository_id, member_id)
);

⸻

五、Blob（超级核心）

真正系统：

Blob 才是核心。

⸻

六、blobs 表（CAS）

CREATE TABLE blobs (
    id              BIGSERIAL PRIMARY KEY,
    algorithm       VARCHAR(32) NOT NULL,
    digest          VARCHAR(128) NOT NULL,
    size            BIGINT NOT NULL,
    storage_path    TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL,
    UNIQUE(algorithm, digest)
);

⸻

七、为什么这样设计

因为：

Docker layer dedupe

例如：

sha256:xxxx

多个 artifact：

共享同一个 blob。

⸻

八、storage_path 不要是业务路径

不要：

repo/com/foo/demo.jar

应该：

blobs/sha256/ab/cd/abcdef...

即：

CAS 存储。

⸻

九、Artifact（真正核心）

这里最容易设计错。

⸻

十、错误设计（千万别）

name
version
path

这是：

Maven-only 思维。

Docker/Go/npm 会崩。

⸻

十一、真正 Artifact 表

CREATE TABLE artifacts (
    id                  BIGSERIAL PRIMARY KEY,
    repository_id       BIGINT NOT NULL,
    format              VARCHAR(64) NOT NULL,
    kind                VARCHAR(64),
    coordinates         JSONB NOT NULL,
    metadata            JSONB,
    created_at          TIMESTAMP NOT NULL,
    updated_at          TIMESTAMP NOT NULL
);

⸻

十二、为什么 coordinates 必须 JSONB

因为：

不存在统一字段。

⸻

十三、Maven coordinates

{
  "group": "com.foo",
  "artifact": "demo",
  "version": "1.0.0"
}

⸻

十四、Docker coordinates

{
  "image": "nginx",
  "tag": "latest"
}

⸻

十五、Go module coordinates

{
  "module": "github.com/gin-gonic/gin",
  "version": "v1.0.0"
}

⸻

十六、npm coordinates

{
  "package": "react",
  "version": "18.0.0"
}

⸻

十七、为什么 metadata 单独 JSONB

因为：

metadata 是协议特定。

例如：

Docker manifest：

{
  "mediaType": "...",
  "architecture": "amd64"
}

Maven：

{
  "packaging": "jar"
}

⸻

十八、Artifact 和 Blob 的关系（最重要）

artifact_blobs

CREATE TABLE artifact_blobs (
    artifact_id     BIGINT NOT NULL,
    blob_id         BIGINT NOT NULL,
    position        INT NOT NULL,
    role            VARCHAR(64),
    PRIMARY KEY(artifact_id, blob_id, position)
);

⸻

十九、为什么需要 position

因为：

Docker layer 有顺序。

⸻

二十、为什么需要 role

例如：

Docker：

manifest
config
layer

Maven：

pom
jar
sources
javadoc

⸻

二十一、Maven 示例

artifact：

{
  "group": "com.foo",
  "artifact": "demo",
  "version": "1.0.0"
}

artifact_blobs：

jar
pom
sources

⸻

二十二、Docker 示例

artifact：

{
  "image": "nginx",
  "tag": "latest"
}

artifact_blobs：

manifest
config
layer1
layer2
layer3

⸻

二十三、为什么 artifact 不等于 blob

这是整个系统最关键认知。

⸻

二十四、Docker Manifest

manifest：

只是：

metadata object。

真正内容：

layers。

⸻

二十五、Tag 表（非常重要）

Docker/npm：

tag 和 version 不是一回事。

⸻

二十六、artifact_tags

CREATE TABLE artifact_tags (
    id              BIGSERIAL PRIMARY KEY,
    artifact_id     BIGINT NOT NULL,
    tag             VARCHAR(255) NOT NULL,
    created_at      TIMESTAMP NOT NULL
);

⸻

二十七、为什么 Docker 必须 tag 表

因为：

latest
stable
prod

可能：

指向同一个 artifact。

⸻

二十八、Version 表（推荐）

不要直接写 coordinates。

推荐：

artifact_versions

CREATE TABLE artifact_versions (
    id              BIGSERIAL PRIMARY KEY,
    artifact_id     BIGINT NOT NULL,
    version         VARCHAR(255) NOT NULL,
    normalized      VARCHAR(255),
    created_at      TIMESTAMP NOT NULL
);

⸻

二十九、为什么要 normalized

因为：

semantic version 比较。

例如：

1.0
1.0.0
v1.0.0

⸻

三十、Property 表（企业级非常重要）

很多系统最终都需要：

自定义 metadata。

⸻

三十一、artifact_properties

CREATE TABLE artifact_properties (
    artifact_id     BIGINT NOT NULL,
    key             VARCHAR(255) NOT NULL,
    value           TEXT,
    PRIMARY KEY(artifact_id, key)
);

⸻

三十二、Relationship（高级）

Docker：

manifest → layers

Maven：

pom → parent pom

Go：

module → dependency

最终：

artifact_relations

CREATE TABLE artifact_relations (
    source_id       BIGINT NOT NULL,
    target_id       BIGINT NOT NULL,
    relation_type   VARCHAR(64) NOT NULL
);

⸻

三十三、为什么这个设计强大

因为：

你开始支持：

* dependency graph
* SBOM
* vulnerability scan
* impact analysis

⸻

三十四、真正推荐索引（非常关键）

blobs

CREATE UNIQUE INDEX idx_blobs_digest
ON blobs(algorithm, digest);

⸻

三十五、artifacts

CREATE INDEX idx_artifacts_repo
ON artifacts(repository_id);

⸻

三十六、JSONB GIN

CREATE INDEX idx_artifacts_coordinates
ON artifacts
USING GIN(coordinates);

这个非常重要。

⸻

三十七、Docker tag 查询

CREATE INDEX idx_artifact_tags
ON artifact_tags(tag);

⸻

三十八、真正成熟后的模型

最终：

Repository
    ↓
Artifact
    ↓
ArtifactBlob
    ↓
Blob(CAS)

⸻

三十九、真正成熟系统的关键

Artifact 是逻辑对象

Blob 是物理对象

千万别混。

⸻

四十、最终 ER 结构

repositories
    ↓
artifacts
    ↓
artifact_blobs
    ↓
blobs
artifacts
    ↓
artifact_tags
artifacts
    ↓
artifact_properties
artifacts
    ↓
artifact_relations

⸻

四十一、真正工业级扩展能力

这个模型后期可以支持：

1. OCI

Docker/Helm 都能进。

⸻

2. Vulnerability Scan

artifact
    ↓
sbom
    ↓
cve

⸻

3. Immutable Tag

latest -> immutable digest

⸻

4. Replication

跨仓库同步。

⸻

5. Garbage Collection

artifact 删除：

blob 引用计数减少。

⸻

四十二、最终建议（很关键）

PostgreSQL

强烈推荐。

因为：

* JSONB
* GIN
* transactional
* advisory lock
* recursive query

非常适合仓库系统。

⸻

四十三、最终一句话（核心）

真正成熟仓库系统：

不是 “文件服务器”

而是：

“元数据驱动的 CAS 系统”