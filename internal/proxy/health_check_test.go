package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/types"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockStorageChecker struct{}

func (m *mockStorageChecker) GetDefaultBackendPath() (string, error) {
	return "/tmp/test-storage", nil
}

func (m *mockStorageChecker) CheckStoragePath(path string) error {
	return nil
}

type mockConfigReader struct{}

func (m *mockConfigReader) GetConfigAsBool(key string, defaultValue bool) bool {
	return defaultValue
}

type testMockAdapter struct{}

func (m *testMockAdapter) Type() types.PackageType { return "maven" }
func (m *testMockAdapter) RoutePrefix() string     { return "/maven" }
func (m *testMockAdapter) ParsePackagePath(path string) (*types.PackageIdentity, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *testMockAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	return &types.PackagePathInfo{
		Name:       path,
		RemotePath: path,
	}, nil
}
func (m *testMockAdapter) BuildRemotePath(name, version, filename string) string {
	return fmt.Sprintf("%s/%s/%s", name, version, filename)
}
func (m *testMockAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *testMockAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	return fmt.Errorf("not implemented")
}
func (m *testMockAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	return &types.RequestIntent{Type: types.RequestUnknown}
}
func (m *testMockAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *testMockAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockConfigReader) GetConfigAsInt(key string, defaultValue int) int {
	return defaultValue
}

func TestHealthCheckService_BasicCheck(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// 初始化测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	db.AutoMigrate(&model.Repository{})

	// 创建测试仓库
	repo := &model.Repository{
		Name:    "test-proxy",
		Type:    model.RepoTypeProxy,
		Enabled: true,
		Config: &model.RepositoryConfig{
			RemoteURL: server.URL,
		},
	}
	db.Create(repo)

	// 创建远程客户端
	tm := NewTransportManager(5*time.Second, NewDNSResolver(nil))
	remoteClient := NewRemoteClient(tm, 10)

	// 创建健康检查服务
	config := HealthCheckConfig{
		Enabled:          true,
		Interval:         100 * time.Millisecond,
		Timeout:          2 * time.Second,
		FailureThreshold: 3,
	}

	repoRepo := repository.NewRepositoryRepository(db)
	healthSvc := NewHealthCheckService(db, repoRepo, &mockStorageChecker{}, remoteClient, config, &mockConfigReader{})

	// 执行健康检查
	healthSvc.checkRepoHealth(repo)

	// 验证健康状态
	status := healthSvc.GetHealthStatus(repo.ID)
	assert.NotNil(t, status)
	assert.True(t, status.IsHealthy)
	assert.Empty(t, status.LastCheckError)

	// 验证断路器状态
	cb := healthSvc.GetCircuitBreaker(repo.ID)
	assert.NotNil(t, cb)
	assert.Equal(t, CircuitClosed, cb.GetState())
}

func TestHealthCheckService_ReusesRemoteClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tm := NewTransportManager(5*time.Second, NewDNSResolver(nil))
	remoteClient := NewRemoteClient(tm, 10)
	healthSvc := NewHealthCheckService(nil, nil, &mockStorageChecker{}, remoteClient, HealthCheckConfig{
		Timeout: 2 * time.Second,
	}, &mockConfigReader{})

	for i := 0; i < 2; i++ {
		status, err := healthSvc.doHealthCheck(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("health check %d failed: %v", i, err)
		}
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
	}

	cacheEntries := 0
	remoteClient.clientCache.Range(func(_, _ interface{}) bool {
		cacheEntries++
		return true
	})
	if cacheEntries != 1 {
		t.Fatalf("remote client cache entries = %d, want 1 reused client", cacheEntries)
	}
}

func TestHealthCheckService_FailedCheck(t *testing.T) {
	// 创建总是返回500的测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// 初始化测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	db.AutoMigrate(&model.Repository{})

	// 创建测试仓库
	repo := &model.Repository{
		Name:    "test-proxy-fail",
		Type:    model.RepoTypeProxy,
		Enabled: true,
		Config: &model.RepositoryConfig{
			RemoteURL: server.URL,
		},
	}
	db.Create(repo)

	// 创建远程客户端
	tm := NewTransportManager(5*time.Second, NewDNSResolver(nil))
	remoteClient := NewRemoteClient(tm, 10)

	// 创建健康检查服务
	config := HealthCheckConfig{
		Enabled:          true,
		Interval:         100 * time.Millisecond,
		Timeout:          2 * time.Second,
		FailureThreshold: 3,
	}

	repoRepo := repository.NewRepositoryRepository(db)
	healthSvc := NewHealthCheckService(db, repoRepo, &mockStorageChecker{}, remoteClient, config, &mockConfigReader{})

	// 执行3次失败的健康检查
	healthSvc.checkRepoHealth(repo)
	healthSvc.checkRepoHealth(repo)
	healthSvc.checkRepoHealth(repo)

	// 验证健康状态
	status := healthSvc.GetHealthStatus(repo.ID)
	assert.NotNil(t, status)
	assert.False(t, status.IsHealthy)
	assert.Equal(t, 3, status.ConsecutiveFailures)
}

