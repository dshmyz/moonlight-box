package runtime

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

type ctxKey string

const clientIPKey ctxKey = "client_ip"

// Coord* 常量 —— Coordinates map 中跨协议通用的键。
// 协议特有的键（如 Maven 的 group/artifact）由各 Plugin 各自定义。
const (
	CoordName    = "name"     // 包名（搜索聚合用），所有协议必须设置
	CoordVersion = "version"  // 版本号
	CoordPath    = "path"     // 路径
	CoordFileNm  = "filename" // 文件名
)

// Kind* 常量 —— Artifact.Kind 中跨协议通用的值。
// 协议特有的 Kind 由各 Plugin 各自定义。
const (
	KindVersion  = "version"  // 版本记录（从远程元数据解析的版本列表）
	KindArtifact = "artifact" // 具体包产物（上传/下载的包文件）
)

// mustHaveNameKinds 是需要 name 坐标的 Kind 集合。
// Plugin 生成这些 Kind 的 Artifact 时，必须设置 Coordinates[CoordName]。
var mustHaveNameKinds = map[string]bool{
	KindVersion:  true,
	KindArtifact: true,
}

// ValidateArtifactForStore 对写入存储的 Artifact 做合规检查。
// MetadataStore 在 Put/BatchPut 时自动调用；Plugin 无需主动调用。
func ValidateArtifactForStore(a *Artifact) error {
	if a == nil {
		return fmt.Errorf("artifact: nil artifact")
	}
	if a.Format == "" {
		return fmt.Errorf("artifact: format is required")
	}
	if a.Kind == "" {
		return fmt.Errorf("artifact: kind is required")
	}
	if mustHaveNameKinds[a.Kind] && a.Coordinates[CoordName] == "" {
		return fmt.Errorf("artifact: missing required coordinate %q for kind=%q (format=%s)", CoordName, a.Kind, a.Format)
	}
	return nil
}

// ContextWithClientIP 将客户端 IP 注入 context，供 Runtime 层回源时使用。
func ContextWithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey, ip)
}

// ClientIPFromContext 从 context 中提取客户端 IP。
func ClientIPFromContext(ctx context.Context) string {
	if ip, ok := ctx.Value(clientIPKey).(string); ok {
		return ip
	}
	return ""
}

type ArtifactKey struct {
	RepositoryID string
	Format       string
	Coordinates  map[string]string
	Filename     string
	Extension    string
	RemoteURL    string
}

func (k *ArtifactKey) String() string {
	base := k.RepositoryID + "/" + k.Format
	// 按 key 排序 Coordinates 保证稳定输出
	coords := make([]string, 0, len(k.Coordinates))
	for key := range k.Coordinates {
		coords = append(coords, key)
	}
	sort.Strings(coords)
	for _, key := range coords {
		base += "/" + key + "=" + k.Coordinates[key]
	}
	if k.Filename != "" {
		base += "/" + k.Filename
	}
	return base
}

type ArtifactQuery struct {
	RepositoryID string
	Format       string
	Coordinates  map[string]string
	Limit        int
	Offset       int
	RemotePath   string // 远端拉取路径，ProxyRuntime 据此回调 RemoteFetcher
}

type ProjectionQuery struct {
	RepositoryID string
	Format       string
	Kind         string
	Coordinates  map[string]string
	RemotePath   string // 远端拉取路径，ProxyRuntime 据此回调 RemoteFetcher
}

type ProjectionResult struct {
	Dynamic  bool
	Content  []byte
	Artifact *Artifact
}

type UploadRequest struct {
	RepositoryID string
	Format       string
	Filename     string
	Size         int64
}

type BlobRef struct {
	BlobID    uint
	Algorithm string
	Digest    string
	Size      int64
}

type BlobMetadata struct {
	Algorithm   string
	Digest      string
	Size        int64
	StoragePath string
	CreatedAt   time.Time
}

type ArtifactRelation struct {
	Type     string
	TargetID string
}

type Artifact struct {
	ID           string
	RepositoryID string
	Format       string
	Kind         string
	Coordinates  map[string]string
	Properties   map[string]string
	Relations    []ArtifactRelation
	BlobRefs     []BlobRef
	Content      io.ReadCloser
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// 请求统计字段（由 ProxyRuntime 设置）
	FromCache bool   // 是否命中缓存
	RemoteURL string // 回源 URL（未命中缓存时）
	SizeBytes int64  // 文件大小
}

type RemoteMetadata struct {
	Exists     bool
	Digest     string
	Size       int64
	ModifiedAt time.Time
}

type CachePolicy struct {
	MetadataTTL     time.Duration
	BlobTTL         time.Duration
	NegativeTTL     time.Duration
	MaxBlobSize     int64
	SnapshotRefresh bool
}

type Repository struct {
	ID      string
	Name    string
	Format  string
	Type    string
	Config  map[string]interface{}
	Runtime RepositoryRuntime
}

type Permission struct {
	RepositoryID string
	Actions      []Action
}

type Action string

const (
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionDelete Action = "delete"
	ActionAdmin  Action = "admin"
)

type RouteStyle int

const (
	Nexus3Route RouteStyle = iota
	Nexus2Route RouteStyle = iota
)

type ResolvedRepository struct {
	Repository    *Repository
	RemainingPath string
	RouteStyle    RouteStyle
}

// SanitizeFilename 移除文件名中的控制字符（\r, \n, \x00 等）和双引号，
// 防止 Content-Disposition header 注入。
func SanitizeFilename(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '"' {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
