package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
)

type BaseAdapter struct {
	storageSvc *service.StorageService
	auditSvc   *service.AuditService
	fetcher    proxy.ProxyFetcher
	pkgCache   *cache.PackageCache
}

func NewBaseAdapter(storageSvc *service.StorageService, auditSvc *service.AuditService, pkgCache *cache.PackageCache) *BaseAdapter {
	return &BaseAdapter{
		storageSvc: storageSvc,
		auditSvc:   auditSvc,
		pkgCache:   pkgCache,
	}
}

func (b *BaseAdapter) SetFetcher(fetcher proxy.ProxyFetcher) {
	b.fetcher = fetcher
}

func (b *BaseAdapter) CheckDownloadPermission(c *gin.Context, repo *model.Repository, pkgType model.PackageType, name, version, filename string) *DownloadDecision {
	var pluginChain *DownloadPluginChain
	if chain, ok := c.Get("downloadPlugin"); ok {
		pluginChain = chain.(*DownloadPluginChain)
	}

	if pluginChain == nil {
		return AllowDownload()
	}

	userID := c.GetUint("userID")
	downloadCtx := &types.DownloadContext{
		GinCtx:   c,
		Repo:     repo,
		PkgType:  pkgType,
		Name:     name,
		Version:  version,
		Filename: filename,
		UserID:   userID,
		ClientIP: c.ClientIP(),
	}

	return pluginChain.Execute(downloadCtx)
}

func (b *BaseAdapter) CheckDownloadPermissionFromContext(ctx *types.DownloadContext, c *gin.Context) *DownloadDecision {
	var pluginChain *DownloadPluginChain
	if chain, ok := c.Get("downloadPlugin"); ok {
		pluginChain = chain.(*DownloadPluginChain)
	}

	if pluginChain == nil {
		return AllowDownload()
	}

	ctx.GinCtx = c

	return pluginChain.Execute(ctx)
}

func (b *BaseAdapter) GetPackageMetadata(ctx context.Context, name string, pkgType model.PackageType, typeStr types.PackageType) (*types.PackageMeta, error) {
	if b.pkgCache == nil {
		return nil, fmt.Errorf("package cache not initialized")
	}

	pkg, err := b.pkgCache.GetByNameAndType(name, pkgType)
	if err != nil {
		return nil, err
	}

	meta := &types.PackageMeta{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Type:        typeStr,
		Description: pkg.Description,
	}

	for _, ver := range pkg.Versions {
		var totalSize int64
		for _, f := range ver.Files {
			totalSize += f.SizeBytes
		}
		meta.Versions = append(meta.Versions, types.VersionInfo{
			Version:       ver.Version,
			PublishedAt:   ver.PublishedAt.Format(time.RFC3339),
			Size:          totalSize,
			DownloadCount: int64(ver.DownloadCount),
		})
	}

	return meta, nil
}

func (b *BaseAdapter) GetPackageRepository() *repository.PackageRepository {
	if b.pkgCache == nil {
		return nil
	}
	return b.pkgCache.GetPackageRepository()
}

func (b *BaseAdapter) GetStorageService() *service.StorageService {
	return b.storageSvc
}

func (b *BaseAdapter) LogDeleteAudit(c *gin.Context, repoName, pkgName, version string, pkgID *uint) {
	if b.auditSvc == nil {
		return
	}

	userID := c.GetUint("userID")
	var uid *uint
	if userID > 0 {
		uid = &userID
	}

	details := fmt.Sprintf(`{"repo":"%s","name":"%s","version":"%s"}`, repoName, pkgName, version)

	b.auditSvc.LogWithRequestAndStatus(
		c.Request.Context(),
		uid,
		model.ActionPackageDelete,
		"package",
		pkgID,
		pkgName,
		details,
		c.ClientIP(),
		c.Request.UserAgent(),
		200,
		0,
	)
}
