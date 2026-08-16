package runtime

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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
const maxMetadataFailures = 5000   // 元数据失败缓存上限，防止 DoS

// backgroundRefreshTimeout 限制异步刷新的最长执行时间，避免 FetchRemote 挂起导致
// refreshingPaths 永不释放、该路径后续刷新全部失效以及 goroutine 泄漏。
const backgroundRefreshTimeout = 30 * time.Second

// defaultUpstreamTimeout 是回源请求（FetchRemote/FetchMetadata/FetchBlob）的兜底超时。
// 与 cmd/registry 全局默认（pluginHTTPClient.Timeout，默认 60s）对齐，抑制上游 accept
// 后不回包导致的长时间挂起。
const defaultUpstreamTimeout = 60 * time.Second

type ProxyRuntime struct {
	MetadataStore        MetadataStore
	BlobStore            BlobStore
	RemoteClient         RemoteClient
	RepositoryID         string
	RemoteBaseURL        string
	CachePolicy          CachePolicy
	Fetcher              RemoteFetcher  // 由 Plugin 实现，Runtime 控制回源时机
	Blocker              PackageBlocker // 阻断规则检查
	Format               string         // 仓库协议类型，供阻断检查使用
	ConditionAudit       ConditionAuditLogger
	MetadataFetchTimeout time.Duration
	MetadataFailureTTL   time.Duration
	// UpstreamTimeout 是回源请求的固定超时（仓库级覆盖，未配置时为 0，回退 defaultUpstreamTimeout）。
	// 回源不再透传任意调用者的 ctx，统一用带此 deadline 的背景 ctx，保证共享回源不被单个
	// 客户端断开而取消。
	UpstreamTimeout time.Duration

	metadataCacheMu   sync.RWMutex
	metadataCache     map[string]*list.Element // LRU: 值为 *cachedArtifactEntry，链表队首为最久未访问
	metadataCacheLL   *list.List
	negativeCache     map[string]time.Time
	fetchGroup        singleflight.Group
	refreshingMu      sync.Mutex
	refreshingPaths   map[string]struct{}
	metadataFailureMu sync.Mutex
	metadataFailures  map[string]time.Time
}

// upstreamTimeout 返回回源请求用的固定超时（仓库级 UpstreamTimeout，未配置时回退默认值）。
// 回源（FetchRemote/FetchMetadata/FetchBlob）只在 leader 的共享回源内使用带此 deadline 的
// background ctx（见 doFlight），不再透传任意调用者的 ctx，避免共享回源的存亡耦合到最先到达
// 的那个客户端（其断连把等待同一 key 的所有人一起取消）。
func (n *ProxyRuntime) upstreamTimeout() time.Duration {
	to := n.UpstreamTimeout
	if to <= 0 {
		to = defaultUpstreamTimeout
	}
	return to
}

// flightResult[T] 是 doFlight 在 singleflight 内的统一结果载体。
type flightResult[T any] struct {
	v   T
	err error
}

// doFlight 是 ctx-aware 的 singleflight 封装。
//
// 解决了标准库 singleflight.Group.Do 的两个问题：
//   - 等待方 `call.Wait()` 不检查自身 ctx，会被动阻塞在慢 leader 上；
//   - leader 被调用方的 ctx 取消时会把共享回源一起取消，连坐等待同一 key 的所有人。
//
// 回源上下文由 leader 在 g.Do 闭包内创建（background + 固定超时），生命周期等于共享回源本身，
// 不依赖任何调用方 ctx：首个调用方断连只让它自己立即返回 ctx.Err()，正在进行的回源继续完成，
// 等待方仍能拿到结果、缓存仍会被填充。timeout 由调用方传入（见 upstreamTimeout / MetadataFetchTimeout）。
// 由于结果通道带缓冲，等待方提前退出不会阻塞内部 goroutine。
func doFlight[T any](ctx context.Context, g *singleflight.Group, key string, timeout time.Duration, run func(context.Context) (T, error)) (T, error) {
	var zero T
	resCh := make(chan flightResult[T], 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resCh <- flightResult[T]{err: fmt.Errorf("panic in doFlight: %v", r)}
			}
		}()
		raw, _, _ := g.Do(key, func() (interface{}, error) {
			runCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			v, err := run(runCtx)
			return flightResult[T]{v: v, err: err}, nil
		})
		r, _ := raw.(flightResult[T])
		resCh <- r
	}()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case r := <-resCh:
		return r.v, r.err
	}
}

