package service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
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
	db             *gorm.DB
	repoRepo       *repository.RepositoryRepository
	healthCheckSvc *proxy.HealthCheckService
	storagePath    string

	cacheMu        sync.RWMutex
	cachedStats    *DashboardStats
	cacheTime      time.Time
	cacheTTL       time.Duration
	storageSize    int64
	storageSizeMu  sync.RWMutex
	storageUpdated time.Time

	cacheInfoMu        sync.RWMutex
	cachedCacheInfo    *CacheInfo
	cacheInfoUpdatedAt time.Time
	cacheInfoTTL       time.Duration

	stopCh chan struct{}
}

// NewDashboardService 创建仪表盘服务实例
func NewDashboardService(db *gorm.DB, repoRepo *repository.RepositoryRepository, healthCheckSvc *proxy.HealthCheckService, storagePath string) *DashboardService {
	svc := &DashboardService{
		db:             db,
		repoRepo:       repoRepo,
		healthCheckSvc: healthCheckSvc,
		storagePath:    storagePath,
		cacheTTL:       30 * time.Second,
		cacheInfoTTL:   5 * time.Minute,
		stopCh:         make(chan struct{}),
	}

	// 启动后台任务定期计算目录大小
	go svc.storageSizeWorker()

	return svc
}

// storageSizeWorker 后台定期计算目录大小
func (s *DashboardService) storageSizeWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// 立即执行一次
	s.updateStorageSize()

	for {
		select {
		case <-ticker.C:
			s.updateStorageSize()
		case <-s.stopCh:
			return
		}
	}
}

// updateStorageSize 更新存储大小
func (s *DashboardService) updateStorageSize() {
	if s.storagePath == "" {
		return
	}

	size := calculateDirSize(s.storagePath)

	s.storageSizeMu.Lock()
	s.storageSize = size
	s.storageUpdated = time.Now()
	s.storageSizeMu.Unlock()
}

// Stop 停止后台任务
func (s *DashboardService) Stop() {
	close(s.stopCh)
}

// GetStats 获取仪表盘统计数据
func (s *DashboardService) GetStats(ctx context.Context) (*DashboardStats, error) {
	s.cacheMu.RLock()
	if s.cachedStats != nil && time.Since(s.cacheTime) < s.cacheTTL {
		stats := s.cachedStats
		s.cacheMu.RUnlock()
		return stats, nil
	}
	s.cacheMu.RUnlock()

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if s.cachedStats != nil && time.Since(s.cacheTime) < s.cacheTTL {
		return s.cachedStats, nil
	}

	stats, err := s.computeStats(ctx)
	if err != nil {
		return nil, err
	}

	s.cachedStats = stats
	s.cacheTime = time.Now()
	return stats, nil
}

func (s *DashboardService) computeStats(ctx context.Context) (*DashboardStats, error) {
	repos, err := s.repoRepo.List(nil)
	if err != nil {
		return nil, err
	}

	var repoIDs []uint
	for _, repo := range repos {
		repoIDs = append(repoIDs, repo.ID)
	}

	var pkgCounts []struct {
		RepositoryID uint
		Count        int64
	}
	if len(repoIDs) > 0 {
		s.db.Model(&model.Package{}).
			Select("repository_id, COUNT(*) as count").
			Where("repository_id IN ?", repoIDs).
			Group("repository_id").
			Scan(&pkgCounts)
	}

	pkgCountMap := make(map[uint]int64)
	for _, pc := range pkgCounts {
		pkgCountMap[pc.RepositoryID] = pc.Count
	}

	var todayDownloads []struct {
		RepositoryID uint
		Count        int64
	}
	if len(repoIDs) > 0 {
		todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Now().Location())
		s.db.Model(&model.ProxyDownloadLog{}).
			Select("repository_id, COUNT(*) as count").
			Where("repository_id IN ? AND created_at >= ?", repoIDs, todayStart).
			Group("repository_id").
			Scan(&todayDownloads)
	}

	todayDownloadMap := make(map[uint]int64)
	for _, td := range todayDownloads {
		todayDownloadMap[td.RepositoryID] = td.Count
	}

	healthStatuses := s.getHealthStatuses()

	repoStatuses := make([]RepoStatus, 0, len(repos))
	for _, repo := range repos {
		healthStatus := healthStatuses[repo.ID]

		status := RepoStatus{
			Name:               repo.Name,
			Type:               string(repo.Type),
			PackageType:        repo.PackageType,
			Status:             s.getRepoHealthStatus(healthStatus),
			PackageCount:       pkgCountMap[repo.ID],
			DownloadCountToday: todayDownloadMap[repo.ID],
			StorageBytes:       s.getRepoStorageBytes(repo),
		}
		repoStatuses = append(repoStatuses, status)
	}

	storageInfo := s.getStorageInfo()
	cacheInfo := s.getCacheInfo()
	downloadsLast7Days := s.getDownloadsLast7Days()
	topPackages := s.getTopPackages(5)

	stats := &DashboardStats{
		Repositories:       repoStatuses,
		Storage:            storageInfo,
		Cache:              cacheInfo,
		DownloadsLast7Days: downloadsLast7Days,
		TopPackages:        topPackages,
	}

	return stats, nil
}

