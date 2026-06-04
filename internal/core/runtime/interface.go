package runtime

import (
	"context"
	"io"
	"net/http"
)

type UploadSession interface {
	PutBlob(ctx context.Context, blob io.Reader) (BlobRef, error)
	PutArtifact(ctx context.Context, artifact *Artifact) error
	Commit(ctx context.Context) error
	Abort(ctx context.Context) error
}

type RepositoryRuntime interface {
	GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error)
	QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
	RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error)
	BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error)
	DeleteArtifact(ctx context.Context, key ArtifactKey) error
}

type RepositoryNode interface {
	GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error)
	QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
	RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error)
	BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error)
	DeleteArtifact(ctx context.Context, key ArtifactKey) error
}

type MetadataStore interface {
	Get(ctx context.Context, key ArtifactKey) (*Artifact, error)
	Put(ctx context.Context, artifact *Artifact) error
	BatchPut(ctx context.Context, artifacts []*Artifact) error
	Delete(ctx context.Context, key ArtifactKey) error
	Query(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
}

type BlobStore interface {
	Put(reader io.Reader) (BlobRef, error)
	Open(ref BlobRef) (io.ReadCloser, error)
	Stat(ref BlobRef) (*BlobMetadata, error)
	Delete(ref BlobRef) error
}

type RemoteClient interface {
	FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error)
	FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error)
}

type RepositoryManager interface {
	Get(id string) *Repository
	Reload() error
}

type RepositoryPathResolver interface {
	Resolve(req *http.Request) (*ResolvedRepository, error)
}

// ProtocolPlugin 协议插件接口。
// 所有包协议（Maven/NPM/PyPI/Go 等）必须实现此接口。
//
// 约束（MetadataStore 在写入时会强制校验）：
//   - Handle 中通过 runtime 写入的 Artifact，如果 Kind 是 version/artifact/package-file/module-file，
//     Coordinates 中必须包含 CoordName 字段，用于搜索聚合。
//   - 建议使用 runtime 包中定义的 Coord* 常量作为 Coordinates key，
//     避免各 Plugin 硬编码字符串导致不一致。
type ProtocolPlugin interface {
	Name() string
	Handle(ctx *RequestContext, runtime RepositoryRuntime) error
}

// RemoteFetcher 由 ProtocolPlugin 实现，供 ProxyRuntime 回调。
// Runtime 控制回源时机和缓存策略；Plugin 只负责远端协议交互（HTTP 请求 + 响应解析）。
//
// 约束：
//   - FetchRemote 返回的 Artifact 中，如果 Kind 属于 version/artifact/package-file/module-file，
//     Coordinates 中必须包含 CoordName 字段（格式由各协议定义，如 Maven 为 group:artifact）。
//   - 注意：返回的 Artifact 会被 MetadataStore.BatchPut 持久化，缺少 CoordName 会导致存储失败。
type RemoteFetcher interface {
	FetchRemote(ctx context.Context, remoteURL, path string) ([]*Artifact, error)
}

// PackageBlocker 阻断规则检查——在请求进入 Plugin 前检查。
type PackageBlocker interface {
	IsBlocked(packageType, packageName, version string) bool
	BlockReason(packageType, packageName, version string) string
}

// AuditLogger 审计日志记录器。
type AuditLogger interface {
	Log(ctx context.Context, entry AuditEntry)
}

type AuditEntry struct {
	UserID         uint
	Username       string
	Action         string
	ResourceType   string
	ResourceName   string
	IPAddress      string
	UserAgent      string
	ResponseStatus int
}

type RequestContext struct {
	Writer         http.ResponseWriter
	Request        *http.Request
	Repository     *Repository
	Runtime        RepositoryRuntime
	RepositoryPath string
	RouteStyle     RouteStyle
	Blocker        PackageBlocker
	StatusCode     int    // 实际响应状态码，由 handleRequest 设置
	FromCache      bool   // 是否命中缓存，由 Runtime 设置
	RemoteURL      string // 回源 URL，由 Runtime 设置
	SizeBytes      int64  // 下载字节数，由 Runtime 设置

	// 协议解析结果（由 ProtocolPlugin 在 Handle 中填充），Runtime 层不感知协议格式。
	// 仅在请求对应"具体包/产物"时填写；目录/索引/projection 类请求可留空。
	PackageName string // 包名（npm pkg / go module / pypi project / maven groupId:artifactId）
	Version     string // 版本（若该请求与具体版本相关）
	Filename    string // 实际下载文件名
}
