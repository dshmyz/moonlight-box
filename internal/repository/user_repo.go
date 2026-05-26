package repository

import (
	"errors"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(user *model.User) error {
	result := r.db.Create(user)
	return result.Error
}

func (r *UserRepository) FindByID(id uint) (*model.User, error) {
	var user model.User
	result := r.db.Preload("Roles").First(&user, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrUserNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}

func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	result := r.db.Preload("Roles").Where("username = ?", username).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrUserNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}

func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var user model.User
	result := r.db.Preload("Roles").Where("email = ?", email).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, util.ErrUserNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}

func (r *UserRepository) Update(user *model.User) error {
	return r.db.Save(user).Error
}

func (r *UserRepository) List(page, pageSize int, keyword string, isActive *bool) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	query := r.db.Model(&model.User{}).Preload("Roles")

	if keyword != "" {
		query = query.Where("username LIKE ? OR email LIKE ? OR display_name LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	result := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&users)

	return users, total, result.Error
}

func (r *UserRepository) Delete(id uint) error {
	return r.db.Delete(&model.User{}, id).Error
}

func (r *UserRepository) UpdateLastLogin(id uint) error {
	now := time.Now()
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("last_login_at", now).Error
}

func (r *UserRepository) FindOrCreateCASUser(username string) (*model.User, error) {
	var user model.User
	result := r.db.Preload("Roles").Where("username = ?", username).First(&user)
	if result.Error == nil {
		if !user.IsActive {
			return nil, errors.New("account is disabled")
		}
		return &user, nil
	}

	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}

	newUser := &model.User{
		Username:     username,
		PasswordHash: "",
		Email:        username + "@cas.local",
		DisplayName:  username,
		AuthSource:   model.AuthSourceCAS,
		IsActive:     true,
	}
	if err := r.db.Create(newUser).Error; err != nil {
		return nil, err
	}

	return r.FindByUsername(username)
}

func (r *UserRepository) AssignRoles(userID uint, roleIDs []uint) error {
	for _, roleID := range roleIDs {
		userRole := model.UserRole{
			UserID: userID,
			RoleID: roleID,
		}
		if err := r.db.Where(userRole).FirstOrCreate(&userRole).Error; err != nil {
			return err
		}
	}
	return nil
}
