package handler

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/adapter"
	"github.com/moonlight-box/registry/internal/metrics"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
)

type RepoRouter struct {
	repoSvc        *service.RepositoryService
	repoCache      *proxy.RepositoryCache
	resolver       *proxy.RepoHandler
	downloadPlugin *adapter.DownloadPluginChain
	webhookSvc     *service.WebhookService
	permCache      *service.PermissionCacheService
	blockSvc       *service.BlockRuleService
}

func NewRepoRouter(repoSvc *service.RepositoryService) *RepoRouter {
	return &RepoRouter{
		repoSvc: repoSvc,
	}
}

func (r *RepoRouter) SetRepoCache(cache *proxy.RepositoryCache) {
	r.repoCache = cache
}

func (r *RepoRouter) SetResolver(resolver *proxy.RepoHandler) {
	r.resolver = resolver
}

func (r *RepoRouter) SetWebhookService(webhookSvc *service.WebhookService) {
	r.webhookSvc = webhookSvc
}

func (r *RepoRouter) SetPermCache(permCache *service.PermissionCacheService) {
	r.permCache = permCache
}

func (r *RepoRouter) SetBlockService(blockSvc *service.BlockRuleService) {
	r.blockSvc = blockSvc
}

func (r *RepoRouter) checkBlock(c *gin.Context, pkgType, pkgName, version string) bool {
	if r.blockSvc == nil || pkgName == "" {
		return false
	}

	result, err := r.blockSvc.IsBlocked(pkgType, pkgName, version)
	if err != nil {
		return false
	}

	if result.Blocked {
		ipAddress := c.ClientIP()
		userAgent := c.Request.UserAgent()
		_ = r.blockSvc.LogBlock(c.Request.Context(), pkgName, version, result.Rule, ipAddress, userAgent)

		reason := result.Rule.Reason
		if reason == "" {
			reason = "该版本已被管理员阻断"
		}
		msg := fmt.Sprintf("包 %s@%s 已被阻断: %s", pkgName, version, reason)
		response.Forbidden(c, msg)
		return true
	}

	return false
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

	pkgType := repo.PackageType

	if r.downloadPlugin != nil {
		c.Set("downloadPlugin", r.downloadPlugin)
	}

	c.Set("repo", repo)

	if r.resolver == nil {
		response.NotFound(c, "resolver 未初始化")
		return
	}

	// 先尝试解析为下载请求
	result, err := r.resolver.TryResolveDownload(c.Request.Context(), repo, pkgType, strings.TrimPrefix(path, "/"))
	if err == nil && result != nil {
		// 阻断检查
		if r.checkBlock(c, pkgType, result.Name, result.Version) {
			result.Content.Close()
			return
		}

		// 下载路径，检查权限后返回内容
		filename := result.Filename
		decision := r.CheckDownloadPermission(c, repo, model.PackageType(pkgType), result.Name, result.Version, filename)
		if !decision.Allow {
			c.JSON(decision.Code, gin.H{"error": decision.Message})
			result.Content.Close()
			return
		}

		defer result.Content.Close()
		r.resolver.FormatDownloadResponse(c, pkgType, result)
		return
	}

	// 非下载路径，交给 adapter 处理
	if err := r.resolver.HandleRepoRequest(c, repo, strings.TrimPrefix(path, "/")); err != nil {
		response.NotFound(c, err.Error())
		return
	}
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

	if r.permCache != nil {
		userID := c.GetUint("userID")
		if userID == 0 {
			response.Unauthorized(c, "missing user information")
			return
		}

		permissions, err := r.permCache.GetUserPermissions(userID)
		if err != nil {
			response.InternalError(c, "failed to load user permissions")
			return
		}

		hasPermission := false
		packageType := strings.ToLower(string(repo.PackageType))
		for _, p := range permissions {
			if p.Resource == packageType && p.Action == "write" {
				hasPermission = true
				break
			}
			if p.Resource == "system" && p.Action == "admin" {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			response.Forbidden(c, "insufficient permissions for "+packageType+" repository")
			return
		}
	}

	if r.resolver == nil {
		response.NotFound(c, "resolver 未初始化")
		return
	}

	c.Set("repo", repo)
	c.Set("allowOverwrite", repo.AllowOverwrite)
	result, err := r.resolver.HandleRepoPublish(c, repo)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if result != nil {
		metrics.RecordUpload(string(repo.PackageType), result.PackageName, result.Version)

		if result.Response != nil {
			response.Success(c, result.Response)
		} else {
			response.Success(c, &types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  result.PackageName,
				Version:  result.Version,
				Filename: result.Filename,
				Size:     result.Size,
			})
		}

		if r.webhookSvc != nil {
			r.webhookSvc.TriggerEvent(model.WebhookEventPackageUploaded, &service.WebhookPayload{
				Event:       string(model.WebhookEventPackageUploaded),
				PackageName: result.PackageName,
				Version:     result.Version,
				Repository:  repo.Name,
				Data:        result.ExtraData,
			})
		}
	} else {
		c.JSON(200, &types.PublishResponse{
			Success: true,
			Message: "Package published successfully",
		})
	}
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

	if r.resolver == nil {
		response.NotFound(c, "resolver 未初始化")
		return
	}

	result, err := r.resolver.HandleRepoDelete(c, repo)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if result != nil && r.webhookSvc != nil {
		r.webhookSvc.TriggerEvent(model.WebhookEventPackageDeleted, &service.WebhookPayload{
			Event:       string(model.WebhookEventPackageDeleted),
			PackageName: result.PackageName,
			Version:     result.Version,
			Repository:  repo.Name,
			Data:        result.ExtraData,
		})
	}
}
