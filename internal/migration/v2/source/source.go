package source

import (
	"context"
	"io"
)

// SourceRepository represents a repository from a migration source.
type SourceRepository struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Type   string `json:"type"` // hosted / proxy / group (source-native types)
	URL    string `json:"url"`
}

// SourceRepositoryDetail contains full configuration for a source repository.
type SourceRepositoryDetail struct {
	Name    string               `json:"name"`
	Format  string               `json:"format"`
	Type    string               `json:"type"`
	URL     string               `json:"url"`
	Proxy   *SourceProxyConfig   `json:"proxy"`
	Storage *SourceStorageConfig `json:"storage"`
	HTTP    *SourceHTTPConfig    `json:"http"`
	Group   *SourceGroupConfig   `json:"group"`
}

// SourceGroupConfig contains group repository member info.
type SourceGroupConfig struct {
	MemberNames []string `json:"member_names"`
}

type SourceProxyConfig struct {
	RemoteURL string `json:"remote_url"`
}

type SourceStorageConfig struct {
	BlobStoreName string `json:"blob_store_name"`
}

type SourceHTTPConfig struct {
	Connection     *SourceConnectionConfig `json:"connection"`
	Authentication *SourceAuthConfig       `json:"authentication"`
}

type SourceConnectionConfig struct {
	Timeout      int `json:"timeout"`
	MaxRedirects int `json:"max_redirects"`
}

type SourceAuthConfig struct {
	Type       string `json:"type"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	NtlmHost   string `json:"ntlm_host,omitempty"`
	NtlmDomain string `json:"ntlm_domain,omitempty"`
}

// SourceRole represents a role from a migration source.
type SourceRole struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Privileges  []string `json:"privileges"`
	Roles       []string `json:"roles"`
	External    bool     `json:"external"`
}

// SourcePrivilege represents a privilege from a migration source.
type SourcePrivilege struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Actions     string `json:"actions,omitempty"`
	Repository  string `json:"repository,omitempty"`
	Format      string `json:"format,omitempty"`
}

// SourceUser represents a user from a migration source.
type SourceUser struct {
	UserID     string   `json:"user_id"`
	FirstName  string   `json:"first_name"`
	LastName   string   `json:"last_name"`
	Email      string   `json:"email"`
	Status     string   `json:"status"`
	Roles      []string `json:"roles"`
	External   bool     `json:"external"`
	ExternalID string   `json:"external_id,omitempty"`
}

// SourceComponent represents a component from a migration source.
type SourceComponent struct {
	ID         string        `json:"id"`
	Repository string        `json:"repository"`
	Format     string        `json:"format"`
	Group      string        `json:"group"`
	Name       string        `json:"name"`
	Version    string        `json:"version"`
	Assets     []SourceAsset `json:"assets"`
}

// SourceAsset represents an asset within a component.
// Nexus 3 API uses camelCase field names.
type SourceAsset struct {
	DownloadURL string            `json:"downloadUrl"`
	Path        string            `json:"path"`
	Checksum    map[string]string `json:"checksum"`
	ContentType string            `json:"contentType"`
	FileSize    int64             `json:"fileSize"`
}

// SourceComponentPage is a paginated response of components.
type SourceComponentPage struct {
	Items             []SourceComponent `json:"items"`
	ContinuationToken string            `json:"continuation_token"`
}

// AssetStream wraps an asset reader with metadata.
type AssetStream struct {
	Reader      io.ReadCloser
	ContentType string
	Size        int64
}

// MigrationSource defines the interface for interacting with a migration source.
type MigrationSource interface {
	TestConnection(ctx context.Context) error
	ListRepositories(ctx context.Context) ([]SourceRepository, error)
	GetRepositoryDetail(ctx context.Context, format, repoType, name string) (*SourceRepositoryDetail, error)
	ListRoles(ctx context.Context) ([]SourceRole, error)
	ListPrivileges(ctx context.Context) ([]SourcePrivilege, error)
	ListUsers(ctx context.Context) ([]SourceUser, error)
	ListComponentsPage(ctx context.Context, repo string, cursor string) (SourceComponentPage, error)
	DownloadAsset(ctx context.Context, assetURL string) (AssetStream, error)
}
