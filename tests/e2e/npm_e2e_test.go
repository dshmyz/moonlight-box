package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/adapter"
	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/handler"
	"github.com/moonlight-box/registry/internal/middleware"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
	"github.com/moonlight-box/registry/internal/types"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/gormlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

var (
	testServer     *httptest.Server
	testDB         *gorm.DB
	npmAdapter     adapter.RepoAwareAdapter
	repoSvc        *service.RepositoryService
	pkgRepo        *repository.PackageRepository
	storageSvc     *service.StorageService
	npmRepoHandler *proxy.RepoHandler
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	var err error
	testDB, err = gorm.Open(gormlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	database.DB = testDB

	testDB.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.RolePermission{},
		&model.UserRole{},
		&model.Repository{},
		&model.RepositoryGroup{},
		&model.Package{},
		&model.PackageVersion{},
		&model.PackageFile{},
		&model.PackageDependency{},
		&model.AuditLog{},
		&model.BlockRule{},
		&model.ScanResult{},
		&model.Vulnerability{},
		&model.StorageBackend{},
	)

	localStorage, _ := storage.NewLocalStorage("/tmp/test-e2e", 1024)
	storageBackendRepo := repository.NewStorageBackendRepository(testDB)
	storageSvc, _ = service.NewStorageService(storageBackendRepo, "", 0)
	storageSvc.SetDefaultBackendForTest(localStorage)

	pkgRepo = repository.NewPackageRepository(testDB)
	repoRepo := repository.NewRepositoryRepository(testDB)
	groupRepo := repository.NewGroupRepository(testDB)

	repoSvc = service.NewRepositoryService(repoRepo, groupRepo, testDB)

	cacheSvc := proxy.NewCacheService()
	dnsResolver := proxy.NewDNSResolver(nil)
	tm := proxy.NewTransportManager(30*time.Second, dnsResolver)
	remoteClient := proxy.NewRemoteClient(tm, 5)
	proxyDownloader := proxy.NewProxyDownloader(cacheSvc, remoteClient, nil)

	repoCache := proxy.NewRepositoryCache(repoRepo, groupRepo, 5*time.Minute)
	npmRepoHandler = proxy.NewRepoHandler(repoRepo, groupRepo, repoCache)

	auditSvc := service.NewAuditService()
	npmAdapter = adapter.NewNpmAdapter(pkgRepo, repoRepo, storageSvc, auditSvc, nil)
	npmRepoHandler.RegisterAdapter("npm", npmAdapter)

	logRepo := repository.NewProxyDownloadLogRepository(testDB)
	countBatcher := service.NewDownloadCountBatcher(testDB, 5*time.Second)
	adapters := map[string]types.Adapter{"npm": npmAdapter}
	downloadSvc := service.NewDownloadService(repoRepo, groupRepo, adapters, pkgRepo, storageSvc, proxyDownloader, logRepo, nil, countBatcher)
	npmRepoHandler.SetDownloadService(downloadSvc)

	router := setupRouter()
	testServer = httptest.NewServer(router)

	code := m.Run()

	testServer.Close()
	os.Exit(code)
}

func setupRouter() *gin.Engine {
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

	repoRouter := handler.NewRepoRouter(repoSvc)
	repoRouter.SetResolver(npmRepoHandler)

	repoGroup := router.Group("/repo/:repoName")
	{
		repoGroup.GET("/*path", repoRouter.HandleRequest)
		repoGroup.PUT("/*path", authMw, permMw("npm", "write"), repoRouter.HandlePublish)
		repoGroup.DELETE("/*path", authMw, permMw("npm", "delete"), repoRouter.HandleDelete)
	}

	repoHandler := handler.NewRepositoryHandler(repoSvc)
	repoAPI := router.Group("/api/repositories")
	{
		repoAPI.GET("", repoHandler.List)
		repoAPI.GET("/:name", repoHandler.Get)
		repoAPI.POST("", repoHandler.Create)
		repoAPI.PUT("/:name", repoHandler.Update)
		repoAPI.DELETE("/:name", repoHandler.Delete)
		repoAPI.GET("/:name/members", repoHandler.GetMembers)
		repoAPI.POST("/:name/members", repoHandler.AddMember)
		repoAPI.DELETE("/:name/members/:memberName", repoHandler.RemoveMember)
	}

	return router
}

// ==================== E2E 测试：npm 本地仓库 ====================

