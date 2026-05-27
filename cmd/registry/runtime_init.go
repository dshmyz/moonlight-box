package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/storage"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// initRepoRuntimes 从 DB 加载所有仓库，创建对应的 Runtime 并注册到 DefaultRepositoryManager
func initRepoRuntimes(
	repoManager *runtime.DefaultRepositoryManager,
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	db *gorm.DB,
	storageSvc *service.StorageService,
	fetchers map[string]runtime.RemoteFetcher,
	blocker runtime.PackageBlocker,
	httpClient *http.Client,
) {
	allRepos, err := repoRepo.List(nil)
	if err != nil {
		logrus.WithError(err).Error("Failed to load repositories for runtime init")
		return
	}

	defaultBackend := storageSvc.GetDefaultBackend()
	if defaultBackend == nil {
		logrus.Warn("No default storage backend available, skipping runtime init")
		return
	}

	for i := range allRepos {
		repo := &allRepos[i]
		repoRuntime, createErr := createRuntimeForRepo(repo, repoRepo, groupRepo, db, defaultBackend, repoManager, fetchers, blocker, httpClient)
		if createErr != nil {
			logrus.WithError(createErr).WithField("repo", repo.Name).Warn("Failed to create runtime for repo")
			continue
		}

		config := map[string]interface{}{
			"allow_overwrite": repo.AllowOverwrite,
		}
		if repo.Config != nil {
			config["remote_url"] = repo.Config.RemoteURL
			config["auth_type"] = repo.Config.AuthType
			config["cache_enabled"] = repo.Config.CacheEnabled
			config["cache_ttl_seconds"] = repo.Config.CacheTTLSeconds
			config["cache_negative_ttl"] = repo.Config.CacheNegativeTTL
			config["timeout_seconds"] = repo.Config.TimeoutSeconds
			config["insecure_skip_verify"] = repo.Config.InsecureSkipVerify
		}

		if repo.Type == model.RepoTypeProxy {
			remoteURL := remoteURLForRepo(*repo)
			if remoteURL == "" {
				db.Raw("SELECT remote_url FROM repositories WHERE id = ?", repo.ID).Scan(&remoteURL)
			}
			if remoteURL != "" {
				config["remote_url"] = remoteURL
			}
		}

		rtRepo := &runtime.Repository{
			ID:      fmt.Sprintf("%d", repo.ID),
			Name:    repo.Name,
			Format:  repo.PackageType,
			Type:    string(repo.Type),
			Config:  config,
			Runtime: repoRuntime,
		}
		repoManager.Set(rtRepo)

		logrus.WithFields(logrus.Fields{
			"repo":   repo.Name,
			"type":   repo.Type,
			"format": repo.PackageType,
		}).Debug("Registered repo runtime")
	}
}

func createRuntimeForRepo(
	repo *model.Repository,
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	db *gorm.DB,
	backend storage.Backend,
	repoManager *runtime.DefaultRepositoryManager,
	fetchers map[string]runtime.RemoteFetcher,
	blocker runtime.PackageBlocker,
	httpClient *http.Client,
) (runtime.RepositoryRuntime, error) {
	metadataStore := storage.NewMetadataStore(db)
	blobStore := storage.NewCASBlobStore(backend, db)

	switch repo.Type {
	case model.RepoTypeLocal:
		return &runtime.HostedRuntime{
			MetadataStore: metadataStore,
			BlobStore:     blobStore,
			RepositoryID:  fmt.Sprintf("%d", repo.ID),
		}, nil

	case model.RepoTypeProxy:
		cachePolicy := runtime.CachePolicy{
			MetadataTTL: 0,
			BlobTTL:     0,
			NegativeTTL: 0,
		}
		remoteBaseURL := ""
		if repo.Config != nil {
			remoteBaseURL = repo.Config.RemoteURL
			if repo.Config.CacheTTLSeconds > 0 {
				cachePolicy.MetadataTTL = time.Duration(repo.Config.CacheTTLSeconds) * time.Second
			}
			if repo.Config.CacheNegativeTTL > 0 {
				cachePolicy.NegativeTTL = time.Duration(repo.Config.CacheNegativeTTL) * time.Second
			}
		}
		if remoteBaseURL == "" {
			db.Raw("SELECT remote_url FROM repositories WHERE id = ?", repo.ID).Scan(&remoteBaseURL)
		}
		pr := &runtime.ProxyRuntime{
			MetadataStore: metadataStore,
			BlobStore:     blobStore,
			RemoteClient:  runtime.NewHTTPRemoteClient(httpClient),
			RepositoryID:  fmt.Sprintf("%d", repo.ID),
			RemoteBaseURL: remoteBaseURL,
			CachePolicy:   cachePolicy,
			Format:        repo.PackageType,
		}
		if f, ok := fetchers[repo.PackageType]; ok {
			pr.Fetcher = f
		}
		pr.Blocker = blocker
		return pr, nil

	case model.RepoTypeVirtual:
		return createGroupRuntime(repo, repoRepo, groupRepo, db, backend, repoManager, fetchers, blocker, httpClient)

	default:
		return nil, fmt.Errorf("unsupported repo type: %s", repo.Type)
	}
}

func remoteURLForRepo(repo model.Repository) string {
	if repo.Config != nil && repo.Config.RemoteURL != "" {
		return repo.Config.RemoteURL
	}
	return ""
}

