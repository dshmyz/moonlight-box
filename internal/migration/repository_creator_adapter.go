package migration

import (
	"errors"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"
)

type RepoCreatorRepoRepo interface {
	Create(repo *model.Repository) error
	FindByName(name string) (*model.Repository, error)
}

type RepoCreatorStorageBackendRepo interface {
	FindDefault() (*model.StorageBackend, error)
}

type RepositoryCreatorAdapter struct {
	repoRepo           RepoCreatorRepoRepo
	storageBackendRepo RepoCreatorStorageBackendRepo
}

func NewRepositoryCreatorAdapter(repoRepo RepoCreatorRepoRepo, storageBackendRepo RepoCreatorStorageBackendRepo) *RepositoryCreatorAdapter {
	return &RepositoryCreatorAdapter{
		repoRepo:           repoRepo,
		storageBackendRepo: storageBackendRepo,
	}
}

func (a *RepositoryCreatorAdapter) CreateRepo(name, repoType, packageType string, remoteURL string, cacheEnabled bool, cacheTTLSeconds int, storageBackendID *uint) error {
	return a.CreateRepoWithConfig(name, repoType, packageType, remoteURL, cacheEnabled, cacheTTLSeconds, storageBackendID, nil, 0, 0, false)
}

func (a *RepositoryCreatorAdapter) CreateRepoWithConfig(name, repoType, packageType string, remoteURL string, cacheEnabled bool, cacheTTLSeconds int, storageBackendID *uint, authConfig *model.ProxyAuthConfig, timeoutSeconds, maxRedirects int, insecureSkipVerify bool) error {
	repo := &model.Repository{
		Name:             name,
		Type:             model.RepositoryType(repoType),
		PackageType:      packageType,
		Enabled:          true,
		StorageBackendID: storageBackendID,
	}

	if repoType == "proxy" && remoteURL != "" {
		repo.Config = &model.RepositoryConfig{
			RemoteURL:          remoteURL,
			CacheEnabled:       cacheEnabled,
			CacheTTLSeconds:    cacheTTLSeconds,
			CacheNegativeTTL:   300,
			TimeoutSeconds:     timeoutSeconds,
			MaxRedirects:       maxRedirects,
			InsecureSkipVerify: insecureSkipVerify,
		}
		if authConfig != nil && authConfig.Type != "" && authConfig.Type != "none" {
			repo.Config.Auth = authConfig
			if authConfig.Type == "username" {
				repo.Config.AuthType = "basic"
			} else {
				repo.Config.AuthType = authConfig.Type
			}
		}
	}

	if repoType == "virtual" {
		repo.StorageBackendID = nil
	}

	return a.repoRepo.Create(repo)
}

func (a *RepositoryCreatorAdapter) RepoExists(name string) (bool, error) {
	_, err := a.repoRepo.FindByName(name)
	if err != nil {
		if errors.Is(err, util.ErrRepoNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
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
