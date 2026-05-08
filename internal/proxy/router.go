package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
	"gorm.io/gorm"
)

var ErrPackageNotFound = fmt.Errorf("package not found")
var ErrNotComposite = fmt.Errorf("cannot add/remove members from non-composite repository")
var ErrMemberNotFound = fmt.Errorf("member repository not found")

type ProxyRouter struct {
	db                 *gorm.DB
	cache              *CacheService
	client             *RemoteClient
	repoRepo           *repository.RepositoryRepository
	groupRepo          *repository.GroupRepository
	adapters           map[string]types.Adapter
	healthCheckSvc     *HealthCheckService
	largeFileThreshold int64
	repoCache          *RepositoryCache
}

func NewProxyRouter(
	db *gorm.DB,
	cache *CacheService,
	client *RemoteClient,
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	adapters map[string]types.Adapter,
) *ProxyRouter {
	return &ProxyRouter{
		db:        db,
		cache:     cache,
		client:    client,
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		adapters:  adapters,
	}
}

func (r *ProxyRouter) SetLargeFileThreshold(threshold int64) {
	r.largeFileThreshold = threshold
}

func (r *ProxyRouter) SetRepoCache(cache *RepositoryCache) {
	r.repoCache = cache
}

func (r *ProxyRouter) SetHealthCheckService(svc *HealthCheckService) {
	r.healthCheckSvc = svc
}

type RouteResult struct {
	Source     string        // 来源仓库名称
	SourceType string        // 来源类型：local 或 proxy
	RepoID     uint          // 来源仓库 ID
	Content    io.ReadCloser // 包内容流
	Size       int64         // 内容大小
	FromCache  bool          // 是否来自缓存
	CacheTTL   int           // 缓存 TTL（秒）
	IsLarge    bool          // 是否大文件（流式处理）
}

type URLBuilder func(repo *model.Repository, name, version string) string

func (r *ProxyRouter) getVirtualRepo(pkgType string) (*model.Repository, error) {
	if r.repoCache != nil {
		return r.repoCache.GetVirtualRepo(pkgType)
	}
	return r.repoRepo.FindVirtualByPackageType(pkgType)
}

func (r *ProxyRouter) getMembers(virtualRepoID uint) ([]model.RepositoryGroup, error) {
	if r.repoCache != nil {
		return r.repoCache.GetMembers(virtualRepoID)
	}
	return r.groupRepo.GetMembersByVirtualRepo(virtualRepoID)
}

func (r *ProxyRouter) Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
	return r.resolveFull(ctx, pkgType, name, version, nil)
}

func (r *ProxyRouter) ResolveProxyOnlyForRepo(ctx context.Context, repo *model.Repository, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	return r.resolveProxyWithURL(ctx, repo, name, version, urlBuilder)
}

func (r *ProxyRouter) ResolveSmart(ctx context.Context, repo *model.Repository, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	if repo != nil && repo.Type == model.RepoTypeProxy {
		repoComp := NewProxyRepository(repo, r)
		return repoComp.Resolve(ctx, pkgType, name, version, urlBuilder)
	}

	virtualRepo, err := r.getVirtualRepo(pkgType)
	if err != nil {
		if repo != nil && repo.Type == model.RepoTypeProxy {
			repoComp := NewProxyRepository(repo, r)
			return repoComp.Resolve(ctx, pkgType, name, version, urlBuilder)
		}
		return nil, ErrPackageNotFound
	}

	virtualRepoComp, err := r.buildCompositeRepository(virtualRepo)
	if err != nil {
		return nil, err
	}

	return virtualRepoComp.Resolve(ctx, pkgType, name, version, urlBuilder)
}

func (r *ProxyRouter) ResolveForVirtualRepo(ctx context.Context, virtualRepo *model.Repository, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	virtualRepoComp, err := r.buildCompositeRepository(virtualRepo)
	if err != nil {
		return nil, err
	}
	return virtualRepoComp.Resolve(ctx, pkgType, name, version, urlBuilder)
}

