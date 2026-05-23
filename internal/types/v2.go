package types

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

type ArtifactKey struct {
	RepositoryID string
	Format       string
	Coordinates  map[string]string
	Filename     string
	Extension    string
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
}

type ProjectionQuery struct {
	RepositoryID string
	Format       string
	Kind         string
	Coordinates  map[string]string
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

type UploadSession interface {
	PutBlob(ctx context.Context, blob io.Reader) (BlobRef, error)
	PutArtifact(ctx context.Context, artifact *Artifact) error
	Commit(ctx context.Context) error
	Abort(ctx context.Context) error
}

type RepositoryRuntime interface {
	GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error)
	QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
	RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error)
	BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error)
}

type RepositoryNode interface {
	GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error)
	QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
	RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error)
	BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error)
}

type GroupRuntime struct {
	Members  []RepositoryNode
	Writable RepositoryNode
}

func (g *GroupRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	for _, node := range g.Members {
		artifact, err := node.GetArtifact(ctx, key)
		if err == nil {
			return artifact, nil
		}
	}
	return nil, ErrNotFound
}

func (g *GroupRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	allArtifacts := make([]*Artifact, 0)
	seen := make(map[string]bool)

	for _, node := range g.Members {
		artifacts, err := node.QueryArtifacts(ctx, query)
		if err != nil {
			continue
		}
		for _, a := range artifacts {
			if !seen[a.ID] {
				seen[a.ID] = true
				allArtifacts = append(allArtifacts, a)
			}
		}
	}
	return allArtifacts, nil
}

func (g *GroupRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	for _, node := range g.Members {
		result, err := node.RenderProjection(ctx, query)
		if err == nil {
			return result, nil
		}
	}
	return nil, ErrNotFound
}

func (g *GroupRuntime) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	if g.Writable == nil {
		return nil, ErrReadOnly
	}
	return g.Writable.BeginUpload(ctx, request)
}

type HostedRuntime struct {
	MetadataStore MetadataStore
	BlobStore     BlobStore
	RepositoryID  string
}

func (n *HostedRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	return n.MetadataStore.Get(ctx, key)
}

func (n *HostedRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return n.MetadataStore.Query(ctx, query)
}

func (n *HostedRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	artifacts, err := n.MetadataStore.Query(ctx, ArtifactQuery{
		RepositoryID: query.RepositoryID,
		Format:       query.Format,
		Coordinates:  query.Coordinates,
	})
	if err != nil {
		return nil, err
	}
	return &ProjectionResult{
		Dynamic:  true,
		Artifact: artifacts[0],
	}, nil
}

func (n *HostedRuntime) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, ErrNotImplemented
}

type ProxyRuntime struct {
	MetadataStore MetadataStore
	BlobStore     BlobStore
	RemoteClient  RemoteClient
	RepositoryID  string
	CachePolicy   CachePolicy
}

func (n *ProxyRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	artifact, err := n.MetadataStore.Get(ctx, key)
	if err == nil {
		return artifact, nil
	}

	metadata, err := n.RemoteClient.FetchMetadata(ctx, key)
	if err != nil {
		return nil, err
	}
	if !metadata.Exists {
		return nil, ErrNotFound
	}

	artifact = &Artifact{
		RepositoryID: n.RepositoryID,
		Format:       key.Format,
		Coordinates:  key.Coordinates,
	}

	if err := n.MetadataStore.Put(ctx, artifact); err != nil {
		return nil, err
	}

	return artifact, nil
}

func (n *ProxyRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return n.MetadataStore.Query(ctx, query)
}

func (n *ProxyRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	artifacts, err := n.MetadataStore.Query(ctx, ArtifactQuery{
		RepositoryID: query.RepositoryID,
		Format:       query.Format,
		Coordinates:  query.Coordinates,
	})
	if err != nil {
		return nil, err
	}
	return &ProjectionResult{
		Dynamic:  true,
		Artifact: artifacts[0],
	}, nil
}

func (n *ProxyRuntime) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, ErrReadOnly
}

