package domain

import (
	"encoding/json"
	"time"

	"github.com/dshmyz/moonlight-box/internal/util"
)

// Plan statuses
type PlanStatus string

const (
	PlanDraft          PlanStatus = "draft"
	PlanScanning       PlanStatus = "scanning"
	PlanPrechecking    PlanStatus = "prechecking"
	PlanPrecheckFailed PlanStatus = "precheck_failed"
	PlanReady          PlanStatus = "ready"
	PlanRunning        PlanStatus = "running"
	PlanPaused         PlanStatus = "paused"
	PlanVerifying      PlanStatus = "verifying"
	PlanCompleted      PlanStatus = "completed"
	PlanFailed         PlanStatus = "failed"
	PlanCancelling     PlanStatus = "cancelling"
	PlanCancelled      PlanStatus = "cancelled"
)

// Plan stages
type PlanStage string

const (
	StageScan     PlanStage = "scan"
	StagePrecheck PlanStage = "precheck"
	StageExecute  PlanStage = "execute"
	StageVerify   PlanStage = "verify"
	StageDone     PlanStage = "done"
)

// Job kinds
type JobKind string

const (
	JobRepoConfig       JobKind = "repo_config"
	JobGroupMembership  JobKind = "group_membership"
	JobPermission       JobKind = "permission"
	JobRole             JobKind = "role"
	JobUser             JobKind = "user"
	JobArtifactScan     JobKind = "artifact_scan"
	JobArtifactCopy     JobKind = "artifact_copy"
	JobVerify           JobKind = "verify"
)

// Job statuses
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobBlocked   JobStatus = "blocked"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobSkipped   JobStatus = "skipped"
	JobCancelled JobStatus = "cancelled"
)

// Item kinds
type ItemKind string

const (
	ItemArtifact ItemKind = "artifact"
	ItemAsset    ItemKind = "asset"
)

// Item statuses
type ItemStatus string

const (
	ItemPending   ItemStatus = "pending"
	ItemRunning   ItemStatus = "running"
	ItemCompleted ItemStatus = "completed"
	ItemFailed    ItemStatus = "failed"
	ItemSkipped   ItemStatus = "skipped"
)

// Conflict severities
type ConflictSeverity string

const (
	SeverityWarning  ConflictSeverity = "warning"
	SeverityBlocking ConflictSeverity = "blocking"
)

// ConflictResolution represents a user's resolution for a conflict.
type ConflictResolution struct {
	ConflictID uint          `json:"conflict_id"`
	Policy     ConflictPolicy `json:"policy"`
}

// Conflict kinds
type ConflictKind string

const (
	ConflictRepoExists              ConflictKind = "repo_exists"
	ConflictEmailExists             ConflictKind = "email_exists"
	ConflictRoleExists              ConflictKind = "role_exists"
	ConflictMissingGroupMember      ConflictKind = "missing_group_member"
	ConflictSourceDetailUnavailable ConflictKind = "source_detail_unavailable"
	ConflictTargetRepoMissing       ConflictKind = "target_repository_missing"
	ConflictStorageBackendMissing   ConflictKind = "storage_backend_missing"
	ConflictArtifactTargetConflict  ConflictKind = "artifact_target_conflict"
)

// Conflict policies
type ConflictPolicy string

const (
	PolicySkip          ConflictPolicy = "skip"
	PolicyMapExisting   ConflictPolicy = "map_existing"
	PolicyRename        ConflictPolicy = "rename"
	PolicyUseFallback   ConflictPolicy = "use_fallback"
	PolicyCreateMissing ConflictPolicy = "create_missing"
	PolicyFail          ConflictPolicy = "fail"
)

// Error codes
type ErrorCode string

const (
	ErrSourceUnavailable              ErrorCode = "SOURCE_UNAVAILABLE"
	ErrSourceAuthFailed               ErrorCode = "SOURCE_AUTH_FAILED"
	ErrSourceRepoDetailUnavailable    ErrorCode = "SOURCE_REPOSITORY_DETAIL_UNAVAILABLE"
	ErrSourceComponentPageFailed      ErrorCode = "SOURCE_COMPONENT_PAGE_FAILED"
	ErrTargetRepoExists               ErrorCode = "TARGET_REPOSITORY_EXISTS"
	ErrTargetEmailExists              ErrorCode = "TARGET_EMAIL_EXISTS"
	ErrTargetRoleExists               ErrorCode = "TARGET_ROLE_EXISTS"
	ErrTargetGroupMemberMissing       ErrorCode = "TARGET_GROUP_MEMBER_MISSING"
	ErrTargetStorageBackendMissing    ErrorCode = "TARGET_STORAGE_BACKEND_MISSING"
	ErrArtifactDownloadFailed         ErrorCode = "ARTIFACT_DOWNLOAD_FAILED"
	ErrArtifactChecksumMismatch       ErrorCode = "ARTIFACT_CHECKSUM_MISMATCH"
	ErrArtifactStorageFailed          ErrorCode = "ARTIFACT_STORAGE_FAILED"
	ErrJobCancelled                   ErrorCode = "JOB_CANCELLED"
	ErrJobRetryExhausted              ErrorCode = "JOB_RETRY_EXHAUSTED"
)

