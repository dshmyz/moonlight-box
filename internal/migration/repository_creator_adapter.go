package migration

import (
	"github.com/moonlight-box/registry/internal/model"
)

type RepositoryCreatorAdapter struct {
	repoRepo           interface {
		Create(repo *model.Repository) error
		FindByName(name string) (*model.Repository, error)
	}
	storageBackendRepo interface {
		FindDefault() (*model.StorageBackend, error)
	}
}

func NewRepositoryCreatorAdapter(repoRepo interface {
	Create(repo *model.Repository) error
	FindByName(name string) (*model.Repository, error)
}, storageBackendRepo interface {
	FindDefault() (*model.StorageBackend, error)
}) *RepositoryCreatorAdapter {
	return &RepositoryCreatorAdapter{
		repoRepo:           repoRepo,
		storageBackendRepo: storageBackendRepo,
	}
}

func (a *RepositoryCreatorAdapter) CreateRepo(name, repoType, packageType string, remoteURL string, cacheEnabled bool, cacheTTLSeconds int, storageBackendID *uint) error {
	repo := &model.Repository{
		Name:             name,
		Type:             model.RepositoryType(repoType),
		PackageType:      packageType,
		Enabled:          true,
		StorageBackendID: storageBackendID,
	}

	if repoType == "proxy" && remoteURL != "" {
		repo.RemoteURL = remoteURL
		repo.CacheEnabled = cacheEnabled
		repo.CacheTTLSeconds = cacheTTLSeconds
	}

	if repoType == "virtual" {
		repo.CacheEnabled = false
		repo.StorageBackendID = nil
	}

	return a.repoRepo.Create(repo)
}

func (a *RepositoryCreatorAdapter) RepoExists(name string) bool {
	_, err := a.repoRepo.FindByName(name)
	return err == nil
}

func (a *RepositoryCreatorAdapter) FindDefaultStorageBackendID() (*uint, error) {
	if a.storageBackendRepo == nil {
		return nil, nil
	}
	backend, err := a.storageBackendRepo.FindDefault()
	if err != nil {
		return nil, err
	}
	return &backend.ID, nil
}
