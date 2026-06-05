package runtime

import "context"

type HostedRuntime struct {
	MetadataStore MetadataStore
	BlobStore     BlobStore
	RepositoryID  string
}

func (n *HostedRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	key.RepositoryID = n.RepositoryID
	artifact, err := n.MetadataStore.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(artifact.BlobRefs) > 0 {
		rc, openErr := n.BlobStore.Open(artifact.BlobRefs[0])
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
	query.RepositoryID = n.RepositoryID
	return n.MetadataStore.Query(ctx, query)
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
		_ = n.BlobStore.Delete(ref)
	}
	return nil
}
