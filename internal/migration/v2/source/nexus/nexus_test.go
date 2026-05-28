package nexus

import (
	"encoding/json"
	"testing"
)

func TestNexusUserV1Deserialization(t *testing.T) {
	// Simulate Nexus 3.30.2 /service/rest/v1/security/users response
	body := `[
		{
			"userId": "admin",
			"firstName": "Administrator",
			"lastName": "User",
			"emailAddress": "admin@example.com",
			"source": "default",
			"status": "active",
			"readOnly": true,
			"roles": ["nx-admin"],
			"externalRoles": []
		},
		{
			"userId": "ldap-user",
			"firstName": "LDAP",
			"lastName": "Person",
			"emailAddress": "ldap@example.com",
			"source": "LDAP",
			"status": "active",
			"readOnly": false,
			"roles": ["nx-anonymous"],
			"externalRoles": ["ldap-role-1"]
		}
	]`

	var nexusUsers []nexusUserV1
	if err := json.Unmarshal([]byte(body), &nexusUsers); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(nexusUsers) != 2 {
		t.Fatalf("expected 2 users, got %d", len(nexusUsers))
	}

	// Check admin user
	admin := nexusUsers[0].toSourceUser()
	if admin.UserID != "admin" {
		t.Errorf("expected userId 'admin', got %q", admin.UserID)
	}
	if admin.FirstName != "Administrator" {
		t.Errorf("expected firstName 'Administrator', got %q", admin.FirstName)
	}
	if admin.LastName != "User" {
		t.Errorf("expected lastName 'User', got %q", admin.LastName)
	}
	if admin.Email != "admin@example.com" {
		t.Errorf("expected email 'admin@example.com', got %q", admin.Email)
	}
	if admin.External {
		t.Error("admin should not be external")
	}
	if len(admin.Roles) != 1 || admin.Roles[0] != "nx-admin" {
		t.Errorf("expected roles [nx-admin], got %v", admin.Roles)
	}

	// Check LDAP user
	ldap := nexusUsers[1].toSourceUser()
	if ldap.UserID != "ldap-user" {
		t.Errorf("expected userId 'ldap-user', got %q", ldap.UserID)
	}
	if ldap.Email != "ldap@example.com" {
		t.Errorf("expected email 'ldap@example.com', got %q", ldap.Email)
	}
	if !ldap.External {
		t.Error("LDAP user should be external")
	}
	if ldap.ExternalID != "LDAP" {
		t.Errorf("expected externalID 'LDAP', got %q", ldap.ExternalID)
	}
}

func TestNexusRoleV1Deserialization(t *testing.T) {
	body := `[
		{
			"id": "nx-admin",
			"name": "nx-admin",
			"description": "Administrator",
			"privileges": ["nx-all"],
			"roles": [],
			"readOnly": true,
			"source": "default"
		}
	]`

	var nexusRoles []nexusRoleV1
	if err := json.Unmarshal([]byte(body), &nexusRoles); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(nexusRoles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(nexusRoles))
	}

	role := nexusRoles[0].toSourceRole()
	if role.ID != "nx-admin" {
		t.Errorf("expected id 'nx-admin', got %q", role.ID)
	}
	if role.Name != "nx-admin" {
		t.Errorf("expected name 'nx-admin', got %q", role.Name)
	}
	if role.Description != "Administrator" {
		t.Errorf("expected description 'Administrator', got %q", role.Description)
	}
	if len(role.Privileges) != 1 || role.Privileges[0] != "nx-all" {
		t.Errorf("expected privileges [nx-all], got %v", role.Privileges)
	}
	if role.External {
		t.Error("default source role should not be external")
	}
}

func TestNexusUserV1SourceDetection(t *testing.T) {
	tests := []struct {
		source   string
		external bool
	}{
		{"default", false},
		{"LDAP", true},
		{"SAML", true},
		{"Crowd", true},
	}
	for _, tt := range tests {
		u := nexusUserV1{Source: tt.source}
		su := u.toSourceUser()
		if su.External != tt.external {
			t.Errorf("source=%q: expected external=%v, got %v", tt.source, tt.external, su.External)
		}
	}
}

func TestNexusRepoDetailV1ProxyDeserialization(t *testing.T) {
	// Simulate Nexus 3.30.2 proxy repository detail response
	body := `{
		"name": "maven-central",
		"format": "maven2",
		"type": "proxy",
		"url": "http://localhost:8081/repository/maven-central",
		"online": true,
		"storage": {
			"blobStoreName": "default",
			"strictContentTypeValidation": true
		},
		"proxy": {
			"remoteUrl": "https://repo1.maven.org/maven2/",
			"contentMaxAge": 1440,
			"metadataMaxAge": 1440
		}
	}`

	var detail nexusRepoDetailV1
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	sd := detail.toSourceDetail()
	if sd.Name != "maven-central" {
		t.Errorf("expected name 'maven-central', got %q", sd.Name)
	}
	if sd.Format != "maven2" {
		t.Errorf("expected format 'maven2', got %q", sd.Format)
	}
	if sd.Type != "proxy" {
		t.Errorf("expected type 'proxy', got %q", sd.Type)
	}
	if sd.Proxy == nil {
		t.Fatal("expected proxy config, got nil")
	}
	if sd.Proxy.RemoteURL != "https://repo1.maven.org/maven2/" {
		t.Errorf("expected remoteUrl 'https://repo1.maven.org/maven2/', got %q", sd.Proxy.RemoteURL)
	}
	if sd.Storage == nil {
		t.Fatal("expected storage config, got nil")
	}
	if sd.Storage.BlobStoreName != "default" {
		t.Errorf("expected blobStoreName 'default', got %q", sd.Storage.BlobStoreName)
	}
}

func TestNexusRepoDetailV1GroupDeserialization(t *testing.T) {
	body := `{
		"name": "maven-public",
		"format": "maven2",
		"type": "group",
		"url": "http://localhost:8081/repository/maven-public",
		"online": true,
		"storage": {
			"blobStoreName": "default"
		},
		"group": {
			"memberNames": ["maven-releases", "maven-central"]
		}
	}`

	var detail nexusRepoDetailV1
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	sd := detail.toSourceDetail()
	if sd.Group == nil {
		t.Fatal("expected group config, got nil")
	}
	if len(sd.Group.MemberNames) != 2 {
		t.Fatalf("expected 2 members, got %d", len(sd.Group.MemberNames))
	}
	if sd.Group.MemberNames[0] != "maven-releases" || sd.Group.MemberNames[1] != "maven-central" {
		t.Errorf("unexpected members: %v", sd.Group.MemberNames)
	}
}
