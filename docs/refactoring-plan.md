# Moonlight Box 工业级架构重构文档（彻底重构版）

## 文档概述

本文档基于 `/docs/new.md` 中的设计理念，详细描述如何将 Moonlight Box 彻底重构为企业级 Artifact Repository 系统（Nexus/Artifactory 级别）。

**核心设计原则**：真正成熟仓库系统不是"文件服务器"，而是"元数据驱动的 CAS 系统"。

**重构策略**：彻底重构，不考虑向后兼容，直接采用新的数据模型和架构。

---

## 一、当前架构问题分析

### 1.1 数据模型问题

```
当前模型：
Repository
    ↓
Component (namespace/name/version)  ← Maven 中心化
    ↓
Asset (path/filename/kind)
    ↓
Blob (ref/sha256/md5)
```

**核心问题**：

1. **Component 模型过于 Maven 中心化**
   - 使用 `namespace/name/version` 固定字段
   - 无法支持 Docker `image:tag`、Go `module:version`、npm `package:version`

2. **Artifact 与 Blob 边界模糊**
   - `Asset` 同时承担元数据和存储引用职责
   - 缺乏清晰的 `Artifact` 概念
   - 无法支持一个 Artifact 引用多个 Blob（Docker manifest → layers）

3. **缺少核心抽象 ArtifactKey**
   - 系统围绕 `name/version/path` 运转
   - 不同协议需要不同的解析逻辑

4. **存储路径耦合业务逻辑**
   - 当前：`repo/com/foo/demo.jar`
   - 应该：`blobs/sha256/ab/cd/abcdef...`（CAS 存储）

### 1.2 请求处理问题

```
当前流程：
RepoRouter → Adapter.ParseIntent → RepoHandler.Resolve → DownloadService → StorageService
```

**问题**：

1. **Adapter 职责过重**：同时负责路径解析、意图判断、内容获取、元数据生成
2. **缺少 RepositoryChain 抽象**：Local/Proxy/Virtual 逻辑分散
3. **缓存系统未分离**：metadata cache 和 blob cache 混在一起

---

## 二、目标架构

### 2.1 核心设计原则

1. **Artifact 是逻辑对象，Blob 是物理对象**（绝对不能混淆）
2. **Artifact metadata 和 Blob storage 必须彻底分离**
3. **所有协议统一到 ArtifactKey 抽象**
4. **BlobStore 必须是 append-only CAS**

### 2.2 目标数据模型

```
Repository
    ↓
Artifact (format/coordinates/kind)
    ↓
ArtifactBlob (position/role)
    ↓
Blob (algorithm/digest/size)

扩展：
Artifact → ArtifactTags
Artifact → ArtifactProperties
Artifact → ArtifactRelations
```

### 2.3 目标请求流程

```
Gin Engine
    ↓
HTTP Middleware (Auth/Log/ACL)
    ↓
RepositoryRouter
    ↓
RepositoryManager (内存缓存)
    ↓
Adapter.ParseRequest → ArtifactKey
    ↓
RepositoryChain.Fetch/Put/Delete
    ↓
HostedNode / ProxyNode / GroupNode
    ↓
MetadataStore / BlobStore
    ↓
RemoteClient (if needed)
    ↓
Streaming Response
```

---

## 三、数据模型设计

### 3.1 repositories 表

```sql
CREATE TABLE repositories (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(255) UNIQUE NOT NULL,
    display_name    VARCHAR(255),
    description     TEXT,
    format          VARCHAR(64) NOT NULL,      -- maven/docker/npm/go/pypi/yum/apt/generic
    type            VARCHAR(32) NOT NULL,      -- local/proxy/group
    enabled         BOOLEAN DEFAULT true,
    public_visible  BOOLEAN DEFAULT true,
    
    -- 存储配置
    storage_backend_id BIGINT,
    
    -- 代理配置（仅 proxy 类型）
    remote_url      TEXT,
    auth_type       VARCHAR(32) DEFAULT 'none',
    auth_config     JSONB,
    
    -- 缓存配置
    cache_enabled       BOOLEAN DEFAULT true,
    cache_metadata_ttl  INT DEFAULT 300,       -- 元数据缓存 TTL（秒）
    cache_blob_ttl      INT DEFAULT 86400,     -- Blob 缓存 TTL（秒）
    cache_negative_ttl  INT DEFAULT 30,        -- 404 缓存 TTL（秒）
    cache_max_blob_size BIGINT DEFAULT 104857600, -- 最大缓存 Blob 大小
    
    -- 代理高级配置
    timeout_seconds     INT DEFAULT 30,
    max_redirects       INT DEFAULT 5,
    insecure_skip_verify BOOLEAN DEFAULT false,
    
    -- 本地仓库配置
    allow_overwrite BOOLEAN DEFAULT false,
    allow_delete    BOOLEAN DEFAULT false,
    
    -- 统计
    download_count  BIGINT DEFAULT 0,
    
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_repositories_format ON repositories(format);
CREATE INDEX idx_repositories_type ON repositories(type);
```

### 3.2 repository_members 表（Group 仓库）

```sql
CREATE TABLE repository_members (
    group_id    BIGINT NOT NULL REFERENCES repositories(id),
    member_id   BIGINT NOT NULL REFERENCES repositories(id),
    priority    INT NOT NULL DEFAULT 0,
    writable    BOOLEAN DEFAULT false,      -- 是否为可写成员
    PRIMARY KEY(group_id, member_id)
);

CREATE INDEX idx_repository_members_group ON repository_members(group_id);
CREATE INDEX idx_repository_members_member ON repository_members(member_id);
```

### 3.3 blobs 表（CAS 核心）