var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"TE":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func (n *ProxyRuntime) OpenRemote(ctx context.Context, request RemoteOpenRequest) (*RemoteResponse, error) {
	method := strings.ToUpper(request.Method)
	if method != http.MethodGet && method != http.MethodHead {
		return nil, ErrRemoteUnsupported
	}
	if strings.TrimSpace(n.RemoteBaseURL) == "" {
		return nil, ErrRemoteUnsupported
	}
	if n.RemoteClient == nil {
		return nil, NewUpstreamUnavailableError("", fmt.Errorf("remote client is not configured"))
	}

	headers := make(http.Header)
	for _, name := range []string{"Accept", "If-None-Match", "If-Modified-Since"} {
		if values := request.Headers.Values(name); len(values) > 0 {
			headers[name] = append([]string(nil), values...)
		}
	}
	headers.Set("User-Agent", "Moonlight-Registry/1.0")
	remoteURL := strings.TrimRight(n.RemoteBaseURL, "/") + "/" + strings.TrimLeft(request.Path, "/")
	start := time.Now()
	response, err := n.RemoteClient.Open(ctx, RemoteRequest{URL: remoteURL, Method: method, Headers: headers})
	if err != nil {
		metrics.RecordProxyFetch(n.Format, "error", time.Since(start).Seconds())
		return nil, NewUpstreamUnavailableError(remoteURL, err)
	}
	if response == nil {
		metrics.RecordProxyFetch(n.Format, "error", time.Since(start).Seconds())
		return nil, NewUpstreamUnavailableError(remoteURL, fmt.Errorf("empty upstream response"))
	}
	metrics.RecordProxyFetch(n.Format, "success", time.Since(start).Seconds())
	response.Header = filterHopByHopHeaders(response.Header)
	return response, nil
}

func filterHopByHopHeaders(headers http.Header) http.Header {
	connectionHeaders := make(map[string]struct{})
	for name, values := range headers {
		if !strings.EqualFold(name, "Connection") {
			continue
		}
		for _, value := range values {
			for _, header := range strings.Split(value, ",") {
				connectionHeaders[strings.ToLower(strings.TrimSpace(header))] = struct{}{}
			}
		}
	}

	filtered := make(http.Header, len(headers))
	for name, values := range headers {
		if isHopByHopHeader(name) {
			continue
		}
		if _, ok := connectionHeaders[strings.ToLower(name)]; ok {
			continue
		}
		for _, value := range values {
			filtered.Add(name, value)
		}
	}
	return filtered
}

func isHopByHopHeader(name string) bool {
	_, ok := hopByHopHeaders[http.CanonicalHeaderKey(name)]
	return ok
}

type cachedArtifact struct {
	artifact  *Artifact
	expiresAt time.Time
	negative  bool
}

// cachedArtifactEntry 是 LRU 链表节点，持有原始 ArtifactKey 以便 O(1) 从 map 删除，
// 也供 WarmUp 在无损耗地重放检索（保留 scoped 名称与 Qualifiers，无需从字符串反向解析）。
type cachedArtifactEntry struct {
	key       ArtifactKey
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
		return NewBlockedError(n.Blocker.BlockReason(n.Format, name, ver))
	}
	return nil
}

// checkBlockedWithAttrs 第二层阻断检查：拿到 artifact 后，用其 Attributes 做条件阻断判断。
// 与第一层（checkBlocked）的区别：第一层只有包名+版本，第二层结合 license/publish_time 等元数据。
func (n *ProxyRuntime) checkBlockedWithAttrs(key ArtifactKey, artifact *Artifact) error {
	if n.Blocker == nil || artifact == nil {
		return nil
	}
	name := key.Name
	ver := key.Version
	if name == "" {
		return nil
	}
	// 把 map[string]string 转成 map[string]interface{}
	attrs := make(map[string]interface{}, len(artifact.Attributes))
	for k, v := range artifact.Attributes {
		attrs[k] = v
	}
	blocked, reason := n.Blocker.IsBlockedWithAttrs(n.Format, name, ver, attrs)
	if blocked {
		return NewBlockedError(reason)
	}
	return nil
}

