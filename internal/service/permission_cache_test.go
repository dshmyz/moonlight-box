package service

import (
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPermissionCacheInvalidateUserClearsPermissionSet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Role{}, &model.Permission{}, &model.UserRole{}, &model.RolePermission{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	user := model.User{Username: "alice", Email: "alice@example.com"}
	role := model.Role{Name: "developer"}
	readPerm := model.Permission{Resource: "repo", Action: "read"}
	writePerm := model.Permission{Resource: "repo", Action: "write"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := db.Create(&readPerm).Error; err != nil {
		t.Fatalf("create read permission: %v", err)
	}
	if err := db.Create(&writePerm).Error; err != nil {
		t.Fatalf("create write permission: %v", err)
	}
	if err := db.Create(&model.UserRole{UserID: user.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatalf("create user role: %v", err)
	}
	if err := db.Create(&model.RolePermission{RoleID: role.ID, PermissionID: readPerm.ID}).Error; err != nil {
		t.Fatalf("create role permission: %v", err)
	}

	svc := NewPermissionCacheService(repository.NewRoleRepository(db), time.Hour)
	hasRead, err := svc.HasPermission(user.ID, "repo", "read")
	if err != nil {
		t.Fatalf("check read permission: %v", err)
	}
	if !hasRead {
		t.Fatal("expected initial read permission")
	}

	if err := db.Where("role_id = ?", role.ID).Delete(&model.RolePermission{}).Error; err != nil {
		t.Fatalf("delete role permissions: %v", err)
	}
	if err := db.Create(&model.RolePermission{RoleID: role.ID, PermissionID: writePerm.ID}).Error; err != nil {
		t.Fatalf("create replacement role permission: %v", err)
	}

	svc.InvalidateUser(user.ID)

	hasRead, err = svc.HasPermission(user.ID, "repo", "read")
	if err != nil {
		t.Fatalf("check invalidated read permission: %v", err)
	}
	if hasRead {
		t.Fatal("expected read permission to be removed after invalidation")
	}
	hasWrite, err := svc.HasPermission(user.ID, "repo", "write")
	if err != nil {
		t.Fatalf("check invalidated write permission: %v", err)
	}
	if !hasWrite {
		t.Fatal("expected write permission after invalidation")
	}
}
