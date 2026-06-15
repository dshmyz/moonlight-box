package types

import (
	"io"
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

// RequestType 定义请求意图类型
type RequestType string

const (
	RequestDownload      RequestType = "download"
	RequestMetadata      RequestType = "metadata"
	RequestList          RequestType = "list"
	RequestChecksum      RequestType = "checksum"
	RequestGPG           RequestType = "gpg"
	RequestDelete        RequestType = "delete"
	RequestUnknown       RequestType = "unknown"
	RequestDistTags      RequestType = "dist-tags"       // npm dist-tags 查询
	RequestDistTagUpdate RequestType = "dist-tag-update" // npm dist-tag 更新
)

// RequestIntent 表示 adapter 解析出的请求意图
type RequestIntent struct {
	Type        RequestType
	Name        string
	Version     string
	Filename    string
	Path        string
	PkgPathInfo *PackagePathInfo
	Extra       map[string]interface{}
}

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

type PublishResult struct {
	PackageName    string
	StorageName    string
	Version        string
	Filename       string
	Content        io.Reader
	Size           int64
	StorageVersion string
	Metadata       map[string]interface{}
	DownloadURL    string
	Response       interface{}
}

// RouteResult 包解析结果
type RouteResult struct {
	Source      string        // 来源仓库名称
	SourceType  string        // 来源类型：local 或 proxy
	RepoID      uint          // 来源仓库 ID
	Content     io.ReadCloser // 包内容流
	Size        int64         // 内容大小
	FromCache   bool          // 是否来自缓存
	CacheTTL    int           // 缓存 TTL（秒）
	IsLarge     bool          // 是否大文件（流式传输）
	Name        string        // 包名称
	Version     string        // 包版本
	Filename    string        // 文件名
	ContentType string        // 内容类型（如 application/json, text/plain, application/zip 等）
}