```sql
CREATE TABLE blobs (
    id              BIGSERIAL PRIMARY KEY,
    algorithm       VARCHAR(32) NOT NULL,      -- sha256/sha512/md5
    digest          VARCHAR(128) NOT NULL,
    size            BIGINT NOT NULL,
    storage_path    TEXT NOT NULL,             -- blobs/sha256/ab/cd/abcdef...
    storage_backend_id BIGINT,
    ref_count       INT DEFAULT 0,             -- 引用计数
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(algorithm, digest)
);

CREATE UNIQUE INDEX idx_blobs_digest ON blobs(algorithm, digest);
CREATE INDEX idx_blobs_storage_backend ON blobs(storage_backend_id);
```

**storage_path 规则**：

```
blobs/{algorithm}/{digest[0:2]}/{digest[2:4]}/{digest}
```

例如：`blobs/sha256/ab/cd/abcdef123456...`

### 3.4 artifacts 表（核心）

```sql
CREATE TABLE artifacts (
    id              BIGSERIAL PRIMARY KEY,
    repository_id   BIGINT NOT NULL REFERENCES repositories(id),
    format          VARCHAR(64) NOT NULL,      -- maven/docker/npm/go/pypi/...
    kind            VARCHAR(64),               -- primary/pom/sources/manifest/config/layer/...
    
    -- 协议特定坐标（JSONB）
    coordinates     JSONB NOT NULL,
    
    -- 协议特定元数据（JSONB）
    metadata        JSONB,
    
    -- 状态
    status          VARCHAR(32) DEFAULT 'published',
    
    -- 统计
    download_count  BIGINT DEFAULT 0,
    size_bytes      BIGINT DEFAULT 0,
    
    -- 审计
    published_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    published_by    BIGINT,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

-- JSONB GIN 索引（关键！）
CREATE INDEX idx_artifacts_coordinates ON artifacts USING GIN(coordinates);
CREATE INDEX idx_artifacts_repo ON artifacts(repository_id);
CREATE INDEX idx_artifacts_format ON artifacts(format);
CREATE INDEX idx_artifacts_status ON artifacts(status);
CREATE INDEX idx_artifacts_repo_format ON artifacts(repository_id, format);
```

**coordinates 示例**：

```json
// Maven
{"group": "com.foo", "artifact": "demo", "version": "1.0.0"}

// Go module
{"module": "github.com/gin-gonic/gin", "version": "v1.0.0"}

// npm
{"package": "react", "version": "18.0.0"}

// PyPI
{"package": "requests", "version": "2.28.0"}

// Yum/RPM
{"name": "nginx", "version": "1.18.0", "release": "1.el7", "arch": "x86_64"}
```

### 3.5 artifact_blobs 关联表

```sql
CREATE TABLE artifact_blobs (
    artifact_id BIGINT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    blob_id     BIGINT NOT NULL REFERENCES blobs(id),
    position    INT NOT NULL DEFAULT 0,        -- Docker layer 顺序
    role        VARCHAR(64) NOT NULL,          -- primary/pom/jar/sources/manifest/config/layer/...
    
    PRIMARY KEY(artifact_id, blob_id, position)
);

CREATE INDEX idx_artifact_blobs_blob ON artifact_blobs(blob_id);
CREATE INDEX idx_artifact_blobs_role ON artifact_blobs(role);
```

**role 说明**：

| 协议 | role 值 | 说明 |
|------|---------|------|
| Maven | primary, pom, sources, javadoc | 主文件、POM、源码、文档 |
| npm | package, tarball | 包元数据、压缩包 |
| Go | mod, zip, info | 模块文件 |
| PyPI | primary, wheel, sdist | 主文件、wheel、源码包 |

### 3.6 artifact_tags 表

```sql
CREATE TABLE artifact_tags (
    id            BIGSERIAL PRIMARY KEY,
    artifact_id   BIGINT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    repository_id BIGINT NOT NULL,
    tag           VARCHAR(255) NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(repository_id, tag)
);

CREATE INDEX idx_artifact_tags_artifact ON artifact_tags(artifact_id);
CREATE INDEX idx_artifact_tags_repo ON artifact_tags(repository_id);
```

**用途**：
- npm：dist-tags 如 next、beta、latest

### 3.7 artifact_properties 表

```sql
CREATE TABLE artifact_properties (
    artifact_id BIGINT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    key         VARCHAR(255) NOT NULL,
    value       TEXT,
    
    PRIMARY KEY(artifact_id, key)
);

CREATE INDEX idx_artifact_properties_key ON artifact_properties(key);
```

**用途**：企业级自定义元数据（审批状态、安全扫描结果、自定义标签等）

### 3.8 artifact_relations 表

```sql
CREATE TABLE artifact_relations (
    source_id     BIGINT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    target_id     BIGINT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    relation_type VARCHAR(64) NOT NULL,        -- depends_on/parent/extends/replaces
    
    PRIMARY KEY(source_id, target_id, relation_type)
);

CREATE INDEX idx_artifact_relations_source ON artifact_relations(source_id);
CREATE INDEX idx_artifact_relations_target ON artifact_relations(target_id);
```

**用途**：
- 依赖图：支持 SBOM 生成
- Maven parent POM 继承
- 漏洞影响分析

### 3.9 remote_artifact_states 表（代理状态）

```sql
CREATE TABLE remote_artifact_states (
    artifact_id   BIGINT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    repository_id BIGINT NOT NULL,
    
    -- 状态机
    state         VARCHAR(32) NOT NULL,        -- missing/fetching/ready/corrupted
    
    -- 缓存信息
    cached_at     TIMESTAMP,
    cache_expires_at TIMESTAMP,
    
    -- 错误信息
    last_error    TEXT,
    error_at      TIMESTAMP,
    
    -- 远程信息
    remote_etag       VARCHAR(255),
    remote_last_modified TIMESTAMP,
    
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    
    PRIMARY KEY(artifact_id, repository_id)
);

CREATE INDEX idx_remote_states_repo ON remote_artifact_states(repository_id);
CREATE INDEX idx_remote_states_state ON remote_artifact_states(state);
```

---

## 四、Go 模型定义

### 4.1 ArtifactKey（核心）

