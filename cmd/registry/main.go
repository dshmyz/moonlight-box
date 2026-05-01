package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moonlight-box/registry/internal/adapter"
	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/handler"
	"github.com/moonlight-box/registry/internal/middleware"
	"github.com/moonlight-box/registry/internal/migration"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gin-gonic/gin"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "config file path")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Moonlight Registry v%s (built: %s)\n", version, buildTime)
		return
	}

	fmt.Printf("Moonlight Registry v%s\n", version)
	fmt.Println("Starting server...")

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		fmt.Println("Using default configuration...")
		// 使用默认配置继续
		cfg, err = config.Load("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load default config: %v\n", err)
			os.Exit(1)
		}
	}

	// 初始化数据库
	if err := database.Initialize(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// 自动迁移
	if err := database.AutoMigrate(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to migrate database: %v\n", err)
		os.Exit(1)
	}

	// 种子数据
	if err := database.SeedData(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seed data: %v\n", err)
		os.Exit(1)
	}

	// 初始化系统配置
	systemConfigRepo := repository.NewSystemConfigRepository(database.GetDB())
	configInitializer := service.NewConfigInitializer(systemConfigRepo)
	if err := configInitializer.InitializeDefaultConfigs(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize configs: %v\n", err)
		os.Exit(1)
	}

	// 设置 Gin 模式
	gin.SetMode(cfg.Server.Mode)

	// 初始化仓库
	userRepo := repository.NewUserRepository(database.GetDB())
	roleRepo := repository.NewRoleRepository(database.GetDB())
	packageRepo := repository.NewPackageRepository(database.GetDB())

	// 初始化服务
	authService := service.NewAuthService(userRepo, roleRepo, &cfg.Auth)
	auditSvc := service.NewAuditService()

	// 初始化仓库管理和缓存服务
	db := database.GetDB()
	repoRepo := repository.NewRepositoryRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	blockRuleRepo := repository.NewBlockRuleRepository(db)
	storageBackendRepo := repository.NewStorageBackendRepository(db)
	cacheSvc := proxy.NewCacheService()
	dnsResolver := proxy.NewDNSResolver(cfg.Proxy.DNSMapping)
	tm := proxy.NewTransportManager(cfg.Proxy.ConnectTimeout, dnsResolver)
	remoteClient := proxy.NewRemoteClient(tm, cfg.Proxy.MaxRedirects)

	// 初始化存储服务（依赖于 storageBackendRepo）
	storageSvc, err := service.NewStorageService(storageBackendRepo, cfg.Storage.Local.BasePath, int64(cfg.Storage.Local.MaxSizeGB))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize storage service: %v\n", err)
		os.Exit(1)
	}

	// 初始化适配器（先创建，用于构建 adapter map）
	npmAdapter := adapter.NewNpmAdapter(packageRepo, storageSvc, auditSvc, nil)
	mavenAdapter := adapter.NewMavenAdapter(packageRepo, storageSvc, auditSvc, nil)
	pypiAdapter := adapter.NewPyPIAdapter(packageRepo, storageSvc, auditSvc, nil)
	goAdapter := adapter.NewGoAdapter(packageRepo, storageSvc, auditSvc, nil)
	nugetAdapter := adapter.NewNuGetAdapter(packageRepo, storageSvc, auditSvc)
	genericAdapter := adapter.NewGenericAdapter(packageRepo, storageSvc, auditSvc)
	yumAdapter := adapter.NewYumAdapter(packageRepo, storageSvc, auditSvc, nil)
	aptAdapter := adapter.NewAptAdapter(packageRepo, storageSvc, auditSvc)

	// 构建 adapter map 用于 ProxyRouter
	adapterMap := map[string]types.Adapter{
		string(types.NpmType):     npmAdapter,
		string(types.MavenType):   mavenAdapter,
		string(types.PyPIType):    pypiAdapter,
		string(types.GoType):      goAdapter,
		string(types.NuGetType):   nugetAdapter,
		string(types.GenericType): genericAdapter,
		string(types.YumType):     yumAdapter,
		string(types.AptType):     aptAdapter,
	}

	// 创建 ProxyRouter 实例
	proxyRouter := proxy.NewProxyRouter(db, cacheSvc, remoteClient, repoRepo, groupRepo, adapterMap)

	// 注入 ProxyRouter 到需要代理的适配器
	npmAdapter.SetProxyRouter(proxyRouter)
	mavenAdapter.SetProxyRouter(proxyRouter)
	pypiAdapter.SetProxyRouter(proxyRouter)
	goAdapter.SetProxyRouter(proxyRouter)
	yumAdapter.SetProxyRouter(proxyRouter)

	adapters := []adapter.Adapter{
		npmAdapter,
		mavenAdapter,
		pypiAdapter,
		goAdapter,
		nugetAdapter,
		genericAdapter,
		yumAdapter,
		aptAdapter,
	}

	repoSvc := service.NewRepositoryService(repoRepo, groupRepo, db)
	blockRuleSvc := service.NewBlockRuleService(blockRuleRepo, auditSvc)
	repoHandler := handler.NewRepositoryHandler(repoSvc)
	cacheHandler := handler.NewCacheHandler(cacheSvc)
	blockRuleHandler := handler.NewBlockRuleHandler(blockRuleSvc, auditSvc)
	publicRepoHandler := handler.NewPublicRepoHandler(repoSvc)

	// 初始化存储后端管理服务
	storageBackendSvc := service.NewStorageBackendService(storageBackendRepo)
	storageBackendHandler := handler.NewStorageBackendHandler(storageBackendSvc)

	// 初始化搜索和 Dashboard 服务
	searchSvc := service.NewPackageSearchService(db)
	searchHandler := handler.NewPackageSearchHandler(searchSvc)

	dashboardSvc := service.NewDashboardService(db, repoRepo, cfg.Storage.Local.BasePath)
	dashboardHandler := handler.NewDashboardHandler(dashboardSvc)

	// 初始化用户和审计日志管理
	auditRepo := repository.NewAuditRepository(db)
	auditLogHandler := handler.NewAuditLogHandler(auditRepo)
	userHandler := handler.NewUserHandler(userRepo, roleRepo)
	pkgVersionHandler := handler.NewPackageVersionHandler(packageRepo)

	// 初始化 CAS 认证服务
	casConfigRepo := repository.NewCASConfigRepository(db)
	casConfigSvc := service.NewCASConfigService(casConfigRepo)
	casSvc := service.NewCASService(&cfg.Auth, userRepo, roleRepo, authService)
	casSvc.SetCASConfigService(casConfigSvc)
	casHandler := handler.NewCASHandler(casSvc)
	casAdminHandler := handler.NewCASAdminHandler(casConfigSvc)

	// 初始化安全扫描服务
	scanRepo := repository.NewScanRepository(db)
	scanner := service.NewSecurityScanner(scanRepo, packageRepo, blockRuleRepo)
	securityHandler := handler.NewSecurityHandler(scanner)

	// 初始化备份服务
	backupRepo := repository.NewBackupRepository(db)
	backupSvc := service.NewBackupService(backupRepo, cfg.Storage.Local.BasePath, cfg.Storage.Local.BasePath+"/backups")
	backupHandler := handler.NewBackupHandler(backupSvc)

	// 初始化 Webhook 服务
	webhookRepo := repository.NewWebhookRepository(db)
	webhookSvc := service.NewWebhookService(webhookRepo)
	webhookHandler := handler.NewWebhookHandler(webhookSvc)

	// 初始化系统配置服务
	systemConfigSvc := service.NewSystemConfigService(systemConfigRepo)
	systemConfigHandler := handler.NewSystemConfigHandler(systemConfigSvc)
	systemInfoHandler := handler.NewSystemInfoHandler(version, buildTime, time.Now().Unix())

	// 初始化文件浏览服务
	fileBrowseHandler := handler.NewFileBrowseHandler(cfg.Storage.Local.BasePath)

	// 初始化调度器服务
	schedulerSvc := service.NewSchedulerService(backupSvc, systemConfigSvc, webhookSvc)

	// 初始化迁移服务
	migrationSvc := migration.NewMigrationService(db)
	migrationWorker := migration.NewMigrationWorker(migrationSvc, 5)
	migrationHandler := handler.NewMigrationHandler(migrationSvc, migrationWorker)

	// 创建路由器
	router := setupRouter(cfg, authService, adapters, repoHandler, cacheHandler, blockRuleHandler, searchHandler, dashboardHandler, casHandler, casAdminHandler, blockRuleSvc, storageBackendHandler, securityHandler, auditLogHandler, userHandler, pkgVersionHandler, roleRepo, publicRepoHandler, backupHandler, webhookHandler, systemConfigHandler, systemInfoHandler, fileBrowseHandler, repoSvc, migrationHandler)

	// 设置 Webhook 服务到适配器
	for _, adap := range adapters {
		if webhookAware, ok := adap.(interface{ SetWebhookService(*service.WebhookService) }); ok {
			webhookAware.SetWebhookService(webhookSvc)
		}
	}

	// 启动调度器
	if err := schedulerSvc.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start scheduler: %v\n", err)
	}
	defer schedulerSvc.Stop()

	// 创建 HTTP 服务器
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 优雅关闭
	go func() {
		fmt.Printf("Server listening on %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server forced to shutdown: %v\n", err)
	}

	fmt.Println("Server exited")
}

