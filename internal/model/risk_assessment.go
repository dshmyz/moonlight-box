package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// JSONArray 存储 JSON 数组列（如命中明细列表）。
type JSONArray []map[string]interface{}

func (a JSONArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return json.Marshal(a)
}

func (a *JSONArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported JSONArray scan type: %T", value)
	}
	if len(bytes) == 0 {
		*a = nil
		return nil
	}
	return json.Unmarshal(bytes, a)
}

// RiskAssessment 风险组件研判记录（头表）。
// 对应一次 Excel 上传的研判，Items 为明细。
type RiskAssessment struct {
	ID             uint                 `gorm:"primaryKey" json:"id"`
	FileName       string               `gorm:"size:255" json:"file_name"`
	TotalItems     int                  `gorm:"not null;default:0" json:"total_items"`
	MatchedItems   int                  `gorm:"not null;default:0" json:"matched_items"`
	ArtifactHits   int                  `gorm:"not null;default:0" json:"artifact_hits"`
	DependencyHits int                  `gorm:"not null;default:0" json:"dependency_hits"`
	CreatedBy      uint                 `gorm:"index" json:"created_by,omitempty"`
	CreatedAt      time.Time            `gorm:"autoCreateTime" json:"created_at"`
	Items          []RiskAssessmentItem `gorm:"foreignKey:AssessmentID" json:"items,omitempty"`
}

func (RiskAssessment) TableName() string {
	return "risk_assessments"
}

// 处置状态
type DispositionStatus string

const (
	DispositionNone     DispositionStatus = ""            // 未处置
	DispositionRemoved  DispositionStatus = "removed"     // 已移除制品
	DispositionBlocked  DispositionStatus = "blocked"     // 仅生成阻断规则
	DispositionIgnored  DispositionStatus = "ignored"     // 已忽略
)

// RiskAssessmentItem 风险组件研判明细。
// ArtifactHit 形如 [{"repo":"central","format":"maven","version":"1.2.3"}]
// DependencyHit 形如 [{"dependent_name":"my-app","version":"1.0.0","format":"npm","repo":"central","constraint":">=1.0.0"}]
type RiskAssessmentItem struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	AssessmentID     uint      `gorm:"not null;index" json:"assessment_id"`
	Name             string    `gorm:"size:512;index" json:"name"`
	Version          string    `gorm:"size:255" json:"version"`
	Format           string    `gorm:"size:32" json:"format,omitempty"`
	Severity         string    `gorm:"size:32" json:"severity,omitempty"`
	Reason           string    `gorm:"size:512" json:"reason,omitempty"`
	CVE              string    `gorm:"size:64" json:"cve,omitempty"`
	Matched          bool      `gorm:"not null;default:false;index" json:"matched"`
	ArtifactHit      JSONArray `gorm:"type:json" json:"artifact_hit,omitempty"`
	DependencyHit    JSONArray `gorm:"type:json" json:"dependency_hit,omitempty"`
	Disposition      DispositionStatus `gorm:"size:20;not null;default:'';index" json:"disposition"`
	DispositionNote  string    `gorm:"size:512" json:"disposition_note,omitempty"`
	DisposedBy       uint      `json:"disposed_by,omitempty"`
	DisposedAt       *time.Time `json:"disposed_at,omitempty"`
	Suggestion       string    `gorm:"-" json:"suggestion,omitempty"` // 处置建议，研判时动态计算，不落库
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (RiskAssessmentItem) TableName() string {
	return "risk_assessment_items"
}
