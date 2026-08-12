package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dshmyz/moonlight-box/internal/ai"
	"github.com/dshmyz/moonlight-box/internal/ai/tools"
	handler "github.com/dshmyz/moonlight-box/internal/api/http"
	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/core/cache"
	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/database"
	migv2executor "github.com/dshmyz/moonlight-box/internal/migration/v2/executor"
	migv2handler "github.com/dshmyz/moonlight-box/internal/migration/v2/handler"
	migv2repo "github.com/dshmyz/moonlight-box/internal/migration/v2/repository"
	migv2sched "github.com/dshmyz/moonlight-box/internal/migration/v2/scheduler"
	migv2svc "github.com/dshmyz/moonlight-box/internal/migration/v2/service"
	"github.com/dshmyz/moonlight-box/internal/plugins/apt"
	gomod "github.com/dshmyz/moonlight-box/internal/plugins/go"
	"github.com/dshmyz/moonlight-box/internal/plugins/maven"
	"github.com/dshmyz/moonlight-box/internal/plugins/npm"
	"github.com/dshmyz/moonlight-box/internal/plugins/pypi"
	"github.com/dshmyz/moonlight-box/internal/plugins/raw"
	"github.com/dshmyz/moonlight-box/internal/plugins/yum"
	"github.com/dshmyz/moonlight-box/internal/mcp"
	"github.com/dshmyz/moonlight-box/internal/proxy"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"
	mcpserver "github.com/mark3labs/mcp-go/server"
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
			SampleRate:       cfg.Logging.SampleRate,
			SampledModules:   cfg.Logging.SampleByModule,
		})
	}

	util.WithFields(logrus.Fields{
		"server_port": cfg.Server.Port,
		"db_driver":   cfg.Database.Driver,
		"storage":     cfg.Storage.Backend,
		"ai_enabled":  cfg.AI.Enabled,
	}).Info("Configuration loaded")

	// 校验配置安全性：release 模式下不安全的 JWT 密钥（默认值/为空/过短）将拒绝启动
	if warnings, vErr := cfg.Validate(); vErr != nil {
		util.WithFields(logrus.Fields{"error": vErr}).Error("配置校验未通过，拒绝启动")
		os.Exit(1)
	} else {
		for _, w := range warnings {
			util.WithField("warning", w).Warn("配置安全提醒")
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
	downloadLogRepo := repository.NewDownloadLogRepository(database.GetDB())

	// 初始化服务
	auditSvc := service.NewAuditService()
	authService := service.NewAuthService(userRepo, roleRepo, &cfg.Auth, auditSvc)

	db := database.GetDB()
	apiTokenRepo := repository.NewAPITokenRepository(db)
	apiTokenSvc := service.NewAPITokenService(apiTokenRepo)

	// 初始化仓库管理和缓存服务
	repoRepo := repository.NewRepositoryRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	blockRuleRepo := repository.NewBlockRuleRepository(db)
	storageBackendRepo := repository.NewStorageBackendRepository(db)
	// 初始化缓存服务
	cacheOpts := proxy.CacheServiceOptions{
		MaxBytes: cfg.Cache.MaxSizeGB * 1024 * 1024 * 1024,
	}
	cacheSvc := proxy.NewCacheServiceWithOptions(cacheOpts)
	dnsResolver := proxy.NewDNSResolver(cfg.Proxy.DNSMapping)
	tm := proxy.NewTransportManager(cfg.Proxy.ConnectTimeout, dnsResolver)
	remoteClient := proxy.NewRemoteClient(tm, cfg.Proxy.MaxRedirects)

	// 共享 HTTP 客户端：使用 proxy.TransportManager 的 DNS 映射、TLS 配置和连接池
	pluginTimeout := cfg.Proxy.DefaultTimeout
	if pluginTimeout <= 0 {
		pluginTimeout = 60 * time.Second
	}
	pluginHTTPClient := &http.Client{
		Transport: tm.GetTransport(cfg.Proxy.InsecureSkipVerify),
		Timeout:   pluginTimeout,
	}

	healthCheckCfg := proxy.HealthCheckConfig{
		Enabled:          cfg.Proxy.HealthCheck.Enabled,
		Interval:         cfg.Proxy.HealthCheck.Interval,
		Timeout:          cfg.Proxy.HealthCheck.Timeout,
		FailureThreshold: cfg.Proxy.HealthCheck.FailureThreshold,
		BlockOnUnhealthy: cfg.Proxy.HealthCheck.BlockOnUnhealthy,
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
	healthCheckCfg.BlockOnUnhealthy = configInitializer.GetConfigAsBool("health_check.block_on_unhealthy", false)

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
	logBatcher := service.NewLogBatcher(downloadLogRepo, 100, 5*time.Second)
	defer logBatcher.Stop()

	// 初始化日志清理服务
	logCleanupSvc := service.NewLogCleanupService(
		downloadLogRepo,
		cfg.Logging.LogRetentionDays,
		cfg.Logging.CleanupInterval,
	)
	logCleanupSvc.SetConfigService(systemConfigSvc)
	logCleanupSvc.Start()
	defer logCleanupSvc.Stop()

	// 初始化缓存管理器
	cacheMgr := cache.NewCacheManager()

	// 初始化权限缓存服务（5分钟TTL）
	permCacheSvc := service.NewPermissionCacheService(roleRepo, 5*time.Minute)

	// 初始化协议插件（新架构）
	npmPlugin := npm.NewNpmPlugin(pluginHTTPClient)
	mavenPlugin := maven.NewMavenPlugin(pluginHTTPClient)
	goPlugin := gomod.NewGoPlugin(pluginHTTPClient)
	pypiPlugin := pypi.NewPyPIPlugin(pluginHTTPClient)
	genericPlugin := raw.NewGenericPlugin(pluginHTTPClient)
	yumPlugin := yum.NewYumPlugin(pluginHTTPClient)
	aptPlugin := apt.NewAptPlugin(pluginHTTPClient)

	// 创建新架构 RepositoryRouter
	repoManager := runtime.NewDefaultRepositoryManager()
	compositeResolver := &runtime.CompositeResolver{
		Resolvers: []runtime.RepositoryPathResolver{
			&runtime.Nexus3Resolver{},
			runtime.NewNexus2RepoResolver(),
			runtime.NewNexus2GroupResolver(),
		},
	}
	repositoryRouter := runtime.NewRepositoryRouter(compositeResolver, repoManager)
	repositoryRouter.RegisterPlugin("maven", mavenPlugin)
	repositoryRouter.RegisterPlugin("npm", npmPlugin)
	repositoryRouter.RegisterPlugin("go", goPlugin)
	repositoryRouter.RegisterPlugin("pypi", pypiPlugin)
	repositoryRouter.RegisterPlugin("generic", genericPlugin)
	repositoryRouter.RegisterPlugin("yum", yumPlugin)
	repositoryRouter.RegisterPlugin("apt", aptPlugin)

	fetchers := map[string]runtime.RemoteFetcher{
		"pypi":    pypiPlugin,
		"npm":     npmPlugin,
		"maven":   mavenPlugin,
		"go":      goPlugin,
		"yum":     yumPlugin,
		"apt":     aptPlugin,
		"generic": genericPlugin,
	}
	normalizers := map[string]runtime.ArtifactNormalizer{
		"maven": mavenPlugin,
	}
	blockRuleSvc := service.NewBlockRuleService(blockRuleRepo, auditSvc)
	blocker := &blockRuleBlocker{svc: blockRuleSvc}

	// 创建统一的 ArtifactService，用于 artifact 写入时自动同步 packages 聚合表
	artifactSvc := service.NewArtifactService(db)
	defer artifactSvc.Stop()

	// 设置懒加载 factory：Get() 在内存缓存未命中时自动从 DB 创建 Runtime
	repoManager.SetFactory(NewRepositoryFactory(
		repoRepo, groupRepo, db, storageSvc, repoManager, fetchers, blocker, pluginHTTPClient, artifactSvc,
		&healthCheckLookup{svc: healthCheckSvc},
	))

	// 预热所有仓库 Runtime（可选，避免首次请求冷启动）
	initRepoRuntimes(repoManager, repoRepo)

	// 初始化仓库缓存（5分钟TTL）
	repoCache := proxy.NewRepositoryCache(repoRepo, groupRepo, 5*time.Minute)
	repoCache.StartCleanup(1 * time.Minute)
	defer repoCache.Stop()

	repoSvc := service.NewRepositoryService(repoRepo, groupRepo, db)
	repoSvc.SetRepoCache(repoCache)
	repoSvc.SetRepoManager(repoManager)

	// Wire block rules and audit logging into the repository router
	repositoryRouter.Blocker = blocker
	repositoryRouter.AuditLog = &auditLoggerAdapter{svc: auditSvc}
	repositoryRouter.DownloadCount = newDownloadCountAdapter(countBatcher)
	repositoryRouter.ProxyLog = newDownloadLogAdapter(logBatcher)

	// 注册所有缓存到缓存管理器
	cacheSvcProvider := proxy.NewCacheServiceProvider(cacheSvc, "proxy-content", "代理下载内容缓存")
	cacheMgr.Register(cacheSvcProvider)

	repoCacheProvider := proxy.NewRepositoryCacheProvider(repoCache, "repository-config", "仓库配置缓存")
	cacheMgr.Register(repoCacheProvider)

	permCacheProvider := service.NewPermissionCacheProvider(permCacheSvc, "permission", "权限缓存")
	cacheMgr.Register(permCacheProvider)

	// 初始化备份服务
	backupRepo := repository.NewBackupRepository(db)
	backupTargets := []service.BackupTarget{
		{ArchivePath: "db/registry.db", LocalPath: cfg.Database.DSN},
		{ArchivePath: "config/config.yaml", LocalPath: *configPath},
	}
	backupSvc := service.NewBackupService(backupRepo, storageSvc, "backups", backupTargets)

	// 初始化 Webhook 服务
	webhookRepo := repository.NewWebhookRepository(db)
	webhookSvc := service.NewWebhookService(webhookRepo)

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
	artifactSvc.SetCacheInvalidationCallback(searchSvc.InvalidateCache)
	searchHandler := handler.NewPackageSearchHandler(searchSvc)

	dashboardSvc := service.NewDashboardService(db, repoRepo, healthCheckSvc, cfg.Storage.Local.BasePath)
	dashboardHandler := handler.NewDashboardHandler(dashboardSvc)

	// 初始化用户和审计日志管理
	auditRepo := repository.NewAuditRepository(db)
	auditLogHandler := handler.NewAuditLogHandler(auditRepo)
	blockRuleHandler := handler.NewBlockRuleHandler(blockRuleSvc, auditSvc, auditRepo)
	userHandler := handler.NewUserHandler(userRepo, roleRepo, auditSvc)
	roleHandler := handler.NewRoleHandler(roleRepo, auditSvc)

	// 初始化 CAS 认证服务
	casSvc := service.NewCASService(&cfg.Auth, userRepo, roleRepo, authService, systemConfigSvc)
	casHandler := handler.NewCASHandler(casSvc)

	// 初始化安全扫描服务
	scanRepo := repository.NewScanRepository(db)
	scanner := service.NewSecurityScanner(scanRepo, db, blockRuleRepo)
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

	// V2 migration pipeline
	migV2PlanRepo := migv2repo.NewPlanRepo(db)
	migV2JobRepo := migv2repo.NewJobRepo(db)
	migV2ItemRepo := migv2repo.NewItemRepo(db)
	migV2ConflictRepo := migv2repo.NewConflictRepo(db)
	migV2EventRepo := migv2repo.NewEventRepo(db)
	migV2ExecMgr := migv2executor.NewExecutorManager(db, nil, storageSvc, migV2ItemRepo, migV2EventRepo, 5, artifactSvc)
	migV2ExecMgr.SetNormalizers(normalizers)
	migV2Scheduler := migv2sched.New(db, migV2PlanRepo, migV2JobRepo, migV2ItemRepo, migV2EventRepo, migV2ExecMgr, 3)
	migV2Svc := migv2svc.New(db, migV2PlanRepo, migV2JobRepo, migV2ItemRepo, migV2ConflictRepo, migV2EventRepo, migV2Scheduler)
	migV2Svc.RecoverInterruptedPlans(context.Background())
	migV2Handler := migv2handler.NewMigrationV2Handler(migV2Svc)

	// 初始化下载日志 handler
	downloadLogHandler := handler.NewDownloadLogHandler(downloadLogRepo)

	// 初始化日志清理配置 handler
	logCleanupConfigHandler := handler.NewLogCleanupConfigHandler(systemConfigSvc, logCleanupSvc)

	// 初始化健康检查 handler
	healthCheckHandler := handler.NewHealthCheckHandler(healthCheckSvc)

	// 初始化包版本管理 handler（新架构，读取 artifacts 表）
	classifiers := map[string]runtime.FileTypeClassifier{
		"maven": mavenPlugin,
		"pypi":  pypiPlugin,
		"go":    goPlugin,
	}
	packageVersionHandler := handler.NewPackageVersionHandlerWithClassifiers(db, artifactSvc, classifiers)

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

		// 阻断规则生成工具 - 管理员和安全管理员可用
		// Preview-only：根据漏洞数据或用户描述生成阻断规则草案，不自动写入 DB
		blockRuleGenTool := tools.NewBlockRuleGeneratorTool(scanRepo, blockRuleSvc)
		blockRuleGenTool.SetContext(toolContext)
		aiService.RegisterTool(blockRuleGenTool, []string{"admin", "security_admin"})

		// 阻断规则优化分析工具 - 管理员和安全管理员可用
		// 只读分析现有规则集，输出 over_broad/stale/redundant 优化建议
		blockRuleOptimizerTool := tools.NewBlockRuleOptimizerTool()
		blockRuleOptimizerTool.SetContext(toolContext)
		aiService.RegisterTool(blockRuleOptimizerTool, []string{"admin", "security_admin"})

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
	routerCtx := NewRouterContext(cfg, authService, auditSvc, permCacheSvc, blockRuleSvc, repoSvc, repositoryRouter, webhookSvc)
	routerCtx.Handlers.Auth.SetAPITokenService(apiTokenSvc)
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
	routerCtx.Handlers.Backup = backupHandler
	routerCtx.Handlers.BackupConfig = backupConfigHandler
	routerCtx.Handlers.Webhook = webhookHandler
	routerCtx.Handlers.SystemConfig = systemConfigHandler
	routerCtx.Handlers.SystemInfo = systemInfoHandler
	routerCtx.Handlers.FileBrowse = fileBrowseHandler
	routerCtx.Handlers.MigrationV2 = migV2Handler
	routerCtx.Handlers.AI = aiHandler
	routerCtx.Handlers.DownloadLog = downloadLogHandler
	routerCtx.Handlers.LogCleanupConfig = logCleanupConfigHandler
	routerCtx.Handlers.HealthCheck = healthCheckHandler
	routerCtx.Handlers.VulnRule = vulnRuleHandler
	routerCtx.Handlers.PackageVersion = packageVersionHandler

	router := routerCtx.SetupRouter(version)

	// 挂载 MCP Server（共用主服务端口）
	var rootHandler http.Handler = router
	if cfg.MCP.Enabled {
		mcpSrv := mcp.NewMCPServer(cfg, db, repoSvc, auditSvc, blockRuleSvc, permCacheSvc)
		mcpPath := cfg.MCP.Path
		if mcpPath == "" {
			mcpPath = mcp.DefaultMCPServerPath
		}

		// 同时支持 SSE（旧协议）和 Streamable HTTP（新协议）
		sseServer := mcpserver.NewSSEServer(
			mcpSrv.GetMCPServer(),
			mcpserver.WithStaticBasePath(mcpPath),
			mcpserver.WithSSEDisableLocalhostProtection(true),
		)
		streamableServer := mcpserver.NewStreamableHTTPServer(
			mcpSrv.GetMCPServer(),
			mcpserver.WithEndpointPath(mcpPath),
			mcpserver.WithDisableLocalhostProtection(true),
		)

		rootHandler = mcpDualHandler(sseServer, streamableServer, mcpPath, cfg.MCP.Token, authService, apiTokenSvc, router)
		logrus.WithFields(logrus.Fields{
			"path": mcpPath,
		}).Info("MCP Server mounted (SSE + Streamable HTTP)")
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
		Handler:      rootHandler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
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

	// 关闭日志文件（lumberjack 句柄），避免句柄泄露
	util.CloseLoggers()

	logrus.Info("Server exited")
}

// mcpDualHandler 同时支持 SSE 和 Streamable HTTP 两种 MCP 传输协议。
// 认证方式（按优先级）：静态 token → API Token → 用户 JWT
func mcpDualHandler(
	sseServer *mcpserver.SSEServer,
	streamableServer *mcpserver.StreamableHTTPServer,
	mcpPath, staticToken string,
	authSvc *service.AuthService,
	apiTokenSvc *service.APITokenService,
	ginEngine *gin.Engine,
) http.HandlerFunc {
	ssePrefix := strings.TrimRight(mcpPath, "/") + "/sse"
	msgPrefix := strings.TrimRight(mcpPath, "/") + "/message"
	mcpPrefix := strings.TrimRight(mcpPath, "/") + "/"

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		isMCP := path == mcpPath ||
			strings.HasPrefix(path, mcpPrefix) ||
			path == ssePrefix ||
			strings.HasPrefix(path, msgPrefix)

		if !isMCP {
			ginEngine.ServeHTTP(w, r)
			return
		}

		// 认证并提取用户身份
		user := mcpAuthenticate(r, staticToken, authSvc, apiTokenSvc)
		if user == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		// 将用户身份注入请求 context，MCP 工具处理器可从中读取
		ctx := mcp.SetMCPUser(r.Context(), user)
		r = r.WithContext(ctx)

		if path == ssePrefix || strings.HasPrefix(path, msgPrefix) {
			sseServer.ServeHTTP(w, r)
			return
		}
		streamableServer.ServeHTTP(w, r)
	}
}

// mcpAuthenticate 认证并返回用户信息；静态 token 返回 admin 身份
func mcpAuthenticate(r *http.Request, staticToken string, authSvc *service.AuthService, apiTokenSvc *service.APITokenService) *mcp.MCPUser {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil
	}
	token := parts[1]

	// 1. 静态 token → admin 权限
	if staticToken != "" && token == staticToken {
		return &mcp.MCPUser{UserID: 0, Username: "static", Static: true}
	}

	// 2. API Token
	if apiTokenSvc != nil {
		if apiToken, err := apiTokenSvc.ValidateToken(token); err == nil {
			return &mcp.MCPUser{
				UserID:   apiToken.UserID,
				Static:   false,
			}
		}
	}

	// 3. 用户 JWT
	if authSvc != nil {
		if claims, err := authSvc.ValidateToken(token); err == nil {
			return &mcp.MCPUser{
				UserID:   claims.UserID,
				Username: claims.Username,
				Roles:    claims.Roles,
				Static:   false,
			}
		}
	}

	return nil
}
