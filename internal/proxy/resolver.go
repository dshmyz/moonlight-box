package proxy

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
)

// RepoRequestHandler 定义仓库请求处理接口
// 由 adapter 包实现，避免 proxy 包直接依赖 adapter 包
type RepoRequestHandler interface {
	types.Adapter
}

// ProxyFetcher 定义代理获取能力接口
// ProxyDownloader 实现此接口，供 adapter 做元数据刷新等场景使用
type ProxyFetcher interface {
	FetchFromRemote(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error)
}

type DownloadService interface {
	Download(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error)
}

// RepoHandler 负责仓库请求的统一处理
// 职责：
// 1. 路径解析 + 仓库解析策略（Local/Proxy/Virtual）
// 2. 虚拟仓库遍历和解析
// 3. 并发解析多个代理仓库
// 4. 调度 Adapter 处理请求和响应格式化
type RepoHandler struct {
	repoRepo    *repository.RepositoryRepository
	groupRepo   *repository.GroupRepository
	downloadSvc DownloadService
	repoCache   *RepositoryCache
	adapters    map[string]RepoRequestHandler
}

func NewRepoHandler(
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	repoCache *RepositoryCache,
) *RepoHandler {
	return &RepoHandler{
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		repoCache: repoCache,
		adapters:  make(map[string]RepoRequestHandler),
	}
}

func (r *RepoHandler) SetDownloadService(svc DownloadService) {
	r.downloadSvc = svc
}

func (r *RepoHandler) RegisterAdapter(pkgType string, handler RepoRequestHandler) {
	r.adapters[pkgType] = handler
}

// Resolve 解析包：根据仓库类型路由到不同的解析策略
// 供 RepoRouter 使用，传入 name/version
func (r *RepoHandler) Resolve(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	switch repo.Type {
	case model.RepoTypeLocal:
		return r.resolveLocal(ctx, repo, pkgType, name, version)
	case model.RepoTypeProxy:
		return r.resolveProxy(ctx, repo, pkgType, name, version)
	case model.RepoTypeVirtual:
		return r.resolveVirtual(ctx, repo, pkgType, name, version)
	default:
		return nil, fmt.Errorf("unsupported repository type: %s", repo.Type)
	}
}

// TryResolveDownload 尝试将路径解析为下载请求
// 返回 nil 表示不是下载路径，应交给 HandleRepoRequest 处理
func (r *RepoHandler) TryResolveDownload(ctx context.Context, repo *model.Repository, pkgType, path string) (*RouteResult, error) {
	adp, ok := r.adapters[pkgType]
	if !ok {
		return nil, fmt.Errorf("unsupported package type: %s", pkgType)
	}

	pkgIdentity, err := adp.ParsePackagePath(path)
	if err != nil {
		return nil, err
	}

	result, err := r.Resolve(ctx, repo, pkgType, pkgIdentity.Name, pkgIdentity.Version)
	if err != nil {
		return nil, err
	}

	result.Name = pkgIdentity.Name
	result.Version = pkgIdentity.Version
	result.Filename = filepath.Base(path)

	return result, nil
}

// FormatDownloadResponse 格式化下载响应，内部调用对应 adapter
func (r *RepoHandler) FormatDownloadResponse(c *gin.Context, pkgType string, result *RouteResult) error {
	adp, ok := r.adapters[pkgType]
	if !ok {
		return fmt.Errorf("unsupported package type: %s", pkgType)
	}
	downloadResult := &types.DownloadResult{
		Content:   result.Content,
		Size:      result.Size,
		FromCache: result.FromCache,
		RepoID:    result.RepoID,
		Filename:  result.Filename,
		Name:      result.Name,
		Version:   result.Version,
	}
	adp.FormatDownloadResponse(c, downloadResult)
	return nil
}

// HandleRepoRequest 处理仓库请求，内部调用对应 adapter
func (r *RepoHandler) HandleRepoRequest(c *gin.Context, repo *model.Repository, path string) error {
	adp, ok := r.adapters[repo.PackageType]
	if !ok {
		return fmt.Errorf("unsupported package type: %s", repo.PackageType)
	}
	ctx := &types.RepoRequestContext{
		Repo: repo,
		Path: path,
	}
	adp.HandleRepoRequest(c, ctx)
	return nil
}

