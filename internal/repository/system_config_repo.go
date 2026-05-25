package repository

import (
	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/gorm"
)

type SystemConfigRepository struct {
	db *gorm.DB
}

func NewSystemConfigRepository(db *gorm.DB) *SystemConfigRepository {
	return &SystemConfigRepository{db: db}
}

func (r *SystemConfigRepository) Get(key string) (*model.SystemConfig, error) {
	var config model.SystemConfig
	err := r.db.Where("key = ?", key).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *SystemConfigRepository) GetByCategory(category string) ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := r.db.Where("category = ?", category).Find(&configs).Error
	return configs, err
}

func (r *SystemConfigRepository) List() ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := r.db.Order("category, key").Find(&configs).Error
	return configs, err
}

func (r *SystemConfigRepository) Set(config *model.SystemConfig) error {
	var existing model.SystemConfig
	result := r.db.Where("key = ?", config.Key).First(&existing)

	if result.Error == gorm.ErrRecordNotFound {
		return r.db.Create(config).Error
	}

	return r.db.Model(&existing).Updates(map[string]interface{}{
		"value":        config.Value,
		"value_type":   config.ValueType,
		"category":     config.Category,
		"description":  config.Description,
		"is_sensitive": config.IsSensitive,
		"updated_by":   config.UpdatedBy,
	}).Error
}

func (r *SystemConfigRepository) Delete(key string) error {
	return r.db.Where("key = ?", key).Delete(&model.SystemConfig{}).Error
}
