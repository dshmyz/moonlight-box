package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/util"
	"github.com/sirupsen/logrus"
)

type UploadService struct {
	compRepo   *repository.ComponentRepository
	storageSvc *StorageService
}

func NewUploadService(compRepo *repository.ComponentRepository, storageSvc *StorageService) *UploadService {
	return &UploadService{
		compRepo:   compRepo,
		storageSvc: storageSvc,
	}
}

type UploadContext struct {
	PkgType          string
	Name             string
	StorageName      string
	Version          string
	StorageVersion   string
	Filename         string
	Content          io.Reader
	Size             int64
	PackageType      model.PackageType
	RepositoryID     uint
	UploadedBy       uint
	Metadata         map[string]interface{}
	Dependencies     []model.ComponentDependency
	FileType         model.AssetKind
	DownloadURL      string
	RepoName         string
	StorageBackendID uint
	Namespace        string
}

type UploadResult struct {
	ComponentID    uint
	VersionID      uint // alias of ComponentID for API compatibility
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
	storageName := uc.StorageName
	if storageName == "" {
		storageName = uc.Name
	}

	storageKey, err := s.storageSvc.StorePackageWithBackend(ctx, uc.RepoName, uc.PkgType, storageName, storageVersion, checksumReader, uc.Size, uc.StorageBackendID)
	if err != nil {
		return nil, fmt.Errorf("failed to store package: %w", err)
	}

	checksum := checksumReader.GetResult()
	if checksum == nil {
		return nil, fmt.Errorf("failed to calculate checksum")
	}

	comp := &model.Component{
		RepositoryID: uc.RepositoryID,
		Format:       uc.PackageType,
		Namespace:    uc.Namespace,
		Name:         uc.Name,
		Version:      uc.Version,
		Description:  getDescription(uc.Metadata),
		Status:       model.StatusPublished,
		PublishedBy:  uc.UploadedBy,
		Metadata:     marshalMetadata(uc.Metadata),
		CreatedBy:    uc.UploadedBy,
	}
	asset := &model.Asset{
		FileName:    uc.Filename,
		Kind:        uc.FileType,
		Path:        storageKey,
		DownloadURL: uc.DownloadURL,
		Blob: model.Blob{
			Ref:       storageKey,
			SHA256:    checksum.SHA256,
			MD5:       checksum.MD5,
			SizeBytes: checksumReader.GetWrittenBytes(),
		},
	}

	savedComp, _, err := s.compRepo.StoreComponentAsset(ctx, comp, asset)
	if err != nil {
		s.storageSvc.DeletePackageWithBackend(ctx, uc.RepoName, uc.PkgType, storageName, storageVersion, uc.StorageBackendID)
		return nil, fmt.Errorf("failed to store component metadata: %w", err)
	}

	if len(uc.Dependencies) > 0 {
		componentID := savedComp.ID
		deps := make([]model.ComponentDependency, len(uc.Dependencies))
		copy(deps, uc.Dependencies)
		go func() {
			bg, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			if depErr := s.compRepo.UpsertComponentDependencies(bg, componentID, deps); depErr != nil {
				logrus.WithError(depErr).WithFields(logrus.Fields{
					"component_id": componentID,
					"dep_count":    len(deps),
				}).Warn("failed to async upsert component dependencies")
			}
		}()
	}

	return &UploadResult{
		ComponentID:    savedComp.ID,
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
	data, err := json.Marshal(meta)
	if err != nil {
		logrus.WithError(err).Warn("failed to marshal component metadata, returning empty")
		return ""
	}
	return string(data)
}

func (s *UploadService) GetComponentRepository() *repository.ComponentRepository {
	return s.compRepo
}

func (s *UploadService) GetStorageService() *StorageService {
	return s.storageSvc
}
