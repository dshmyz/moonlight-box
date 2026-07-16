package runtime

import (
	"context"
	"io"
	"strings"
)

type HostedRuntime struct {
	MetadataStore MetadataStore
	BlobStore     BlobStore
	RepositoryID  string
	Blocker       PackageBlocker
	Format        string
}

func (n *HostedRuntime) checkBlocked(name, version string) error {
	if n.Blocker != nil && name != "" && n.Blocker.IsBlocked(n.Format, name, version) {
		return ErrBlocked
	}
	return nil
}

func (n *HostedRuntime) checkBlockedWithAttrs(key ArtifactKey, artifact *Artifact) error {
	if n.Blocker == nil || artifact == nil || key.Name == "" {
		return nil
	}
	attrs := make(map[string]interface{}, len(artifact.Attributes))
	for name, value := range artifact.Attributes {
		attrs[name] = value
	}
	blocked, _ := n.Blocker.IsBlockedWithAttrs(n.Format, key.Name, key.Version, attrs)
	if blocked {
		return ErrBlocked
	}
	return nil
}

func (n *HostedRuntime) filterBlockedArtifacts(artifacts []*Artifact) []*Artifact {
	if n.Blocker == nil || len(artifacts) == 0 {
		return artifacts
	}
	result := make([]*Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil || artifact.Name == "" {
			result = append(result, artifact)
			continue
		}
		attrs := make(map[string]interface{}, len(artifact.Attributes))
		for name, value := range artifact.Attributes {
			attrs[name] = value
		}
		blocked, _ := n.Blocker.IsBlockedWithAttrs(n.Format, artifact.Name, artifact.Version, attrs)
		if !blocked {
			result = append(result, artifact)
		}
	}
	return result
}

func (n *HostedRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	if err := n.checkBlocked(key.Name, key.Version); err != nil {
		return nil, err
	}
	key.RepositoryID = n.RepositoryID
	artifact, err := n.MetadataStore.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if err := n.checkBlockedWithAttrs(key, artifact); err != nil {
		return nil, err
	}
	if len(artifact.BlobRefs) > 0 {
		var (
			rc      io.ReadCloser
			openErr error
		)
		if store, ok := n.BlobStore.(ContextBlobOpener); ok {
			rc, openErr = store.OpenContext(ctx, artifact.BlobRefs[0])
		} else {
			rc, openErr = n.BlobStore.Open(artifact.BlobRefs[0])
		}
		if openErr != nil {
			return nil, openErr
		}
		artifact.Content = rc
		artifact.SizeBytes = artifact.BlobRefs[0].Size
	}
	// Hosted 仓库的文件就在本地，始终视为缓存命中
	artifact.FromCache = true
	return artifact, nil
}

func (n *HostedRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	if err := n.checkBlocked(query.Name, query.Version); err != nil {
		return nil, err
	}
	query.RepositoryID = n.RepositoryID
	artifacts, err := n.MetadataStore.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(artifacts) > 0 {
		return n.filterBlockedArtifacts(artifacts), nil
	}
	if isPyPIPackageListProjection(query) {
		query.RemotePath = ""
		query.RemotePathPrefix = ""
		artifacts, err = n.MetadataStore.Query(ctx, query)
		if err != nil {
			return nil, err
		}
		return n.filterBlockedArtifacts(artifacts), nil
	}
	return n.filterBlockedArtifacts(artifacts), nil
}

func isPyPIPackageListProjection(query ArtifactQuery) bool {
	if query.Format != "pypi" || query.Name == "" || query.RemotePath == "" {
		return false
	}
	return strings.Trim(query.RemotePath, "/") == "simple/"+query.Name
}

func (n *HostedRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	artifacts, err := n.MetadataStore.Query(ctx, ArtifactQuery{
		RepositoryID: n.RepositoryID,
		Format:       query.Format,
		Kind:         query.Kind,
		Name:         query.Name,
		Namespace:    query.Namespace,
		Version:      query.Version,
		Path:         query.Path,
		Filename:     query.Filename,
		RemotePath:   query.RemotePath,
		IdentityKey:  query.IdentityKey,
		Qualifiers:   query.Qualifiers,
	})
	if err != nil {
		return nil, err
	}
	if len(artifacts) == 0 {
		return nil, ErrNotFound
	}
	return &ProjectionResult{
		Dynamic:  true,
		Artifact: artifacts[0],
	}, nil
}

func (n *HostedRuntime) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return NewHostedUploadSession(n.MetadataStore, n.BlobStore), nil
}

func (n *HostedRuntime) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	key.RepositoryID = n.RepositoryID
	artifact, err := n.MetadataStore.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := n.MetadataStore.Delete(ctx, key); err != nil {
		return err
	}
	for _, ref := range artifact.BlobRefs {
		if store, ok := n.BlobStore.(ContextBlobDeleter); ok {
			_ = store.DeleteContext(ctx, ref)
		} else {
			_ = n.BlobStore.Delete(ref)
		}
	}
	return nil
}
