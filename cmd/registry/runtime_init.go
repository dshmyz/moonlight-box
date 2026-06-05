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

// initRepoRuntimes 预热所有仓库的 Runtime。启动时调用可避免首次请求冷启动。
// 即使不调用，Get() 懒加载也能正常工作。
func initRepoRuntimes(
	repoManager *runtime.DefaultRepositoryManager,
	repoRepo *repository.RepositoryRepository,
) {
	allRepos, err := repoRepo.List(nil)
	if err != nil {
		logrus.WithError(err).Error("Failed to load repositories for runtime prewarm")
		return
	}

	for i := range allRepos {
		repo := &allRepos[i]
		// 通过 manager.Get 触发懒加载 factory，统一走同一条路径
		if rt := repoManager.Get(repo.Name); rt != nil {
			logrus.WithFields(logrus.Fields{
				"repo":   repo.Name,
				"type":   repo.Type,
				"format": repo.PackageType,
			}).Debug("Prewarmed repo runtime")
		}
	}
}

// NewRepositoryFactory 创建懒加载工厂函数，注入到 DefaultRepositoryManager。
// Get() 在内存缓存未命中时自动调用此函数从 DB 加载并创建 Runtime。
func NewRepositoryFactory(
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	db *gorm.DB,
	storageSvc *service.StorageService,
	repoManager *runtime.DefaultRepositoryManager,
	fetchers map[string]runtime.RemoteFetcher,
	blocker runtime.PackageBlocker,
	httpClient *http.Client,
	artifactSvc *service.ArtifactService,
) runtime.RepositoryFactory {
	return func(name string) (*runtime.Repository, error) {
		repo, err := repoRepo.FindByName(name)
		if err != nil {
			return nil, fmt.Errorf("repo not found: %s: %w", name, err)
		}

		backend := storageSvc.GetDefaultBackend()
		if backend == nil {
			return nil, fmt.Errorf("no default storage backend available")
		}

		repoRuntime, err := createRuntimeForRepo(repo, repoRepo, groupRepo, db, backend, repoManager, fetchers, blocker, httpClient, artifactSvc)
		if err != nil {
			return nil, err
		}

		return buildRuntimeRepo(repo, repoRuntime), nil
	}
}

// buildRuntimeRepo 将 model.Repository + RepositoryRuntime 转换为 runtime.Repository
func buildRuntimeRepo(repo *model.Repository, repoRuntime runtime.RepositoryRuntime) *runtime.Repository {
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
	if repo.Type == model.RepoTypeProxy && repo.Config == nil {
		// Config 为 nil 时尝试从 DB 补充 remote_url
		config["remote_url"] = ""
	}
	return &runtime.Repository{
		ID:      fmt.Sprintf("%d", repo.ID),
		Name:    repo.Name,
		Format:  repo.PackageType,
		Type:    string(repo.Type),
		Config:  config,
		Runtime: repoRuntime,
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
	artifactSvc *service.ArtifactService,
) (runtime.RepositoryRuntime, error) {
	metadataStore := storage.NewMetadataStoreWithArtifactService(db, artifactSvc)
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
		return createGroupRuntime(repo, repoRepo, groupRepo, db, backend, repoManager, fetchers, blocker, httpClient, artifactSvc)

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
	artifactSvc *service.ArtifactService,
) (runtime.RepositoryRuntime, error) {
	members, err := repoRepo.FindByName(repo.Name)
	if err != nil {
		return nil, fmt.Errorf("loading members for group %s: %w", repo.Name, err)
	}

	logrus.WithFields(logrus.Fields{
		"groupName":   repo.Name,
		"memberCount": len(members.Members),
		"packageType": repo.PackageType,
	}).Debug("createGroupRuntime: loading group members")

	var nodes []runtime.RepositoryNode
	var writable runtime.RepositoryNode

	for i, member := range members.Members {
		memberRepo := member.MemberRepo
		memberID := strconv.FormatUint(uint64(memberRepo.ID), 10)

		logrus.WithFields(logrus.Fields{
			"index":        i,
			"memberName":   memberRepo.Name,
			"memberType":   memberRepo.Type,
			"memberFormat": memberRepo.PackageType,
		}).Debug("createGroupRuntime: processing member")

		// 通过 manager.Get 获取成员仓库的 Runtime（懒加载，统一路径）
		memberRT := repoManager.Get(memberRepo.Name)
		if memberRT == nil || memberRT.Runtime == nil {
			// 成员仓库还没注册或没有 Runtime，手动创建
			memberMeta := storage.NewMetadataStoreWithArtifactService(db, artifactSvc)
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
				logrus.WithFields(logrus.Fields{
					"memberName": memberRepo.Name,
					"memberType": memberRepo.Type,
				}).Warn("createGroupRuntime: skipping unsupported member type")
				continue
			}

			// 注册到 manager，下次不再重复创建
			repoManager.Set(buildRuntimeRepo(&memberRepo, node.(runtime.RepositoryRuntime)))
			nodes = append(nodes, node)
			if memberRepo.Type == model.RepoTypeLocal && writable == nil {
				writable = node
			}
			continue
		}

		// 成员已有 Runtime，直接复用
		nodes = append(nodes, memberRT.Runtime.(runtime.RepositoryNode))
		if memberRepo.Type == model.RepoTypeLocal && writable == nil {
			writable = memberRT.Runtime.(runtime.RepositoryNode)
		}

		logrus.WithFields(logrus.Fields{
			"memberName": memberRepo.Name,
		}).Debug("createGroupRuntime: reused existing member runtime")
	}

	logrus.WithFields(logrus.Fields{
		"groupName":   repo.Name,
		"nodeCount":   len(nodes),
		"hasWritable": writable != nil,
	}).Debug("createGroupRuntime: group runtime created")

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
	a.batcher.Increment(repoID, format, name, version)
}

func newDownloadCountAdapter(batcher *service.DownloadCountBatcher) *downloadCountAdapter {
	return &downloadCountAdapter{batcher: batcher}
}

// downloadLogAdapter 将 LogBatcher 适配为 runtime.DownloadLogger
type downloadLogAdapter struct {
	batcher *service.LogBatcher
}

func (a *downloadLogAdapter) LogDownload(params runtime.DownloadLogParams) {
	var status string
	switch {
	case params.StatusCode >= 400:
		status = model.DownloadStatusFailed
	case params.FromCache:
		status = model.DownloadStatusCached
	default:
		status = model.DownloadStatusSuccess
	}
	a.batcher.Record(&model.DownloadLog{
		RepositoryID: params.RepoID,
		PackageType:  params.PackageType,
		PackageName:  params.PackageName,
		Version:      params.Version,
		Filename:     params.Filename,
		RemoteURL:    params.RemoteURL,
		Status:       status,
		StatusCode:   params.StatusCode,
		SizeBytes:    params.SizeBytes,
		FromCache:    params.FromCache,
		IPAddress:    params.ClientIP,
		UserAgent:    params.UserAgent,
		RequestID:    params.RequestID,
	})
}

func newDownloadLogAdapter(batcher *service.LogBatcher) *downloadLogAdapter {
	return &downloadLogAdapter{batcher: batcher}
}
