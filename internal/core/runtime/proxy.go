package runtime

import (
	"context"
	"errors"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dshmyz/moonlight-box/internal/metrics"
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
	refreshing      int32 // 原子标记，防止并发刷新
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
	start := time.Now()
	if err := n.checkBlocked(key); err != nil {
		return nil, err
	}
	// 统一使用带 RepositoryID 的 key
	key.RepositoryID = n.RepositoryID

	logrus.WithFields(logrus.Fields{
		"repositoryID": n.RepositoryID,
		"format":       key.Format,
		"coordinates":  key.Coordinates,
		"filename":     key.Filename,
	}).Debug("proxy: GetArtifact called")

	if n.isNegativeCached(key) {
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
		if err := n.openArtifactContent(artifact); err != nil {
			return nil, err
		}
		artifact.FromCache = true
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
		logrus.WithFields(logrus.Fields{
			"key":      key.String(),
			"duration": time.Since(start).Seconds(),
		}).Debug("proxy: GetArtifact metadata store hit")
		artifact.FromCache = true
		return artifact, nil
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
		return nil, err
	}
	if !metadata.Exists {
		n.setNegativeCache(key)
		logrus.WithFields(logrus.Fields{
			"key":       key.String(),
			"remoteURL": key.RemoteURL,
			"duration":  time.Since(start).Seconds(),
		}).Debug("proxy: GetArtifact remote not found, set negative cache")
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
	if ip := ClientIPFromContext(ctx); ip != "" {
		artifact.Properties["trigger_ip"] = ip
	}

	if err := n.MetadataStore.Put(ctx, artifact); err != nil {
		logrus.WithFields(logrus.Fields{
			"key":   key.String(),
			"error": err.Error(),
		}).Error("proxy: GetArtifact store metadata failed")
		return nil, err
	}

	if ensureErr := n.ensureArtifactBlob(ctx, artifact, key); ensureErr != nil {
		return nil, ensureErr
	}
	if err := n.openArtifactContent(artifact); err != nil {
		return nil, err
	}
	n.setCachedArtifact(key, artifact)
	logrus.WithFields(logrus.Fields{
		"key":       key.String(),
		"remoteURL": key.RemoteURL,
		"duration":  time.Since(start).Seconds(),
	}).Debug("proxy: GetArtifact fetch from remote success")
	artifact.FromCache = false
	artifact.RemoteURL = key.RemoteURL
	if len(artifact.BlobRefs) > 0 {
		artifact.SizeBytes = artifact.BlobRefs[0].Size
	}
	return artifact, nil
}

func (n *ProxyRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	if n.Blocker != nil {
		if name := query.Coordinates["name"]; name != "" && n.Blocker.IsBlocked(n.Format, name, query.Coordinates["version"]) {
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
				if atomic.CompareAndSwapInt32(&n.refreshing, 0, 1) {
					go func() {
						defer atomic.StoreInt32(&n.refreshing, 0)
						fetchStart := time.Now()
						fetched, fetchErr := n.Fetcher.FetchRemote(context.Background(), n.RemoteBaseURL, query.RemotePath)
						fetchDuration := time.Since(fetchStart).Seconds()
						if fetchErr == nil && len(fetched) > 0 {
							metrics.RecordProxyFetch(n.Format, "success", fetchDuration)
							oldMap := buildArtifactMap(artifacts)
							toUpdate := n.prepareArtifactsForUpdate(context.Background(), fetched, oldMap)
							if len(toUpdate) > 0 {
								_ = n.MetadataStore.BatchPut(context.Background(), toUpdate)
							}
						} else if fetchErr != nil {
							metrics.RecordProxyFetch(n.Format, "error", fetchDuration)
							logrus.WithError(fetchErr).Warn("QueryArtifacts: background refresh failed")
						}
					}()
				}
				return artifacts, nil
			}
		}
	}

	if len(artifacts) > 0 {
		return artifacts, nil
	}
	// 本地缓存为空,通过 RemoteFetcher 回源
	if n.Fetcher != nil && n.RemoteBaseURL != "" {
		logrus.WithFields(logrus.Fields{
			"remoteBaseURL": n.RemoteBaseURL,
			"remotePath":    query.RemotePath,
		}).Debug("proxy: local cache empty, fetching from remote")

		fetchStart := time.Now()
		fetched, fetchErr := n.Fetcher.FetchRemote(ctx, n.RemoteBaseURL, query.RemotePath)
		fetchDuration := time.Since(fetchStart).Seconds()
		if fetchErr != nil {
			metrics.RecordProxyFetch(n.Format, "error", fetchDuration)
			logrus.WithFields(logrus.Fields{
				"remoteBaseURL": n.RemoteBaseURL,
				"remotePath":    query.RemotePath,
				"error":         fetchErr.Error(),
			}).Error("proxy: FetchRemote failed")
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
		_ = n.MetadataStore.BatchPut(ctx, fetched)
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

	key.RemoteURL = n.buildRemoteURL(key)
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

	blobRef, err := n.BlobStore.Put(blobReader)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"remoteURL": key.RemoteURL,
			"duration":  time.Since(start).Seconds(),
			"error":     err.Error(),
		}).Error("proxy: ensureArtifactBlob store blob failed")
		return err
	}
	artifact.BlobRefs = []BlobRef{blobRef}
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
	n.metadataCache[key.String()] = cachedArtifact{artifact: artifact, expiresAt: time.Now().Add(n.CachePolicy.MetadataTTL)}
	n.metadataCacheMu.Unlock()
}

func (n *ProxyRuntime) invalidateCachedArtifact(key ArtifactKey) {
	n.metadataCacheMu.Lock()
	delete(n.metadataCache, key.String())
	n.metadataCacheMu.Unlock()
}

// evictOldestEntries 淘汰 expiresAt 最早的 count 个条目。
// 调用方必须持有 metadataCacheMu 写锁。
func (n *ProxyRuntime) evictOldestEntries(count int) {
	if count <= 0 || len(n.metadataCache) == 0 {
		return
	}
	type kv struct {
		key       string
		expiresAt time.Time
	}
	entries := make([]kv, 0, len(n.metadataCache))
	for k, v := range n.metadataCache {
		entries = append(entries, kv{key: k, expiresAt: v.expiresAt})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].expiresAt.Before(entries[j].expiresAt)
	})
	if count > len(entries) {
		count = len(entries)
	}
	for i := 0; i < count; i++ {
		delete(n.metadataCache, entries[i].key)
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

func artifactMapKey(a *Artifact) string {
	keys := make([]string, 0, len(a.Coordinates))
	for k := range a.Coordinates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(a.Coordinates[k])
		b.WriteByte(';')
	}
	return b.String()
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
