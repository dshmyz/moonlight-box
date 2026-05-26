package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/types"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/semaphore"
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

// MetadataMerger is an optional interface that adapters can implement
// to support merging metadata results from multiple member repositories
// in a virtual repository. If an adapter does not implement this interface,
// the default "first success wins" strategy is used.
type MetadataMerger interface {
	// MergeMetadata merges multiple metadata results into one.
	// results contains all successful responses from member repositories.
	// Returns the merged result, or nil+error if merging fails.
	MergeMetadata(ctx context.Context, results []*types.ContentResult, intent *types.RequestIntent) (*types.ContentResult, error)
}

// ProxyFetcher 定义代理获取能力接口
// ProxyDownloader 实现此接口，供 adapter 做元数据刷新等场景使用
type ProxyFetcher interface {
	FetchFromRemote(ctx context.Context, repo *model.Repository, remoteURL string) (*RouteResult, error)
}

// RepoHandler 负责仓库请求的统一处理
type RepoHandler struct {
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	repoCache *RepositoryCache
	adapters  map[string]RepoRequestHandler
	virtSem   *semaphore.Weighted
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
		virtSem:   semaphore.NewWeighted(50),
	}
}

func (r *RepoHandler) GetAdapter(pkgType model.PackageType) (RepoRequestHandler, bool) {
	adp, ok := r.adapters[string(pkgType)]
	return adp, ok
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
		Repo:     repo,
		UserID:   c.GetUint("userID"),
		ClientIP: c.ClientIP(),
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
	return nil, fmt.Errorf("local repository resolution not supported via RepoHandler")
}

func (r *RepoHandler) resolveProxy(ctx context.Context, downloadCtx *types.DownloadContext) (*RouteResult, error) {
	return nil, fmt.Errorf("proxy repository resolution not supported via RepoHandler")
}

func (r *RepoHandler) resolveVirtual(ctx context.Context, downloadCtx *types.DownloadContext) (*RouteResult, error) {
	members, err := r.getMembers(ctx, downloadCtx.Repo.ID)
	if err != nil {
		return nil, err
	}

	var matchingMembers []model.RepositoryMember
	for _, member := range members {
		if r.isMemberTypeMatch(&member.MemberRepo, string(downloadCtx.PkgType)) {
			matchingMembers = append(matchingMembers, member)
		}
	}

	if len(matchingMembers) == 0 {
		return nil, ErrPackageNotFound
	}

	// Acquire semaphore to limit concurrent virtual repo resolution
	if err := r.virtSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("virtual repo resolution throttled: %w", err)
	}
	defer r.virtSem.Release(1)

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
// 遍历成员仓库，对每个成员调用 adapter 的 HandleGet。
// 如果 adapter 实现了 MetadataMerger 接口，收集所有成功结果并合并；
// 否则使用 "第一个成功即返回" 策略。
func (r *RepoHandler) ResolveMetadata(ctx context.Context, virtualRepo *model.Repository, intent *types.RequestIntent, adp RepoRequestHandler) (*types.ContentResult, error) {
	members, err := r.getMembers(ctx, virtualRepo.ID)
	if err != nil {
		return nil, err
	}

	var matchingMembers []model.RepositoryMember
	for _, member := range members {
		if r.isMemberTypeMatch(&member.MemberRepo, string(adp.Type())) {
			matchingMembers = append(matchingMembers, member)
		}
	}

	if len(matchingMembers) == 0 {
		return nil, ErrPackageNotFound
	}

	// Check if adapter supports metadata merging
	_, supportsMerge := adp.(MetadataMerger)

	// Acquire semaphore
	if err := r.virtSem.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("virtual repo metadata resolution throttled: %w", err)
	}
	defer r.virtSem.Release(1)

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

	// Collect all results
	var successResults []*types.ContentResult
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
				if !supportsMerge {
					// No merge support: return first success immediately
					cancel()
					return mr.res, nil
				}
				// Read the content into memory for merging
				if mr.res.Content != nil {
					body, readErr := io.ReadAll(mr.res.Content)
					mr.res.Content.Close()
					if readErr != nil {
						continue
					}
					mr.res.Content = io.NopCloser(bytes.NewReader(body))
				}
				successResults = append(successResults, mr.res)
			}
		}
	}

	if len(successResults) > 0 && supportsMerge {
		merger := adp.(MetadataMerger)
		merged, mergeErr := merger.MergeMetadata(ctx, successResults, intent)
		if mergeErr != nil {
			// Merge failed, return the first successful result as fallback
			return successResults[0], nil
		}
		return merged, nil
	}

	if len(successResults) > 0 {
		return successResults[0], nil
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrPackageNotFound
}

func (r *RepoHandler) getMembers(ctx context.Context, virtualRepoID uint) ([]model.RepositoryMember, error) {
	if r.repoCache != nil {
		return r.repoCache.GetMembersContext(ctx, virtualRepoID)
	}
	return r.groupRepo.GetMembersByVirtualRepoContext(ctx, virtualRepoID)
}

func (r *RepoHandler) isMemberTypeMatch(repo *model.Repository, pkgType string) bool {
	return repo.PackageType == pkgType
}
