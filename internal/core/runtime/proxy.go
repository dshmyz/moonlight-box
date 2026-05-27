package runtime

import (
	"context"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const maxMetadataCacheSize = 10000 // 内存缓存上限，防止无限增长

type ProxyRuntime struct {
	MetadataStore MetadataStore
	BlobStore     BlobStore
	RemoteClient  RemoteClient
	RepositoryID  string
	RemoteBaseURL string
	CachePolicy   CachePolicy
	Fetcher       RemoteFetcher  // 由 Plugin 实现，Runtime 控制回源时机
	Blocker       PackageBlocker // 阻断规则检查
	Format        string         // 仓库协议类型，供阻断检查使用

	metadataCacheMu sync.RWMutex
	metadataCache   map[string]cachedArtifact
	negativeCache   map[string]time.Time
}

type cachedArtifact struct {
	artifact  *Artifact
	expiresAt time.Time
	negative  bool
}

func (n *ProxyRuntime) checkBlocked(key ArtifactKey) error {
	if n.Blocker == nil {
		return nil
	}
	name := key.Coordinates["name"]
	ver := key.Coordinates["version"]
	if name != "" && n.Blocker.IsBlocked(n.Format, name, ver) {
		return ErrBlocked
	}
	return nil
}

func (n *ProxyRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	if err := n.checkBlocked(key); err != nil {
		return nil, err
	}
	// 统一使用带 RepositoryID 的 key
	key.RepositoryID = n.RepositoryID

	if n.isNegativeCached(key) {
		return nil, ErrNotFound
	}
	if artifact, ok := n.getCachedArtifact(key); ok {
		if err := n.openArtifactContent(artifact); err != nil {
			return nil, err
		}
		return artifact, nil
	}

	artifact, err := n.MetadataStore.Get(ctx, key)
	if err == nil {
		if refreshErr := n.refreshStaleMetadata(ctx, artifact, key); refreshErr != nil {
			return nil, refreshErr
		}
		if ensureErr := n.ensureArtifactBlob(ctx, artifact, key); ensureErr != nil {
			return nil, ensureErr
		}
		if err := n.openArtifactContent(artifact); err != nil {
			return nil, err
		}
		n.setCachedArtifact(key, artifact)
		return artifact, nil
	}

	key.RemoteURL = n.buildRemoteURL(key)
	metadata, err := n.RemoteClient.FetchMetadata(ctx, key)
	if err != nil {
		return nil, err
	}
	if !metadata.Exists {
		n.setNegativeCache(key)
		return nil, ErrNotFound
	}

	now := time.Now()
	artifact = &Artifact{
		RepositoryID: n.RepositoryID,
		Format:       key.Format,
		Coordinates:  key.Coordinates,
		Properties: map[string]string{
			"remote_digest": metadata.Digest,
			"remote_size":   strconv.FormatInt(metadata.Size, 10),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := n.MetadataStore.Put(ctx, artifact); err != nil {
		return nil, err
	}

	if ensureErr := n.ensureArtifactBlob(ctx, artifact, key); ensureErr != nil {
		return nil, ensureErr
	}
	if err := n.openArtifactContent(artifact); err != nil {
		return nil, err
	}
	n.setCachedArtifact(key, artifact)
	return artifact, nil
}

func (n *ProxyRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	if n.Blocker != nil {
		if name := query.Coordinates["name"]; name != "" && n.Blocker.IsBlocked(n.Format, name, query.Coordinates["version"]) {
			return nil, ErrBlocked
		}
	}
	query.RepositoryID = n.RepositoryID
	artifacts, err := n.MetadataStore.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	// 检查缓存是否过期，过期则刷新
	if len(artifacts) > 0 && n.CachePolicy.MetadataTTL > 0 {
		oldest := artifacts[0].UpdatedAt
		for _, a := range artifacts {
			if a.UpdatedAt.Before(oldest) {
				oldest = a.UpdatedAt
			}
		}
		if time.Since(oldest) > n.CachePolicy.MetadataTTL {
			// 缓存过期，尝试回源刷新
			if n.Fetcher != nil && n.RemoteBaseURL != "" {
				fetched, fetchErr := n.Fetcher.FetchRemote(ctx, n.RemoteBaseURL, query.RemotePath)
				if fetchErr == nil && len(fetched) > 0 {
					for _, a := range fetched {
						a.RepositoryID = n.RepositoryID
						_ = n.MetadataStore.Put(ctx, a)
					}
					return fetched, nil
				}
				// 回源失败，返回过期数据
				logrus.WithError(fetchErr).Warn("QueryArtifacts: refresh failed, serving stale data")
			}
		}
	}

	if len(artifacts) > 0 {
		return artifacts, nil
	}
	// 本地缓存为空,通过 RemoteFetcher 回源
	if n.Fetcher != nil && n.RemoteBaseURL != "" {
		fetched, fetchErr := n.Fetcher.FetchRemote(ctx, n.RemoteBaseURL, query.RemotePath)
		if fetchErr != nil {
			return nil, fetchErr
		}
		// 缓存回源结果
		for _, a := range fetched {
			a.RepositoryID = n.RepositoryID
			_ = n.MetadataStore.Put(ctx, a)
		}
		return fetched, nil
	}
	return artifacts, nil
}

func (n *ProxyRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	artifacts, err := n.MetadataStore.Query(ctx, ArtifactQuery{
		RepositoryID: n.RepositoryID,
		Format:       query.Format,
		Coordinates:  query.Coordinates,
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

func (n *ProxyRuntime) ensureArtifactBlob(ctx context.Context, artifact *Artifact, key ArtifactKey) error {
	if artifact == nil {
		return ErrNotFound
	}
	if len(artifact.BlobRefs) > 0 {
		return nil
	}
	// Only artifact download paths require blob fetch; metadata-only keys have no filename.
	if key.Filename == "" {
		return nil
	}

	key.RemoteURL = n.buildRemoteURL(key)
	if key.RemoteURL == "" {
		return ErrNotFound
	}
	blobReader, err := n.RemoteClient.FetchBlob(ctx, key)
	if err != nil {
		return err
	}
	defer blobReader.Close()

	blobRef, err := n.BlobStore.Put(blobReader)
	if err != nil {
		return err
	}
	artifact.BlobRefs = []BlobRef{blobRef}
	return n.MetadataStore.Put(ctx, artifact)
}

func (n *ProxyRuntime) openArtifactContent(artifact *Artifact) error {
	if len(artifact.BlobRefs) == 0 {
		return nil
	}
	rc, err := n.BlobStore.Open(artifact.BlobRefs[0])
	if err != nil {
		return err
	}
	artifact.Content = rc
	return nil
}

func (n *ProxyRuntime) refreshStaleMetadata(ctx context.Context, artifact *Artifact, key ArtifactKey) error {
	if artifact == nil || n.CachePolicy.MetadataTTL <= 0 {
		return nil
	}
	if time.Since(artifact.UpdatedAt) < n.CachePolicy.MetadataTTL {
		return nil
	}

	key.RemoteURL = n.buildRemoteURL(key)
	if key.RemoteURL == "" {
		return nil
	}
	remoteMeta, err := n.RemoteClient.FetchMetadata(ctx, key)
	if err != nil {
		logrus.WithError(err).Warn("remote unreachable, serving cached artifact")
		return nil
	}
	if !remoteMeta.Exists {
		_ = n.MetadataStore.Delete(ctx, key)
		n.invalidateCachedArtifact(key)
		n.setNegativeCache(key)
		return ErrNotFound
	}

	if artifact.Properties == nil {
		artifact.Properties = map[string]string{}
	}
	oldDigest := artifact.Properties["remote_digest"]
	oldSize := artifact.Properties["remote_size"]
	newSize := strconv.FormatInt(remoteMeta.Size, 10)
	changed := remoteMeta.Digest != "" && oldDigest != "" && remoteMeta.Digest != oldDigest
	if !changed && oldSize != "" && oldSize != newSize {
		changed = true
	}

	artifact.Properties["remote_digest"] = remoteMeta.Digest
	artifact.Properties["remote_size"] = newSize
	if changed {
		artifact.BlobRefs = nil
	}
	return n.MetadataStore.Put(ctx, artifact)
}

func (n *ProxyRuntime) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, ErrReadOnly
}

func (n *ProxyRuntime) getCachedArtifact(key ArtifactKey) (*Artifact, bool) {
	if n.CachePolicy.MetadataTTL <= 0 {
		return nil, false
	}
	n.metadataCacheMu.RLock()
	entry, ok := n.metadataCache[key.String()]
	n.metadataCacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) || entry.negative {
		if ok {
			n.invalidateCachedArtifact(key)
		}
		return nil, false
	}
	return entry.artifact, true
}

func (n *ProxyRuntime) setCachedArtifact(key ArtifactKey, artifact *Artifact) {
	if n.CachePolicy.MetadataTTL <= 0 || artifact == nil {
		return
	}
	n.metadataCacheMu.Lock()
	if n.metadataCache == nil {
		n.metadataCache = map[string]cachedArtifact{}
	}
	// 超过上限时清空缓存，防止内存无限增长
	if len(n.metadataCache) >= maxMetadataCacheSize {
		n.metadataCache = map[string]cachedArtifact{}
	}
	n.metadataCache[key.String()] = cachedArtifact{artifact: artifact, expiresAt: time.Now().Add(n.CachePolicy.MetadataTTL)}
	n.metadataCacheMu.Unlock()
}

func (n *ProxyRuntime) invalidateCachedArtifact(key ArtifactKey) {
	n.metadataCacheMu.Lock()
	delete(n.metadataCache, key.String())
	n.metadataCacheMu.Unlock()
}

func (n *ProxyRuntime) isNegativeCached(key ArtifactKey) bool {
	if n.CachePolicy.NegativeTTL <= 0 {
		return false
	}
	n.metadataCacheMu.RLock()
	expiresAt, ok := n.negativeCache[key.String()]
	n.metadataCacheMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(expiresAt) {
		n.metadataCacheMu.Lock()
		delete(n.negativeCache, key.String())
		n.metadataCacheMu.Unlock()
		return false
	}
	return true
}

func (n *ProxyRuntime) setNegativeCache(key ArtifactKey) {
	if n.CachePolicy.NegativeTTL <= 0 {
		return
	}
	n.metadataCacheMu.Lock()
	if n.negativeCache == nil {
		n.negativeCache = map[string]time.Time{}
	}
	n.negativeCache[key.String()] = time.Now().Add(n.CachePolicy.NegativeTTL)
	n.metadataCacheMu.Unlock()
}

func (n *ProxyRuntime) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	key.RepositoryID = n.RepositoryID
	return ErrReadOnly
}

func (n *ProxyRuntime) buildRemoteURL(key ArtifactKey) string {
	if key.RemoteURL != "" {
		return key.RemoteURL
	}
	if n.RemoteBaseURL == "" {
		return ""
	}
	base := strings.TrimRight(n.RemoteBaseURL, "/")
	p := strings.TrimLeft(key.Coordinates["path"], "/")
	filename := strings.TrimLeft(key.Filename, "/")
	switch {
	case p == "" && filename == "":
		return base
	case p == "":
		return base + "/" + filename
	case filename == "":
		return base + "/" + p
	default:
		return base + "/" + path.Join(p, filename)
	}
}
