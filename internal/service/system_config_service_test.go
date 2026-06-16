package service

import (
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSystemConfigServiceGetUsesCacheOnRepeatedReads(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.SystemConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&model.SystemConfig{
		Key:       "health_check.enabled",
		Value:     "true",
		ValueType: "bool",
		Category:  "network",
	}).Error; err != nil {
		t.Fatalf("seed config: %v", err)
	}

	svc := NewSystemConfigService(repository.NewSystemConfigRepository(db))
	first, err := svc.Get("health_check.enabled")
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	if first.Value != "true" {
		t.Fatalf("first value = %q, want true", first.Value)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	second, err := svc.Get("health_check.enabled")
	if err != nil {
		t.Fatalf("second get should use cache: %v", err)
	}
	if second.Value != "true" {
		t.Fatalf("second value = %q, want true", second.Value)
	}
}
