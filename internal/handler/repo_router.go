package handler

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/adapter"
	apperr "github.com/moonlight-box/registry/internal/errors"
	"github.com/moonlight-box/registry/internal/metrics"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/sirupsen/logrus"
)

type RepoRouter struct {
	repoSvc        *service.RepositoryService
	repoCache      *proxy.RepositoryCache
	resolver       *proxy.RepoHandler
	downloadPlugin *adapter.DownloadPluginChain
	webhookSvc     *service.WebhookService
	permCache      *service.PermissionCacheService
	blockSvc       *service.BlockRuleService
	uploadSvc      *service.UploadService
	auditSvc       *service.AuditService
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

func (r *RepoRouter) SetUploadService(uploadSvc *service.UploadService) {
	r.uploadSvc = uploadSvc
}

func (r *RepoRouter) SetAuditService(auditSvc *service.AuditService) {
	r.auditSvc = auditSvc
}

func (r *RepoRouter) checkBlock(c *gin.Context, pkgType, pkgName, version string) bool {
	if r.blockSvc == nil || pkgName == "" {
		return false
	}

	result, err := r.blockSvc.IsBlocked(pkgType, pkgName, version)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":   "block_rule",
			"pkg_type": pkgType,
			"pkg_name": pkgName,
			"version":  version,
			"error":    err,
		}).Error("Block check failed, blocking request for safety (fail-closed)")
		return true
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

	return r.downloadPlugin.Execute(downloadCtx)
}

func (r *RepoRouter) getRepo(name string) (*model.Repository, error) {
	return r.getRepoContext(context.Background(), name)
}

func (r *RepoRouter) getRepoContext(ctx context.Context, name string) (*model.Repository, error) {
	if r.repoCache != nil {
		return r.repoCache.GetByNameContext(ctx, name)
	}
	return r.repoSvc.GetContext(ctx, name)
}

func (r *RepoRouter) HandleRequest(c *gin.Context) {
	repoName := c.Param("repoName")
	path := c.Param("path")

	repo, err := r.getRepoContext(c.Request.Context(), repoName)
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

	// 获取 adapter
	adp, ok := r.resolver.GetAdapter(model.PackageType(pkgType))
	if !ok {
		response.NotFound(c, "不支持的包类型: "+pkgType)
		return
	}

	// 第一步：解析意图
	requestPath := strings.TrimPrefix(path, "/")
	intent := adp.ParseIntent(requestPath, c.Request.Method)
	if intent.Type == types.RequestUnknown {
		response.NotFound(c, "无法识别的请求路径")
		return
	}

	// 第二步：阻断检查（对下载和元数据请求都检查）
	if r.blockSvc != nil {
		blockName := intent.Name
		blockVersion := intent.Version
		if blockName != "" && r.checkBlock(c, pkgType, blockName, blockVersion) {
			return
		}
	}

	// 第三步：权限检查（针对下载类型请求）
	if intent.Type == types.RequestDownload {
		decision := r.CheckDownloadPermission(c, repo, model.PackageType(pkgType), intent.Name, intent.Version, intent.Filename)
		if !decision.Allow {
			c.JSON(decision.Code, gin.H{"error": decision.Message})
			return
		}
	}

	// 第四步：获取内容 — 下载走 Resolve（含 Local/Proxy/Virtual + 缓存 + 日志），非下载走 FetchContent
	if intent.Type == types.RequestDownload {
		if intent.PkgPathInfo == nil {
			response.NotFound(c, "invalid package path")
			return
		}
		downloadCtx := &types.DownloadContext{
			Repo:         repo,
			PkgType:      model.PackageType(pkgType),
			Name:         intent.Name,
			Version:      intent.Version,
			Filename:     intent.Filename,
			UserID:       c.GetUint("userID"),
			ClientIP:     c.ClientIP(),
			ResolvedPath: intent.PkgPathInfo,
		}
		reqCtx := context.WithValue(c.Request.Context(), "repo", repo)
		routeResult, err := r.resolver.Resolve(reqCtx, downloadCtx)
		if err != nil {
			r.writeRepoError(c, repo, intent, err)
			return
		}
		defer routeResult.Content.Close()
		r.formatContentResponse(c, &types.ContentResult{
			Content:     routeResult.Content,
			Size:        routeResult.Size,
			ContentType: "application/octet-stream",
			StatusCode:  200,
			Headers: map[string]string{
				"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, intent.Filename),
			},
		})
	} else {
		// Build base URL for metadata URL rewriting (npm tarball URLs, etc.)
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		baseURL := fmt.Sprintf("%s://%s/repo/%s", scheme, c.Request.Host, repoName)

		var contentResult *types.ContentResult
		var err error
		if repo.Type == model.RepoTypeVirtual {
			// 虚拟仓库没有 RemoteURL 也没有本地包数据，需要遍历成员仓库处理元数据请求
			ctx := context.WithValue(c.Request.Context(), "repo", repo)
			ctx = context.WithValue(ctx, adapter.BaseURLCtxKey{}, baseURL)
			contentResult, err = r.resolver.ResolveMetadata(ctx, repo, intent, adp)
		} else {
			ctx := context.WithValue(c.Request.Context(), "repo", repo)
			ctx = context.WithValue(ctx, adapter.BaseURLCtxKey{}, baseURL)
			contentResult, err = adp.HandleGet(ctx, repo, intent)
		}
		if err != nil {
			r.writeRepoError(c, repo, intent, err)
			return
		}
		r.formatContentResponse(c, contentResult)
	}

	// 审计日志（下载请求）
	if intent.Type == types.RequestDownload && r.auditSvc != nil {
		r.auditSvc.LogWithStatus(c.Request.Context(), nil, model.ActionPackageDownload, pkgType, nil, intent.Name, intent.Version, 0, 0)
	}
}

