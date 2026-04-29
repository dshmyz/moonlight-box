package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/adapter"
	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/handler"
	"github.com/moonlight-box/registry/internal/middleware"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/storage"
	"github.com/stretchr/testify/assert"
)

var (
	testServer *httptest.Server
	testDB     *database.Database
	npmAdapter *adapter.NpmAdapter
	repoSvc    *service.RepositoryService
	pkgRepo    *repository.PackageRepository
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	// 初始化测试数据库
	testDB = database.New(&database.Config{
		Driver: "sqlite",
		Path:   ":memory:",
	})

	// 初始化存储
	localStorage, _ := storage.NewLocalStorage("/tmp/test-e2e")

	// 初始化仓库层
	pkgRepo = repository.NewPackageRepository(testDB.DB)
	repoRepo := repository.NewRepositoryRepository(testDB.DB)
	groupRepo := repository.NewGroupRepository(testDB.DB)

	// 初始化服务层
	storageSvc := service.NewStorageService(localStorage, pkgRepo)
	auditSvc := service.NewAuditService(testDB.DB)
	repoSvc = service.NewRepositoryService(repoRepo, groupRepo, testDB.DB)

	// 初始化适配器
	npmAdapter = adapter.NewNpmAdapter(pkgRepo, storageSvc, auditSvc)

	// 设置路由
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

	// NPM 路由
	npmGroup := router.Group("/npm")
	npmAdapter.RegisterRoutes(npmGroup, func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	}, func(resource, action string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Next()
		}
	})

	// 仓库管理路由
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
	// 1. 创建 npm 本地仓库
	repo := &model.Repository{
		Name:        "npm-local-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}
	err := repoSvc.Create(repo, nil)
	assert.Nil(t, err)

	// 2. 发布包
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

	resp, err := http.Post(
		testServer.URL+"/npm/test-pkg/-rev/123",
		writer.FormDataContentType(),
		body,
	)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, true, result["ok"])
}

func TestE2E_GetNpmPackage(t *testing.T) {
	// 先发布一个包
	pkgRepo.CreateOrUpdate(testDB.DB.Context(), &model.Package{
		Name:        "e2e-pkg",
		Type:        model.PackageTypeNPM,
		Description: "E2E package for testing",
	}, &model.PackageVersion{
		Version:   "1.0.0",
		Status:    model.StatusPublished,
		SizeBytes: 1000,
	})

	// 获取包元数据
	resp, err := http.Get(testServer.URL + "/npm/e2e-pkg")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var meta map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&meta)
	assert.Equal(t, "e2e-pkg", meta["name"])
}

func TestE2E_GetNpmPackage_NotFound(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/npm/nonexistent-pkg")
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestE2E_GetNpmVersion(t *testing.T) {
	// 先发布一个包
	pkgRepo.CreateOrUpdate(testDB.DB.Context(), &model.Package{
		Name:        "version-test-pkg",
		Type:        model.PackageTypeNPM,
		Description: "Version test package",
	}, &model.PackageVersion{
		Version:   "2.0.0",
		Status:    model.StatusPublished,
		SizeBytes: 2000,
	})

	resp, err := http.Get(testServer.URL + "/npm/version-test-pkg/2.0.0")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var version map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&version)
	assert.Equal(t, "2.0.0", version["version"])
}

