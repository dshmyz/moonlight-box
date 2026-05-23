package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
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
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

var (
	mavenTestServer  *httptest.Server
	mavenTestDB      *gorm.DB
	mavenAdapter     adapter.RepoAwareAdapter
	mavenRepoSvc     *service.RepositoryService
	mavenPkgRepo     *repository.ComponentRepository
	mavenStorageSvc  *service.StorageService
	mavenRepoHandler *proxy.RepoHandler
)

func setupMavenTestEnv() {
	gin.SetMode(gin.TestMode)

	var err error
	mavenTestDB, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	database.DB = mavenTestDB

	mavenTestDB.AutoMigrate(
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

	localStorage, _ := storage.NewLocalStorage("/tmp/test-maven-e2e", 1024)
	storageBackendRepo := repository.NewStorageBackendRepository(mavenTestDB)
	mavenStorageSvc, _ = service.NewStorageService(storageBackendRepo, "", 0)
	mavenStorageSvc.SetDefaultBackendForTest(localStorage)

	mavenPkgRepo = repository.NewComponentRepository(mavenTestDB)
	repoRepo := repository.NewRepositoryRepository(mavenTestDB)
	groupRepo := repository.NewGroupRepository(mavenTestDB)

	mavenRepoSvc = service.NewRepositoryService(repoRepo, groupRepo, mavenTestDB)

	cacheSvc := proxy.NewCacheService()
	dnsResolver := proxy.NewDNSResolver(nil)
	tm := proxy.NewTransportManager(30*time.Second, dnsResolver)
	remoteClient := proxy.NewRemoteClient(tm, 5)
	proxyDownloader := proxy.NewProxyDownloader(cacheSvc, remoteClient, nil)

	repoCache := proxy.NewRepositoryCache(repoRepo, groupRepo, 5*time.Minute)
	mavenRepoHandler = proxy.NewRepoHandler(repoRepo, groupRepo, repoCache)

	mavenAdapter = adapter.NewMavenAdapter(mavenPkgRepo, repoRepo, mavenStorageSvc, nil, nil)
	mavenRepoHandler.RegisterAdapter("maven", mavenAdapter)

	logRepo := repository.NewProxyDownloadLogRepository(mavenTestDB)
	countBatcher := service.NewDownloadCountBatcher(mavenTestDB, 5*time.Second)
	downloadSvc := service.NewDownloadService(mavenPkgRepo, mavenStorageSvc, proxyDownloader, logRepo, nil, countBatcher)
	mavenRepoHandler.SetDownloadService(downloadSvc)

	router := setupMavenRouter()
	mavenTestServer = httptest.NewServer(router)
}

func teardownMavenTestEnv() {
	if mavenTestServer != nil {
		mavenTestServer.Close()
	}
}

func setupMavenRouter() *gin.Engine {
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

	repoRouter := handler.NewRepoRouter(mavenRepoSvc)
	repoRouter.SetResolver(mavenRepoHandler)
	repoRouter.SetUploadService(service.NewUploadService(mavenPkgRepo, mavenStorageSvc))

	repoGroup := router.Group("/repo/:repoName")
	{
		repoGroup.GET("/*path", repoRouter.HandleRequest)
		repoGroup.PUT("/*path", authMw, permMw("maven", "write"), repoRouter.HandlePublish)
		repoGroup.DELETE("/*path", authMw, permMw("maven", "delete"), repoRouter.HandleDelete)
	}

	repoHandler := handler.NewRepositoryHandler(mavenRepoSvc)
	repoAPI := router.Group("/api/v1/repositories")
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

func TestE2E_Maven_PublishReleaseVersion(t *testing.T) {
	setupMavenTestEnv()
	defer teardownMavenTestEnv()

	repo := &model.Repository{
		Name:        "maven-releases-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeMaven),
		Enabled:     true,
	}
	err := mavenRepoSvc.Create(repo, nil)
	assert.Nil(t, err)

	jarContent := []byte("fake jar content for e2e test")
	pomContent := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.test</groupId>
  <artifactId>my-lib</artifactId>
  <version>1.0.0</version>
  <packaging>jar</packaging>
  <description>Test library for E2E</description>
</project>`)

	jarReq, _ := http.NewRequest(
		"PUT",
		mavenTestServer.URL+"/repo/maven-releases-e2e/com/test/my-lib/1.0.0/my-lib-1.0.0.jar",
		bytes.NewReader(jarContent),
	)
	jarReq.ContentLength = int64(len(jarContent))
	client := &http.Client{}
	jarResp, err := client.Do(jarReq)
	assert.Nil(t, err)
	assert.Equal(t, 200, jarResp.StatusCode)

	pomReq, _ := http.NewRequest(
		"PUT",
		mavenTestServer.URL+"/repo/maven-releases-e2e/com/test/my-lib/1.0.0/my-lib-1.0.0.pom",
		bytes.NewReader(pomContent),
	)
	pomReq.ContentLength = int64(len(pomContent))
	pomResp, err := client.Do(pomReq)
	assert.Nil(t, err)
	assert.Equal(t, 200, pomResp.StatusCode)

	metadataResp, err := http.Get(mavenTestServer.URL + "/repo/maven-releases-e2e/com/test/my-lib/maven-metadata.xml")
	assert.Nil(t, err)
	assert.Equal(t, 200, metadataResp.StatusCode)

	body, _ := io.ReadAll(metadataResp.Body)
	var metadata adapter.MavenMetadata
	err = xml.Unmarshal(body, &metadata)
	assert.Nil(t, err)
	assert.Equal(t, "com.test", metadata.GroupID)
	assert.Equal(t, "my-lib", metadata.ArtifactID)
	assert.Equal(t, "1.0.0", metadata.Versioning.Release)
	assert.Contains(t, metadata.Versioning.Versions.Version, "1.0.0")
}

func TestE2E_Maven_PublishSnapshotVersion(t *testing.T) {
	setupMavenTestEnv()
	defer teardownMavenTestEnv()

	repo := &model.Repository{
		Name:        "maven-snapshots-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeMaven),
		Enabled:     true,
	}
	err := mavenRepoSvc.Create(repo, nil)
	assert.Nil(t, err)

	jarContent := []byte("fake jar content for snapshot test")
	pomContent := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.test</groupId>
  <artifactId>snapshot-lib</artifactId>
  <version>1.0-SNAPSHOT</version>
  <packaging>jar</packaging>
</project>`)

	jarReq, _ := http.NewRequest(
		"PUT",
		mavenTestServer.URL+"/repo/maven-snapshots-e2e/com/test/snapshot-lib/1.0-SNAPSHOT/snapshot-lib-1.0-SNAPSHOT.jar",
		bytes.NewReader(jarContent),
	)
	jarReq.ContentLength = int64(len(jarContent))
	client := &http.Client{}
	jarResp, err := client.Do(jarReq)
	assert.Nil(t, err)
	assert.Equal(t, 200, jarResp.StatusCode)

	pomReq, _ := http.NewRequest(
		"PUT",
		mavenTestServer.URL+"/repo/maven-snapshots-e2e/com/test/snapshot-lib/1.0-SNAPSHOT/snapshot-lib-1.0-SNAPSHOT.pom",
		bytes.NewReader(pomContent),
	)
	pomReq.ContentLength = int64(len(pomContent))
	pomResp, err := client.Do(pomReq)
	assert.Nil(t, err)
	assert.Equal(t, 200, pomResp.StatusCode)

	metadataResp, err := http.Get(mavenTestServer.URL + "/repo/maven-snapshots-e2e/com/test/snapshot-lib/maven-metadata.xml")
	assert.Nil(t, err)
	assert.Equal(t, 200, metadataResp.StatusCode)

	body, _ := io.ReadAll(metadataResp.Body)
	var metadata adapter.MavenMetadata
	err = xml.Unmarshal(body, &metadata)
	assert.Nil(t, err)
	assert.Equal(t, "com.test", metadata.GroupID)
	assert.Equal(t, "snapshot-lib", metadata.ArtifactID)
	assert.Contains(t, metadata.Versioning.Versions.Version, "1.0-SNAPSHOT")
}

func TestE2E_Maven_DownloadArtifact(t *testing.T) {
	setupMavenTestEnv()
	defer teardownMavenTestEnv()

	mavenPkgRepo.StoreComponentAsset(context.Background(), &model.Component{
		Name:        "com.test:download-lib",
		Format: model.PackageTypeMaven,
		Description: "Download test library",
	}, &model.Component{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
	}, &model.Asset{
		FileName:    "download-lib-1.0.0.jar",
		Kind:    model.AssetKindPrimary,
		Path: "com/test/download-lib/1.0.0/download-lib-1.0.0.jar",
		SizeBytes:   1000,
	})
	_, _ = mavenStorageSvc.StorePackageWithBackend(
		context.Background(),
		"maven-download-e2e",
		"maven",
		"com/test/download-lib",
		"1.0.0/download-lib-1.0.0.jar",
		bytes.NewReader([]byte("download test content")),
		int64(len([]byte("download test content"))),
		0,
	)

	repo := &model.Repository{
		Name:        "maven-download-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeMaven),
		Enabled:     true,
	}
	mavenRepoSvc.Create(repo, nil)

	resp, err := http.Get(mavenTestServer.URL + "/repo/maven-download-e2e/com/test/download-lib/1.0.0/download-lib-1.0.0.jar")
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	content, _ := io.ReadAll(resp.Body)
	assert.NotEmpty(t, content)
}

func TestE2E_Maven_ChecksumFiles(t *testing.T) {
	setupMavenTestEnv()
	defer teardownMavenTestEnv()

	jarContent := []byte("test jar for checksum")
	repo := &model.Repository{
		Name:        "maven-checksum-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeMaven),
		Enabled:     true,
	}
	mavenRepoSvc.Create(repo, nil)

	_, _ = mavenStorageSvc.StorePackageWithBackend(
		context.Background(),
		"maven-checksum-e2e",
		"maven",
		"com/test/checksum-lib",
		"1.0.0/checksum-lib-1.0.0.jar",
		bytes.NewReader(jarContent),
		int64(len(jarContent)),
		0,
	)

	mavenPkgRepo.StoreComponentAsset(context.Background(), &model.Component{
		Name:        "com.test:checksum-lib",
		Format: model.PackageTypeMaven,
		Description: "Checksum test library",
	}, &model.Component{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
	}, &model.Asset{
		FileName:    "checksum-lib-1.0.0.jar",
		Kind:    model.AssetKindPrimary,
		Path: "com/test/checksum-lib/1.0.0/checksum-lib-1.0.0.jar",
		SizeBytes:   int64(len(jarContent)),
	})

	sha1Resp, err := http.Get(mavenTestServer.URL + "/repo/maven-checksum-e2e/com/test/checksum-lib/1.0.0/checksum-lib-1.0.0.jar.sha1")
	assert.Nil(t, err)
	assert.Equal(t, 200, sha1Resp.StatusCode)

	sha1Content, _ := io.ReadAll(sha1Resp.Body)
	assert.NotEmpty(t, string(sha1Content))

	md5Resp, err := http.Get(mavenTestServer.URL + "/repo/maven-checksum-e2e/com/test/checksum-lib/1.0.0/checksum-lib-1.0.0.jar.md5")
	assert.Nil(t, err)
	assert.Equal(t, 200, md5Resp.StatusCode)

	md5Content, _ := io.ReadAll(md5Resp.Body)
	assert.NotEmpty(t, string(md5Content))
}

func TestE2E_Maven_DeleteArtifact(t *testing.T) {
	setupMavenTestEnv()
	defer teardownMavenTestEnv()

	mavenPkgRepo.StoreComponentAsset(context.Background(), &model.Component{
		Name:         "com.test:deletable-lib",
		Format: model.PackageTypeMaven,
		Description:  "Deletable test library",
		RepositoryID: 1,
	}, &model.Component{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
	}, &model.Asset{
		FileName:    "deletable-lib-1.0.0.jar",
		Kind:    model.AssetKindPrimary,
		Path: "com/test/deletable-lib/1.0.0/deletable-lib-1.0.0.jar",
		SizeBytes:   1000,
	})
	_, _ = mavenStorageSvc.StorePackageWithBackend(
		context.Background(),
		"maven-delete-e2e",
		"maven",
		"com/test/deletable-lib",
		"1.0.0/deletable-lib-1.0.0.jar",
		bytes.NewReader([]byte("deletable content")),
		int64(len([]byte("deletable content"))),
		0,
	)

	repo := &model.Repository{
		Name:        "maven-delete-e2e",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeMaven),
		Enabled:     true,
		AllowDelete: true,
	}
	mavenRepoSvc.Create(repo, nil)

	req, _ := http.NewRequest(
		"DELETE",
		mavenTestServer.URL+"/repo/maven-delete-e2e/com/test/deletable-lib/1.0.0/deletable-lib-1.0.0.jar",
		nil,
	)
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	getResp, err := http.Get(mavenTestServer.URL + "/repo/maven-delete-e2e/com/test/deletable-lib/1.0.0/deletable-lib-1.0.0.jar")
	assert.Nil(t, err)
	assert.Equal(t, 404, getResp.StatusCode)
}

