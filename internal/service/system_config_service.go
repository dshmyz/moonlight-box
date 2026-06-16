package service

import (
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
)

type SystemConfigService struct {
	configRepo *repository.SystemConfigRepository
	mu         sync.RWMutex
	cache      map[string]systemConfigCacheEntry
}

const systemConfigCacheTTL = 5 * time.Minute

type systemConfigCacheEntry struct {
	config    model.SystemConfig
	expiresAt time.Time
}

func NewSystemConfigService(configRepo *repository.SystemConfigRepository) *SystemConfigService {
	return &SystemConfigService{
		configRepo: configRepo,
		cache:      make(map[string]systemConfigCacheEntry),
	}
}

func (s *SystemConfigService) Get(key string) (*model.SystemConfig, error) {
	now := time.Now()
	s.mu.RLock()
	entry, ok := s.cache[key]
	s.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		config := entry.config
		return &config, nil
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

	expiresAt := time.Now().Add(systemConfigCacheTTL)
	s.mu.Lock()
	for i := range configs {
		s.cache[configs[i].Key] = systemConfigCacheEntry{
			config:    configs[i],
			expiresAt: expiresAt,
		}
	}
	s.mu.Unlock()
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
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()
	return nil
}

func (s *SystemConfigService) cacheConfig(config *model.SystemConfig) {
	if config == nil {
		return
	}
	s.mu.Lock()
	s.cache[config.Key] = systemConfigCacheEntry{
		config:    *config,
		expiresAt: time.Now().Add(systemConfigCacheTTL),
	}
	s.mu.Unlock()
}
