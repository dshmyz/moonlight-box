package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"
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
	RepositoryID uint      `gorm:"not null;index:idx_artifacts_repo;uniqueIndex:idx_artifact_identity,priority:1" json:"repository_id"`
	Format       string    `gorm:"not null;size:64" json:"format"`
	Kind         string    `gorm:"size:64" json:"kind,omitempty"`
	IdentityKey  string    `gorm:"not null;size:1024;uniqueIndex:idx_artifact_identity,priority:2" json:"identity_key"`
	Name         string    `gorm:"size:512;index:idx_artifact_name" json:"name,omitempty"`
	Namespace    string    `gorm:"size:512;index:idx_artifact_namespace" json:"namespace,omitempty"`
	Version      string    `gorm:"size:255;index:idx_artifact_version" json:"version,omitempty"`
	Path         string    `gorm:"type:text" json:"path,omitempty"`
	Filename     string    `gorm:"size:1024;index:idx_artifact_filename" json:"filename,omitempty"`
	RemotePath   string    `gorm:"type:text" json:"remote_path,omitempty"`
	DownloadPath string    `gorm:"type:text" json:"download_path,omitempty"`
	DownloadURL  string    `gorm:"type:text" json:"download_url,omitempty"`
	Extension    string    `gorm:"size:64" json:"extension,omitempty"`
	ContentType  string    `gorm:"size:255" json:"content_type,omitempty"`
	SizeBytes    int64     `gorm:"not null;default:0" json:"size_bytes"`
	Checksums    JSONB     `gorm:"type:jsonb" json:"checksums,omitempty"`
	Qualifiers   JSONB     `gorm:"type:jsonb" json:"qualifiers,omitempty"`
	Attributes   JSONB     `gorm:"type:jsonb" json:"attributes,omitempty"`
	Metadata     JSONB     `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime;not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime;not null" json:"updated_at"`
}

func (Artifact) TableName() string {
	return "artifacts"
}

func (a *Artifact) BeforeSave(tx *gorm.DB) error {
	if a.Metadata == nil {
		a.Metadata = JSONB{}
	}
	a.Path = cleanSlashPath(a.Path)
	a.RemotePath = cleanSlashPath(a.RemotePath)
	a.DownloadPath = cleanSlashPath(a.DownloadPath)
	if a.RemotePath != "" {
		if a.Filename == "" {
			a.Filename = path.Base(a.RemotePath)
		}
		if a.Path == "" {
			dir := path.Dir(a.RemotePath)
			if dir != "." {
				a.Path = dir
			}
		}
	}
	if a.RemotePath == "" && a.Path != "" && a.Filename != "" {
		a.RemotePath = joinSlashPath(a.Path, a.Filename)
	}
	if a.DownloadPath == "" {
		a.DownloadPath = a.RemotePath
	}
	if a.DownloadPath == "" && a.Path != "" && a.Filename != "" {
		a.DownloadPath = joinSlashPath(a.Path, a.Filename)
	}
	if a.Extension == "" && a.Filename != "" {
		a.Extension = path.Ext(a.Filename)
	}
	if a.RemotePath != "" {
		a.Metadata["remote_path"] = a.RemotePath
	}
	if a.DownloadPath != "" {
		a.Metadata["download_path"] = a.DownloadPath
	}
	if a.DownloadURL != "" {
		a.Metadata["download_url"] = a.DownloadURL
	}
	if a.IdentityKey == "" {
		a.IdentityKey = artifactIdentityKey(a)
	}
	return nil
}

func artifactIdentityKey(a *Artifact) string {
	switch a.Kind {
	case "package":
		return "package/" + a.Name
	case "version":
		return "version/" + a.Name + "/" + a.Version
	case "metadata":
		if a.RemotePath != "" {
			return "metadata/" + a.RemotePath
		}
	case "checksum":
		if a.RemotePath != "" {
			return "checksum/" + a.RemotePath
		}
	}
	if a.RemotePath != "" {
		return "file/" + a.RemotePath
	}
	if a.Name != "" || a.Version != "" || a.Path != "" || a.Filename != "" {
		return "artifact/" + a.Name + "/" + a.Version + "/" + joinSlashPath(a.Path, a.Filename)
	}
	return "artifact/" + a.Format + "/" + a.Kind
}

func cleanSlashPath(value string) string {
	return strings.Trim(strings.ReplaceAll(value, "\\", "/"), "/")
}

func joinSlashPath(dir, file string) string {
	dir = cleanSlashPath(dir)
	file = strings.Trim(file, "/")
	if dir == "" {
		return file
	}
	if file == "" {
		return dir
	}
	return dir + "/" + file
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
