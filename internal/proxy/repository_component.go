package proxy

import (
	"context"

	"github.com/moonlight-box/registry/internal/model"
)

// Repository 组合模式中的 Component 接口
// 定义了所有仓库类型共同的操作
type Repository interface {
	GetID() uint
	GetName() string
	GetType() model.RepositoryType
	GetRepo() *model.Repository
	Resolve(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error)
	AddMember(child Repository) error
	RemoveMember(child Repository) error
	GetMembers() []Repository
}

// BaseRepository 基础仓库实现，提供通用功能
type BaseRepository struct {
	repo *model.Repository
}

func (b *BaseRepository) GetID() uint {
	return b.repo.ID
}

func (b *BaseRepository) GetName() string {
	return b.repo.Name
}

func (b *BaseRepository) GetType() model.RepositoryType {
	return b.repo.Type
}

func (b *BaseRepository) GetRepo() *model.Repository {
	return b.repo
}

// LocalRepository 本地仓库 - Leaf 实现
type LocalRepository struct {
	*BaseRepository
	router *ProxyRouter
}

func NewLocalRepository(repo *model.Repository, router *ProxyRouter) *LocalRepository {
	return &LocalRepository{
		BaseRepository: &BaseRepository{repo: repo},
		router:         router,
	}
}

func (r *LocalRepository) Resolve(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	return r.router.resolveLocal(ctx, r.repo, pkgType, name, version)
}

func (r *LocalRepository) AddMember(child Repository) error {
	return ErrNotComposite
}

func (r *LocalRepository) RemoveMember(child Repository) error {
	return ErrNotComposite
}

func (r *LocalRepository) GetMembers() []Repository {
	return nil
}

// ProxyRepository 代理仓库 - Leaf 实现
type ProxyRepository struct {
	*BaseRepository
	router *ProxyRouter
}

func NewProxyRepository(repo *model.Repository, router *ProxyRouter) *ProxyRepository {
	return &ProxyRepository{
		BaseRepository: &BaseRepository{repo: repo},
		router:         router,
	}
}

func (r *ProxyRepository) Resolve(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	if urlBuilder != nil {
		return r.router.resolveProxyWithURL(ctx, r.repo, name, version, urlBuilder)
	}
	return r.router.resolveProxy(ctx, r.repo, pkgType, name, version)
}

func (r *ProxyRepository) AddMember(child Repository) error {
	return ErrNotComposite
}

func (r *ProxyRepository) RemoveMember(child Repository) error {
	return ErrNotComposite
}

func (r *ProxyRepository) GetMembers() []Repository {
	return nil
}

// VirtualRepository 虚拟仓库 - Composite 实现
type VirtualRepository struct {
	*BaseRepository
	members []Repository
	router  *ProxyRouter
}

func NewVirtualRepository(repo *model.Repository, router *ProxyRouter) *VirtualRepository {
	return &VirtualRepository{
		BaseRepository: &BaseRepository{repo: repo},
		members:        make([]Repository, 0),
		router:         router,
	}
}

func (v *VirtualRepository) Resolve(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	for _, member := range v.members {
		if !v.router.isMemberTypeMatch(member.GetRepo(), pkgType) {
			continue
		}

		result, err := member.Resolve(ctx, pkgType, name, version, urlBuilder)
		if err == nil && result != nil {
			result.Source = member.GetName()
			result.RepoID = member.GetID()
			return result, nil
		}
	}
	return nil, ErrPackageNotFound
}

func (v *VirtualRepository) AddMember(child Repository) error {
	v.members = append(v.members, child)
	return nil
}

func (v *VirtualRepository) RemoveMember(child Repository) error {
	for i, member := range v.members {
		if member.GetID() == child.GetID() {
			v.members = append(v.members[:i], v.members[i+1:]...)
			return nil
		}
	}
	return ErrMemberNotFound
}

func (v *VirtualRepository) GetMembers() []Repository {
	return v.members
}

func (v *VirtualRepository) ResolveConcurrent(ctx context.Context, pkgType, name, version string, urlBuilder URLBuilder) (*RouteResult, error) {
	var tasks []proxyResolveTask
	for _, member := range v.members {
		if member.GetType() == model.RepoTypeProxy {
			tasks = append(tasks, proxyResolveTask{
				member:     model.RepositoryGroup{MemberRepo: *member.GetRepo()},
				urlBuilder: urlBuilder,
				pkgType:    pkgType,
				name:       name,
				version:    version,
			})
		}
	}
	return v.router.resolveConcurrent(ctx, tasks)
}