func TestE2E_PublishNpmPackage(t *testing.T) {
	repo := &model.Repository{
		Name:        "npm-local-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}
	err := repoSvc.Create(repo, nil)
	assert.Nil(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, _ := writer.CreateFormFile("_attachments", "test-pkg-1.0.0.tgz")
	part.Write([]byte("fake tarball content for e2e test"))

	writer.WriteField("_attachment", `{
		"name": "test-pkg",
		"version": "1.0.0",
		"description": "E2E test package"
	}`)
	writer.Close()

	req, _ := http.NewRequest("PUT", testServer.URL+"/repo/npm-local-e2e/test-pkg/-rev/123", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, true, result["ok"])
}

func TestE2E_GetNpmPackage(t *testing.T) {
	pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:        "e2e-pkg",
		Type:        model.PackageTypeNPM,
		Description: "E2E package for testing",
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: "npm/e2e-pkg/1.0.0",
	}, &model.PackageFile{
		Filename:    "package.tgz",
		FileType:    model.FileTypePrimary,
		StoragePath: "npm/e2e-pkg/1.0.0/package.tgz",
		SizeBytes:   1000,
	})

	repo := &model.Repository{
		Name:        "npm-local-get",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}
	repoSvc.Create(repo, nil)

	resp, err := http.Get(testServer.URL + "/repo/npm-local-get/e2e-pkg")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var meta map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&meta)
	assert.Equal(t, "e2e-pkg", meta["name"])
}

func TestE2E_GetNpmPackage_NotFound(t *testing.T) {
	repo := &model.Repository{
		Name:        "npm-local-notfound",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}
	repoSvc.Create(repo, nil)

	resp, err := http.Get(testServer.URL + "/repo/npm-local-notfound/nonexistent-pkg")
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestE2E_UnpublishNpmPackage(t *testing.T) {
	pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:        "unpublish-test",
		Type:        model.PackageTypeNPM,
		Description: "Unpublish test package",
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: "npm/unpublish-test/1.0.0",
	}, &model.PackageFile{
		Filename:    "package.tgz",
		FileType:    model.FileTypePrimary,
		StoragePath: "npm/unpublish-test/1.0.0/package.tgz",
		SizeBytes:   1000,
	})

	repo := &model.Repository{
		Name:        "npm-local-unpublish",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
		AllowDelete: true,
	}
	repoSvc.Create(repo, nil)

	req, _ := http.NewRequest("DELETE", testServer.URL+"/repo/npm-local-unpublish/unpublish-test/-rev/123", nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, true, result["ok"])
}

// ==================== E2E 测试：仓库管理 ====================

func TestE2E_CreateLocalRepository(t *testing.T) {
	payload := map[string]interface{}{
		"name":         "npm-local-create",
		"display_name": "NPM Local Repository",
		"description":  "Local npm repository for e2e testing",
		"type":         "local",
		"package_type": "npm",
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		testServer.URL+"/api/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "npm-local-create", result["name"])
}

func TestE2E_CreateProxyRepository(t *testing.T) {
	payload := map[string]interface{}{
		"name":         "npm-proxy-e2e",
		"display_name": "NPM Proxy Repository",
		"description":  "Proxy npm repository for e2e testing",
		"type":         "proxy",
		"package_type": "npm",
		"remote_url":   "https://registry.npmjs.org",
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		testServer.URL+"/api/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "https://registry.npmjs.org", result["remote_url"])
}

func TestE2E_GetRepository(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "get-repo-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}, nil)

	resp, err := http.Get(testServer.URL + "/api/repositories/get-repo-e2e")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "get-repo-e2e", result["name"])
}

func TestE2E_ListRepositories(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/api/repositories")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.NotNil(t, result["list"])
}

func TestE2E_DeleteRepository(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "delete-repo-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}, nil)

	req, _ := http.NewRequest(
		"DELETE",
		testServer.URL+"/api/repositories/delete-repo-e2e",
		nil,
	)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	resp, _ = http.Get(testServer.URL + "/api/repositories/delete-repo-e2e")
	assert.Equal(t, 404, resp.StatusCode)
}

// ==================== E2E 测试：虚拟仓库成员管理 ====================

func TestE2E_AddVirtualMember(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "npm-local-member",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}, nil)

	repoSvc.Create(&model.Repository{
		Name:        "npm-virtual-members",
		Type:        model.RepoTypeVirtual,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}, nil)

	payload := map[string]interface{}{
		"member_name": "npm-local-member",
		"priority":    0,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		testServer.URL+"/api/repositories/npm-virtual-members/members",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestE2E_GetVirtualMembers(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "npm-local-get-members",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}, nil)

	repoSvc.Create(&model.Repository{
		Name:        "npm-virtual-get-members",
		Type:        model.RepoTypeVirtual,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}, nil)

	repoSvc.AddMember("npm-virtual-get-members", "npm-local-get-members", 0)

	resp, err := http.Get(testServer.URL + "/api/repositories/npm-virtual-get-members/members")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	data := result["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 1)
}

