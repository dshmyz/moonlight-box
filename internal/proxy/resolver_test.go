package proxy

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockMetadataAdapter struct {
	pkgType     types.PackageType
	handleGetFn func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error)
}

func (m *mockMetadataAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	return &types.RequestIntent{Type: types.RequestMetadata, Path: path}
}

func (m *mockMetadataAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	if m.handleGetFn != nil {
		return m.handleGetFn(ctx, repo, intent)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockMetadataAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockMetadataAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	return fmt.Errorf("not implemented")
}

func (m *mockMetadataAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	return &types.PackagePathInfo{Name: path, RemotePath: path}, nil
}

func (m *mockMetadataAdapter) Type() types.PackageType {
	return m.pkgType
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = db.AutoMigrate(&model.Repository{}, &model.RepositoryGroup{})
	assert.NoError(t, err)
	return db
}

func createVirtualRepoWithMembers(t *testing.T, db *gorm.DB, virtualName string, pkgType string, memberNames []string) *model.Repository {
	virtualRepo := &model.Repository{
		Name:        virtualName,
		Type:        model.RepoTypeVirtual,
		PackageType: pkgType,
		Enabled:     true,
	}
	err := db.Create(virtualRepo).Error
	assert.NoError(t, err)

	for i, memberName := range memberNames {
		memberRepo := &model.Repository{
			Name:        memberName,
			Type:        model.RepoTypeProxy,
			PackageType: pkgType,
			Enabled:     true,
			RemoteURL:   "https://example.com/" + memberName,
		}
		err := db.Create(memberRepo).Error
		assert.NoError(t, err)

		group := model.RepositoryGroup{
			VirtualRepoID: virtualRepo.ID,
			MemberRepoID:  memberRepo.ID,
			Priority:      i,
		}
		err = db.Create(&group).Error
		assert.NoError(t, err)
	}

	return virtualRepo
}

func TestResolveMetadata_FirstMemberSucceeds(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-local", "npm-proxy-cn", "npm-proxy-official"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	callCount := int32(0)
	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			atomic.AddInt32(&callCount, 1)
			return &types.ContentResult{
				StatusCode:  200,
				ContentType: "application/json",
				Content:     io.NopCloser(nil),
				Size:        100,
			}, nil
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "lodash", Path: "lodash"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
	assert.Equal(t, "application/json", result.ContentType)
	assert.True(t, atomic.LoadInt32(&callCount) > 0)
}

func TestResolveMetadata_FirstMemberFailsSecondSucceeds(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-local", "npm-proxy-cn"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			switch repo.Name {
			case "npm-local":
				return &types.ContentResult{
					StatusCode: 404,
					ExtraData:  map[string]interface{}{"message": "not found in local"},
				}, nil
			case "npm-proxy-cn":
				return &types.ContentResult{
					StatusCode:  200,
					ContentType: "application/json",
					Content:     io.NopCloser(nil),
					Size:        200,
				}, nil
			default:
				return nil, fmt.Errorf("unexpected repo: %s", repo.Name)
			}
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "express", Path: "express"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}

func TestResolveMetadata_AllMembersFail(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-local", "npm-proxy-cn"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			return &types.ContentResult{
				StatusCode: 404,
				ExtraData:  map[string]interface{}{"message": "not found"},
			}, nil
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "nonexistent", Path: "nonexistent"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrPackageNotFound)
}

func TestResolveMetadata_NoMembers(t *testing.T) {
	db := setupTestDB(t)

	virtualRepo := &model.Repository{
		Name:        "empty-virtual",
		Type:        model.RepoTypeVirtual,
		PackageType: "npm",
		Enabled:     true,
	}
	err := db.Create(virtualRepo).Error
	assert.NoError(t, err)

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{pkgType: types.NpmType}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "lodash", Path: "lodash"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrPackageNotFound)
}

func TestResolveMetadata_NoMatchingMembers(t *testing.T) {
	db := setupTestDB(t)

	virtualRepo := &model.Repository{
		Name:        "maven-virtual",
		Type:        model.RepoTypeVirtual,
		PackageType: "maven",
		Enabled:     true,
	}
	err := db.Create(virtualRepo).Error
	assert.NoError(t, err)

	npmRepo := &model.Repository{
		Name:        "npm-proxy",
		Type:        model.RepoTypeProxy,
		PackageType: "npm",
		Enabled:     true,
	}
	err = db.Create(npmRepo).Error
	assert.NoError(t, err)

	group := model.RepositoryGroup{
		VirtualRepoID: virtualRepo.ID,
		MemberRepoID:  npmRepo.ID,
		Priority:      0,
	}
	err = db.Create(&group).Error
	assert.NoError(t, err)

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{pkgType: types.MavenType}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "org.apache.commons", Path: "org/apache/commons/commons-lang3"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrPackageNotFound)
}

