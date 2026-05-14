package proxy

import (
	"context"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
)

// RepoRequestHandler 定义仓库请求处理接口
type RepoRequestHandler interface {
	ParseIntent(path string, method string) *types.RequestIntent
	HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error)
	HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error)
	HandleDelete(c *gin.Context, ctx *types.DeleteContext) error
	ParsePath(path string) (*types.PackagePathInfo, error)
	Type() types.PackageType
}

// ProxyFetcher 定义代理获取能力接口
// ProxyDownloader 实现此接口，供 adapter 做元数据刷新等场景使用
type ProxyFetcher interface {
	FetchFromRemote(ctx context.Context, repo *model.Repository, remoteURL string) (*RouteResult, error)
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

func (r *RepoHandler) GetAdapter(pkgType model.PackageType) (RepoRequestHandler, bool) {
	adp, ok := r.adapters[string(pkgType)]
	return adp, ok
}

func (r *RepoHandler) SetDownloadService(svc DownloadService) {
	r.downloadSvc = svc
}

func (r *RepoHandler) RegisterAdapter(pkgType string, handler RepoRequestHandler) {
	r.adapters[pkgType] = handler
}

// Resolve 解析包：根据仓库类型路由到不同的解析策略
func (r *RepoHandler) Resolve(ctx context.Context, downloadCtx *types.DownloadContext) (*RouteResult, error) {
	switch downloadCtx.Repo.Type {
	case model.RepoTypeLocal:
		return r.resolveLocal(ctx, downloadCtx)
	case model.RepoTypeProxy:
		return r.resolveProxy(ctx, downloadCtx)
	case model.RepoTypeVirtual:
		return r.resolveVirtual(ctx, downloadCtx)
	default:
		return nil, fmt.Errorf("unsupported repository type: %s", downloadCtx.Repo.Type)
	}
}

func (r *RepoHandler) HandleRepoPublish(c *gin.Context, repo *model.Repository) (*types.PublishResult, error) {
	adp, ok := r.adapters[repo.PackageType]
	if !ok {
		return nil, fmt.Errorf("unsupported package type: %s", repo.PackageType)
	}
	ctx := &types.PublishContext{
		Repo: repo,
	}
	return adp.HandlePut(c, ctx)
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

func (r *RepoHandler) resolveLocal(ctx context.Context, downloadCtx *types.DownloadContext) (*RouteResult, error) {
	if r.downloadSvc == nil {
		return nil, fmt.Errorf("download service not initialized")
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

func (r *RepoHandler) resolveProxy(ctx context.Context, downloadCtx *types.DownloadContext) (*RouteResult, error) {
	if r.downloadSvc == nil {
		return nil, fmt.Errorf("download service not initialized")
	}

	result, err := r.downloadSvc.Download(ctx, downloadCtx)
	if err != nil {
		return nil, err
	}
	return &RouteResult{
		Source:     downloadCtx.Repo.Name,
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

func (r *RepoHandler) resolveVirtual(ctx context.Context, downloadCtx *types.DownloadContext) (*RouteResult, error) {
	members, err := r.getMembers(ctx, downloadCtx.Repo.ID)
	if err != nil {
		return nil, err
	}

	var matchingMembers []model.RepositoryGroup
	for _, member := range members {
		if r.isMemberTypeMatch(&member.MemberRepo, string(downloadCtx.PkgType)) {
			matchingMembers = append(matchingMembers, member)
		}
	}

	if len(matchingMembers) == 0 {
		return nil, ErrPackageNotFound
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type memberResult struct {
		res        *RouteResult
		err        error
		sourceName string
		repoID     uint
	}
	resultCh := make(chan memberResult, len(matchingMembers))

	for _, member := range matchingMembers {
		member := member
		go func() {
			memberCtx := *downloadCtx
			memberCtx.Repo = &member.MemberRepo
			ctxWithRepo := context.WithValue(ctx, "repo", &member.MemberRepo)
			res, err := r.Resolve(ctxWithRepo, &memberCtx)
			resultCh <- memberResult{
				res:        res,
				err:        err,
				sourceName: member.MemberRepo.Name,
				repoID:     member.MemberRepo.ID,
			}
		}()
	}

	var firstErr error
	remaining := len(matchingMembers)
	for remaining > 0 {
		select {
		case <-ctx.Done():
			if firstErr == nil {
				return nil, ErrPackageNotFound
			}
			return nil, firstErr
		case mr := <-resultCh:
			remaining--
			if mr.err != nil {
				if firstErr == nil && !isPackageNotFoundError(mr.err) {
					firstErr = mr.err
				}
				continue
			}
			if mr.res != nil {
				cancel()
				mr.res.Source = mr.sourceName
				mr.res.RepoID = mr.repoID
				return mr.res, nil
			}
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrPackageNotFound
}

func isPackageNotFoundError(err error) bool {
	return errors.Is(err, ErrPackageNotFound)
}

// ResolveMetadata 解析虚拟仓库的元数据请求
// 遍历成员仓库，对每个成员调用 adapter 的 HandleGet，返回第一个成功的结果
func (r *RepoHandler) ResolveMetadata(ctx context.Context, virtualRepo *model.Repository, intent *types.RequestIntent, adp RepoRequestHandler) (*types.ContentResult, error) {
	members, err := r.getMembers(ctx, virtualRepo.ID)
	if err != nil {
		return nil, err
	}

	var matchingMembers []model.RepositoryGroup
	for _, member := range members {
		if r.isMemberTypeMatch(&member.MemberRepo, string(adp.Type())) {
			matchingMembers = append(matchingMembers, member)
		}
	}

	if len(matchingMembers) == 0 {
		return nil, ErrPackageNotFound
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type memberResult struct {
		res        *types.ContentResult
		err        error
		sourceName string
	}
	resultCh := make(chan memberResult, len(matchingMembers))

	for _, member := range matchingMembers {
		member := member
		go func() {
			ctxWithRepo := context.WithValue(ctx, "repo", &member.MemberRepo)
			res, err := adp.HandleGet(ctxWithRepo, &member.MemberRepo, intent)
			resultCh <- memberResult{
				res:        res,
				err:        err,
				sourceName: member.MemberRepo.Name,
			}
		}()
	}

	var firstErr error
	remaining := len(matchingMembers)
	for remaining > 0 {
		select {
		case <-ctx.Done():
			if firstErr == nil {
				return nil, ErrPackageNotFound
			}
			return nil, firstErr
		case mr := <-resultCh:
			remaining--
			if mr.err != nil {
				if firstErr == nil && !isPackageNotFoundError(mr.err) {
					firstErr = mr.err
				}
				continue
			}
			if mr.res != nil && mr.res.StatusCode < 400 {
				cancel()
				return mr.res, nil
			}
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrPackageNotFound
}

func (r *RepoHandler) getMembers(ctx context.Context, virtualRepoID uint) ([]model.RepositoryGroup, error) {
	if r.repoCache != nil {
		return r.repoCache.GetMembersContext(ctx, virtualRepoID)
	}
	return r.groupRepo.GetMembersByVirtualRepoContext(ctx, virtualRepoID)
}

func (r *RepoHandler) isMemberTypeMatch(repo *model.Repository, pkgType string) bool {
	return repo.PackageType == pkgType
}
