package service

import (
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
)

type SystemConfigService struct {
	configRepo *repository.SystemConfigRepository
}

func NewSystemConfigService(configRepo *repository.SystemConfigRepository) *SystemConfigService {
	return &SystemConfigService{
		configRepo: configRepo,
	}
}

func (s *SystemConfigService) Get(key string) (*model.SystemConfig, error) {
	return s.configRepo.Get(key)
}

func (s *SystemConfigService) GetAll() ([]model.SystemConfig, error) {
	return s.configRepo.List()
}

func (s *SystemConfigService) Set(key, value, valueType, category, description string, isSensitive bool, updatedBy uint) error {
	config := &model.SystemConfig{
		Key:         key,
		Value:       value,
		ValueType:   valueType,
		Category:    category,
		Description: description,
		IsSensitive: isSensitive,
		UpdatedBy:   &updatedBy,
	}
	return s.configRepo.Set(config)
}

func (s *SystemConfigService) Delete(key string) error {
	return s.configRepo.Delete(key)
}
