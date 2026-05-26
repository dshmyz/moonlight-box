package model

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