```go
// internal/types/artifact_key.go

package types

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "sort"
)

type ArtifactKey struct {
    RepositoryID string            `json:"repository_id"`
    Format       string            `json:"format"`
    Coordinates  map[string]string `json:"coordinates"`
    Filename     string            `json:"filename,omitempty"`
    Extension    string            `json:"extension,omitempty"`
}

func (k *ArtifactKey) String() string {
    coords, _ := json.Marshal(k.Coordinates)
    return fmt.Sprintf("%s/%s/%s", k.RepositoryID, k.Format, string(coords))
}

func (k *ArtifactKey) BuildCASPath() string {
    h := sha256.New()
    h.Write([]byte(k.Format))
    
    keys := make([]string, 0, len(k.Coordinates))
    for key := range k.Coordinates {
        keys = append(keys, key)
    }
    sort.Strings(keys)
    
    for _, key := range keys {
        h.Write([]byte(key))
        h.Write([]byte("="))
        h.Write([]byte(k.Coordinates[key]))
    }
    
    digest := hex.EncodeToString(h.Sum(nil))
    return fmt.Sprintf("blobs/sha256/%s/%s/%s", digest[:2], digest[2:4], digest)
}

func (k *ArtifactKey) Matches(coords map[string]string) bool {
    for key, value := range coords {
        if k.Coordinates[key] != value {
            return false
        }
    }
    return true
}

func (k *ArtifactKey) GetCoordinate(key string) string {
    return k.Coordinates[key]
}
```

### 4.2 Artifact

```go
// internal/types/artifact.go

package types

import "time"

type ArtifactStatus string

const (
    ArtifactStatusDraft      ArtifactStatus = "draft"
    ArtifactStatusPublished  ArtifactStatus = "published"
    ArtifactStatusDeprecated ArtifactStatus = "deprecated"
    ArtifactStatusYanked     ArtifactStatus = "yanked"
)

type Artifact struct {
    ID            uint                    `json:"id"`
    RepositoryID  uint                    `json:"repository_id"`
    Format        string                  `json:"format"`
    Kind          string                  `json:"kind"`
    Coordinates   map[string]string       `json:"coordinates"`
    Metadata      map[string]interface{}  `json:"metadata,omitempty"`
    BlobRefs      []BlobRef               `json:"blob_refs,omitempty"`
    Properties    map[string]string       `json:"properties,omitempty"`
    Tags          []string                `json:"tags,omitempty"`
    Status        ArtifactStatus          `json:"status"`
    DownloadCount int64                   `json:"download_count"`
    SizeBytes     int64                   `json:"size_bytes"`
    PublishedAt   time.Time               `json:"published_at"`
    PublishedBy   uint                    `json:"published_by,omitempty"`
    CreatedAt     time.Time               `json:"created_at"`
    UpdatedAt     time.Time               `json:"updated_at"`
    
    RepositoryName string `json:"repository_name,omitempty"`
}

type BlobRef struct {
    BlobID    uint   `json:"blob_id"`
    Algorithm string `json:"algorithm"`
    Digest    string `json:"digest"`
    Size      int64  `json:"size"`
    Position  int    `json:"position"`
    Role      string `json:"role"`
}

type ArtifactQuery struct {
    RepositoryID uint
    Format       string
    Coordinates  map[string]string
    Status       ArtifactStatus
    Limit        int
    Offset       int
}
```

### 4.3 Blob

```go
// internal/types/blob.go

package types

import "time"

type Blob struct {
    ID              uint      `json:"id"`
    Algorithm       string    `json:"algorithm"`
    Digest          string    `json:"digest"`
    Size            int64     `json:"size"`
    StoragePath     string    `json:"storage_path"`
    StorageBackendID uint     `json:"storage_backend_id,omitempty"`
    RefCount        int       `json:"ref_count"`
    CreatedAt       time.Time `json:"created_at"`
}

type BlobRef struct {
    BlobID    uint
    Algorithm string
    Digest    string
    Size      int64
    Position  int
    Role      string
}
```

### 4.4 Repository

```go
// internal/types/repository.go

package types

type RepositoryType string

const (
    RepoTypeLocal  RepositoryType = "local"
    RepoTypeProxy  RepositoryType = "proxy"
    RepoTypeGroup  RepositoryType = "group"
)

type Repository struct {
    ID             uint            `json:"id"`
    Name           string          `json:"name"`
    DisplayName    string          `json:"display_name"`
    Description    string          `json:"description"`
    Format         string          `json:"format"`
    Type           RepositoryType  `json:"type"`
    Enabled        bool            `json:"enabled"`
    PublicVisible  bool            `json:"public_visible"`
    
    StorageBackendID *uint         `json:"storage_backend_id,omitempty"`
    
    RemoteURL       string         `json:"remote_url,omitempty"`
    AuthType        string         `json:"auth_type"`
    AuthConfig      map[string]interface{} `json:"auth_config,omitempty"`
    
    CacheEnabled       bool        `json:"cache_enabled"`
    CacheMetadataTTL   int         `json:"cache_metadata_ttl"`
    CacheBlobTTL       int         `json:"cache_blob_ttl"`
    CacheNegativeTTL   int         `json:"cache_negative_ttl"`
    CacheMaxBlobSize   int64       `json:"cache_max_blob_size"`
    
    TimeoutSeconds     int         `json:"timeout_seconds"`
    MaxRedirects       int         `json:"max_redirects"`
    InsecureSkipVerify bool        `json:"insecure_skip_verify"`
    
    AllowOverwrite bool            `json:"allow_overwrite"`
    AllowDelete    bool            `json:"allow_delete"`
    
    DownloadCount int64            `json:"download_count"`
    
    Members []RepositoryMember      `json:"members,omitempty"`
}

type RepositoryMember struct {
    GroupID   uint            `json:"group_id"`
    MemberID  uint            `json:"member_id"`
    Priority  int             `json:"priority"`
    Writable  bool            `json:"writable"`
    Member    *Repository     `json:"member,omitempty"`
}
```

---

## 五、Adapter 重构

### 5.1 Adapter 接口

**核心认知**：Adapter 本质是 `Request → ArtifactKey` 的转换器。

