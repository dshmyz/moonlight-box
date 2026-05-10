package service

import (
	"fmt"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
)

type PermissionCacheService struct {
	cache    *cache.MemoryCache
	roleRepo *repository.RoleRepository
	ttl      time.Duration
}

func NewPermissionCacheService(roleRepo *repository.RoleRepository, ttl time.Duration) *PermissionCacheService {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	return &PermissionCacheService{
		cache:    cache.NewMemoryCache(),
		roleRepo: roleRepo,
		ttl:      ttl,
	}
}

func (s *PermissionCacheService) GetUserPermissions(userID uint) ([]model.Permission, error) {
	cacheKey := fmt.Sprintf("permissions:user:%d", userID)

	if cached, ok := s.cache.Get(cacheKey); ok {
		if perms, ok := cached.([]model.Permission); ok {
			return perms, nil
		}
	}

	perms, err := s.roleRepo.GetUserPermissions(userID)
	if err != nil {
		return nil, err
	}

	s.cache.Set(cacheKey, perms, s.ttl)

	return perms, nil
}

func (s *PermissionCacheService) InvalidateUser(userID uint) {
	cacheKey := fmt.Sprintf("permissions:user:%d", userID)
	s.cache.Delete(cacheKey)
}

func (s *PermissionCacheService) InvalidateAll() {
	s.cache.Clear()
}

func (s *PermissionCacheService) GetStats() map[string]interface{} {
	return s.cache.Stats()
}

func (s *PermissionCacheService) GetTTL() time.Duration {
	return s.ttl
}

func (s *PermissionCacheService) GetAllItems() map[string]*cache.Item {
	return s.cache.GetAllItems()
}
