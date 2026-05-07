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
	"github.com/moonlight-box/registry/internal/ai"
	"github.com/moonlight-box/registry/internal/ai/tools"
	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/handler"
	"github.com/moonlight-box/registry/internal/middleware"
	"github.com/moonlight-box/registry/internal/migration"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "config file path")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Moonlight Registry v%s (built: %s)\n", version, buildTime)
		return
	}

	logrus.WithFields(logrus.Fields{
		"version":    version,
		"build_time": buildTime,
	}).Info("Moonlight Registry starting")

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error":       err,
			"config_path": *configPath,
		}).Warn("Failed to load config file, using default configuration")
		// 不再重新加载，直接使用默认配置（已经在 Load 函数中设置好了默认值）
		cfg = config.Get()
		if cfg == nil {
			logrus.Error("Failed to get default config")
			os.Exit(1)
		}
	}

	logrus.WithFields(logrus.Fields{
		"server_port": cfg.Server.Port,
		"db_driver":   cfg.Database.Driver,
		"storage":     cfg.Storage.Backend,
		"ai_enabled":  cfg.AI.Enabled,
	}).Info("Configuration loaded")

	// 初始化日志
	if err := util.InitLogger(&cfg.Logging); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
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
	proxyDownloadLogRepo := repository.NewProxyDownloadLogRepository(database.GetDB())

	// 初始化服务
	auditSvc := service.NewAuditService()
	authService := service.NewAuthService(userRepo, roleRepo, &cfg.Auth, auditSvc)

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

	healthCheckCfg := proxy.HealthCheckConfig{
		Enabled:          cfg.Proxy.HealthCheck.Enabled,
		Interval:         cfg.Proxy.HealthCheck.Interval,
		Timeout:          cfg.Proxy.HealthCheck.Timeout,
		FailureThreshold: cfg.Proxy.HealthCheck.FailureThreshold,
	}
	if healthCheckCfg.Interval == 0 {
		healthCheckCfg.Interval = proxy.DefaultHealthCheckConfig().Interval
	}
	if healthCheckCfg.Timeout == 0 {
		healthCheckCfg.Timeout = proxy.DefaultHealthCheckConfig().Timeout
	}
	if healthCheckCfg.FailureThreshold == 0 {
		healthCheckCfg.FailureThreshold = proxy.DefaultHealthCheckConfig().FailureThreshold
	}

	healthCheckSvc := proxy.NewHealthCheckService(db, repoRepo, remoteClient, healthCheckCfg)

	// 初始化存储服务（依赖于 storageBackendRepo）
	storageSvc, err := service.NewStorageService(storageBackendRepo, cfg.Storage.Local.BasePath, int64(cfg.Storage.Local.MaxSizeGB))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize storage service: %v\n", err)
		os.Exit(1)
	}

	// 初始化下载计数批量处理器
	countBatcher := service.NewDownloadCountBatcher(packageRepo, 10*time.Second)
	defer countBatcher.Stop()

	// 创建共享的代理下载服务
	proxyDownloadSvc := service.NewProxyDownloadService(packageRepo, storageSvc, nil, proxyDownloadLogRepo, countBatcher)

	// 初始化适配器（先创建，用于构建 adapter map）
	npmAdapter := adapter.NewNpmAdapter(packageRepo, storageSvc, auditSvc, nil, proxyDownloadLogRepo, proxyDownloadSvc)
	mavenAdapter := adapter.NewMavenAdapter(packageRepo, storageSvc, auditSvc, nil, proxyDownloadLogRepo, proxyDownloadSvc)
	pypiAdapter := adapter.NewPyPIAdapter(packageRepo, repoRepo, storageSvc, auditSvc, nil, proxyDownloadLogRepo, proxyDownloadSvc)
	goAdapter := adapter.NewGoAdapter(packageRepo, storageSvc, auditSvc, nil, proxyDownloadSvc)
	genericAdapter := adapter.NewGenericAdapter(packageRepo, repoRepo, storageSvc, auditSvc)
	yumAdapter := adapter.NewYumAdapter(packageRepo, storageSvc, auditSvc, nil, proxyDownloadSvc)
	aptAdapter := adapter.NewAptAdapter(packageRepo, repoRepo, storageSvc, auditSvc, nil, proxyDownloadSvc)

	// 构建 adapter map 用于 ProxyRouter
	adapterMap := map[string]types.Adapter{
		string(types.NpmType):     npmAdapter,
		string(types.MavenType):   mavenAdapter,
		string(types.PyPIType):    pypiAdapter,
		string(types.GoType):      goAdapter,
		string(types.GenericType): genericAdapter,
		string(types.YumType):     yumAdapter,
		string(types.AptType):     aptAdapter,
	}

	// 创建 ProxyRouter 实例
	proxyRouter := proxy.NewProxyRouter(db, cacheSvc, remoteClient, repoRepo, groupRepo, adapterMap)

	// 注入健康检查服务到 ProxyRouter
	proxyRouter.SetHealthCheckService(healthCheckSvc)

	// 注入 ProxyRouter 到代理下载服务
	proxyDownloadSvc.SetProxyRouter(proxyRouter)

	// 注入 ProxyRouter 到需要代理的适配器
	npmAdapter.SetProxyRouter(proxyRouter)
	mavenAdapter.SetProxyRouter(proxyRouter)
	pypiAdapter.SetProxyRouter(proxyRouter)
	goAdapter.SetProxyRouter(proxyRouter)
	yumAdapter.SetProxyRouter(proxyRouter)

	// 注入日志仓库到需要记录代理下载日志的适配器
	npmAdapter.SetLogRepo(proxyDownloadLogRepo)
	mavenAdapter.SetLogRepo(proxyDownloadLogRepo)
	pypiAdapter.SetLogRepo(proxyDownloadLogRepo)
	goAdapter.SetLogRepo(proxyDownloadLogRepo)

	adapters := []adapter.Adapter{
		npmAdapter,
		mavenAdapter,
		pypiAdapter,
		goAdapter,
		genericAdapter,
		yumAdapter,
		aptAdapter,
	}

	repoSvc := service.NewRepositoryService(repoRepo, groupRepo, db)
	blockRuleSvc := service.NewBlockRuleService(blockRuleRepo, auditSvc)

	// 初始化权限缓存服务（5分钟TTL）
	permCacheSvc := service.NewPermissionCacheService(roleRepo, 5*time.Minute)

	// 初始化元数据同步服务
	metadataSyncTaskRepo := repository.NewMetadataSyncTaskRepository(db)
	metadataSyncSvc := service.NewMetadataSyncService(db, metadataSyncTaskRepo, repoRepo, packageRepo)

	// 注册适配器到元数据同步服务
	for _, adap := range adapters {
		if syncer, ok := adap.(types.MetadataSyncer); ok {
			metadataSyncSvc.RegisterAdapter(string(adap.Type()), syncer)
		}
	}

	// 初始化备份服务
	backupRepo := repository.NewBackupRepository(db)
	backupSvc := service.NewBackupService(backupRepo, cfg.Storage.Local.BasePath, cfg.Storage.Local.BasePath+"/backups")

	// 初始化 Webhook 服务
	webhookRepo := repository.NewWebhookRepository(db)
	webhookSvc := service.NewWebhookService(webhookRepo)

	// 初始化系统配置服务
	systemConfigSvc := service.NewSystemConfigService(systemConfigRepo)

	// 初始化调度器服务（需要在 repoHandler 之前创建）
	schedulerSvc := service.NewSchedulerService(backupSvc, systemConfigSvc, webhookSvc, metadataSyncSvc, repoRepo)

	repoHandler := handler.NewRepositoryHandler(repoSvc, metadataSyncSvc, schedulerSvc)
	cacheHandler := handler.NewCacheHandler(cacheSvc)
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
	blockRuleHandler := handler.NewBlockRuleHandler(blockRuleSvc, auditSvc, auditRepo)
	userHandler := handler.NewUserHandler(userRepo, roleRepo, auditSvc)
	roleHandler := handler.NewRoleHandler(roleRepo, auditSvc)
	pkgVersionHandler := handler.NewPackageVersionHandler(packageRepo)

	// 初始化 CAS 认证服务
	casSvc := service.NewCASService(&cfg.Auth, userRepo, roleRepo, authService, systemConfigSvc)
	casHandler := handler.NewCASHandler(casSvc)

	// 初始化安全扫描服务
	scanRepo := repository.NewScanRepository(db)
	scanner := service.NewSecurityScanner(scanRepo, packageRepo, blockRuleRepo)
	securityHandler := handler.NewSecurityHandler(scanner)

	// 初始化备份服务 handler
	backupHandler := handler.NewBackupHandler(backupSvc)

	// 初始化 Webhook 服务 handler
	webhookHandler := handler.NewWebhookHandler(webhookSvc)

	// 初始化系统配置服务 handler
	systemConfigHandler := handler.NewSystemConfigHandler(systemConfigSvc, auditSvc)
	systemInfoHandler := handler.NewSystemInfoHandler(version, buildTime, gitCommit, time.Now().Unix())

	// 初始化文件浏览服务
	fileBrowseHandler := handler.NewFileBrowseHandler(cfg.Storage.Local.BasePath)

	// 初始化迁移服务
	migrationSvc := migration.NewMigrationService(db)
	migrationWorker := migration.NewMigrationWorker(migrationSvc, storageSvc, packageRepo, 5)
	migrationHandler := handler.NewMigrationHandler(migrationSvc, migrationWorker)

	// 初始化代理下载日志 handler
	proxyDownloadLogHandler := handler.NewProxyDownloadLogHandler(proxyDownloadLogRepo)

	// 初始化健康检查 handler
	healthCheckHandler := handler.NewHealthCheckHandler(healthCheckSvc)

	// 初始化AI服务
	var aiService *ai.AIService
	var aiHandler *handler.AIHandler
	if cfg.AI.Enabled {
		// 创建AI服务
		aiService = ai.NewAIService(&cfg.AI, db, auditRepo)

		// 注册工具
		toolContext := &tools.ToolContext{
			DB:      db,
			Config:  cfg,
			LogPath: cfg.Logging.Output,
		}

		// 日志查询工具 - 所有用户可用
		logQueryTool := tools.NewLogQueryTool()
		logQueryTool.SetContext(toolContext)
		aiService.RegisterTool(logQueryTool, []string{})

		// 数据库查询工具 - 所有用户可用
		dbQueryTool := tools.NewDBQueryTool()
		dbQueryTool.SetContext(toolContext)
		aiService.RegisterTool(dbQueryTool, []string{})

		// 包信息查询工具 - 所有用户可用
		packageInfoTool := tools.NewPackageInfoTool()
		packageInfoTool.SetContext(toolContext)
		aiService.RegisterTool(packageInfoTool, []string{})

		// 安全分析工具 - 管理员和安全管理员可用
		securityTool := tools.NewSecurityTool()
		securityTool.SetContext(toolContext)
		aiService.RegisterTool(securityTool, []string{"admin", "security_admin"})

		// 阻断日志分析工具 - 管理员和安全管理员可用
		blockLogAnalyzerTool := tools.NewBlockLogAnalyzerTool(auditRepo)
		blockLogAnalyzerTool.SetContext(toolContext)
		aiService.RegisterTool(blockLogAnalyzerTool, []string{"admin", "security_admin"})

		// 代码生成工具 - 所有用户可用
		codeGenTool := tools.NewCodeGenTool()
		codeGenTool.SetContext(toolContext)
		aiService.RegisterTool(codeGenTool, []string{})

		// 创建AI处理器
		aiHandler = handler.NewAIHandler(aiService)

		fmt.Println("AI服务已启用")
	}

	// 创建路由器
	router := setupRouter(cfg, authService, auditSvc, adapters, repoHandler, cacheHandler, blockRuleHandler, searchHandler, dashboardHandler, casHandler, blockRuleSvc, storageBackendHandler, securityHandler, auditLogHandler, userHandler, pkgVersionHandler, permCacheSvc, roleHandler, publicRepoHandler, backupHandler, webhookHandler, systemConfigHandler, systemInfoHandler, fileBrowseHandler, repoSvc, migrationHandler, aiHandler, proxyDownloadLogHandler, healthCheckHandler)

	// 设置 Webhook 服务到适配器
	for _, adap := range adapters {
		if webhookAware, ok := adap.(interface{ SetWebhookService(*service.WebhookService) }); ok {
			webhookAware.SetWebhookService(webhookSvc)
		}
	}

	// 启动健康检查服务
	healthCheckSvc.Start()
	defer healthCheckSvc.Stop()

	// 启动调度器
	if err := schedulerSvc.Start(); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("Failed to start scheduler")
	}
	defer schedulerSvc.Stop()

	// 启动AI服务
	if aiService != nil {
		defer aiService.Stop()
	}

	// 创建 HTTP 服务器
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 优雅关闭
	go func() {
		logrus.WithFields(logrus.Fields{
			"address": srv.Addr,
		}).Info("Server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Error("Server error")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logrus.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("Server forced to shutdown")
	}

	// 关闭审计服务，确保所有日志都已写入
	auditSvc.Shutdown()

	logrus.Info("Server exited")
}

