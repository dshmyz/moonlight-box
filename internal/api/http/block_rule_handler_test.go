package http

import (
	"bytes"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupBlockRuleHandlerTest 构建内存 SQLite 数据库并组装真实的 handler 链路。
// 测试不使用 mock，而是真实走 repository → service → handler。
func setupBlockRuleHandlerTest(t *testing.T) (*BlockRuleHandler, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.BlockRule{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewBlockRuleRepository(db)
	svc := service.NewBlockRuleService(repo, nil)
	handler := NewBlockRuleHandler(svc, nil, nil)
	return handler, db
}

// blockRuleResponse 统一解析 Create 接口返回的 data 字段
type blockRuleResponse struct {
	ConditionType  string `json:"condition_type"`
	ConditionOp    string `json:"condition_op"`
	ConditionValue string `json:"condition_value"`
	PackageName    string `json:"package_name"`
	Version        string `json:"version"`
	MatchType      string `json:"match_type"`
	PackageType    string `json:"package_type"`
	ID             uint   `json:"id"`
}

// doCreateBlockRule 向 handler 发送 POST 创建请求并返回响应。
func doCreateBlockRule(t *testing.T, handler *BlockRuleHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.POST("/api/block-rules", handler.Create)

	req := httptest.NewRequest(stdhttp.MethodPost, "/api/block-rules", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func doBatchImportBlockRules(t *testing.T, handler *BlockRuleHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.POST("/api/block-rules/batch-import", handler.BatchImport)

	req := httptest.NewRequest(stdhttp.MethodPost, "/api/block-rules/batch-import", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestCreateBlockRule_WithCondition 验证带条件字段的创建请求能正常返回 201，
// 且返回的规则包含正确的 condition_type / condition_op / condition_value。
func TestCreateBlockRule_WithCondition(t *testing.T) {
	handler, db := setupBlockRuleHandlerTest(t)

	body := `{
		"package_name": "lodash",
		"version": "4.17.21",
		"match_type": "exact",
		"package_type": "npm",
		"reason": "license blocked",
		"condition_type": "license",
		"condition_op": "equals",
		"condition_value": "GPL-3.0"
	}`

	w := doCreateBlockRule(t, handler, body)
	if w.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data blockRuleResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	got := resp.Data
	if got.ConditionType != "license" {
		t.Fatalf("condition_type = %q, want %q", got.ConditionType, "license")
	}
	if got.ConditionOp != "equals" {
		t.Fatalf("condition_op = %q, want %q", got.ConditionOp, "equals")
	}
	if got.ConditionValue != "GPL-3.0" {
		t.Fatalf("condition_value = %q, want %q", got.ConditionValue, "GPL-3.0")
	}

	// 验证落库数据一致
	var stored model.BlockRule
	if err := db.First(&stored, got.ID).Error; err != nil {
		t.Fatalf("load stored rule: %v", err)
	}
	if stored.ConditionType != model.ConditionTypeLicense {
		t.Fatalf("stored condition_type = %q, want %q", stored.ConditionType, model.ConditionTypeLicense)
	}
	if stored.ConditionOp != model.ConditionOpEquals {
		t.Fatalf("stored condition_op = %q, want %q", stored.ConditionOp, model.ConditionOpEquals)
	}
	if stored.ConditionValue != "GPL-3.0" {
		t.Fatalf("stored condition_value = %q, want %q", stored.ConditionValue, "GPL-3.0")
	}
}

// TestCreateBlockRule_InvalidConditionType 验证非法的 condition_type 返回 400。
func TestCreateBlockRule_InvalidConditionType(t *testing.T) {
	handler, _ := setupBlockRuleHandlerTest(t)

	body := `{
		"package_name": "lodash",
		"version": "4.17.21",
		"match_type": "exact",
		"package_type": "npm",
		"condition_type": "invalid",
		"condition_op": "equals",
		"condition_value": "GPL-3.0"
	}`

	w := doCreateBlockRule(t, handler, body)
	if w.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

// TestCreateBlockRule_InvalidConditionOp 验证非法的 condition_op 返回 400。
func TestCreateBlockRule_InvalidConditionOp(t *testing.T) {
	handler, _ := setupBlockRuleHandlerTest(t)

	body := `{
		"package_name": "lodash",
		"version": "4.17.21",
		"match_type": "exact",
		"package_type": "npm",
		"condition_type": "license",
		"condition_op": "invalid",
		"condition_value": "GPL-3.0"
	}`

	w := doCreateBlockRule(t, handler, body)
	if w.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

// TestCreateBlockRule_NoCondition 验证不传条件字段时行为与现有一致，
// 条件字段应为空字符串，且正常创建。
func TestCreateBlockRule_NoCondition(t *testing.T) {
	handler, db := setupBlockRuleHandlerTest(t)

	body := `{
		"package_name": "lodash",
		"version": "4.17.21",
		"match_type": "exact",
		"package_type": "npm",
		"reason": "plain block"
	}`

	w := doCreateBlockRule(t, handler, body)
	if w.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data blockRuleResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	got := resp.Data
	if got.ConditionType != "" {
		t.Fatalf("condition_type = %q, want empty", got.ConditionType)
	}
	if got.ConditionOp != "" {
		t.Fatalf("condition_op = %q, want empty", got.ConditionOp)
	}
	if got.ConditionValue != "" {
		t.Fatalf("condition_value = %q, want empty", got.ConditionValue)
	}

	// 验证落库条件字段为空
	var stored model.BlockRule
	if err := db.First(&stored, got.ID).Error; err != nil {
		t.Fatalf("load stored rule: %v", err)
	}
	if stored.ConditionType != "" {
		t.Fatalf("stored condition_type = %q, want empty", stored.ConditionType)
	}
	if stored.ConditionOp != "" {
		t.Fatalf("stored condition_op = %q, want empty", stored.ConditionOp)
	}
	if stored.ConditionValue != "" {
		t.Fatalf("stored condition_value = %q, want empty", stored.ConditionValue)
	}
}

func TestBatchImportBlockRules_CountsInvalidRulesAsFailed(t *testing.T) {
	handler, db := setupBlockRuleHandlerTest(t)

	body := `{
		"rules": [
			{
				"package_name": "lodash",
				"version": "4.17.21",
				"package_type": "npm",
				"condition_type": "license",
				"condition_op": "contains",
				"condition_value": "GPL"
			},
			{
				"package_name": "fresh-pkg",
				"version": "*",
				"match_type": "wildcard",
				"package_type": "npm",
				"condition_type": "publish_time",
				"condition_op": "within_last",
				"condition_value": "0"
			}
		]
	}`

	w := doBatchImportBlockRules(t, handler, body)
	if w.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Success int `json:"success"`
			Failed  int `json:"failed"`
			Total   int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Success != 1 || resp.Data.Failed != 1 || resp.Data.Total != 2 {
		t.Fatalf("response data = %+v, want success=1 failed=1 total=2", resp.Data)
	}

	var count int64
	if err := db.Model(&model.BlockRule{}).Count(&count).Error; err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if count != 1 {
		t.Fatalf("只应落库有效规则 1 条，实际 %d 条", count)
	}
}