func (r *ProxyRouter) buildCompositeRepository(repo *model.Repository) (Repository, error) {
	switch repo.Type {
	case model.RepoTypeLocal:
		return NewLocalRepository(repo, r), nil
	case model.RepoTypeProxy:
		return NewProxyRepository(repo, r), nil
	case model.RepoTypeVirtual:
		virtualRepo := NewVirtualRepository(repo, r)
		members, err := r.getMembers(repo.ID)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			memberComp, err := r.buildCompositeRepository(&member.MemberRepo)
			if err != nil {
				return nil, err
			}
			virtualRepo.AddMember(memberComp)
		}
		return virtualRepo, nil
	default:
		return nil, fmt.Errorf("unsupported repository type: %s", repo.Type)
	}
}

func (r *ProxyRouter) ResolveProxyOnly(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	virtualRepo, err := r.getVirtualRepo(pkgType)
	if err != nil {
		return nil, err
	}

	virtualRepoComp, err := r.buildCompositeRepository(virtualRepo)
	if err != nil {
		return nil, err
	}

	virtual, ok := virtualRepoComp.(*VirtualRepository)
	if !ok {
		return nil, ErrPackageNotFound
	}

	return virtual.ResolveConcurrent(ctx, pkgType, name, version, urlBuilder)
}

func (r *ProxyRouter) resolveFull(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	virtualRepo, err := r.getVirtualRepo(pkgType)
	if err != nil {
		return nil, err
	}

	virtualRepoComp, err := r.buildCompositeRepository(virtualRepo)
	if err != nil {
		return nil, err
	}
	return virtualRepoComp.Resolve(ctx, pkgType, name, version, urlBuilder)
}

func (r *ProxyRouter) resolveLocal(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	adp, ok := r.adapters[pkgType]
	if !ok {
		return nil, fmt.Errorf("no adapter for package type: %s", pkgType)
	}

	identity := &types.PackageIdentity{
		Type:    types.PackageType(pkgType),
		Name:    name,
		Version: version,
	}

	content, err := adp.Download(ctx, identity)
	if err != nil {
		return nil, err
	}

	var readCloser io.ReadCloser
	switch v := content.Content.(type) {
	case io.ReadCloser:
		readCloser = v
	case io.Reader:
		readCloser = io.NopCloser(v)
	case []byte:
		readCloser = io.NopCloser(bytes.NewReader(v))
	default:
		return nil, fmt.Errorf("unsupported content type from adapter: %T", content.Content)
	}

	return &RouteResult{
		SourceType: "local",
		Content:    readCloser,
		Size:       content.Size,
		FromCache:  false,
	}, nil
}