```go
// internal/adapter/adapter.go

package adapter

import (
    "context"
    "net/http"
    
    "github.com/moonlight-box/registry/internal/types"
)

type RequestIntent string

const (
    IntentDownload   RequestIntent = "download"
    IntentMetadata   RequestIntent = "metadata"
    IntentList       RequestIntent = "list"
    IntentChecksum   RequestIntent = "checksum"
    IntentUpload     RequestIntent = "upload"
    IntentDelete     RequestIntent = "delete"
    IntentUnknown    RequestIntent = "unknown"
)

type ParsedRequest struct {
    Key    *types.ArtifactKey
    Intent RequestIntent
    Extra  map[string]interface{}
}

type Adapter interface {
    Format() string
    
    ParseRequest(path string, method string, headers http.Header) (*ParsedRequest, error)
    
    BuildArtifactKey(repoID string, coords map[string]string) (*types.ArtifactKey, error)
    
    ValidateCoordinates(coords map[string]string) error
    
    NormalizeCoordinates(coords map[string]string) map[string]string
}

type ContentAdapter interface {
    Adapter
    
    FormatMetadata(ctx context.Context, artifacts []*types.Artifact, baseURL string) (*types.ContentResult, error)
    
    FormatList(ctx context.Context, artifacts []*types.Artifact, baseURL string) (*types.ContentResult, error)
}

type ContentResult struct {
    Content     io.ReadCloser
    Size        int64
    ContentType string
    StatusCode  int
    Headers     map[string]string
    ExtraData   map[string]interface{}
}
```

### 5.2 Maven Adapter

```go
// internal/adapter/maven_adapter.go

package adapter

import (
    "fmt"
    "net/http"
    "path/filepath"
    "strings"
    
    "github.com/moonlight-box/registry/internal/types"
)

type MavenAdapter struct{}

func (a *MavenAdapter) Format() string {
    return "maven"
}

func (a *MavenAdapter) ParseRequest(path string, method string, headers http.Header) (*ParsedRequest, error) {
    path = strings.Trim(path, "/")
    
    gav, err := a.parseGAV(path)
    if err != nil {
        return nil, err
    }
    
    key := &types.ArtifactKey{
        Format: "maven",
        Coordinates: map[string]string{
            "group":    gav.Group,
            "artifact": gav.Artifact,
            "version":  gav.Version,
        },
        Filename:  gav.Filename,
        Extension: gav.Extension,
    }
    
    intent := a.detectIntent(path, method)
    
    return &ParsedRequest{
        Key:    key,
        Intent: intent,
        Extra: map[string]interface{}{
            "classifier": gav.Classifier,
            "packaging":  gav.Packaging,
        },
    }, nil
}

type MavenGAV struct {
    Group      string
    Artifact   string
    Version    string
    Classifier string
    Packaging  string
    Filename   string
    Extension  string
}

func (a *MavenAdapter) parseGAV(path string) (*MavenGAV, error) {
    parts := strings.Split(path, "/")
    if len(parts) < 4 {
        return nil, fmt.Errorf("invalid maven path: %s", path)
    }
    
    filename := parts[len(parts)-1]
    ext := filepath.Ext(filename)
    
    version := parts[len(parts)-2]
    artifact := parts[len(parts)-3]
    
    groupParts := parts[:len(parts)-3]
    group := strings.Join(groupParts, ".")
    
    gav := &MavenGAV{
        Group:    group,
        Artifact: artifact,
        Version:  version,
        Filename: filename,
        Extension: ext,
    }
    
    if strings.HasSuffix(filename, "-sources.jar") {
        gav.Classifier = "sources"
        gav.Packaging = "jar"
    } else if strings.HasSuffix(filename, "-javadoc.jar") {
        gav.Classifier = "javadoc"
        gav.Packaging = "jar"
    } else if ext == ".pom" {
        gav.Packaging = "pom"
    } else if ext == ".jar" {
        gav.Packaging = "jar"
    }
    
    return gav, nil
}

func (a *MavenAdapter) detectIntent(path string, method string) RequestIntent {
    switch method {
    case http.MethodGet:
        if strings.HasSuffix(path, "maven-metadata.xml") {
            return IntentMetadata
        }
        if strings.HasSuffix(path, ".sha1") || strings.HasSuffix(path, ".md5") {
            return IntentChecksum
        }
        return IntentDownload
    case http.MethodPut:
        return IntentUpload
    case http.MethodDelete:
        return IntentDelete
    }
    return IntentUnknown
}

func (a *MavenAdapter) BuildArtifactKey(repoID string, coords map[string]string) (*types.ArtifactKey, error) {
    required := []string{"group", "artifact", "version"}
    for _, r := range required {
        if coords[r] == "" {
            return nil, fmt.Errorf("missing required coordinate: %s", r)
        }
    }
    
    return &types.ArtifactKey{
        RepositoryID: repoID,
        Format:       "maven",
        Coordinates:  coords,
    }, nil
}

func (a *MavenAdapter) ValidateCoordinates(coords map[string]string) error {
    if coords["group"] == "" {
        return fmt.Errorf("missing group")
    }
    if coords["artifact"] == "" {
        return fmt.Errorf("missing artifact")
    }
    if coords["version"] == "" {
        return fmt.Errorf("missing version")
    }
    return nil
}

func (a *MavenAdapter) NormalizeCoordinates(coords map[string]string) map[string]string {
    return map[string]string{
        "group":    coords["group"],
        "artifact": coords["artifact"],
        "version":  coords["version"],
    }
}
```

### 5.3 其他 Adapter

各协议 Adapter 实现 `Adapter` 接口，核心职责是 `Request → ArtifactKey` 转换：

- **NpmAdapter** - npm 包：解析 `@scope/package/version` 路径
- **PyPIAdapter** - Python 包：解析 `package/version` 路径
- **GoAdapter** - Go module：解析 `module/@v/version` 路径
- **YumAdapter** - RPM 包：解析 RPM 仓库结构
- **AptAdapter** - Debian 包：解析 APT 仓库结构
- **GenericAdapter** - 通用文件：简单路径映射