func (n *ProxyRuntime) evaluateConditionalAccess(ctx context.Context, key ArtifactKey, artifact *Artifact) error {
	conditional, ok := n.Blocker.(ConditionalBlocker)
	if !ok || key.Name == "" {
		return n.checkBlockedWithAttrs(key, artifact)
	}
	requirements := conditional.RequiredAttributes(n.Format, key.Name, key.Version)
	if len(requirements) == 0 {
		return nil
	}
	missing := missingConditionAttributes(artifact, requirements)
	if len(missing) == 0 {
		return n.checkBlockedWithAttrs(key, artifact)
	}

	failureKey := "attrs:" + n.RepositoryID + ":" + n.Format + ":" + key.Name + ":" + key.Version
	if n.metadataFailureCached(failureKey) {
		n.auditConditionUnverified(ctx, key, requirements, missing, "cached_unavailable")
		return nil
	}

	fetcher, ok := n.Fetcher.(ArtifactMetadataFetcher)
	if !ok {
		n.cacheMetadataFailure(failureKey)
		n.auditConditionUnverified(ctx, key, requirements, missing, "unsupported")
		return nil
	}
	timeout := n.MetadataFetchTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	metadata, err := doFlight(ctx, &n.fetchGroup, failureKey, timeout, func(runCtx context.Context) (*ArtifactMetadata, error) {
		return fetcher.FetchArtifactMetadata(runCtx, n.RemoteBaseURL, key)
	})
	if err != nil {
		n.cacheMetadataFailure(failureKey)
		reason := "fetch_failed"
		if errors.Is(err, ErrMetadataUnsupported) {
			reason = "unsupported"
		} else if errors.Is(err, ErrMetadataUnavailable) {
			reason = "unavailable"
		}
		n.auditConditionUnverified(ctx, key, requirements, missing, reason)
		return nil
	}
	if metadata == nil {
		n.cacheMetadataFailure(failureKey)
		n.auditConditionUnverified(ctx, key, requirements, missing, "unavailable")
		return nil
	}
	if artifact.Attributes == nil {
		artifact.Attributes = make(map[string]string)
	}
	for name, value := range metadata.Attributes {
		artifact.Attributes[name] = value
	}
	missing = missingConditionAttributes(artifact, requirements)
	if len(missing) > 0 {
		n.cacheMetadataFailure(failureKey)
		n.auditConditionUnverified(ctx, key, requirements, missing, "unavailable")
		return nil
	}
	return n.checkBlockedWithAttrs(key, artifact)
}

func missingConditionAttributes(artifact *Artifact, requirements []ConditionRequirement) []string {
	seen := make(map[string]struct{})
	missing := make([]string, 0)
	for _, requirement := range requirements {
		if artifact == nil || artifact.Attributes[requirement.Attribute] == "" {
			if _, ok := seen[requirement.Attribute]; !ok {
				seen[requirement.Attribute] = struct{}{}
				missing = append(missing, requirement.Attribute)
			}
		}
	}
	return missing
}

func (n *ProxyRuntime) metadataFailureCached(key string) bool {
	n.metadataFailureMu.Lock()
	defer n.metadataFailureMu.Unlock()
	expiry, ok := n.metadataFailures[key]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(n.metadataFailures, key)
		return false
	}
	return true
}
func (n *ProxyRuntime) cacheMetadataFailure(key string) {
	ttl := n.MetadataFailureTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	n.metadataFailureMu.Lock()
	defer n.metadataFailureMu.Unlock()
	if n.metadataFailures == nil {
		n.metadataFailures = make(map[string]time.Time)
	}
	if len(n.metadataFailures) >= maxMetadataFailures {
		n.evictMetadataFailuresLocked()
	}
	n.metadataFailures[key] = time.Now().Add(ttl)
}

// evictMetadataFailuresLocked 清理失败缓存：先删过期项，仍超限时淘汰至 75%。
// 调用时必须已持有 n.metadataFailureMu。
func (n *ProxyRuntime) evictMetadataFailuresLocked() {
	now := time.Now()
	for k, expiry := range n.metadataFailures {
		if now.After(expiry) {
			delete(n.metadataFailures, k)
		}
	}
	target := maxMetadataFailures * 3 / 4
	for k := range n.metadataFailures {
		if len(n.metadataFailures) <= target {
			break
		}
		delete(n.metadataFailures, k)
	}
}
func (n *ProxyRuntime) auditConditionUnverified(ctx context.Context, key ArtifactKey, requirements []ConditionRequirement, missing []string, reason string) {
	if n.ConditionAudit == nil {
		return
	}
	ids := make([]uint, 0, len(requirements))
	for _, requirement := range requirements {
		ids = append(ids, requirement.RuleID)
	}
	n.ConditionAudit.LogConditionUnverified(ctx, ConditionUnverifiedEntry{RepositoryID: n.RepositoryID, Format: n.Format, Name: key.Name, Version: key.Version, RemotePath: key.RemotePath, RuleIDs: ids, MissingAttributes: missing, Reason: reason})
}

