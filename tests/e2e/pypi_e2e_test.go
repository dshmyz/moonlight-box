package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	// "github.com/moonlight-box/registry/internal/plugins"
	"github.com/moonlight-box/registry/internal/database"
	handler "github.com/moonlight-box/registry/internal/api/http"
	"github.com/moonlight-box/registry/internal/middleware"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

var (
	pypiTestServer  *httptest.Server
	pypiTestDB      *gorm.DB
	pypiAdapter     adapter.RepoAwareAdapter
	pypiRepoSvc     *service.RepositoryService
	pypiPkgRepo     *repository.ComponentRepository
	pypiStorageSvc  *service.StorageService
	pypiRepoHandler *proxy.RepoHandler
)

func setupPyPITestEnv() {
	gin.SetMode(gin.TestMode)

	var err error
	pypiTestDB, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	database.DB = pypiTestDB

	pypiTestDB.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.RolePermission{},
		&model.UserRole{},
		&model.Repository{},
		&model.RepositoryGroup{},
		&model.Component{},
		&model.Component{},
		&model.Asset{},
		&model.ComponentDependency{},
		&model.AuditLog{},
		&model.BlockRule{},
		&model.ScanResult{},
		&model.Vulnerability{},
		&model.StorageBackend{},
	)

	localStorage, _ := storage.NewLocalStorage("/tmp/test-pypi-e2e", 1024)
	storageBackendRepo := repository.NewStorageBackendRepository(pypiTestDB)
	pypiStorageSvc, _ = service.NewStorageService(storageBackendRepo, "", 0)
	pypiStorageSvc.SetDefaultBackendForTest(localStorage)

	pypiPkgRepo = repository.NewComponentRepository(pypiTestDB)
	repoRepo := repository.NewRepositoryRepository(pypiTestDB)
	groupRepo := repository.NewGroupRepository(pypiTestDB)

	pypiRepoSvc = service.NewRepositoryService(repoRepo, groupRepo, pypiTestDB)

	cacheSvc := proxy.NewCacheService()
	dnsResolver := proxy.NewDNSResolver(nil)
	tm := proxy.NewTransportManager(30*time.Second, dnsResolver)
	remoteClient := proxy.NewRemoteClient(tm, 5)
	proxyDownloader := proxy.NewProxyDownloader(cacheSvc, remoteClient, nil)

	repoCache := proxy.NewRepositoryCache(repoRepo, groupRepo, 5*time.Minute)
	pypiRepoHandler = proxy.NewRepoHandler(repoRepo, groupRepo, repoCache)

	pypiAdapter = adapter.NewPyPIAdapter(pypiPkgRepo, repoRepo, pypiStorageSvc, nil, nil)
	pypiRepoHandler.RegisterAdapter("pypi", pypiAdapter)

	logRepo := repository.NewProxyDownloadLogRepository(pypiTestDB)
	countBatcher := service.NewDownloadCountBatcher(pypiTestDB, 5*time.Second)
	downloadSvc := service.NewDownloadService(pypiPkgRepo, pypiStorageSvc, proxyDownloader, logRepo, nil, countBatcher)
	pypiRepoHandler.SetDownloadService(downloadSvc)

	router := setupPyPIRouter()
	pypiTestServer = httptest.NewServer(router)
}

func teardownPyPITestEnv() {
	if pypiTestServer != nil {
		pypiTestServer.Close()
	}
}

func setupPyPIRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS())

	authMw := func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	}
	permMw := func(resource, action string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	repoRouter := handler.NewRepoRouter(pypiRepoSvc)
	repoRouter.SetResolver(pypiRepoHandler)
	repoRouter.SetUploadService(service.NewUploadService(pypiPkgRepo, pypiStorageSvc))

	repoGroup := router.Group("/repo/:repoName")
	{
		repoGroup.GET("/*path", repoRouter.HandleRequest)
		repoGroup.PUT("/*path", authMw, permMw("pypi", "write"), repoRouter.HandlePublish)
		repoGroup.DELETE("/*path", authMw, permMw("pypi", "delete"), repoRouter.HandleDelete)
	}

	repoHandler := handler.NewRepositoryHandler(pypiRepoSvc)
	repoAPI := router.Group("/api/v1/repositories")
	{
		repoAPI.GET("", repoHandler.List)
		repoAPI.GET("/:name", repoHandler.Get)
		repoAPI.POST("", repoHandler.Create)
		repoAPI.PUT("/:name", repoHandler.Update)
		repoAPI.DELETE("/:name", repoHandler.Delete)
	}

	return router
}

