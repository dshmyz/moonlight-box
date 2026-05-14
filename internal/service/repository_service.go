package service

import (
	"context"
	"fmt"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
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
}

// NewRepositoryService 创建仓库管理服务实例
func NewRepositoryService(repoRepo *repository.RepositoryRepository, groupRepo *repository.GroupRepository, db *gorm.DB) *RepositoryService {
	return &RepositoryService{
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		db:        db,
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

// invalidateCache 失效缓存
func (s *RepositoryService) invalidateCache(name string) {
	if s.repoCache != nil {
		s.repoCache.Invalidate(name)
	}
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
				group := model.RepositoryGroup{
					VirtualRepoID: repo.ID,
					MemberRepoID:  memberRepo.ID,
					Priority:      i,
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
	return s.ListContext(context.Background(), filter)
}

// ListContext 列出仓库，支持按条件过滤
func (s *RepositoryService) ListContext(ctx context.Context, filter map[string]interface{}) ([]model.Repository, error) {
	return s.repoRepo.ListContext(ctx, filter)
}

// ListWithHealth 列出仓库并附加健康状态信息
func (s *RepositoryService) ListWithHealth(filter map[string]interface{}) ([]RepositoryListView, error) {
	return s.ListWithHealthContext(context.Background(), filter)
}

// ListWithHealthContext 列出仓库并附加健康状态信息
func (s *RepositoryService) ListWithHealthContext(ctx context.Context, filter map[string]interface{}) ([]RepositoryListView, error) {
	repos, err := s.repoRepo.ListContext(ctx, filter)
	if err != nil {
		return nil, err
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
	return result, nil
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
func (s *RepositoryService) Update(name string, updates map[string]interface{}) error {
	var members []string
	if membersRaw, ok := updates["members"]; ok {
		delete(updates, "members")

		switch v := membersRaw.(type) {
		case []string:
			members = v
		case []interface{}:
			for _, item := range v {
				if str, ok := item.(string); ok {
					members = append(members, str)
				}
			}
		}
	}

	if len(members) > 0 {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			// 在事务中直接更新仓库基本信息
			if err := tx.Model(&model.Repository{}).Where("name = ?", name).Updates(updates).Error; err != nil {
				return err
			}

			var virtualRepo model.Repository
			if err := tx.Where("name = ?", name).First(&virtualRepo).Error; err != nil {
				return err
			}

			if err := tx.Where("virtual_repo_id = ?", virtualRepo.ID).Delete(&model.RepositoryGroup{}).Error; err != nil {
				return err
			}

			for i, memberName := range members {
				var memberRepo model.Repository
				if err := tx.Where("name = ?", memberName).First(&memberRepo).Error; err != nil {
					return fmt.Errorf("member repository not found: %s", memberName)
				}
				group := model.RepositoryGroup{
					VirtualRepoID: virtualRepo.ID,
					MemberRepoID:  memberRepo.ID,
					Priority:      i,
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

	err := s.repoRepo.Update(name, updates)
	if err == nil {
		s.invalidateCache(name)
	}
	return err
}

// Delete 删除仓库
func (s *RepositoryService) Delete(name string) error {
	err := s.repoRepo.Delete(name)
	if err == nil {
		s.invalidateCache(name)
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
func (s *RepositoryService) GetMembers(virtualRepoName string) ([]model.RepositoryGroup, error) {
	return s.GetMembersContext(context.Background(), virtualRepoName)
}

// GetMembersContext 获取虚拟仓库的所有成员
func (s *RepositoryService) GetMembersContext(ctx context.Context, virtualRepoName string) ([]model.RepositoryGroup, error) {
	virtualRepo, err := s.repoRepo.FindByNameContext(ctx, virtualRepoName)
	if err != nil {
		return nil, err
	}
	return s.groupRepo.GetMembersByVirtualRepoContext(ctx, virtualRepo.ID)
}
