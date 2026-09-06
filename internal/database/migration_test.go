package database

import (
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBackfillRepositoryPolicies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.SystemConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = db

	// 存量 local/virtual 仓库默认 false（策略激活前的死配置值）
	for _, repo := range []model.Repository{
		{Name: "existing-local", Type: model.RepoTypeLocal, PackageType: "npm", AllowOverwrite: false, AllowDelete: false},
		{Name: "existing-virtual", Type: model.RepoTypeVirtual, PackageType: "npm", AllowOverwrite: false, AllowDelete: false},
		{Name: "existing-proxy", Type: model.RepoTypeProxy, PackageType: "npm", AllowOverwrite: false, AllowDelete: false},
	} {
		if err := db.Create(&repo).Error; err != nil {
			t.Fatalf("create repo %s: %v", repo.Name, err)
		}
	}

	if err := backfillRepositoryPolicies(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var local model.Repository
	if err := db.Where("name = ?", "existing-local").First(&local).Error; err != nil {
		t.Fatal(err)
	}
	if !local.AllowOverwrite || !local.AllowDelete {
		t.Fatalf("existing local repo not backfilled: overwrite=%v delete=%v, want true/true", local.AllowOverwrite, local.AllowDelete)
	}
	var virtual model.Repository
	if err := db.Where("name = ?", "existing-virtual").First(&virtual).Error; err != nil {
		t.Fatal(err)
	}
	if !virtual.AllowOverwrite || !virtual.AllowDelete {
		t.Fatalf("existing virtual repo not backfilled: overwrite=%v delete=%v", virtual.AllowOverwrite, virtual.AllowDelete)
	}
	// proxy 仓库不参与部署策略回填
	var proxy model.Repository
	if err := db.Where("name = ?", "existing-proxy").First(&proxy).Error; err != nil {
		t.Fatal(err)
	}
	if proxy.AllowOverwrite || proxy.AllowDelete {
		t.Fatalf("proxy repo should not be backfilled: overwrite=%v delete=%v", proxy.AllowOverwrite, proxy.AllowDelete)
	}
}

func TestBackfillRepositoryPoliciesIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Repository{}, &model.SystemConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = db
	repo := model.Repository{Name: "local", Type: model.RepoTypeLocal, PackageType: "npm", AllowOverwrite: false, AllowDelete: false}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("create repo: %v", err)
	}

	if err := backfillRepositoryPolicies(); err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	// 管理员把某仓库改成严格策略后，第二次启动不应再次回填覆盖
	if err := db.Model(&model.Repository{}).Where("name = ?", "local").
		Updates(map[string]interface{}{"allow_overwrite": false, "allow_delete": false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillRepositoryPolicies(); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	var after model.Repository
	if err := db.Where("name = ?", "local").First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.AllowOverwrite || after.AllowDelete {
		t.Fatalf("second backfill should be a no-op (marker set), got overwrite=%v delete=%v", after.AllowOverwrite, after.AllowDelete)
	}
}

// TestMigrateLogCleanupSchedule 验证旧 log_cleanup.interval 迁移到 scheduler.cron.log_cleanup，且幂等。
func TestMigrateLogCleanupSchedule(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.SystemConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = db
	if err := db.Create(&model.SystemConfig{Key: "log_cleanup.interval", Value: "6h", ValueType: "string"}).Error; err != nil {
		t.Fatal(err)
	}

	if err := migrateLogCleanupSchedule(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var migrated model.SystemConfig
	if err := db.Where("key = ?", "scheduler.cron.log_cleanup").First(&migrated).Error; err != nil {
		t.Fatalf("scheduler.cron.log_cleanup not created: %v", err)
	}
	if migrated.Value != "@every 6h" {
		t.Fatalf("migrated value = %q, want @every 6h", migrated.Value)
	}

	// 幂等：再次运行不重复/覆盖
	if err := migrateLogCleanupSchedule(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var again model.SystemConfig
	if err := db.Where("key = ?", "scheduler.cron.log_cleanup").First(&again).Error; err != nil {
		t.Fatal(err)
	}
	if again.Value != "@every 6h" {
		t.Fatalf("value changed on second migrate = %q", again.Value)
	}
}

// TestMigrateLogCleanupScheduleSkipsDefault 验证默认 24h 不被迁移为专属配置。
func TestMigrateLogCleanupScheduleSkipsDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.SystemConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = db
	if err := db.Create(&model.SystemConfig{Key: "log_cleanup.interval", Value: "24h", ValueType: "string"}).Error; err != nil {
		t.Fatal(err)
	}

	if err := migrateLogCleanupSchedule(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var n int64
	if err := db.Model(&model.SystemConfig{}).Where("key = ?", "scheduler.cron.log_cleanup").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("default 24h should not be migrated, scheduler.cron.log_cleanup count = %d", n)
	}
}