// filterBlockedArtifacts 用条件规则过滤掉被阻断的 artifacts（用于 QueryArtifacts）。
// 与 GetArtifact 不同，QueryArtifacts 是过滤而非报错：返回过滤后的列表。
func (n *ProxyRuntime) filterBlockedArtifacts(artifacts []*Artifact) []*Artifact {
	if n.Blocker == nil || len(artifacts) == 0 {
		return artifacts
	}
	result := make([]*Artifact, 0, len(artifacts))
	for _, a := range artifacts {
		// 没有 Name 的 artifact（如纯文件/目录）不参与条件阻断，直接保留
		if a == nil || a.Name == "" {
			result = append(result, a)
			continue
		}
		attrs := make(map[string]interface{}, len(a.Attributes))
		for k, v := range a.Attributes {
			attrs[k] = v
		}
		blocked, _ := n.Blocker.IsBlockedWithAttrs(n.Format, a.Name, a.Version, attrs)
		if !blocked {
			result = append(result, a)
		}
	}
	return result
}

func (n *ProxyRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	start := time.Now()
	if err := n.checkBlocked(key); err != nil {
		return nil, err
	}
	// 统一使用带 RepositoryID 的 key
	key.RepositoryID = n.RepositoryID

	logrus.WithFields(logrus.Fields{
		"repo_id": n.RepositoryID,
		"format":       key.Format,
		"name":         key.Name,
		"version":      key.Version,
		"remote_path":   key.RemotePath,
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
			"duration_ms": time.Since(start).Seconds(),
		}).Debug("proxy: GetArtifact memory cache hit")
		artifact = cloneArtifactForResponse(artifact)
		if err := n.evaluateConditionalAccess(ctx, key, artifact); err != nil {
			return nil, err
		}
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
	res, err := doFlight(ctx, &n.fetchGroup, sfKey, n.upstreamTimeout(), func(runCtx context.Context) (getArtifactResult, error) {
		if n.isNegativeCached(key) {
			metrics.RecordProxyNegativeCacheHit(n.Format)
			return getArtifactResult{}, ErrNotFound
		}
		if artifact, ok := n.getCachedArtifact(key); ok {
			metrics.RecordCacheHit(n.RepositoryID, n.Format)
			return getArtifactResult{artifact: artifact, fromCache: true}, nil
		}
		return n.loadArtifact(runCtx, key, start)
	})
	if err != nil {
		return nil, err
	}
	artifact := cloneArtifactForResponse(res.artifact)
	if err := n.evaluateConditionalAccess(ctx, key, artifact); err != nil {
		return nil, err
	}
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
		if blockErr := n.evaluateConditionalAccess(ctx, key, artifact); blockErr != nil {
			return getArtifactResult{}, blockErr
		}
		hadBlob := len(artifact.BlobRefs) > 0
		if ensureErr := n.ensureArtifactBlob(ctx, artifact, key); ensureErr != nil {
			// 上游超时/不可达时，如果本地已有旧 blob，先返回旧版本（stale-while-revalidate）。
			// 对包管理器来说，旧版本通常比完全拿不到要好得多。
			if hadBlob {
				logrus.WithFields(logrus.Fields{
					"key":   key.String(),
					"error": ensureErr.Error(),
				}).Warn("proxy: ensureArtifactBlob failed, serving stale blob")
				return getArtifactResult{artifact: artifact, fromCache: true, remoteURL: ""}, nil
			}
			return getArtifactResult{}, ensureErr
		}
		if len(artifact.BlobRefs) > 0 {
			artifact.SizeBytes = artifact.BlobRefs[0].Size
		}
		n.setCachedArtifact(key, artifact)
		logrus.WithFields(logrus.Fields{
			"key":      key.String(),
			"duration_ms": time.Since(start).Seconds(),
		}).Debug("proxy: GetArtifact metadata store hit")
		fromCache := hadBlob && artifact.RemoteURL == ""
		return getArtifactResult{artifact: artifact, fromCache: fromCache, remoteURL: artifact.RemoteURL}, nil
	}

	// 内存缓存和 MetadataStore 均未命中，需要回源
	metrics.RecordCacheMiss(n.RepositoryID, n.Format)

	logrus.WithFields(logrus.Fields{
		"key":           key.String(),
		"remote_base_url": n.RemoteBaseURL,
	}).Debug("proxy: GetArtifact cache miss, fetching from remote")

	key.RemoteURL = n.buildRemoteURL(key)
	metadata, err := n.RemoteClient.FetchMetadata(ctx, key)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"key":       key.String(),
			"remote_url": key.RemoteURL,
			"duration_ms":  time.Since(start).Seconds(),
			"error":     err.Error(),
		}).Error("proxy: GetArtifact fetch metadata failed")
		return getArtifactResult{}, err
	}
	if !metadata.Exists {
		n.setNegativeCache(key)
		logrus.WithFields(logrus.Fields{
			"key":       key.String(),
			"remote_url": key.RemoteURL,
			"duration_ms":  time.Since(start).Seconds(),
		}).Debug("proxy: GetArtifact remote not found, set negative cache")
		return getArtifactResult{}, ErrNotFound
	}

	now := time.Now()
	updatedAt := now
	if !metadata.ModifiedAt.IsZero() {
		updatedAt = metadata.ModifiedAt
	}
	remoteETag := firstNonEmpty(metadata.ETag, metadata.Digest)
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
		UpdatedAt: updatedAt,
	}
	if remoteETag != "" {
		artifact.Properties["remote_etag"] = remoteETag
	}
	if !metadata.ModifiedAt.IsZero() {
		artifact.Properties["remote_last_modified"] = metadata.ModifiedAt.UTC().Format(http.TimeFormat)
	}
	if ip := ClientIPFromContext(ctx); ip != "" {
		artifact.Properties["trigger_ip"] = ip
	}
	if blockErr := n.evaluateConditionalAccess(ctx, key, artifact); blockErr != nil {
		return getArtifactResult{}, blockErr
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
		"remote_url": key.RemoteURL,
		"duration_ms":  time.Since(start).Seconds(),
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
			return nil, NewBlockedError(n.Blocker.BlockReason(n.Format, name, version))
		}
	}
	query.RepositoryID = n.RepositoryID

	logrus.WithFields(logrus.Fields{
		"repo_id":  n.RepositoryID,
		"remote_base_url": n.RemoteBaseURL,
		"remote_path":    query.RemotePath,
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
			if n.Fetcher != nil && n.RemoteBaseURL != "" && query.RemotePath != "" {
				if n.tryRefreshPath(query.RemotePath) {
					go func() {
						defer n.doneRefreshPath(query.RemotePath)
						// 用带超时的独立 context，避免请求结束后父 ctx 取消导致刷新中断；
						// 同时限制超时防止 FetchRemote 挂起泄漏 goroutine 和 refreshingPaths 槽位。
						refreshCtx, cancel := context.WithTimeout(context.Background(), backgroundRefreshTimeout)
						defer cancel()
						// 保留触发请求的 client IP 用于审计，否则背景 ctx 拿不到 IP
						if ip := ClientIPFromContext(ctx); ip != "" {
							refreshCtx = ContextWithClientIP(refreshCtx, ip)
						}
						fetchStart := time.Now()
						fetched, fetchErr := n.Fetcher.FetchRemote(refreshCtx, n.RemoteBaseURL, query.RemotePath)
						fetchDuration := time.Since(fetchStart).Seconds()
						if fetchErr == nil && len(fetched) > 0 {
							metrics.RecordProxyFetch(n.Format, "success", fetchDuration)
							oldMap := buildArtifactMap(artifacts)
							toUpdate := n.prepareArtifactsForUpdate(refreshCtx, fetched, oldMap)
							if len(toUpdate) > 0 {
								if err := n.MetadataStore.BatchPut(refreshCtx, toUpdate); err != nil {
									logrus.WithFields(logrus.Fields{
										"remote_base_url": n.RemoteBaseURL,
										"remote_path":    query.RemotePath,
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
				return n.filterBlockedArtifacts(artifacts), nil
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
			return n.filterBlockedArtifacts(artifacts), nil
		}
		// 缓存不完整，继续走回源逻辑
		logrus.WithFields(logrus.Fields{
			"cachedCount": len(artifacts),
			"remote_path":  query.RemotePath,
		}).Debug("proxy: cache has only artifact records, fetching from remote for complete metadata")
	}
	// 本地缓存为空,通过 RemoteFetcher 回源
	if n.Fetcher != nil && n.RemoteBaseURL != "" && query.RemotePath != "" {
		logrus.WithFields(logrus.Fields{
			"remote_base_url": n.RemoteBaseURL,
			"remote_path":    query.RemotePath,
		}).Debug("proxy: local cache empty, fetching from remote")

		fetchStart := time.Now()
		sgKey := n.RepositoryID + ":" + query.RemotePath
		// 将 FetchRemote + stampTriggerIP + BatchPut + clearNegativeCache 全部放入 singleflight 闭包，
		// 确保对同一 remotePath 的并发请求只执行一次副作用，避免多个 goroutine 并发写同一批
		// artifact 的 Properties map（fatal error: concurrent map writes）。
		type queryResult struct {
			fetched []*Artifact
			err     error
		}
		res, sgErr := doFlight(ctx, &n.fetchGroup, sgKey, n.upstreamTimeout(), func(runCtx context.Context) (queryResult, error) {
			fetched, fetchErr := n.Fetcher.FetchRemote(runCtx, n.RemoteBaseURL, query.RemotePath)
			if fetchErr != nil {
				return queryResult{err: fetchErr}, nil
			}
			for _, a := range fetched {
				a.RepositoryID = n.RepositoryID
				// 保留触发请求的 client IP 用于审计，用调用方 ctx 而非上游上下文
				n.stampTriggerIP(ctx, a)
			}
			if err := n.MetadataStore.BatchPut(runCtx, fetched); err != nil {
				return queryResult{err: err}, nil
			}
			for _, a := range fetched {
				n.clearNegativeCacheForArtifact(a)
			}
			return queryResult{fetched: fetched}, nil
		})
		fetchDuration := time.Since(fetchStart).Seconds()

		fetchErr := sgErr
		if fetchErr == nil && res.err != nil {
			fetchErr = res.err
		}
		fetched := res.fetched

		if fetchErr != nil {
			metrics.RecordProxyFetch(n.Format, "error", fetchDuration)
			logrus.WithFields(logrus.Fields{
				"remote_base_url": n.RemoteBaseURL,
				"remote_path":    query.RemotePath,
				"error":         fetchErr.Error(),
			}).Error("proxy: FetchRemote failed")
			if len(artifacts) > 0 {
				logrus.WithFields(logrus.Fields{
					"cachedCount": len(artifacts),
					"remote_path":  query.RemotePath,
				}).Warn("proxy: serving cached artifacts after FetchRemote failure")
				return n.filterBlockedArtifacts(artifacts), nil
			}
			if IsUpstreamTimeout(fetchErr) {
				return nil, NewUpstreamTimeoutError(
					strings.TrimRight(n.RemoteBaseURL, "/")+"/"+query.RemotePath,
					30, fetchErr,
				)
			}
			return nil, fetchErr
		}
		metrics.RecordProxyFetch(n.Format, "success", fetchDuration)
		logrus.WithFields(logrus.Fields{
			"fetchedCount": len(fetched),
		}).Debug("proxy: FetchRemote success")
		return n.filterBlockedArtifacts(fetched), nil
	}
	logrus.WithFields(logrus.Fields{
		"hasFetcher":    n.Fetcher != nil,
		"remote_base_url": n.RemoteBaseURL,
		"remote_path":    query.RemotePath,
	}).Warn("proxy: no fetcher or remote URL, returning empty result")
	return n.filterBlockedArtifacts(artifacts), nil
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
		"repo_id": n.RepositoryID,
		"remote_url":    key.RemoteURL,
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
				"remote_url": key.RemoteURL,
				"duration_ms":  time.Since(start).Seconds(),
			}).Debug("proxy: ensureArtifactBlob blob not found, set negative cache")
			return ErrNotFound
		}
		if IsUpstreamTimeout(err) {
			logrus.WithFields(logrus.Fields{
				"remote_url": key.RemoteURL,
				"duration_ms":  time.Since(start).Seconds(),
				"error":     err.Error(),
			}).Warn("proxy: ensureArtifactBlob upstream timeout")
			return NewUpstreamTimeoutError(key.RemoteURL, 30, err)
		}
		logrus.WithFields(logrus.Fields{
			"remote_url": key.RemoteURL,
			"duration_ms":  time.Since(start).Seconds(),
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
			"remote_url": key.RemoteURL,
			"duration_ms":  time.Since(start).Seconds(),
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
			"remote_url": key.RemoteURL,
			"size":      readSize,
			"maxSize":   n.CachePolicy.MaxBlobSize,
		}).Warn("proxy: blob too large, rejecting")
		return fmt.Errorf("blob too large: %d bytes exceeds limit %d", readSize, n.CachePolicy.MaxBlobSize)
	}
	artifact.BlobRefs = []BlobRef{blobRef}
	if blobRef.Algorithm == "sha256" && blobRef.Digest != "" {
		if artifact.Checksums == nil {
			artifact.Checksums = map[string]string{}
		}
		artifact.Checksums["sha256"] = blobRef.Digest
	}
	artifact.RemoteURL = key.RemoteURL
	artifact.SizeBytes = blobRef.Size
	if blobRef.Size > 0 {
		metrics.RecordProxyBlobStored(n.Format, blobRef.Size)
	}
	if err := n.MetadataStore.Put(ctx, artifact); err != nil {
		logrus.WithFields(logrus.Fields{
			"remote_url": key.RemoteURL,
			"duration_ms":  time.Since(start).Seconds(),
			"error":     err.Error(),
		}).Error("proxy: ensureArtifactBlob update metadata failed")
		return err
	}

	logrus.WithFields(logrus.Fields{
		"remote_url": key.RemoteURL,
		"duration_ms":  time.Since(start).Seconds(),
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
		// 远端不可达：设置短 TTL 负缓存，避免故障期间每个请求都重复回源放大流量。
		// 不返回错误（stale-while-revalidate 语义），继续服务本地缓存数据。
		// 用 NegativeTTL 的一半作为故障负缓存 TTL，区别于"远端确认不存在"的完整 NegativeTTL。
		failureTTL := n.CachePolicy.NegativeTTL / 2
		if failureTTL < 30*time.Second {
			failureTTL = 30 * time.Second
		}
		n.setNegativeCacheWithTTL(key, failureTTL)
		logrus.WithError(err).Warn("remote unreachable, serving cached artifact (negative cache set)")
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
	if remoteETag := firstNonEmpty(remoteMeta.ETag, remoteMeta.Digest); remoteETag != "" {
		artifact.Properties["remote_etag"] = remoteETag
	}
	if !remoteMeta.ModifiedAt.IsZero() {
		artifact.Properties["remote_last_modified"] = remoteMeta.ModifiedAt.UTC().Format(http.TimeFormat)
		artifact.UpdatedAt = remoteMeta.ModifiedAt
	}
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
	cacheKey := key.String()
	n.metadataCacheMu.Lock()
	defer n.metadataCacheMu.Unlock()
	el, ok := n.metadataCache[cacheKey]
	if !ok {
		return nil, false
	}
	entry := el.Value.(*cachedArtifactEntry)
	if time.Now().After(entry.expiresAt) || entry.negative {
		n.removeElementLocked(el)
		return nil, false
	}
	// 命中后移到队尾（最近访问），LRU 淘汰时从队首删
	n.metadataCacheLL.MoveToBack(el)
	return entry.artifact, true
}

func (n *ProxyRuntime) setCachedArtifact(key ArtifactKey, artifact *Artifact) {
	if n.CachePolicy.MetadataTTL <= 0 || artifact == nil {
		return
	}
	cacheKey := key.String()
	entry := &cachedArtifactEntry{
		key:       key,
		artifact:  cloneArtifactForCache(artifact),
		expiresAt: time.Now().Add(n.CachePolicy.MetadataTTL),
	}
	n.metadataCacheMu.Lock()
	defer n.metadataCacheMu.Unlock()
	if n.metadataCache == nil {
		n.metadataCache = map[string]*list.Element{}
		n.metadataCacheLL = list.New()
	}
	if el, ok := n.metadataCache[cacheKey]; ok {
		el.Value = entry
		n.metadataCacheLL.MoveToBack(el)
		return
	}
	el := n.metadataCacheLL.PushBack(entry)
	n.metadataCache[cacheKey] = el
	// 超过上限时淘汰队首（最久未访问），O(1)
	for n.metadataCacheLL.Len() > maxMetadataCacheSize {
		n.removeOldestLocked()
	}
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
	cacheKey := key.String()
	n.metadataCacheMu.Lock()
	defer n.metadataCacheMu.Unlock()
	if el, ok := n.metadataCache[cacheKey]; ok {
		n.removeElementLocked(el)
	}
}

// removeOldestLocked 删除队首元素（最久未访问），O(1)。
// 调用方必须持有 metadataCacheMu 写锁。
func (n *ProxyRuntime) removeOldestLocked() {
	if n.metadataCacheLL == nil {
		return
	}
	el := n.metadataCacheLL.Front()
	if el == nil {
		return
	}
	n.removeElementLocked(el)
}

// removeElementLocked 从 map 和链表中同步移除元素，O(1)。
// 调用方必须持有 metadataCacheMu 写锁。
func (n *ProxyRuntime) removeElementLocked(el *list.Element) {
	entry := el.Value.(*cachedArtifactEntry)
	delete(n.metadataCache, entry.key.String())
	n.metadataCacheLL.Remove(el)
}

// evictOldestEntries 淘汰 count 个条目。
// LRU 实现下从队首删除，O(count)。
// 调用方必须持有 metadataCacheMu 写锁。
func (n *ProxyRuntime) evictOldestEntries(count int) {
	if count <= 0 || n.metadataCacheLL == nil {
		return
	}
	for i := 0; i < count && n.metadataCacheLL.Len() > 0; i++ {
		n.removeOldestLocked()
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
	n.setNegativeCacheWithTTL(key, n.CachePolicy.NegativeTTL)
}

// setNegativeCacheWithTTL 设置自定义 TTL 的负缓存条目。
// failureTTL <= 0 时不记录（调用方应保证 failureTTL > 0，
// refreshStaleMetadata 用 30s 下限兜底确保满足此约束）。
func (n *ProxyRuntime) setNegativeCacheWithTTL(key ArtifactKey, failureTTL time.Duration) {
	if failureTTL <= 0 {
		return
	}
	n.metadataCacheMu.Lock()
	if n.negativeCache == nil {
		n.negativeCache = map[string]time.Time{}
	}
	if len(n.negativeCache) >= maxNegativeCacheSize {
		n.evictNegativeCache(maxNegativeCacheSize / 4)
	}
	n.negativeCache[key.String()] = time.Now().Add(failureTTL)
	n.metadataCacheMu.Unlock()
}

func (n *ProxyRuntime) evictNegativeCache(count int) {
	if count <= 0 {
		return
	}
	oldest := make([]string, 0, count)
	for k, v := range n.negativeCache {
		if len(oldest) < count {
			oldest = append(oldest, k)
			continue
		}
		// 找到当前 oldest 中过期最晚的条目替换
		maxIdx := 0
		maxTime := n.negativeCache[oldest[0]]
		for i := 1; i < len(oldest); i++ {
			if t := n.negativeCache[oldest[i]]; t.After(maxTime) {
				maxTime = t
				maxIdx = i
			}
		}
		if v.Before(maxTime) {
			oldest[maxIdx] = k
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

// WarmUp 在熔断器恢复后预热内存缓存：遍历 metadataCache，对每个过期条目
// 异步触发 refreshStaleMetadata，使后续请求命中缓存而非重新回源。
func (n *ProxyRuntime) WarmUp() {
	if n.CachePolicy.MetadataTTL <= 0 {
		return
	}
	n.metadataCacheMu.Lock()
	now := time.Now()
	type entry struct {
		key   ArtifactKey
		art   *Artifact
		stale bool
	}
	var staleEntries []entry
	for _, el := range n.metadataCache {
		e := el.Value.(*cachedArtifactEntry)
		if now.After(e.expiresAt) || e.negative {
			continue
		}
		// 检查是否过期（距上次更新超过 TTL 的一半）
		if now.Sub(e.artifact.UpdatedAt) > n.CachePolicy.MetadataTTL/2 {
			staleEntries = append(staleEntries, entry{key: e.key, art: e.artifact, stale: true})
		}
	}
	n.metadataCacheMu.Unlock()

	if len(staleEntries) == 0 {
		return
	}
	logrus.WithFields(logrus.Fields{
		"repo_id":  n.RepositoryID,
		"staleCount": len(staleEntries),
	}).Info("proxy: warm-up triggered, refreshing stale metadata")

	for _, e := range staleEntries {
		go func(key ArtifactKey, art *Artifact) {
			refreshCtx, cancel := context.WithTimeout(context.Background(), backgroundRefreshTimeout)
			defer cancel()
			if err := n.refreshStaleMetadata(refreshCtx, art, key); err != nil {
				logrus.WithFields(logrus.Fields{
					"key":   key.String(),
					"error": err.Error(),
				}).Debug("proxy: warm-up refresh failed")
			}
		}(e.key, e.art)
	}
}
