package ai

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAuditStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestAuditStore_PersistAndChain 验证日志持久化与哈希链完整性。
func TestAuditStore_PersistAndChain(t *testing.T) {
	db := newAuditStoreTestDB(t)
	repo := repository.NewAuditRepository(db)
	store := NewAuditStore(repo, true, 30*24*time.Hour)
	defer store.Stop()

	store.Add(AuditEntry{
		Timestamp: time.Now(),
		ToolName:  "security_analysis",
		UserID:    1,
		Username:  "admin",
		Params:    map[string]interface{}{"analysis_type": "package_scan"},
		Result:    "report...",
		Duration:  12 * time.Millisecond,
		Success:   true,
	})
	store.Add(AuditEntry{
		Timestamp: time.Now(),
		ToolName:  "block_rule_generator",
		UserID:    1,
		Username:  "admin",
		Params:    map[string]interface{}{"operation": "preview"},
		Result:    "draft...",
		Error:     "boom",
		Duration:  3 * time.Millisecond,
		Success:   false,
	})

	// 等待异步 flush
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		db.Model(&model.AuditLog{}).Count(&count)
		if count >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	var logs []model.AuditLog
	if err := db.Order("id ASC").Find(&logs).Error; err != nil {
		t.Fatalf("query logs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("persisted %d logs, want 2", len(logs))
	}

	// 哈希链：第二条的 PrevHash 应等于第一条的 LogHash
	if logs[0].PrevHash != "" {
		t.Errorf("first log PrevHash = %q, want empty", logs[0].PrevHash)
	}
	wantPrev := logs[0].LogHash()
	if logs[1].PrevHash != wantPrev {
		t.Errorf("second log PrevHash = %q, want %q", logs[1].PrevHash, wantPrev)
	}

	// 完整链路校验通过
	tampered, err := model.VerifyAuditChain(db, logs[0].ID)
	if err != nil {
		t.Fatalf("verify chain: %v", err)
	}
	if len(tampered) != 0 {
		t.Errorf("chain verification found tampered logs: %v", tampered)
	}

	// 篡改第一条，链路校验应发现
	if err := db.Model(&model.AuditLog{}).Where("id = ?", logs[0].ID).
		Update("tool_result", "tampered").Error; err != nil {
		t.Fatalf("tamper: %v", err)
	}
	tampered, err = model.VerifyAuditChain(db, logs[0].ID)
	if err != nil {
		t.Fatalf("verify chain after tamper: %v", err)
	}
	if len(tampered) == 0 {
		t.Error("chain verification should detect tampering")
	}
}

// insertAIAudit 直接按哈希链插入一条 AI 审计行（模拟 flushBatch 的落库行为）。
func insertAIAudit(t *testing.T, db *gorm.DB, prevLog *model.AuditLog, action model.AuditAction) *model.AuditLog {
	t.Helper()
	log := &model.AuditLog{
		ResourceType: "ai_tool",
		Details:      "admin",
		Action:       action,
		CreatedAt:    time.Now(),
	}
	if action == model.ActionAIToolCall {
		log.ToolName = "security_analysis"
		log.ToolResult = "report"
	}
	if prevLog != nil {
		log.PrevHash = prevLog.LogHash()
	}
	if err := db.Create(log).Error; err != nil {
		t.Fatalf("insert ai log: %v", err)
	}
	return log
}

// insertHTTPAudit 插入一条不带哈希链的普通 HTTP 审计行。
func insertHTTPAudit(t *testing.T, db *gorm.DB, action model.AuditAction) *model.AuditLog {
	t.Helper()
	log := &model.AuditLog{
		Action:     action,
		Details:    "http",
		CreatedAt:  time.Now(),
	}
	if err := db.Create(log).Error; err != nil {
		t.Fatalf("insert http log: %v", err)
	}
	return log
}

// TestAuditChain_IgnoresInterleavedHTTPRows 验证普通 HTTP 审计行（无 PrevHash）与 AI 日志混在同一张表
// 时不破坏 AI 哈希链校验：任何交错行不应导致"链校验失败"误报。
func TestAuditChain_IgnoresInterleavedHTTPRows(t *testing.T) {
	db := newAuditStoreTestDB(t)

	l1 := insertAIAudit(t, db, nil, model.ActionAIToolCall)
	// 第二条记录是普通 HTTP 审计行（id 序第二），旧实现会在此误报
	insertHTTPAudit(t, db, model.ActionLogin)
	l2 := insertAIAudit(t, db, l1, model.ActionAIToolCall)
	_ = l2

	tampered, err := model.VerifyAuditChain(db, 0)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(tampered) != 0 {
		t.Errorf("interleaved HTTP audit row broke AI chain, tampered = %v", tampered)
	}

	// 篡改 AI 行仍应被识别
	if err := db.Model(&model.AuditLog{}).Where("id = ?", l1.ID).
		Update("tool_result", "tampered").Error; err != nil {
		t.Fatalf("tamper: %v", err)
	}
	tampered, err = model.VerifyAuditChain(db, 0)
	if err != nil {
		t.Fatalf("verify after tamper: %v", err)
	}
	if len(tampered) == 0 {
		t.Error("chain verification should detect tampering of AI row")
	}
}

// TestAuditChain_HeadCropNotFalsePositive 验证保留策略裁剪链头后（首行 PrevHash 指向已删行）
// 校验不误报篡改；裁剪后链条完整性对剩余行仍然有效。
func TestAuditChain_HeadCropNotFalsePositive(t *testing.T) {
	db := newAuditStoreTestDB(t)

	l1 := insertAIAudit(t, db, nil, model.ActionAIToolCall)
	l2 := insertAIAudit(t, db, l1, model.ActionAIToolCall)
	l3 := insertAIAudit(t, db, l2, model.ActionAIPromptChange)
	_ = l3

	// 模拟保留策略裁剪最老的链头
	if err := db.Where("id = ?", l1.ID).Delete(&model.AuditLog{}).Error; err != nil {
		t.Fatalf("delete head: %v", err)
	}

	tampered, err := model.VerifyAuditChain(db, 0)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(tampered) != 0 {
		t.Errorf("cropped chain head produced false tampering: %v", tampered)
	}

	// 继续篡改保留行，仍应被识别
	if err := db.Model(&model.AuditLog{}).Where("id = ?", l2.ID).
		Update("details", "evil").Error; err != nil {
		t.Fatalf("tamper: %v", err)
	}
	tampered, err = model.VerifyAuditChain(db, 0)
	if err != nil {
		t.Fatalf("verify after tamper: %v", err)
	}
	if len(tampered) == 0 {
		t.Error("cropped chain should still detect tampering of retained rows")
	}
}

// TestAuditStore_QueryAndExport 验证过滤查询与导出。
func TestAuditStore_QueryAndExport(t *testing.T) {
	db := newAuditStoreTestDB(t)
	repo := repository.NewAuditRepository(db)
	store := NewAuditStore(repo, true, 30*24*time.Hour)
	defer store.Stop()

	for i := 0; i < 5; i++ {
		store.Add(AuditEntry{
			Timestamp: time.Now(),
			ToolName:  "package_info",
			UserID:    2,
			Username:  "dev",
			Params:    map[string]interface{}{"name": "lodash"},
			Result:    "info",
			Duration:  1 * time.Millisecond,
			Success:   true,
		})
	}
	store.Add(AuditEntry{
		Timestamp: time.Now(),
		ToolName:  "security_analysis",
		UserID:    1,
		Username:  "admin",
		Params:    map[string]interface{}{"analysis_type": "package_scan"},
		Result:    "report",
		Duration:  5 * time.Millisecond,
		Success:   false,
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		db.Model(&model.AuditLog{}).Count(&count)
		if count >= 6 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	filter := AuditFilter{ToolName: "package_info", Limit: 100}
	entries, total, err := store.Query(filter)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 5 {
		t.Errorf("filtered total = %d, want 5", total)
	}
	if len(entries) != 5 {
		t.Errorf("filtered entries = %d, want 5", len(entries))
	}

	// CSV 导出
	csvData, err := store.Export(AuditFilter{Limit: 100}, "csv")
	if err != nil {
		t.Fatalf("csv export: %v", err)
	}
	if len(csvData) == 0 {
		t.Error("csv export empty")
	}

	// JSON 导出
	jsonData, err := store.Export(AuditFilter{Limit: 100}, "json")
	if err != nil {
		t.Fatalf("json export: %v", err)
	}
	if len(jsonData) == 0 {
		t.Error("json export empty")
	}
}

// TestAuditStore_MemoryFallback 验证无 DB 时退化为内存缓冲。
func TestAuditStore_MemoryFallback(t *testing.T) {
	store := NewAuditStore(nil, true, 0)
	defer store.Stop()

	store.Add(AuditEntry{
		Timestamp: time.Now(),
		ToolName:  "package_info",
		UserID:    1,
		Username:  "admin",
		Params:    map[string]interface{}{"name": "lodash"},
		Result:    "info",
		Duration:  1 * time.Millisecond,
		Success:   true,
	})
	store.Add(AuditEntry{
		Timestamp: time.Now(),
		ToolName:  "log_query",
		UserID:    1,
		Username:  "admin",
		Result:    "logs",
		Duration:  2 * time.Millisecond,
		Success:   true,
	})

	entries := store.Get(10)
	if len(entries) != 2 {
		t.Fatalf("memory entries = %d, want 2", len(entries))
	}
	// 最新的在前
	if entries[0].ToolName != "log_query" {
		t.Errorf("newest entry = %q, want log_query", entries[0].ToolName)
	}

	entries, total, err := store.Query(AuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("query fallback: %v", err)
	}
	if total != 2 || len(entries) != 2 {
		t.Errorf("fallback query total=%d len=%d, want 2/2", total, len(entries))
	}
}

// TestAuditStore_Disabled 验证禁用时不记录。
func TestAuditStore_Disabled(t *testing.T) {
	db := newAuditStoreTestDB(t)
	repo := repository.NewAuditRepository(db)
	store := NewAuditStore(repo, false, 0)
	defer store.Stop()

	store.Add(AuditEntry{ToolName: "package_info", Success: true})

	var count int64
	db.Model(&model.AuditLog{}).Count(&count)
	if count != 0 {
		t.Errorf("disabled store persisted %d logs, want 0", count)
	}
	if store.Count() != 0 {
		t.Errorf("disabled store memory count = %d, want 0", store.Count())
	}
}

// TestTruncate_UTF8Safe 验证 truncate 不会切坏多字节 UTF-8 序列。
func TestTruncate_UTF8Safe(t *testing.T) {
	s := "中文提示词内容示例"
	got := truncate(s, 4)
	if !utf8.ValidString(got) {
		t.Errorf("truncate produced invalid UTF-8: %q", got)
	}
	want := "中文提示...[truncated]"
	if got != want {
		t.Errorf("truncate(%q, 4) = %q, want %q", s, got, want)
	}
	// 不超长时原样返回
	if truncate(s, 100) != s {
		t.Errorf("truncate below limit should return input unchanged")
	}
}