func (s *DashboardService) getHealthStatuses() map[uint]*proxy.HealthStatus {
	if s.healthCheckSvc == nil {
		return nil
	}
	return s.healthCheckSvc.GetAllHealthStatuses()
}

func (s *DashboardService) getRepoHealthStatus(healthStatus *proxy.HealthStatus) string {
	if healthStatus == nil {
		return "unknown"
	}

	if healthStatus.ConsecutiveFailures > 0 {
		return "warning"
	}

	if healthStatus.IsHealthy {
		return "healthy"
	}

	return "error"
}

func (s *DashboardService) getRepoStorageBytes(repo model.Repository) int64 {
	if repo.Type != model.RepoTypeLocal {
		return 0
	}

	if repo.StorageBackendID == nil {
		return s.getDirStorageBytes(s.storagePath)
	}

	var backend model.StorageBackend
	if err := s.db.First(&backend, *repo.StorageBackendID).Error; err != nil {
		return 0
	}

	if backend.Type == model.StorageTypeLocal && backend.Config.Local != nil {
		return s.getDirStorageBytes(backend.Config.Local.BasePath)
	}

	return 0
}

func (s *DashboardService) getDirStorageBytes(path string) int64 {
	if path == "" {
		return 0
	}
	return calculateDirSize(path)
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

	s.storageSizeMu.RLock()
	cached := s.storageSize
	lastUpdated := s.storageUpdated
	s.storageSizeMu.RUnlock()

	if cached > 0 && time.Since(lastUpdated) < 5*time.Minute {
		totalBytes := cached * 2
		usagePercent := float64(cached) / float64(totalBytes) * 100
		return StorageInfo{
			TotalBytes:   totalBytes,
			UsedBytes:    cached,
			UsagePercent: usagePercent,
		}
	}

	var totalBytes int64
	var usagePercent float64
	if cached > 0 {
		totalBytes = cached * 2
		usagePercent = float64(cached) / float64(totalBytes) * 100
	}

	// 返回缓存值，后台任务会定期更新
	return StorageInfo{
		TotalBytes:   totalBytes,
		UsedBytes:    cached,
		UsagePercent: usagePercent,
	}
}

// calculateDirSize 计算目录总大小
func calculateDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// getCacheInfo 获取缓存统计信息（带缓存优化）
func (s *DashboardService) getCacheInfo() CacheInfo {
	s.cacheInfoMu.RLock()
	if s.cachedCacheInfo != nil && time.Since(s.cacheInfoUpdatedAt) < s.cacheInfoTTL {
		cached := *s.cachedCacheInfo
		s.cacheInfoMu.RUnlock()
		return cached
	}
	s.cacheInfoMu.RUnlock()

	s.cacheInfoMu.Lock()
	defer s.cacheInfoMu.Unlock()

	if s.cachedCacheInfo != nil && time.Since(s.cacheInfoUpdatedAt) < s.cacheInfoTTL {
		return *s.cachedCacheInfo
	}

	cacheInfo := s.computeCacheInfo()
	s.cachedCacheInfo = &cacheInfo
	s.cacheInfoUpdatedAt = time.Now()
	return cacheInfo
}

// computeCacheInfo 实际计算缓存统计信息
func (s *DashboardService) computeCacheInfo() CacheInfo {
	var totalEntries int64
	s.db.Model(&model.CacheEntry{}).Count(&totalEntries)

	var totalLogs int64
	s.db.Model(&model.ProxyDownloadLog{}).Count(&totalLogs)

	var cachedLogs int64
	s.db.Model(&model.ProxyDownloadLog{}).Where("status = ?", model.DownloadStatusCached).Count(&cachedLogs)

	var hitRate float64
	if totalLogs > 0 {
		hitRate = float64(cachedLogs) / float64(totalLogs) * 100
		hitRate = float64(int(hitRate*10)) / 10
	}

	return CacheInfo{
		HitRate:      hitRate,
		TotalEntries: totalEntries,
	}
}

// getDownloadsLast7Days 获取最近 7 天每天的下载量
func (s *DashboardService) getDownloadsLast7Days() []int64 {
	result := make([]int64, 7)
	now := time.Now()

	sevenDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)

	type DailyCount struct {
		Date  time.Time
		Count int64
	}

	var dailyCounts []DailyCount
	s.db.Model(&model.ProxyDownloadLog{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at >= ?", sevenDaysAgo).
		Group("DATE(created_at)").
		Scan(&dailyCounts)

	countMap := make(map[string]int64)
	for _, dc := range dailyCounts {
		countMap[dc.Date.Format("2006-01-02")] = dc.Count
	}

	for i := 6; i >= 0; i-- {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -i)
		dateKey := day.Format("2006-01-02")
		result[6-i] = countMap[dateKey]
	}

	return result
}