func TestE2E_GetNpmVersion_NotFound(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/npm/nonexistent-pkg/1.0.0")
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestE2E_UnpublishNpmPackage(t *testing.T) {
	// 先发布一个包
	pkgRepo.CreateOrUpdate(testDB.DB.Context(), &model.Package{
		Name:        "unpublish-test",
		Type:        model.PackageTypeNPM,
		Description: "Unpublish test package",
	}, &model.PackageVersion{
		Version:   "1.0.0",
		Status:    model.StatusPublished,
		SizeBytes: 1000,
	})

	// 取消发布
	req, _ := http.NewRequest("DELETE", testServer.URL+"/npm/unpublish-test/-rev/123", nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, true, result["ok"])
}

func TestE2E_DownloadTarball_NotFound(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/npm/-/tarball/nonexistent-1.0.0.tgz")
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// ==================== E2E 测试：仓库管理 ====================

func TestE2E_CreateLocalRepository(t *testing.T) {
	payload := map[string]interface{}{
		"name":         "npm-local-e2e",
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
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "npm-local-e2e", result["name"])
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
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "https://registry.npmjs.org", result["remote_url"])
}

func TestE2E_CreateVirtualRepository(t *testing.T) {
	// 先创建成员仓库
	repoSvc.Create(&model.Repository{
		Name:        "npm-local-virtual-member",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}, nil)

	payload := map[string]interface{}{
		"name":         "npm-virtual-e2e",
		"display_name": "NPM Virtual Repository",
		"description":  "Virtual npm repository for e2e testing",
		"type":         "virtual",
		"package_type": "npm",
		"members":      []string{"npm-local-virtual-member"},
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		testServer.URL+"/api/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "npm-virtual-e2e", result["name"])
}

func TestE2E_GetRepository(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "get-repo-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
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

func TestE2E_UpdateRepository(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "update-repo-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}, nil)

	payload := map[string]interface{}{
		"display_name": "Updated Name",
		"description":  "Updated description",
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(
		"PUT",
		testServer.URL+"/api/repositories/update-repo-e2e",
		bytes.NewBuffer(body),
	)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestE2E_DeleteRepository(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "delete-repo-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
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

	// 验证已删除
	resp, _ = http.Get(testServer.URL + "/api/repositories/delete-repo-e2e")
	assert.Equal(t, 404, resp.StatusCode)
}

// ==================== E2E 测试：虚拟仓库成员管理 ====================

func TestE2E_AddVirtualMember(t *testing.T) {
	// 创建本地仓库和虚拟仓库
	repoSvc.Create(&model.Repository{
		Name:        "npm-local-member",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}, nil)

	repoSvc.Create(&model.Repository{
		Name:        "npm-virtual-members",
		Type:        model.RepoTypeVirtual,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}, nil)

	// 添加成员
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
	// 创建本地仓库和虚拟仓库
	repoSvc.Create(&model.Repository{
		Name:        "npm-local-get-members",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}, nil)

	repoSvc.Create(&model.Repository{
		Name:        "npm-virtual-get-members",
		Type:        model.RepoTypeVirtual,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}, nil)

	repoSvc.AddMember("npm-virtual-get-members", "npm-local-get-members", 0)

	resp, err := http.Get(testServer.URL + "/api/repositories/npm-virtual-get-members/members")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.GreaterOrEqual(t, len(result), 1)
}

func TestE2E_RemoveVirtualMember(t *testing.T) {
	// 创建本地仓库和虚拟仓库
	repoSvc.Create(&model.Repository{
		Name:        "npm-local-remove",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}, nil)

	repoSvc.Create(&model.Repository{
		Name:        "npm-virtual-remove",
		Type:        model.RepoTypeVirtual,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}, nil)

	repoSvc.AddMember("npm-virtual-remove", "npm-local-remove", 0)

	// 移除成员
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

// ==================== E2E 测试：npm 包操作完整流程 ====================

func TestE2E_CompleteNpmWorkflow(t *testing.T) {
	// 1. 创建 npm 本地仓库
	repo := &model.Repository{
		Name:        "npm-workflow-local",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
		Enabled:     true,
	}
	err := repoSvc.Create(repo, nil)
	assert.Nil(t, err)

	// 2. 发布包
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, _ := writer.CreateFormFile("_attachments", "workflow-pkg-1.0.0.tgz")
	part.Write([]byte("fake tarball content"))

	writer.WriteField("_attachment", `{
		"name": "workflow-pkg",
		"version": "1.0.0",
		"description": "Workflow test package"
	}`)
	writer.Close()

	resp, err := http.Post(
		testServer.URL+"/npm/workflow-pkg/-rev/123",
		writer.FormDataContentType(),
		body,
	)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	// 3. 获取包元数据
	resp, err = http.Get(testServer.URL + "/npm/workflow-pkg")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 4. 获取特定版本
	resp, err = http.Get(testServer.URL + "/npm/workflow-pkg/1.0.0")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 5. 取消发布
	req, _ := http.NewRequest(
		"DELETE",
		testServer.URL+"/npm/workflow-pkg/-rev/123",
		nil,
	)
	client := &http.Client{}
	resp, err = client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 6. 验证包已删除
	resp, err = http.Get(testServer.URL + "/npm/workflow-pkg")
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// ==================== E2E 测试：认证配置 ====================

func TestE2E_RepositoryWithBasicAuth(t *testing.T) {
	payload := map[string]interface{}{
		"name":         "npm-proxy-basicauth",
		"type":         "proxy",
		"package_type": "npm",
		"remote_url":   "https://private.registry.com",
		"auth_type":    "basic",
		"auth_config":  `{"username":"admin","password":"secret"}`,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		testServer.URL+"/api/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestE2E_RepositoryWithBearerToken(t *testing.T) {
	payload := map[string]interface{}{
		"name":         "npm-proxy-bearer",
		"type":         "proxy",
		"package_type": "npm",
		"remote_url":   "https://token.registry.com",
		"auth_type":    "bearer",
		"auth_config":  `{"token":"my-secret-token"}`,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		testServer.URL+"/api/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestE2E_RepositoryWithApiKey(t *testing.T) {
	payload := map[string]interface{}{
		"name":         "npm-proxy-apikey",
		"type":         "proxy",
		"package_type": "npm",
		"remote_url":   "https://apikey.registry.com",
		"auth_type":    "api_key",
		"auth_config":  `{"header_name":"X-API-Key","key_value":"secret-key"}`,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		testServer.URL+"/api/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// ==================== E2E 测试：缓存配置 ====================

func TestE2E_RepositoryWithCacheConfig(t *testing.T) {
	payload := map[string]interface{}{
		"name":              "npm-proxy-cache",
		"type":              "proxy",
		"package_type":      "npm",
		"remote_url":        "https://registry.npmjs.org",
		"cache_enabled":     true,
		"cache_ttl_seconds": 3600,
		"cache_max_size_gb": 5,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		testServer.URL+"/api/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(3600), result["cache_ttl_seconds"])
}

// ==================== E2E 测试：错误场景 ====================

func TestE2E_CreateRepositoryDuplicateName(t *testing.T) {
	repoSvc.Create(&model.Repository{
		Name:        "duplicate-name",
		Type:        model.RepoTypeLocal,
		PackageType: model.PackageTypeNPM,
	}, nil)

	payload := map[string]interface{}{
		"name":         "duplicate-name",
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
	// 应返回 400 或 500 错误
	assert.NotEqual(t, 200, resp.StatusCode)
}

func TestE2E_GetNonExistentRepository(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/api/repositories/nonexistent")
	assert.Nil(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestE2E_PublishInvalidPackage(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	resp, err := http.Post(
		testServer.URL+"/npm/invalid/-rev/123",
		writer.FormDataContentType(),
		body,
	)
	assert.Nil(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestE2E_RepositoryValidation(t *testing.T) {
	// 测试空名称
	payload := map[string]interface{}{
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
	assert.NotEqual(t, 200, resp.StatusCode)
}

// ==================== 辅助函数 ====================

func readBody(t *testing.T, resp *http.Response) []byte {
	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)
	return body
}
