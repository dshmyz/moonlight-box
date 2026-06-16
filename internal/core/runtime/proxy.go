package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/metrics"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

const maxMetadataCacheSize = 10000 // 内存缓存上限，防止无限增长
const maxNegativeCacheSize = 5000  // 负缓存上限，防止 DoS

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
	fetchGroup      singleflight.Group
	refreshingMu    sync.Mutex
	refreshingPaths map[string]struct{}
}

type cachedArtifact struct {
	artifact  *Artifact
	expiresAt time.Time
	negative  bool
}

type proxyCountingReader struct {
	reader io.Reader
	n      int64
}

func (r *proxyCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.n += int64(n)
	return n, err
}

func (n *ProxyRuntime) checkBlocked(key ArtifactKey) error {
	if n.Blocker == nil {
		return nil
	}
	name := key.Name
	ver := key.Version
	if name != "" && n.Blocker.IsBlocked(n.Format, name, ver) {
		return ErrBlocked
	}
	return nil
}

func (n *ProxyRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	start := time.Now()
	if err := n.checkBlocked(key); err != nil {
		return nil, err
	}
	// 统一使用带 RepositoryID 的 key
	key.RepositoryID = n.RepositoryID

	logrus.WithFields(logrus.Fields{
		"repositoryID": n.RepositoryID,
		"format":       key.Format,
		"name":         key.Name,
		"version":      key.Version,
		"remotePath":   key.RemotePath,
		"filename":     key.Filename,
	}).Debug("proxy: GetArtifact called")

	if n.isNegativeCached(key) {
		metrics.RecordProxyNegativeCacheHit(n.Format)
		logrus.WithFields(logrus.Fields{
			"key": key.String(),
		}).Debug("proxy: GetArtifact negative cache hit")
		return nil, ErrNotFound
	}
	if artifact, ok := n.getCachedArtifact(key); ok {
		metrics.RecordCacheHit(n.RepositoryID, n.Format)
		logrus.WithFields(logrus.Fields{
			"key":      key.String(),
			"duration": time.Since(start).Seconds(),
		}).Debug("proxy: GetArtifact memory cache hit")
		artifact = cloneArtifactForResponse(artifact)
		if err := n.openArtifactContent(ctx, artifact); err != nil {
			return nil, err
		}
		artifact.FromCache = true
		artifact.RemoteURL = ""
		if len(artifact.BlobRefs) > 0 {
			artifact.SizeBytes = artifact.BlobRefs[0].Size
		}
		return artifact, nil
	}

	sfKey := "get:" + key.String()
	result, err, _ := n.fetchGroup.Do(sfKey, func() (interface{}, error) {
		if n.isNegativeCached(key) {
			metrics.RecordProxyNegativeCacheHit(n.Format)
			return nil, ErrNotFound
		}
		if artifact, ok := n.getCachedArtifact(key); ok {
			metrics.RecordCacheHit(n.RepositoryID, n.Format)
			return getArtifactResult{artifact: artifact, fromCache: true}, nil
		}
		return n.loadArtifact(ctx, key, start)
	})
	if err != nil {
		return nil, err
	}
	res := result.(getArtifactResult)
	artifact := cloneArtifactForResponse(res.artifact)
	if err := n.openArtifactContent(ctx, artifact); err != nil {
		return nil, err
	}
	artifact.FromCache = res.fromCache
	if res.fromCache {
		artifact.RemoteURL = ""
	} else {
		artifact.RemoteURL = res.remoteURL
	}
	if len(artifact.BlobRefs) > 0 {
		artifact.SizeBytes = artifact.BlobRefs[0].Size
	}
	return artifact, nil
}

type getArtifactResult struct {
	artifact  *Artifact
	fromCache bool
	remoteURL string
}

