package database

import (
	"fmt"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/database/dialect"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/util"

	"github.com/sirupsen/logrus"
)

func AutoMigrate() error {
	if err := DB.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.UserRole{},
		&model.RolePermission{},
		&model.Blob{},
		&model.Artifact{},
		&model.ArtifactBlob{},
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
		&model.DownloadLog{},
		&model.DownloadDailyStats{},
		&model.Package{},
		&model.PackageVersion{},
		&model.AIPromptTemplate{},
		&domain.MigrationPlan{},
		&domain.MigrationJob{},
		&domain.MigrationItem{},
		&domain.MigrationConflict{},
		&domain.MigrationEvent{},
		&model.APIToken{},
	); err != nil {
		return err
	}

	return cleanupLegacyArtifactColumns()
}

func cleanupLegacyArtifactColumns() error {
	if DB == nil {
		return nil
	}

	const tableName = "artifacts"
	const columnName = "coordinates"

	exists, err := legacyColumnExists(tableName, columnName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	util.WithFields(logrus.Fields{
		util.LogKeyModule: "database",
		"table":           tableName,
		"column":          columnName,
	}).Warn("Dropping legacy artifact column")

	query, err := dialect.DropColumnSQL(DB.Dialector.Name(), tableName, columnName)
	if err != nil {
		return fmt.Errorf("unsupported database dialect for legacy artifact cleanup: %s", DB.Dialector.Name())
	}
	return DB.Exec(query).Error
}

func legacyColumnExists(tableName, columnName string) (bool, error) {
	var count int64
	query, args, err := dialect.ColumnExistsQuery(DB.Dialector.Name(), tableName, columnName)
	if err != nil {
		return false, fmt.Errorf("unsupported database dialect for legacy artifact cleanup: %s", DB.Dialector.Name())
	}
	err = DB.Raw(query, args...).Scan(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func SeedData() error {
	// 清理废弃角色/权限是一次性数据迁移，用 system_configs 中的标记守卫，确保只在首次启动执行一次。
	// 之前这段逻辑每次启动都跑，而 developer 既是"废弃角色"又是当前系统角色，
	// 导致每次重启都会删除所有用户的 developer 角色关联后重建空角色 -> 用户角色信息丢失。
	const legacyCleanupMarker = "migration.legacy_roles_perms_cleaned"
	var marker model.SystemConfig
	if DB.Where("key = ?", legacyCleanupMarker).First(&marker).Error == nil {
		// 标记已存在，说明一次性清理已执行过，跳过
	} else {
		logrus.WithField("step", "legacy_cleanup").Info("Running one-time legacy role/permission cleanup")

		// developer 是当前系统角色，绝不能删；maintainer/readonly 为已废弃角色名
		oldRoles := []string{"maintainer", "readonly"}
		for _, roleName := range oldRoles {
			var oldRole model.Role
			if err := DB.Where("name = ?", roleName).First(&oldRole).Error; err == nil {
				DB.Where("role_id = ?", oldRole.ID).Delete(&model.RolePermission{})
				DB.Where("role_id = ?", oldRole.ID).Delete(&model.UserRole{})
				DB.Delete(&oldRole)
			}
		}

		deprecatedPerms := []struct{ resource, action string }{
			{"npm", "read"}, {"npm", "write"}, {"npm", "delete"}, {"npm", "admin"},
			{"maven", "read"}, {"maven", "write"}, {"maven", "delete"}, {"maven", "admin"},
		}
		for _, dp := range deprecatedPerms {
			var perm model.Permission
			if err := DB.Where("resource = ? AND action = ?", dp.resource, dp.action).First(&perm).Error; err == nil {
				DB.Where("permission_id = ?", perm.ID).Delete(&model.RolePermission{})
				DB.Delete(&perm)
			}
		}

		// 标记已完成，后续启动不再执行（即使写入失败也无妨：developer 已不在清单，不会重复伤人）
		DB.Create(&model.SystemConfig{
			Key:         legacyCleanupMarker,
			Value:       "true",
			ValueType:   "bool",
			Category:    "migration",
			Description: "标记废弃角色/权限的一次性清理已完成",
		})
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
			{"webhooks", "read"}, {"webhooks", "write"}, {"package", "read"},
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
			{"users", "read"}, {"package", "read"},
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
		isRelease := false
		if cfg := config.Get(); cfg != nil {
			isRelease = strings.EqualFold(cfg.Server.Mode, "release")
		}

		var initialPassword string
		if isRelease {
			initialPassword = util.GenerateRandomString(20)
		} else {
			initialPassword = "admin123"
		}

		hashedPassword, err := util.HashPassword(initialPassword)
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
		printInitialAdminCredentials(initialPassword, isRelease)
	}

	return nil
}

// printInitialAdminCredentials 将首次生成的初始管理员密码打印到标准输出。
// 出于安全考虑，密码只在此处一次性显示，不写入持久化日志文件。
func printInitialAdminCredentials(password string, isRelease bool) {
	banner := strings.Repeat("=", 64)
	if isRelease {
		fmt.Printf("\n%s\n"+
			"  初始管理员账号已创建（此密码仅本次启动显示，请立即保存并登录后修改）\n"+
			"  用户名: admin\n"+
			"  密码  : %s\n"+
			"%s\n\n", banner, password, banner)
	} else {
		fmt.Printf("\n%s\n"+
			"  初始管理员账号已创建（测试/开发环境，密码固定为 admin123）\n"+
			"  用户名: admin\n"+
			"  密码  : %s\n"+
			"%s\n\n", banner, password, banner)
	}
	util.WithFields(logrus.Fields{"username": "admin"}).
		Warn("已生成随机初始管理员密码，请查看启动输出（stdout）并尽快修改")
}