func TestE2E_PyPI_UploadPackage(t *testing.T) {
	setupPyPITestEnv()
	defer teardownPyPITestEnv()

	repo := &model.Repository{
		Name:        "pypi-hosted-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypePyPI),
		Enabled:     true,
	}
	err := pypiRepoSvc.Create(repo, nil)
	assert.Nil(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, _ := writer.CreateFormFile("content", "test_package-1.0.0-py3-none-any.whl")
	part.Write([]byte("fake wheel content for e2e test"))

	writer.WriteField("name", "test-package")
	writer.WriteField("version", "1.0.0")
	writer.Close()

	req, _ := http.NewRequest("PUT", pypiTestServer.URL+"/repo/pypi-hosted-e2e/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 201)
}

func TestE2E_PyPI_ListPackages(t *testing.T) {
	setupPyPITestEnv()
	defer teardownPyPITestEnv()

	pypiPkgRepo.StoreComponentAsset(context.Background(), &model.Component{
		Name:        "requests",
		Format: model.PackageTypePyPI,
		Description: "HTTP library",
	}, &model.Component{
		Version:     "2.28.0",
		Status:      model.StatusPublished,
	}, &model.Asset{
		FileName:    "requests-2.28.0-py3-none-any.whl",
		Kind:    model.AssetKindPrimary,
		SizeBytes:   1000,
	})

	repo := &model.Repository{
		Name:        "pypi-list-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypePyPI),
		Enabled:     true,
	}
	pypiRepoSvc.Create(repo, nil)

	resp, err := http.Get(pypiTestServer.URL + "/repo/pypi-list-e2e/simple/")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "requests")
}

func TestE2E_PyPI_GetPackageFiles(t *testing.T) {
	setupPyPITestEnv()
	defer teardownPyPITestEnv()

	repo := &model.Repository{
		Name:        "pypi-files-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypePyPI),
		Enabled:     true,
	}
	pypiRepoSvc.Create(repo, nil)

	pypiPkgRepo.StoreComponentAsset(context.Background(), &model.Component{
		Name:         "flask",
		Format: model.PackageTypePyPI,
		Description:  "Flask framework",
		RepositoryID: repo.ID,
	}, &model.Component{
		Version:     "2.0.0",
		Status:      model.StatusPublished,
	}, &model.Asset{
		FileName:    "Flask-2.0.0-py3-none-any.whl",
		Kind:    model.AssetKindPrimary,
		SizeBytes:   1000,
	})

	resp, err := http.Get(pypiTestServer.URL + "/repo/pypi-files-e2e/simple/flask/")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Flask-2.0.0-py3-none-any.whl")
}

func TestE2E_PyPI_DownloadPackage(t *testing.T) {
	setupPyPITestEnv()
	defer teardownPyPITestEnv()

	repo := &model.Repository{
		Name:        "pypi-download-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypePyPI),
		Enabled:     true,
	}
	pypiRepoSvc.Create(repo, nil)

	pypiPkgRepo.StoreComponentAsset(context.Background(), &model.Component{
		Name:         "download-test",
		Format: model.PackageTypePyPI,
		Description:  "Download test package",
		RepositoryID: repo.ID,
	}, &model.Component{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
	}, &model.Asset{
		FileName:    "download_test-1.0.0-py3-none-any.whl",
		Kind:    model.AssetKindPrimary,
		SizeBytes:   1000,
	})

	resp, err := http.Get(pypiTestServer.URL + "/repo/pypi-download-e2e/packages/download_test-1.0.0-py3-none-any.whl")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestE2E_PyPI_ProxyRepository(t *testing.T) {
	setupPyPITestEnv()
	defer teardownPyPITestEnv()

	payload := map[string]interface{}{
		"name":         "pypi-proxy-e2e",
		"display_name": "PyPI Proxy Repository",
		"description":  "Proxy PyPI repository for e2e testing",
		"type":         "proxy",
		"package_type": "pypi",
		"remote_url":   "https://pypi.org",
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		pypiTestServer.URL+"/api/v1/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "https://pypi.org", data["remote_url"])
}

func TestE2E_PyPI_CompleteWorkflow(t *testing.T) {
	setupPyPITestEnv()
	defer teardownPyPITestEnv()

	repo := &model.Repository{
		Name:        "pypi-workflow-complete",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypePyPI),
		Enabled:     true,
	}
	err := pypiRepoSvc.Create(repo, nil)
	assert.Nil(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, _ := writer.CreateFormFile("content", "workflow-pkg-1.0.0-py3-none-any.whl")
	part.Write([]byte("fake wheel content for workflow test"))

	writer.WriteField("name", "workflow-pkg")
	writer.WriteField("version", "1.0.0")
	writer.Close()

	req, _ := http.NewRequest("PUT", pypiTestServer.URL+"/repo/pypi-workflow-complete/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 201)

	listResp, err := http.Get(pypiTestServer.URL + "/repo/pypi-workflow-complete/simple/")
	assert.Nil(t, err)
	assert.Equal(t, 200, listResp.StatusCode)
}
