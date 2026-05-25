package main

import (
	"compress/gzip"
	"net/http"

	handler "github.com/dshmyz/moonlight-box/internal/api/http"
	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/middleware"
	"github.com/dshmyz/moonlight-box/internal/proxy"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type RouterContext struct {
	Config           *config.Config
	AuthSvc          *service.AuthService
	AuditSvc         *service.AuditService
	PermCache        *service.PermissionCacheService
	BlockRule        *service.BlockRuleService
	RepoSvc          *service.RepositoryService
	RepoCache        *proxy.RepositoryCache
	WebhookSvc       *service.WebhookService
	RepositoryRouter http.Handler
	Handlers         struct {
		Auth             *handler.AuthHandler
		Repo             *handler.RepositoryHandler
		PublicRepo       *handler.PublicRepoHandler
		Cache            *handler.CacheHandler
		BlockRule        *handler.BlockRuleHandler
		Search           *handler.PackageSearchHandler
		Dashboard        *handler.DashboardHandler
		CAS              *handler.CASHandler
		StorageBackend   *handler.StorageBackendHandler
		Security         *handler.SecurityHandler
		AuditLog         *handler.AuditLogHandler
		User             *handler.UserHandler
		Role             *handler.RoleHandler
		Backup           *handler.BackupHandler
		BackupConfig     *handler.BackupConfigHandler
		Webhook          *handler.WebhookHandler
		SystemConfig     *handler.SystemConfigHandler
		SystemInfo       *handler.SystemInfoHandler
		FileBrowse       *handler.FileBrowseHandler
		Migration        *handler.MigrationHandler
		AI               *handler.AIHandler
		ProxyDownloadLog *handler.ProxyDownloadLogHandler
		HealthCheck      *handler.HealthCheckHandler
		VulnRule         *handler.VulnRuleHandler
		PackageVersion   *handler.PackageVersionHandler
	}
}

func NewRouterContext(
	cfg *config.Config,
	authSvc *service.AuthService,
	auditSvc *service.AuditService,
	permCache *service.PermissionCacheService,
	blockRule *service.BlockRuleService,
	repoSvc *service.RepositoryService,
	repositoryRouter http.Handler,
	webhookSvc *service.WebhookService,
) *RouterContext {
	ctx := &RouterContext{
		Config: cfg, AuthSvc: authSvc, AuditSvc: auditSvc, PermCache: permCache,
		BlockRule: blockRule, RepoSvc: repoSvc,
		RepositoryRouter: repositoryRouter, WebhookSvc: webhookSvc,
	}

	ctx.Handlers.Auth = handler.NewAuthHandler(authSvc, auditSvc)

	return ctx
}

func (ctx *RouterContext) SetupRouter(version string) *gin.Engine {
	r := gin.New()

	r.Use(middleware.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS())
	r.Use(middleware.PrometheusMiddleware())
	r.Use(middleware.Gzip(gzip.DefaultCompression))
	if ctx.Config != nil && ctx.Config.Server.MaxUploadSize > 0 {
		r.Use(middleware.BodySizeLimit(ctx.Config.Server.MaxUploadSize))
	}
	r.Use(gin.Logger())

	ctx.setupPublicRoutes(r, version)
	ctx.setupAPIRoutes(r)

	return r
}

func (ctx *RouterContext) setupPublicRoutes(r *gin.Engine, version string) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": version,
		})
	})

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/docs/*filepath", serveEmbeddedDocs)
}

func (ctx *RouterContext) setupAPIRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong"})
		})

		ctx.setupPackagePublicRoutes(api)
		ctx.setupAuthPublicRoutes(api)
		ctx.setupProtectedRoutes(api)
	}

	ctx.setupRepoRoutes(r, ctx.RepoCache)
	setupFrontendRouter(r, ctx.Config.Server.StaticDir)
}

func (ctx *RouterContext) setupPackagePublicRoutes(api *gin.RouterGroup) {
	api.GET("/packages/search", ctx.Handlers.Search.Search)
	api.GET("/packages/:type/versions", ctx.Handlers.PackageVersion.ListVersions)
	api.GET("/public/repo/:name", ctx.Handlers.PublicRepo.GetRepoConfig)
	api.GET("/public/repositories", ctx.Handlers.PublicRepo.List)
}

func (ctx *RouterContext) setupAuthPublicRoutes(api *gin.RouterGroup) {
	public := api.Group("/auth")
	{
		public.POST("/login", ctx.Handlers.Auth.Login)
		public.POST("/refresh", ctx.Handlers.Auth.RefreshToken)
	}

	api.GET("/auth/cas/login", ctx.Handlers.CAS.Login)
	api.GET("/auth/cas/callback", ctx.Handlers.CAS.Callback)
	api.GET("/auth/cas/config", ctx.Handlers.CAS.Config)
}

func (ctx *RouterContext) setupProtectedRoutes(api *gin.RouterGroup) {
	protected := api.Group("")
	protected.Use(middleware.Auth(ctx.AuthSvc))
	{
		ctx.setupAuthProtectedRoutes(protected)
		ctx.setupRepositoryRoutes(protected)
		ctx.setupCacheRoutes(protected)
		ctx.setupBlockRuleRoutes(protected)
		ctx.setupStorageRoutes(protected)
		ctx.setupSecurityRoutes(protected)
		ctx.setupUserRoutes(protected)
		ctx.setupAuditRoutes(protected)
		ctx.setupBackupRoutes(protected)
		ctx.setupWebhookRoutes(protected)
		ctx.setupSystemRoutes(protected)
		ctx.setupMigrationRoutes(protected)
		ctx.setupAIRoutes(protected)
		ctx.setupHealthRoutes(protected)

		protected.GET("/dashboard/stats", ctx.Handlers.Dashboard.GetStats)

		ctx.setupPackageRoutes(protected)
	}
}

func (ctx *RouterContext) setupAuthProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/auth/logout", ctx.Handlers.Auth.Logout)
	protected.GET("/auth/profile", ctx.Handlers.Auth.Profile)
	protected.PUT("/auth/profile", ctx.Handlers.Auth.UpdateProfile)
	protected.PUT("/auth/password", ctx.Handlers.Auth.ChangePassword)
}

func (ctx *RouterContext) setupRepositoryRoutes(protected *gin.RouterGroup) {
	repos := protected.Group("/repositories")
	repos.Use(ctx.requirePermission("repositories", "read"))
	{
		repos.GET("", ctx.Handlers.Repo.List)
		repos.GET("/:name", ctx.Handlers.Repo.Get)
		repos.GET("/:name/members", ctx.Handlers.Repo.GetMembers)
	}

	reposWrite := protected.Group("/repositories")
	reposWrite.Use(ctx.requirePermission("repositories", "write"))
	{
		reposWrite.POST("", ctx.Handlers.Repo.Create)
		reposWrite.PUT("/:name", ctx.Handlers.Repo.Update)
		reposWrite.POST("/:name/members", ctx.Handlers.Repo.AddMember)
	}

	reposDelete := protected.Group("/repositories")
	reposDelete.Use(ctx.requirePermission("repositories", "delete"))
	{
		reposDelete.DELETE("/:name", ctx.Handlers.Repo.Delete)
		reposDelete.DELETE("/:name/members/:memberName", ctx.Handlers.Repo.RemoveMember)
	}
}

func (ctx *RouterContext) setupCacheRoutes(protected *gin.RouterGroup) {
	cache := protected.Group("/cache")
	cache.Use(ctx.requirePermission("cache", "read"))
	{
		cache.GET("/caches", ctx.Handlers.Cache.ListCaches)
		cache.GET("/stats", ctx.Handlers.Cache.GetStats)
		cache.GET("/stats/:name", ctx.Handlers.Cache.GetStats)
		cache.GET("/items", ctx.Handlers.Cache.List)
		cache.GET("/caches/:name/items", ctx.Handlers.Cache.List)
	}

	cacheWrite := protected.Group("/cache")
	cacheWrite.Use(ctx.requirePermission("cache", "write"))
	{
		cacheWrite.DELETE("", ctx.Handlers.Cache.Clear)
		cacheWrite.DELETE("/caches/:name", ctx.Handlers.Cache.Clear)
		cacheWrite.DELETE("/items/:key", ctx.Handlers.Cache.DeleteItem)
		cacheWrite.DELETE("/caches/:name/items/:key", ctx.Handlers.Cache.DeleteItem)
		cacheWrite.DELETE("/expired", ctx.Handlers.Cache.CleanupExpired)
		cacheWrite.DELETE("/caches/:name/expired", ctx.Handlers.Cache.CleanupExpired)
		cacheWrite.POST("/invalidate", ctx.Handlers.Cache.Invalidate)
	}
}

func (ctx *RouterContext) setupBlockRuleRoutes(protected *gin.RouterGroup) {
	blockRules := protected.Group("/block-rules")
	blockRules.Use(ctx.requirePermission("block-rules", "read"))
	{
		blockRules.GET("", ctx.Handlers.BlockRule.List)
		blockRules.GET("/logs", ctx.Handlers.BlockRule.ListBlockLogs)
		blockRules.GET("/template", ctx.Handlers.BlockRule.DownloadTemplate)
		blockRules.GET("/stats", ctx.Handlers.BlockRule.GetBlockStats)
	}

	blockRulesWrite := protected.Group("/block-rules")
	blockRulesWrite.Use(ctx.requirePermission("block-rules", "write"))
	{
		blockRulesWrite.POST("", ctx.Handlers.BlockRule.Create)
		blockRulesWrite.POST("/batch-import", ctx.Handlers.BlockRule.BatchImport)
		blockRulesWrite.PUT("/:id", ctx.Handlers.BlockRule.Update)
	}

	blockRulesDelete := protected.Group("/block-rules")
	blockRulesDelete.Use(ctx.requirePermission("block-rules", "delete"))
	{
		blockRulesDelete.DELETE("/:id", ctx.Handlers.BlockRule.Delete)
	}
}

func (ctx *RouterContext) setupStorageRoutes(protected *gin.RouterGroup) {
	storage := protected.Group("/storage-backends")
	storage.Use(ctx.requirePermission("storage-backends", "read"))
	{
		storage.GET("", ctx.Handlers.StorageBackend.List)
		storage.GET("/:id", ctx.Handlers.StorageBackend.Get)
		storage.POST("/test", ctx.Handlers.StorageBackend.TestConnection)
	}

	storageWrite := protected.Group("/storage-backends")
	storageWrite.Use(ctx.requirePermission("storage-backends", "write"))
	{
		storageWrite.POST("", ctx.Handlers.StorageBackend.Create)
		storageWrite.PUT("/:id", ctx.Handlers.StorageBackend.Update)
		storageWrite.POST("/:id/default", ctx.Handlers.StorageBackend.SetDefault)
		storageWrite.DELETE("/:id", ctx.Handlers.StorageBackend.Delete)
	}
}

func (ctx *RouterContext) setupSecurityRoutes(protected *gin.RouterGroup) {
	security := protected.Group("/security")
	security.Use(ctx.requirePermission("security", "read"))
	{
		security.GET("/vulnerabilities", ctx.Handlers.Security.ListVulnerabilities)
		security.GET("/scan-results", ctx.Handlers.Security.ListScanResults)
		security.GET("/statistics", ctx.Handlers.Security.GetSecurityStats)
		security.GET("/dashboard", ctx.Handlers.Security.GetDashboard)
		security.GET("/packages/:id/scan", ctx.Handlers.Security.GetScanResult)
	}

	securityWrite := protected.Group("/security")
	securityWrite.Use(ctx.requirePermission("security", "write"))
	{
		securityWrite.POST("/scan/full", ctx.Handlers.Security.TriggerFullScan)
		securityWrite.POST("/block/:cve", ctx.Handlers.Security.BlockByCVE)
		securityWrite.POST("/packages/:id/scan/trigger", ctx.Handlers.Security.TriggerScan)
	}

	vulnRules := protected.Group("/security/vuln-rules")
	vulnRules.Use(ctx.requirePermission("security", "read"))
	{
		vulnRules.GET("", ctx.Handlers.VulnRule.ListRules)
		vulnRules.GET("/:id", ctx.Handlers.VulnRule.GetRule)
		vulnRules.POST("/import", ctx.Handlers.VulnRule.ImportRules)
	}

	vulnRulesWrite := protected.Group("/security/vuln-rules")
	vulnRulesWrite.Use(ctx.requirePermission("security", "write"))
	{
		vulnRulesWrite.POST("", ctx.Handlers.VulnRule.CreateRule)
		vulnRulesWrite.PUT("/:id", ctx.Handlers.VulnRule.UpdateRule)
		vulnRulesWrite.DELETE("/:id", ctx.Handlers.VulnRule.DeleteRule)
	}

	vulnSources := protected.Group("/security/vuln-sources")
	vulnSources.Use(ctx.requirePermission("security", "read"))
	{
		vulnSources.GET("", ctx.Handlers.VulnRule.ListDataSources)
	}

	vulnSourcesWrite := protected.Group("/security/vuln-sources")
	vulnSourcesWrite.Use(ctx.requirePermission("security", "write"))
	{
		vulnSourcesWrite.POST("", ctx.Handlers.VulnRule.CreateDataSource)
		vulnSourcesWrite.PUT("/:id", ctx.Handlers.VulnRule.UpdateDataSource)
		vulnSourcesWrite.DELETE("/:id", ctx.Handlers.VulnRule.DeleteDataSource)
		vulnSourcesWrite.POST("/:id/sync", ctx.Handlers.VulnRule.SyncDataSource)
		vulnSourcesWrite.POST("/sync-all", ctx.Handlers.VulnRule.SyncAllDataSources)
		vulnSourcesWrite.POST("/test", ctx.Handlers.VulnRule.TestDataSource)
	}
}

func (ctx *RouterContext) setupUserRoutes(protected *gin.RouterGroup) {
	users := protected.Group("/users")
	users.Use(ctx.requirePermission("users", "read"))
	{
		users.GET("", ctx.Handlers.User.List)
	}

	usersWrite := protected.Group("/users")
	usersWrite.Use(ctx.requirePermission("users", "write"))
	{
		usersWrite.POST("", ctx.Handlers.User.Create)
		usersWrite.PUT("/:id/status", ctx.Handlers.User.UpdateStatus)
		usersWrite.PUT("/:id/roles", ctx.Handlers.User.AssignRoles)
	}

	protected.GET("/roles", ctx.requirePermission("users", "read"), ctx.Handlers.Role.List)
	protected.GET("/roles/permissions", ctx.requirePermission("users", "read"), ctx.Handlers.Role.ListPermissions)
	protected.GET("/roles/:id", ctx.requirePermission("users", "read"), ctx.Handlers.Role.Get)

	rolesWrite := protected.Group("/roles")
	rolesWrite.Use(ctx.requirePermission("users", "write"))
	{
		rolesWrite.POST("", ctx.Handlers.Role.Create)
		rolesWrite.POST("/:id/clone", ctx.Handlers.Role.CloneRole)
		rolesWrite.PUT("/:id", ctx.Handlers.Role.Update)
		rolesWrite.DELETE("/:id", ctx.Handlers.Role.Delete)
		rolesWrite.PUT("/:id/permissions", ctx.Handlers.Role.UpdatePermissions)
	}
}

func (ctx *RouterContext) setupAuditRoutes(protected *gin.RouterGroup) {
	audit := protected.Group("/audit")
	audit.Use(ctx.requirePermission("audit", "read"))
	{
		audit.GET("/logs", ctx.Handlers.AuditLog.List)
		audit.GET("/logs/:id", ctx.Handlers.AuditLog.Get)
	}

	proxyDownloads := protected.Group("/proxy-downloads")
	proxyDownloads.Use(ctx.requirePermission("audit", "read"))
	{
		proxyDownloads.GET("/logs", ctx.Handlers.ProxyDownloadLog.List)
		proxyDownloads.GET("/stats", ctx.Handlers.ProxyDownloadLog.GetStats)
	}
}

func (ctx *RouterContext) setupBackupRoutes(protected *gin.RouterGroup) {
	backups := protected.Group("/backups")
	backups.Use(ctx.requirePermission("system", "admin"))
	{
		backups.GET("", ctx.Handlers.Backup.List)
		backups.GET("/:id", ctx.Handlers.Backup.Get)
		backups.POST("", ctx.Handlers.Backup.Create)
		backups.POST("/:id/restore", ctx.Handlers.Backup.Restore)
		backups.DELETE("/:id", ctx.Handlers.Backup.Delete)
	}

	backups.GET("/config", ctx.requirePermission("system", "admin"), ctx.Handlers.BackupConfig.GetConfig)
	backups.PUT("/config", ctx.requirePermission("system", "admin"), ctx.Handlers.BackupConfig.UpdateConfig)
}

func (ctx *RouterContext) setupWebhookRoutes(protected *gin.RouterGroup) {
	webhooks := protected.Group("/webhooks")
	webhooks.Use(ctx.requirePermission("webhooks", "read"))
	{
		webhooks.GET("", ctx.Handlers.Webhook.List)
		webhooks.GET("/:id", ctx.Handlers.Webhook.Get)
		webhooks.GET("/:id/deliveries", ctx.Handlers.Webhook.ListDeliveries)
	}

	webhooksWrite := protected.Group("/webhooks")
	webhooksWrite.Use(ctx.requirePermission("webhooks", "write"))
	{
		webhooksWrite.POST("", ctx.Handlers.Webhook.Create)
		webhooksWrite.PUT("/:id", ctx.Handlers.Webhook.Update)
		webhooksWrite.POST("/:id/test", ctx.Handlers.Webhook.Test)
		webhooksWrite.DELETE("/:id", ctx.Handlers.Webhook.Delete)
	}
}

func (ctx *RouterContext) setupSystemRoutes(protected *gin.RouterGroup) {
	configs := protected.Group("/configs")
	configs.Use(ctx.requirePermission("system", "admin"))
	{
		configs.GET("", ctx.Handlers.SystemConfig.List)
		configs.GET("/:key", ctx.Handlers.SystemConfig.Get)
		configs.POST("", ctx.Handlers.SystemConfig.BatchUpdate)
		configs.DELETE("/:key", ctx.Handlers.SystemConfig.Delete)
	}

	protected.GET("/system/info", ctx.Handlers.SystemInfo.GetInfo)

	files := protected.Group("/files")
	files.Use(ctx.requirePermission("system", "admin"))
	{
		files.GET("/backends", ctx.Handlers.FileBrowse.ListBackends)
		files.GET("/browse", ctx.Handlers.FileBrowse.ListDirectory)
		files.GET("/stats", ctx.Handlers.FileBrowse.GetFileStats)
		files.GET("/download", ctx.Handlers.FileBrowse.DownloadFile)
	}
}

func (ctx *RouterContext) setupMigrationRoutes(protected *gin.RouterGroup) {
	migration := protected.Group("/migration")
	migration.Use(ctx.requirePermission("system", "admin"))
	{
		migration.GET("/queue/status", ctx.Handlers.Migration.GetQueueStatus)
		migration.GET("", ctx.Handlers.Migration.ListMigrations)
		migration.POST("/nexus/test", ctx.Handlers.Migration.TestNexusConnection)
		migration.POST("/nexus/repositories", ctx.Handlers.Migration.ListNexusRepositories)
		migration.POST("/nexus/sync-repos", ctx.Handlers.Migration.SyncNexusRepos)
		migration.POST("/nexus/sync-config-only", ctx.Handlers.Migration.SyncConfigOnly)
		migration.POST("/nexus", ctx.Handlers.Migration.CreateMigration)
		// 用户迁移相关API
		migration.POST("/nexus/users", ctx.Handlers.Migration.ListNexusUsers)
		migration.POST("/nexus/roles", ctx.Handlers.Migration.ListNexusRoles)
		migration.POST("/nexus/sync-users", ctx.Handlers.Migration.SyncUsersFromNexus)
		migration.GET("/:id/status", ctx.Handlers.Migration.GetMigrationStatus)
		migration.POST("/:id/cancel", ctx.Handlers.Migration.CancelMigration)
		migration.POST("/:id/retry", ctx.Handlers.Migration.RetryFailedMigration)
		migration.POST("/:id/start", ctx.Handlers.Migration.StartMigration)
		migration.GET("/:id/items", ctx.Handlers.Migration.ListMigrationItems)
	}
}

func (ctx *RouterContext) setupAIRoutes(protected *gin.RouterGroup) {
	if ctx.Handlers.AI == nil {
		return
	}

	ai := protected.Group("/ai")
	{
		ai.POST("/chat", ctx.Handlers.AI.Chat)
		ai.POST("/chat/stream", ctx.Handlers.AI.StreamChat)
		ai.GET("/tools", ctx.Handlers.AI.ListTools)
		ai.DELETE("/sessions/:id", ctx.Handlers.AI.DeleteSession)
		ai.GET("/rate-limit", ctx.Handlers.AI.GetRateLimitStatus)
		ai.GET("/health", ctx.Handlers.AI.HealthCheck)

		ai.GET("/stats", ctx.requirePermission("system", "admin"), ctx.Handlers.AI.GetStats)
		ai.GET("/cache/stats", ctx.requirePermission("system", "admin"), ctx.Handlers.AI.GetCacheStats)
		ai.GET("/audit-logs", ctx.requirePermission("system", "admin"), ctx.Handlers.AI.GetAuditLogs)
	}
}

func (ctx *RouterContext) setupHealthRoutes(protected *gin.RouterGroup) {
	health := protected.Group("/health")
	health.Use(ctx.requirePermission("system", "admin"))
	{
		health.GET("/repos", ctx.Handlers.HealthCheck.GetAllHealthStatuses)
		health.GET("/repos/:id", ctx.Handlers.HealthCheck.GetHealthStatus)
		health.POST("/repos/:id/reset", ctx.Handlers.HealthCheck.ResetCircuitBreaker)
	}
}

func (ctx *RouterContext) setupRepoRoutes(r *gin.Engine, repoCache *proxy.RepositoryCache) {
	authMw := middleware.Auth(ctx.AuthSvc)
	permMw := ctx.requirePermission

	// 使用新架构 RepositoryRouter
	repoHandler := gin.WrapH(ctx.RepositoryRouter)

	// Nexus 兼容路由: /repository/:repoName/*path
	repoGroup := r.Group("/repository/:repoName")
	{
		repoGroup.GET("/*path", repoHandler)
		repoGroup.PUT("/*path", authMw, repoHandler)
		repoGroup.POST("/*path", authMw, repoHandler)
		repoGroup.DELETE("/*path", authMw, permMw("package", "delete"), repoHandler)
	}

	// Nexus 2 兼容: /content/repositories/:repoName/*path
	nexus2Group := r.Group("/content/repositories/:repoName")
	{
		nexus2Group.GET("/*path", repoHandler)
		nexus2Group.PUT("/*path", authMw, repoHandler)
		nexus2Group.DELETE("/*path", authMw, permMw("package", "delete"), repoHandler)
	}

	// 组合仓库: /content/groups/:groupName/*path
	groupGroup := r.Group("/content/groups/:groupName")
	{
		groupGroup.GET("/*path", repoHandler)
		groupGroup.PUT("/*path", authMw, repoHandler)
		groupGroup.DELETE("/*path", authMw, permMw("package", "delete"), repoHandler)
	}
}

func (ctx *RouterContext) requirePermission(resource, action string) gin.HandlerFunc {
	return middleware.RequirePermission(ctx.PermCache, resource, action)
}

func (ctx *RouterContext) setupPackageRoutes(protected *gin.RouterGroup) {
	if ctx.Handlers.PackageVersion == nil {
		return
	}

	packagesRead := protected.Group("/packages")
	packagesRead.Use(ctx.requirePermission("package", "read"))

	packageWrite := protected.Group("/packages")
	packageWrite.Use(ctx.requirePermission("package", "write"))
	{
		packageWrite.POST("/versions/:id/deprecate", ctx.Handlers.PackageVersion.DeprecateVersion)
		packageWrite.POST("/versions/:id/restore", ctx.Handlers.PackageVersion.RestoreVersion)
		packageWrite.POST("/versions/:id/yank", ctx.Handlers.PackageVersion.YankVersion)
	}

	packageDelete := protected.Group("/packages")
	packageDelete.Use(ctx.requirePermission("package", "delete"))
	{
		packageDelete.DELETE("/versions/:id", ctx.Handlers.PackageVersion.DeleteVersion)
		packageDelete.DELETE("/:id", ctx.Handlers.PackageVersion.DeletePackage)
	}
}
