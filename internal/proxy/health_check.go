package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"gorm.io/gorm"
)

// StorageChecker 存储后端检查接口，避免循环依赖
type StorageChecker interface {
	GetDefaultBackendPath() (string, error)
	CheckStoragePath(path string) error
}

// HealthCheckConfig 健康检查配置
type HealthCheckConfig struct {
	Enabled          bool          `json:"enabled"`           // 是否启用健康检查
	Interval         time.Duration `json:"interval"`          // 检查间隔
	Timeout          time.Duration `json:"timeout"`           // 检查超时
	FailureThreshold int           `json:"failure_threshold"` // 失败阈值
}

// DefaultHealthCheckConfig 默认健康检查配置
func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		Enabled:          true,
		Interval:         30 * time.Second, // 每30秒检查一次
		Timeout:          5 * time.Second,  // 检查超时5秒
		FailureThreshold: 3,                // 连续3次失败标记为不健康
	}
}

// HealthStatus 仓库健康状态
type HealthStatus struct {
	RepoID              uint          `json:"repo_id"`
	RepoName            string        `json:"repo_name"`
	IsHealthy           bool          `json:"is_healthy"`
	LastCheckTime       time.Time     `json:"last_check_time"`
	LastCheckError      string        `json:"last_check_error,omitempty"`
	ResponseTime        time.Duration `json:"response_time"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	StatusCode          int           `json:"status_code,omitempty"`
}

// HealthCheckService 健康检查服务
type HealthCheckService struct {
	mu             sync.RWMutex
	db             *gorm.DB
	repoRepo       *repository.RepositoryRepository
	storageChecker StorageChecker
	remoteClient   *RemoteClient
	config         HealthCheckConfig
	breakers       map[uint]*CircuitBreaker // repoID -> CircuitBreaker
	statuses       map[uint]*HealthStatus   // repoID -> HealthStatus
	stopCh         chan struct{}
	running        bool
}

// NewHealthCheckService 创建健康检查服务
func NewHealthCheckService(
	db *gorm.DB,
	repoRepo *repository.RepositoryRepository,
	storageChecker StorageChecker,
	remoteClient *RemoteClient,
	config HealthCheckConfig,
) *HealthCheckService {
	return &HealthCheckService{
		db:             db,
		repoRepo:       repoRepo,
		storageChecker: storageChecker,
		remoteClient:   remoteClient,
		config:         config,
		breakers:       make(map[uint]*CircuitBreaker),
		statuses:       make(map[uint]*HealthStatus),
		stopCh:         make(chan struct{}),
	}
}

// Start 启动健康检查服务
func (h *HealthCheckService) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running || !h.config.Enabled {
		return
	}

	h.running = true
	h.stopCh = make(chan struct{})

	slog.Info("starting health check service", "interval", h.config.Interval)
	go h.runHealthChecks()
}

// Stop 停止健康检查服务
func (h *HealthCheckService) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return
	}

	h.running = false
	close(h.stopCh)
	slog.Info("stopped health check service")
}

// GetCircuitBreaker 获取指定仓库的断路器
func (h *HealthCheckService) GetCircuitBreaker(repoID uint) *CircuitBreaker {
	h.mu.RLock()
	defer h.mu.RUnlock()

	cb, exists := h.breakers[repoID]
	if !exists {
		return nil
	}
	return cb
}

// GetOrCreateCircuitBreaker 获取或创建断路器
func (h *HealthCheckService) GetOrCreateCircuitBreaker(repoID uint) *CircuitBreaker {
	h.mu.Lock()
	defer h.mu.Unlock()

	cb, exists := h.breakers[repoID]
	if !exists {
		cb = NewCircuitBreaker(DefaultCircuitBreakerConfig())
		h.breakers[repoID] = cb
	}
	return cb
}

// GetHealthStatus 获取仓库健康状态
func (h *HealthCheckService) GetHealthStatus(repoID uint) *HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status, exists := h.statuses[repoID]
	if !exists {
		return nil
	}
	return status
}

// GetAllHealthStatuses 获取所有仓库健康状态
func (h *HealthCheckService) GetAllHealthStatuses() map[uint]*HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[uint]*HealthStatus)
	for repoID, status := range h.statuses {
		result[repoID] = status
	}
	return result
}

// ResetCircuitBreaker 重置指定仓库的断路器
func (h *HealthCheckService) ResetCircuitBreaker(repoID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if cb, exists := h.breakers[repoID]; exists {
		cb.Reset()
		slog.Info("circuit breaker reset", "repo_id", repoID)
	}
}

// runHealthChecks 定期执行健康检查
func (h *HealthCheckService) runHealthChecks() {
	ticker := time.NewTicker(h.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkAllRepos()
		case <-h.stopCh:
			return
		}
	}
}

// checkAllRepos 检查所有仓库的健康状态
func (h *HealthCheckService) checkAllRepos() {
	// 查询所有启用的仓库
	var repos []model.Repository
	if err := h.db.Where("enabled = ?", true).Find(&repos).Error; err != nil {
		slog.Error("failed to query repositories", "error", err)
		return
	}

	slog.Debug("checking health of all repositories", "count", len(repos))

	for _, repo := range repos {
		h.checkRepoHealth(&repo)
	}
}

// checkRepoHealth 检查单个仓库的健康状态
func (h *HealthCheckService) checkRepoHealth(repo *model.Repository) {
	ctx, cancel := context.WithTimeout(context.Background(), h.config.Timeout)
	defer cancel()

	cb := h.GetOrCreateCircuitBreaker(repo.ID)
	startTime := time.Now()

	// 如果断路器处于熔断状态，跳过检查
	if !cb.AllowRequest() {
		retryAfter := cb.GetRetryAfter()
		h.updateStatus(repo, false, fmt.Sprintf("circuit breaker open, retry after %d seconds", retryAfter), 0, 0)
		return
	}

	// 根据仓库类型执行不同的健康检查
	var statusCode int
	var err error

	switch repo.Type {
	case model.RepoTypeProxy:
		// 代理仓库：检查远程URL可达性
		statusCode, err = h.checkProxyRepo(ctx, repo)
	case model.RepoTypeLocal:
		// 本地仓库：检查存储后端可用性
		statusCode, err = h.checkLocalRepo(repo)
	case model.RepoTypeVirtual:
		// 虚拟仓库：检查成员仓库可用性
		statusCode, err = h.checkVirtualRepo(repo)
	default:
		// 未知类型，标记为不健康
		err = fmt.Errorf("unknown repository type: %s", repo.Type)
	}

	responseTime := time.Since(startTime)

	if err != nil {
		// 检查失败，记录失败
		cb.RecordFailure()
		h.updateStatus(repo, false, err.Error(), statusCode, responseTime)
		slog.Warn("health check failed",
			"repo", repo.Name,
			"type", repo.Type,
			"error", err,
			"response_time", responseTime)
	} else {
		// 检查成功
		cb.RecordSuccess()
		h.updateStatus(repo, true, "", statusCode, responseTime)
		slog.Debug("health check passed",
			"repo", repo.Name,
			"type", repo.Type,
			"status_code", statusCode,
			"response_time", responseTime)
	}
}

// checkProxyRepo 检查代理仓库的健康状态
func (h *HealthCheckService) checkProxyRepo(ctx context.Context, repo *model.Repository) (int, error) {
	healthURL := repo.RemoteURL
	return h.doHealthCheck(ctx, healthURL)
}

// checkLocalRepo 检查本地仓库的健康状态
func (h *HealthCheckService) checkLocalRepo(repo *model.Repository) (int, error) {
	var storagePath string

	if repo.StorageBackendID != nil {
		// 查询存储后端配置
		var backend model.StorageBackend
		if err := h.db.First(&backend, *repo.StorageBackendID).Error; err != nil {
			return 0, fmt.Errorf("storage backend not found: %w", err)
		}

		// 根据存储类型获取路径
		switch backend.Type {
		case model.StorageTypeLocal:
			if backend.Config.Local == nil {
				return 0, fmt.Errorf("local storage config missing")
			}
			storagePath = backend.Config.Local.BasePath
		case model.StorageTypeS3, model.StorageTypeOBS:
			// 远程存储：配置存在即视为健康
			if backend.Type == model.StorageTypeS3 && backend.Config.S3 == nil {
				return 0, fmt.Errorf("s3 storage config missing")
			}
			if backend.Type == model.StorageTypeOBS && backend.Config.OBS == nil {
				return 0, fmt.Errorf("obs storage config missing")
			}
			return http.StatusOK, nil
		default:
			return 0, fmt.Errorf("unsupported storage type: %s", backend.Type)
		}
	} else {
		// 没有配置存储后端，使用默认后端
		if h.storageChecker == nil {
			return 0, fmt.Errorf("no storage backend configured and no default backend available")
		}
		defaultPath, err := h.storageChecker.GetDefaultBackendPath()
		if err != nil {
			return 0, fmt.Errorf("failed to get default backend: %w", err)
		}
		storagePath = defaultPath
	}

	// 检查存储路径
	if h.storageChecker != nil {
		return http.StatusOK, h.storageChecker.CheckStoragePath(storagePath)
	}

	// 兼容旧逻辑：直接检查路径
	return h.checkStoragePath(storagePath)
}

// checkStoragePath 检查存储路径是否可用
func (h *HealthCheckService) checkStoragePath(basePath string) (int, error) {
	if _, err := os.Stat(basePath); err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("storage path does not exist: %s", basePath)
		}
		return 0, fmt.Errorf("storage path inaccessible: %w", err)
	}

	// 检查目录是否可写
	testFile := filepath.Join(basePath, ".health_check")
	if err := os.WriteFile(testFile, []byte{}, 0644); err != nil {
		return 0, fmt.Errorf("storage path not writable: %w", err)
	}
	// 清理测试文件
	os.Remove(testFile)

	return http.StatusOK, nil
}

// checkVirtualRepo 检查虚拟仓库的健康状态
func (h *HealthCheckService) checkVirtualRepo(repo *model.Repository) (int, error) {
	// 检查虚拟仓库是否有成员
	var memberCount int64
	if err := h.db.Model(&model.RepositoryGroup{}).
		Where("virtual_repo_id = ?", repo.ID).
		Count(&memberCount).Error; err != nil {
		return 0, fmt.Errorf("failed to check members: %w", err)
	}

	if memberCount == 0 {
		return 0, fmt.Errorf("no member repositories configured")
	}

	// 检查成员仓库是否都启用
	var disabledMembers int64
	if err := h.db.Table("repository_groups rg").
		Joins("JOIN repositories r ON rg.member_repo_id = r.id").
		Where("rg.virtual_repo_id = ? AND r.enabled = ?", repo.ID, false).
		Count(&disabledMembers).Error; err != nil {
		return 0, fmt.Errorf("failed to check member status: %w", err)
	}

	if disabledMembers > 0 {
		// 有成员被禁用，但仍然视为健康（部分功能可用）
		return http.StatusPartialContent, nil
	}

	return http.StatusOK, nil
}

// doHealthCheck 执行HTTP健康检查
func (h *HealthCheckService) doHealthCheck(ctx context.Context, rawURL string) (int, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return 0, fmt.Errorf("invalid URL: %w", err)
	}

	serverName := parsedURL.Hostname()

	client := &http.Client{
		Timeout: h.config.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         serverName,
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 2 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Moonlight-HealthCheck/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, nil
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return resp.StatusCode, nil
	}

	return resp.StatusCode, fmt.Errorf("server error: %d", resp.StatusCode)
}

// updateStatus 更新健康状态
func (h *HealthCheckService) updateStatus(repo *model.Repository, healthy bool, errMsg string, statusCode int, responseTime time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	status, exists := h.statuses[repo.ID]
	if !exists {
		status = &HealthStatus{
			RepoID:   repo.ID,
			RepoName: repo.Name,
		}
		h.statuses[repo.ID] = status
	}

	status.IsHealthy = healthy
	status.LastCheckTime = time.Now()
	status.ResponseTime = responseTime
	status.StatusCode = statusCode

	if errMsg != "" {
		status.LastCheckError = errMsg
		if !healthy {
			status.ConsecutiveFailures++
		}
	} else {
		status.LastCheckError = ""
		status.ConsecutiveFailures = 0
	}
}

// IsHealthy 检查指定仓库是否健康
func (h *HealthCheckService) IsHealthy(repoID uint) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status, exists := h.statuses[repoID]
	if !exists {
		return false // 没有检查记录时视为未知状态，不默认为健康
	}

	return status.IsHealthy && status.ConsecutiveFailures < h.config.FailureThreshold
}

// ShouldSkipRequest 判断是否应该跳过请求（断路器打开）
func (h *HealthCheckService) ShouldSkipRequest(repoID uint) bool {
	cb := h.GetCircuitBreaker(repoID)
	if cb == nil {
		return false
	}
	return !cb.AllowRequest()
}

// GetRetryAfter 获取重试等待时间（秒）
func (h *HealthCheckService) GetRetryAfter(repoID uint) int {
	cb := h.GetCircuitBreaker(repoID)
	if cb == nil {
		return 0
	}
	return cb.GetRetryAfter()
}
