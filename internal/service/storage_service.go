package service

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"sync"

	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/storage"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

type StorageService struct {
	storageBackendRepo *repository.StorageBackendRepository
	defaultBackend     storage.Backend
	backendMap         map[uint]storage.Backend
	mutex              sync.RWMutex
	localBasePath      string
	localMaxSizeGB     int64
	backendLoader      singleflight.Group
}

func NewStorageService(storageBackendRepo *repository.StorageBackendRepository, localBasePath string, localMaxSizeGB int64) (*StorageService, error) {
	svc := &StorageService{
		storageBackendRepo: storageBackendRepo,
		backendMap:         make(map[uint]storage.Backend),
		localBasePath:      localBasePath,
		localMaxSizeGB:     localMaxSizeGB,
	}

	// 初始化默认存储后端
	defaultBackend, err := svc.initDefaultBackend()
	if err != nil {
		return nil, err
	}
	svc.defaultBackend = defaultBackend

	// 初始化已存在的存储后端
	if err := svc.initStorageBackends(); err != nil {
		return nil, err
	}

	return svc, nil
}

func (s *StorageService) SetDefaultBackendForTest(backend storage.Backend) {
	s.defaultBackend = backend
}

// GetDefaultBackendPath 返回默认存储后端的路径，实现 proxy.StorageChecker 接口
func (s *StorageService) GetDefaultBackendPath() (string, error) {
	if s.defaultBackend == nil {
		return "", fmt.Errorf("no default backend available")
	}
	return s.defaultBackend.BasePath(), nil
}

// CheckStoragePath 检查存储路径是否可用，实现 proxy.StorageChecker 接口
func (s *StorageService) CheckStoragePath(path string) error {
	return nil
}

func (s *StorageService) initDefaultBackend() (storage.Backend, error) {
	// 首先尝试从数据库获取默认存储后端
	defaultBackend, err := s.storageBackendRepo.FindDefault()
	if err == nil {
		return CreateStorageBackend(defaultBackend)
	}

	// 如果数据库中没有默认后端，则使用本地存储作为默认值
	return storage.NewLocalStorage(s.localBasePath, s.localMaxSizeGB)
}

func (s *StorageService) initStorageBackends() error {
	backends, err := s.storageBackendRepo.List()
	if err != nil {
		return err
	}

	for _, backend := range backends {
		storageBackend, err := CreateStorageBackend(&backend)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":       "storage",
				"backend_id":   backend.ID,
				"backend_name": backend.Name,
				"backend_type": backend.Type,
			}).Warn("Failed to initialize storage backend, skipping")
			continue
		}
		s.backendMap[backend.ID] = storageBackend
	}

	logrus.WithFields(logrus.Fields{
		"module":          "storage",
		"loaded_backends": len(s.backendMap),
	}).Info("Storage backends initialized")

	return nil
}