func createGroupRuntime(
	repo *model.Repository,
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	db *gorm.DB,
	backend storage.Backend,
	repoManager *runtime.DefaultRepositoryManager,
	fetchers map[string]runtime.RemoteFetcher,
	blocker runtime.PackageBlocker,
	httpClient *http.Client,
) (runtime.RepositoryRuntime, error) {
	members, err := repoRepo.FindByName(repo.Name)
	if err != nil {
		return nil, fmt.Errorf("loading members for group %s: %w", repo.Name, err)
	}

	var nodes []runtime.RepositoryNode
	var writable runtime.RepositoryNode

	for _, member := range members.Members {
		memberRepo := member.MemberRepo
		memberID := strconv.FormatUint(uint64(memberRepo.ID), 10)
		memberMeta := storage.NewMetadataStore(db)
		memberBlob := storage.NewCASBlobStore(backend, db)

		var node runtime.RepositoryNode
		switch memberRepo.Type {
		case model.RepoTypeLocal:
			n := &runtime.HostedRuntime{
				MetadataStore: memberMeta,
				BlobStore:     memberBlob,
				RepositoryID:  memberID,
			}
			node = n
			if writable == nil {
				writable = n
			}
		case model.RepoTypeProxy:
			remoteBaseURL := ""
			if memberRepo.Config != nil {
				remoteBaseURL = memberRepo.Config.RemoteURL
			}
			if remoteBaseURL == "" {
				db.Raw("SELECT remote_url FROM repositories WHERE id = ?", memberRepo.ID).Scan(&remoteBaseURL)
			}
			n := &runtime.ProxyRuntime{
				MetadataStore: memberMeta,
				BlobStore:     memberBlob,
				RemoteClient:  runtime.NewHTTPRemoteClient(httpClient),
				RepositoryID:  memberID,
				RemoteBaseURL: remoteBaseURL,
				Format:        memberRepo.PackageType,
			}
			if f, ok := fetchers[memberRepo.PackageType]; ok {
				n.Fetcher = f
			}
			n.Blocker = blocker
			node = n
		default:
			continue
		}

		nodes = append(nodes, node)

		// 确保成员仓库也在 manager 中注册
		if repoManager.Get(memberRepo.Name) == nil {
			rtRepo := &runtime.Repository{
				ID:     memberID,
				Name:   memberRepo.Name,
				Format: memberRepo.PackageType,
				Type:   string(memberRepo.Type),
				Config: map[string]interface{}{
					"remote_url": remoteURLForRepo(memberRepo),
				},
				Runtime: runtime.RepositoryRuntime(nil),
			}
			switch n := node.(type) {
			case *runtime.HostedRuntime:
				rtRepo.Runtime = n
			case *runtime.ProxyRuntime:
				rtRepo.Runtime = n
			}
			repoManager.Set(rtRepo)
		}
	}

	return &runtime.GroupRuntime{
		Members:  nodes,
		Writable: writable,
	}, nil
}

// === 阻断规则 & 审计日志适配器 ===

type blockRuleBlocker struct {
	svc *service.BlockRuleService
}

func (b *blockRuleBlocker) IsBlocked(packageType, packageName, version string) bool {
	result, err := b.svc.IsBlocked(packageType, packageName, version)
	if err != nil {
		return false
	}
	return result.Blocked
}

func (b *blockRuleBlocker) BlockReason(packageType, packageName, version string) string {
	result, err := b.svc.IsBlocked(packageType, packageName, version)
	if err != nil || result.Rule == nil {
		return "blocked"
	}
	return result.Rule.Reason
}

type auditLoggerAdapter struct {
	svc *service.AuditService
}

func (a *auditLoggerAdapter) Log(ctx context.Context, entry runtime.AuditEntry) {
	_ = a.svc.LogWithRequestAndStatus(ctx, &entry.UserID, model.ActionPackageDownload, entry.ResourceType, nil, entry.ResourceName, "", entry.IPAddress, entry.UserAgent, entry.ResponseStatus, 0)
}

// downloadCountAdapter 将 DownloadCountBatcher 适配为 runtime.DownloadCounter
type downloadCountAdapter struct {
	batcher *service.DownloadCountBatcher
}

func (a *downloadCountAdapter) IncrementDownload(repoID uint, format, name, version string) {
	a.batcher.Increment(0, 0, repoID)
}

func newDownloadCountAdapter(batcher *service.DownloadCountBatcher) *downloadCountAdapter {
	return &downloadCountAdapter{batcher: batcher}
}

// proxyLogAdapter 将 LogBatcher 适配为 runtime.ProxyDownloadLogger
type proxyLogAdapter struct {
	batcher *service.LogBatcher
}

func (a *proxyLogAdapter) LogDownload(repoID uint, packageType, packageName, version, filename string, statusCode int, sizeBytes int64, fromCache bool) {
	a.batcher.Record(&model.ProxyDownloadLog{
		RepositoryID: repoID,
		PackageType:  packageType,
		PackageName:  packageName,
		Version:      version,
		Filename:     filename,
		Status:       "success",
		StatusCode:   statusCode,
		SizeBytes:    sizeBytes,
		FromCache:    fromCache,
	})
}

func newProxyLogAdapter(batcher *service.LogBatcher) *proxyLogAdapter {
	return &proxyLogAdapter{batcher: batcher}
}
