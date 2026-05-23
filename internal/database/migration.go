package database

import (
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/util"
)

func AutoMigrate() error {
	if err := DB.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.UserRole{},
		&model.RolePermission{},
		&model.BlobV2{},
		&model.Artifact{},
		&model.ArtifactBlob{},
		&model.ArtifactTag{},
		&model.ArtifactVersion{},
		&model.ArtifactProperty{},
		&model.ArtifactRelation{},
		&model.AuditLog{},
		&model.CacheEntry{},
		&model.SystemConfig{},
		&model.Repository{},
		&model.RepositoryMember{},
		&model.BlockRule{},
		&model.StorageBackend{},
		&model.ScanResult{},
		&model.Vulnerability{},
		&model.VulnRule{},
		&model.VulnDataSource{},
		&model.Webhook{},
		&model.WebhookDelivery{},
		&model.Backup{},
		&model.MigrationTask{},
		&model.MigrationItem{},
		&model.ProxyDownloadLog{},
	); err != nil {
		return err
	}

	return nil
}

func SeedData() error {
	oldRoles := []string{"maintainer", "developer", "readonly"}
	for _, roleName := range oldRoles {
		var oldRole model.Role
		if err := DB.Where("name = ?", roleName).First(&oldRole).Error; err == nil {
			DB.Where("role_id = ?", oldRole.ID).Delete(&model.RolePermission{})
			DB.Where("role_id = ?", oldRole.ID).Delete(&model.UserRole{})
			DB.Delete(&oldRole)
		}
	}

	roles := []model.Role{
		{Name: "admin", Description: "系统管理员，拥有所有权限", IsSystemRole: true},
		{Name: "operations", Description: "运维人员，管理基础设施和仓库", IsSystemRole: true},
		{Name: "developer", Description: "普通开发人员，使用包管理服务", IsSystemRole: true},
		{Name: "security-auditor", Description: "安全员/审计，安全审计和合规检查", IsSystemRole: true},
	}

	for _, role := range roles {
		result := DB.Where("name = ?", role.Name).FirstOrCreate(&role)
		if result.Error != nil {
			return result.Error
		}
	}

	permissions := []model.Permission{
		{Resource: "system", Action: "admin"},
		{Resource: "system", Action: "read"},
		{Resource: "users", Action: "read"},
		{Resource: "users", Action: "write"},
		{Resource: "audit", Action: "read"},
		{Resource: "repositories", Action: "read"},
		{Resource: "repositories", Action: "write"},
		{Resource: "repositories", Action: "delete"},
		{Resource: "cache", Action: "read"},
		{Resource: "cache", Action: "write"},
		{Resource: "block-rules", Action: "read"},
		{Resource: "block-rules", Action: "write"},
		{Resource: "block-rules", Action: "delete"},
		{Resource: "storage-backends", Action: "read"},
		{Resource: "storage-backends", Action: "write"},
		{Resource: "security", Action: "read"},
		{Resource: "security", Action: "write"},
		{Resource: "webhooks", Action: "read"},
		{Resource: "webhooks", Action: "write"},
		{Resource: "npm", Action: "read"},
		{Resource: "npm", Action: "write"},
		{Resource: "npm", Action: "delete"},
		{Resource: "npm", Action: "admin"},
		{Resource: "maven", Action: "read"},
		{Resource: "maven", Action: "write"},
		{Resource: "maven", Action: "delete"},
		{Resource: "maven", Action: "admin"},
		{Resource: "package", Action: "read"},
		{Resource: "package", Action: "write"},
		{Resource: "package", Action: "delete"},
		{Resource: "package", Action: "delete_own"},
	}

	for _, perm := range permissions {
		result := DB.Where("resource = ? AND action = ?", perm.Resource, perm.Action).FirstOrCreate(&perm)
		if result.Error != nil {
			return result.Error
		}
	}

	var adminRole model.Role
	if err := DB.Where("name = ?", "admin").First(&adminRole).Error; err != nil {
		return err
	}

	var allPermissions []model.Permission
	DB.Find(&allPermissions)

	for _, perm := range allPermissions {
		rp := model.RolePermission{RoleID: adminRole.ID, PermissionID: perm.ID}
		DB.Where(rp).FirstOrCreate(&rp)
	}

	var operationsRole model.Role
	if err := DB.Where("name = ?", "operations").First(&operationsRole).Error; err == nil {
		opsPerms := []struct{ resource, action string }{
			{"system", "read"}, {"repositories", "read"}, {"repositories", "write"}, {"repositories", "delete"},
			{"cache", "read"}, {"cache", "write"}, {"storage-backends", "read"}, {"storage-backends", "write"},
			{"webhooks", "read"}, {"webhooks", "write"}, {"npm", "read"}, {"npm", "write"}, {"npm", "delete"},
			{"maven", "read"}, {"maven", "write"}, {"maven", "delete"}, {"package", "read"},
			{"package", "write"}, {"package", "delete"}, {"audit", "read"},
		}
		for _, p := range opsPerms {
			var perm model.Permission
			if err := DB.Where("resource = ? AND action = ?", p.resource, p.action).First(&perm).Error; err == nil {
				rp := model.RolePermission{RoleID: operationsRole.ID, PermissionID: perm.ID}
				DB.Where(rp).FirstOrCreate(&rp)
			}
		}
	}

	var developerRole model.Role
	if err := DB.Where("name = ?", "developer").First(&developerRole).Error; err == nil {
		devPerms := []struct{ resource, action string }{
			{"npm", "read"}, {"npm", "write"}, {"maven", "read"}, {"maven", "write"},
			{"package", "read"}, {"package", "write"}, {"package", "delete_own"}, {"repositories", "read"},
		}
		for _, p := range devPerms {
			var perm model.Permission
			if err := DB.Where("resource = ? AND action = ?", p.resource, p.action).First(&perm).Error; err == nil {
				rp := model.RolePermission{RoleID: developerRole.ID, PermissionID: perm.ID}
				DB.Where(rp).FirstOrCreate(&rp)
			}
		}
	}

	var securityAuditorRole model.Role
	if err := DB.Where("name = ?", "security-auditor").First(&securityAuditorRole).Error; err == nil {
		secPerms := []struct{ resource, action string }{
			{"audit", "read"}, {"security", "read"}, {"security", "write"}, {"block-rules", "read"},
			{"block-rules", "write"}, {"block-rules", "delete"}, {"system", "read"}, {"repositories", "read"},
			{"users", "read"}, {"npm", "read"}, {"maven", "read"}, {"package", "read"},
		}
		for _, p := range secPerms {
			var perm model.Permission
			if err := DB.Where("resource = ? AND action = ?", p.resource, p.action).First(&perm).Error; err == nil {
				rp := model.RolePermission{RoleID: securityAuditorRole.ID, PermissionID: perm.ID}
				DB.Where(rp).FirstOrCreate(&rp)
			}
		}
	}

	var adminUser model.User
	result := DB.Where("username = ?", "admin").First(&adminUser)
	if result.Error != nil {
		hashedPassword, err := util.HashPassword("admin123")
		if err != nil {
			return err
		}
		adminUser = model.User{
			Username:     "admin",
			PasswordHash: hashedPassword,
			Email:        "admin@moonlight.local",
			DisplayName:  "系统管理员",
			IsActive:     true,
		}
		if err := DB.Create(&adminUser).Error; err != nil {
			return err
		}
		userRole := model.UserRole{UserID: adminUser.ID, RoleID: adminRole.ID, AssignedBy: adminUser.ID}
		if err := DB.Create(&userRole).Error; err != nil {
			return err
		}
	}

	return nil
}
