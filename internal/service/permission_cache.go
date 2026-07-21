package service

import (
	"fmt"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/cache"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
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
	s.cache.Delete(fmt.Sprintf("permissions:user:%d", userID))
	s.cache.Delete(fmt.Sprintf("permset:user:%d", userID))
}

func (s *PermissionCacheService) InvalidateAll() {
	s.cache.Clear()
}

func (s *PermissionCacheService) GetStats() map[string]interface{} {
	return s.cache.Stats()
}

func (s *PermissionCacheService) GetAllItems() map[string]*cache.Item {
	return s.cache.GetAllItems()
}

// GetUserPermissionSet returns a permission lookup map "resource:action" -> bool.
// O(1) check instead of linear scan over permission list.
func (s *PermissionCacheService) GetUserPermissionSet(userID uint) (map[string]bool, error) {
	cacheKey := fmt.Sprintf("permset:user:%d", userID)

	if cached, ok := s.cache.Get(cacheKey); ok {
		if permSet, ok := cached.(map[string]bool); ok {
			return permSet, nil
		}
	}

	perms, err := s.roleRepo.GetUserPermissions(userID)
	if err != nil {
		return nil, err
	}

	permSet := make(map[string]bool, len(perms))
	for _, p := range perms {
		permSet[p.Resource+":"+p.Action] = true
	}
	// admin always passes
	permSet["system:admin"] = permSet["system:admin"]

	s.cache.Set(cacheKey, permSet, s.ttl)
	return permSet, nil
}

func (s *PermissionCacheService) HasPermission(userID uint, resource, action string) (bool, error) {
	permSet, err := s.GetUserPermissionSet(userID)
	if err != nil {
		return false, err
	}
	return permSet[resource+":"+action] || permSet["system:admin"], nil
}