**注意**：Docker 协议暂不实现，后续根据需要添加。

---

## 六、RepositoryChain

### 6.1 接口定义

```go
// internal/repository/chain.go

package repository

import (
    "context"
    "io"
    
    "github.com/moonlight-box/registry/internal/types"
)

type RepositoryChain interface {
    Fetch(ctx context.Context, key *types.ArtifactKey) (*types.Artifact, error)
    Put(ctx context.Context, artifact *types.Artifact, content io.Reader) error
    Delete(ctx context.Context, key *types.ArtifactKey) error
    Exists(ctx context.Context, key *types.ArtifactKey) (bool, error)
    List(ctx context.Context, query *types.ArtifactQuery) ([]*types.Artifact, error)
}

type FetchResult struct {
    Artifact   *types.Artifact
    Content    io.ReadCloser
    FromCache  bool
    SourceType string
}
```

### 6.2 HostedNode（本地仓库）

```go
// internal/repository/hosted_node.go

package repository

import (
    "context"
    "io"
    
    "github.com/moonlight-box/registry/internal/types"
)

type HostedNode struct {
    repo          *types.Repository
    metadataStore MetadataStore
    blobStore     BlobStore
}

func NewHostedNode(repo *types.Repository, metadataStore MetadataStore, blobStore BlobStore) *HostedNode {
    return &HostedNode{
        repo:          repo,
        metadataStore: metadataStore,
        blobStore:     blobStore,
    }
}

func (n *HostedNode) Fetch(ctx context.Context, key *types.ArtifactKey) (*types.Artifact, error) {
    artifact, err := n.metadataStore.Get(ctx, key)
    if err != nil {
        return nil, err
    }
    
    if artifact.Status != types.ArtifactStatusPublished {
        return nil, ErrArtifactNotAvailable
    }
    
    return artifact, nil
}

func (n *HostedNode) Put(ctx context.Context, artifact *types.Artifact, content io.Reader) error {
    blobRef, err := n.blobStore.Put(ctx, content)
    if err != nil {
        return err
    }
    
    artifact.BlobRefs = []types.BlobRef{*blobRef}
    artifact.RepositoryID = n.repo.ID
    
    return n.metadataStore.Put(ctx, artifact)
}

func (n *HostedNode) Delete(ctx context.Context, key *types.ArtifactKey) error {
    artifact, err := n.metadataStore.Get(ctx, key)
    if err != nil {
        return err
    }
    
    if err := n.metadataStore.Delete(ctx, key); err != nil {
        return err
    }
    
    for _, blobRef := range artifact.BlobRefs {
        n.blobStore.DecrRef(ctx, blobRef.BlobID)
    }
    
    return nil
}

func (n *HostedNode) Exists(ctx context.Context, key *types.ArtifactKey) (bool, error) {
    return n.metadataStore.Exists(ctx, key)
}

func (n *HostedNode) List(ctx context.Context, query *types.ArtifactQuery) ([]*types.Artifact, error) {
    query.RepositoryID = n.repo.ID
    return n.metadataStore.List(ctx, query)
}
```

### 6.3 ProxyNode（代理仓库）

```go
// internal/repository/proxy_node.go

package repository

import (
    "context"
    "io"
    "time"
    
    "github.com/moonlight-box/registry/internal/types"
)

type RemoteArtifactState string

const (
    StateMissing   RemoteArtifactState = "missing"
    StateFetching  RemoteArtifactState = "fetching"
    StateReady     RemoteArtifactState = "ready"
    StateCorrupted RemoteArtifactState = "corrupted"
)

type ProxyNode struct {
    repo          *types.Repository
    metadataStore MetadataStore
    blobStore     BlobStore
    remoteClient  RemoteClient
    stateManager  *RemoteStateManager
    metadataCache *MetadataCache
}

func NewProxyNode(
    repo *types.Repository,
    metadataStore MetadataStore,
    blobStore BlobStore,
    remoteClient RemoteClient,
    stateManager *RemoteStateManager,
    metadataCache *MetadataCache,
) *ProxyNode {
    return &ProxyNode{
        repo:          repo,
        metadataStore: metadataStore,
        blobStore:     blobStore,
        remoteClient:  remoteClient,
        stateManager:  stateManager,
        metadataCache: metadataCache,
    }
}

func (n *ProxyNode) Fetch(ctx context.Context, key *types.ArtifactKey) (*types.Artifact, error) {
    state := n.stateManager.GetState(key)
    if state == StateCorrupted {
        return nil, ErrArtifactCorrupted
    }
    
    if artifact, ok := n.metadataCache.Get(ctx, key); ok {
        if !n.isCacheExpired(artifact) {
            return artifact, nil
        }
    }
    
    artifact, err := n.metadataStore.Get(ctx, key)
    if err == nil && !n.isCacheExpired(artifact) {
        n.metadataCache.Set(ctx, key, artifact)
        return artifact, nil
    }
    
    n.stateManager.SetState(key, StateFetching)
    
    remoteArtifact, err := n.remoteClient.FetchMetadata(ctx, key)
    if err != nil {
        n.stateManager.SetState(key, StateMissing)
        n.metadataCache.SetNegative(ctx, key)
        return nil, err
    }
    
    if err := n.metadataStore.Put(ctx, remoteArtifact); err != nil {
        n.stateManager.SetState(key, StateCorrupted)
        return nil, err
    }
    
    n.stateManager.SetState(key, StateReady)
    n.metadataCache.Set(ctx, key, remoteArtifact)
    
    return remoteArtifact, nil
}

func (n *ProxyNode) Put(ctx context.Context, artifact *types.Artifact, content io.Reader) error {
    return ErrProxyNotWritable
}

func (n *ProxyNode) Delete(ctx context.Context, key *types.ArtifactKey) error {
    return ErrProxyNotWritable
}

func (n *ProxyNode) isCacheExpired(artifact *types.Artifact) bool {
    if artifact == nil {
        return true
    }
    
    cacheTTL := time.Duration(n.repo.CacheMetadataTTL) * time.Second
    return time.Since(artifact.UpdatedAt) > cacheTTL
}

func (n *ProxyNode) FetchBlob(ctx context.Context, key *types.ArtifactKey) (io.ReadCloser, error) {
    artifact, err := n.Fetch(ctx, key)
    if err != nil {
        return nil, err
    }
    
    if len(artifact.BlobRefs) == 0 {
        return nil, ErrBlobNotFound
    }
    
    blobRef := artifact.BlobRefs[0]
    
    reader, err := n.blobStore.Open(ctx, &blobRef)
    if err == nil {
        return reader, nil
    }
    
    remoteReader, err := n.remoteClient.FetchBlob(ctx, key)
    if err != nil {
        return nil, err
    }
    
    go func() {
        defer remoteReader.Close()
        n.blobStore.PutWithRef(ctx, remoteReader, &blobRef)
    }()
    
    return remoteReader, nil
}
```

