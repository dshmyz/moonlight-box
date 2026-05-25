package runtime

import (
	"io"
	"time"
)

type ArtifactKey struct {
	RepositoryID string
	Format       string
	Coordinates  map[string]string
	Filename     string
	Extension    string
	RemoteURL    string
}

func (k *ArtifactKey) String() string {
	return k.RepositoryID + "/" + k.Format + "/" + k.Filename
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
