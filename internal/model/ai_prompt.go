package model

import "time"

// PromptStatus AI 提示词模板状态。
type PromptStatus string

const (
	PromptStatusDraft   PromptStatus = "draft"   // 草稿：不生效，供评审
	PromptStatusActive  PromptStatus = "active"  // 生效：被 GetSystemPrompt 选中
	PromptStatusRetired PromptStatus = "retired" // 已下线
)

// AIPromptTemplate 集中式系统提示词模板。
//
// 治理设计：
//   - 变更评审：新内容以 draft 状态创建，只有显式 Activate 才生效；
//     Activate/Create 都会写入 AI 审计日志（ActionAIPromptChange）。
//   - 版本管理：同一 Name 下每次创建递增 Version，历史版本可回溯。
//   - A/B 测试：ABGroup 为 "A"/"B" 的模板按 Weight（0-100，用户百分比）分配流量，
//     未被分配的流量回落到 ABGroup 为空字符串的 active 模板。
type AIPromptTemplate struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"size:100;not null;index:idx_prompt_name_status,priority:1;uniqueIndex:idx_prompt_name_version,priority:1" json:"name"`
	Version     int          `gorm:"not null;default:1;uniqueIndex:idx_prompt_name_version,priority:2" json:"version"`
	Content     string       `gorm:"type:text;not null" json:"content"`
	Status      PromptStatus `gorm:"size:20;not null;default:draft;index:idx_prompt_name_status,priority:2" json:"status"`
	ABGroup     string       `gorm:"size:10;default:''" json:"ab_group,omitempty"`
	Weight      int          `gorm:"not null;default:0" json:"weight"` // 0-100，A/B 实验流量占比
	Description string       `gorm:"size:500" json:"description,omitempty"`
	UpdatedBy   *uint        `json:"updated_by,omitempty"`
	CreatedAt   time.Time    `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time    `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AIPromptTemplate) TableName() string {
	return "ai_prompt_templates"
}
