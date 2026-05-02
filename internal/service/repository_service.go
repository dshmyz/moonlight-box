package service

import (
	"encoding/json"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"gorm.io/gorm"
)

// RepositoryService 仓库管理服务层
type RepositoryService struct {
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	db        *gorm.DB
}

// NewRepositoryService 创建仓库管理服务实例
func NewRepositoryService(repoRepo *repository.RepositoryRepository, groupRepo *repository.GroupRepository, db *gorm.DB) *RepositoryService {
	return &RepositoryService{
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		db:        db,
	}
}

// Create 创建仓库，如果是虚拟仓则同时添加成员
func (s *RepositoryService) Create(repo *model.Repository, members []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 如果 PackageTypes 非空，取第一个值填充 PackageType（向后兼容）
		if repo.PackageTypes != "" && repo.PackageType == "" {
			types := parseJSONStringArray(repo.PackageTypes)
			if len(types) > 0 {
				repo.PackageType = types[0]
			}
		}

		if err := tx.Create(repo).Error; err != nil {
			return err
		}

		// 如果是虚拟仓库且提供了成员列表，则添加成员关系
		if repo.Type == model.RepoTypeVirtual && len(members) > 0 {
			for i, memberName := range members {
				memberRepo, err := s.repoRepo.FindByName(memberName)
				if err != nil {
					// 成员仓库不存在则跳过
					continue
				}
				s.groupRepo.AddMember(repo.ID, memberRepo.ID, i)
			}
		}

		return nil
	})
}

// List 列出仓库，支持按条件过滤
func (s *RepositoryService) List(filter map[string]interface{}) ([]model.Repository, error) {
	return s.repoRepo.List(filter)
}

// Get 根据名称获取仓库详情
func (s *RepositoryService) Get(name string) (*model.Repository, error) {
	return s.repoRepo.FindByName(name)
}

// GetByID 根据ID获取仓库详情
func (s *RepositoryService) GetByID(id uint) (*model.Repository, error) {
	return s.repoRepo.FindByID(id)
}

// Update 更新仓库信息
func (s *RepositoryService) Update(name string, updates map[string]interface{}) error {
	// 如果更新了 PackageTypes，同步更新 PackageType
	if packageTypes, ok := updates["package_types"].(string); ok && packageTypes != "" {
		types := parseJSONStringArray(packageTypes)
		if len(types) > 0 {
			updates["package_type"] = types[0]
		}
	}
	return s.repoRepo.Update(name, updates)
}

// Delete 删除仓库
func (s *RepositoryService) Delete(name string) error {
	return s.repoRepo.Delete(name)
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
	return s.groupRepo.AddMember(virtualRepo.ID, memberRepo.ID, priority)
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
	return s.groupRepo.RemoveMember(virtualRepo.ID, memberRepo.ID)
}

// GetMembers 获取虚拟仓库的所有成员
func (s *RepositoryService) GetMembers(virtualRepoName string) ([]model.RepositoryGroup, error) {
	virtualRepo, err := s.repoRepo.FindByName(virtualRepoName)
	if err != nil {
		return nil, err
	}
	return s.groupRepo.GetMembersByVirtualRepo(virtualRepo.ID)
}

// parseJSONStringArray 解析 JSON 字符串数组
func parseJSONStringArray(s string) []string {
	var result []string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}
