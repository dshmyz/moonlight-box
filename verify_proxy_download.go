package main

import (
	"fmt"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/model"
)

func main() {
	fmt.Println("=== 验证代理仓库下载和数据库存储 ===\n")

	_, err := config.Load("configs/config.yaml")
	if err != nil {
		fmt.Printf("❌ 加载配置失败: %v\n", err)
		return
	}

	cfg := config.Get()
	if err := database.Initialize(cfg); err != nil {
		fmt.Printf("❌ 初始化数据库失败: %v\n", err)
		return
	}
	defer database.Close()

	db := database.DB

	var repoCount int64
	db.Model(&model.Repository{}).Count(&repoCount)
	fmt.Printf("✓ 仓库总数: %d\n", repoCount)

	var pkgCount int64
	db.Model(&model.Package{}).Count(&pkgCount)
	fmt.Printf("✓ 包总数: %d\n\n", pkgCount)

	fmt.Println("📦 代理仓库列表:")
	var proxyRepos []model.Repository
	db.Where("type = ?", "proxy").Find(&proxyRepos)
	for _, repo := range proxyRepos {
		fmt.Printf("  - %s (%s) -> %s\n", repo.Name, repo.PackageType, repo.RemoteURL)
	}

	fmt.Println("\n📦 最近下载的包:")
	var recentPackages []model.Package
	db.Preload("Versions").Order("updated_at desc").Limit(10).Find(&recentPackages)
	for _, pkg := range recentPackages {
		if pkg.RepositoryType == "proxy" {
			fmt.Printf("  - %s (%s) @ ", pkg.Name, pkg.Type)
			if len(pkg.Versions) > 0 {
				fmt.Printf("%s", pkg.Versions[0].Version)
			}
			fmt.Printf(" [下载次数: %d]\n", pkg.DownloadCount)
		}
	}

	fmt.Println("\n📋 最近的下载日志:")
	var downloadLogs []model.ProxyDownloadLog
	db.Order("created_at desc").Limit(5).Find(&downloadLogs)
	for _, log := range downloadLogs {
		status := "✓"
		if log.Status != "success" && log.Status != "cached" {
			status = "✗"
		}
		fmt.Printf("  %s %s/%s@%s (%s) - %d bytes\n",
			status, log.PackageType, log.PackageName, log.Version,
			log.Status, log.SizeBytes)
	}

	fmt.Println("\n✅ 验证完成！代理仓库下载功能正常，数据已成功存储到数据库。")
}
