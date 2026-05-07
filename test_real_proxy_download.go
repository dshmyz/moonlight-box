package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
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
	db.Model(&model.Package{}).Count(&pkgCount)
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

	pkgRepo := repository.NewPackageRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	logRepo := repository.NewProxyDownloadLogRepository(db)

	cacheSvc := proxy.NewCacheService()
	dnsResolver := proxy.NewDNSResolver(nil)
	tm := proxy.NewTransportManager(30*time.Second, dnsResolver)
	remoteClient := proxy.NewRemoteClient(tm, 5)
	proxyRouter := proxy.NewProxyRouter(db, cacheSvc, remoteClient, repoRepo, groupRepo, nil)

	countBatcher := service.NewDownloadCountBatcher(pkgRepo, 10*time.Second)

	proxyDownloadSvc := service.NewProxyDownloadService(
		pkgRepo,
		storageSvc,
		proxyRouter,
		logRepo,
		countBatcher,
	)

	fmt.Println("\n=== 测试 1: NPM 包下载 ===")
	testNPMPackage(proxyDownloadSvc, repoRepo, db)

	fmt.Println("\n=== 测试 2: PyPI 包下载 ===")
	testPyPIPackage(proxyDownloadSvc, repoRepo, db)

	fmt.Println("\n=== 验证数据库存储 ===")
	verifyDatabaseStorage(db)

	fmt.Println("\n=== 测试完成 ===")
}

func testNPMPackage(svc *service.ProxyDownloadService, repoRepo *repository.RepositoryRepository, db *gorm.DB) {
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

	req := &service.ProxyDownloadRequest{
		PkgType:        "npm",
		Name:           "lodash",
		Version:        "4.17.21",
		Filename:       "lodash-4.17.21.tgz",
		Repo:           repo,
		PackageType:    model.PackageTypeNPM,
		RepositoryType: model.RepoTypeProxy,
		FileType:       model.FileTypePrimary,
		ResolutionMode: service.ResolutionModeProxyOnly,
		IPAddress:      "127.0.0.1",
		UserAgent:      "test-script",
		URLBuilder: func(repo *model.Repository, name, version string) string {
			return fmt.Sprintf("%s/%s/-/%s-%s.tgz", repo.RemoteURL, name, name, version)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := svc.Download(ctx, req)
	if err != nil {
		fmt.Printf("下载 lodash 失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 下载 lodash 成功!\n")
	fmt.Printf("  - 大小: %d bytes\n", result.Size)
	fmt.Printf("  - 存储路径: %s\n", result.StorageKey)
	fmt.Printf("  - 来自缓存: %v\n", result.FromCache)
}

func testPyPIPackage(svc *service.ProxyDownloadService, repoRepo *repository.RepositoryRepository, db *gorm.DB) {
	repo := &model.Repository{
		Name:        "pypi-proxy-test",
		DisplayName: "PyPI Proxy Test",
		Type:        model.RepoTypeProxy,
		PackageType: string(model.PackageTypePyPI),
		RemoteURL:   "https://pypi.org/simple",
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

	req := &service.ProxyDownloadRequest{
		PkgType:        "pypi",
		Name:           "requests",
		Version:        "2.31.0",
		Filename:       "requests-2.31.0-py3-none-any.whl",
		Repo:           repo,
		PackageType:    model.PackageTypePyPI,
		RepositoryType: model.RepoTypeProxy,
		FileType:       model.FileTypePrimary,
		ResolutionMode: service.ResolutionModeProxyOnly,
		IPAddress:      "127.0.0.1",
		UserAgent:      "test-script",
		URLBuilder: func(repo *model.Repository, name, _ string) string {
			base := strings.TrimSuffix(repo.RemoteURL, "/")
			if !strings.HasSuffix(base, "/simple") {
				base = base + "/simple"
			}
			return fmt.Sprintf("%s/%s/", base, name)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := svc.Download(ctx, req)
	if err != nil {
		fmt.Printf("下载 requests 失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 下载 requests 成功!\n")
	fmt.Printf("  - 大小: %d bytes\n", result.Size)
	fmt.Printf("  - 存储路径: %s\n", result.StorageKey)
	fmt.Printf("  - 来自缓存: %v\n", result.FromCache)
}

func verifyDatabaseStorage(db *gorm.DB) {
	var packages []model.Package
	if err := db.Preload("Versions").Preload("Versions.Files").Find(&packages).Error; err != nil {
		fmt.Printf("查询包失败: %v\n", err)
		return
	}

	fmt.Printf("\n数据库中的包记录:\n")
	for _, pkg := range packages {
		fmt.Printf("\n📦 包名: %s\n", pkg.Name)
		fmt.Printf("   类型: %s\n", pkg.Type)
		fmt.Printf("   仓库ID: %d\n", pkg.RepositoryID)
		fmt.Printf("   下载次数: %d\n", pkg.DownloadCount)

		for _, version := range pkg.Versions {
			fmt.Printf("   版本: %s (状态: %s)\n", version.Version, version.Status)
			fmt.Printf("      存储路径: %s\n", version.StoragePath)
			fmt.Printf("      下载次数: %d\n", version.DownloadCount)

			for _, file := range version.Files {
				fmt.Printf("      文件: %s (%d bytes)\n", file.Filename, file.SizeBytes)
				fmt.Printf("         存储路径: %s\n", file.StoragePath)
			}
		}
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