func TestHealthCheckService_CircuitBreakerIntegration(t *testing.T) {
	// 初始化测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	db.AutoMigrate(&model.Repository{})

	// 创建测试仓库
	repo := &model.Repository{
		Name:    "test-proxy-cb",
		Type:    model.RepoTypeProxy,
		Enabled: true,
		Config: &model.RepositoryConfig{
			RemoteURL: "http://invalid-host-that-does-not-exist:12345",
		},
	}
	db.Create(repo)

	// 创建远程客户端
	tm := NewTransportManager(1*time.Second, NewDNSResolver(nil))
	remoteClient := NewRemoteClient(tm, 10)

	// 创建健康检查服务
	config := HealthCheckConfig{
		Enabled:          true,
		Interval:         100 * time.Millisecond,
		Timeout:          500 * time.Millisecond,
		FailureThreshold: 3,
	}

	repoRepo := repository.NewRepositoryRepository(db)
	healthSvc := NewHealthCheckService(db, repoRepo, &mockStorageChecker{}, remoteClient, config, &mockConfigReader{})

	// 执行多次失败的健康检查，触发断路器
	for i := 0; i < 5; i++ {
		healthSvc.checkRepoHealth(repo)
	}

	// 验证断路器已经打开
	cb := healthSvc.GetCircuitBreaker(repo.ID)
	assert.NotNil(t, cb)
	assert.Equal(t, CircuitOpen, cb.GetState())

	// 验证应该跳过请求
	assert.True(t, healthSvc.ShouldSkipRequest(repo.ID))

	// 验证有重试等待时间
	retryAfter := healthSvc.GetRetryAfter(repo.ID)
	assert.True(t, retryAfter > 0)

	// 重置断路器
	healthSvc.ResetCircuitBreaker(repo.ID)

	// 验证断路器已重置
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.False(t, healthSvc.ShouldSkipRequest(repo.ID))
}

func TestHealthCheckService_Recovery(t *testing.T) {
	// 创建可控制的测试服务器
	var failFlag bool = true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failFlag {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// 初始化测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	db.AutoMigrate(&model.Repository{})

	// 创建测试仓库
	repo := &model.Repository{
		Name:    "test-proxy-recovery",
		Type:    model.RepoTypeProxy,
		Config:  &model.RepositoryConfig{RemoteURL: server.URL},
		Enabled: true,
	}
	db.Create(repo)

	// 创建远程客户端
	tm := NewTransportManager(5*time.Second, NewDNSResolver(nil))
	remoteClient := NewRemoteClient(tm, 10)

	// 创建健康检查服务，使用较短的重置超时
	config := HealthCheckConfig{
		Enabled:          true,
		Interval:         100 * time.Millisecond,
		Timeout:          2 * time.Second,
		FailureThreshold: 3,
	}

	repoRepo := repository.NewRepositoryRepository(db)
	healthSvc := NewHealthCheckService(db, repoRepo, &mockStorageChecker{}, remoteClient, config, &mockConfigReader{})

	// 手动设置一个较短的reset timeout的断路器
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:   5,
		ResetTimeout:  100 * time.Millisecond,
		ProbeInterval: 50 * time.Millisecond,
	})
	healthSvc.breakers[repo.ID] = cb

	// 先模拟失败
	failFlag = true
	for i := 0; i < 5; i++ {
		healthSvc.checkRepoHealth(repo)
	}

	// 验证断路器打开
	assert.Equal(t, CircuitOpen, cb.GetState())

	// 现在让服务器成功响应
	failFlag = false

	// 等待重置超时，让断路器进入半开状态
	time.Sleep(150 * time.Millisecond)

	// 执行健康检查（应该在半开状态下成功）
	healthSvc.checkRepoHealth(repo)

	// 验证断路器恢复到关闭状态
	assert.Equal(t, CircuitClosed, cb.GetState())
}

func TestHealthCheckService_StartStop(t *testing.T) {
	// 初始化测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	db.AutoMigrate(&model.Repository{})

	// 创建远程客户端
	tm := NewTransportManager(5*time.Second, NewDNSResolver(nil))
	remoteClient := NewRemoteClient(tm, 10)

	// 创建健康检查服务
	config := HealthCheckConfig{
		Enabled:          true,
		Interval:         50 * time.Millisecond,
		Timeout:          2 * time.Second,
		FailureThreshold: 3,
	}

	repoRepo := repository.NewRepositoryRepository(db)
	healthSvc := NewHealthCheckService(db, repoRepo, &mockStorageChecker{}, remoteClient, config, &mockConfigReader{})

	// 启动服务
	healthSvc.Start()
	time.Sleep(200 * time.Millisecond)

	// 停止服务
	healthSvc.Stop()
	time.Sleep(50 * time.Millisecond)

	// 再次启动（应该可以正常工作）
	healthSvc.Start()
	time.Sleep(100 * time.Millisecond)
	healthSvc.Stop()
}