func setupRouter(cfg *config.Config, authService *service.AuthService, adapters []adapter.Adapter, repoHandler *handler.RepositoryHandler, cacheHandler *handler.CacheHandler, blockRuleHandler *handler.BlockRuleHandler, searchHandler *handler.PackageSearchHandler, dashboardHandler *handler.DashboardHandler, casHandler *handler.CASHandler, casAdminHandler *handler.CASAdminHandler, blockRuleSvc *service.BlockRuleService, storageBackendHandler *handler.StorageBackendHandler, securityHandler *handler.SecurityHandler, auditLogHandler *handler.AuditLogHandler, userHandler *handler.UserHandler, pkgVersionHandler *handler.PackageVersionHandler, roleRepo *repository.RoleRepository, publicRepoHandler *handler.PublicRepoHandler, backupHandler *handler.BackupHandler, webhookHandler *handler.WebhookHandler, systemConfigHandler *handler.SystemConfigHandler, systemInfoHandler *handler.SystemInfoHandler, fileBrowseHandler *handler.FileBrowseHandler, repoSvc *service.RepositoryService, migrationHandler *handler.MigrationHandler) *gin.Engine {
	r := gin.New()

	// 全局中间件
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS())
	r.Use(middleware.PrometheusMiddleware())
	r.Use(gin.Logger())

	// 初始化 auth handler
	authHandler := handler.NewAuthHandler(authService)

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": version,
		})
	})

	// Prometheus 指标端点
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API 路由组
	api := r.Group("/api/v1")
	{
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong"})
		})

		// 包搜索（公开）
		api.GET("/packages/search", searchHandler.Search)

		// 公开路由 (无需认证)
		public := api.Group("/auth")
		{
			public.POST("/login", authHandler.Login)
			public.POST("/refresh", authHandler.RefreshToken)
		}

		// 公开仓库配置（无需认证）
		api.GET("/public/repo/:name", publicRepoHandler.GetRepoConfig)

		// CAS 认证（公开路由）
		api.GET("/auth/cas/login", casHandler.Login)
		api.GET("/auth/cas/callback", casHandler.Callback)
		api.GET("/auth/cas/config", casHandler.Config)

		// 受保护路由 (需要认证)
		protected := api.Group("")
		protected.Use(middleware.Auth(authService))
		{
			protected.POST("/auth/logout", authHandler.Logout)
			protected.GET("/auth/profile", authHandler.Profile)

			// 仓库管理
			repos := protected.Group("/repositories")
			repos.Use(middleware.RequirePermission(roleRepo, "repositories", "read"))
			{
				repos.GET("", repoHandler.List)
				repos.GET("/:name", repoHandler.Get)
				repos.GET("/:name/members", repoHandler.GetMembers)
			}
			reposWrite := protected.Group("/repositories")
			reposWrite.Use(middleware.RequirePermission(roleRepo, "repositories", "write"))
			{
				reposWrite.POST("", repoHandler.Create)
				reposWrite.PUT("/:name", repoHandler.Update)
				reposWrite.POST("/:name/members", repoHandler.AddMember)
			}
			reposDelete := protected.Group("/repositories")
			reposDelete.Use(middleware.RequirePermission(roleRepo, "repositories", "delete"))
			{
				reposDelete.DELETE("/:name", repoHandler.Delete)
				reposDelete.DELETE("/:name/members/:memberName", repoHandler.RemoveMember)
			}

			// 缓存管理
			cache := protected.Group("/cache")
			cache.Use(middleware.RequirePermission(roleRepo, "cache", "read"))
			{
				cache.GET("/stats", cacheHandler.GetStats)
			}
			cacheWrite := protected.Group("/cache")
			cacheWrite.Use(middleware.RequirePermission(roleRepo, "cache", "write"))
			{
				cacheWrite.DELETE("", cacheHandler.Clear)
				cacheWrite.POST("/invalidate", cacheHandler.Invalidate)
			}

			// 阻断规则管理
			blockRules := protected.Group("/block-rules")
			blockRules.Use(middleware.RequirePermission(roleRepo, "block-rules", "read"))
			{
				blockRules.GET("", blockRuleHandler.List)
				blockRules.GET("/logs", blockRuleHandler.ListBlockLogs)
				blockRules.GET("/template", blockRuleHandler.DownloadTemplate)
			}
			blockRulesWrite := protected.Group("/block-rules")
			blockRulesWrite.Use(middleware.RequirePermission(roleRepo, "block-rules", "write"))
			{
				blockRulesWrite.POST("", blockRuleHandler.Create)
				blockRulesWrite.POST("/batch-import", blockRuleHandler.BatchImport)
				blockRulesWrite.PUT("/:id", blockRuleHandler.Update)
			}
			blockRulesDelete := protected.Group("/block-rules")
			blockRulesDelete.Use(middleware.RequirePermission(roleRepo, "block-rules", "delete"))
			{
				blockRulesDelete.DELETE("/:id", blockRuleHandler.Delete)
			}

			// CAS 配置管理
			casAdmin := protected.Group("/cas/config")
			casAdmin.Use(middleware.RequirePermission(roleRepo, "system", "admin"))
			{
				casAdmin.GET("", casAdminHandler.GetConfig)
				casAdmin.PUT("", casAdminHandler.UpdateConfig)
				casAdmin.DELETE("", casAdminHandler.DeleteConfig)
			}

			// 存储后端管理
			storageBackends := protected.Group("/storage-backends")
			storageBackends.Use(middleware.RequirePermission(roleRepo, "storage-backends", "read"))
			{
				storageBackends.GET("", storageBackendHandler.List)
				storageBackends.GET("/:id", storageBackendHandler.Get)
				storageBackends.POST("/test", storageBackendHandler.TestConnection)
			}
			storageBackendsWrite := protected.Group("/storage-backends")
			storageBackendsWrite.Use(middleware.RequirePermission(roleRepo, "storage-backends", "write"))
			{
				storageBackendsWrite.POST("", storageBackendHandler.Create)
				storageBackendsWrite.PUT("/:id", storageBackendHandler.Update)
				storageBackendsWrite.POST("/:id/default", storageBackendHandler.SetDefault)
			}
			storageBackendsDelete := protected.Group("/storage-backends")
			storageBackendsDelete.Use(middleware.RequirePermission(roleRepo, "storage-backends", "write"))
			{
				storageBackendsDelete.DELETE("/:id", storageBackendHandler.Delete)
			}

			// 安全扫描
			security := protected.Group("/security")
			security.Use(middleware.RequirePermission(roleRepo, "security", "read"))
			{
				security.GET("/vulnerabilities", securityHandler.ListVulnerabilities)
				security.GET("/statistics", securityHandler.GetSecurityStats)
				security.GET("/dashboard", securityHandler.GetDashboard)
				security.GET("/packages/:id/scan", securityHandler.GetScanResult)
			}
			securityWrite := protected.Group("/security")
			securityWrite.Use(middleware.RequirePermission(roleRepo, "security", "write"))
			{
				securityWrite.POST("/scan/full", securityHandler.TriggerFullScan)
				securityWrite.POST("/block/:cve", securityHandler.BlockByCVE)
				securityWrite.POST("/packages/:id/scan/trigger", securityHandler.TriggerScan)
			}

			// 用户管理
			users := protected.Group("/users")
			users.Use(middleware.RequirePermission(roleRepo, "users", "read"))
			{
				users.GET("", userHandler.List)
			}
			usersWrite := protected.Group("/users")
			usersWrite.Use(middleware.RequirePermission(roleRepo, "users", "write"))
			{
				usersWrite.POST("", userHandler.Create)
				usersWrite.PUT("/:id/status", userHandler.UpdateStatus)
				usersWrite.PUT("/:id/roles", userHandler.AssignRoles)
			}
			protected.GET("/roles", middleware.RequirePermission(roleRepo, "users", "read"), userHandler.ListRoles)

			// 审计日志
			audit := protected.Group("/audit")
			audit.Use(middleware.RequirePermission(roleRepo, "audit", "read"))
			{
				audit.GET("/logs", auditLogHandler.List)
				audit.GET("/logs/:id", auditLogHandler.Get)
			}

			// 包版本管理
			protected.GET("/packages/:type/:name/versions", pkgVersionHandler.ListVersions)
			protected.POST("/packages/versions/:id/deprecate", middleware.RequirePermission(roleRepo, "npm", "write"), pkgVersionHandler.DeprecateVersion)
			protected.POST("/packages/versions/:id/restore", middleware.RequirePermission(roleRepo, "npm", "write"), pkgVersionHandler.RestoreVersion)
			protected.POST("/packages/versions/:id/yank", middleware.RequirePermission(roleRepo, "npm", "write"), pkgVersionHandler.YankVersion)
			protected.DELETE("/packages/versions/:id", middleware.RequirePermission(roleRepo, "npm", "delete"), pkgVersionHandler.DeleteVersion)

			// Dashboard 统计（需要认证）
			protected.GET("/dashboard/stats", dashboardHandler.GetStats)

			// 备份管理
			backups := protected.Group("/backups")
			backups.Use(middleware.RequirePermission(roleRepo, "system", "admin"))
			{
				backups.GET("", backupHandler.List)
				backups.GET("/:id", backupHandler.Get)
				backups.POST("", backupHandler.Create)
				backups.POST("/:id/restore", backupHandler.Restore)
				backups.DELETE("/:id", backupHandler.Delete)
			}

			// Webhook 管理
			webhooks := protected.Group("/webhooks")
			webhooks.Use(middleware.RequirePermission(roleRepo, "webhooks", "read"))
			{
				webhooks.GET("", webhookHandler.List)
				webhooks.GET("/:id", webhookHandler.Get)
				webhooks.GET("/:id/deliveries", webhookHandler.ListDeliveries)
			}
			webhooksWrite := protected.Group("/webhooks")
			webhooksWrite.Use(middleware.RequirePermission(roleRepo, "webhooks", "write"))
			{
				webhooksWrite.POST("", webhookHandler.Create)
				webhooksWrite.PUT("/:id", webhookHandler.Update)
				webhooksWrite.POST("/:id/test", webhookHandler.Test)
				webhooksWrite.DELETE("/:id", webhookHandler.Delete)
			}

			// 系统配置管理
			configs := protected.Group("/configs")
			configs.Use(middleware.RequirePermission(roleRepo, "system", "admin"))
			{
				configs.GET("", systemConfigHandler.List)
				configs.GET("/:key", systemConfigHandler.Get)
				configs.POST("", systemConfigHandler.Set)
				configs.DELETE("/:key", systemConfigHandler.Delete)
			}

			// 系统信息
			protected.GET("/system/info", systemInfoHandler.GetInfo)

			// 文件浏览
			files := protected.Group("/files")
			files.Use(middleware.RequirePermission(roleRepo, "system", "admin"))
			{
				files.GET("/browse", fileBrowseHandler.ListDirectory)
				files.GET("/stats", fileBrowseHandler.GetFileStats)
				files.GET("/download", fileBrowseHandler.DownloadFile)
			}

			// 数据迁移
			migrationGroup := protected.Group("/migration")
			migrationGroup.Use(middleware.RequirePermission(roleRepo, "system", "admin"))
			{
				migrationGroup.GET("", migrationHandler.ListMigrations)
				migrationGroup.POST("/nexus/test", migrationHandler.TestNexusConnection)
				migrationGroup.POST("/nexus/repositories", migrationHandler.ListNexusRepositories)
				migrationGroup.POST("/nexus", migrationHandler.CreateMigration)
				migrationGroup.GET("/:id/status", migrationHandler.GetMigrationStatus)
				migrationGroup.POST("/:id/cancel", migrationHandler.CancelMigration)
			}
		}
	}

	// 注册统一仓库路由
	repoRouter := handler.NewRepoRouter(repoHandler.Service())
	for _, adap := range adapters {
		if repoAware, ok := adap.(adapter.RepoAwareAdapter); ok {
			repoRouter.RegisterAdapter(string(adap.Type()), repoAware)
		}
	}

	authMw := middleware.Auth(authService)
	blockMw := middleware.BlockCheck(blockRuleSvc, repoSvc)
	permMw := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(roleRepo, resource, action)
	}

	repoGroup := r.Group("/repo/:repoName")
	repoGroup.Use(blockMw)
	{
		repoGroup.GET("/*path", repoRouter.HandleRequest)

		publishGroup := repoGroup.Group("")
		publishGroup.Use(authMw, permMw("npm", "write"))
		{
			publishGroup.PUT("/*path", repoRouter.HandlePublish)
		}

		deleteGroup := repoGroup.Group("")
		deleteGroup.Use(authMw, permMw("npm", "delete"))
		{
			deleteGroup.DELETE("/*path", repoRouter.HandleDelete)
		}
	}

	return r
}
