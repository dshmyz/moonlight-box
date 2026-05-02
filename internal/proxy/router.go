package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
	"gorm.io/gorm"
)

// ErrPackageNotFound 包未找到错误
var ErrPackageNotFound = fmt.Errorf("package not found")

// ProxyRouter 多代理路由引擎，负责根据虚拟仓配置将包请求路由到正确的来源
type ProxyRouter struct {
	db        *gorm.DB
	cache     *CacheService
	client    *RemoteClient
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	adapters  map[string]types.Adapter
}

// NewProxyRouter 创建一个新的代理路由引擎实例
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

// RouteResult 路由结果，包含内容流和元信息
type RouteResult struct {
	Source     string        // 来源仓库名称
	SourceType string        // 来源类型：local 或 proxy
	RepoID     uint          // 来源仓库 ID
	Content    io.ReadCloser // 包内容流
	Size       int64         // 内容大小
	FromCache  bool          // 是否来自缓存
	CacheTTL   int           // 缓存 TTL（秒）
}

type URLBuilder func(repo *model.Repository, name, version string) string

func (r *ProxyRouter) Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
	return r.resolveFull(ctx, pkgType, name, version, nil)
}

func (r *ProxyRouter) ResolveProxyOnlyForRepo(ctx context.Context, repo *model.Repository, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	return r.resolveProxyWithURL(ctx, repo, name, version, urlBuilder)
}

func (r *ProxyRouter) ResolveSmart(ctx context.Context, repo *model.Repository, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	if repo != nil && repo.Type == model.RepoTypeProxy {
		return r.resolveProxyWithURL(ctx, repo, name, version, urlBuilder)
	}

	virtualRepo, err := r.repoRepo.FindVirtualByPackageType(pkgType)
	if err != nil {
		if repo != nil && repo.Type == model.RepoTypeProxy {
			return r.resolveProxyWithURL(ctx, repo, name, version, urlBuilder)
		}
		return nil, ErrPackageNotFound
	}

	members, err := r.groupRepo.GetMembersByVirtualRepo(virtualRepo.ID)
	if err != nil {
		return nil, err
	}

	for _, member := range members {
		memberRepo := member.MemberRepo
		if memberRepo.Type != model.RepoTypeProxy {
			continue
		}

		result, err := r.resolveProxyWithURL(ctx, &memberRepo, name, version, urlBuilder)
		if err == nil && result != nil {
			result.Source = memberRepo.Name
			result.RepoID = memberRepo.ID
			return result, nil
		}
	}

	return nil, ErrPackageNotFound
}

func (r *ProxyRouter) ResolveForVirtualRepo(ctx context.Context, virtualRepo *model.Repository, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	members, err := r.groupRepo.GetMembersByVirtualRepo(virtualRepo.ID)
	if err != nil {
		return nil, err
	}

	for _, member := range members {
		repo := member.MemberRepo

		// 过滤不匹配类型的成员
		if !r.isMemberTypeMatch(&repo, pkgType) {
			continue
		}

		var result *RouteResult
		switch repo.Type {
		case model.RepoTypeLocal:
			result, err = r.resolveLocal(ctx, &repo, pkgType, name, version)
		case model.RepoTypeProxy:
			if urlBuilder != nil {
				result, err = r.resolveProxyWithURL(ctx, &repo, name, version, urlBuilder)
			} else {
				result, err = r.resolveProxy(ctx, &repo, pkgType, name, version)
			}
		default:
			continue
		}

		if err == nil && result != nil {
			result.Source = repo.Name
			result.RepoID = repo.ID
			return result, nil
		}
	}

	return nil, ErrPackageNotFound
}

func (r *ProxyRouter) ResolveProxyOnly(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	virtualRepo, err := r.repoRepo.FindVirtualByPackageType(pkgType)
	if err != nil {
		return nil, err
	}

	members, err := r.groupRepo.GetMembersByVirtualRepo(virtualRepo.ID)
	if err != nil {
		return nil, err
	}

	for _, member := range members {
		repo := member.MemberRepo
		if repo.Type != model.RepoTypeProxy {
			continue
		}

		result, err := r.resolveProxyWithURL(ctx, &repo, name, version, urlBuilder)
		if err == nil && result != nil {
			result.Source = repo.Name
			result.RepoID = repo.ID
			return result, nil
		}
	}

	return nil, ErrPackageNotFound
}

func (r *ProxyRouter) resolveFull(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	virtualRepo, err := r.repoRepo.FindVirtualByPackageType(pkgType)
	if err != nil {
		return nil, err
	}

	members, err := r.groupRepo.GetMembersByVirtualRepo(virtualRepo.ID)
	if err != nil {
		return nil, err
	}

	for _, member := range members {
		repo := member.MemberRepo

		var result *RouteResult
		switch repo.Type {
		case model.RepoTypeLocal:
			result, err = r.resolveLocal(ctx, &repo, pkgType, name, version)
		case model.RepoTypeProxy:
			if urlBuilder != nil {
				result, err = r.resolveProxyWithURL(ctx, &repo, name, version, urlBuilder)
			} else {
				result, err = r.resolveProxy(ctx, &repo, pkgType, name, version)
			}
		default:
			continue
		}

		if err == nil && result != nil {
			result.Source = repo.Name
			result.RepoID = repo.ID
			return result, nil
		}
	}

	return nil, ErrPackageNotFound
}