func (r *ProxyRouter) resolveProxyWithURL(ctx context.Context, repo *model.Repository, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	remoteURL := urlBuilder(repo, name, version)
	cacheKey := fmt.Sprintf("proxy:%s:%s", repo.Name, remoteURL)

	slog.Info("resolveProxyWithURL called", "repo", repo.Name, "remoteURL", remoteURL, "cacheKey", cacheKey)

	cached, err := r.cache.Get(ctx, cacheKey)
	if err == nil && cached != nil {
		if cached.IsNegative {
			slog.Warn("cache hit but negative", "cacheKey", cacheKey)
			return nil, ErrPackageNotFound
		}
		slog.Info("cache hit", "cacheKey", cacheKey, "size", cached.Size)
		return &RouteResult{
			Source:     repo.Name,
			SourceType: "proxy",
			RepoID:     repo.ID,
			Content:    io.NopCloser(bytes.NewReader(cached.Content)),
			Size:       cached.Size,
			FromCache:  true,
			CacheTTL:   repo.CacheTTLSeconds,
		}, nil
	}

	slog.Info("cache miss, fetching from remote", "remoteURL", remoteURL)

	if r.healthCheckSvc != nil && r.healthCheckSvc.ShouldSkipRequest(repo.ID) {
		retryAfter := r.healthCheckSvc.GetRetryAfter(repo.ID)
		slog.Warn("circuit breaker open, skipping request", "repo", repo.Name, "retry_after", retryAfter)
		return nil, fmt.Errorf("circuit breaker open for repo %s, retry after %d seconds", repo.Name, retryAfter)
	}

	authCfg, err := repo.GetAuthConfig()
	if err != nil {
		slog.Error("failed to get auth config", "error", err)
		return nil, err
	}

	readTimeout := r.calcReadTimeout(repo, -1)
	failureRules, _ := ParseFailureCacheRules(repo.FailureCacheRules)

	opts := RequestOptions{
		ReadTimeout:        readTimeout,
		MaxRedirects:       repo.MaxRedirects,
		InsecureSkipVerify: repo.InsecureSkipVerify,
	}

	if r.largeFileThreshold > 0 {
		return r.resolveProxyStream(ctx, repo, remoteURL, cacheKey, opts, authCfg, failureRules)
	}

	content, contentType, err := r.client.GetBytes(ctx, remoteURL, opts, toProxyAuthConfig(authCfg))
	if err != nil {
		slog.Error("failed to fetch from remote", "error", err, "remoteURL", remoteURL)

		if r.healthCheckSvc != nil {
			r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordFailure()
		}

		if remoteErr, ok := err.(*RemoteError); ok {
			slog.Error("remote error details", "statusCode", remoteErr.StatusCode, "url", remoteErr.URL)
			if failureRules.ShouldCache(remoteErr.StatusCode) {
				ttl := failureRules.Match(remoteErr.StatusCode)
				r.cache.SetNegative(ctx, cacheKey, time.Duration(ttl)*time.Second)
			} else if remoteErr.IsNotFound() {
				r.cache.SetNegative(ctx, cacheKey, time.Duration(repo.CacheNegativeTTL)*time.Second)
			}
		}
		return nil, err
	}

	if r.healthCheckSvc != nil {
		r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordSuccess()
	}

	slog.Info("successfully fetched from remote", "remoteURL", remoteURL, "size", len(content), "contentType", contentType)

	size := int64(len(content))
	shouldCache := r.largeFileThreshold == 0 || size <= r.largeFileThreshold

	if shouldCache {
		r.cache.Set(ctx, &CacheItem{
			Key:         cacheKey,
			Content:     content,
			ContentType: contentType,
			Size:        size,
		}, time.Duration(repo.CacheTTLSeconds)*time.Second)
	}

	return &RouteResult{
		Source:     repo.Name,
		SourceType: "proxy",
		RepoID:     repo.ID,
		Content:    io.NopCloser(bytes.NewReader(content)),
		Size:       size,
		FromCache:  false,
		CacheTTL:   repo.CacheTTLSeconds,
	}, nil
}

func (r *ProxyRouter) resolveProxyStream(ctx context.Context, repo *model.Repository, remoteURL, cacheKey string, opts RequestOptions, authCfg *model.ProxyAuthConfig, failureRules FailureCacheRules) (*RouteResult, error) {
	resp, err := r.client.GetStream(ctx, remoteURL, opts, toProxyAuthConfig(authCfg))
	if err != nil {
		slog.Error("failed to fetch stream from remote", "error", err, "remoteURL", remoteURL)

		if r.healthCheckSvc != nil {
			r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordFailure()
		}

		if remoteErr, ok := err.(*RemoteError); ok {
			slog.Error("remote error details", "statusCode", remoteErr.StatusCode, "url", remoteErr.URL)
			if failureRules.ShouldCache(remoteErr.StatusCode) {
				ttl := failureRules.Match(remoteErr.StatusCode)
				r.cache.SetNegative(ctx, cacheKey, time.Duration(ttl)*time.Second)
			} else if remoteErr.IsNotFound() {
				r.cache.SetNegative(ctx, cacheKey, time.Duration(repo.CacheNegativeTTL)*time.Second)
			}
		}
		return nil, err
	}

	if r.healthCheckSvc != nil {
		r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordSuccess()
	}

	contentLength := resp.ContentLength
	slog.Info("successfully fetched stream from remote", "remoteURL", remoteURL, "contentLength", contentLength)

	return &RouteResult{
		Source:     repo.Name,
		SourceType: "proxy",
		RepoID:     repo.ID,
		Content:    resp.Body,
		Size:       contentLength,
		FromCache:  false,
		CacheTTL:   repo.CacheTTLSeconds,
		IsLarge:    true,
	}, nil
}