func TestE2E_Maven_RepositoryManagement(t *testing.T) {
	setupMavenTestEnv()
	defer teardownMavenTestEnv()

	payload := map[string]interface{}{
		"name":         "maven-local-create",
		"display_name": "Maven Local Repository",
		"description":  "Local Maven repository for e2e testing",
		"type":         "local",
		"package_type": "maven",
	}

	body, _ := marshalJSON(payload)
	resp, err := http.Post(
		mavenTestServer.URL+"/api/v1/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	parseJSONResponse(resp, &result)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "maven-local-create", data["name"])

	getResp, err := http.Get(mavenTestServer.URL + "/api/v1/repositories/maven-local-create")
	assert.Nil(t, err)
	assert.Equal(t, 200, getResp.StatusCode)

	listResp, err := http.Get(mavenTestServer.URL + "/api/v1/repositories")
	assert.Nil(t, err)
	assert.Equal(t, 200, listResp.StatusCode)

	req, _ := http.NewRequest(
		"DELETE",
		mavenTestServer.URL+"/api/v1/repositories/maven-local-create",
		nil,
	)
	client := &http.Client{}
	deleteResp, err := client.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, deleteResp.StatusCode)

	getResp2, err := http.Get(mavenTestServer.URL + "/api/v1/repositories/maven-local-create")
	assert.Nil(t, err)
	assert.Equal(t, 404, getResp2.StatusCode)
}

