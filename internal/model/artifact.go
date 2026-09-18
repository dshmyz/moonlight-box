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

// JSONB JSON 对象列（SQLite TEXT / MySQL JSON / PostgreSQL JSON）。
// 列类型统一用 type:json：jsonb 仅 PostgreSQL 认识，MySQL AutoMigrate 会生成非法 DDL。
type JSONB map[string]interface{}

// Value 实现 driver.Valuer 接口
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现 sql.Scanner 接口。
// []byte 来自 gorm 写入的 BLOB 存储类；string 来自原始 SQL 写入的 TEXT 存储类
// （历史数据/外部工具写入的行），两者都必须接受，否则这类行读出来直接报 Scan error。
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, j)
	case string:
		return json.Unmarshal([]byte(v), j)
	default:
		return errors.New("unexpected type for JSONB, expecting []byte or string")
	}
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
	ID           uint   `gorm:"primaryKey" json:"id"`
	RepositoryID uint   `gorm:"not null;index:idx_artifacts_repo;index:idx_artifacts_repo_format_remote_path,priority:1;index:idx_artifacts_repo_format_name,priority:1;index:idx_artifacts_repo_format_name_version,priority:1;index:idx_artifacts_repo_format_filename,priority:1;index:idx_artifacts_repo_format_kind_name_version,priority:1;uniqueIndex:idx_artifact_identity,priority:1" json:"repository_id"`
	Format       string `gorm:"not null;size:64;index:idx_artifact_format;index:idx_artifacts_repo_format_remote_path,priority:2;index:idx_artifacts_repo_format_name,priority:2;index:idx_artifacts_repo_format_name_version,priority:2;index:idx_artifacts_repo_format_filename,priority:2;index:idx_artifacts_repo_format_kind_name_version,priority:2" json:"format"`
	Kind         string `gorm:"size:64;index:idx_artifacts_repo_format_kind_name_version,priority:3" json:"kind,omitempty"`
	// IdentityKey 1024 字符全列进唯一索引会超 MySQL utf8mb4 3072 字节键长上限
	// （8 + 1024×4 = 4104 > 3072），加 760 字符前缀（8+3040=3048 ≤ 3072）。
	// SQLite/PG 忽略 length 前缀，唯一性仍按全列生效。
	IdentityKey string `gorm:"not null;size:1024;uniqueIndex:idx_artifact_identity,priority:2,length:760" json:"identity_key"`
	// Name 进 name_version 复合索引的部分加 384 前缀：8+256+1536(+1020) ≤ 3072。
	Name      string `gorm:"size:512;index:idx_artifact_name;index:idx_artifacts_repo_format_name,priority:3;index:idx_artifacts_repo_format_name_version,priority:3,length:384;index:idx_artifacts_repo_format_kind_name_version,priority:4,length:384" json:"name,omitempty"`
	Namespace string `gorm:"size:512;index:idx_artifact_namespace" json:"namespace,omitempty"`
	// Version 进 kind_name_version 复合索引的部分加 191 前缀：8+256+256+1536+764=2820 ≤ 3072。
	Version string `gorm:"size:255;index:idx_artifact_version;index:idx_artifacts_repo_format_name_version,priority:4;index:idx_artifacts_repo_format_kind_name_version,priority:5,length:191" json:"version,omitempty"`
	// Path 逻辑分组路径，不含文件名，如 "left-pad/-"、"com/google/guava/guava"
	Path     string `gorm:"type:text" json:"path,omitempty"`
	Filename string `gorm:"size:1024;index:idx_artifact_filename,length:512;index:idx_artifacts_repo_format_filename,priority:3,length:512" json:"filename,omitempty"`
	// RemotePath 仓库内的相对路径（含文件名），用于回源定位、存储寻址和构造下载 URL。
	// 格式：协议相关的相对路径，如 "left-pad/-/left-pad-1.0.0.tgz"、"packages/ab/cd/requests-2.28.0.tar.gz"
	// 用途：ProxyRuntime 回源时拼接完整远端 URL；存储层寻址（file/{RemotePath}）；前端下载链接构造（/repository/{repoName}/{RemotePath}）
	RemotePath string `gorm:"type:varchar(1024);index:idx_artifacts_repo_format_remote_path,priority:3,length:512" json:"remote_path,omitempty"`
	// DownloadURL 远端文件的绝对 URL，仅用于后端 ProxyRuntime 服务端回源拉取。
	// 格式：完整的 HTTP(S) URL，如 "https://files.pythonhosted.org/packages/ab/cd/requests-2.28.0.tar.gz"
	// 注意：此字段绝不暴露给前端作为下载链接（会导致 CORS 跨域问题），前端下载统一走 RemotePath 构造的本地路径
	DownloadURL string    `gorm:"type:text" json:"download_url,omitempty"`
	Extension   string    `gorm:"size:64" json:"extension,omitempty"`
	ContentType string    `gorm:"size:255" json:"content_type,omitempty"`
	SizeBytes   int64     `gorm:"not null;default:0" json:"size_bytes"`
	Checksums   JSONB     `gorm:"type:json" json:"checksums,omitempty"`
	Qualifiers  JSONB     `gorm:"type:json" json:"qualifiers,omitempty"`
	Attributes  JSONB     `gorm:"type:json" json:"attributes,omitempty"`
	Metadata    JSONB     `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime;not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime;not null" json:"updated_at"`
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
	if a.Extension == "" && a.Filename != "" {
		a.Extension = path.Ext(a.Filename)
	}
	if a.RemotePath != "" {
		a.Metadata["remote_path"] = a.RemotePath
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