### 6.4 GroupNode（虚拟仓库）

```go
// internal/repository/group_node.go

package repository

import (
    "context"
    "io"
    
    "github.com/moonlight-box/registry/internal/types"
    "github.com/sirupsen/logrus"
)

type GroupNode struct {
    repo     *types.Repository
    members  []RepositoryChain
    writable RepositoryChain
}

func NewGroupNode(repo *types.Repository, members []RepositoryChain, writable RepositoryChain) *GroupNode {
    return &GroupNode{
        repo:     repo,
        members:  members,
        writable: writable,
    }
}

func (n *GroupNode) Fetch(ctx context.Context, key *types.ArtifactKey) (*types.Artifact, error) {
    for i, member := range n.members {
        artifact, err := member.Fetch(ctx, key)
        if err == nil {
            return artifact, nil
        }
        
        if !IsNotFoundError(err) {
            logrus.WithError(err).Warnf("group member %d fetch failed", i)
        }
    }
    return nil, ErrArtifactNotFound
}

func (n *GroupNode) Put(ctx context.Context, artifact *types.Artifact, content io.Reader) error {
    if n.writable == nil {
        return ErrGroupNotWritable
    }
    return n.writable.Put(ctx, artifact, content)
}

func (n *GroupNode) Delete(ctx context.Context, key *types.ArtifactKey) error {
    if n.writable == nil {
        return ErrGroupNotWritable
    }
    return n.writable.Delete(ctx, key)
}

func (n *GroupNode) Exists(ctx context.Context, key *types.ArtifactKey) (bool, error) {
    for _, member := range n.members {
        exists, err := member.Exists(ctx, key)
        if err == nil && exists {
            return true, nil
        }
    }
    return false, nil
}

func (n *GroupNode) List(ctx context.Context, query *types.ArtifactQuery) ([]*types.Artifact, error) {
    allArtifacts := make([]*types.Artifact, 0)
    seen := make(map[string]bool)
    
    for _, member := range n.members {
        artifacts, err := member.List(ctx, query)
        if err != nil {
            continue
        }
        
        for _, a := range artifacts {
            key := a.Coordinates["group"] + ":" + a.Coordinates["artifact"] + ":" + a.Coordinates["version"]
            if !seen[key] {
                seen[key] = true
                allArtifacts = append(allArtifacts, a)
            }
        }
    }
    
    return allArtifacts, nil
}
```

---

## 七、存储层

### 7.1 BlobStore 接口

```go
// internal/storage/blob_store.go

package storage

import (
    "context"
    "io"
    
    "github.com/moonlight-box/registry/internal/types"
)

type BlobStore interface {
    Put(ctx context.Context, reader io.Reader) (*types.BlobRef, error)
    PutWithRef(ctx context.Context, reader io.Reader, ref *types.BlobRef) error
    Open(ctx context.Context, ref *types.BlobRef) (io.ReadCloser, error)
    Stat(ctx context.Context, ref *types.BlobRef) (*types.Blob, error)
    Exists(ctx context.Context, algorithm, digest string) (bool, error)
    Delete(ctx context.Context, ref *types.BlobRef) error
    DecrRef(ctx context.Context, blobID uint) error
    IncrRef(ctx context.Context, blobID uint) error
}
```

### 7.2 CASBlobStore 实现

```go
// internal/storage/cas_blob_store.go

package storage

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    
    "github.com/google/uuid"
    "github.com/moonlight-box/registry/internal/types"
)

type CASBlobStore struct {
    backend       Backend
    db            *gorm.DB
    hashAlgorithm string
}

func NewCASBlobStore(backend Backend, db *gorm.DB) *CASBlobStore {
    return &CASBlobStore{
        backend:       backend,
        db:            db,
        hashAlgorithm: "sha256",
    }
}

func (s *CASBlobStore) Put(ctx context.Context, reader io.Reader) (*types.BlobRef, error) {
    hasher := sha256.New()
    teeReader := io.TeeReader(reader, hasher)
    
    tempPath := fmt.Sprintf("temp/%s", uuid.New().String())
    size, err := s.backend.Put(ctx, tempPath, teeReader, -1)
    if err != nil {
        return nil, err
    }
    
    digest := hex.EncodeToString(hasher.Sum(nil))
    
    existingBlob, err := s.findByDigest(ctx, "sha256", digest)
    if err == nil {
        s.backend.Delete(ctx, tempPath)
        s.incrRef(ctx, existingBlob.ID)
        return &types.BlobRef{
            BlobID:    existingBlob.ID,
            Algorithm: "sha256",
            Digest:    digest,
            Size:      existingBlob.Size,
        }, nil
    }
    
    casPath := s.buildCASPath("sha256", digest)
    if err := s.backend.Move(ctx, tempPath, casPath); err != nil {
        return nil, err
    }
    
    blob := &model.Blob{
        Algorithm:   "sha256",
        Digest:      digest,
        Size:        size,
        StoragePath: casPath,
        RefCount:    1,
    }
    if err := s.db.Create(blob).Error; err != nil {
        return nil, err
    }
    
    return &types.BlobRef{
        BlobID:    blob.ID,
        Algorithm: "sha256",
        Digest:    digest,
        Size:      size,
    }, nil
}

func (s *CASBlobStore) Open(ctx context.Context, ref *types.BlobRef) (io.ReadCloser, error) {
    var blob model.Blob
    if err := s.db.First(&blob, ref.BlobID).Error; err != nil {
        return nil, err
    }
    return s.backend.Get(ctx, blob.StoragePath)
}

func (s *CASBlobStore) buildCASPath(algorithm, digest string) string {
    return fmt.Sprintf("blobs/%s/%s/%s/%s", algorithm, digest[:2], digest[2:4], digest)
}

func (s *CASBlobStore) findByDigest(ctx context.Context, algorithm, digest string) (*model.Blob, error) {
    var blob model.Blob
    err := s.db.Where("algorithm = ? AND digest = ?", algorithm, digest).First(&blob).Error
    if err != nil {
        return nil, err
    }
    return &blob, nil
}

func (s *CASBlobStore) incrRef(ctx context.Context, blobID uint) error {
    return s.db.Model(&model.Blob{}).Where("id = ?", blobID).UpdateColumn("ref_count", gorm.Expr("ref_count + 1")).Error
}

func (s *CASBlobStore) DecrRef(ctx context.Context, blobID uint) error {
    return s.db.Model(&model.Blob{}).Where("id = ?", blobID).UpdateColumn("ref_count", gorm.Expr("ref_count - 1")).Error
}
```