func (s *StorageService) GetBackend(backendID uint) (storage.Backend, error) {
	s.mutex.RLock()
	backend, exists := s.backendMap[backendID]
	s.mutex.RUnlock()

	if exists {
		return backend, nil
	}

	key := fmt.Sprintf("backend:%d", backendID)
	result, err, _ := s.backendLoader.Do(key, func() (interface{}, error) {
		s.mutex.RLock()
		backend, exists := s.backendMap[backendID]
		s.mutex.RUnlock()

		if exists {
			return backend, nil
		}

		modelBackend, err := s.storageBackendRepo.FindByID(backendID)
		if err != nil {
			return nil, fmt.Errorf("storage backend not found: %d", backendID)
		}

		storageBackend, err := CreateStorageBackend(modelBackend)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage backend: %w", err)
		}

		s.mutex.Lock()
		s.backendMap[backendID] = storageBackend
		s.mutex.Unlock()

		return storageBackend, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(storage.Backend), nil
}

func (s *StorageService) GetDefaultBackend() storage.Backend {
	return s.defaultBackend
}

func (s *StorageService) StorePackage(ctx context.Context, pkgType, name, version string, content io.Reader, size int64) (string, error) {
	return s.StorePackageWithBackend(ctx, "", pkgType, name, version, content, size, 0)
}

func (s *StorageService) StorePackageWithBackend(ctx context.Context, repoName, pkgType, name, version string, content io.Reader, size int64, backendID uint) (string, error) {
	var backend storage.Backend
	var err error

	if backendID == 0 {
		backend = s.GetDefaultBackend()
	} else {
		backend, err = s.GetBackend(backendID)
		if err != nil {
			return "", err
		}
	}

	key := s.buildKey(repoName, pkgType, name, version)

	if err := backend.Put(ctx, key, content, size); err != nil {
		return "", err
	}

	return key, nil
}

func (s *StorageService) GetPackage(ctx context.Context, pkgType, name, version string) (io.ReadCloser, int64, error) {
	return s.GetPackageWithBackend(ctx, "", pkgType, name, version, 0)
}

func (s *StorageService) GetPackageWithBackend(ctx context.Context, repoName, pkgType, name, version string, backendID uint) (io.ReadCloser, int64, error) {
	var backend storage.Backend
	var err error

	if backendID == 0 {
		backend = s.GetDefaultBackend()
	} else {
		backend, err = s.GetBackend(backendID)
		if err != nil {
			return nil, 0, err
		}
	}

	key := s.buildKey(repoName, pkgType, name, version)

	size, err := backend.Size(ctx, key)
	if err != nil {
		return nil, 0, err
	}

	reader, err := backend.Get(ctx, key)
	if err != nil {
		return nil, 0, err
	}

	return reader, size, nil
}

func (s *StorageService) DeletePackage(ctx context.Context, pkgType, name, version string) error {
	return s.DeletePackageWithBackend(ctx, "", pkgType, name, version, 0)
}

func (s *StorageService) DeletePackageWithBackend(ctx context.Context, repoName, pkgType, name, version string, backendID uint) error {
	var backend storage.Backend
	var err error

	if backendID == 0 {
		backend = s.GetDefaultBackend()
	} else {
		backend, err = s.GetBackend(backendID)
		if err != nil {
			return err
		}
	}

	key := s.buildKey(repoName, pkgType, name, version)
	return backend.Delete(ctx, key)
}

func (s *StorageService) Exists(ctx context.Context, pkgType, name, version string) (bool, error) {
	return s.ExistsWithBackend(ctx, "", pkgType, name, version, 0)
}

func (s *StorageService) ExistsWithBackend(ctx context.Context, repoName, pkgType, name, version string, backendID uint) (bool, error) {
	var backend storage.Backend
	var err error

	if backendID == 0 {
		backend = s.GetDefaultBackend()
	} else {
		backend, err = s.GetBackend(backendID)
		if err != nil {
			return false, err
		}
	}

	key := s.buildKey(repoName, pkgType, name, version)
	return backend.Exists(ctx, key)
}

func (s *StorageService) GetContentType(filename string) string {
	ext := filepath.Ext(filename)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return contentType
}

func (s *StorageService) buildKey(repoName, pkgType, name, version string) string {
	name = strings.TrimPrefix(name, "/")
	version = s.normalizeVersion(pkgType, version)

	if repoName != "" {
		return filepath.Join(pkgType, repoName, name, version)
	}

	switch pkgType {
	case "npm":
		if strings.Contains(name, "@") {
			parts := strings.SplitN(name, "/", 2)
			return filepath.Join("npm", parts[0], parts[1], version)
		}
		return filepath.Join("npm", name, version)

	case "maven":
		return filepath.Join("maven2", name, version)

	case "pypi":
		return filepath.Join("pypi", name, version)

	case "go":
		return filepath.Join("go", name, version)

	case "nuget":
		return filepath.Join("nuget", name, version)

	case "yum":
		return filepath.Join("yum", version)

	case "apt":
		return filepath.Join("apt", version)

	default:
		return filepath.Join(pkgType, name, version)
	}
}

// normalizeVersion 根据不同包类型规范化版本号
func (s *StorageService) normalizeVersion(pkgType, version string) string {
	version = strings.TrimPrefix(version, "/")

	// Go: 版本格式 "@v/filename"
	// 例如: "@v/v1.8.4.zip"
	// 符合 Go module proxy 规范
	if pkgType == "go" && !strings.HasPrefix(version, "@v/") {
		return "@v/" + version
	}

	return version
}

// RefreshBackends 从数据库刷新存储后端配置
func (s *StorageService) RefreshBackends() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 清空现有后端缓存
	s.backendMap = make(map[uint]storage.Backend)

	// 重新加载所有后端
	backends, err := s.storageBackendRepo.List()
	if err != nil {
		return err
	}

	for _, backend := range backends {
		storageBackend, err := CreateStorageBackend(&backend)
		if err != nil {
			return fmt.Errorf("failed to initialize storage backend %s: %w", backend.Name, err)
		}
		s.backendMap[backend.ID] = storageBackend
	}

	// 重新初始化默认后端
	defaultBackend, err := s.initDefaultBackend()
	if err != nil {
		return err
	}
	s.defaultBackend = defaultBackend

	return nil
}
