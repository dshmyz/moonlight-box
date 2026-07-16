package main

import (
	"context"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/database"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuditLoggerAdapterMapsBlockAction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	svc := service.NewAuditService()
	defer svc.Shutdown()
	adapter := &auditLoggerAdapter{svc: svc}
	adapter.Log(context.Background(), runtime.AuditEntry{Action: "block", ResourceType: "npm", ResourceName: "left-pad", ResponseStatus: 403})
	for i := 1; i < 100; i++ {
		adapter.Log(context.Background(), runtime.AuditEntry{Action: "package_download", ResourceType: "npm", ResourceName: "left-pad"})
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		var count int64
		if err := db.Model(&model.AuditLog{}).Count(&count).Error; err != nil {
			t.Fatalf("count audit logs: %v", err)
		}
		if count == 100 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("audit logs persisted = %d, want 100", count)
		}
		time.Sleep(10 * time.Millisecond)
	}

	var blockLog model.AuditLog
	if err := db.Where("action = ?", model.ActionBlock).First(&blockLog).Error; err != nil {
		t.Fatalf("find block audit log: %v", err)
	}
	if blockLog.ResponseStatus != 403 {
		t.Fatalf("block response status = %d, want 403", blockLog.ResponseStatus)
	}
	var downloadLog model.AuditLog
	if err := db.Where("action = ?", model.ActionPackageDownload).First(&downloadLog).Error; err != nil {
		t.Fatalf("find download audit log: %v", err)
	}
}
