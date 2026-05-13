package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/util"
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
	Content        io.Reader
	Size           int64
	PackageType    model.PackageType
	RepositoryType model.RepositoryType
	RepositoryID   uint
	UploadedBy     uint
	Metadata       map[string]interface{}
	FileType       model.PackageFileType
	DownloadURL    string
	RepoName       string
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
	checksumReader := util.NewChecksumReader(uc.Content)

	storageVersion := uc.StorageVersion
	if storageVersion == "" {
		storageVersion = uc.Version
	}

	storageKey, err := s.storageSvc.StorePackageWithBackend(ctx, uc.RepoName, uc.PkgType, uc.Name, storageVersion, checksumReader, uc.Size, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to store package: %w", err)
	}

	checksum := checksumReader.GetResult()
	if checksum == nil {
		return nil, fmt.Errorf("failed to calculate checksum")
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
		PublishedBy: uc.UploadedBy,
		Metadata:    marshalMetadata(uc.Metadata),
	}, &model.PackageFile{
		Filename:       uc.Filename,
		FileType:       uc.FileType,
		StoragePath:    storageKey,
		SizeBytes:      checksumReader.GetWrittenBytes(),
		ChecksumSHA256: checksum.SHA256,
		ChecksumMD5:    checksum.MD5,
		DownloadURL:    uc.DownloadURL,
	})

	if err != nil {
		s.storageSvc.DeletePackageWithBackend(ctx, uc.RepoName, uc.PkgType, uc.Name, storageVersion, 0)
		return nil, fmt.Errorf("failed to store package metadata: %w", err)
	}

	return &UploadResult{
		PackageID:      pkg.ID,
		VersionID:      ver.ID,
		Version:        uc.Version,
		StorageKey:     storageKey,
		Size:           checksumReader.GetWrittenBytes(),
		ChecksumMD5:    checksum.MD5,
		ChecksumSHA256: checksum.SHA256,
	}, nil
}

func (s *UploadService) UploadWithPostProcess(ctx context.Context, uc *UploadContext, postProcess func(ctx context.Context, content io.Reader, name, version string) error) (*UploadResult, error) {
	result, err := s.Upload(ctx, uc)
	if err != nil {
		return nil, err
	}

	if postProcess != nil {
		storageVersion := uc.StorageVersion
		if storageVersion == "" {
			storageVersion = uc.Version
		}
		if seeker, ok := uc.Content.(io.ReadSeeker); ok {
			_, _ = seeker.Seek(0, io.SeekStart)
			_ = postProcess(ctx, uc.Content, uc.Name, storageVersion)
		}
	}

	return result, nil
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