func setupRouter(cfg *config.Config, authService *service.AuthService, auditSvc *service.AuditService, adapters []adapter.Adapter, repoHandler *handler.RepositoryHandler, cacheHandler *handler.CacheHandler, blockRuleHandler *handler.BlockRuleHandler, searchHandler *handler.PackageSearchHandler, dashboardHandler *handler.DashboardHandler, casHandler *handler.CASHandler, blockRuleSvc *service.BlockRuleService, storageBackendHandler *handler.StorageBackendHandler, securityHandler *handler.SecurityHandler, auditLogHandler *handler.AuditLogHandler, userHandler *handler.UserHandler, pkgVersionHandler *handler.PackageVersionHandler, permCacheSvc *service.PermissionCacheService, roleHandler *handler.RoleHandler, publicRepoHandler *handler.PublicRepoHandler, backupHandler *handler.BackupHandler, webhookHandler *handler.WebhookHandler, systemConfigHandler *handler.SystemConfigHandler, systemInfoHandler *handler.SystemInfoHandler, fileBrowseHandler *handler.FileBrowseHandler, repoSvc *service.RepositoryService, migrationHandler *handler.MigrationHandler, aiHandler *handler.AIHandler, proxyDownloadLogHandler *handler.ProxyDownloadLogHandler, healthCheckHandler *handler.HealthCheckHandler) *gin.Engine {
	r := gin.New()

	// 全局中间件
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS())
	r.Use(middleware.PrometheusMiddleware())
	r.Use(gin.Logger())

	// 初始化 auth handler
	authHandler := handler.NewAuthHandler(authService, auditSvc)

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": version,
		})
	})

	// Prometheus 指标端点
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// 文档静态文件服务
	r.Static("/docs", "./docs")

	// API 路由组
	api := r.Group("/api/v1")
	{
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong"})
		})

		// 包搜索（公开）
		api.GET("/packages/search", searchHandler.Search)
		api.GET("/packages/:type/versions", pkgVersionHandler.ListVersions)

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
			protected.PUT("/auth/profile", authHandler.UpdateProfile)
			protected.PUT("/auth/password", authHandler.ChangePassword)

			// 仓库管理
			repos := protected.Group("/repositories")
			repos.Use(middleware.RequirePermission(permCacheSvc, "repositories", "read"))
			{
				repos.GET("", repoHandler.List)
				repos.GET("/:name", repoHandler.Get)
				repos.GET("/:name/members", repoHandler.GetMembers)
			}
			reposWrite := protected.Group("/repositories")
			reposWrite.Use(middleware.RequirePermission(permCacheSvc, "repositories", "write"))
			{
				reposWrite.POST("", repoHandler.Create)
				reposWrite.PUT("/:name", repoHandler.Update)
				reposWrite.POST("/:name/members", repoHandler.AddMember)
				reposWrite.POST("/:name/metadata-sync", repoHandler.TriggerMetadataSync)
				reposWrite.PUT("/:name/metadata-sync-config", repoHandler.UpdateMetadataSyncConfig)
			}
			reposDelete := protected.Group("/repositories")
			reposDelete.Use(middleware.RequirePermission(permCacheSvc, "repositories", "delete"))
			{
				reposDelete.DELETE("/:name", repoHandler.Delete)
				reposDelete.DELETE("/:name/members/:memberName", repoHandler.RemoveMember)
			}

			// 缓存管理
			cache := protected.Group("/cache")
			cache.Use(middleware.RequirePermission(permCacheSvc, "cache", "read"))
			{
				cache.GET("/stats", cacheHandler.GetStats)
				cache.GET("/items", cacheHandler.List)
			}
			cacheWrite := protected.Group("/cache")
			cacheWrite.Use(middleware.RequirePermission(permCacheSvc, "cache", "write"))
			{
				cacheWrite.DELETE("", cacheHandler.Clear)
				cacheWrite.DELETE("/items/:key", cacheHandler.DeleteItem)
				cacheWrite.DELETE("/expired", cacheHandler.CleanupExpired)
				cacheWrite.POST("/invalidate", cacheHandler.Invalidate)
			}

			// 阻断规则管理
			blockRules := protected.Group("/block-rules")
			blockRules.Use(middleware.RequirePermission(permCacheSvc, "block-rules", "read"))
			{
				blockRules.GET("", blockRuleHandler.List)
				blockRules.GET("/logs", blockRuleHandler.ListBlockLogs)
				blockRules.GET("/template", blockRuleHandler.DownloadTemplate)
				blockRules.GET("/stats", blockRuleHandler.GetBlockStats)
			}
			blockRulesWrite := protected.Group("/block-rules")
			blockRulesWrite.Use(middleware.RequirePermission(permCacheSvc, "block-rules", "write"))
			{
				blockRulesWrite.POST("", blockRuleHandler.Create)
				blockRulesWrite.POST("/batch-import", blockRuleHandler.BatchImport)
				blockRulesWrite.PUT("/:id", blockRuleHandler.Update)
			}
			blockRulesDelete := protected.Group("/block-rules")
			blockRulesDelete.Use(middleware.RequirePermission(permCacheSvc, "block-rules", "delete"))
			{
				blockRulesDelete.DELETE("/:id", blockRuleHandler.Delete)
			}

			// CAS 认证
			casGroup := protected.Group("/cas")
			{
				casGroup.GET("/login", casHandler.Login)
				casGroup.GET("/callback", casHandler.Callback)
				casGroup.GET("/config", casHandler.Config)
			}

			// 存储后端管理
			storageBackends := protected.Group("/storage-backends")
			storageBackends.Use(middleware.RequirePermission(permCacheSvc, "storage-backends", "read"))
			{
				storageBackends.GET("", storageBackendHandler.List)
				storageBackends.GET("/:id", storageBackendHandler.Get)
				storageBackends.POST("/test", storageBackendHandler.TestConnection)
			}
			storageBackendsWrite := protected.Group("/storage-backends")
			storageBackendsWrite.Use(middleware.RequirePermission(permCacheSvc, "storage-backends", "write"))
			{
				storageBackendsWrite.POST("", storageBackendHandler.Create)
				storageBackendsWrite.PUT("/:id", storageBackendHandler.Update)
				storageBackendsWrite.POST("/:id/default", storageBackendHandler.SetDefault)
			}
			storageBackendsDelete := protected.Group("/storage-backends")
			storageBackendsDelete.Use(middleware.RequirePermission(permCacheSvc, "storage-backends", "write"))
			{
				storageBackendsDelete.DELETE("/:id", storageBackendHandler.Delete)
			}

			// 安全扫描
			security := protected.Group("/security")
			security.Use(middleware.RequirePermission(permCacheSvc, "security", "read"))
			{
				security.GET("/vulnerabilities", securityHandler.ListVulnerabilities)
				security.GET("/statistics", securityHandler.GetSecurityStats)
				security.GET("/dashboard", securityHandler.GetDashboard)
				security.GET("/packages/:id/scan", securityHandler.GetScanResult)
			}
			securityWrite := protected.Group("/security")
			securityWrite.Use(middleware.RequirePermission(permCacheSvc, "security", "write"))
			{
				securityWrite.POST("/scan/full", securityHandler.TriggerFullScan)
				securityWrite.POST("/block/:cve", securityHandler.BlockByCVE)
				securityWrite.POST("/packages/:id/scan/trigger", securityHandler.TriggerScan)
			}

			// 用户管理
			users := protected.Group("/users")
			users.Use(middleware.RequirePermission(permCacheSvc, "users", "read"))
			{
				users.GET("", userHandler.List)
			}
			usersWrite := protected.Group("/users")
			usersWrite.Use(middleware.RequirePermission(permCacheSvc, "users", "write"))
			{
				usersWrite.POST("", userHandler.Create)
				usersWrite.PUT("/:id/status", userHandler.UpdateStatus)
				usersWrite.PUT("/:id/roles", userHandler.AssignRoles)
			}
			protected.GET("/roles", middleware.RequirePermission(permCacheSvc, "users", "read"), roleHandler.List)
			protected.GET("/roles/permissions", middleware.RequirePermission(permCacheSvc, "users", "read"), roleHandler.ListPermissions)
			protected.GET("/roles/:id", middleware.RequirePermission(permCacheSvc, "users", "read"), roleHandler.Get)
			rolesWrite := protected.Group("/roles")
			rolesWrite.Use(middleware.RequirePermission(permCacheSvc, "users", "write"))
			{
				rolesWrite.POST("", roleHandler.Create)
				rolesWrite.PUT("/:id", roleHandler.Update)
				rolesWrite.DELETE("/:id", roleHandler.Delete)
				rolesWrite.PUT("/:id/permissions", roleHandler.UpdatePermissions)
			}

			// 审计日志
			audit := protected.Group("/audit")
			audit.Use(middleware.RequirePermission(permCacheSvc, "audit", "read"))
			{
				audit.GET("/logs", auditLogHandler.List)
				audit.GET("/logs/:id", auditLogHandler.Get)
			}

			// 代理下载日志
			proxyDownloads := protected.Group("/proxy-downloads")
			proxyDownloads.Use(middleware.RequirePermission(permCacheSvc, "audit", "read"))
			{
				proxyDownloads.GET("/logs", proxyDownloadLogHandler.List)
				proxyDownloads.GET("/stats", proxyDownloadLogHandler.GetStats)
			}

			// 包版本管理
			protected.POST("/packages/versions/:id/deprecate", middleware.RequirePermission(permCacheSvc, "npm", "write"), pkgVersionHandler.DeprecateVersion)
			protected.POST("/packages/versions/:id/restore", middleware.RequirePermission(permCacheSvc, "npm", "write"), pkgVersionHandler.RestoreVersion)
			protected.POST("/packages/versions/:id/yank", middleware.RequirePermission(permCacheSvc, "npm", "write"), pkgVersionHandler.YankVersion)
			protected.DELETE("/packages/versions/:id", middleware.RequirePermission(permCacheSvc, "npm", "delete"), pkgVersionHandler.DeleteVersion)

			// Dashboard 统计（需要认证）
			protected.GET("/dashboard/stats", dashboardHandler.GetStats)

			// 备份管理
			backups := protected.Group("/backups")
			backups.Use(middleware.RequirePermission(permCacheSvc, "system", "admin"))
			{
				backups.GET("", backupHandler.List)
				backups.GET("/:id", backupHandler.Get)
				backups.POST("", backupHandler.Create)
				backups.POST("/:id/restore", backupHandler.Restore)
				backups.DELETE("/:id", backupHandler.Delete)
			}

			// Webhook 管理
			webhooks := protected.Group("/webhooks")
			webhooks.Use(middleware.RequirePermission(permCacheSvc, "webhooks", "read"))
			{
				webhooks.GET("", webhookHandler.List)
				webhooks.GET("/:id", webhookHandler.Get)
				webhooks.GET("/:id/deliveries", webhookHandler.ListDeliveries)
			}
			webhooksWrite := protected.Group("/webhooks")
			webhooksWrite.Use(middleware.RequirePermission(permCacheSvc, "webhooks", "write"))
			{
				webhooksWrite.POST("", webhookHandler.Create)
				webhooksWrite.PUT("/:id", webhookHandler.Update)
				webhooksWrite.POST("/:id/test", webhookHandler.Test)
				webhooksWrite.DELETE("/:id", webhookHandler.Delete)
			}

			// 系统配置管理
			configs := protected.Group("/configs")
			configs.Use(middleware.RequirePermission(permCacheSvc, "system", "admin"))
			{
				configs.GET("", systemConfigHandler.List)
				configs.GET("/:key", systemConfigHandler.Get)
				configs.POST("", systemConfigHandler.BatchUpdate)
				configs.DELETE("/:key", systemConfigHandler.Delete)
			}

			// 系统信息
			protected.GET("/system/info", systemInfoHandler.GetInfo)

			// 文件浏览
			files := protected.Group("/files")
			files.Use(middleware.RequirePermission(permCacheSvc, "system", "admin"))
			{
				files.GET("/browse", fileBrowseHandler.ListDirectory)
				files.GET("/stats", fileBrowseHandler.GetFileStats)
				files.GET("/download", fileBrowseHandler.DownloadFile)
			}

			// 数据迁移
			migrationGroup := protected.Group("/migration")
			migrationGroup.Use(middleware.RequirePermission(permCacheSvc, "system", "admin"))
			{
				migrationGroup.GET("", migrationHandler.ListMigrations)
				migrationGroup.POST("/nexus/test", migrationHandler.TestNexusConnection)
				migrationGroup.POST("/nexus/repositories", migrationHandler.ListNexusRepositories)
				migrationGroup.POST("/nexus", migrationHandler.CreateMigration)
				migrationGroup.GET("/:id/status", migrationHandler.GetMigrationStatus)
				migrationGroup.POST("/:id/cancel", migrationHandler.CancelMigration)
			}

			// AI服务
			if aiHandler != nil {
				ai := protected.Group("/ai")
				{
					// 聊天接口 - 所有认证用户可用
					ai.POST("/chat", aiHandler.Chat)

					// 流式聊天接口 - 所有认证用户可用
					ai.POST("/chat/stream", aiHandler.StreamChat)

					// 工具列表 - 所有认证用户可用
					ai.GET("/tools", aiHandler.ListTools)

					// 会话管理 - 所有认证用户可用
					ai.DELETE("/sessions/:id", aiHandler.DeleteSession)

					// 限流状态 - 所有认证用户可用
					ai.GET("/rate-limit", aiHandler.GetRateLimitStatus)

					// 服务统计 - 管理员可用
					ai.GET("/stats", middleware.RequirePermission(permCacheSvc, "system", "admin"), aiHandler.GetStats)

					// 缓存统计 - 管理员可用
					ai.GET("/cache/stats", middleware.RequirePermission(permCacheSvc, "system", "admin"), aiHandler.GetCacheStats)

					// 审计日志 - 管理员可用
					ai.GET("/audit-logs", middleware.RequirePermission(permCacheSvc, "system", "admin"), aiHandler.GetAuditLogs)

					// 健康检查 - 所有认证用户可用
					ai.GET("/health", aiHandler.HealthCheck)
				}
			}

			// 健康检查管理 - 管理员可用
			health := protected.Group("/health")
			health.Use(middleware.RequirePermission(permCacheSvc, "system", "admin"))
			{
				health.GET("/repos", healthCheckHandler.GetAllHealthStatuses)
				health.GET("/repos/:id", healthCheckHandler.GetHealthStatus)
				health.POST("/repos/:id/reset", healthCheckHandler.ResetCircuitBreaker)
			}
		}
	}

	// 注册统一仓库路由
	repoRouter := handler.NewRepoRouter(repoHandler.Service(), auditSvc)
	for _, adap := range adapters {
		if repoAware, ok := adap.(adapter.RepoAwareAdapter); ok {
			repoRouter.RegisterAdapter(string(adap.Type()), repoAware)
		}
	}

	authMw := middleware.Auth(authService)
	blockMw := middleware.BlockCheck(blockRuleSvc, repoSvc)
	permMw := func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(permCacheSvc, resource, action)
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
		deleteGroup.Use(authMw, permMw("package", "delete"))
		{
			deleteGroup.DELETE("/*path", repoRouter.HandleDelete)
		}
	}

	// 前端静态文件服务
	setupFrontendRouter(r, cfg.Server.StaticDir)

	return r
}