// Event levels
type EventLevel string

const (
	LevelInfo  EventLevel = "info"
	LevelWarn  EventLevel = "warn"
	LevelError EventLevel = "error"
)

// Event types
type EventType string

const (
	EventStatusChanged  EventType = "status_changed"
	EventConflictFound  EventType = "conflict_found"
	EventPolicyApplied  EventType = "policy_applied"
	EventRetryScheduled EventType = "retry_scheduled"
	EventItemCompleted  EventType = "item_completed"
	EventSourceWarning  EventType = "source_warning"
	EventJobFailed      EventType = "job_failed"
	EventPlanRecovered  EventType = "plan_recovered"
)

// Scope selection
type ScopeSelection struct {
	RepoConfig       bool     `json:"repo_config"`
	HostedRepos      bool     `json:"hosted_repos"`
	ProxyRepos       bool     `json:"proxy_repos"`
	GroupRepos       bool     `json:"group_repos"`
	GroupMemberships bool     `json:"group_memberships"`
	Privileges       bool     `json:"privileges"`
	Roles            bool     `json:"roles"`
	Users            bool     `json:"users"`
	UserRoles        bool     `json:"user_roles"`
	Artifacts        bool     `json:"artifacts"`
	ArtifactRepos    []string `json:"artifact_repos"`
	TargetStrategy   string   `json:"target_strategy"` // "keep_structure" or specific repo name
	TargetRepoID     uint     `json:"target_repo_id"`
	TargetRepoName   string   `json:"target_repo_name"`
}

// Stats summary
type PlanStats struct {
	TotalRepos      int `json:"total_repos"`
	SyncedRepos     int `json:"synced_repos"`
	SkippedRepos    int `json:"skipped_repos"`
	FailedRepos     int `json:"failed_repos"`
	TotalUsers      int `json:"total_users"`
	SyncedUsers     int `json:"synced_users"`
	SkippedUsers    int `json:"skipped_users"`
	FailedUsers     int `json:"failed_users"`
	TotalRoles      int `json:"total_roles"`
	SyncedRoles     int `json:"synced_roles"`
	SkippedRoles    int `json:"skipped_roles"`
	TotalArtifacts  int `json:"total_artifacts"`
	SyncedArtifacts int `json:"synced_artifacts"`
	SkippedArtifacts int `json:"skipped_artifacts"`
	FailedArtifacts int `json:"failed_artifacts"`
}

var encryptionKey = []byte("moonlight-box-registry-32bytes!!")

// MigrationPlan
type MigrationPlan struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	Name            string         `json:"name" gorm:"size:200"`
	SourceType      string         `json:"source_type" gorm:"size:50;default:nexus"`
	SourceURL       string         `json:"source_url" gorm:"size:500"`
	Username        string         `json:"username" gorm:"size:100"`
	PasswordEnc     string         `json:"-" gorm:"column:password_encrypted;type:text"`
	Status          PlanStatus     `json:"status" gorm:"size:20;default:draft;index"`
	CurrentStage    PlanStage      `json:"current_stage" gorm:"size:20"`
	ScopeJSON       string         `json:"-" gorm:"column:selected_scope_json;type:text"`
	PolicyJSON      string         `json:"-" gorm:"column:conflict_policy_json;type:text"`
	StatsJSON       string         `json:"-" gorm:"column:stats_json;type:text"`
	Stats           *PlanStats     `json:"stats,omitempty" gorm:"-"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	StartedAt       *time.Time     `json:"started_at"`
	CompletedAt     *time.Time     `json:"completed_at"`
}

func (MigrationPlan) TableName() string { return "migration_v2_plans" }

func (p *MigrationPlan) SetPassword(password string) error {
	if password == "" {
		p.PasswordEnc = ""
		return nil
	}
	enc, err := util.EncryptString(password, encryptionKey)
	if err != nil {
		return err
	}
	p.PasswordEnc = enc
	return nil
}

func (p *MigrationPlan) GetPassword() (string, error) {
	if p.PasswordEnc == "" {
		return "", nil
	}
	return util.DecryptString(p.PasswordEnc, encryptionKey)
}

func (p *MigrationPlan) SetScope(scope *ScopeSelection) error {
	b, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	p.ScopeJSON = string(b)
	return nil
}

func (p *MigrationPlan) GetScope() (*ScopeSelection, error) {
	if p.ScopeJSON == "" {
		return &ScopeSelection{}, nil
	}
	var s ScopeSelection
	if err := json.Unmarshal([]byte(p.ScopeJSON), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (p *MigrationPlan) SetStats(stats *PlanStats) error {
	b, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	p.StatsJSON = string(b)
	return nil
}

func (p *MigrationPlan) GetStats() (*PlanStats, error) {
	if p.StatsJSON == "" {
		return &PlanStats{}, nil
	}
	var s PlanStats
	if err := json.Unmarshal([]byte(p.StatsJSON), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// MigrationJob
type MigrationJob struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	PlanID       uint      `json:"plan_id" gorm:"index:idx_job_plan_status"`
	Kind         JobKind   `json:"kind" gorm:"size:30"`
	Status       JobStatus `json:"status" gorm:"size:20;index:idx_job_plan_status"`
	SourceKey    string    `json:"source_key" gorm:"size:500"`
	TargetKey    string    `json:"target_key" gorm:"size:500"`
	DependsOn    string    `json:"-" gorm:"column:depends_on_json;type:text"`
	AttemptCount int       `json:"attempt_count" gorm:"default:0"`
	MaxAttempts  int       `json:"max_attempts" gorm:"default:3"`
	Checkpoint   string    `json:"-" gorm:"column:checkpoint_json;type:text"`
	ErrorCode    ErrorCode `json:"error_code" gorm:"size:50"`
	ErrorMessage string    `json:"error_message" gorm:"type:text"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
}

