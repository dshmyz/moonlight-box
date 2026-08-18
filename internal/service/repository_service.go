package service

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	apperr "github.com/dshmyz/moonlight-box/internal/errors"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/proxy"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/gorm"
)

// RepositoryListView 仓库列表视图，包含运行时健康状态
type RepositoryListView struct {
	model.Repository
	HealthInfo *RepositoryHealthInfo `json:"health_info,omitempty"`
}

// RepositoryHealthInfo 仓库健康信息视图
type RepositoryHealthInfo struct {
	HealthStatus   *proxy.HealthStatus `json:"health_status,omitempty"`
	CircuitBreaker interface{}         `json:"circuit_breaker,omitempty"`
}

// RepositoryService 仓库管理服务层
type RepositoryService struct {
	repoRepo       *repository.RepositoryRepository
	groupRepo      *repository.GroupRepository
	repoCache      *proxy.RepositoryCache
	healthCheckSvc *proxy.HealthCheckService
	db             *gorm.DB
	repoMgr        *runtime.DefaultRepositoryManager
	artifactSvc    *ArtifactService

	// 列表缓存
	listCacheMu  sync.RWMutex
	listCache    map[string]*listCacheEntry
	listCacheTTL time.Duration
}

type listCacheEntry struct {
	result    []RepositoryListView
	expiresAt time.Time
}

// NewRepositoryService 创建仓库管理服务实例
func NewRepositoryService(repoRepo *repository.RepositoryRepository, groupRepo *repository.GroupRepository, db *gorm.DB) *RepositoryService {
	return &RepositoryService{
		repoRepo:     repoRepo,
		groupRepo:    groupRepo,
		db:           db,
		listCache:    make(map[string]*listCacheEntry),
		listCacheTTL: 5 * time.Second,
	}
}

// SetRepoCache 设置仓库缓存
func (s *RepositoryService) SetRepoCache(cache *proxy.RepositoryCache) {
	s.repoCache = cache
}

// SetHealthCheckService 设置健康检查服务
func (s *RepositoryService) SetHealthCheckService(svc *proxy.HealthCheckService) {
	s.healthCheckSvc = svc
}

// SetRepoManager 设置运行时仓库管理器
func (s *RepositoryService) SetRepoManager(mgr *runtime.DefaultRepositoryManager) {
	s.repoMgr = mgr
}

// SetArtifactService 注入 ArtifactService，用于缓存迁移等需要行级数据操作的功能。
func (s *RepositoryService) SetArtifactService(as *ArtifactService) {
	s.artifactSvc = as
}

// invalidateCache 失效缓存
func (s *RepositoryService) invalidateCache(name string) {
	if s.repoCache != nil {
		s.repoCache.Invalidate(name)
	}
	// 使 manager 中的内存缓存失效，下次请求时懒加载
	if s.repoMgr != nil {
		if name == "*" {
			s.repoMgr.Reload()
		} else {
			s.repoMgr.Invalidate(name)
		}
	}
	// 列表缓存全部失效（仓库变更后列表都可能变化）
	s.listCacheMu.Lock()
	s.listCache = make(map[string]*listCacheEntry)
	s.listCacheMu.Unlock()
}

// cascadeInvalidateParents 级联失效：当成员仓库变更时，同时失效包含它的虚拟仓库。
// 因为虚拟仓库的 GroupRuntime 持有成员的 Runtime 引用，成员变了必须重建。
func (s *RepositoryService) cascadeInvalidateParents(memberRepoName string) {
	if s.repoMgr == nil {
		return
	}
	repo, err := s.repoRepo.FindByName(memberRepoName)
	if err != nil {
		return
	}
	parents, err := s.groupRepo.GetParentVirtualRepos(repo.ID)
	if err != nil {
		return
	}
	for _, parent := range parents {
		s.repoMgr.Invalidate(parent.Name)
	}
}

