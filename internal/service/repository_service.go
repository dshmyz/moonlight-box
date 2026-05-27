package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
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

// RuntimeFactory 创建仓库运行时实例的函数类型
type RuntimeFactory func(repo *model.Repository, members []string) (*runtime.Repository, error)

// RepositoryService 仓库管理服务层
type RepositoryService struct {
	repoRepo       *repository.RepositoryRepository
	groupRepo      *repository.GroupRepository
	repoCache      *proxy.RepositoryCache
	healthCheckSvc *proxy.HealthCheckService
	db             *gorm.DB
	repoMgr        *runtime.DefaultRepositoryManager
	runtimeFactory RuntimeFactory

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

// SetRuntimeFactory 设置运行时工厂函数
func (s *RepositoryService) SetRuntimeFactory(f RuntimeFactory) {
	s.runtimeFactory = f
}

// invalidateCache 失效缓存
func (s *RepositoryService) invalidateCache(name string) {
	if s.repoCache != nil {
		s.repoCache.Invalidate(name)
	}
	// 列表缓存全部失效（仓库变更后列表都可能变化）
	s.listCacheMu.Lock()
	s.listCache = make(map[string]*listCacheEntry)
	s.listCacheMu.Unlock()
}

// reloadVirtualRepoRuntime 重新加载虚拟仓库的 GroupRuntime
// 当成员变更时需要调用，确保 in-memory Runtime 与 DB 一致
func (s *RepositoryService) reloadVirtualRepoRuntime(virtualRepo *model.Repository) {
	if s.runtimeFactory == nil || s.repoMgr == nil {
		return
	}
	// 重新从 DB 加载（包含最新的成员列表）
	updated, err := s.repoRepo.FindByName(virtualRepo.Name)
	if err != nil {
		return
	}
	rtRepo, err := s.runtimeFactory(updated, nil)
	if err != nil {
		return
	}
	s.repoMgr.Set(rtRepo)
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
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(repo).Error; err != nil {
			return err
		}

		if repo.Type == model.RepoTypeVirtual && len(members) > 0 {
			for i, memberName := range members {
				var memberRepo model.Repository
				if err := tx.Where("name = ?", memberName).First(&memberRepo).Error; err != nil {
					return fmt.Errorf("member repository not found: %s", memberName)
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
	if err == nil && s.repoMgr != nil && s.runtimeFactory != nil {
		rtRepo, factoryErr := s.runtimeFactory(repo, members)
		if factoryErr == nil {
			s.repoMgr.Set(rtRepo)
		}
	}
	return err
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
	return s.repoRepo.FindByNameContext(ctx, name)
}

// GetByID 根据ID获取仓库详情
func (s *RepositoryService) GetByID(id uint) (*model.Repository, error) {
	return s.GetByIDContext(context.Background(), id)
}

// GetByIDContext 根据ID获取仓库详情
func (s *RepositoryService) GetByIDContext(ctx context.Context, id uint) (*model.Repository, error) {
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
		if err == nil && s.repoMgr != nil && s.runtimeFactory != nil {
			if updatedRepo, repoErr := s.repoRepo.FindByName(name); repoErr == nil {
				if rtRepo, factoryErr := s.runtimeFactory(updatedRepo, members); factoryErr == nil {
					s.repoMgr.Set(rtRepo)
				}
			}
		}
		return err
	}

	err := s.repoRepo.Update(name, repo, fields)
	if err == nil {
		s.invalidateCache(name)
	}
	if err == nil && s.repoMgr != nil && s.runtimeFactory != nil {
		if updatedRepo, repoErr := s.repoRepo.FindByName(name); repoErr == nil {
			if rtRepo, factoryErr := s.runtimeFactory(updatedRepo, nil); factoryErr == nil {
				s.repoMgr.Set(rtRepo)
			}
		}
	}
	return err
}

// Delete 删除仓库
func (s *RepositoryService) Delete(name string) error {
	err := s.repoRepo.Delete(name)
	if err == nil {
		s.invalidateCache(name)
	}
	if err == nil && s.repoMgr != nil {
		s.repoMgr.Delete(name)
	}
	return err
}

// AddMember 向虚拟仓库添加成员
func (s *RepositoryService) AddMember(virtualRepoName, memberRepoName string, priority int) error {
	virtualRepo, err := s.repoRepo.FindByName(virtualRepoName)
	if err != nil {
		return err
	}
	memberRepo, err := s.repoRepo.FindByName(memberRepoName)
	if err != nil {
		return err
	}
	err = s.groupRepo.AddMember(virtualRepo.ID, memberRepo.ID, priority)
	if err == nil {
		s.invalidateCache(virtualRepoName)
		s.reloadVirtualRepoRuntime(virtualRepo)
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
		s.reloadVirtualRepoRuntime(virtualRepo)
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
