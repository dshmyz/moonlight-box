package runtime

import (
	"context"
	"errors"
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
	OpenRemote(ctx context.Context, request RemoteOpenRequest) (*RemoteResponse, error)
	BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error)
	DeleteArtifact(ctx context.Context, key ArtifactKey) error
}

type RepositoryNode interface {
	GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error)
	QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
	RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error)
	OpenRemote(ctx context.Context, request RemoteOpenRequest) (*RemoteResponse, error)
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

type ContextBlobPutter interface {
	PutContext(ctx context.Context, reader io.Reader) (BlobRef, error)
}

type ContextBlobOpener interface {
	OpenContext(ctx context.Context, ref BlobRef) (io.ReadCloser, error)
}

type ContextBlobDeleter interface {
	DeleteContext(ctx context.Context, ref BlobRef) error
}

type RemoteRequest struct {
	URL     string
	Method  string
	Headers http.Header
}

type RemoteOpenRequest struct {
	Path    string
	Method  string
	Headers http.Header
}

type RemoteResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

type RemoteClient interface {
	FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error)
	FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error)
	Open(ctx context.Context, request RemoteRequest) (*RemoteResponse, error)
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
//     必须填充 Name，Version/Path/Filename/RemotePath 按协议语义填充。
//   - 会影响 artifact 身份的协议限定字段放入 Qualifiers，例如 classifier、extension、arch。
type ProtocolPlugin interface {
	Name() string
	Handle(ctx *RequestContext, runtime RepositoryRuntime) error
}

type NormalizeInput struct {
	RepositoryID string
	Format       string

	RemotePath string
	Filename   string

	ContentType string
	SizeBytes   int64

	Checksums  map[string]string
	Attributes map[string]string
	Hints      map[string]string
	BlobRefs   []BlobRef
}

// ArtifactNormalizer is an optional plugin capability for offline asset
// normalization, such as migration. It must not write HTTP responses or invoke
// repository runtime behavior.
type ArtifactNormalizer interface {
	NormalizeAsset(ctx context.Context, input NormalizeInput) (*Artifact, error)
}

// RemoteFetcher 由 ProtocolPlugin 实现，供 ProxyRuntime 回调。
// Runtime 控制回源时机和缓存策略；Plugin 只负责远端协议交互（HTTP 请求 + 响应解析）。
//
// 约束：
//   - FetchRemote 返回的 Artifact 中，如果 Kind 属于 version/artifact/package-file/module-file，
//     必须填充 Name，必要时用 Qualifiers 表达协议限定字段。
//   - 注意：返回的 Artifact 会被 MetadataStore.BatchPut 持久化，缺少 Name 会导致存储失败。
type RemoteFetcher interface {
	FetchRemote(ctx context.Context, remoteURL, path string) ([]*Artifact, error)
}

var (
	// ErrMetadataUnsupported indicates that a protocol cannot provide the
	// conditional metadata required to evaluate a rule.
	ErrMetadataUnsupported = errors.New("artifact metadata unsupported")
	// ErrMetadataUnavailable indicates that metadata is unavailable for a
	// concrete artifact although the protocol supports metadata retrieval.
	ErrMetadataUnavailable = errors.New("artifact metadata unavailable")
)

// ArtifactMetadata contains protocol-normalized metadata for a concrete artifact.
// Plugins return only semantic attributes; Runtime owns the cache and policy.
type ArtifactMetadata struct {
	Attributes map[string]string
}

// ArtifactMetadataFetcher is an optional plugin capability used by ProxyRuntime
// to obtain semantic attributes for a direct artifact download.
type ArtifactMetadataFetcher interface {
	FetchArtifactMetadata(ctx context.Context, remoteURL string, key ArtifactKey) (*ArtifactMetadata, error)
}

// ConditionRequirement describes one rule that requires an artifact attribute.
type ConditionRequirement struct {
	RuleID    uint
	Attribute string
}

// ConditionalBlocker is an optional extension of PackageBlocker. It lets Runtime
// avoid metadata requests unless a conditional rule can match the package key.
type ConditionalBlocker interface {
	RequiredAttributes(packageType, packageName, version string) []ConditionRequirement
}

// ConditionUnverifiedEntry describes a download that was allowed because a
// potentially applicable conditional rule could not be evaluated.
type ConditionUnverifiedEntry struct {
	RepositoryID      string
	Format            string
	Name              string
	Version           string
	RemotePath        string
	RuleIDs           []uint
	MissingAttributes []string
	Reason            string
}

// ConditionAuditLogger is an optional audit sink for conditional-rule bypasses.
type ConditionAuditLogger interface {
	LogConditionUnverified(ctx context.Context, entry ConditionUnverifiedEntry)
}

// PackageBlocker 阻断规则检查——在请求进入 Plugin 前检查。
type PackageBlocker interface {
	IsBlocked(packageType, packageName, version string) bool
	BlockReason(packageType, packageName, version string) string
	// IsBlockedWithAttrs 带元数据的第二层阻断检查。
	// 在包名+版本第一层未命中时，结合 attrs（license/published_at 等元数据）做条件匹配。
	// 返回是否阻断及原因；未阻断时 reason 为空字符串。
	IsBlockedWithAttrs(packageType, packageName, version string, attrs map[string]interface{}) (blocked bool, reason string)
	// IsBlockedByPath URL 路由层的早阻断检查。
	//
	// 在请求进入 Plugin 解析出包名/版本之前，router 只能看到剩余 URL 路径（非包名），
	// 故只能评估按路径形态匹配的通配符规则。精确/版本范围/阻断整包(Version=*)规则
	// 需要真包名+版本，由 Plugin 解析后在 runtime 的 checkBlocked/checkBlockedWithAttrs
	// 中权威评估——本方法不负责这些规则类型。
	IsBlockedByPath(packageType, path string) bool
	BlockReasonByPath(packageType, path string) string
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
	Reason         string
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
