package repository

import (
	"encoding/json"

	"github.com/dshmyz/moonlight-box/internal/model"

	"gorm.io/gorm"
)

type CASConfigRepository struct {
	db *gorm.DB
}

func NewCASConfigRepository(db *gorm.DB) *CASConfigRepository {
	return &CASConfigRepository{db: db}
}

const (
	casConfigKey = "cas_config"
)

func (r *CASConfigRepository) GetConfig() (*model.CASConfig, error) {
	var config model.SystemConfig
	result := r.db.Where("key = ?", casConfigKey).First(&config)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}

	var casConfig model.CASConfig
	if err := json.Unmarshal([]byte(config.Value), &casConfig); err != nil {
		return nil, err
	}

	return &casConfig, nil
}

func (r *CASConfigRepository) SaveConfig(config *model.CASConfig, userID *uint) error {
	value, err := json.Marshal(config)
	if err != nil {
		return err
	}

	sysConfig := model.SystemConfig{
		Key:         casConfigKey,
		Value:       string(value),
		ValueType:   "json",
		Description: "CAS 单点登录配置",
		UpdatedBy:   userID,
	}

	return r.db.Where("key = ?", casConfigKey).
		Assign(model.SystemConfig{
			Value:     string(value),
			UpdatedBy: userID,
		}).
		FirstOrCreate(&sysConfig).Error
}

func (r *CASConfigRepository) DeleteConfig() error {
	return r.db.Where("key = ?", casConfigKey).Delete(&model.SystemConfig{}).Error
}
