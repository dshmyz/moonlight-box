package repository

import (
	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type StorageBackendRepository struct {
	db *gorm.DB
}

func NewStorageBackendRepository(db *gorm.DB) *StorageBackendRepository {
	return &StorageBackendRepository{db: db}
}

func (r *StorageBackendRepository) Create(backend *model.StorageBackend) error {
	return r.db.Create(backend).Error
}

func (r *StorageBackendRepository) Update(backend *model.StorageBackend) error {
	return r.db.Save(backend).Error
}

func (r *StorageBackendRepository) Delete(id uint) error {
	return r.db.Delete(&model.StorageBackend{}, id).Error
}

func (r *StorageBackendRepository) FindByID(id uint) (*model.StorageBackend, error) {
	var backend model.StorageBackend
	err := r.db.First(&backend, id).Error
	if err != nil {
		return nil, err
	}
	return &backend, nil
}

func (r *StorageBackendRepository) FindByName(name string) (*model.StorageBackend, error) {
	var backend model.StorageBackend
	err := r.db.Where("name = ?", name).First(&backend).Error
	if err != nil {
		return nil, err
	}
	return &backend, nil
}

func (r *StorageBackendRepository) List() ([]model.StorageBackend, error) {
	var backends []model.StorageBackend
	err := r.db.Order("is_default DESC, created_at DESC").Find(&backends).Error
	return backends, err
}

func (r *StorageBackendRepository) FindDefault() (*model.StorageBackend, error) {
	var backend model.StorageBackend
	err := r.db.Where("is_default = ? AND is_active = ?", true, true).First(&backend).Error
	if err != nil {
		return nil, err
	}
	return &backend, nil
}

func (r *StorageBackendRepository) SetDefault(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.StorageBackend{}).UpdateColumn("is_default", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.StorageBackend{}).Where("id = ?", id).UpdateColumn("is_default", true).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *StorageBackendRepository) Exists(name string) (bool, error) {
	var count int64
	err := r.db.Model(&model.StorageBackend{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}
