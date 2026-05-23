package model

import "time"

// PackageType identifies the repository format (npm, maven, pypi, ...).
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

type ComponentStatus string

const (
	StatusDraft      ComponentStatus = "draft"
	StatusPublished  ComponentStatus = "published"
	StatusDeprecated ComponentStatus = "deprecated"
	StatusYanked     ComponentStatus = "yanked"
)

type AssetKind string

const (
	AssetKindPrimary  AssetKind = "primary"
	AssetKindPom      AssetKind = "pom"
	AssetKindSources  AssetKind = "sources"
	AssetKindJavadoc  AssetKind = "javadoc"
	AssetKindMetadata AssetKind = "metadata"
	AssetKindOther    AssetKind = "other"
)

// Component is the installable unit (Nexus component): coordinates include version.
type Component struct {
	BaseModel
	RepositoryID    uint            `gorm:"not null;uniqueIndex:idx_comp_repo_coords,priority:1" json:"repository_id"`
	Format          PackageType     `gorm:"not null;size:20;uniqueIndex:idx_comp_repo_coords,priority:2" json:"format"`
	Namespace       string          `gorm:"size:255;uniqueIndex:idx_comp_repo_coords,priority:3;default:''" json:"namespace,omitempty"`
	Name            string          `gorm:"not null;size:500;uniqueIndex:idx_comp_repo_coords,priority:4" json:"name"`
	Version         string          `gorm:"not null;size:255;uniqueIndex:idx_comp_repo_coords,priority:5" json:"version"`
	DisplayName     string          `gorm:"size:255;index" json:"display_name"`
	Description     string          `gorm:"size:500" json:"description,omitempty"`
	Status          ComponentStatus `gorm:"default:published;index" json:"status"`
	PublishedAt     time.Time       `gorm:"autoCreateTime" json:"published_at"`
	PublishedBy     uint            `json:"published_by,omitempty"`
	Metadata        string          `gorm:"type:text" json:"metadata,omitempty"`
	License         string          `gorm:"size:100;index" json:"license,omitempty"`
	DownloadCount   int64           `gorm:"default:0;index:idx_comp_download_count" json:"download_count"`
	SizeBytes       int64           `gorm:"default:0" json:"size_bytes"`
	FilesDownloaded bool            `json:"files_downloaded" gorm:"default:false"`
	CreatedBy       uint            `json:"created_by,omitempty"`

	Assets         []Asset                `gorm:"foreignKey:ComponentID" json:"assets,omitempty"`
	Dependencies   []ComponentDependency  `gorm:"foreignKey:ComponentID" json:"dependencies,omitempty"`

	// Computed for API (not stored)
	RepositoryName string `gorm:"-" json:"repository_name,omitempty"`
}

func (Component) TableName() string { return "components" }

// Blob is content-addressed storage metadata (Nexus blob).
type Blob struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Ref              string    `gorm:"not null;uniqueIndex;size:500" json:"ref"`
	SHA256           string    `gorm:"size:64;index" json:"checksum_sha256,omitempty"`
	MD5              string    `gorm:"size:32" json:"checksum_md5,omitempty"`
	SizeBytes        int64     `gorm:"default:0" json:"size_bytes"`
	StorageBackendID *uint     `json:"storage_backend_id,omitempty"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Blob) TableName() string { return "blobs" }

// Asset is a file attached to a component (Nexus asset).
type Asset struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ComponentID   uint      `gorm:"not null;uniqueIndex:idx_asset_comp_path,priority:1" json:"component_id"`
	Path          string    `gorm:"size:500;uniqueIndex:idx_asset_comp_path,priority:2;default:''" json:"path,omitempty"`
	FileName      string    `gorm:"not null;size:255" json:"file_name"`
	Kind          AssetKind `gorm:"not null;size:20;index" json:"kind"`
	ContentType   string    `gorm:"size:100" json:"content_type,omitempty"`
	BlobID        uint      `gorm:"not null;index" json:"blob_id"`
	DownloadCount int64     `gorm:"default:0" json:"download_count"`
	DownloadURL   string    `gorm:"size:500" json:"download_url,omitempty"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Blob Blob `gorm:"foreignKey:BlobID" json:"-"`
}

func (Asset) TableName() string { return "assets" }

// ComponentDependency lists dependencies for a component version.
type ComponentDependency struct {
	ID                   uint   `gorm:"primaryKey" json:"id"`
	ComponentID          uint   `gorm:"not null;index" json:"component_id"`
	DepName              string `gorm:"not null;index" json:"dep_name"`
	DepVersionConstraint string `gorm:"not null" json:"dep_version_constraint"`
	DepType              string `gorm:"not null" json:"dep_type"`
	PackageType          string `gorm:"not null" json:"package_type"`
	IsOptional           bool   `gorm:"default:false" json:"is_optional"`
}

func (ComponentDependency) TableName() string { return "component_dependencies" }

// ComponentCatalogEntry groups components by name for browse/search (one row per name in a repo).
type ComponentCatalogEntry struct {
	ID             uint        `json:"id"`
	RepositoryID   uint        `json:"repository_id"`
	Format         PackageType `json:"format"`
	Namespace      string      `json:"namespace,omitempty"`
	Name           string      `json:"name"`
	DisplayName    string      `json:"display_name"`
	Description    string      `json:"description,omitempty"`
	LatestVersion  string      `json:"latest_version,omitempty"`
	VersionCount   int         `json:"version_count"`
	DownloadCount  int64       `json:"download_count"`
	RepositoryName string      `json:"repository_name,omitempty"`
	UpdatedAt      time.Time   `json:"updated_at"`
}
