package adapter

import (
	"context"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/types"
)

func repositoryStorageBackendID(repo *model.Repository) uint {
	if repo == nil || repo.StorageBackendID == nil {
		return 0
	}
	return *repo.StorageBackendID
}

func repositoryID(repo *model.Repository) uint {
	if repo == nil {
		return 0
	}
	return repo.ID
}

func repositoryFromContext(ctx context.Context) *model.Repository {
	repo, _ := ctx.Value("repo").(*model.Repository)
	return repo
}

func packageMetaFromModel(pkg *model.Package, pkgType types.PackageType) *types.PackageMeta {
	meta := &types.PackageMeta{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Type:        pkgType,
		Description: pkg.Description,
	}

	for _, ver := range pkg.Versions {
		var totalSize int64
		for _, f := range ver.Files {
			totalSize += f.SizeBytes
		}
		if totalSize == 0 {
			totalSize = ver.SizeBytes
		}
		meta.Versions = append(meta.Versions, types.VersionInfo{
			Version:       ver.Version,
			PublishedAt:   ver.PublishedAt.Format(time.RFC3339),
			Size:          totalSize,
			DownloadCount: int64(ver.DownloadCount),
		})
	}

	return meta
}
