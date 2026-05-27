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

type ProtocolPlugin interface {
	Name() string
	Handle(ctx *RequestContext, runtime RepositoryRuntime) error
}

// RemoteFetcher 由 ProtocolPlugin 实现，供 ProxyRuntime 回调。
// Runtime 控制回源时机和缓存策略；Plugin 只负责远端协议交互（HTTP 请求 + 响应解析）。
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
	StatusCode     int // 实际响应状态码，由 handleRequest 设置
}
