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

func (r *RoleRepository) Create(role *model.Role) error {
	return r.db.Create(role).Error
}

func (r *RoleRepository) Update(role *model.Role) error {
	return r.db.Save(role).Error
}

func (r *RoleRepository) Delete(id uint) error {
	return r.db.Delete(&model.Role{}, id).Error
}

func (r *RoleRepository) IsRoleInUse(roleID uint) (bool, error) {
	var count int64
	err := r.db.Model(&model.UserRole{}).Where("role_id = ?", roleID).Count(&count).Error
	return count > 0, err
}

func (r *RoleRepository) UpdateRolePermissions(roleID uint, permissionIDs []uint) error {
	if err := r.db.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
		return err
	}
	for _, permID := range permissionIDs {
		rp := model.RolePermission{RoleID: roleID, PermissionID: permID}
		if err := r.db.Create(&rp).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *RoleRepository) ListPermissions() ([]model.Permission, error) {
	var perms []model.Permission
	result := r.db.Find(&perms)
	return perms, result.Error
}
