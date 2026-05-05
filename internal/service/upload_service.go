package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
)

type UploadService struct {
	pkgRepo    *repository.PackageRepository
	storageSvc *StorageService
}

func NewUploadService(pkgRepo *repository.PackageRepository, storageSvc *StorageService) *UploadService {
	return &UploadService{
		pkgRepo:    pkgRepo,
		storageSvc: storageSvc,
	}
}

type UploadContext struct {
	PkgType        string
	Name           string
	Version        string
	StorageVersion string
	Filename       string
	Content        []byte
	Size           int64
	PackageType    model.PackageType
	RepositoryType model.RepositoryType
	RepositoryID   uint
	UploadedBy     uint
	Metadata       map[string]interface{}
	FileType       model.PackageFileType
}

type UploadResult struct {
	PackageID      uint
	VersionID      uint
	Version        string
	StorageKey     string
	Size           int64
	ChecksumMD5    string
	ChecksumSHA256 string
}

func (s *UploadService) Upload(ctx context.Context, uc *UploadContext) (*UploadResult, error) {
	checksumSHA256 := s.calculateSHA256(uc.Content)
	checksumMD5 := s.calculateMD5(uc.Content)

	storageVersion := uc.StorageVersion
	if storageVersion == "" {
		storageVersion = uc.Version
	}

	storageKey, err := s.storageSvc.StorePackage(ctx, uc.PkgType, uc.Name, storageVersion, bytes.NewReader(uc.Content), uc.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to store package: %w", err)
	}

	pkg, ver, _, err := s.pkgRepo.StorePackageFile(ctx, &model.Package{
		Name:           uc.Name,
		Type:           uc.PackageType,
		Description:    getDescription(uc.Metadata),
		RepositoryID:   uc.RepositoryID,
		RepositoryType: uc.RepositoryType,
		CreatedBy:      uc.UploadedBy,
	}, &model.PackageVersion{
		Version:     uc.Version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
		PublishedBy: uc.UploadedBy,
		Metadata:    marshalMetadata(uc.Metadata),
	}, &model.PackageFile{
		Filename:       uc.Filename,
		FileType:       uc.FileType,
		StoragePath:    storageKey,
		SizeBytes:      uc.Size,
		ChecksumSHA256: checksumSHA256,
		ChecksumMD5:    checksumMD5,
	})

	if err != nil {
		s.storageSvc.DeletePackage(ctx, uc.PkgType, uc.Name, storageVersion)
		return nil, fmt.Errorf("failed to store package metadata: %w", err)
	}

	return &UploadResult{
		PackageID:      pkg.ID,
		VersionID:      ver.ID,
		Version:        uc.Version,
		StorageKey:     storageKey,
		Size:           uc.Size,
		ChecksumMD5:    checksumMD5,
		ChecksumSHA256: checksumSHA256,
	}, nil
}

func (s *UploadService) UploadWithPostProcess(ctx context.Context, uc *UploadContext, postProcess func(ctx context.Context, content []byte, name, version string) error) (*UploadResult, error) {
	result, err := s.Upload(ctx, uc)
	if err != nil {
		return nil, err
	}

	if postProcess != nil {
		storageVersion := uc.StorageVersion
		if storageVersion == "" {
			storageVersion = uc.Version
		}
		_ = postProcess(ctx, uc.Content, uc.Name, storageVersion)
	}

	return result, nil
}

func (s *UploadService) calculateSHA256(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func (s *UploadService) calculateMD5(content []byte) string {
	hash := md5.Sum(content)
	return hex.EncodeToString(hash[:])
}

func (s *UploadService) ReadAllContent(reader io.Reader, size int64) ([]byte, error) {
	if size > 0 {
		content := make([]byte, size)
		_, err := io.ReadFull(reader, content)
		return content, err
	}
	return io.ReadAll(reader)
}

func getDescription(meta map[string]interface{}) string {
	if meta == nil {
		return ""
	}
	if desc, ok := meta["description"]; ok {
		if s, ok := desc.(string); ok {
			return s
		}
	}
	return ""
}

func marshalMetadata(meta map[string]interface{}) string {
	if meta == nil {
		return ""
	}
	data, _ := json.Marshal(meta)
	return string(data)
}

func (s *UploadService) GetPackageRepository() *repository.PackageRepository {
	return s.pkgRepo
}

func (s *UploadService) GetStorageService() *StorageService {
	return s.storageSvc
}
