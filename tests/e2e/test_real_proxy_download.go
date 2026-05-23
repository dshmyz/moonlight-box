package main

import (
	"context"
	"fmt"
	"time"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
	"github.com/moonlight-box/registry/internal/types"
	"gorm.io/gorm"
)

func main() {
	fmt.Println("=== 真实代理仓库下载测试 ===")

	_, err := config.Load("configs/config.yaml")
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	cfg := config.Get()
	if err := database.Initialize(cfg); err != nil {
		fmt.Printf("初始化数据库失败: %v\n", err)
		return
	}
	defer database.Close()

	db := database.DB

	var repoCount int64
	db.Model(&model.Repository{}).Count(&repoCount)
	fmt.Printf("当前仓库数量: %d\n", repoCount)

	var pkgCount int64
	db.Model(&model.Component{}).Count(&pkgCount)
	fmt.Printf("当前包数量: %d\n", pkgCount)

	storageBackendRepo := repository.NewStorageBackendRepository(db)
	storageSvc, err := service.NewStorageService(storageBackendRepo, cfg.Storage.Local.BasePath, cfg.Storage.Local.MaxSizeGB)
	if err != nil {
		fmt.Printf("创建存储服务失败: %v\n", err)
		return
	}

	localStorage, err := storage.NewLocalStorage(cfg.Storage.Local.BasePath, cfg.Storage.Local.MaxSizeGB)
	if err != nil {
		fmt.Printf("创建本地存储失败: %v\n", err)
		return
	}
	storageSvc.SetDefaultBackendForTest(localStorage)

	compRepo := repository.NewComponentRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	logRepo := repository.NewProxyDownloadLogRepository(db)

	cacheSvc := proxy.NewCacheService()
	dnsResolver := proxy.NewDNSResolver(nil)
	tm := proxy.NewTransportManager(30*time.Second, dnsResolver)
	remoteClient := proxy.NewRemoteClient(tm, 5)
	proxyDownloader := proxy.NewProxyDownloader(cacheSvc, remoteClient)

	countBatcher := service.NewDownloadCountBatcher(db, 10*time.Second)
	defer countBatcher.Stop()

	logBatcher := service.NewLogBatcher(logRepo, 100, 5*time.Second)
	defer logBatcher.Stop()

	downloadSvc := service.NewDownloadService(
		compRepo,
		storageSvc,
		proxyDownloader,
		logRepo,
		logBatcher,
		countBatcher,
	)

	fmt.Println("\n=== 测试 1: NPM 包下载 ===")
	testNPMPackage(downloadSvc, repoRepo, db)

	fmt.Println("\n=== 测试 2: PyPI 包下载 ===")
	testPyPIPackage(downloadSvc, repoRepo, db)

	fmt.Println("\n=== 验证数据库存储 ===")
	verifyDatabaseStorage(db)

	fmt.Println("\n=== 测试完成 ===")
}

func testNPMPackage(svc *service.DownloadService, repoRepo *repository.RepositoryRepository, db *gorm.DB) {
	repo := &model.Repository{
		Name:        "npm-proxy-test",
		DisplayName: "NPM Proxy Test",
		Type:        model.RepoTypeProxy,
		PackageType: string(model.PackageTypeNPM),
		RemoteURL:   "https://registry.npmjs.org",
		Enabled:     true,
	}

	var existingRepo model.Repository
	if err := db.Where("name = ?", repo.Name).First(&existingRepo).Error; err == nil {
		repo = &existingRepo
		fmt.Printf("使用已存在的仓库: %s (ID: %d)\n", repo.Name, repo.ID)
	} else {
		if err := repoRepo.Create(repo); err != nil {
			fmt.Printf("创建 NPM 代理仓库失败: %v\n", err)
			return
		}
		fmt.Printf("创建 NPM 代理仓库成功: %s (ID: %d)\n", repo.Name, repo.ID)
	}

	downloadCtx := &types.DownloadContext{
		Repo:     repo,
		PkgType:  model.PackageTypeNPM,
		Name:     "lodash",
		Version:  "4.17.21",
		Filename: "lodash-4.17.21.tgz",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := svc.Download(ctx, downloadCtx)
	if err != nil {
		fmt.Printf("下载 lodash 失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 下载 lodash 成功!\n")
	fmt.Printf("  - 大小: %d bytes\n", result.Size)
	fmt.Printf("  - 存储路径: %s\n", result.Filename)
	fmt.Printf("  - 来自缓存: %v\n", result.FromCache)
}

func testPyPIPackage(svc *service.DownloadService, repoRepo *repository.RepositoryRepository, db *gorm.DB) {
	repo := &model.Repository{
		Name:        "pypi-proxy-test",
		DisplayName: "PyPI Proxy Test",
		Type:        model.RepoTypeProxy,
		PackageType: string(model.PackageTypePyPI),
		RemoteURL:   "https://pypi.org",
		Enabled:     true,
	}

	var existingRepo model.Repository
	if err := db.Where("name = ?", repo.Name).First(&existingRepo).Error; err == nil {
		repo = &existingRepo
		fmt.Printf("使用已存在的仓库: %s (ID: %d)\n", repo.Name, repo.ID)
	} else {
		if err := repoRepo.Create(repo); err != nil {
			fmt.Printf("创建 PyPI 代理仓库失败: %v\n", err)
			return
		}
		fmt.Printf("创建 PyPI 代理仓库成功: %s (ID: %d)\n", repo.Name, repo.ID)
	}

	downloadCtx := &types.DownloadContext{
		Repo:     repo,
		PkgType:  model.PackageTypePyPI,
		Name:     "requests",
		Version:  "2.31.0",
		Filename: "requests-2.31.0-py3-none-any.whl",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := svc.Download(ctx, downloadCtx)
	if err != nil {
		fmt.Printf("下载 requests 失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 下载 requests 成功!\n")
	fmt.Printf("  - 大小: %d bytes\n", result.Size)
	fmt.Printf("  - 存储路径: %s\n", result.Filename)
	fmt.Printf("  - 来自缓存: %v\n", result.FromCache)
}

func verifyDatabaseStorage(db *gorm.DB) {
	var packages []model.Component
	if err := db.Preload("Versions").Preload("Versions.Files").Find(&packages).Error; err != nil {
		fmt.Printf("查询包失败: %v\n", err)
		return
	}

	fmt.Printf("\n数据库中的包记录:\n")
	for _, pkg := range packages {
		fmt.Printf("\n📦 包名: %s\n", pkg.Name)
		fmt.Printf("   格式: %s\n", pkg.Format)
		fmt.Printf("   仓库ID: %d\n", pkg.RepositoryID)
		fmt.Printf("   版本: %s\n", pkg.Version)
		fmt.Printf("   下载次数: %d\n", pkg.DownloadCount)
		fmt.Printf("   状态: %s\n", pkg.Status)
	}

	var downloadLogs []model.ProxyDownloadLog
	db.Order("created_at desc").Limit(10).Find(&downloadLogs)

	fmt.Printf("\n最近的下载日志:\n")
	for _, log := range downloadLogs {
		fmt.Printf("  - %s/%s@%s (%s) - %s\n",
			log.PackageType, log.PackageName, log.Version,
			log.Status, log.CreatedAt.Format("2006-01-02 15:04:05"))
	}
}