func TestResolveMetadata_AdapterReturnsError(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-local", "npm-proxy-cn"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			return nil, fmt.Errorf("network error")
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "lodash", Path: "lodash"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "network error")
}

func TestResolveMetadata_PackageNotFoundErrorIgnored(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-local", "npm-proxy-cn"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			return nil, ErrPackageNotFound
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "lodash", Path: "lodash"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrPackageNotFound)
}

func TestResolveMetadata_MixedErrorsWithOneSuccess(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "maven-virtual", "maven", []string{"maven-local", "maven-proxy-aliyun", "maven-proxy-central"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{
		pkgType: types.MavenType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			switch repo.Name {
			case "maven-local":
				return &types.ContentResult{StatusCode: 404, ExtraData: map[string]interface{}{"error": "not found"}}, nil
			case "maven-proxy-aliyun":
				return nil, ErrPackageNotFound
			case "maven-proxy-central":
				return &types.ContentResult{
					StatusCode:  200,
					ContentType: "application/xml",
					ExtraData:   map[string]interface{}{"xml_body": []byte("<metadata>")},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected repo: %s", repo.Name)
			}
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "org.apache.commons:commons-lang3", Path: "org/apache/commons/commons-lang3/maven-metadata.xml"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}

func TestResolveMetadata_ContextCancellation(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-local", "npm-proxy-cn"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			return &types.ContentResult{StatusCode: 200, ContentType: "application/json"}, nil
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "lodash", Path: "lodash"}

	result, err := handler.ResolveMetadata(ctx, virtualRepo, intent, adp)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestResolveMetadata_ContextTimeout(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-proxy-cn"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return &types.ContentResult{StatusCode: 200}, nil
			}
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "lodash", Path: "lodash"}

	result, err := handler.ResolveMetadata(ctx, virtualRepo, intent, adp)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestResolveMetadata_StatusCodeBoundary(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-local", "npm-proxy-cn"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			if repo.Name == "npm-local" {
				return &types.ContentResult{StatusCode: 399}, nil
			}
			return &types.ContentResult{StatusCode: 400, ExtraData: map[string]interface{}{"error": "bad request"}}, nil
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "test", Path: "test"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 399, result.StatusCode)
}

func TestResolveMetadata_MultipleMembersFirstFastReturns(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-local", "npm-proxy-cn", "npm-proxy-official"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			switch repo.Name {
			case "npm-local":
				time.Sleep(100 * time.Millisecond)
				return &types.ContentResult{StatusCode: 200, ContentType: "application/json"}, nil
			case "npm-proxy-cn":
				return &types.ContentResult{StatusCode: 200, ContentType: "application/json"}, nil
			case "npm-proxy-official":
				time.Sleep(200 * time.Millisecond)
				return &types.ContentResult{StatusCode: 200, ContentType: "application/json"}, nil
			default:
				return nil, fmt.Errorf("unexpected repo")
			}
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "lodash", Path: "lodash"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}

func TestResolveMetadata_WithCache(t *testing.T) {
	db := setupTestDB(t)
	virtualRepo := createVirtualRepoWithMembers(t, db, "npm-virtual", "npm", []string{"npm-proxy-cn"})

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)

	cache := NewRepositoryCache(repoRepo, groupRepo, 5*time.Minute)
	handler := NewRepoHandler(repoRepo, groupRepo, cache)

	adp := &mockMetadataAdapter{
		pkgType: types.NpmType,
		handleGetFn: func(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
			return &types.ContentResult{
				StatusCode:  200,
				ContentType: "application/json",
				Content:     io.NopCloser(nil),
				Size:        50,
			}, nil
		},
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "lodash", Path: "lodash"}

	result, err := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}

func TestResolveMetadata_GetMembersError(t *testing.T) {
	db, dbErr := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, dbErr)
	assert.NoError(t, db.AutoMigrate(&model.Repository{}))

	virtualRepo := &model.Repository{
		Name:        "broken-virtual",
		Type:        model.RepoTypeVirtual,
		PackageType: "npm",
		Enabled:     true,
	}
	assert.NoError(t, db.Create(virtualRepo).Error)

	groupRepo := repository.NewGroupRepository(db)
	repoRepo := repository.NewRepositoryRepository(db)
	handler := NewRepoHandler(repoRepo, groupRepo, nil)

	adp := &mockMetadataAdapter{pkgType: types.NpmType}
	intent := &types.RequestIntent{Type: types.RequestMetadata, Name: "lodash", Path: "lodash"}

	_, resolveErr := handler.ResolveMetadata(context.Background(), virtualRepo, intent, adp)

	assert.Error(t, resolveErr)
}