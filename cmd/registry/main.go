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
	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/handler"
	"github.com/moonlight-box/registry/internal/migration"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"
	"github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"
)

// @title Moonlight Registry API
// @version 1.0
// @description 制品仓库管理系统 API 文档，支持 npm、maven、pypi、go、yum、apt 等多种包类型的代理、缓存和管理。
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 输入格式：Bearer {token}

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

	// 加载配置（先加载配置，再初始化日志）
	// 使用临时日志记录配置加载过程
	logrus.WithFields(logrus.Fields{
		"version":    version,
		"build_time": buildTime,
	}).Info("Moonlight Registry starting")

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

	// 初始化日志系统（使用配置中的日志设置）
	// 注意：日志初始化必须在数据库初始化之前
	{
		// 转换配置格式
		// 注意：如果日志配置变更，需要同步更新此处
		_ = util.InitLogger(&util.LoggerConfig{
			Level:            cfg.Logging.Level,
			Format:           cfg.Logging.Format,
			Output:           cfg.Logging.Output,
			EnableSplitFiles: cfg.Logging.EnableSplitFiles,
			SqlLogFile:       cfg.Logging.SqlLogFile,
			ErrorLogFile:     cfg.Logging.ErrorLogFile,
			AccessLogFile:    cfg.Logging.AccessLogFile,
			LogRetentionDays: cfg.Logging.LogRetentionDays,
		})
	}

	util.WithFields(logrus.Fields{
		"server_port": cfg.Server.Port,
		"db_driver":   cfg.Database.Driver,
		"storage":     cfg.Storage.Backend,
		"ai_enabled":  cfg.AI.Enabled,
	}).Info("Configuration loaded")

	// 初始化日志
	if err := util.InitLogger(&util.LoggerConfig{
		Level:            cfg.Logging.Level,
		Format:           cfg.Logging.Format,
		Output:           cfg.Logging.Output,
		EnableSplitFiles: cfg.Logging.EnableSplitFiles,
		SqlLogFile:       cfg.Logging.SqlLogFile,
		ErrorLogFile:     cfg.Logging.ErrorLogFile,
		AccessLogFile:    cfg.Logging.AccessLogFile,
		LogRetentionDays: cfg.Logging.LogRetentionDays,
	}); err != nil {
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
	migrationItemRepo := repository.NewMigrationItemRepository(db)
	// 初始化缓存服务
	cacheOpts := proxy.CacheServiceOptions{
		MaxBytes: cfg.Cache.MaxSizeGB * 1024 * 1024 * 1024,
	}
	cacheSvc := proxy.NewCacheServiceWithOptions(cacheOpts)
	dnsResolver := proxy.NewDNSResolver(cfg.Proxy.DNSMapping)
	tm := proxy.NewTransportManager(cfg.Proxy.ConnectTimeout, dnsResolver)
	remoteClient := proxy.NewRemoteClient(tm, cfg.Proxy.MaxRedirects)

	healthCheckCfg := proxy.HealthCheckConfig{
		Enabled:          cfg.Proxy.HealthCheck.Enabled,
		Interval:         cfg.Proxy.HealthCheck.Interval,
		Timeout:          cfg.Proxy.HealthCheck.Timeout,
		FailureThreshold: cfg.Proxy.HealthCheck.FailureThreshold,
	}

	// 从系统配置中读取健康检查配置（覆盖配置文件中的值）
	if enabled := configInitializer.GetConfigAsBool("health_check.enabled", true); enabled {
		healthCheckCfg.Enabled = true
	} else {
		healthCheckCfg.Enabled = false
	}
	if interval := configInitializer.GetConfigAsInt("health_check.interval", 0); interval > 0 {
		healthCheckCfg.Interval = time.Duration(interval) * time.Second
	}
	if timeout := configInitializer.GetConfigAsInt("health_check.timeout", 0); timeout > 0 {
		healthCheckCfg.Timeout = time.Duration(timeout) * time.Second
	}
	if threshold := configInitializer.GetConfigAsInt("health_check.failure_threshold", 0); threshold > 0 {
		healthCheckCfg.FailureThreshold = threshold
	}

	// 使用默认值填充未配置的值
	if healthCheckCfg.Interval == 0 {
		healthCheckCfg.Interval = proxy.DefaultHealthCheckConfig().Interval
	}
	if healthCheckCfg.Timeout == 0 {
		healthCheckCfg.Timeout = proxy.DefaultHealthCheckConfig().Timeout
	}
	if healthCheckCfg.FailureThreshold == 0 {
		healthCheckCfg.FailureThreshold = proxy.DefaultHealthCheckConfig().FailureThreshold
	}

	// 初始化存储服务（依赖于 storageBackendRepo）
	storageSvc, err := service.NewStorageService(storageBackendRepo, cfg.Storage.Local.BasePath, int64(cfg.Storage.Local.MaxSizeGB))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize storage service: %v\n", err)
		os.Exit(1)
	}

	// 初始化系统配置服务
	systemConfigSvc := service.NewSystemConfigService(systemConfigRepo)

	// 初始化健康检查服务
	healthCheckSvc := proxy.NewHealthCheckService(db, repoRepo, storageSvc, remoteClient, healthCheckCfg, nil)

	// 初始化下载计数批量处理器
	countBatcher := service.NewDownloadCountBatcher(db, 10*time.Second)
	defer countBatcher.Stop()

	// 初始化日志批量处理器
	logBatcher := service.NewLogBatcher(proxyDownloadLogRepo, 100, 5*time.Second)
	defer logBatcher.Stop()

	// 初始化日志清理服务
	logCleanupSvc := service.NewLogCleanupService(
		proxyDownloadLogRepo,
		cfg.Logging.LogRetentionDays,
		cfg.Logging.CleanupInterval,
	)
	logCleanupSvc.Start()
	defer logCleanupSvc.Stop()

	// 初始化缓存管理器
	cacheMgr := cache.NewCacheManager()

	// 初始化权限缓存服务（5分钟TTL）
	permCacheSvc := service.NewPermissionCacheService(roleRepo, 5*time.Minute)

	// 创建 PackageCache（5分钟TTL）
	pkgCache := cache.NewPackageCache(packageRepo, 5*time.Minute)

	// 创建 MetadataCache（元数据本地磁盘缓存，支持 TTL 和 stale-while-revalidate）
	metaCache := service.NewMetadataCache(storageSvc)

	// 初始化适配器（传入 pkgCache）
	npmAdapter := adapter.NewNpmAdapter(storageSvc, pkgCache)
	mavenAdapter := adapter.NewMavenAdapter(storageSvc, pkgCache)
	pypiAdapter := adapter.NewPyPIAdapter(repoRepo, storageSvc, pkgCache)
	goAdapter := adapter.NewGoAdapter(storageSvc, pkgCache)
	genericAdapter := adapter.NewGenericAdapter(storageSvc, pkgCache)
	yumAdapter := adapter.NewYumAdapter(repoRepo, storageSvc, pkgCache)
	aptAdapter := adapter.NewAptAdapter(storageSvc, pkgCache)

	// 注入元数据缓存
	type metaCacher interface{ SetMetadataCache(*service.MetadataCache) }
	for _, adp := range []metaCacher{
		npmAdapter, mavenAdapter, pypiAdapter, goAdapter,
		genericAdapter, yumAdapter, aptAdapter,
	} {
		adp.SetMetadataCache(metaCache)
	}

	// 构建 adapter map 用于 DownloadService
	adapterMap := map[string]types.Adapter{
		string(types.NpmType):     npmAdapter,
		string(types.MavenType):   mavenAdapter,
		string(types.PyPIType):    pypiAdapter,
		string(types.GoType):      goAdapter,
		string(types.GenericType): genericAdapter,
		string(types.YumType):     yumAdapter,
		string(types.AptType):     aptAdapter,
	}

	// 创建 ProxyDownloader 实例（纯代理下载组件）
	proxyDownloader := proxy.NewProxyDownloader(cacheSvc, remoteClient, adapterMap)

	// 注入健康检查服务到 ProxyDownloader
	proxyDownloader.SetHealthCheckService(healthCheckSvc)

	// 创建 DownloadService
	downloadSvc := service.NewDownloadService(
		packageRepo,
		storageSvc,
		proxyDownloader,
		proxyDownloadLogRepo,
		logBatcher,
		countBatcher,
	)

	// 初始化仓库缓存（5分钟TTL）
	repoCache := proxy.NewRepositoryCache(repoRepo, groupRepo, 5*time.Minute)
	repoCache.StartCleanup(1 * time.Minute)
	defer repoCache.Stop()

	// 创建 RepoHandler（负责仓库请求的统一处理）
	proxyRepoHandler := proxy.NewRepoHandler(repoRepo, groupRepo, repoCache)
	proxyRepoHandler.SetDownloadService(downloadSvc)

	adapters := []types.Adapter{
		npmAdapter,
		mavenAdapter,
		pypiAdapter,
		goAdapter,
		genericAdapter,
		yumAdapter,
		aptAdapter,
	}

	repoSvc := service.NewRepositoryService(repoRepo, groupRepo, db)
	repoSvc.SetRepoCache(repoCache)
	blockRuleSvc := service.NewBlockRuleService(blockRuleRepo, auditSvc)
	uploadSvc := service.NewUploadService(packageRepo, storageSvc)

	// 注册所有缓存到缓存管理器
	cacheSvcProvider := proxy.NewCacheServiceProvider(cacheSvc, "proxy-content", "代理下载内容缓存")
	cacheMgr.Register(cacheSvcProvider)

	repoCacheProvider := proxy.NewRepositoryCacheProvider(repoCache, "repository-config", "仓库配置缓存")
	cacheMgr.Register(repoCacheProvider)

	permCacheProvider := service.NewPermissionCacheProvider(permCacheSvc, "permission", "权限缓存")
	cacheMgr.Register(permCacheProvider)

	pkgCacheProvider := cache.NewPackageCacheProvider(pkgCache, "package-metadata", "包元数据缓存")
	cacheMgr.Register(pkgCacheProvider)

	// 注入 Fetcher 到所有适配器
	for _, adap := range adapters {
		if repoAdap, ok := adap.(adapter.RepoAwareAdapter); ok {
			repoAdap.SetFetcher(proxyDownloader)
		}
	}

	// 初始化备份服务
	backupRepo := repository.NewBackupRepository(db)
	backupSvc := service.NewBackupService(backupRepo, cfg.Storage.Local.BasePath, cfg.Storage.Local.BasePath+"/backups")

	// 初始化 Webhook 服务
	webhookRepo := repository.NewWebhookRepository(db)
	webhookSvc := service.NewWebhookService(webhookRepo)

	// 重新初始化系统配置服务（供调度器使用）
	systemConfigSvc = service.NewSystemConfigService(systemConfigRepo)

	// 初始化调度器服务（需要在 repoHandler 之前创建）
	schedulerSvc := service.NewSchedulerService(backupSvc, systemConfigSvc, webhookSvc)

	repoHandler := handler.NewRepositoryHandler(repoSvc)
	repoSvc.SetHealthCheckService(healthCheckSvc)
	cacheHandler := handler.NewCacheHandler(cacheMgr)
	publicRepoHandler := handler.NewPublicRepoHandler(repoSvc)

	// 初始化存储后端管理服务
	storageBackendSvc := service.NewStorageBackendService(storageBackendRepo)
	storageBackendHandler := handler.NewStorageBackendHandler(storageBackendSvc)

	// 初始化搜索和 Dashboard 服务
	searchSvc := service.NewPackageSearchService(db)
	searchHandler := handler.NewPackageSearchHandler(searchSvc)

	dashboardSvc := service.NewDashboardService(db, repoRepo, healthCheckSvc, cfg.Storage.Local.BasePath)
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

	vulnRuleRepo := repository.NewVulnRuleRepository(db)
	vulnDataSourceRepo := repository.NewVulnDataSourceRepository(db)
	vulnRuleService := service.NewVulnRuleService(vulnRuleRepo, vulnDataSourceRepo)
	vulnRuleHandler := handler.NewVulnRuleHandler(vulnRuleService)
	scanner.SetVulnRuleService(vulnRuleService)

	// 初始化备份服务 handler
	backupHandler := handler.NewBackupHandler(backupSvc)

	// 初始化备份配置 handler
	backupConfigHandler := handler.NewBackupConfigHandler(systemConfigSvc, schedulerSvc)

	// 初始化 Webhook 服务 handler
	webhookHandler := handler.NewWebhookHandler(webhookSvc)

	// 初始化系统配置服务 handler
	systemConfigHandler := handler.NewSystemConfigHandler(systemConfigSvc, auditSvc)
	systemInfoHandler := handler.NewSystemInfoHandler(version, buildTime, gitCommit, time.Now().Unix())

	// 初始化文件浏览服务
	fileBrowseHandler := handler.NewFileBrowseHandler(storageSvc)

	// 初始化迁移 worker（先传 nil 作为 service）
	migrationWorker := migration.NewMigrationWorkerV2(nil, storageSvc, packageRepo, repoRepo, migrationItemRepo, 5, 3, 50)

	// 创建仓库创建器适配器
	repoCreator := migration.NewRepositoryCreatorAdapter(repoRepo, storageBackendRepo)

	// 初始化迁移 service（注入 worker 和 repoCreator，最大并发 1 个任务）
	migrationSvc := migration.NewMigrationService(db, migrationWorker, repoCreator, 1)
	migrationSvc.StartQueueWithRecovery() // 启动队列处理器并自动恢复中断的任务

	// 设置 worker 的 service 引用
	migrationWorker.SetService(migrationSvc)

	migrationHandler := handler.NewMigrationHandler(migrationSvc, migrationWorker, repoRepo, storageBackendRepo)

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
		// 依赖优化分析工具 - 所有用户可用
		depOptimizerTool := tools.NewDependencyOptimizerTool()
		depOptimizerTool.SetContext(toolContext)
		aiService.RegisterTool(depOptimizerTool, []string{})

		// 创建AI处理器
		aiHandler = handler.NewAIHandler(aiService)

		fmt.Println("AI服务已启用")
	}

	// 创建路由器上下文
	routerCtx := NewRouterContext(cfg, authService, auditSvc, permCacheSvc, blockRuleSvc, repoSvc, proxyRepoHandler, adapters, webhookSvc, uploadSvc)
	routerCtx.RepoCache = repoCache
	routerCtx.Handlers.Repo = repoHandler
	routerCtx.Handlers.PublicRepo = publicRepoHandler
	routerCtx.Handlers.Cache = cacheHandler
	routerCtx.Handlers.BlockRule = blockRuleHandler
	routerCtx.Handlers.Search = searchHandler
	routerCtx.Handlers.Dashboard = dashboardHandler
	routerCtx.Handlers.CAS = casHandler
	routerCtx.Handlers.StorageBackend = storageBackendHandler
	routerCtx.Handlers.Security = securityHandler
	routerCtx.Handlers.AuditLog = auditLogHandler
	routerCtx.Handlers.User = userHandler
	routerCtx.Handlers.Role = roleHandler
	routerCtx.Handlers.PackageVersion = pkgVersionHandler
	routerCtx.Handlers.Backup = backupHandler
	routerCtx.Handlers.BackupConfig = backupConfigHandler
	routerCtx.Handlers.Webhook = webhookHandler
	routerCtx.Handlers.SystemConfig = systemConfigHandler
	routerCtx.Handlers.SystemInfo = systemInfoHandler
	routerCtx.Handlers.FileBrowse = fileBrowseHandler
	routerCtx.Handlers.Migration = migrationHandler
	routerCtx.Handlers.AI = aiHandler
	routerCtx.Handlers.ProxyDownloadLog = proxyDownloadLogHandler
	routerCtx.Handlers.HealthCheck = healthCheckHandler
	routerCtx.Handlers.VulnRule = vulnRuleHandler

	router := routerCtx.SetupRouter(version)

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