func TestE2E_RemoveVirtualMember(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "npm-local-remove",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}, nil)

	repoSvc.Create(&model.Repository{
		Name:        "npm-virtual-remove",
		Type:        model.RepoTypeVirtual,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}, nil)

	repoSvc.AddMember("npm-virtual-remove", "npm-local-remove", 0)

	req, _ := http.NewRequest(
		"DELETE",
		testServer.URL+"/api/repositories/npm-virtual-remove/members/npm-local-remove",
		nil,
	)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// ==================== E2E 测试：公开仓库配置 API ====================

func TestE2E_PublicRepoConfig(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "npm-public-config",
		DisplayName: "NPM Public Config Test",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}, nil)

	publicHandler := handler.NewPublicRepoHandler(repoSvc)
	testRouter := gin.New()
	testRouter.GET("/api/v1/public/repo/:name", publicHandler.GetRepoConfig)
	server := httptest.NewServer(testRouter)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/v1/public/repo/npm-public-config")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(200), result["code"])

	data := result["data"].(map[string]interface{})
	assert.Equal(t, "npm-public-config", data["name"])
	assert.Equal(t, "NPM Public Config Test", data["display_name"])
	assert.Contains(t, data["registry_url"], "/repo/npm-public-config/")
}

// ==================== E2E 测试：新路由架构 ====================

func TestE2E_RepoRoute_GetPackage(t *testing.T) {
	pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:        "route-test-pkg",
		Type:        model.PackageTypeNPM,
		Description: "Route test package",
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: "npm/route-test-pkg/1.0.0",
	}, &model.PackageFile{
		Filename:    "package.tgz",
		FileType:    model.FileTypePrimary,
		StoragePath: "npm/route-test-pkg/1.0.0/package.tgz",
		SizeBytes:   1000,
	})

	repo := &model.Repository{
		Name:        "npm-route-test",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}
	repoSvc.Create(repo, nil)

	resp, err := http.Get(testServer.URL + "/repo/npm-route-test/route-test-pkg")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestE2E_RepoRoute_DisabledRepo(t *testing.T) {
	repo := &model.Repository{
		Name:        "npm-disabled",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     false,
	}
	repoSvc.Create(repo, nil)

	resp, err := http.Get(testServer.URL + "/repo/npm-disabled/some-pkg")
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestE2E_RepoRoute_NonExistentRepo(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/repo/nonexistent-repo/some-pkg")
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestE2E_RepoRoute_ProxyRepoPublish(t *testing.T) {
	repo := &model.Repository{
		Name:        "npm-proxy-no-publish",
		Type:        model.RepoTypeProxy,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
		RemoteURL:   "https://registry.npmjs.org",
	}
	repoSvc.Create(repo, nil)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req, _ := http.NewRequest("PUT", testServer.URL+"/repo/npm-proxy-no-publish/test-pkg/-rev/123", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

// ==================== 辅助函数 ====================

func readBody(t *testing.T, resp *http.Response) []byte {
	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)
	return body
}

func init() {
	gin.SetMode(gin.TestMode)
}

func TestE2E_CompleteNpmWorkflow(t *testing.T) {
	repo := &model.Repository{
		Name:        "npm-workflow-complete",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeNPM),
		Enabled:     true,
	}
	err := repoSvc.Create(repo, nil)
	assert.Nil(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, _ := writer.CreateFormFile("_attachments", "workflow-pkg-1.0.0.tgz")
	part.Write([]byte("fake tarball content"))

	writer.WriteField("_attachment", fmt.Sprintf(`{
		"name": "workflow-pkg-%d",
		"version": "1.0.0",
		"description": "Workflow test package"
	}`, os.Getpid()))
	writer.Close()

	req, _ := http.NewRequest("PUT", testServer.URL+"/repo/npm-workflow-complete/workflow-pkg/-rev/123", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	req2, _ := http.NewRequest(
		"DELETE",
		testServer.URL+"/repo/npm-workflow-complete/workflow-pkg/-rev/123",
		nil,
	)
	resp, err = client.Do(req2)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
