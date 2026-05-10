package service

import (
	"testing"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/gormlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(gormlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// 限制为单连接，避免 SQLite :memory: 模式下多连接导致数据不一致
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	db.AutoMigrate(&model.Repository{}, &model.RepositoryGroup{})
	return db
}

func setupRepositoryService(t *testing.T) (*RepositoryService, *gorm.DB) {
	db := setupTestDB(t)
	repoRepo := repository.NewRepositoryRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	service := NewRepositoryService(repoRepo, groupRepo, db)
	return service, db
}

func TestRepositoryService_Create_LocalRepo(t *testing.T) {
	service, _ := setupRepositoryService(t)

	repo := &model.Repository{
		Name:        "npm-local",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
		Enabled:     true,
	}

	err := service.Create(repo, nil)
	assert.Nil(t, err)
	assert.NotZero(t, repo.ID)

	// 验证仓库已创建
	retrieved, err := service.Get("npm-local")
	assert.Nil(t, err)
	assert.Equal(t, "npm-local", retrieved.Name)
	assert.Equal(t, model.RepoTypeLocal, retrieved.Type)
}

func TestRepositoryService_Create_ProxyRepo(t *testing.T) {
	service, _ := setupRepositoryService(t)

	repo := &model.Repository{
		Name:        "npm-proxy",
		Type:        model.RepoTypeProxy,
		PackageType: "npm",
		RemoteURL:   "https://registry.npmjs.org",
		Enabled:     true,
	}

	err := service.Create(repo, nil)
	assert.Nil(t, err)
	assert.NotZero(t, repo.ID)

	// 验证代理仓库
	retrieved, err := service.Get("npm-proxy")
	assert.Nil(t, err)
	assert.Equal(t, "https://registry.npmjs.org", retrieved.RemoteURL)
}

func TestRepositoryService_Create_VirtualRepo(t *testing.T) {
	service, _ := setupRepositoryService(t)

	// 先创建本地仓库
	localRepo := &model.Repository{
		Name:        "npm-local",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
		Enabled:     true,
	}
	if err := service.Create(localRepo, nil); err != nil {
		t.Fatalf("failed to create local repo: %v", err)
	}

	// 创建代理仓库
	proxyRepo := &model.Repository{
		Name:        "npm-proxy",
		Type:        model.RepoTypeProxy,
		PackageType: "npm",
		RemoteURL:   "https://registry.npmjs.org",
		Enabled:     true,
	}
	if err := service.Create(proxyRepo, nil); err != nil {
		t.Fatalf("failed to create proxy repo: %v", err)
	}

	// 创建虚拟仓库并添加成员
	virtualRepo := &model.Repository{
		Name:        "npm-virtual",
		Type:        model.RepoTypeVirtual,
		PackageType: "npm",
		Enabled:     true,
	}

	members := []string{"npm-local", "npm-proxy"}
	err := service.Create(virtualRepo, members)
	assert.Nil(t, err)
	assert.NotZero(t, virtualRepo.ID)

	// 验证成员关系
	memberList, err := service.GetMembers("npm-virtual")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(memberList))
}

func TestRepositoryService_List(t *testing.T) {
	service, db := setupRepositoryService(t)

	// 创建多个仓库
	db.Create(&model.Repository{Name: "repo1", Type: model.RepoTypeLocal, PackageType: "npm"})
	db.Create(&model.Repository{Name: "repo2", Type: model.RepoTypeProxy, PackageType: "npm"})
	db.Create(&model.Repository{Name: "repo3", Type: model.RepoTypeLocal, PackageType: "maven"})

	// 列出所有仓库
	repos, err := service.List(nil)
	assert.Nil(t, err)
	assert.GreaterOrEqual(t, len(repos), 3)

	// 按类型过滤
	repos, err = service.List(map[string]interface{}{"type": model.RepoTypeLocal})
	assert.Nil(t, err)
	assert.GreaterOrEqual(t, len(repos), 2)
}

func TestRepositoryService_Update(t *testing.T) {
	service, _ := setupRepositoryService(t)

	// 创建仓库
	repo := &model.Repository{
		Name:        "test-repo",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
	}
	service.Create(repo, nil)

	// 更新仓库
	err := service.Update("test-repo", map[string]interface{}{
		"display_name": "Test Repository",
		"description":  "Updated description",
	})
	assert.Nil(t, err)

	// 验证更新
	updated, err := service.Get("test-repo")
	assert.Nil(t, err)
	assert.Equal(t, "Test Repository", updated.DisplayName)
	assert.Equal(t, "Updated description", updated.Description)
}

func TestRepositoryService_Delete(t *testing.T) {
	service, _ := setupRepositoryService(t)

	// 创建仓库
	repo := &model.Repository{
		Name:        "to-delete",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
	}
	service.Create(repo, nil)

	// 删除仓库
	err := service.Delete("to-delete")
	assert.Nil(t, err)

	// 验证已删除
	_, err = service.Get("to-delete")
	assert.NotNil(t, err)
}

func TestRepositoryService_AddMember(t *testing.T) {
	service, db := setupRepositoryService(t)

	// 创建仓库
	localRepo := &model.Repository{
		Name:        "npm-local-2",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
	}
	db.Create(localRepo)

	virtualRepo := &model.Repository{
		Name:        "npm-virtual-2",
		Type:        model.RepoTypeVirtual,
		PackageType: "npm",
	}
	db.Create(virtualRepo)

	// 添加成员
	err := service.AddMember("npm-virtual-2", "npm-local-2", 0)
	assert.Nil(t, err)

	// 验证成员关系
	members, err := service.GetMembers("npm-virtual-2")
	assert.Nil(t, err)
	assert.Equal(t, 1, len(members))
	assert.Equal(t, "npm-local-2", members[0].MemberRepo.Name)
}

