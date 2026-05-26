package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// JSONB 用于 PostgreSQL JSONB 类型
type JSONB map[string]interface{}

// Value 实现 driver.Valuer 接口
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现 sql.Scanner 接口
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}

// RepositoryMember 仓库成员关系（用于虚拟仓库）
type RepositoryMember struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	RepositoryID uint      `json:"repository_id" gorm:"not null;uniqueIndex:idx_repo_member,priority:1"`
	MemberID     uint      `json:"member_id" gorm:"not null;uniqueIndex:idx_repo_member,priority:2"`
	Position     int       `json:"position" gorm:"not null;default:0"`
	CreatedAt    time.Time `json:"created_at"`

	VirtualRepo Repository `json:"virtual_repo,omitempty" gorm:"foreignKey:RepositoryID"`
	MemberRepo  Repository `json:"member_repo,omitempty" gorm:"foreignKey:MemberID"`
}

func (RepositoryMember) TableName() string {
	return "repository_members"
}

// Blob CAS 存储的 blob 元数据
type Blob struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Algorithm   string    `gorm:"not null;size:32;uniqueIndex:idx_blob_digest,priority:1" json:"algorithm"`
	Digest      string    `gorm:"not null;size:128;uniqueIndex:idx_blob_digest,priority:2" json:"digest"`
	Size        int64     `gorm:"not null" json:"size"`
	StoragePath string    `gorm:"not null;type:text" json:"storage_path"`
	CreatedAt   time.Time `gorm:"autoCreateTime;not null" json:"created_at"`
}

func (Blob) TableName() string {
	return "blobs"
}

// Artifact 制品元数据
type Artifact struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RepositoryID uint      `gorm:"not null;index:idx_artifacts_repo" json:"repository_id"`
	Format       string    `gorm:"not null;size:64" json:"format"`
	Kind         string    `gorm:"size:64" json:"kind,omitempty"`
	Coordinates  JSONB     `gorm:"not null;type:jsonb" json:"coordinates"`
	Metadata     JSONB     `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime;not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime;not null" json:"updated_at"`
}

func (Artifact) TableName() string {
	return "artifacts"
}

// ArtifactBlob 制品与 blob 的关联关系
type ArtifactBlob struct {
	ArtifactID uint   `gorm:"not null;uniqueIndex:idx_artifact_blob_pos,priority:1" json:"artifact_id"`
	BlobID     uint   `gorm:"not null;uniqueIndex:idx_artifact_blob_pos,priority:2" json:"blob_id"`
	Position   int    `gorm:"not null;uniqueIndex:idx_artifact_blob_pos,priority:3" json:"position"`
	Role       string `gorm:"size:64" json:"role,omitempty"`
}

func (ArtifactBlob) TableName() string {
	return "artifact_blobs"
}

// ArtifactTag 制品标签
type ArtifactTag struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ArtifactID uint      `gorm:"not null;index" json:"artifact_id"`
	Tag        string    `gorm:"not null;size:255;index:idx_artifact_tags" json:"tag"`
	CreatedAt  time.Time `gorm:"autoCreateTime;not null" json:"created_at"`
}

func (ArtifactTag) TableName() string {
	return "artifact_tags"
}

// ArtifactVersion 制品版本
type ArtifactVersion struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ArtifactID uint      `gorm:"not null;index" json:"artifact_id"`
	Version    string    `gorm:"not null;size:255" json:"version"`
	Normalized string    `gorm:"size:255" json:"normalized,omitempty"`
	CreatedAt  time.Time `gorm:"autoCreateTime;not null" json:"created_at"`
}

func (ArtifactVersion) TableName() string {
	return "artifact_versions"
}

// ArtifactProperty 制品属性
type ArtifactProperty struct {
	ArtifactID uint   `gorm:"not null;uniqueIndex:idx_artifact_prop,priority:1" json:"artifact_id"`
	Key        string `gorm:"not null;size:255;uniqueIndex:idx_artifact_prop,priority:2" json:"key"`
	Value      string `gorm:"type:text" json:"value,omitempty"`
}

func (ArtifactProperty) TableName() string {
	return "artifact_properties"
}

// ArtifactRelation 制品关系
type ArtifactRelation struct {
	SourceID     uint   `gorm:"not null;index:idx_artifact_source" json:"source_id"`
	TargetID     uint   `gorm:"not null;index:idx_artifact_target" json:"target_id"`
	RelationType string `gorm:"not null;size:64;index" json:"relation_type"`
}

func (ArtifactRelation) TableName() string {
	return "artifact_relations"
}
