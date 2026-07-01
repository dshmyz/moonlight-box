package runtime

import (
	"context"
	"fmt"
	"io"
	"net/url"
	pathpkg "path"
	"strings"
	"time"
)

type ctxKey string

const clientIPKey ctxKey = "client_ip"

// Kind* 常量 —— Artifact.Kind 中跨协议通用的值。
// 协议特有的 Kind 由各 Plugin 各自定义。
const (
	KindPackage   = "package"   // 包聚合入口
	KindVersion   = "version"   // 版本记录（从远程元数据解析的版本列表）
	KindArtifact  = "artifact"  // 具体包产物（上传/下载的包文件）
	KindFile      = "file"      // 可下载文件
	KindDirectory = "directory" // 目录占位（Raw/Generic 列表用，不进入包聚合）
	KindMetadata  = "metadata"  // metadata/index/release 等协议元数据
	KindChecksum  = "checksum"  // checksum 文件或 checksum 投影
)

// mustHaveNameKinds 是需要 Artifact.Name 的 Kind 集合。
var mustHaveNameKinds = map[string]bool{
	KindVersion:  true,
	KindArtifact: true,
}

// IsMetadataKind 判断给定 Kind 是否为协议元数据（不应写入 packages 聚合表）。
func IsMetadataKind(kind string) bool {
	return kind == KindMetadata || kind == KindChecksum
}

func IsCatalogExcludedKind(kind string) bool {
	return IsMetadataKind(kind) || kind == KindDirectory
}

// ValidateArtifactForStore 对写入存储的 Artifact 做合规检查。
// MetadataStore 在 Put/BatchPut 时自动调用；Plugin 无需主动调用。
func ValidateArtifactForStore(a *Artifact) error {
	if a == nil {
		return fmt.Errorf("artifact: nil artifact")
	}
	NormalizeArtifactForStore(a)
	if a.Format == "" {
		return fmt.Errorf("artifact: format is required")
	}
	if a.Kind == "" {
		return fmt.Errorf("artifact: kind is required")
	}
	if mustHaveNameKinds[a.Kind] && a.Name == "" {
		return fmt.Errorf("artifact: name is required for kind=%q (format=%s)", a.Kind, a.Format)
	}
	if a.Filename != "" && strings.Contains(a.Filename, "/") {
		return fmt.Errorf("artifact: filename must not contain slash: %q", a.Filename)
	}
	if a.Path != "" && a.Filename != "" {
		cleanPath := strings.Trim(a.Path, "/")
		if cleanPath == a.Filename || strings.HasSuffix(cleanPath, "/"+a.Filename) {
			return fmt.Errorf("artifact: path must be a directory and must not include filename: path=%q filename=%q", a.Path, a.Filename)
		}
	}
	if a.DownloadURL != "" {
		u, err := url.Parse(a.DownloadURL)
		if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("artifact: download_url must be an absolute http(s) URL: %q", a.DownloadURL)
		}
	}
	return nil
}

type ArtifactSpec struct {
	RepositoryID string
	Format       string
	Kind         string

	Name      string
	Namespace string
	Version   string

	Path        string
	Filename    string
	RemotePath  string
	DownloadURL string

	Extension   string
	ContentType string
	SizeBytes   int64

	Checksums  map[string]string
	Qualifiers map[string]string
	Attributes map[string]string
	Properties map[string]string
	BlobRefs   []BlobRef
	Content    io.ReadCloser
}

func NewArtifact(spec ArtifactSpec) *Artifact {
	a := &Artifact{
		RepositoryID: spec.RepositoryID,
		Format:       spec.Format,
		Kind:         spec.Kind,
		Name:         spec.Name,
		Namespace:    spec.Namespace,
		Version:      spec.Version,
		Path:         spec.Path,
		Filename:     spec.Filename,
		RemotePath:   spec.RemotePath,
		DownloadURL:  spec.DownloadURL,
		Extension:    spec.Extension,
		ContentType:  spec.ContentType,
		SizeBytes:    spec.SizeBytes,
		Checksums:    cloneStringMap(spec.Checksums),
		Qualifiers:   cloneStringMap(spec.Qualifiers),
		Attributes:   cloneStringMap(spec.Attributes),
		Properties:   cloneStringMap(spec.Properties),
		BlobRefs:     spec.BlobRefs,
		Content:      spec.Content,
	}
	NormalizeArtifactForStore(a)
	return a
}

func NormalizeArtifactForStore(a *Artifact) {
	if a == nil || a.normalized {
		return
	}
	a.normalized = true
	if a.Properties == nil {
		a.Properties = map[string]string{}
	}
	if a.Qualifiers == nil {
		a.Qualifiers = map[string]string{}
	}
	if a.Attributes == nil {
		a.Attributes = map[string]string{}
	}
	if a.Checksums == nil {
		a.Checksums = map[string]string{}
	}
	normalizeArtifactKind(a)

	if a.RemotePath == "" {
		a.RemotePath = a.Properties["remote_path"]
	}
	if a.DownloadURL == "" {
		a.DownloadURL = a.Properties["download_url"]
	}

	a.RemotePath = cleanArtifactPath(a.RemotePath)
	a.Path = cleanArtifactPath(a.Path)

	if a.RemotePath != "" {
		if a.Filename == "" {
			a.Filename = pathpkg.Base(a.RemotePath)
		}
		if a.Path == "" {
			dir := pathpkg.Dir(a.RemotePath)
			if dir != "." {
				a.Path = dir
			}
		}
	}
	if a.RemotePath == "" && a.Path != "" && a.Filename != "" {
		a.RemotePath = joinArtifactPath(a.Path, a.Filename)
	}
	if a.Extension == "" && a.Filename != "" {
		a.Extension = pathpkg.Ext(a.Filename)
	}

	if a.RemotePath != "" && a.Properties["remote_path"] != a.RemotePath {
		a.Properties["remote_path"] = a.RemotePath
	}
	if a.DownloadURL != "" && a.Properties["download_url"] != a.DownloadURL {
		a.Properties["download_url"] = a.DownloadURL
	}
	if a.IdentityKey == "" {
		a.IdentityKey = BuildArtifactIdentityKey(a)
	}
}

