package model

import "time"

type PackageType string

const (
	PackageTypeNPM     PackageType = "npm"
	PackageTypeMaven   PackageType = "maven"
	PackageTypePyPI    PackageType = "pypi"
	PackageTypeGo      PackageType = "go"
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

type PackageFileType string

const (
	FileTypePrimary  PackageFileType = "primary"
	FileTypePom      PackageFileType = "pom"
	FileTypeSources  PackageFileType = "sources"
	FileTypeJavadoc  PackageFileType = "javadoc"
	FileTypeMetadata PackageFileType = "metadata"
	FileTypeOther    PackageFileType = "other"
)

type Package struct {
	BaseModel
	Name           string           `gorm:"not null;uniqueIndex:idx_pkg_name_type" json:"name"`
	DisplayName    string           `gorm:"size:255;index" json:"display_name"`
	Type           PackageType      `gorm:"not null;uniqueIndex:idx_pkg_name_type" json:"type"`
	Description    string           `gorm:"size:500" json:"description,omitempty"`
	RepositoryID   uint             `gorm:"index" json:"repository_id"`
	RepositoryType RepositoryType   `gorm:"default:local;index" json:"repository_type"`
	RepositoryName string           `gorm:"-" json:"repository_name"`
	Homepage       string           `gorm:"size:500" json:"homepage,omitempty"`
	License        string           `gorm:"size:100" json:"license,omitempty"`
	DownloadCount  int64            `json:"download_count" gorm:"default:0"`
	CreatedBy      uint             `json:"created_by"`
	Versions       []PackageVersion `gorm:"foreignKey:PackageID" json:"versions,omitempty"`

	// 元数据同步标记
	MetadataSynced bool       `json:"metadata_synced" gorm:"default:false"`
	MetadataSyncAt *time.Time `json:"metadata_sync_at"`
}

type PackageVersion struct {
	ID             uint                `gorm:"primaryKey" json:"id"`
	PackageID      uint                `gorm:"not null;uniqueIndex:idx_ver_pkg_version" json:"package_id"`
	Version        string              `gorm:"not null;uniqueIndex:idx_ver_pkg_version" json:"version"`
	Status         PackageStatus       `gorm:"default:published;index" json:"status"`
	StoragePath    string              `gorm:"size:500" json:"storage_path"`
	PublishedAt    time.Time           `gorm:"autoCreateTime" json:"published_at"`
	PublishedBy    uint                `json:"published_by"`
	Metadata       string              `gorm:"type:text" json:"metadata,omitempty"`
	DownloadCount  int                 `gorm:"default:0" json:"download_count"`
	SizeBytes      int64               `gorm:"default:0" json:"size_bytes"`
	ChecksumMD5    string              `gorm:"size:32" json:"checksum_md5,omitempty"`
	ChecksumSHA256 string              `gorm:"size:64" json:"checksum_sha256,omitempty"`
	Dependencies   []PackageDependency `gorm:"foreignKey:VersionID" json:"dependencies,omitempty"`
	Files          []PackageFile       `gorm:"foreignKey:VersionID" json:"files,omitempty"`
	Package        Package             `gorm:"foreignKey:PackageID" json:"-"`

	// 文件下载状态
	FilesDownloaded bool `json:"files_downloaded" gorm:"default:false"`
}

type PackageFile struct {
	ID             uint            `gorm:"primaryKey" json:"id"`
	VersionID      uint            `gorm:"not null;uniqueIndex:idx_file_ver_filename" json:"version_id"`
	Filename       string          `gorm:"not null;size:255;uniqueIndex:idx_file_ver_filename" json:"filename"`
	FileType       PackageFileType `gorm:"not null;size:20;index" json:"file_type"`
	StoragePath    string          `gorm:"not null;size:500" json:"storage_path"`
	SizeBytes      int64           `gorm:"default:0" json:"size_bytes"`
	ChecksumSHA256 string          `gorm:"size:64" json:"checksum_sha256,omitempty"`
	ChecksumMD5    string          `gorm:"size:32" json:"checksum_md5,omitempty"`
	DownloadCount  int             `gorm:"default:0" json:"download_count"`
	Version        PackageVersion  `gorm:"foreignKey:VersionID" json:"-"`
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
