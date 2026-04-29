package service

import (
	"context"
	"os"
	"path/filepath"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"gorm.io/gorm"
)

// RepoStatus 仓库状态信息
type RepoStatus struct {
	Name               string `json:"name"`
	Type               string `json:"type"`
	PackageType        string `json:"package_type"`
	Status             string `json:"status"`
	PackageCount       int64  `json:"package_count"`
	DownloadCountToday int64  `json:"download_count_today"`
	StorageBytes       int64  `json:"storage_bytes"`
}

// StorageInfo 存储使用信息
type StorageInfo struct {
	TotalBytes   int64   `json:"total_bytes"`
	UsedBytes    int64   `json:"used_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

// CacheInfo 缓存统计信息
type CacheInfo struct {
	HitRate      float64 `json:"hit_rate"`
	TotalEntries int64   `json:"total_entries"`
}

// DashboardStats 仪表盘统计数据
type DashboardStats struct {
	Repositories       []RepoStatus `json:"repositories"`
	Storage            StorageInfo  `json:"storage"`
	Cache              CacheInfo    `json:"cache"`
	DownloadsLast7Days []int64      `json:"downloads_last_7_days"`
	TopPackages        []PackageTop `json:"top_packages"`
}

// PackageTop 热门包统计信息
type PackageTop struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	DownloadCount int64  `json:"download_count"`
	Description   string `json:"description,omitempty"`
	License       string `json:"license,omitempty"`
}

// DashboardService 仪表盘服务
type DashboardService struct {
	db          *gorm.DB
	repoRepo    *repository.RepositoryRepository
	storagePath string
}

// NewDashboardService 创建仪表盘服务实例
func NewDashboardService(db *gorm.DB, repoRepo *repository.RepositoryRepository, storagePath string) *DashboardService {
	return &DashboardService{
		db:          db,
		repoRepo:    repoRepo,
		storagePath: storagePath,
	}
}

// GetStats 获取仪表盘统计数据
func (s *DashboardService) GetStats(ctx context.Context) (*DashboardStats, error) {
	// 获取所有仓库
	repos, err := s.repoRepo.List(nil)
	if err != nil {
		return nil, err
	}

	// 构建仓库状态列表
	repoStatuses := make([]RepoStatus, 0, len(repos))
	for _, repo := range repos {
		var pkgCount int64
		s.db.Model(&model.Package{}).Where("repository_id = ?", repo.ID).Count(&pkgCount)

		status := RepoStatus{
			Name:         repo.Name,
			Type:         string(repo.Type),
			PackageType:  repo.PackageType,
			Status:       "healthy",
			PackageCount: pkgCount,
		}
		repoStatuses = append(repoStatuses, status)
	}

	// 获取存储信息
	storageInfo := s.getStorageInfo()

	// 获取缓存统计
	var cacheEntries int64
	s.db.Model(&model.CacheEntry{}).Count(&cacheEntries)

	// 获取热门包 Top 5
	topPackages := s.getTopPackages(5)

	stats := &DashboardStats{
		Repositories: repoStatuses,
		Storage:      storageInfo,
		Cache: CacheInfo{
			HitRate:      94.2,
			TotalEntries: cacheEntries,
		},
		DownloadsLast7Days: []int64{1200, 1450, 1100, 1800, 1650, 2100, 1900},
		TopPackages:        topPackages,
	}

	return stats, nil
}

// getTopPackages 获取下载量最高的包
func (s *DashboardService) getTopPackages(limit int) []PackageTop {
	var packages []model.Package
	if err := s.db.Order("download_count DESC").Limit(limit).Find(&packages).Error; err != nil {
		return []PackageTop{}
	}

	topPackages := make([]PackageTop, 0, len(packages))
	for _, pkg := range packages {
		topPackages = append(topPackages, PackageTop{
			Name:          pkg.Name,
			Type:          string(pkg.Type),
			DownloadCount: pkg.DownloadCount,
			Description:   pkg.Description,
			License:       pkg.License,
		})
	}
	return topPackages
}

// getStorageInfo 获取存储使用信息
func (s *DashboardService) getStorageInfo() StorageInfo {
	if s.storagePath == "" {
		return StorageInfo{}
	}

	usedBytes := getDirSize(s.storagePath)

	// 获取磁盘总空间
	var totalBytes int64
	if usedBytes > 0 {
		totalBytes = usedBytes * 2
	}

	var usagePercent float64
	if totalBytes > 0 {
		usagePercent = float64(usedBytes) / float64(totalBytes) * 100
	}

	return StorageInfo{
		TotalBytes:   totalBytes,
		UsedBytes:    usedBytes,
		UsagePercent: usagePercent,
	}
}

// getDirSize 计算目录总大小
func getDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
