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
	"github.com/moonlight-box/registry/internal/adapter"
	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/handler"
	"github.com/moonlight-box/registry/internal/middleware"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	yumTestServer *httptest.Server
	yumTestDB     *gorm.DB
	yumAdapter    adapter.RepoAwareAdapter
	yumRepoSvc    *service.RepositoryService
	yumPkgRepo    *repository.PackageRepository
	yumStorageSvc *service.StorageService
)

func setupYumTestEnv() {
	gin.SetMode(gin.TestMode)

	var err error
	yumTestDB, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	database.DB = yumTestDB

	yumTestDB.AutoMigrate(
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

	localStorage, _ := storage.NewLocalStorage("/tmp/test-yum-e2e", 1024)
	storageBackendRepo := repository.NewStorageBackendRepository(yumTestDB)
	yumStorageSvc, _ = service.NewStorageService(storageBackendRepo, "", 0)
	yumStorageSvc.SetDefaultBackendForTest(localStorage)

	yumPkgRepo = repository.NewPackageRepository(yumTestDB)
	repoRepo := repository.NewRepositoryRepository(yumTestDB)
	groupRepo := repository.NewGroupRepository(yumTestDB)

	auditSvc := service.NewAuditService()
	yumRepoSvc = service.NewRepositoryService(repoRepo, groupRepo, yumTestDB)

	cacheSvc := proxy.NewCacheService()
	dnsResolver := proxy.NewDNSResolver(nil)
	tm := proxy.NewTransportManager(30*time.Second, dnsResolver)
	remoteClient := proxy.NewRemoteClient(tm, 5)
	proxyRouter := proxy.NewProxyRouter(yumTestDB, cacheSvc, remoteClient, repoRepo, groupRepo, nil)

	yumAdapter = adapter.NewYumAdapter(yumPkgRepo, repoRepo, yumStorageSvc, auditSvc, proxyRouter, nil)

	router := setupYumRouter()
	yumTestServer = httptest.NewServer(router)
}

func teardownYumTestEnv() {
	if yumTestServer != nil {
		yumTestServer.Close()
	}
}

func setupYumRouter() *gin.Engine {
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

	repoRouter := handler.NewRepoRouter(yumRepoSvc, nil)
	repoRouter.RegisterAdapter("yum", yumAdapter)

	repoGroup := router.Group("/repo/:repoName")
	{
		repoGroup.GET("/*path", repoRouter.HandleRequest)
		repoGroup.POST("/*path", authMw, permMw("yum", "write"), repoRouter.HandlePublish)
		repoGroup.DELETE("/*path", authMw, permMw("yum", "delete"), repoRouter.HandleDelete)
	}

	repoHandler := handler.NewRepositoryHandler(yumRepoSvc, nil, nil)
	repoAPI := router.Group("/api/repositories")
	{
		repoAPI.GET("", repoHandler.List)
		repoAPI.GET("/:name", repoHandler.Get)
		repoAPI.POST("", repoHandler.Create)
		repoAPI.PUT("/:name", repoHandler.Update)
		repoAPI.DELETE("/:name", repoHandler.Delete)
	}

	return router
}

func TestE2E_Yum_UploadRPM(t *testing.T) {
	setupYumTestEnv()
	defer teardownYumTestEnv()

	repo := &model.Repository{
		Name:        "yum-local-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeYum),
		Enabled:     true,
	}
	err := yumRepoSvc.Create(repo, nil)
	assert.Nil(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, _ := writer.CreateFormFile("file", "nginx-1.20.1-1.el9.x86_64.rpm")
	part.Write([]byte("fake rpm content for e2e test"))
	writer.Close()

	req, _ := http.NewRequest("POST", yumTestServer.URL+"/repo/yum-local-e2e/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 201)
}

func TestE2E_Yum_DownloadRPM(t *testing.T) {
	setupYumTestEnv()
	defer teardownYumTestEnv()

	yumPkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:        "nginx",
		Type:        model.PackageTypeYum,
		Description: "Nginx web server",
	}, &model.PackageVersion{
		Version:     "1.20.1",
		Status:      model.StatusPublished,
		StoragePath: "yum/nginx/1.20.1",
	}, &model.PackageFile{
		Filename:    "nginx-1.20.1-1.el9.x86_64.rpm",
		FileType:    model.FileTypePrimary,
		StoragePath: "repos/yum-download-e2e/Packages/x86_64/nginx-1.20.1-1.el9.x86_64.rpm",
		SizeBytes:   1000,
	})

	repo := &model.Repository{
		Name:        "yum-download-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeYum),
		Enabled:     true,
	}
	yumRepoSvc.Create(repo, nil)

	resp, err := http.Get(yumTestServer.URL + "/repo/yum-download-e2e/Packages/x86_64/nginx-1.20.1-1.el9.x86_64.rpm")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	content, _ := io.ReadAll(resp.Body)
	assert.NotEmpty(t, content)
}

func TestE2E_Yum_RepositoryManagement(t *testing.T) {
	setupYumTestEnv()
	defer teardownYumTestEnv()

	payload := map[string]interface{}{
		"name":         "yum-local-create",
		"display_name": "YUM Local Repository",
		"description":  "Local YUM repository for e2e testing",
		"type":         "local",
		"package_type": "yum",
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		yumTestServer.URL+"/api/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "yum-local-create", result["name"])

	getResp, err := http.Get(yumTestServer.URL + "/api/repositories/yum-local-create")
	assert.Nil(t, err)
	assert.Equal(t, 200, getResp.StatusCode)

	listResp, err := http.Get(yumTestServer.URL + "/api/repositories")
	assert.Nil(t, err)
	assert.Equal(t, 200, listResp.StatusCode)

	req, _ := http.NewRequest(
		"DELETE",
		yumTestServer.URL+"/api/repositories/yum-local-create",
		nil,
	)
	client := &http.Client{}
	deleteResp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, deleteResp.StatusCode)

	getResp2, err := http.Get(yumTestServer.URL + "/api/repositories/yum-local-create")
	assert.Nil(t, err)
	assert.Equal(t, 404, getResp2.StatusCode)
}

func TestE2E_Yum_ProxyRepository(t *testing.T) {
	setupYumTestEnv()
	defer teardownYumTestEnv()

	payload := map[string]interface{}{
		"name":         "yum-proxy-e2e",
		"display_name": "YUM Proxy Repository",
		"description":  "Proxy YUM repository for e2e testing",
		"type":         "proxy",
		"package_type": "yum",
		"remote_url":   "http://mirror.centos.org/centos/9-stream/BaseOS/x86_64/os/",
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		yumTestServer.URL+"/api/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "http://mirror.centos.org/centos/9-stream/BaseOS/x86_64/os/", result["remote_url"])
}

func TestE2E_Yum_CompleteWorkflow(t *testing.T) {
	setupYumTestEnv()
	defer teardownYumTestEnv()

	repo := &model.Repository{
		Name:        "yum-workflow-complete",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeYum),
		Enabled:     true,
	}
	err := yumRepoSvc.Create(repo, nil)
	assert.Nil(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, _ := writer.CreateFormFile("file", "httpd-2.4.52-1.el9.x86_64.rpm")
	part.Write([]byte("fake httpd rpm content for workflow test"))
	writer.Close()

	req, _ := http.NewRequest("POST", yumTestServer.URL+"/repo/yum-workflow-complete/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 201)

	listResp, err := http.Get(yumTestServer.URL + "/api/repositories")
	assert.Nil(t, err)
	assert.Equal(t, 200, listResp.StatusCode)
}