func (r *RepoHandler) HandleRepoPublish(c *gin.Context, repo *model.Repository) (*types.RepoOperationResult, error) {
	adp, ok := r.adapters[repo.PackageType]
	if !ok {
		return nil, fmt.Errorf("unsupported package type: %s", repo.PackageType)
	}
	ctx := &types.PublishContext{
		Repo: repo,
	}
	result, err := adp.HandlePublish(c, ctx)
	if err != nil {
		return nil, err
	}
	return &types.RepoOperationResult{
		PackageName: result.PackageName,
		Version:     result.Version,
		Size:        result.Size,
		Filename:    result.Filename,
		Response:    result.Response,
	}, nil
}

func (r *RepoHandler) HandleRepoDelete(c *gin.Context, repo *model.Repository) (*types.RepoOperationResult, error) {
	adp, ok := r.adapters[repo.PackageType]
	if !ok {
		return nil, fmt.Errorf("unsupported package type: %s", repo.PackageType)
	}
	ctx := &types.DeleteContext{
		Repo: repo,
	}
	err := adp.HandleDelete(c, ctx)
	if err != nil {
		return nil, err
	}
	return &types.RepoOperationResult{}, nil
}

func (r *RepoHandler) resolveLocal(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	if r.downloadSvc == nil {
		return nil, fmt.Errorf("download service not initialized")
	}

	downloadCtx := &types.DownloadContext{
		Repo:     repo,
		PkgType:  model.PackageType(pkgType),
		Name:     name,
		Version:  version,
		Filename: "",
	}

	result, err := r.downloadSvc.Download(ctx, downloadCtx)
	if err != nil {
		return nil, err
	}
	return &RouteResult{
		SourceType: "local",
		Content:    result.Content,
		Size:       result.Size,
		FromCache:  result.FromCache,
		Name:       result.Name,
		Version:    result.Version,
		Filename:   result.Filename,
	}, nil
}

func (r *RepoHandler) resolveProxy(ctx context.Context, repo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	if r.downloadSvc == nil {
		return nil, fmt.Errorf("download service not initialized")
	}

	downloadCtx := &types.DownloadContext{
		Repo:     repo,
		PkgType:  model.PackageType(pkgType),
		Name:     name,
		Version:  version,
		Filename: "",
	}

	result, err := r.downloadSvc.Download(ctx, downloadCtx)
	if err != nil {
		return nil, err
	}
	return &RouteResult{
		Source:     repo.Name,
		SourceType: "proxy",
		RepoID:     result.RepoID,
		Content:    result.Content,
		Size:       result.Size,
		FromCache:  result.FromCache,
		Name:       result.Name,
		Version:    result.Version,
		Filename:   result.Filename,
	}, nil
}

func (r *RepoHandler) resolveVirtual(ctx context.Context, virtualRepo *model.Repository, pkgType, name, version string) (*RouteResult, error) {
	members, err := r.getMembers(virtualRepo.ID)
	if err != nil {
		return nil, err
	}

	for _, member := range members {
		if !r.isMemberTypeMatch(&member.MemberRepo, pkgType) {
			continue
		}

		result, err := r.Resolve(ctx, &member.MemberRepo, pkgType, name, version)
		if err == nil && result != nil {
			result.Source = member.MemberRepo.Name
			result.RepoID = member.MemberRepo.ID
			return result, nil
		}
	}

	return nil, ErrPackageNotFound
}

func (r *RepoHandler) getMembers(virtualRepoID uint) ([]model.RepositoryGroup, error) {
	if r.repoCache != nil {
		return r.repoCache.GetMembers(virtualRepoID)
	}
	return r.groupRepo.GetMembersByVirtualRepo(virtualRepoID)
}

func (r *RepoHandler) isMemberTypeMatch(repo *model.Repository, pkgType string) bool {
	return repo.PackageType == pkgType
}