func (n *ProxyRuntime) loadArtifact(ctx context.Context, key ArtifactKey, start time.Time) (getArtifactResult, error) {
	artifact, err := n.MetadataStore.Get(ctx, key)
	if err == nil {
		if refreshErr := n.refreshStaleMetadata(ctx, artifact, key); refreshErr != nil {
			return getArtifactResult{}, refreshErr
		}
		hadBlob := len(artifact.BlobRefs) > 0
		if ensureErr := n.ensureArtifactBlob(ctx, artifact, key); ensureErr != nil {
			return getArtifactResult{}, ensureErr
		}
		if len(artifact.BlobRefs) > 0 {
			artifact.SizeBytes = artifact.BlobRefs[0].Size
		}
		n.setCachedArtifact(key, artifact)
		logrus.WithFields(logrus.Fields{
			"key":      key.String(),
			"duration": time.Since(start).Seconds(),
		}).Debug("proxy: GetArtifact metadata store hit")
		fromCache := hadBlob && artifact.RemoteURL == ""
		return getArtifactResult{artifact: artifact, fromCache: fromCache, remoteURL: artifact.RemoteURL}, nil
	}

	// 内存缓存和 MetadataStore 均未命中，需要回源
	metrics.RecordCacheMiss(n.RepositoryID, n.Format)

	logrus.WithFields(logrus.Fields{
		"key":           key.String(),
		"remoteBaseURL": n.RemoteBaseURL,
	}).Debug("proxy: GetArtifact cache miss, fetching from remote")

	key.RemoteURL = n.buildRemoteURL(key)
	metadata, err := n.RemoteClient.FetchMetadata(ctx, key)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"key":       key.String(),
			"remoteURL": key.RemoteURL,
			"duration":  time.Since(start).Seconds(),
			"error":     err.Error(),
		}).Error("proxy: GetArtifact fetch metadata failed")
		return getArtifactResult{}, err
	}
	if !metadata.Exists {
		n.setNegativeCache(key)
		logrus.WithFields(logrus.Fields{
			"key":       key.String(),
			"remoteURL": key.RemoteURL,
			"duration":  time.Since(start).Seconds(),
		}).Debug("proxy: GetArtifact remote not found, set negative cache")
		return getArtifactResult{}, ErrNotFound
	}

	now := time.Now()
	artifact = &Artifact{
		RepositoryID: n.RepositoryID,
		Format:       key.Format,
		Kind:         KindArtifact,
		Name:         key.Name,
		Namespace:    key.Namespace,
		Version:      key.Version,
		Path:         key.Path,
		Filename:     key.Filename,
		RemotePath:   key.RemotePath,
		Qualifiers:   cloneStringMap(key.Qualifiers),
		Properties: map[string]string{
			"remote_digest": metadata.Digest,
			"remote_size":   strconv.FormatInt(metadata.Size, 10),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if ip := ClientIPFromContext(ctx); ip != "" {
		artifact.Properties["trigger_ip"] = ip
	}

	if err := n.MetadataStore.Put(ctx, artifact); err != nil {
		logrus.WithFields(logrus.Fields{
			"key":   key.String(),
			"error": err.Error(),
		}).Error("proxy: GetArtifact store metadata failed")
		return getArtifactResult{}, err
	}

	if ensureErr := n.ensureArtifactBlob(ctx, artifact, key); ensureErr != nil {
		return getArtifactResult{}, ensureErr
	}
	n.setCachedArtifact(key, artifact)
	logrus.WithFields(logrus.Fields{
		"key":       key.String(),
		"remoteURL": key.RemoteURL,
		"duration":  time.Since(start).Seconds(),
	}).Debug("proxy: GetArtifact fetch from remote success")
	if len(artifact.BlobRefs) > 0 {
		artifact.SizeBytes = artifact.BlobRefs[0].Size
	}
	return getArtifactResult{artifact: artifact, fromCache: false, remoteURL: key.RemoteURL}, nil
}

func (n *ProxyRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	if n.Blocker != nil {
		name := query.Name
		version := query.Version
		if name != "" && n.Blocker.IsBlocked(n.Format, name, version) {
			return nil, ErrBlocked
		}
	}
	query.RepositoryID = n.RepositoryID

	logrus.WithFields(logrus.Fields{
		"repositoryID":  n.RepositoryID,
		"remoteBaseURL": n.RemoteBaseURL,
		"remotePath":    query.RemotePath,
		"format":        query.Format,
		"hasFetcher":    n.Fetcher != nil,
	}).Debug("proxy: QueryArtifacts called")

	artifacts, err := n.MetadataStore.Query(ctx, query)
	if err != nil {
		logrus.WithError(err).Error("proxy: MetadataStore.Query failed")
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"cachedCount": len(artifacts),
	}).Debug("proxy: local cache query result")

	// 检查缓存是否过期，过期则异步刷新（stale-while-revalidate）
	if len(artifacts) > 0 && n.CachePolicy.MetadataTTL > 0 {
		oldest := artifacts[0].UpdatedAt
		for _, a := range artifacts {
			if a.UpdatedAt.Before(oldest) {
				oldest = a.UpdatedAt
			}
		}
		if time.Since(oldest) > n.CachePolicy.MetadataTTL {
			if n.Fetcher != nil && n.RemoteBaseURL != "" {
				if n.tryRefreshPath(query.RemotePath) {
					go func() {
						defer n.doneRefreshPath(query.RemotePath)
						fetchStart := time.Now()
						fetched, fetchErr := n.Fetcher.FetchRemote(context.Background(), n.RemoteBaseURL, query.RemotePath)
						fetchDuration := time.Since(fetchStart).Seconds()
						if fetchErr == nil && len(fetched) > 0 {
							metrics.RecordProxyFetch(n.Format, "success", fetchDuration)
							oldMap := buildArtifactMap(artifacts)
							toUpdate := n.prepareArtifactsForUpdate(context.Background(), fetched, oldMap)
							if len(toUpdate) > 0 {
								if err := n.MetadataStore.BatchPut(context.Background(), toUpdate); err != nil {
									logrus.WithFields(logrus.Fields{
										"remoteBaseURL": n.RemoteBaseURL,
										"remotePath":    query.RemotePath,
										"error":         err.Error(),
									}).Warn("QueryArtifacts: background BatchPut failed")
								}
								// 异步刷新后清除负缓存，使后续 GetArtifact 能命中新记录。
								for _, a := range toUpdate {
									n.clearNegativeCacheForArtifact(a)
								}
							}
						} else if fetchErr != nil {
							metrics.RecordProxyFetch(n.Format, "error", fetchDuration)
							logrus.WithError(fetchErr).Warn("QueryArtifacts: background refresh failed")
						}
					}()
				}
				metrics.RecordProxyStaleServed(n.Format)
				return artifacts, nil
			}
		}
	}

	if len(artifacts) > 0 {
		// 如果缓存中只有单文件下载产生的 artifact 记录（Kind 为 "artifact" 或空），
		// 没有 metadata 级别的记录（如 "version"、"package-file" 等），
		// 说明缓存不完整，应该回源获取完整的 metadata。
		hasMetadataArtifacts := false
		for _, a := range artifacts {
			if a.Kind != "" && a.Kind != "artifact" {
				hasMetadataArtifacts = true
				break
			}
		}
		if hasMetadataArtifacts {
			return artifacts, nil
		}
		// 缓存不完整，继续走回源逻辑
		logrus.WithFields(logrus.Fields{
			"cachedCount": len(artifacts),
			"remotePath":  query.RemotePath,
		}).Debug("proxy: cache has only artifact records, fetching from remote for complete metadata")
	}
	// 本地缓存为空,通过 RemoteFetcher 回源
	if n.Fetcher != nil && n.RemoteBaseURL != "" {
		logrus.WithFields(logrus.Fields{
			"remoteBaseURL": n.RemoteBaseURL,
			"remotePath":    query.RemotePath,
		}).Debug("proxy: local cache empty, fetching from remote")

		fetchStart := time.Now()
		sgKey := n.RepositoryID + ":" + query.RemotePath
		sgResult, sgErr, _ := n.fetchGroup.Do(sgKey, func() (interface{}, error) {
			return n.Fetcher.FetchRemote(ctx, n.RemoteBaseURL, query.RemotePath)
		})
		fetched, fetchErr := sgResult.([]*Artifact), sgErr
		fetchDuration := time.Since(fetchStart).Seconds()
		if fetchErr != nil {
			metrics.RecordProxyFetch(n.Format, "error", fetchDuration)
			logrus.WithFields(logrus.Fields{
				"remoteBaseURL": n.RemoteBaseURL,
				"remotePath":    query.RemotePath,
				"error":         fetchErr.Error(),
			}).Error("proxy: FetchRemote failed")
			if len(artifacts) > 0 {
				logrus.WithFields(logrus.Fields{
					"cachedCount": len(artifacts),
					"remotePath":  query.RemotePath,
				}).Warn("proxy: serving cached artifacts after FetchRemote failure")
				return artifacts, nil
			}
			return nil, fetchErr
		}
		metrics.RecordProxyFetch(n.Format, "success", fetchDuration)
		logrus.WithFields(logrus.Fields{
			"fetchedCount": len(fetched),
		}).Debug("proxy: FetchRemote success")
		// 使用 BatchPut 批量缓存回源结果
		for _, a := range fetched {
			a.RepositoryID = n.RepositoryID
			n.stampTriggerIP(ctx, a)
		}
		if err := n.MetadataStore.BatchPut(ctx, fetched); err != nil {
			logrus.WithFields(logrus.Fields{
				"remoteBaseURL": n.RemoteBaseURL,
				"remotePath":    query.RemotePath,
				"error":         err.Error(),
			}).Error("proxy: BatchPut fetched artifacts failed")
			return nil, err
		}
		// FetchRemote 成功后，清除已缓存 artifacts 对应的负缓存，
		// 使后续 GetArtifact 能命中 store 中的新记录。
		for _, a := range fetched {
			n.clearNegativeCacheForArtifact(a)
		}
		return fetched, nil
	}
	logrus.WithFields(logrus.Fields{
		"hasFetcher":    n.Fetcher != nil,
		"remoteBaseURL": n.RemoteBaseURL,
	}).Warn("proxy: no fetcher or remote URL, returning empty result")
	return artifacts, nil
}