type MetadataStore interface {
	Get(ctx context.Context, key ArtifactKey) (*Artifact, error)
	Put(ctx context.Context, artifact *Artifact) error
	Delete(ctx context.Context, key ArtifactKey) error
	Query(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
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

type BlobStore interface {
	Put(reader io.Reader) (BlobRef, error)
	Open(ref BlobRef) (io.ReadCloser, error)
	Stat(ref BlobRef) (*BlobMetadata, error)
	Delete(ref BlobRef) error
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
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type RemoteMetadata struct {
	Exists     bool
	Digest     string
	Size       int64
	ModifiedAt time.Time
}

type RemoteClient interface {
	FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error)
	FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error)
}

type CachePolicy struct {
	MetadataTTL     time.Duration
	BlobTTL         time.Duration
	NegativeTTL     time.Duration
	MaxBlobSize     int64
	SnapshotRefresh bool
}

type RepositoryManager interface {
	Get(id string) *Repository
	Reload() error
}

type DefaultRepositoryManager struct {
	repos sync.Map
}

func NewDefaultRepositoryManager() *DefaultRepositoryManager {
	return &DefaultRepositoryManager{}
}

func (m *DefaultRepositoryManager) Get(id string) *Repository {
	if v, ok := m.repos.Load(id); ok {
		return v.(*Repository)
	}
	return nil
}

func (m *DefaultRepositoryManager) Set(repo *Repository) {
	m.repos.Store(repo.Name, repo)
}

func (m *DefaultRepositoryManager) Reload() error {
	return nil
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

type RepositoryPathResolver interface {
	Resolve(req *http.Request) (*ResolvedRepository, error)
}

type Nexus3Resolver struct{}

func (r *Nexus3Resolver) Resolve(req *http.Request) (*ResolvedRepository, error) {
	path := req.URL.Path
	if len(path) < 11 || path[:11] != "/repository/" {
		return nil, ErrNotMatched
	}
	remaining := path[11:]
	idx := 0
	for i, c := range remaining {
		if c == '/' {
			idx = i
			break
		}
	}
	repoName := remaining[:idx]
	remainingPath := remaining[idx:]
	return &ResolvedRepository{
		Repository:    &Repository{Name: repoName},
		RemainingPath: remainingPath,
		RouteStyle:    Nexus3Route,
	}, nil
}

type Nexus2Resolver struct{}

func (r *Nexus2Resolver) Resolve(req *http.Request) (*ResolvedRepository, error) {
	path := req.URL.Path
	if len(path) < 21 || path[:21] != "/content/repositories/" {
		return nil, ErrNotMatched
	}
	remaining := path[21:]
	idx := 0
	for i, c := range remaining {
		if c == '/' {
			idx = i
			break
		}
	}
	repoName := remaining[:idx]
	remainingPath := remaining[idx:]
	return &ResolvedRepository{
		Repository:    &Repository{Name: repoName},
		RemainingPath: remainingPath,
		RouteStyle:    Nexus2Route,
	}, nil
}

type CompositeResolver struct {
	Resolvers []RepositoryPathResolver
}

func (r *CompositeResolver) Resolve(req *http.Request) (*ResolvedRepository, error) {
	for _, resolver := range r.Resolvers {
		result, err := resolver.Resolve(req)
		if err == nil {
			return result, nil
		}
	}
	return nil, ErrNotMatched
}

type ProtocolPlugin interface {
	Name() string
	Handle(ctx *RequestContext, runtime RepositoryRuntime) error
}

type RequestContext struct {
	Writer         http.ResponseWriter
	Request        *http.Request
	Repository     *Repository
	Runtime        RepositoryRuntime
	RepositoryPath string
	RouteStyle     RouteStyle
}

type RepositoryRouter struct {
	Resolver RepositoryPathResolver
	Manager  RepositoryManager
	Plugins  map[string]ProtocolPlugin
}

func NewRepositoryRouter(resolver RepositoryPathResolver, manager RepositoryManager) *RepositoryRouter {
	return &RepositoryRouter{
		Resolver: resolver,
		Manager:  manager,
		Plugins:  make(map[string]ProtocolPlugin),
	}
}

func (r *RepositoryRouter) RegisterPlugin(format string, plugin ProtocolPlugin) {
	r.Plugins[format] = plugin
}

func (r *RepositoryRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resolved, err := r.Resolver.Resolve(req)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}
	repo := r.Manager.Get(resolved.Repository.Name)
	if repo == nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}
	ctx := &RequestContext{
		Writer:         w,
		Request:        req,
		Repository:     repo,
		RepositoryPath: resolved.RemainingPath,
		RouteStyle:     resolved.RouteStyle,
	}
	r.handleRequest(ctx)
}

func (r *RepositoryRouter) handleRequest(ctx *RequestContext) {
	plugin, ok := r.Plugins[ctx.Repository.Format]
	if !ok {
		http.Error(ctx.Writer, "Unsupported format", http.StatusBadRequest)
		return
	}

	runtime := ctx.Repository.Runtime
	if runtime == nil {
		http.Error(ctx.Writer, "Repository runtime not available", http.StatusInternalServerError)
		return
	}

	if err := plugin.Handle(ctx, runtime); err != nil {
		if err == ErrNotFound {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
	}
}