### 7.3 MetadataStore 接口

```go
// internal/storage/metadata_store.go

package storage

import (
    "context"
    
    "github.com/moonlight-box/registry/internal/types"
)

type MetadataStore interface {
    Get(ctx context.Context, key *types.ArtifactKey) (*types.Artifact, error)
    Put(ctx context.Context, artifact *types.Artifact) error
    Delete(ctx context.Context, key *types.ArtifactKey) error
    Exists(ctx context.Context, key *types.ArtifactKey) (bool, error)
    List(ctx context.Context, query *types.ArtifactQuery) ([]*types.Artifact, error)
    Search(ctx context.Context, format string, coords map[string]string) ([]*types.Artifact, error)
    
    GetByTag(ctx context.Context, repoID uint, format string, tag string) (*types.Artifact, error)
    AddTag(ctx context.Context, artifactID uint, tag string) error
    RemoveTag(ctx context.Context, artifactID uint, tag string) error
    
    SetProperty(ctx context.Context, artifactID uint, key, value string) error
    GetProperty(ctx context.Context, artifactID uint, key string) (string, error)
    
    AddRelation(ctx context.Context, sourceID, targetID uint, relationType string) error
    GetRelations(ctx context.Context, artifactID uint, relationType string) ([]uint, error)
}
```

---

## 八、缓存系统

### 8.1 MetadataCache

```go
// internal/cache/metadata_cache.go

package cache

import (
    "context"
    "sync"
    "time"
    
    "github.com/moonlight-box/registry/internal/types"
)

type MetadataCache struct {
    store   sync.Map
    ttl     time.Duration
    maxSize int
    size    int
}

type cachedMetadata struct {
    artifact   *types.Artifact
    cachedAt   time.Time
    isNegative bool
}

func NewMetadataCache(ttl time.Duration, maxSize int) *MetadataCache {
    return &MetadataCache{
        ttl:     ttl,
        maxSize: maxSize,
    }
}

func (c *MetadataCache) Get(ctx context.Context, key *types.ArtifactKey) (*types.Artifact, bool) {
    cacheKey := key.String()
    
    value, ok := c.store.Load(cacheKey)
    if !ok {
        return nil, false
    }
    
    cached := value.(*cachedMetadata)
    
    if time.Since(cached.cachedAt) > c.ttl {
        c.store.Delete(cacheKey)
        return nil, false
    }
    
    if cached.isNegative {
        return nil, false
    }
    
    return cached.artifact, true
}

func (c *MetadataCache) Set(ctx context.Context, key *types.ArtifactKey, artifact *types.Artifact) {
    cacheKey := key.String()
    c.store.Store(cacheKey, &cachedMetadata{
        artifact: artifact,
        cachedAt: time.Now(),
    })
}

func (c *MetadataCache) SetNegative(ctx context.Context, key *types.ArtifactKey) {
    cacheKey := key.String() + ":negative"
    c.store.Store(cacheKey, &cachedMetadata{
        cachedAt:   time.Now(),
        isNegative: true,
    })
}

func (c *MetadataCache) Invalidate(ctx context.Context, key *types.ArtifactKey) {
    cacheKey := key.String()
    c.store.Delete(cacheKey)
    c.store.Delete(cacheKey + ":negative")
}
```

---

## 九、RemoteClient

RemoteClient 负责从远程仓库获取元数据和 Blob。当前系统已有代理下载逻辑，重构时复用现有实现，只需适配新的接口。

### 9.1 接口定义

```go
// internal/remote/client.go

package remote

import (
    "context"
    "io"
    
    "github.com/moonlight-box/registry/internal/types"
)

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
```

### 9.2 实现策略

复用现有的 `internal/proxy/client.go` 实现，适配 `ArtifactKey` 接口：

- HTTP 请求逻辑保持不变
- 认证逻辑保持不变
- 熔断器逻辑保持不变
- 仅需将路径构建改为基于 `ArtifactKey.Coordinates`

---

## 十、后台任务

