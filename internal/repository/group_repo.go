package repository

import (
	"context"

	"github.com/dshmyz/moonlight-box/internal/model"
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
func (r *GroupRepository) AddMember(virtualRepoID, memberRepoID uint, position int) error {
	member := model.RepositoryMember{
		RepositoryID: virtualRepoID,
		MemberID:     memberRepoID,
		Position:     position,
	}
	return r.db.Create(&member).Error
}

// RemoveMember 从虚拟仓移除成员
func (r *GroupRepository) RemoveMember(virtualRepoID, memberRepoID uint) error {
	return r.db.Where("repository_id = ? AND member_id = ?",
		virtualRepoID, memberRepoID).Delete(&model.RepositoryMember{}).Error
}

// GetMembersByVirtualRepo 获取虚拟仓的所有成员
func (r *GroupRepository) GetMembersByVirtualRepo(virtualRepoID uint) ([]model.RepositoryMember, error) {
	return r.GetMembersByVirtualRepoContext(context.Background(), virtualRepoID)
}

// GetMembersByVirtualRepoContext 获取虚拟仓的所有成员
func (r *GroupRepository) GetMembersByVirtualRepoContext(ctx context.Context, virtualRepoID uint) ([]model.RepositoryMember, error) {
	var members []model.RepositoryMember
	err := r.db.WithContext(ctx).Where("repository_id = ?", virtualRepoID).
		Preload("MemberRepo").
		Order("position ASC").
		Find(&members).Error
	return members, err
}

// GetParentVirtualRepos 查询包含指定成员仓库的所有虚拟仓库
func (r *GroupRepository) GetParentVirtualRepos(memberRepoID uint) ([]model.Repository, error) {
	var repos []model.Repository
	err := r.db.Distinct("repositories.*").
		Joins("JOIN repository_members ON repository_members.repository_id = repositories.id").
		Where("repository_members.member_id = ? AND repositories.type = ?", memberRepoID, model.RepoTypeVirtual).
		Find(&repos).Error
	return repos, err
}

// UpdatePosition 更新成员顺序
func (r *GroupRepository) UpdatePosition(virtualRepoID, memberRepoID uint, position int) error {
	return r.db.Model(&model.RepositoryMember{}).
		Where("repository_id = ? AND member_id = ?", virtualRepoID, memberRepoID).
		Update("position", position).Error
}
