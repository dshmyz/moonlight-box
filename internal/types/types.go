package types

import (
	"io"

	"github.com/gin-gonic/gin"
)

type PackageType string

const (
	NpmType     PackageType = "npm"
	MavenType   PackageType = "maven"
	PyPIType    PackageType = "pypi"
	GoType      PackageType = "go"
	YumType     PackageType = "yum"
	AptType     PackageType = "apt"
	GenericType PackageType = "generic"
)

type PackageIdentity struct {
	Name    string
	Version string
	Type    PackageType
}

type PackagePathInfo struct {
	Name           string
	Version        string
	Filename       string
	StorageName    string
	StorageVersion string
	RemotePath     string
}

type UploadRequest struct {
	Package      interface{}
	Filename     string
	Size         int64
	Checksum     string
	Metadata     map[string]interface{}
	UploadedBy   uint
	RepositoryID uint
}

type PackageMeta struct {
	ID          uint
	Name        string
	Type        PackageType
	Description string
	Versions    []VersionInfo
}

type VersionInfo struct {
	Version       string
	PublishedAt   string
	Size          int64
	DownloadCount int64
	DistTags      []string // npm specific
}

type PackageVersionResult struct {
	PackageID  uint
	VersionID  uint
	Version    string
	StorageKey string
	Size       int64
	Checksum   string
}

type RepoOperationResult struct {
	PackageName string
	Version     string
	Size        int64
	Filename    string
	ExtraData   map[string]interface{}
	Response    interface{}
}

type PublishResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message,omitempty"`
	Package  string `json:"package,omitempty"`
	Version  string `json:"version,omitempty"`
	Filename string `json:"filename,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type MavenPublishResponse struct {
	PublishResponse
	Packaging string `json:"packaging,omitempty"`
}

type NpmPublishResponse struct {
	PublishResponse
	Description string `json:"description,omitempty"`
}

type PypiPublishResponse struct {
	PublishResponse
}

type YumPublishResponse struct {
	PublishResponse
	Repo       string `json:"repo,omitempty"`
	Arch       string `json:"arch,omitempty"`
	Release    string `json:"release,omitempty"`
	StorageKey string `json:"storageKey,omitempty"`
	PackageId  uint   `json:"packageId,omitempty"`
}

type AptPublishResponse struct {
	PublishResponse
	StorageKey string `json:"storageKey,omitempty"`
	PackageId  uint   `json:"packageId,omitempty"`
}

type GenericPublishResponse struct {
	PublishResponse
	StorageKey string `json:"storageKey,omitempty"`
	PackageId  uint   `json:"packageId,omitempty"`
}

// Adapter defines the interface that all package adapters must implement
type Adapter interface {
	Type() PackageType
	ParsePath(path string) (*PackagePathInfo, error)
	FormatDownloadResponse(c *gin.Context, result *DownloadResult)
	HandlePublish(c *gin.Context, ctx *PublishContext) (*PublishResult, error)
	HandleDelete(c *gin.Context, ctx *DeleteContext) error
	HandleRepoRequest(c *gin.Context, ctx *RepoRequestContext)
}

// RouteResult 包解析结果
type RouteResult struct {
	Source     string        // 来源仓库名称
	SourceType string        // 来源类型：local 或 proxy
	RepoID     uint          // 来源仓库 ID
	Content    io.ReadCloser // 包内容流
	Size       int64         // 内容大小
	FromCache  bool          // 是否来自缓存
	CacheTTL   int           // 缓存 TTL（秒）
	IsLarge    bool          // 是否大文件（流式传输）
	Name       string        // 包名称
	Version    string        // 包版本
	Filename   string        // 文件名
}