```go
// internal/job/gc_job.go

package job

import (
    "context"
    
    "github.com/moonlight-box/registry/internal/model"
    "github.com/moonlight-box/registry/internal/storage"
    "github.com/sirupsen/logrus"
    "gorm.io/gorm"
)

type GCJob struct {
    db        *gorm.DB
    blobStore storage.BlobStore
}

func NewGCJob(db *gorm.DB, blobStore storage.BlobStore) *GCJob {
    return &GCJob{
        db:        db,
        blobStore: blobStore,
    }
}

func (j *GCJob) Name() string {
    return "garbage-collection"
}

func (j *GCJob) Run(ctx context.Context) error {
    logrus.Info("Starting garbage collection")
    
    markedBlobs := make(map[uint]bool)
    
    var artifactBlobs []model.ArtifactBlob
    if err := j.db.Find(&artifactBlobs).Error; err != nil {
        return err
    }
    
    for _, ab := range artifactBlobs {
        markedBlobs[ab.BlobID] = true
    }
    
    var blobs []model.Blob
    if err := j.db.Find(&blobs).Error; err != nil {
        return err
    }
    
    deletedCount := 0
    for _, blob := range blobs {
        if !markedBlobs[blob.ID] && blob.RefCount <= 0 {
            if err := j.blobStore.Delete(ctx, &types.BlobRef{BlobID: blob.ID}); err != nil {
                logrus.WithError(err).Errorf("Failed to delete blob %d", blob.ID)
                continue
            }
            j.db.Delete(&blob)
            deletedCount++
        }
    }
    
    logrus.Infof("GC completed: deleted %d blobs", deletedCount)
    return nil
}
```

---

## 十一、RepositoryManager

```go
// internal/repository/manager.go

package repository

import (
    "context"
    "sync/atomic"
    "time"
    
    "github.com/moonlight-box/registry/internal/types"
)

type RepositoryManager struct {
    cache    atomic.Value
    repoRepo *RepositoryRepository
    groupRepo *GroupRepository
}

func NewRepositoryManager(repoRepo *RepositoryRepository, groupRepo *GroupRepository) *RepositoryManager {
    m := &RepositoryManager{
        repoRepo:  repoRepo,
        groupRepo: groupRepo,
    }
    m.cache.Store(make(map[string]*types.Repository))
    return m
}

func (m *RepositoryManager) Get(name string) (*types.Repository, error) {
    cache := m.cache.Load().(map[string]*types.Repository)
    if repo, ok := cache[name]; ok {
        return repo, nil
    }
    return nil, ErrRepositoryNotFound
}

func (m *RepositoryManager) GetByID(id uint) (*types.Repository, error) {
    cache := m.cache.Load().(map[string]*types.Repository)
    for _, repo := range cache {
        if repo.ID == id {
            return repo, nil
        }
    }
    return nil, ErrRepositoryNotFound
}

func (m *RepositoryManager) GetMembers(repoID uint) ([]types.RepositoryMember, error) {
    return m.groupRepo.GetMembers(repoID)
}

func (m *RepositoryManager) Reload() error {
    repos, err := m.repoRepo.List()
    if err != nil {
        return err
    }
    
    cache := make(map[string]*types.Repository)
    for _, repo := range repos {
        cache[repo.Name] = repo
    }
    
    m.cache.Store(cache)
    return nil
}

func (m *RepositoryManager) Start(ctx context.Context) error {
    if err := m.Reload(); err != nil {
        return err
    }
    
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                if err := m.Reload(); err != nil {
                    logrus.WithError(err).Error("Failed to reload repositories")
                }
            }
        }
    }()
    
    return nil
}
```

---

## 十二、实施计划

### Phase 1: 数据模型（1 周）

**任务**：
1. 删除旧表（components, assets）
2. 创建新表结构
3. 实现 GORM 模型
4. 编写数据库迁移

**验收**：
- 所有新表创建成功
- GORM 模型可正常 CRUD

### Phase 2: 核心对象（1 周）

**任务**：
1. 实现 ArtifactKey
2. 实现 Artifact/Blob 类型
3. 实现 Repository 类型
4. 实现 Adapter 接口

**验收**：
- ArtifactKey 可解析各协议路径
- 单元测试覆盖率 > 80%

### Phase 3: Adapter 实现（2 周）

**任务**：
1. 实现 MavenAdapter
2. 实现 NpmAdapter
3. 实现 PyPIAdapter
4. 实现 GoAdapter
5. 实现 YumAdapter
6. 实现 AptAdapter
7. 实现 GenericAdapter

**验收**：
- 所有协议可正确解析路径
- 可构建正确的 ArtifactKey

### Phase 4: RepositoryChain（1 周）

**任务**：
1. 实现 HostedNode
2. 实现 ProxyNode
3. 实现 GroupNode
4. 实现 RemoteStateManager

**验收**：
- Local/Proxy/Group 仓库功能正常

### Phase 5: 存储层（1 周）

**任务**：
1. 实现 BlobStore 接口
2. 实现 CASBlobStore
3. 实现 MetadataStore
4. 实现 Blob 去重

**验收**：
- Blob 去重功能正常
- CAS 存储路径正确

### Phase 6: 缓存系统（1 周）

**任务**：
1. 实现 MetadataCache
2. 适配 RemoteClient
3. 集成到 ProxyNode

**验收**：
- 缓存命中率统计正常
- TTL 过期逻辑正确

### Phase 7: 后台任务（1 周）

**任务**：
1. 实现 Job 调度器
2. 实现 GCJob
3. 实现 CleanupJob
4. 实现 BlobVerifyJob

**验收**：
- GC 可正确清理未引用 Blob

### Phase 8: 集成测试（1 周）

**任务**：
1. 编写集成测试
2. 性能测试
3. 文档更新

**验收**：
- 所有测试通过
- 性能无明显下降

---

## 十三、总结

本次彻底重构的核心目标：

1. **建立清晰的 Artifact 和 Blob 分离模型**
2. **引入 ArtifactKey 统一所有协议的坐标系统**
3. **实现 CAS 存储以支持 Blob 去重**
4. **分离 metadata cache 和 blob cache**
5. **统一 RepositoryChain 接口**
6. **实现后台 GC 任务**

通过彻底重构，Moonlight Box 将从"文件服务器"升级为"元数据驱动的 CAS 系统"，达到 Nexus/Artifactory 级别的企业级能力。
