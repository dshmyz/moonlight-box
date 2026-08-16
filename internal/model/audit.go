package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type AuditAction string

const (
	ActionLogin               AuditAction = "login"
	ActionLogout              AuditAction = "logout"
	ActionPackageUpload       AuditAction = "package_upload"
	ActionPackageDownload     AuditAction = "package_download"
	ActionPackageDelete       AuditAction = "package_delete"
	ActionUserCreate          AuditAction = "user_create"
	ActionUserUpdate          AuditAction = "user_update"
	ActionUserDelete          AuditAction = "user_delete"
	ActionRoleAssign          AuditAction = "role_assign"
	ActionConfigChange        AuditAction = "config_change"
	ActionBlock               AuditAction = "block"
	ActionConditionUnverified AuditAction = "condition_unverified"
	ActionAIToolCall          AuditAction = "ai_tool_call"
	ActionAIPromptChange      AuditAction = "ai_prompt_change"
)

type AuditLog struct {
	ID             uint        `gorm:"primaryKey" json:"id"`
	UserID         *uint       `json:"user_id,omitempty"`
	Action         AuditAction `gorm:"not null;index" json:"action"`
	ResourceType   string      `gorm:"size:50;index" json:"resource_type,omitempty"`
	ResourceID     *uint       `json:"resource_id,omitempty"`
	ResourceName   string      `gorm:"size:200" json:"resource_name,omitempty"`
	IPAddress      string      `gorm:"size:45" json:"ip_address,omitempty"`
	UserAgent      string      `gorm:"size:500" json:"user_agent,omitempty"`
	RequestID      string      `gorm:"size:36" json:"request_id,omitempty"`
	ResponseStatus int         `json:"response_status,omitempty"`
	Details        string      `gorm:"type:text" json:"details,omitempty"`
	DurationMs     int         `json:"duration_ms,omitempty"`
	// ToolName 等字段用于 AI 工具调用审计（ActionAIToolCall）。
	ToolName   string `gorm:"size:100;index" json:"tool_name,omitempty"`
	ToolParams string `gorm:"type:text" json:"tool_params,omitempty"`
	ToolResult string `gorm:"type:text" json:"tool_result,omitempty"`
	ToolError  string `gorm:"type:text" json:"tool_error,omitempty"`
	// PrevHash 为上一行日志的 LogHash（哈希链），用于防篡改校验。
	PrevHash  string    `gorm:"size:64;index" json:"prev_hash,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// LogHash 计算当前日志的哈希。哈希链：每行日志记录上一行的 LogHash 于 PrevHash，
// 篡改任一行都会导致后续所有行校验失败。
// 覆盖 AI 审计最关键的字段：Details（AI 工具日志存用户名、提示词治理日志存变更描述）与
// ResponseStatus（工具成败标志）必须纳入哈希，否则攻击者可改写操作人或把失败改成成功而不破坏链。
// 注意：指针字段必须显式解引用（fmt 对 %d 打印指针地址），保证哈希确定可复现。
func (l *AuditLog) LogHash() string {
	hasher := sha256.New()
	fmt.Fprintf(hasher, "%d|%s|%d|%s|%s|%s|%s|%s|%s|%d|%d|%s",
		derefUint(l.UserID), l.Action, derefUint(l.ResourceID), l.ResourceName,
		l.Details, l.ToolName, l.ToolParams, l.ToolResult, l.ToolError,
		l.DurationMs, l.ResponseStatus, l.CreatedAt.Format(time.RFC3339Nano))
	return hex.EncodeToString(hasher.Sum(nil))
}

func derefUint(v *uint) uint {
	if v == nil {
		return 0
	}
	return *v
}

// AIToolActions 是参与 AI 审计哈希链的动作集合（工具调用 + 提示词治理变更）。
// 只有这两类日志带 PrevHash 入链；普通 HTTP 审计行（login/package_download 等）不参与哈希链。
var AIToolActions = []AuditAction{ActionAIToolCall, ActionAIPromptChange}

// VerifyAuditChain 从 earliestID 开始逐行校验 AI 哈希链是否完整（未篡改）。
//
// 只遍历 AI 审计行（AIToolActions），普通 HTTP 审计行混在同一张表里但无 PrevHash，
// 必须排除，否则任何交错行都会导致误报"链校验失败"。
//
// 第一条 AI 行是链头：其 PrevHash 可能为空（自然链头），也可能指向已被保留策略
// （CleanOldAIAndToolLogs）合法裁剪掉的旧行——因此链头的 PrevHash 不参与校验，
// 从第二行起才要求 PrevHash == 上一行 LogHash。若校验从链头开始即因"指向已删行"误报，
// 保留策略清理就会制造假篡改。
//
// 返回被篡改的日志 ID 列表，nil 表示链路完整；DB 查询失败时返回 error。
func VerifyAuditChain(db *gorm.DB, earliestID uint) ([]uint, error) {
	var logs []AuditLog
	if err := db.
		Where("action IN ? AND id >= ?", AIToolActions, earliestID).
		Order("id ASC").
		Find(&logs).Error; err != nil {
		return nil, err
	}
	var tampered []uint
	var prevHash string
	for i := range logs {
		l := &logs[i]
		if i > 0 && l.PrevHash != prevHash {
			tampered = append(tampered, l.ID)
			// 链断裂后无法继续校验，返回已发现的问题
			break
		}
		prevHash = l.LogHash()
	}
	return tampered, nil
}

type CacheEntry struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	RemoteURL      string     `gorm:"uniqueIndex;not null" json:"remote_url"`
	LocalKey       string     `gorm:"not null" json:"local_key"`
	PackageType    string     `gorm:"not null;index" json:"package_type"`
	ETag           string     `json:"etag,omitempty"`
	LastModified   string     `json:"last_modified,omitempty"`
	ContentType    string     `json:"content_type,omitempty"`
	CachedAt       time.Time  `gorm:"autoCreateTime" json:"cached_at"`
	ExpiresAt      time.Time  `gorm:"index" json:"expires_at"`
	AccessCount    int64      `gorm:"default:0" json:"access_count"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	SizeBytes      int64      `gorm:"default:0" json:"size_bytes"`
	HitCount       int64      `gorm:"default:0" json:"hit_count"`
	MissCount      int64      `gorm:"default:0" json:"miss_count"`
}

type SystemConfig struct {
	Key         string    `gorm:"primaryKey" json:"key"`
	Value       string    `gorm:"not null" json:"value"`
	ValueType   string    `gorm:"default:string" json:"value_type"`
	Category    string    `gorm:"size:50;index" json:"category"`
	Description string    `gorm:"size:500" json:"description,omitempty"`
	IsSensitive bool      `gorm:"default:false" json:"is_sensitive"`
	UpdatedBy   *uint     `json:"updated_by,omitempty"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type CASConfig struct {
	Enabled      bool   `json:"enabled"`
	ServerURL    string `json:"server_url"`
	ServiceURL   string `json:"service_url"`
	LoginPath    string `json:"login_path"`
	ValidatePath string `json:"validate_path"`
}