// listCacheKey 生成列表缓存键
func listCacheKey(filter map[string]interface{}) string {
	if len(filter) == 0 {
		return "_all"
	}
	keys := make([]string, 0, len(filter))
	for k := range filter {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	key := ""
	for _, k := range keys {
		key += fmt.Sprintf("%s=%v;", k, filter[k])
	}
	return key
}

// copyListView 深拷贝 RepositoryListView 切片
// handler 会修改 URL/Config 等字段，缓存返回必须走拷贝
func copyListView(src []RepositoryListView) []RepositoryListView {
	dst := make([]RepositoryListView, len(src))
	for i := range src {
		dst[i] = src[i]
	}
	return dst
}

// Create 创建仓库，如果是虚拟仓则同时添加成员
func (s *RepositoryService) Create(repo *model.Repository, members []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(repo).Error; err != nil {
			return err
		}

		if repo.Type == model.RepoTypeVirtual && len(members) > 0 {
			for i, memberName := range members {
				var memberRepo model.Repository
				if err := tx.Where("name = ?", memberName).First(&memberRepo).Error; err != nil {
					return fmt.Errorf("member repository not found: %s", memberName)
				}
				if memberRepo.PackageType != repo.PackageType {
					return apperr.NewAppError(http.StatusBadRequest, fmt.Sprintf(
						"member repository %s format %q does not match virtual repository %s format %q",
						memberName, memberRepo.PackageType, repo.Name, repo.PackageType), nil)
				}
				group := model.RepositoryMember{
					RepositoryID: repo.ID,
					MemberID:     memberRepo.ID,
					Position:     i,
				}
				if err := tx.Create(&group).Error; err != nil {
					return err
				}
			}
		}

		s.invalidateCache("*")

		return nil
	})
}

// List 列出仓库，支持按条件过滤
func (s *RepositoryService) List(filter map[string]interface{}) ([]model.Repository, error) {
	return s.ListContext(context.Background(), filter, 0, 0)
}

// ListContext 列出仓库，支持按条件过滤和分页（page=0/pageSize=0 不分页）
func (s *RepositoryService) ListContext(ctx context.Context, filter map[string]interface{}, page, pageSize int) ([]model.Repository, error) {
	return s.repoRepo.ListContext(ctx, filter, page, pageSize)
}

// Count 统计符合条件的仓库数量
func (s *RepositoryService) Count(ctx context.Context, filter map[string]interface{}) (int64, error) {
	return s.repoRepo.CountContext(ctx, filter)
}

// ListWithHealth 列出仓库并附加健康状态信息
func (s *RepositoryService) ListWithHealth(filter map[string]interface{}) ([]RepositoryListView, error) {
	result, _, err := s.ListWithHealthContext(context.Background(), filter, 0, 0)
	return result, err
}