func normalizeArtifactKind(a *Artifact) {
	if a == nil {
		return
	}
	switch a.Kind {
	case "tarball", "package-file":
		if a.Attributes["artifact_type"] == "" {
			a.Attributes["artifact_type"] = a.Kind
		}
		a.Kind = KindArtifact
	case "module-file":
		if a.Attributes["artifact_type"] == "" {
			a.Attributes["artifact_type"] = a.Kind
		}
		a.Kind = KindFile
	case "release":
		if a.Attributes["metadata_type"] == "" {
			a.Attributes["metadata_type"] = a.Kind
		}
		a.Kind = KindMetadata
	}
}

func BuildArtifactIdentityKey(a *Artifact) string {
	if a == nil {
		return ""
	}
	switch a.Kind {
	case KindPackage:
		return "package/" + a.Name
	case KindVersion:
		return "version/" + a.Name + "/" + a.Version
	case KindMetadata:
		if a.RemotePath != "" {
			return "metadata/" + a.RemotePath
		}
	case KindChecksum:
		if a.RemotePath != "" {
			return "checksum/" + a.RemotePath
		}
	}
	if a.RemotePath != "" {
		return "file/" + a.RemotePath
	}
	if a.Name != "" || a.Version != "" || a.Path != "" || a.Filename != "" {
		return "artifact/" + a.Name + "/" + a.Version + "/" + joinArtifactPath(a.Path, a.Filename)
	}
	return "artifact/" + a.Format + "/" + a.Kind
}

func QueryHasIdentityFields(q ArtifactQuery) bool {
	return q.Kind != "" || q.Name != "" || q.Namespace != "" || q.Version != "" || q.Path != "" || q.Filename != "" || q.IdentityKey != "" || len(q.Qualifiers) > 0
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
	Kind         string
	Name         string
	Namespace    string
	Version      string
	Path         string
	Filename     string
	RemotePath   string
	IdentityKey  string
	Qualifiers   map[string]string
	Extension    string
	RemoteURL    string
}

func (k *ArtifactKey) String() string {
	base := k.RepositoryID + "/" + k.Format
	if k.IdentityKey != "" {
		base += "/identity=" + k.IdentityKey
	}
	if k.Kind != "" {
		base += "/kind=" + k.Kind
	}
	if k.Namespace != "" {
		base += "/namespace=" + k.Namespace
	}
	if k.Name != "" {
		base += "/name=" + k.Name
	}
	if k.Version != "" {
		base += "/version=" + k.Version
	}
	if k.Path != "" {
		base += "/path=" + k.Path
	}
	if k.RemotePath != "" {
		base += "/remote_path=" + k.RemotePath
	}
	if k.Filename != "" {
		base += "/" + k.Filename
	}
	return base
}

type ArtifactQuery struct {
	RepositoryID     string
	Format           string
	Kind             string
	Name             string
	Namespace        string
	Version          string
	Path             string
	Filename         string
	RemotePath       string
	RemotePathPrefix string
	IdentityKey      string
	Qualifiers       map[string]string
	Limit            int
	Offset           int
}

type ProjectionQuery struct {
	RepositoryID string
	Format       string
	Kind         string
	Name         string
	Namespace    string
	Version      string
	Path         string
	Filename     string
	RemotePath   string
	IdentityKey  string
	Qualifiers   map[string]string
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
	IdentityKey  string
	Name         string
	Namespace    string
	Version      string
	Path         string
	Filename     string
	RemotePath   string
	DownloadURL  string
	Extension    string
	ContentType  string
	Properties   map[string]string
	Checksums    map[string]string
	Qualifiers   map[string]string
	Attributes   map[string]string
	Relations    []ArtifactRelation
	BlobRefs     []BlobRef
	Content      io.ReadCloser
	CreatedAt    time.Time
	UpdatedAt    time.Time
	normalized   bool
	// 请求统计字段（由 ProxyRuntime 设置）
	FromCache bool   // 是否命中缓存
	RemoteURL string // 回源 URL（未命中缓存时）
	SizeBytes int64  // 文件大小
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func cleanArtifactPath(value string) string {
	return strings.Trim(strings.ReplaceAll(value, "\\", "/"), "/")
}

func joinArtifactPath(dir, file string) string {
	dir = cleanArtifactPath(dir)
	file = strings.Trim(file, "/")
	if dir == "" {
		return file
	}
	if file == "" {
		return dir
	}
	return dir + "/" + file
}

type RemoteMetadata struct {
	Exists     bool
	ETag       string
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