func (r *ProxyRouter) resolveProxy(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	cacheKey := fmt.Sprintf("proxy:%s:%s:%s", repo.Name, name, version)

	cached, err := r.cache.Get(ctx, cacheKey)
	if err == nil && cached != nil {
		if cached.IsNegative {
			return nil, ErrPackageNotFound
		}
		return &RouteResult{
			SourceType: "proxy",
			Content:    io.NopCloser(bytes.NewReader(cached.Content)),
			Size:       cached.Size,
			FromCache:  true,
			CacheTTL:   repo.CacheTTLSeconds,
		}, nil
	}

	if r.healthCheckSvc != nil && r.healthCheckSvc.ShouldSkipRequest(repo.ID) {
		retryAfter := r.healthCheckSvc.GetRetryAfter(repo.ID)
		slog.Warn("circuit breaker open, skipping request", "repo", repo.Name, "retry_after", retryAfter)
		return nil, fmt.Errorf("circuit breaker open for repo %s, retry after %d seconds", repo.Name, retryAfter)
	}

	remoteURL := fmt.Sprintf("%s/%s/%s", repo.RemoteURL, name, version)
	authCfg, err := repo.GetAuthConfig()
	if err != nil {
		return nil, err
	}

	readTimeout := r.calcReadTimeout(repo, -1)
	failureRules, _ := ParseFailureCacheRules(repo.FailureCacheRules)

	opts := RequestOptions{
		ReadTimeout:        readTimeout,
		MaxRedirects:       repo.MaxRedirects,
		InsecureSkipVerify: repo.InsecureSkipVerify,
	}

	content, contentType, err := r.client.GetBytes(ctx, remoteURL, opts, toProxyAuthConfig(authCfg))
	if err != nil {
		if r.healthCheckSvc != nil {
			r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordFailure()
		}

		if remoteErr, ok := err.(*RemoteError); ok {
			if failureRules.ShouldCache(remoteErr.StatusCode) {
				ttl := failureRules.Match(remoteErr.StatusCode)
				r.cache.SetNegative(ctx, cacheKey, time.Duration(ttl)*time.Second)
			} else if remoteErr.IsNotFound() {
				r.cache.SetNegative(ctx, cacheKey, time.Duration(repo.CacheNegativeTTL)*time.Second)
			}
		}
		return nil, err
	}

	if r.healthCheckSvc != nil {
		r.healthCheckSvc.GetOrCreateCircuitBreaker(repo.ID).RecordSuccess()
	}

	size := int64(len(content))
	shouldCache := r.largeFileThreshold == 0 || size <= r.largeFileThreshold

	if shouldCache {
		r.cache.Set(ctx, &CacheItem{
			Key:         cacheKey,
			Content:     content,
			ContentType: contentType,
			Size:        size,
		}, time.Duration(repo.CacheTTLSeconds)*time.Second)
	}

	return &RouteResult{
		SourceType: "proxy",
		Content:    io.NopCloser(bytes.NewReader(content)),
		Size:       size,
		FromCache:  false,
		CacheTTL:   repo.CacheTTLSeconds,
	}, nil
}

type proxyResolveTask struct {
	member     model.RepositoryGroup
	urlBuilder URLBuilder
	pkgType    string
	name       string
	version    string
}

type proxyResolveError struct {
	repoName string
	err      error
}

