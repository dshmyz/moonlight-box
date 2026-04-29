package service

import (
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
)

type CASConfigService struct {
	casConfigRepo *repository.CASConfigRepository
}

func NewCASConfigService(repo *repository.CASConfigRepository) *CASConfigService {
	return &CASConfigService{casConfigRepo: repo}
}

func (s *CASConfigService) GetConfig() (*model.CASConfig, error) {
	config, err := s.casConfigRepo.GetConfig()
	if err != nil {
		return nil, err
	}
	if config == nil {
		return &model.CASConfig{
			LoginPath:    "/cas/login",
			ValidatePath: "/cas/serviceValidate",
		}, nil
	}
	return config, nil
}

func (s *CASConfigService) SaveConfig(config *model.CASConfig, userID *uint) error {
	return s.casConfigRepo.SaveConfig(config, userID)
}

func (s *CASConfigService) DeleteConfig() error {
	return s.casConfigRepo.DeleteConfig()
}
