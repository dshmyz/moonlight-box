package adapter

import (
	"bytes"
	"context"
	"io"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
)

type BaseAdapter struct {
	pkgRepo    *repository.PackageRepository
	storageSvc *service.StorageService
}

func NewBaseAdapter(pkgRepo *repository.PackageRepository, storageSvc *service.StorageService) *BaseAdapter {
	return &BaseAdapter{
		pkgRepo:    pkgRepo,
		storageSvc: storageSvc,
	}
}

type ProxyPackageInfo struct {
	PackageType model.PackageType
	Name        string
	Version     string
	RepoID      uint
	Content     []byte
	Size        int64
	Metadata    map[string]interface{}
}

func (b *BaseAdapter) StoreProxyPackage(ctx context.Context, info *ProxyPackageInfo) (string, error) {
	storageKey, err := b.storageSvc.StorePackage(ctx, string(info.PackageType), info.Name, info.Version, bytes.NewReader(info.Content), info.Size)
	if err != nil {
		return "", err
	}

	_, _, dbErr := b.pkgRepo.CreateOrUpdate(ctx, &model.Package{
		Name:           info.Name,
		Type:           info.PackageType,
		RepositoryID:   info.RepoID,
		RepositoryType: model.RepoTypeProxy,
	}, &model.PackageVersion{
		Version:     info.Version,
		Status:      model.StatusPublished,
		StoragePath: storageKey,
		SizeBytes:   info.Size,
		Metadata:    marshalMetadata(info.Metadata),
	})

	if dbErr != nil {
		return storageKey, dbErr
	}

	return storageKey, nil
}

func (b *BaseAdapter) StoreProxyPackageFromResult(ctx context.Context, pkgType model.PackageType, name, version string, result *proxy.RouteResult, metadata map[string]interface{}) (string, error) {
	body, err := io.ReadAll(result.Content)
	if err != nil {
		return "", err
	}

	info := &ProxyPackageInfo{
		PackageType: pkgType,
		Name:        name,
		Version:     version,
		RepoID:      result.RepoID,
		Content:     body,
		Size:        result.Size,
		Metadata:    metadata,
	}

	return b.StoreProxyPackage(ctx, info)
}

// IncrementDownloadCountForPackage 增加包的下载计数
func (b *BaseAdapter) IncrementDownloadCountForPackage(pkgName string, pkgType model.PackageType, version string, filename string) {
	pkg, err := b.pkgRepo.FindByNameAndType(pkgName, pkgType)
	if err != nil {
		return
	}

	for _, v := range pkg.Versions {
		if v.Version == version {
			for _, f := range v.Files {
				if f.Filename == filename {
					b.pkgRepo.IncrementDownloadCount(pkg.ID, v.ID, f.ID)
					return
				}
			}
			return
		}
	}
}