func (r *ProxyRouter) resolveConcurrent(ctx context.Context, tasks []proxyResolveTask) (*RouteResult, error) {
	if len(tasks) == 0 {
		return nil, ErrPackageNotFound
	}

	if len(tasks) == 1 {
		task := tasks[0]
		repo := task.member.MemberRepo
		if repo.Type != model.RepoTypeProxy {
			return nil, ErrPackageNotFound
		}
		if task.urlBuilder != nil {
			return r.resolveProxyWithURL(ctx, &repo, task.name, task.version, task.urlBuilder)
		}
		return r.resolveProxy(ctx, &repo, task.pkgType, task.name, task.version)
	}

	resultCh := make(chan *RouteResult, len(tasks))
	errCh := make(chan proxyResolveError, len(tasks))
	var wg sync.WaitGroup
	ctxDone := ctx.Done()

	for _, task := range tasks {
		wg.Add(1)
		go func(t proxyResolveTask) {
			defer wg.Done()

			select {
			case <-ctxDone:
				return
			default:
			}

			repo := t.member.MemberRepo
			if repo.Type != model.RepoTypeProxy {
				return
			}

			var result *RouteResult
			var err error
			if t.urlBuilder != nil {
				result, err = r.resolveProxyWithURL(ctx, &repo, t.name, t.version, t.urlBuilder)
			} else {
				result, err = r.resolveProxy(ctx, &repo, t.pkgType, t.name, t.version)
			}

			if err == nil && result != nil {
				result.Source = repo.Name
				result.RepoID = repo.ID
				select {
				case resultCh <- result:
				case <-ctxDone:
				}
			} else if err != nil {
				select {
				case errCh <- proxyResolveError{repoName: repo.Name, err: err}:
				case <-ctxDone:
				}
			}
		}(task)
	}

	go func() {
		wg.Wait()
		close(resultCh)
		close(errCh)
	}()

	select {
	case result := <-resultCh:
		return result, nil
	case <-ctxDone:
		return nil, ctx.Err()
	}

	var errors []proxyResolveError
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		errMsg := fmt.Sprintf("failed to resolve package from %d repos: ", len(errors))
		for i, e := range errors {
			if i > 0 {
				errMsg += "; "
			}
			errMsg += fmt.Sprintf("%s: %v", e.repoName, e.err)
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return nil, ErrPackageNotFound
}

func (r *ProxyRouter) calcReadTimeout(repo *model.Repository, contentLength int64) time.Duration {
	var baseTimeout time.Duration

	if repo.TimeoutSeconds > 0 {
		baseTimeout = time.Duration(repo.TimeoutSeconds) * time.Second
	} else {
		cfg := config.Get()
		if cfg != nil {
			baseTimeout = cfg.Proxy.DefaultTimeout
		} else {
			baseTimeout = 30 * time.Second
		}
	}

	if contentLength > 0 {
		threshold := int64(50 * 1024 * 1024)
		cfg := config.Get()
		if cfg != nil && cfg.Proxy.LargeFileThreshold > 0 {
			threshold = cfg.Proxy.LargeFileThreshold
		}
		if contentLength > threshold {
			baseTimeout *= 2
		}
	}

	return baseTimeout
}

func toProxyAuthConfig(cfg *model.ProxyAuthConfig) *ProxyAuthConfig {
	if cfg == nil {
		return nil
	}
	result := &ProxyAuthConfig{Type: cfg.Type}
	if cfg.Basic != nil {
		result.Basic = &BasicAuth{
			Username: cfg.Basic.Username,
			Password: cfg.Basic.Password,
		}
	}
	if cfg.Bearer != nil {
		result.Bearer = &BearerAuth{Token: cfg.Bearer.Token}
	}
	if cfg.APIKey != nil {
		result.APIKey = &APIKeyAuth{
			HeaderName: cfg.APIKey.HeaderName,
			KeyValue:   cfg.APIKey.KeyValue,
			QueryParam: cfg.APIKey.QueryParam,
		}
	}
	return result
}

func (r *ProxyRouter) isMemberTypeMatch(repo *model.Repository, pkgType string) bool {
	return repo.PackageType == pkgType
}