// ListWithHealthContext 列出仓库并附加健康状态信息，支持分页
// 返回 (列表, 总数, 错误)
func (s *RepositoryService) ListWithHealthContext(ctx context.Context, filter map[string]interface{}, page, pageSize int) ([]RepositoryListView, int64, error) {
	// 检查列表缓存（只在不分页时走缓存）
	if page <= 0 || pageSize <= 0 {
		cacheKey := listCacheKey(filter)
		s.listCacheMu.RLock()
		if entry, ok := s.listCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
			result := copyListView(entry.result)
			s.listCacheMu.RUnlock()
			return result, int64(len(result)), nil
		}
		s.listCacheMu.RUnlock()
	}

	// 先查总数
	total, err := s.repoRepo.CountContext(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	repos, err := s.repoRepo.ListContext(ctx, filter, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	var healthStatuses map[uint]*proxy.HealthStatus
	if s.healthCheckSvc != nil {
		healthStatuses = s.healthCheckSvc.GetAllHealthStatuses()
	}

	result := make([]RepositoryListView, len(repos))
	for i, repo := range repos {
		healthStatus := healthStatuses[repo.ID]
		var healthInfo *RepositoryHealthInfo
		if healthStatus != nil {
			healthInfo = &RepositoryHealthInfo{
				HealthStatus: healthStatus,
			}
		}
		result[i] = RepositoryListView{
			Repository: repo,
			HealthInfo: healthInfo,
		}
	}

	// 不分页且结果不为空时写入缓存
	if page <= 0 || pageSize <= 0 {
		cacheKey := listCacheKey(filter)
		s.listCacheMu.Lock()
		s.listCache[cacheKey] = &listCacheEntry{
			result:    result,
			expiresAt: time.Now().Add(s.listCacheTTL),
		}
		s.listCacheMu.Unlock()
	}

	return result, total, nil
}

// Get 根据名称获取仓库详情
func (s *RepositoryService) Get(name string) (*model.Repository, error) {
	return s.GetContext(context.Background(), name)
}

// GetContext 根据名称获取仓库详情
func (s *RepositoryService) GetContext(ctx context.Context, name string) (*model.Repository, error) {
	if s.repoCache != nil {
		return s.repoCache.GetByNameContext(ctx, name)
	}
	return s.repoRepo.FindByNameContext(ctx, name)
}

// GetByID 根据ID获取仓库详情
func (s *RepositoryService) GetByID(id uint) (*model.Repository, error) {
	return s.GetByIDContext(context.Background(), id)
}

// GetByIDContext 根据ID获取仓库详情
func (s *RepositoryService) GetByIDContext(ctx context.Context, id uint) (*model.Repository, error) {
	if s.repoCache != nil {
		return s.repoCache.GetByIDContext(ctx, id)
	}
	return s.repoRepo.FindByIDContext(ctx, id)
}

// Update 更新仓库信息
func (s *RepositoryService) Update(name string, params *model.UpdateRepositoryParams) error {
	repo, fields := params.ToRepoForUpdate()
	members := params.Members

	if members != nil {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			if len(fields) > 0 {
				if err := tx.Model(&model.Repository{}).Where("name = ?", name).Select(fields).Updates(repo).Error; err != nil {
					return err
				}
			}

			var virtualRepo model.Repository
			if err := tx.Where("name = ?", name).First(&virtualRepo).Error; err != nil {
				return err
			}

			if err := tx.Where("repository_id = ?", virtualRepo.ID).Delete(&model.RepositoryMember{}).Error; err != nil {
				return err
			}

			for i, memberName := range members {
				var memberRepo model.Repository
				if err := tx.Where("name = ?", memberName).First(&memberRepo).Error; err != nil {
					return fmt.Errorf("member repository not found: %s", memberName)
				}
				if memberRepo.PackageType != virtualRepo.PackageType {
					return apperr.NewAppError(http.StatusBadRequest, fmt.Sprintf(
						"member repository %s format %q does not match virtual repository %s format %q",
						memberName, memberRepo.PackageType, name, virtualRepo.PackageType), nil)
				}
				group := model.RepositoryMember{
					RepositoryID: virtualRepo.ID,
					MemberID:     memberRepo.ID,
					Position:     i,
				}
				if err := tx.Create(&group).Error; err != nil {
					return err
				}
			}

			return nil
		})
		if err == nil {
			s.invalidateCache(name)
		}
		return err
	}

	err := s.repoRepo.Update(name, repo, fields)
	if err == nil {
		s.invalidateCache(name)
		// 成员仓库配置变更时，级联失效包含它的虚拟仓库
		s.cascadeInvalidateParents(name)
	}
	return err
}

// Delete 删除仓库
func (s *RepositoryService) Delete(name string) error {
	err := s.repoRepo.Delete(name)
	if err == nil {
		s.invalidateCache(name)
		if s.repoMgr != nil {
			s.repoMgr.Delete(name)
		}
		// 删除成员仓库时，级联失效包含它的虚拟仓库
		s.cascadeInvalidateParents(name)
	}
	return err
}