// resolveLocal 从本地仓库解析包
func (r *ProxyRouter) resolveLocal(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	adp, ok := r.adapters[pkgType]
	if !ok {
		return nil, fmt.Errorf("no adapter for package type: %s", pkgType)
	}

	// 构造包标识
	identity := &types.PackageIdentity{
		Type:    types.PackageType(pkgType),
		Name:    name,
		Version: version,
	}

	// 通过适配器下载包内容
	content, err := adp.Download(ctx, identity)
	if err != nil {
		return nil, err
	}

	// 将 Content 转换为 io.ReadCloser
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

	cached, err := r.cache.Get(ctx, cacheKey)
	if err == nil && cached != nil {
		if cached.IsNegative {
			return nil, ErrPackageNotFound
		}
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

	size := int64(len(content))
	r.cache.Set(ctx, &CacheItem{
		Key:         cacheKey,
		Content:     content,
		ContentType: contentType,
		Size:        size,
	}, time.Duration(repo.CacheTTLSeconds)*time.Second)

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

func (r *ProxyRouter) resolveProxy(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	cacheKey := fmt.Sprintf("proxy:%s:%s:%s", repo.Name, name, version)

	// 尝试从缓存获取
	cached, err := r.cache.Get(ctx, cacheKey)
	if err == nil && cached != nil {
		// 负向缓存：之前请求过且不存在
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

	// 缓存未命中，向远程仓库发起请求
	remoteURL := fmt.Sprintf("%s/%s/%s", repo.RemoteURL, name, version)
	authCfg, err := repo.GetAuthConfig()
	if err != nil {
		return nil, err
	}

	// 计算超时时间
	readTimeout := r.calcReadTimeout(repo, -1)

	// 解析失败缓存规则
	failureRules, _ := ParseFailureCacheRules(repo.FailureCacheRules)

	// 构建请求选项
	opts := RequestOptions{
		ReadTimeout:        readTimeout,
		MaxRedirects:       repo.MaxRedirects,
		InsecureSkipVerify: repo.InsecureSkipVerify,
	}

	// 获取远程内容
	content, contentType, err := r.client.GetBytes(ctx, remoteURL, opts, toProxyAuthConfig(authCfg))
	if err != nil {
		if remoteErr, ok := err.(*RemoteError); ok {
			// 根据失败缓存规则决定是否缓存
			if failureRules.ShouldCache(remoteErr.StatusCode) {
				ttl := failureRules.Match(remoteErr.StatusCode)
				r.cache.SetNegative(ctx, cacheKey, time.Duration(ttl)*time.Second)
			} else if remoteErr.IsNotFound() {
				// 兼容现有的 404 负向缓存逻辑
				r.cache.SetNegative(ctx, cacheKey, time.Duration(repo.CacheNegativeTTL)*time.Second)
			}
		}
		return nil, err
	}

	// 将远程内容写入缓存
	size := int64(len(content))
	r.cache.Set(ctx, &CacheItem{
		Key:         cacheKey,
		Content:     content,
		ContentType: contentType,
		Size:        size,
	}, time.Duration(repo.CacheTTLSeconds)*time.Second)

	return &RouteResult{
		SourceType: "proxy",
		Content:    io.NopCloser(bytes.NewReader(content)),
		Size:       size,
		FromCache:  false,
		CacheTTL:   repo.CacheTTLSeconds,
	}, nil
}

// calcReadTimeout 计算读取超时时间，大文件动态延长
func (r *ProxyRouter) calcReadTimeout(repo *model.Repository, contentLength int64) time.Duration {
	var baseTimeout time.Duration

	if repo.TimeoutSeconds > 0 {
		baseTimeout = time.Duration(repo.TimeoutSeconds) * time.Second
	} else {
		// 使用全局默认值
		cfg := config.Get()
		if cfg != nil {
			baseTimeout = cfg.Proxy.DefaultTimeout
		} else {
			baseTimeout = 30 * time.Second
		}
	}

	// 大文件动态延长超时
	if contentLength > 0 {
		threshold := int64(50 * 1024 * 1024) // 50MB
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

// toProxyAuthConfig 将 model.ProxyAuthConfig 转换为 proxy.ProxyAuthConfig
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
	// 支持 PackageTypes 多类型的成员
	if repo.PackageTypes != "" {
		var types []string
		if err := json.Unmarshal([]byte(repo.PackageTypes), &types); err == nil {
			for _, t := range types {
				if t == pkgType {
					return true
				}
			}
			return false
		}
	}

	// 回退到单一 PackageType
	return repo.PackageType == pkgType
}
