package types

import "context"

type PackageType string

const (
	NpmType     PackageType = "npm"
	MavenType   PackageType = "maven"
	PyPIType    PackageType = "pypi"
	GoType      PackageType = "go"
	NuGetType   PackageType = "nuget"
	YumType     PackageType = "yum"
	AptType     PackageType = "apt"
	GenericType PackageType = "generic"
)

type PackageIdentity struct {
	Name    string
	Version string
	Type    PackageType
}

type UploadRequest struct {
	Package    interface{}
	Filename   string
	Size       int64
	Checksum   string
	Metadata   map[string]interface{}
	UploadedBy uint
}

type PackageContent struct {
	Content     interface{}
	ContentType string
	Size        int64
	Checksum    string
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
	Version    string
	StorageKey string
	Size       int64
	Checksum   string
}

// Adapter defines the interface that all package adapters must implement
type Adapter interface {
	Type() PackageType
	RoutePrefix() string
	ParsePackagePath(path string) (*PackageIdentity, error)
	Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error)
	Download(ctx context.Context, identity *PackageIdentity) (*PackageContent, error)
	GetMetadata(ctx context.Context, name string) (*PackageMeta, error)
	Delete(ctx context.Context, identity *PackageIdentity) error
	ListVersions(ctx context.Context, name string) ([]string, error)
}
