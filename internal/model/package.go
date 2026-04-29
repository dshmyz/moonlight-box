package model

import "time"

type PackageType string

const (
	PackageTypeNPM     PackageType = "npm"
	PackageTypeMaven   PackageType = "maven"
	PackageTypePyPI    PackageType = "pypi"
	PackageTypeGo      PackageType = "go"
	PackageTypeNuGet   PackageType = "nuget"
	PackageTypeYum     PackageType = "yum"
	PackageTypeApt     PackageType = "apt"
	PackageTypeGeneric PackageType = "generic"
)

type RepositoryType string

const (
	RepoTypeLocal   RepositoryType = "local"
	RepoTypeProxy   RepositoryType = "proxy"
	RepoTypeVirtual RepositoryType = "virtual"
)

type PackageStatus string

const (
	StatusDraft      PackageStatus = "draft"
	StatusPublished  PackageStatus = "published"
	StatusDeprecated PackageStatus = "deprecated"
	StatusYanked     PackageStatus = "yanked"
)

type Package struct {
	BaseModel
	Name           string         `gorm:"not null;index" json:"name"`
	Type           PackageType    `gorm:"not null;index" json:"type"`
	Description    string         `gorm:"size:500" json:"description,omitempty"`
	RepositoryID   uint           `gorm:"index" json:"repository_id"`
	RepositoryType RepositoryType `gorm:"default:local;index" json:"repository_type"`
	Homepage       string         `gorm:"size:500" json:"homepage,omitempty"`
	License        string         `gorm:"size:100" json:"license,omitempty"`
	DownloadCount  int64          `json:"download_count" gorm:"default:0"`
	CreatedBy      uint           `json:"created_by"`
	Versions       []PackageVersion `gorm:"foreignKey:PackageID" json:"versions,omitempty"`
}

type PackageVersion struct {
	ID             uint            `gorm:"primaryKey" json:"id"`
	PackageID      uint            `gorm:"not null;index" json:"package_id"`
	Version        string          `gorm:"not null" json:"version"`
	Status         PackageStatus   `gorm:"default:published" json:"status"`
	StoragePath    string          `gorm:"not null" json:"storage_path"`
	SizeBytes      int64           `gorm:"default:0" json:"size_bytes"`
	ChecksumSHA256 string          `json:"checksum_sha256,omitempty"`
	ChecksumMD5    string          `json:"checksum_md5,omitempty"`
	PublishedAt    time.Time       `gorm:"autoCreateTime" json:"published_at"`
	PublishedBy    uint            `json:"published_by"`
	Metadata       string          `gorm:"type:text" json:"metadata,omitempty"`
	DownloadCount  int             `gorm:"default:0" json:"download_count"`
	Dependencies   []PackageDependency `gorm:"foreignKey:VersionID" json:"dependencies,omitempty"`
	Package        Package         `gorm:"foreignKey:PackageID" json:"-"`
}

type PackageDependency struct {
	ID                   uint   `gorm:"primaryKey" json:"id"`
	VersionID            uint   `gorm:"not null;index" json:"version_id"`
	DepName              string `gorm:"not null;index" json:"dep_name"`
	DepVersionConstraint string `gorm:"not null" json:"dep_version_constraint"`
	DepType              string `gorm:"not null" json:"dep_type"`
	PackageType          string `gorm:"not null" json:"package_type"`
	IsOptional           bool   `gorm:"default:false" json:"is_optional"`
}
