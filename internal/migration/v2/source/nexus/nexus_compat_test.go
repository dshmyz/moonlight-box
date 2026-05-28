package nexus

import (
	"encoding/json"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
)

func TestNexusVersionParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected NexusVersion
	}{
		{"3.30.2", NexusVersion{3, 30, 2}},
		{"2.14.21", NexusVersion{2, 14, 21}},
		{"3.15.0", NexusVersion{3, 15, 0}},
		{"2.15.1-02", NexusVersion{2, 15, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var v NexusVersion
			parseVersionString(tt.input, &v)
			if v != tt.expected {
				t.Errorf("parseVersionString(%q) = %v, want %v", tt.input, v, tt.expected)
			}
		})
	}
}

func TestNexusVersionIsNexus3(t *testing.T) {
	tests := []struct {
		version NexusVersion
		want    bool
	}{
		{NexusVersion{3, 30, 2}, true},
		{NexusVersion{3, 0, 0}, true},
		{NexusVersion{2, 14, 21}, false},
		{NexusVersion{1, 0, 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.version.String(), func(t *testing.T) {
			if got := tt.version.IsNexus3(); got != tt.want {
				t.Errorf("IsNexus3() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNexusVersionIsNexus2(t *testing.T) {
	tests := []struct {
		version NexusVersion
		want    bool
	}{
		{NexusVersion{2, 14, 21}, true},
		{NexusVersion{2, 0, 0}, true},
		{NexusVersion{3, 30, 2}, false},
		{NexusVersion{1, 0, 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.version.String(), func(t *testing.T) {
			if got := tt.version.IsNexus2(); got != tt.want {
				t.Errorf("IsNexus2() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNexusVersionGreaterThanOrEqual(t *testing.T) {
	tests := []struct {
		version NexusVersion
		minor   int
		patch   int
		want    bool
	}{
		{NexusVersion{3, 30, 2}, 15, 0, true},
		{NexusVersion{3, 15, 0}, 15, 0, true},
		{NexusVersion{3, 15, 5}, 15, 0, true},
		{NexusVersion{3, 14, 0}, 15, 0, false},
		{NexusVersion{3, 15, 0}, 15, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.version.String(), func(t *testing.T) {
			if got := tt.version.GreaterThanOrEqual(tt.minor, tt.patch); got != tt.want {
				t.Errorf("GreaterThanOrEqual(%d, %d) = %v, want %v", tt.minor, tt.patch, got, tt.want)
			}
		})
	}
}

func TestParseVersionFromResponseV1(t *testing.T) {
	body := `{"version": "3.30.2"}`
	v := parseVersionFromResponse(body)
	if v.Major != 3 || v.Minor != 30 || v.Patch != 2 {
		t.Errorf("expected 3.30.2, got %v", v)
	}
}

func TestParseVersionFromResponseNexus2(t *testing.T) {
	body := `{"data": {"version": "2.14.21"}}`
	v := parseVersionFromResponse(body)
	if v.Major != 2 || v.Minor != 14 || v.Patch != 21 {
		t.Errorf("expected 2.14.21, got %v", v)
	}
}

func TestNormalizeNexus2Format(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"maven2", "maven2"},
		{"Maven2", "maven2"},
		{"MAVEN", "maven2"},
		{"npm", "npm"},
		{"PyPI", "pypi"},
		{"raw", "raw"},
		{"docker", "docker"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeNexus2Format(tt.input); got != tt.expected {
				t.Errorf("normalizeNexus2Format(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeNexus2Type(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"proxy", "proxy"},
		{"Proxy", "proxy"},
		{"PROXY", "proxy"},
		{"hosted", "hosted"},
		{"Hosted", "hosted"},
		{"group", "group"},
		{"GROUP", "group"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeNexus2Type(tt.input); got != tt.expected {
				t.Errorf("normalizeNexus2Type(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNexus2RepositoryDeserialization(t *testing.T) {
	body := `{
		"data": {
			"name": "maven-releases",
			"format": "maven2",
			"type": "hosted",
			"url": "http://localhost:8081/nexus/content/repositories/maven-releases"
		}
	}`

	var repo nexus2Repository
	if err := json.Unmarshal([]byte(body), &repo); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if repo.Data.Name != "maven-releases" {
		t.Errorf("expected name 'maven-releases', got %q", repo.Data.Name)
	}
	if repo.Data.Format != "maven2" {
		t.Errorf("expected format 'maven2', got %q", repo.Data.Format)
	}
	if repo.Data.Type != "hosted" {
		t.Errorf("expected type 'hosted', got %q", repo.Data.Type)
	}
}

func TestNexus2RepoDetailDeserialization(t *testing.T) {
	body := `{
		"data": {
			"name": "maven-central",
			"format": "maven2",
			"type": "proxy",
			"url": "http://localhost:8081/nexus/content/repositories/maven-central",
			"proxy": {
				"remoteStorageUrl": "https://repo1.maven.org/maven2/"
			}
		}
	}`

	var detail nexus2RepoDetail
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if detail.Data.Name != "maven-central" {
		t.Errorf("expected name 'maven-central', got %q", detail.Data.Name)
	}
	if detail.Data.Proxy == nil {
		t.Fatal("expected proxy config, got nil")
	}
	if detail.Data.Proxy.RemoteURL != "https://repo1.maven.org/maven2/" {
		t.Errorf("expected remoteUrl 'https://repo1.maven.org/maven2/', got %q", detail.Data.Proxy.RemoteURL)
	}
}

func TestNexus2GroupRepoDetailDeserialization(t *testing.T) {
	body := `{
		"data": {
			"name": "maven-public",
			"format": "maven2",
			"type": "group",
			"url": "http://localhost:8081/nexus/content/groups/maven-public",
			"group": {
				"memberRepositoryIds": ["maven-releases", "maven-snapshots", "maven-central"]
			}
		}
	}`

	var detail nexus2RepoDetail
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if detail.Data.Group == nil {
		t.Fatal("expected group config, got nil")
	}
	if len(detail.Data.Group.MemberIDs) != 3 {
		t.Fatalf("expected 3 members, got %d", len(detail.Data.Group.MemberIDs))
	}
	expectedMembers := []string{"maven-releases", "maven-snapshots", "maven-central"}
	for i, member := range detail.Data.Group.MemberIDs {
		if member != expectedMembers[i] {
			t.Errorf("expected member %q at index %d, got %q", expectedMembers[i], i, member)
		}
	}
}

func TestNexus2UserDeserialization(t *testing.T) {
	body := `{
		"data": {
			"userId": "admin",
			"firstName": "Administrator",
			"lastName": "User",
			"email": "admin@example.com",
			"status": "active",
			"roles": ["nx-admin"],
			"external": false,
			"externalId": ""
		}
	}`

	var user nexus2User
	if err := json.Unmarshal([]byte(body), &user); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if user.Data.UserID != "admin" {
		t.Errorf("expected userId 'admin', got %q", user.Data.UserID)
	}
	if user.Data.Email != "admin@example.com" {
		t.Errorf("expected email 'admin@example.com', got %q", user.Data.Email)
	}
	if user.Data.External {
		t.Error("admin should not be external")
	}
}

func TestNexus2RoleDeserialization(t *testing.T) {
	body := `{
		"data": {
			"id": "nx-admin",
			"name": "Administrator",
			"description": "Full system administrator",
			"privileges": ["nx-all"],
			"roles": []
		}
	}`

	var role nexus2Role
	if err := json.Unmarshal([]byte(body), &role); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if role.Data.ID != "nx-admin" {
		t.Errorf("expected id 'nx-admin', got %q", role.Data.ID)
	}
	if role.Data.Name != "Administrator" {
		t.Errorf("expected name 'Administrator', got %q", role.Data.Name)
	}
	if len(role.Data.Privileges) != 1 || role.Data.Privileges[0] != "nx-all" {
		t.Errorf("expected privileges [nx-all], got %v", role.Data.Privileges)
	}
}

func TestNexus2PrivilegeDeserialization(t *testing.T) {
	body := `{
		"data": {
			"id": "nx-repository-view-maven2-maven-releases-read",
			"name": "nx-repository-view-maven2-maven-releases-read",
			"description": "Read access to maven-releases",
			"type": "repository-view",
			"actions": "read",
			"repository": "maven-releases",
			"format": "maven2"
		}
	}`

	var priv nexus2Privilege
	if err := json.Unmarshal([]byte(body), &priv); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if priv.Data.ID != "nx-repository-view-maven2-maven-releases-read" {
		t.Errorf("expected id 'nx-repository-view-maven2-maven-releases-read', got %q", priv.Data.ID)
	}
	if priv.Data.Actions != "read" {
		t.Errorf("expected actions 'read', got %q", priv.Data.Actions)
	}
	if priv.Data.Repository != "maven-releases" {
		t.Errorf("expected repository 'maven-releases', got %q", priv.Data.Repository)
	}
}

func TestNexus2ListRepositoriesResponse(t *testing.T) {
	body := `{
		"data": [
			{
				"data": {
					"name": "maven-releases",
					"format": "maven2",
					"type": "hosted",
					"url": "http://localhost:8081/nexus/content/repositories/maven-releases"
				}
			},
			{
				"data": {
					"name": "maven-central",
					"format": "maven2",
					"type": "proxy",
					"url": "http://localhost:8081/nexus/content/repositories/maven-central"
				}
			}
		]
	}`

	var result struct {
		Data []nexus2Repository `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.Data) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(result.Data))
	}

	if result.Data[0].Data.Name != "maven-releases" {
		t.Errorf("expected first repo name 'maven-releases', got %q", result.Data[0].Data.Name)
	}
	if result.Data[1].Data.Name != "maven-central" {
		t.Errorf("expected second repo name 'maven-central', got %q", result.Data[1].Data.Name)
	}
}

func TestNexusVersionString(t *testing.T) {
	v := NexusVersion{3, 30, 2}
	expected := "3.30.2"
	if got := v.String(); got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}

func TestNexusVersionComparisonEdgeCases(t *testing.T) {
	v := NexusVersion{0, 0, 0}
	if v.IsNexus2() {
		t.Error("version 0.0.0 should not be considered Nexus 2")
	}
	if v.IsNexus3() {
		t.Error("version 0.0.0 should not be considered Nexus 3")
	}
}

func TestNexus2ToSourceRepositoryConversion(t *testing.T) {
	repo := nexus2Repository{
		Data: struct {
			Name   string `json:"name"`
			Format string `json:"format"`
			Type   string `json:"type"`
			URL    string `json:"url"`
		}{
			Name:   "npm-proxy",
			Format: "npm",
			Type:   "proxy",
			URL:    "http://localhost:8081/nexus/content/repositories/npm-proxy",
		},
	}

	sourceRepo := source.SourceRepository{
		Name:   repo.Data.Name,
		Format: normalizeNexus2Format(repo.Data.Format),
		Type:   normalizeNexus2Type(repo.Data.Type),
		URL:    repo.Data.URL,
	}

	if sourceRepo.Name != "npm-proxy" {
		t.Errorf("expected name 'npm-proxy', got %q", sourceRepo.Name)
	}
	if sourceRepo.Format != "npm" {
		t.Errorf("expected format 'npm', got %q", sourceRepo.Format)
	}
	if sourceRepo.Type != "proxy" {
		t.Errorf("expected type 'proxy', got %q", sourceRepo.Type)
	}
}