func (MigrationJob) TableName() string { return "migration_v2_jobs" }

// UserCheckpoint stores user information during the scanning phase
// so the executor can create users without re-fetching from the source.
type UserCheckpoint struct {
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Roles       []string `json:"roles,omitempty"`
}

// RepoCheckpoint stores repository configuration during the scanning phase
// so the executor can create repositories without re-fetching from the source.
type RepoCheckpoint struct {
	Type       string `json:"type"`
	Format     string `json:"format"`
	RemoteURL  string `json:"remote_url,omitempty"`
	IsVirtual  bool   `json:"is_virtual"`
}

// RoleCheckpoint stores role information during the scanning phase
// so the executor can create roles without re-fetching from the source.
type RoleCheckpoint struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Privileges  []string `json:"privileges,omitempty"`
}

// MigrationItem
type MigrationItem struct {
	ID               uint       `json:"id" gorm:"primaryKey"`
	PlanID           uint       `json:"plan_id" gorm:"index:idx_item_plan_status"`
	JobID            uint       `json:"job_id" gorm:"index"`
	Kind             ItemKind   `json:"kind" gorm:"size:20"`
	SourceRepository string     `json:"source_repository" gorm:"size:200"`
	SourceID         string     `json:"source_id" gorm:"size:200"`
	SourcePath       string     `json:"source_path" gorm:"size:1000"`
	SourceFormat     string     `json:"source_format" gorm:"size:50"`
	SourceName       string     `json:"source_name" gorm:"size:500"`
	SourceVersion    string     `json:"source_version" gorm:"size:100"`
	TargetRepository string     `json:"target_repository" gorm:"size:200"`
	TargetPath       string     `json:"target_path" gorm:"size:1000"`
	Status           ItemStatus `json:"status" gorm:"size:20;index:idx_item_plan_status"`
	ChecksumJSON     string     `json:"-" gorm:"column:checksum_json;type:text"`
	SizeBytes        int64      `json:"size_bytes" gorm:"default:0"`
	AttemptCount     int        `json:"attempt_count" gorm:"default:0"`
	ErrorCode        ErrorCode  `json:"error_code" gorm:"size:50"`
	ErrorMessage     string     `json:"error_message" gorm:"type:text"`
	Checkpoint       string     `json:"-" gorm:"column:checkpoint_json;type:text"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CompletedAt      *time.Time `json:"completed_at"`
}

func (MigrationItem) TableName() string { return "migration_v2_items" }

// MigrationConflict
type MigrationConflict struct {
	ID              uint            `json:"id" gorm:"primaryKey"`
	PlanID          uint            `json:"plan_id" gorm:"index"`
	Kind            ConflictKind    `json:"kind" gorm:"size:50"`
	Severity        ConflictSeverity `json:"severity" gorm:"size:20"`
	SourceKey       string          `json:"source_key" gorm:"size:500"`
	TargetKey       string          `json:"target_key" gorm:"size:500"`
	Message         string          `json:"message" gorm:"type:text"`
	SuggestedPolicy ConflictPolicy  `json:"suggested_policy" gorm:"size:30"`
	SelectedPolicy  *ConflictPolicy `json:"selected_policy" gorm:"size:30"`
	PayloadJSON     string          `json:"-" gorm:"column:payload_json;type:text"`
	ResolvedAt      *time.Time      `json:"resolved_at"`
	CreatedAt       time.Time       `json:"created_at"`
}

func (MigrationConflict) TableName() string { return "migration_v2_conflicts" }

// MigrationEvent
type MigrationEvent struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	PlanID      uint      `json:"plan_id" gorm:"index"`
	JobID       *uint     `json:"job_id"`
	ItemID      *uint     `json:"item_id"`
	Level       EventLevel `json:"level" gorm:"size:10"`
	EventType   EventType  `json:"event_type" gorm:"size:30"`
	Message     string    `json:"message" gorm:"type:text"`
	PayloadJSON string    `json:"-" gorm:"column:payload_json;type:text"`
	CreatedAt   time.Time `json:"created_at"`
}

func (MigrationEvent) TableName() string { return "migration_v2_events" }