// MigrateCacheToRepo 把 proxy 仓库的缓存内容整体迁移到指定的 local 仓库。
// 返回 ArtifactService 的迁移结果；目标仓库存在重叠内容时 Conflicts 非空，由调用方转 409。
func (s *RepositoryService) MigrateCacheToRepo(ctx context.Context, sourceName, targetName string) (*MigrateResult, error) {
	source, err := s.repoRepo.FindByNameContext(ctx, sourceName)
	if err != nil {
		return nil, err
	}
	target, err := s.repoRepo.FindByNameContext(ctx, targetName)
	if err != nil {
		return nil, err
	}

	if source.Type != model.RepoTypeProxy {
		return nil, apperr.NewAppError(http.StatusBadRequest,
			fmt.Sprintf("repository %s is not a proxy repository (type=%s)", sourceName, source.Type), nil)
	}
	if target.Type != model.RepoTypeLocal {
		return nil, apperr.NewAppError(http.StatusBadRequest,
			fmt.Sprintf("target repository %s must be a local repository (type=%s)", targetName, target.Type), nil)
	}
	if source.PackageType != target.PackageType {
		return nil, apperr.NewAppError(http.StatusBadRequest, fmt.Sprintf(
			"package format mismatch: source %q vs target %q", source.PackageType, target.PackageType), nil)
	}
	if source.ID == target.ID {
		return nil, apperr.NewAppError(http.StatusBadRequest,
			fmt.Sprintf("source and target repository must differ (%s)", sourceName), nil)
	}

	if s.artifactSvc == nil {
		return nil, fmt.Errorf("artifact service not injected")
	}
	result, err := s.artifactSvc.MigrateArtifactsToRepo(ctx, source.ID, target.ID)
	if err != nil {
		return nil, err
	}

	// 迁移（或冲突检查）后源/目标仓库的缓存与运行时均需失效。
	// 级联失效包含它们的虚拟仓库：源仓库内容被清空、目标仓库内容被填充，
	// 持有任一 Runtime 引用的 GroupRuntime 都必须重建。
	s.invalidateCache(sourceName)
	s.invalidateCache(targetName)
	s.cascadeInvalidateParents(sourceName)
	s.cascadeInvalidateParents(targetName)
	return result, nil
}

// AddMember 向虚拟仓库添加成员
func (s *RepositoryService) AddMember(virtualRepoName, memberRepoName string, priority int) error {
	virtualRepo, err := s.repoRepo.FindByName(virtualRepoName)
	if err != nil {
		return err
	}
	if virtualRepo.Type != model.RepoTypeVirtual {
		return apperr.NewAppError(http.StatusBadRequest,
			fmt.Sprintf("repository %s is not a virtual (group) repository", virtualRepoName), nil)
	}
	memberRepo, err := s.repoRepo.FindByName(memberRepoName)
	if err != nil {
		return err
	}
	if memberRepo.PackageType != virtualRepo.PackageType {
		return apperr.NewAppError(http.StatusBadRequest, fmt.Sprintf(
			"member repository %s format %q does not match virtual repository %s format %q",
			memberRepoName, memberRepo.PackageType, virtualRepoName, virtualRepo.PackageType), nil)
	}
	err = s.groupRepo.AddMember(virtualRepo.ID, memberRepo.ID, priority)
	if err == nil {
		s.invalidateCache(virtualRepoName)
	}
	return err
}

// RemoveMember 从虚拟仓库移除成员
func (s *RepositoryService) RemoveMember(virtualRepoName, memberRepoName string) error {
	virtualRepo, err := s.repoRepo.FindByName(virtualRepoName)
	if err != nil {
		return err
	}
	memberRepo, err := s.repoRepo.FindByName(memberRepoName)
	if err != nil {
		return err
	}
	err = s.groupRepo.RemoveMember(virtualRepo.ID, memberRepo.ID)
	if err == nil {
		s.invalidateCache(virtualRepoName)
	}
	return err
}

// GetMembers 获取虚拟仓库的所有成员
func (s *RepositoryService) GetMembers(virtualRepoName string) ([]model.RepositoryMember, error) {
	return s.GetMembersContext(context.Background(), virtualRepoName)
}

// GetMembersContext 获取虚拟仓库的所有成员
func (s *RepositoryService) GetMembersContext(ctx context.Context, virtualRepoName string) ([]model.RepositoryMember, error) {
	virtualRepo, err := s.repoRepo.FindByNameContext(ctx, virtualRepoName)
	if err != nil {
		return nil, err
	}
	return s.groupRepo.GetMembersByVirtualRepoContext(ctx, virtualRepo.ID)
}
