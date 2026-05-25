package proxy

import (
	"context"
	"sync"
	"time"

	"log/slog"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
)

type RepositoryCache struct {
	mu        sync.RWMutex
	repos     map[string]*repositoryCacheEntry
	reposByID map[uint]*repositoryCacheEntry
	members   map[uint][]*memberCacheEntry
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	ttl       time.Duration
	stopCh    chan struct{}
}

type repositoryCacheEntry struct {
	repo      *model.Repository
	expiresAt time.Time
}

type memberCacheEntry struct {
	member    *model.RepositoryGroup
	expiresAt time.Time
}

func NewRepositoryCache(repoRepo *repository.RepositoryRepository, groupRepo *repository.GroupRepository, ttl time.Duration) *RepositoryCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	return &RepositoryCache{
		repos:     make(map[string]*repositoryCacheEntry),
		reposByID: make(map[uint]*repositoryCacheEntry),
		members:   make(map[uint][]*memberCacheEntry),
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		ttl:       ttl,
		stopCh:    make(chan struct{}),
	}
}

func (c *RepositoryCache) GetByName(name string) (*model.Repository, error) {
	return c.GetByNameContext(context.Background(), name)
}

func (c *RepositoryCache) GetByNameContext(ctx context.Context, name string) (*model.Repository, error) {
	c.mu.RLock()
	entry, exists := c.repos[name]
	c.mu.RUnlock()

	if exists && time.Now().Before(entry.expiresAt) {
		return entry.repo, nil
	}

	repo, err := c.repoRepo.FindByNameContext(ctx, name)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.setCacheEntry(repo)
	c.mu.Unlock()

	slog.Debug("Cached repository",
		"module", "repository_cache",
		"repo_name", name,
		"repo_id", repo.ID,
	)

	return repo, nil
}

func (c *RepositoryCache) GetByID(id uint) (*model.Repository, error) {
	return c.GetByIDContext(context.Background(), id)
}

func (c *RepositoryCache) GetByIDContext(ctx context.Context, id uint) (*model.Repository, error) {
	c.mu.RLock()
	entry, exists := c.reposByID[id]
	c.mu.RUnlock()

	if exists && time.Now().Before(entry.expiresAt) {
		return entry.repo, nil
	}

	repo, err := c.repoRepo.FindByIDContext(ctx, id)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.setCacheEntry(repo)
	c.mu.Unlock()

	slog.Debug("Cached repository by ID",
		"module", "repository_cache",
		"repo_id", repo.ID,
		"repo_name", repo.Name,
	)

	return repo, nil
}

func (c *RepositoryCache) setCacheEntry(repo *model.Repository) {
	entry := &repositoryCacheEntry{
		repo:      repo,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.repos[repo.Name] = entry
	c.reposByID[repo.ID] = entry
}

func (c *RepositoryCache) GetVirtualRepo(pkgType string) (*model.Repository, error) {
	return c.GetVirtualRepoContext(context.Background(), pkgType)
}

func (c *RepositoryCache) GetVirtualRepoContext(ctx context.Context, pkgType string) (*model.Repository, error) {
	c.mu.RLock()
	for _, entry := range c.repos {
		if entry.repo.Type == model.RepoTypeVirtual && entry.repo.PackageType == pkgType && entry.repo.Enabled {
			if time.Now().Before(entry.expiresAt) {
				return entry.repo, nil
			}
		}
	}
	c.mu.RUnlock()

	repo, err := c.repoRepo.FindVirtualByPackageTypeContext(ctx, pkgType)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.setCacheEntry(repo)
	c.mu.Unlock()

	slog.Debug("Cached virtual repository",
		"module", "repository_cache",
		"pkg_type", pkgType,
		"repo_id", repo.ID,
	)

	return repo, nil
}

func (c *RepositoryCache) GetMembers(virtualRepoID uint) ([]model.RepositoryGroup, error) {
	return c.GetMembersContext(context.Background(), virtualRepoID)
}

func (c *RepositoryCache) GetMembersContext(ctx context.Context, virtualRepoID uint) ([]model.RepositoryGroup, error) {
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

	members, err := c.groupRepo.GetMembersByVirtualRepoContext(ctx, virtualRepoID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	var entriesToCache []*memberCacheEntry
	for i := range members {
		memberCopy := members[i]
		entriesToCache = append(entriesToCache, &memberCacheEntry{
			member:    &memberCopy,
			expiresAt: time.Now().Add(c.ttl),
		})
	}
	c.members[virtualRepoID] = entriesToCache
	c.mu.Unlock()

	slog.Debug("Cached virtual repository members",
		"module", "repository_cache",
		"virtual_repo_id", virtualRepoID,
		"member_count", len(members),
	)

	return members, nil
}

func (c *RepositoryCache) Invalidate(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if name == "*" {
		c.repos = make(map[string]*repositoryCacheEntry)
		c.reposByID = make(map[uint]*repositoryCacheEntry)
		c.members = make(map[uint][]*memberCacheEntry)
		slog.Info("Invalidated all repository cache",
			"module", "repository_cache",
		)
		return
	}

	if entry, exists := c.repos[name]; exists {
		delete(c.repos, name)
		delete(c.reposByID, entry.repo.ID)
		delete(c.members, entry.repo.ID)
		slog.Info("Invalidated repository cache",
			"module", "repository_cache",
			"repo_name", name,
		)
	}
}

func (c *RepositoryCache) InvalidateByID(id uint) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.reposByID[id]; exists {
		delete(c.repos, entry.repo.Name)
		delete(c.reposByID, id)
		delete(c.members, id)
		slog.Info("Invalidated repository cache by ID",
			"module", "repository_cache",
			"repo_id", id,
			"repo_name", entry.repo.Name,
		)
	}
}

func (c *RepositoryCache) TTL() time.Duration {
	return c.ttl
}

func (c *RepositoryCache) StartCleanup(interval time.Duration) {
	if interval == 0 {
		interval = 1 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.cleanup()
			case <-c.stopCh:
				c.cleanup()
				return
			}
		}
	}()
}

func (c *RepositoryCache) Stop() {
	close(c.stopCh)
}

func (c *RepositoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	for name, entry := range c.repos {
		if now.After(entry.expiresAt) {
			delete(c.repos, name)
			delete(c.reposByID, entry.repo.ID)
		}
	}

	for repoID, entries := range c.members {
		var validEntries []*memberCacheEntry
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
