package repository

import (
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuditRepositoryBlockLogsAndStatsExcludePackageDownloads(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate audit logs: %v", err)
	}
	repo := NewAuditRepository(db)
	for _, log := range []model.AuditLog{
		{Action: model.ActionBlock, ResourceType: "npm", ResourceName: "left-pad", ResponseStatus: 403},
		{Action: model.ActionPackageDownload, ResourceType: "npm", ResourceName: "left-pad", ResponseStatus: 200},
	} {
		if err := repo.Create(&log); err != nil {
			t.Fatalf("seed audit log: %v", err)
		}
	}

	blockAction := model.ActionBlock
	logs, total, err := repo.List(1, 20, "", "", &blockAction)
	if err != nil {
		t.Fatalf("list block logs: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].Action != model.ActionBlock || logs[0].ResponseStatus != 403 {
		t.Fatalf("logs = %#v total = %d, want one 403 block log", logs, total)
	}

	stats, err := repo.GetBlockStats(24)
	if err != nil {
		t.Fatalf("get block stats: %v", err)
	}
	if stats.TotalBlocks != 1 {
		t.Fatalf("total blocks = %d, want 1", stats.TotalBlocks)
	}
}