func (n *ProxyRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
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

func (n *ProxyRuntime) ensureArtifactBlob(ctx context.Context, artifact *Artifact, key ArtifactKey) error {
	start := time.Now()
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

	key.RemoteURL = n.artifactRemoteURL(artifact, key)
	if key.RemoteURL == "" {
		return ErrNotFound
	}

	logrus.WithFields(logrus.Fields{
		"repositoryID": n.RepositoryID,
		"remoteURL":    key.RemoteURL,
		"filename":     key.Filename,
	}).Debug("proxy: ensureArtifactBlob fetching from remote")

	blobReader, err := n.RemoteClient.FetchBlob(ctx, key)
	if err != nil {
		// blob 不存在（上游 404）→ 清理过期 metadata，设置负缓存，返回 ErrNotFound
		// 这样 Go CLI 收到 404 而非 500，不会无限重试
		if errors.Is(err, ErrNotFound) {
			_ = n.MetadataStore.Delete(ctx, key)
			n.setNegativeCache(key)
			logrus.WithFields(logrus.Fields{
				"remoteURL": key.RemoteURL,
				"duration":  time.Since(start).Seconds(),
			}).Debug("proxy: ensureArtifactBlob blob not found, set negative cache")
			return ErrNotFound
		}
		logrus.WithFields(logrus.Fields{
			"remoteURL": key.RemoteURL,
			"duration":  time.Since(start).Seconds(),
			"error":     err.Error(),
		}).Error("proxy: ensureArtifactBlob fetch blob failed")
		return err
	}
	defer blobReader.Close()

	blobReaderForStore := io.Reader(blobReader)
	var limitedCounter *proxyCountingReader
	if n.CachePolicy.MaxBlobSize > 0 {
		limitedCounter = &proxyCountingReader{reader: io.LimitReader(blobReader, n.CachePolicy.MaxBlobSize+1)}
		blobReaderForStore = limitedCounter
	}

	var blobRef BlobRef
	if store, ok := n.BlobStore.(ContextBlobPutter); ok {
		blobRef, err = store.PutContext(ctx, blobReaderForStore)
	} else {
		blobRef, err = n.BlobStore.Put(blobReaderForStore)
	}
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"remoteURL": key.RemoteURL,
			"duration":  time.Since(start).Seconds(),
			"error":     err.Error(),
		}).Error("proxy: ensureArtifactBlob store blob failed")
		return err
	}
	readSize := blobRef.Size
	if limitedCounter != nil {
		readSize = limitedCounter.n
	}
	if n.CachePolicy.MaxBlobSize > 0 && readSize > n.CachePolicy.MaxBlobSize {
		if store, ok := n.BlobStore.(ContextBlobDeleter); ok {
			_ = store.DeleteContext(ctx, blobRef)
		} else {
			_ = n.BlobStore.Delete(blobRef)
		}
		logrus.WithFields(logrus.Fields{
			"remoteURL": key.RemoteURL,
			"size":      readSize,
			"maxSize":   n.CachePolicy.MaxBlobSize,
		}).Warn("proxy: blob too large, rejecting")
		return fmt.Errorf("blob too large: %d bytes exceeds limit %d", readSize, n.CachePolicy.MaxBlobSize)
	}
	artifact.BlobRefs = []BlobRef{blobRef}
	artifact.RemoteURL = key.RemoteURL
	artifact.SizeBytes = blobRef.Size
	if blobRef.Size > 0 {
		metrics.RecordProxyBlobStored(n.Format, blobRef.Size)
	}
	if err := n.MetadataStore.Put(ctx, artifact); err != nil {
		logrus.WithFields(logrus.Fields{
			"remoteURL": key.RemoteURL,
			"duration":  time.Since(start).Seconds(),
			"error":     err.Error(),
		}).Error("proxy: ensureArtifactBlob update metadata failed")
		return err
	}

	logrus.WithFields(logrus.Fields{
		"remoteURL": key.RemoteURL,
		"duration":  time.Since(start).Seconds(),
	}).Debug("proxy: ensureArtifactBlob success")
	return nil
}