func (r *RepoRouter) formatContentResponse(c *gin.Context, result *types.ContentResult) {
	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	for key, value := range result.Headers {
		c.Header(key, value)
	}

	if result.Content != nil {
		defer result.Content.Close()
		c.DataFromReader(result.StatusCode, result.Size, result.ContentType, result.Content, nil)
		return
	}

	if result.ExtraData != nil {
		if xmlBody, ok := result.ExtraData["xml_body"]; ok {
			if body, ok := xmlBody.([]byte); ok {
				contentType := result.ContentType
				if contentType == "" {
					contentType = "application/xml"
				}
				c.Data(result.StatusCode, contentType, body)
				return
			}
		}
		if xmlStruct, ok := result.ExtraData["xml_struct"]; ok {
			b, err := xml.MarshalIndent(xmlStruct, "", "  ")
			if err == nil {
				buf := bytes.NewBufferString(xml.Header)
				buf.Write(b)
				contentType := result.ContentType
				if contentType == "" {
					contentType = "application/xml"
				}
				c.Data(result.StatusCode, contentType, buf.Bytes())
				return
			}
		}
		c.JSON(result.StatusCode, result.ExtraData)
		return
	}

	c.Status(result.StatusCode)
}

func (r *RepoRouter) writeRepoError(c *gin.Context, repo *model.Repository, intent *types.RequestIntent, err error) {
	statusCode, message, category := mapRepoError(err)
	fields := []any{
		"status", statusCode,
		"category", category,
		"error", err,
	}
	if repo != nil {
		fields = append(fields, "repo", repo.Name, "repo_type", repo.Type, "package_type", repo.PackageType)
	}
	if intent != nil {
		fields = append(fields, "request_type", intent.Type, "package", intent.Name, "version", intent.Version, "filename", intent.Filename)
	}

	if statusCode >= http.StatusInternalServerError {
		logrus.WithFields(slogFields(fields)).Error("repository request failed")
	} else {
		logrus.WithFields(slogFields(fields)).Warn("repository request failed")
	}

	response.ErrorResponse(c, statusCode, message)
}

