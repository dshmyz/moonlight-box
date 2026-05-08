package handler

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/adapter"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
)

type RepoRouter struct {
	repoSvc        *service.RepositoryService
	repoCache      *proxy.RepositoryCache
	auditSvc       *service.AuditService
	adapters       map[string]adapter.RepoAwareAdapter
	typeDetector   *proxy.TypeDetector
	downloadPlugin *adapter.DownloadPluginChain
}

func NewRepoRouter(repoSvc *service.RepositoryService, auditSvc *service.AuditService) *RepoRouter {
	return &RepoRouter{
		repoSvc:      repoSvc,
		auditSvc:     auditSvc,
		adapters:     make(map[string]adapter.RepoAwareAdapter),
		typeDetector: proxy.NewTypeDetector(),
	}
}

func (r *RepoRouter) SetRepoCache(cache *proxy.RepositoryCache) {
	r.repoCache = cache
}

func (r *RepoRouter) SetDownloadPlugin(plugin *adapter.DownloadPluginChain) {
	r.downloadPlugin = plugin
}

func (r *RepoRouter) CheckDownloadPermission(c *gin.Context, repo *model.Repository, pkgType model.PackageType, name, version, filename string) *adapter.DownloadDecision {
	if r.downloadPlugin == nil {
		return adapter.AllowDownload()
	}

	userID := c.GetUint("userID")
	downloadCtx := &adapter.DownloadContext{
		Ctx:      c,
		Repo:     repo,
		PkgType:  pkgType,
		Name:     name,
		Version:  version,
		Filename: filename,
		UserID:   userID,
		ClientIP: c.ClientIP(),
	}

	return r.downloadPlugin.Execute(downloadCtx)
}

func (r *RepoRouter) RegisterAdapter(pkgType string, adp adapter.RepoAwareAdapter) {
	r.adapters[pkgType] = adp
}

func (r *RepoRouter) getRepo(name string) (*model.Repository, error) {
	if r.repoCache != nil {
		return r.repoCache.GetByName(name)
	}
	return r.repoSvc.Get(name)
}

func (r *RepoRouter) HandleRequest(c *gin.Context) {
	repoName := c.Param("repoName")
	path := c.Param("path")

	repo, err := r.getRepo(repoName)
	if err != nil {
		response.NotFound(c, "仓库不存在")
		return
	}

	if !repo.Enabled {
		response.NotFound(c, "仓库已禁用")
		return
	}

	var pkgType string

	if repo.Type == model.RepoTypeVirtual {
		trimmedPath := strings.TrimPrefix(path, "/")
		pkgType = r.typeDetector.Detect(trimmedPath)

		if pkgType == "" {
			response.BadRequest(c, "无法从请求路径识别包类型",
				"请确保 URL 包含包类型前缀，如 /npm/ 或 /maven/")
			return
		}

		if pkgType != repo.PackageType {
			response.NotFound(c, fmt.Sprintf("此虚拟仓库不支持 %s 类型的包", pkgType))
			return
		}
	} else {
		pkgType = repo.PackageType
	}

	adp, ok := r.adapters[pkgType]
	if !ok {
		response.NotFound(c, fmt.Sprintf("不支持的包类型: %s", pkgType))
		return
	}

	if r.downloadPlugin != nil {
		c.Set("downloadPlugin", r.downloadPlugin)
	}

	adp.HandleRepoRequest(c, repo, strings.TrimPrefix(path, "/"))
}

func (r *RepoRouter) HandlePublish(c *gin.Context) {
	repoName := c.Param("repoName")

	repo, err := r.getRepo(repoName)
	if err != nil {
		response.NotFound(c, "仓库不存在")
		return
	}

	if !repo.Enabled {
		response.NotFound(c, "仓库已禁用")
		return
	}

	switch repo.Type {
	case model.RepoTypeProxy:
		response.Forbidden(c, "代理仓库不支持发布，代理仓库只能从远程仓库下载")
		return
	case model.RepoTypeVirtual:
		response.Forbidden(c, "虚拟仓库不支持直接发布，请发布到成员仓库")
		return
	case model.RepoTypeLocal:
		break
	default:
		response.BadRequest(c, "未知的仓库类型", "")
		return
	}

	adp, ok := r.adapters[repo.PackageType]
	if !ok {
		response.NotFound(c, fmt.Sprintf("不支持的包类型: %s", repo.PackageType))
		return
	}

	c.Set("repo", repo)
	c.Set("allowOverwrite", repo.AllowOverwrite)
	adp.HandleRepoPublish(c, repo)
}

func (r *RepoRouter) HandleDelete(c *gin.Context) {
	repoName := c.Param("repoName")

	repo, err := r.getRepo(repoName)
	if err != nil {
		response.NotFound(c, "仓库不存在")
		return
	}

	if !repo.Enabled {
		response.NotFound(c, "仓库已禁用")
		return
	}

	switch repo.Type {
	case model.RepoTypeProxy:
		response.Forbidden(c, "代理仓库不支持删除，代理仓库只能从远程仓库下载")
		return
	case model.RepoTypeVirtual:
		response.Forbidden(c, "虚拟仓库不支持直接删除，请在成员仓库中删除")
		return
	case model.RepoTypeLocal:
		if !repo.AllowDelete {
			response.Forbidden(c, "此仓库不允许删除，请联系管理员启用删除权限")
			return
		}
		break
	default:
		response.BadRequest(c, "未知的仓库类型", "")
		return
	}

	adp, ok := r.adapters[repo.PackageType]
	if !ok {
		response.NotFound(c, fmt.Sprintf("不支持的包类型: %s", repo.PackageType))
		return
	}

	adp.HandleRepoDelete(c, repo)
}