func TestE2E_Maven_ProxyRepository(t *testing.T) {
	setupMavenTestEnv()
	defer teardownMavenTestEnv()

	payload := map[string]interface{}{
		"name":         "maven-proxy-e2e",
		"display_name": "Maven Proxy Repository",
		"description":  "Proxy Maven repository for e2e testing",
		"type":         "proxy",
		"package_type": "maven",
		"remote_url":   "https://repo.maven.apache.org/maven2",
	}

	body, _ := marshalJSON(payload)
	resp, err := http.Post(
		mavenTestServer.URL+"/api/v1/repositories",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result map[string]interface{}
	parseJSONResponse(resp, &result)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "https://repo.maven.apache.org/maven2", data["remote_url"])
}

func TestE2E_Maven_VirtualRepository(t *testing.T) {
	setupMavenTestEnv()
	defer teardownMavenTestEnv()

	mavenRepoSvc.Create(&model.Repository{
		Name:        "maven-local-member",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeMaven),
		Enabled:     true,
	}, nil)

	mavenRepoSvc.Create(&model.Repository{
		Name:        "maven-virtual-members",
		Type:        model.RepoTypeVirtual,
		PackageType: string(model.PackageTypeMaven),
		Enabled:     true,
	}, nil)

	payload := map[string]interface{}{
		"member_name": "maven-local-member",
		"priority":    0,
	}

	body, _ := marshalJSON(payload)
	resp, err := http.Post(
		mavenTestServer.URL+"/api/v1/repositories/maven-virtual-members/members",
		"application/json",
		bytes.NewBuffer(body),
	)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	membersResp, err := http.Get(mavenTestServer.URL + "/api/v1/repositories/maven-virtual-members/members")
	assert.Nil(t, err)
	assert.Equal(t, 200, membersResp.StatusCode)

	var result map[string]interface{}
	parseJSONResponse(membersResp, &result)
	data := result["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 1)
}