func TestRepositoryService_RemoveMember(t *testing.T) {
	service, db := setupRepositoryService(t)

	// 创建仓库
	localRepo := &model.Repository{
		Name:        "npm-local-3",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
	}
	db.Create(localRepo)

	virtualRepo := &model.Repository{
		Name:        "npm-virtual-3",
		Type:        model.RepoTypeVirtual,
		PackageType: "npm",
	}
	db.Create(virtualRepo)
	service.AddMember("npm-virtual-3", "npm-local-3", 0)

	// 移除成员
	err := service.RemoveMember("npm-virtual-3", "npm-local-3")
	assert.Nil(t, err)

	// 验证已移除
	members, err := service.GetMembers("npm-virtual-3")
	assert.Nil(t, err)
	assert.Equal(t, 0, len(members))
}

func TestRepositoryService_Create_VirtualRepoWithNonExistentMember(t *testing.T) {
	service, _ := setupRepositoryService(t)

	// 创建虚拟仓库，包含不存在的成员
	virtualRepo := &model.Repository{
		Name:        "npm-virtual-empty",
		Type:        model.RepoTypeVirtual,
		PackageType: "npm",
		Enabled:     true,
	}

	members := []string{"non-existent-repo"}
	err := service.Create(virtualRepo, members)
	assert.Nil(t, err)
	assert.NotZero(t, virtualRepo.ID)

	// 验证没有成员被添加
	memberList, err := service.GetMembers("npm-virtual-empty")
	assert.Nil(t, err)
	assert.Equal(t, 0, len(memberList))
}

func TestRepositoryService_GetAuthConfig_None(t *testing.T) {
	repo := &model.Repository{
		Name:       "test",
		AuthType:   "none",
		AuthConfig: "",
	}

	cfg, err := repo.GetAuthConfig()
	assert.Nil(t, err)
	assert.Equal(t, "none", cfg.Type)
}

func TestRepositoryService_GetAuthConfig_Basic(t *testing.T) {
	repo := &model.Repository{
		Name:       "test",
		AuthType:   "basic",
		AuthConfig: `{"type":"basic","basic":{"username":"user","password":"pass"}}`,
	}

	cfg, err := repo.GetAuthConfig()
	assert.Nil(t, err)
	assert.Equal(t, "basic", cfg.Type)
	assert.Equal(t, "user", cfg.Basic.Username)
	assert.Equal(t, "pass", cfg.Basic.Password)
}

func TestRepositoryService_GetAuthConfig_Bearer(t *testing.T) {
	repo := &model.Repository{
		Name:       "test",
		AuthType:   "bearer",
		AuthConfig: `{"type":"bearer","bearer":{"token":"test-token"}}`,
	}

	cfg, err := repo.GetAuthConfig()
	assert.Nil(t, err)
	assert.Equal(t, "bearer", cfg.Type)
	assert.Equal(t, "test-token", cfg.Bearer.Token)
}

func TestRepositoryService_GetAuthConfig_APIKey(t *testing.T) {
	repo := &model.Repository{
		Name:       "test",
		AuthType:   "api_key",
		AuthConfig: `{"type":"api_key","api_key":{"header_name":"X-API-Key","key_value":"secret"}}`,
	}

	cfg, err := repo.GetAuthConfig()
	assert.Nil(t, err)
	assert.Equal(t, "api_key", cfg.Type)
	assert.Equal(t, "X-API-Key", cfg.APIKey.HeaderName)
	assert.Equal(t, "secret", cfg.APIKey.KeyValue)
}

func TestRepositoryService_RepositoryTypes(t *testing.T) {
	service, _ := setupRepositoryService(t)

	// 测试所有仓库类型
	repoTypes := []model.RepositoryType{
		model.RepoTypeLocal,
		model.RepoTypeProxy,
		model.RepoTypeVirtual,
	}

	for _, repoType := range repoTypes {
		repo := &model.Repository{
			Name:        "repo-type-" + string(repoType),
			Type:        repoType,
			PackageType: "npm",
			Enabled:     true,
		}

		err := service.Create(repo, nil)
		assert.Nil(t, err)
		assert.NotZero(t, repo.ID, "Failed for type: %s", repoType)

		// 验证类型
		retrieved, err := service.Get(repo.Name)
		assert.Nil(t, err)
		assert.Equal(t, repoType, retrieved.Type)
	}
}

func TestRepositoryService_Update_VirtualRepoMembers(t *testing.T) {
	service, db := setupRepositoryService(t)

	// 创建成员仓库
	localRepo1 := &model.Repository{
		Name:        "local-1",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
	}
	db.Create(localRepo1)

	localRepo2 := &model.Repository{
		Name:        "local-2",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
	}
	db.Create(localRepo2)

	localRepo3 := &model.Repository{
		Name:        "local-3",
		Type:        model.RepoTypeLocal,
		PackageType: "npm",
	}
	db.Create(localRepo3)

	// 创建虚拟仓库
	virtualRepo := &model.Repository{
		Name:        "virtual-update-test",
		Type:        model.RepoTypeVirtual,
		PackageType: "npm",
	}
	service.Create(virtualRepo, []string{"local-1", "local-2"})

	// 验证初始成员
	members, err := service.GetMembers("virtual-update-test")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(members))

	// 更新成员列表（使用 []interface{} 模拟 JSON 解析结果）
	err = service.Update("virtual-update-test", map[string]interface{}{
		"members": []interface{}{"local-2", "local-3"},
	})
	assert.Nil(t, err)

	// 验证成员已更新
	members, err = service.GetMembers("virtual-update-test")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(members))
	assert.Equal(t, "local-2", members[0].MemberRepo.Name)
	assert.Equal(t, "local-3", members[1].MemberRepo.Name)
}
