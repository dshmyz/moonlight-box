package service

import (
	"context"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
)

type StorageBackendService struct {
	repo *repository.StorageBackendRepository
}

func NewStorageBackendService(repo *repository.StorageBackendRepository) *StorageBackendService {
	return &StorageBackendService{repo: repo}
}

func (s *StorageBackendService) List() ([]model.StorageBackendDTO, error) {
	backends, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	result := make([]model.StorageBackendDTO, 0, len(backends))
	for _, b := range backends {
		result = append(result, b.ToDTO())
	}
	return result, nil
}

func (s *StorageBackendService) GetByID(id uint) (*model.StorageBackendDTO, error) {
	backend, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	dto := backend.ToDTO()
	return &dto, nil
}

func (s *StorageBackendService) Create(backend *model.StorageBackend) (*model.StorageBackendDTO, error) {
	exists, err := s.repo.Exists(backend.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("storage backend %q already exists", backend.Name)
	}

	if err := s.repo.Create(backend); err != nil {
		return nil, err
	}

	dto := backend.ToDTO()
	return &dto, nil
}

func (s *StorageBackendService) Update(backend *model.StorageBackend) (*model.StorageBackendDTO, error) {
	existing, err := s.repo.FindByID(backend.ID)
	if err != nil {
		return nil, err
	}

	if existing.Name != backend.Name {
		exists, _ := s.repo.Exists(backend.Name)
		if exists {
			return nil, fmt.Errorf("storage backend %q already exists", backend.Name)
		}
	}

	if err := s.repo.Update(backend); err != nil {
		return nil, err
	}

	dto := backend.ToDTO()
	return &dto, nil
}

func (s *StorageBackendService) Delete(id uint) error {
	backend, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	if backend.IsDefault {
		return fmt.Errorf("cannot delete default storage backend")
	}
	return s.repo.Delete(id)
}

func (s *StorageBackendService) SetDefault(id uint) error {
	_, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	return s.repo.SetDefault(id)
}

func (s *StorageBackendService) GetDefault() (*model.StorageBackendDTO, error) {
	backend, err := s.repo.FindDefault()
	if err != nil {
		return nil, err
	}
	dto := backend.ToDTO()
	return &dto, nil
}

func (s *StorageBackendService) TestConnection(backend *model.StorageBackend) error {
	storageBackend, err := CreateStorageBackend(backend)
	if err != nil {
		return err
	}

	ctx := context.Background()
	_, err = storageBackend.Exists(ctx, "test-connection")
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	return nil
}
