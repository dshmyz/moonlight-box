package runtime

import (
	"context"
	"io"
	"net/http"
)

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

type MetadataStore interface {
	Get(ctx context.Context, key ArtifactKey) (*Artifact, error)
	Put(ctx context.Context, artifact *Artifact) error
	Delete(ctx context.Context, key ArtifactKey) error
	Query(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
}

type BlobStore interface {
	Put(reader io.Reader) (BlobRef, error)
	Open(ref BlobRef) (io.ReadCloser, error)
	Stat(ref BlobRef) (*BlobMetadata, error)
	Delete(ref BlobRef) error
}

type RemoteClient interface {
	FetchMetadata(ctx context.Context, key ArtifactKey) (*RemoteMetadata, error)
	FetchBlob(ctx context.Context, key ArtifactKey) (io.ReadCloser, error)
}

type RepositoryManager interface {
	Get(id string) *Repository
	Reload() error
}

type RepositoryPathResolver interface {
	Resolve(req *http.Request) (*ResolvedRepository, error)
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