func TestE2E_Maven_CompleteWorkflow(t *testing.T) {
	setupMavenTestEnv()
	defer teardownMavenTestEnv()

	repo := &model.Repository{
		Name:        "maven-workflow-complete",
		Type:        model.RepoTypeLocal,
		PackageType: string(model.PackageTypeMaven),
		Enabled:     true,
	}
	err := mavenRepoSvc.Create(repo, nil)
	assert.Nil(t, err)

	jarContent := []byte("complete workflow test jar")
	pomContent := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.test</groupId>
  <artifactId>workflow-lib</artifactId>
  <version>1.0.0</version>
  <packaging>jar</packaging>
</project>`)

	jarReq, _ := http.NewRequest(
		"PUT",
		mavenTestServer.URL+"/repo/maven-workflow-complete/com/test/workflow-lib/1.0.0/workflow-lib-1.0.0.jar",
		bytes.NewReader(jarContent),
	)
	jarReq.ContentLength = int64(len(jarContent))
	client := &http.Client{}
	jarResp, err := client.Do(jarReq)
	assert.Nil(t, err)
	assert.Equal(t, 200, jarResp.StatusCode)

	pomReq, _ := http.NewRequest(
		"PUT",
		mavenTestServer.URL+"/repo/maven-workflow-complete/com/test/workflow-lib/1.0.0/workflow-lib-1.0.0.pom",
		bytes.NewReader(pomContent),
	)
	pomReq.ContentLength = int64(len(pomContent))
	pomResp, err := client.Do(pomReq)
	assert.Nil(t, err)
	assert.Equal(t, 200, pomResp.StatusCode)

	_, _ = mavenStorageSvc.StorePackageWithBackend(
		context.Background(),
		"maven-workflow-complete",
		"maven",
		"com/test/workflow-lib",
		"1.0.0/workflow-lib-1.0.0.jar",
		bytes.NewReader(jarContent),
		int64(len(jarContent)),
		0,
	)
	_, _ = mavenStorageSvc.StorePackageWithBackend(
		context.Background(),
		"maven-workflow-complete",
		"maven",
		"com/test/workflow-lib",
		"1.0.0/workflow-lib-1.0.0.pom",
		bytes.NewReader(pomContent),
		int64(len(pomContent)),
		0,
	)

	metadataResp, err := http.Get(mavenTestServer.URL + "/repo/maven-workflow-complete/com/test/workflow-lib/maven-metadata.xml")
	assert.Nil(t, err)
	assert.Equal(t, 200, metadataResp.StatusCode)

	downloadResp, err := http.Get(mavenTestServer.URL + "/repo/maven-workflow-complete/com/test/workflow-lib/1.0.0/workflow-lib-1.0.0.jar")
	assert.Nil(t, err)
	assert.Equal(t, 200, downloadResp.StatusCode)

	sha1Resp, err := http.Get(mavenTestServer.URL + "/repo/maven-workflow-complete/com/test/workflow-lib/1.0.0/workflow-lib-1.0.0.jar.sha1")
	assert.Nil(t, err)
	assert.Equal(t, 200, sha1Resp.StatusCode)
}

func marshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func parseJSONResponse(resp *http.Response, v interface{}) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func unmarshalJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
