package model

import "time"

type BlockMatchType string

const (
	BlockMatchExact    BlockMatchType = "exact"
	BlockMatchWildcard BlockMatchType = "wildcard"
	BlockMatchRange    BlockMatchType = "range"
)

// ConditionType 条件阻断类型
type ConditionType string

const (
	ConditionTypeLicense     ConditionType = "license"      // 按 license 阻断
	ConditionTypePublishTime ConditionType = "publish_time" // 按发布时间阻断
)

// ConditionOp 条件运算符
type ConditionOp string

const (
	ConditionOpEquals     ConditionOp = "equals"      // 等于
	ConditionOpContains   ConditionOp = "contains"    // 包含
	ConditionOpBefore     ConditionOp = "before"      // 早于
	ConditionOpAfter      ConditionOp = "after"       // 晚于
	ConditionOpWithinLast ConditionOp = "within_last" // 最近 N 天内（ConditionValue 为天数）
)

// PackageTypeAll 表示匹配所有包类型的特殊值
const PackageTypeAll = "all"

type BlockRule struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	PackageName string         `gorm:"size:200;not null;index" json:"package_name"`
	Version     string         `gorm:"size:100;not null" json:"version"`
	MatchType   BlockMatchType `gorm:"size:20;not null;default:exact" json:"match_type"`
	PackageType string         `gorm:"size:20;not null" json:"package_type"`
	Reason      string         `gorm:"size:500" json:"reason,omitempty"`
	Enabled     bool           `gorm:"default:true" json:"enabled"`
	// 条件阻断字段：支持按 license、发布时间等条件阻断
	ConditionType  ConditionType `gorm:"size:30" json:"condition_type"`
	ConditionOp    ConditionOp   `gorm:"size:20" json:"condition_op"`
	ConditionValue string        `gorm:"size:500" json:"condition_value"`
	CreatedBy      *uint         `json:"created_by,omitempty"`
	CreatedAt      time.Time     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time     `gorm:"autoUpdateTime" json:"updated_at"`
}
