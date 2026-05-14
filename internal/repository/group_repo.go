package repository

import (
	"context"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

// GroupRepository 虚拟仓成员关系数据访问层
type GroupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

// AddMember 向虚拟仓添加成员
func (r *GroupRepository) AddMember(virtualRepoID, memberRepoID uint, priority int) error {
	group := model.RepositoryGroup{
		VirtualRepoID: virtualRepoID,
		MemberRepoID:  memberRepoID,
		Priority:      priority,
	}
	return r.db.Create(&group).Error
}

// RemoveMember 从虚拟仓移除成员
func (r *GroupRepository) RemoveMember(virtualRepoID, memberRepoID uint) error {
	return r.db.Where("virtual_repo_id = ? AND member_repo_id = ?",
		virtualRepoID, memberRepoID).Delete(&model.RepositoryGroup{}).Error
}

// GetMembersByVirtualRepo 获取虚拟仓的所有成员
func (r *GroupRepository) GetMembersByVirtualRepo(virtualRepoID uint) ([]model.RepositoryGroup, error) {
	return r.GetMembersByVirtualRepoContext(context.Background(), virtualRepoID)
}

// GetMembersByVirtualRepoContext 获取虚拟仓的所有成员
func (r *GroupRepository) GetMembersByVirtualRepoContext(ctx context.Context, virtualRepoID uint) ([]model.RepositoryGroup, error) {
	var groups []model.RepositoryGroup
	err := r.db.WithContext(ctx).Where("virtual_repo_id = ?", virtualRepoID).
		Preload("MemberRepo").
		Order("priority ASC").
		Find(&groups).Error
	return groups, err
}

// UpdatePriority 更新成员优先级
func (r *GroupRepository) UpdatePriority(virtualRepoID, memberRepoID uint, priority int) error {
	return r.db.Model(&model.RepositoryGroup{}).
		Where("virtual_repo_id = ? AND member_repo_id = ?", virtualRepoID, memberRepoID).
		Update("priority", priority).Error
}
