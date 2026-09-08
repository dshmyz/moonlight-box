package service

import (
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/cache"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
)

type SystemConfigService struct {
	configRepo *repository.SystemConfigRepository
	cache      *cache.MemoryCache
}

const systemConfigCacheTTL = 5 * time.Minute

func NewSystemConfigService(configRepo *repository.SystemConfigRepository) *SystemConfigService {
	return &SystemConfigService{
		configRepo: configRepo,
		cache:      cache.NewMemoryCache(),
	}
}

// ConfigCache 暴露系统配置缓存供 main.go 注册进 CacheManager（管理页可见/可清空）。
func (s *SystemConfigService) ConfigCache() *cache.MemoryCache {
	return s.cache
}

func (s *SystemConfigService) Get(key string) (*model.SystemConfig, error) {
	if v, ok := s.cache.Get(key); ok {
		if config, ok := v.(*model.SystemConfig); ok {
			// 返回副本，防止调用方修改影响缓存内的共享对象（与旧 map 实现一致）
			cp := *config
			return &cp, nil
		}
	}

	config, err := s.configRepo.Get(key)
	if err != nil {
		return nil, err
	}
	s.cacheConfig(config)
	return config, nil
}

func (s *SystemConfigService) GetAll() ([]model.SystemConfig, error) {
	configs, err := s.configRepo.List()
	if err != nil {
		return nil, err
	}

	for i := range configs {
		config := configs[i]
		s.cache.Set(config.Key, &config, systemConfigCacheTTL)
	}
	return configs, nil
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
	if err := s.configRepo.Set(config); err != nil {
		return err
	}
	s.cacheConfig(config)
	return nil
}

func (s *SystemConfigService) Delete(key string) error {
	if err := s.configRepo.Delete(key); err != nil {
		return err
	}
	s.cache.Delete(key)
	return nil
}

func (s *SystemConfigService) cacheConfig(config *model.SystemConfig) {
	if config == nil {
		return
	}
	s.cache.Set(config.Key, config, systemConfigCacheTTL)
}
