package repository

import (
	"github.com/moonlight-box/registry/internal/model"

	"gorm.io/gorm"
)

type RoleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

func (r *RoleRepository) FindByID(id uint) (*model.Role, error) {
	var role model.Role
	result := r.db.Preload("Permissions").First(&role, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &role, nil
}

func (r *RoleRepository) FindByName(name string) (*model.Role, error) {
	var role model.Role
	result := r.db.Preload("Permissions").Where("name = ?", name).First(&role)
	if result.Error != nil {
		return nil, result.Error
	}
	return &role, nil
}

func (r *RoleRepository) List() ([]model.Role, error) {
	var roles []model.Role
	result := r.db.Preload("Permissions").Find(&roles)
	return roles, result.Error
}

func (r *RoleRepository) GetUserRoles(userID uint) ([]model.Role, error) {
	var roles []model.Role
	result := r.db.
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Find(&roles)
	return roles, result.Error
}

func (r *RoleRepository) GetUserPermissions(userID uint) ([]model.Permission, error) {
	var perms []model.Permission
	result := r.db.
		Distinct().
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id").
		Where("user_roles.user_id = ?", userID).
		Find(&perms)
	return perms, result.Error
}

func (r *RoleRepository) AssignRole(userID, roleID uint, assignedBy uint) error {
	userRole := model.UserRole{
		UserID:     userID,
		RoleID:     roleID,
		AssignedBy: assignedBy,
	}
	return r.db.Where(userRole).FirstOrCreate(&userRole).Error
}

func (r *RoleRepository) RemoveRole(userID, roleID uint) error {
	return r.db.Where("user_id = ? AND role_id = ?", userID, roleID).Delete(&model.UserRole{}).Error
}
