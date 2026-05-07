package proxy

import (
	"sync"
	"time"

	"log/slog"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
)

type VirtualRepoCache struct {
	mu        sync.RWMutex
	virtuals  map[string]*virtualRepoEntry
	members   map[uint][]*memberEntry
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	ttl       time.Duration
}

type virtualRepoEntry struct {
	repo      *model.Repository
	expiresAt time.Time
}

type memberEntry struct {
	member    *model.RepositoryGroup
	expiresAt time.Time
}

func NewVirtualRepoCache(repoRepo *repository.RepositoryRepository, groupRepo *repository.GroupRepository, ttl time.Duration) *VirtualRepoCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	return &VirtualRepoCache{
		virtuals:  make(map[string]*virtualRepoEntry),
		members:   make(map[uint][]*memberEntry),
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		ttl:       ttl,
	}
}

func (c *VirtualRepoCache) GetVirtualRepo(pkgType string) (*model.Repository, error) {
	c.mu.RLock()
	entry, exists := c.virtuals[pkgType]
	c.mu.RUnlock()

	if exists && time.Now().Before(entry.expiresAt) {
		return entry.repo, nil
	}

	repo, err := c.repoRepo.FindVirtualByPackageType(pkgType)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.virtuals[pkgType] = &virtualRepoEntry{
		repo:      repo,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	slog.Debug("Cached virtual repository",
		"module", "virtual_repo_cache",
		"pkg_type", pkgType,
		"repo_id", repo.ID,
	)

	return repo, nil
}

func (c *VirtualRepoCache) GetMembers(virtualRepoID uint) ([]model.RepositoryGroup, error) {
	c.mu.RLock()
	entries, exists := c.members[virtualRepoID]
	c.mu.RUnlock()

	if exists {
		var validMembers []model.RepositoryGroup
		allValid := true
		for _, entry := range entries {
			if time.Now().Before(entry.expiresAt) {
				validMembers = append(validMembers, *entry.member)
			} else {
				allValid = false
				break
			}
		}
		if allValid && len(validMembers) > 0 {
			return validMembers, nil
		}
	}

	members, err := c.groupRepo.GetMembersByVirtualRepo(virtualRepoID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	var entriesToCache []*memberEntry
	for i := range members {
		memberCopy := members[i]
		entriesToCache = append(entriesToCache, &memberEntry{
			member:    &memberCopy,
			expiresAt: time.Now().Add(c.ttl),
		})
	}
	c.members[virtualRepoID] = entriesToCache
	c.mu.Unlock()

	slog.Debug("Cached virtual repository members",
		"module", "virtual_repo_cache",
		"virtual_repo_id", virtualRepoID,
		"member_count", len(members),
	)

	return members, nil
}

func (c *VirtualRepoCache) Invalidate(pkgType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if pkgType == "*" {
		c.virtuals = make(map[string]*virtualRepoEntry)
		c.members = make(map[uint][]*memberEntry)
		slog.Info("Invalidated all virtual repo cache",
			"module", "virtual_repo_cache",
		)
		return
	}

	if repo, exists := c.virtuals[pkgType]; exists {
		delete(c.virtuals, pkgType)
		delete(c.members, repo.repo.ID)
		slog.Info("Invalidated virtual repo cache",
			"module", "virtual_repo_cache",
			"pkg_type", pkgType,
		)
	}
}

func (c *VirtualRepoCache) StartCleanup(interval time.Duration) {
	if interval == 0 {
		interval = 1 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			c.cleanup()
		}
	}()
}

func (c *VirtualRepoCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	for pkgType, entry := range c.virtuals {
		if now.After(entry.expiresAt) {
			delete(c.virtuals, pkgType)
		}
	}

	for repoID, entries := range c.members {
		var validEntries []*memberEntry
		for _, entry := range entries {
			if now.Before(entry.expiresAt) {
				validEntries = append(validEntries, entry)
			}
		}
		if len(validEntries) == 0 {
			delete(c.members, repoID)
		} else {
			c.members[repoID] = validEntries
		}
	}
}