func (n *ProxyRuntime) openArtifactContent(ctx context.Context, artifact *Artifact) error {
	if len(artifact.BlobRefs) == 0 {
		return nil
	}
	var (
		rc  io.ReadCloser
		err error
	)
	if store, ok := n.BlobStore.(ContextBlobOpener); ok {
		rc, err = store.OpenContext(ctx, artifact.BlobRefs[0])
	} else {
		rc, err = n.BlobStore.Open(artifact.BlobRefs[0])
	}
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

	key.RemoteURL = n.artifactRemoteURL(artifact, key)
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
	if ip := ClientIPFromContext(ctx); ip != "" {
		artifact.Properties["trigger_ip"] = ip
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
	// 超过上限时淘汰最老的 25% 条目，避免全量清空引发缓存击穿
	if len(n.metadataCache) >= maxMetadataCacheSize {
		n.evictOldestEntries(maxMetadataCacheSize / 4)
	}
	n.metadataCache[key.String()] = cachedArtifact{artifact: cloneArtifactForCache(artifact), expiresAt: time.Now().Add(n.CachePolicy.MetadataTTL)}
	n.metadataCacheMu.Unlock()
}

func cloneArtifactForCache(artifact *Artifact) *Artifact {
	clone := cloneArtifactForResponse(artifact)
	clone.Content = nil
	clone.FromCache = false
	clone.RemoteURL = ""
	return clone
}

func cloneArtifactForResponse(artifact *Artifact) *Artifact {
	if artifact == nil {
		return nil
	}
	clone := *artifact
	clone.Properties = cloneStringMap(artifact.Properties)
	clone.Checksums = cloneStringMap(artifact.Checksums)
	clone.Qualifiers = cloneStringMap(artifact.Qualifiers)
	clone.Attributes = cloneStringMap(artifact.Attributes)
	if len(artifact.Relations) > 0 {
		clone.Relations = append([]ArtifactRelation(nil), artifact.Relations...)
	}
	if len(artifact.BlobRefs) > 0 {
		clone.BlobRefs = append([]BlobRef(nil), artifact.BlobRefs...)
	}
	clone.Content = nil
	return &clone
}

func (n *ProxyRuntime) invalidateCachedArtifact(key ArtifactKey) {
	n.metadataCacheMu.Lock()
	delete(n.metadataCache, key.String())
	n.metadataCacheMu.Unlock()
}

// evictOldestEntries 淘汰 count 个条目，优先删除已过期条目。
// 调用方必须持有 metadataCacheMu 写锁。
func (n *ProxyRuntime) evictOldestEntries(count int) {
	if count <= 0 || len(n.metadataCache) == 0 {
		return
	}

	now := time.Now()
	deleted := 0
	for key, entry := range n.metadataCache {
		if deleted >= count {
			return
		}
		if now.After(entry.expiresAt) {
			delete(n.metadataCache, key)
			deleted++
		}
	}
	for key := range n.metadataCache {
		if deleted >= count {
			return
		}
		delete(n.metadataCache, key)
		deleted++
	}
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

func (n *ProxyRuntime) tryRefreshPath(remotePath string) bool {
	n.refreshingMu.Lock()
	defer n.refreshingMu.Unlock()
	if n.refreshingPaths == nil {
		n.refreshingPaths = map[string]struct{}{}
	}
	if _, ok := n.refreshingPaths[remotePath]; ok {
		return false
	}
	n.refreshingPaths[remotePath] = struct{}{}
	return true
}

func (n *ProxyRuntime) doneRefreshPath(remotePath string) {
	n.refreshingMu.Lock()
	defer n.refreshingMu.Unlock()
	delete(n.refreshingPaths, remotePath)
}

func (n *ProxyRuntime) setNegativeCache(key ArtifactKey) {
	if n.CachePolicy.NegativeTTL <= 0 {
		return
	}
	n.metadataCacheMu.Lock()
	if n.negativeCache == nil {
		n.negativeCache = map[string]time.Time{}
	}
	if len(n.negativeCache) >= maxNegativeCacheSize {
		n.evictNegativeCache(maxNegativeCacheSize / 4)
	}
	n.negativeCache[key.String()] = time.Now().Add(n.CachePolicy.NegativeTTL)
	n.metadataCacheMu.Unlock()
}

func (n *ProxyRuntime) evictNegativeCache(count int) {
	oldest := make([]string, 0, count)
	for k, v := range n.negativeCache {
		if len(oldest) < count {
			oldest = append(oldest, k)
			continue
		}
		for i, o := range oldest {
			if v.Before(n.negativeCache[o]) {
				oldest[i] = k
				break
			}
		}
	}
	for _, k := range oldest {
		delete(n.negativeCache, k)
	}
}

// clearNegativeCacheForArtifact 根据 artifact 的各个可能的查询 key 清除负缓存。
// FetchRemote 回源成功后调用，确保后续 GetArtifact 能命中 store 中的新记录。
func (n *ProxyRuntime) clearNegativeCacheForArtifact(a *Artifact) {
	if n.CachePolicy.NegativeTTL <= 0 {
		return
	}
	keys := n.buildNegativeCacheKeys(a)
	n.metadataCacheMu.Lock()
	for _, k := range keys {
		delete(n.negativeCache, k)
	}
	n.metadataCacheMu.Unlock()
}

// buildNegativeCacheKeys 生成 artifact 可能被查询到的所有 key 字符串。
// 必须覆盖 GetArtifact 调用时 ArtifactKey.String() 生成的所有可能组合，
// 否则负缓存条目无法被清除，导致后续请求持续返回 404。
func (n *ProxyRuntime) buildNegativeCacheKeys(a *Artifact) []string {
	base := ArtifactKey{
		RepositoryID: n.RepositoryID,
		Format:       a.Format,
	}
	var keys []string
	// 完整 key：包含所有字段，匹配 GetArtifact 传入的 key.String()
	// 这是清除负缓存最关键的一条，因为 GetArtifact 的 key 通常携带所有字段
	if a.Name != "" || a.Version != "" || a.Path != "" || a.Filename != "" || a.RemotePath != "" {
		k := base
		k.Name = a.Name
		k.Version = a.Version
		k.Path = a.Path
		k.Filename = a.Filename
		k.RemotePath = a.RemotePath
		keys = append(keys, k.String())
	}
	// 按 remote_path 查询
	if a.RemotePath != "" {
		k := base
		k.RemotePath = a.RemotePath
		keys = append(keys, k.String())
	}
	// 按 name/version 查询
	if a.Name != "" {
		k := base
		k.Name = a.Name
		k.Version = a.Version
		keys = append(keys, k.String())
	}
	// 按 name/path/filename 查询
	if a.Name != "" && a.Path != "" && a.Filename != "" {
		k := base
		k.Name = a.Name
		k.Path = a.Path
		k.Filename = a.Filename
		keys = append(keys, k.String())
	}
	return keys
}

func (n *ProxyRuntime) DeleteArtifact(ctx context.Context, key ArtifactKey) error {
	key.RepositoryID = n.RepositoryID
	return ErrReadOnly
}

// stampTriggerIP sets the "trigger_ip" property on the artifact based on the context.
func (n *ProxyRuntime) stampTriggerIP(ctx context.Context, artifact *Artifact) {
	if ip := ClientIPFromContext(ctx); ip != "" {
		if artifact.Properties == nil {
			artifact.Properties = map[string]string{}
		}
		artifact.Properties["trigger_ip"] = ip
	}
}

func (n *ProxyRuntime) buildRemoteURL(key ArtifactKey) string {
	if key.RemoteURL != "" {
		return key.RemoteURL
	}
	if n.RemoteBaseURL == "" {
		return ""
	}
	base := strings.TrimRight(n.RemoteBaseURL, "/")
	if key.RemotePath != "" {
		return base + "/" + strings.TrimLeft(key.RemotePath, "/")
	}
	p := strings.TrimLeft(key.Path, "/")
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

func (n *ProxyRuntime) artifactRemoteURL(artifact *Artifact, key ArtifactKey) string {
	if key.RemoteURL != "" {
		return key.RemoteURL
	}
	if artifact != nil {
		if artifact.DownloadURL != "" {
			return artifact.DownloadURL
		}
		remotePath := artifact.RemotePath
		if remotePath != "" && n.RemoteBaseURL != "" {
			return strings.TrimRight(n.RemoteBaseURL, "/") + "/" + strings.TrimLeft(remotePath, "/")
		}
	}
	return n.buildRemoteURL(key)
}

func artifactMapKey(a *Artifact) string {
	return a.Format + "/" + a.IdentityKey
}

func buildArtifactMap(artifacts []*Artifact) map[string]*Artifact {
	m := make(map[string]*Artifact, len(artifacts))
	for _, a := range artifacts {
		m[artifactMapKey(a)] = a
	}
	return m
}

func mergeArtifactProperties(newArt *Artifact, oldMap map[string]*Artifact) {
	old := oldMap[artifactMapKey(newArt)]
	if old == nil || len(old.Properties) == 0 {
		return
	}
	if newArt.Properties == nil {
		newArt.Properties = make(map[string]string, len(old.Properties))
	}
	for k, v := range old.Properties {
		if newArt.Properties[k] == "" {
			newArt.Properties[k] = v
		}
	}
}

// prepareArtifactsForUpdate 准备需要更新的 artifact 列表（增量更新）
// 只返回新增或变更的 artifact，跳过未变更的
func (n *ProxyRuntime) prepareArtifactsForUpdate(ctx context.Context, fetched []*Artifact, oldMap map[string]*Artifact) []*Artifact {
	var toUpdate []*Artifact
	for _, a := range fetched {
		a.RepositoryID = n.RepositoryID
		n.stampTriggerIP(ctx, a)

		key := artifactMapKey(a)
		old := oldMap[key]
		if old == nil {
			// 新增的 artifact
			toUpdate = append(toUpdate, a)
			continue
		}

		// 比较是否变更
		if hasArtifactChanged(old, a) {
			mergeArtifactProperties(a, map[string]*Artifact{key: old})
			toUpdate = append(toUpdate, a)
		}
	}
	return toUpdate
}

// hasArtifactChanged 检查 artifact 是否有变更（基于 remote_digest 和 remote_size）
func hasArtifactChanged(old, new *Artifact) bool {
	if old.Properties == nil || new.Properties == nil {
		return true
	}

	oldDigest := old.Properties["remote_digest"]
	newDigest := new.Properties["remote_digest"]
	if oldDigest != "" && newDigest != "" && oldDigest != newDigest {
		return true
	}

	oldSize := old.Properties["remote_size"]
	newSize := new.Properties["remote_size"]
	if oldSize != "" && newSize != "" && oldSize != newSize {
		return true
	}

	// 如果有 digest 或 size 信息且匹配，认为没有变更
	if oldDigest != "" && newDigest != "" && oldDigest == newDigest {
		return false
	}
	if oldSize != "" && newSize != "" && oldSize == newSize {
		return false
	}

	// 没有可靠的信息可以比较，保守地认为有变更
	return true
}