func mapRepoError(err error) (int, string, string) {
	if err == nil {
		return http.StatusInternalServerError, "internal server error", "internal"
	}

	var remoteErr *proxy.RemoteError
	if errors.As(err, &remoteErr) {
		switch {
		case remoteErr.StatusCode == http.StatusNotFound:
			return http.StatusNotFound, "package not found", "not_found"
		case remoteErr.StatusCode == http.StatusUnauthorized || remoteErr.StatusCode == http.StatusForbidden:
			return http.StatusBadGateway, "upstream authentication failed", "upstream_auth"
		case remoteErr.StatusCode == http.StatusTooManyRequests:
			return http.StatusTooManyRequests, "upstream rate limited", "upstream_rate_limited"
		case remoteErr.StatusCode >= http.StatusInternalServerError:
			return http.StatusBadGateway, "upstream registry error", "upstream"
		default:
			return http.StatusBadGateway, "upstream registry request failed", "upstream"
		}
	}

	if errors.Is(err, proxy.ErrPackageNotFound) || apperr.IsNotFound(err) || strings.Contains(err.Error(), "package not found") {
		return http.StatusNotFound, "package not found", "not_found"
	}
	if errors.Is(err, context.Canceled) {
		return 499, "request cancelled", "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, "request timeout", "timeout"
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return http.StatusGatewayTimeout, "network timeout", "timeout"
	}
	if strings.Contains(err.Error(), "circuit breaker open") {
		return http.StatusServiceUnavailable, "repository temporarily unavailable", "circuit_breaker"
	}
	if strings.Contains(err.Error(), "remote URL cannot be empty") || strings.Contains(err.Error(), "fetcher not configured") {
		return http.StatusServiceUnavailable, "repository upstream is not configured", "configuration"
	}

	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Code, appErr.Message, string(appErr.Category)
	}

	return http.StatusInternalServerError, "internal server error", "internal"
}

func (r *RepoRouter) HandlePublish(c *gin.Context) {
	repoName := c.Param("repoName")

	repo, err := r.getRepoContext(c.Request.Context(), repoName)
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

	publishResult, err := r.resolver.HandleRepoPublish(c, repo)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if r.uploadSvc == nil {
		response.NotFound(c, "上传服务未初始化")
		return
	}

	uploadCtx := &service.UploadContext{
		PkgType:        string(repo.PackageType),
		Name:           publishResult.PackageName,
		StorageName:    publishResult.StorageName,
		Version:        publishResult.Version,
		StorageVersion: publishResult.StorageVersion,
		Filename:       publishResult.Filename,
		Content:        publishResult.Content,
		Size:           publishResult.Size,
		PackageType:    model.PackageType(repo.PackageType),
		RepositoryType: repo.Type,
		RepositoryID:   repo.ID,
		UploadedBy:     c.GetUint("userID"),
		Metadata:       publishResult.Metadata,
		Dependencies:   publishResult.Dependencies,
		FileType:       publishResult.FileType,
		DownloadURL:    publishResult.DownloadURL,
		RepoName:       repo.Name,
	}
	if repo.StorageBackendID != nil {
		uploadCtx.StorageBackendID = *repo.StorageBackendID
	}

	uploadResult, err := r.uploadSvc.Upload(c.Request.Context(), uploadCtx)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if r.auditSvc != nil {
		r.auditSvc.LogWithStatus(c.Request.Context(), nil, model.ActionPackageUpload, string(repo.PackageType), &uploadResult.PackageID, publishResult.PackageName, "", 0, 0)
	}

	result := &types.RepoOperationResult{
		PackageName: publishResult.PackageName,
		Version:     publishResult.Version,
		Size:        uploadResult.Size,
		Filename:    publishResult.Filename,
		Response:    publishResult.Response,
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

	repo, err := r.getRepoContext(c.Request.Context(), repoName)
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

	if r.auditSvc != nil && result != nil {
		userID := c.GetUint("userID")
		var uid *uint
		if userID > 0 {
			uid = &userID
		}
		details := fmt.Sprintf(`{"repo":"%s","name":"%s","version":"%s"}`, repo.Name, result.PackageName, result.Version)
		r.auditSvc.LogWithRequestAndStatus(
			c.Request.Context(),
			uid,
			model.ActionPackageDelete,
			"package",
			nil,
			result.PackageName,
			details,
			c.ClientIP(),
			c.Request.UserAgent(),
			200,
			0,
		)
	}
}

func slogFields(args []any) logrus.Fields {
	fields := logrus.Fields{}
	for i := 0; i < len(args)-1; i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		fields[key] = args[i+1]
	}
	return fields
}
