package nexus

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
	"github.com/sirupsen/logrus"
)

// NexusSource implements source.MigrationSource for Nexus Repository Manager.
type NexusSource struct {
	baseURL  string
	username string
	password string
	client   *http.Client

	// scriptOnce ensures Groovy helper scripts are registered only once.
	scriptOnce  sync.Once
	scriptReady bool
	scriptErr   error
}

func New(baseURL, username, password string) *NexusSource {
	return &NexusSource{
		baseURL:  baseURL,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

func (s *NexusSource) TestConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/service/rest/v1/status", nil)
	if err != nil {
		return err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connection failed with status: %d", resp.StatusCode)
	}
	return nil
}

func (s *NexusSource) ListRepositories(ctx context.Context) ([]source.SourceRepository, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/service/rest/v1/repositories", nil)
	if err != nil {
		return nil, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list repositories: %d", resp.StatusCode)
	}
	var repos []source.SourceRepository
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// GetRepositoryDetail fetches repository detail.
// It first tries the v1 detail endpoint (available in Nexus 3.15+).
// If that returns 404 (Nexus 3.12 and earlier), it falls back to the Groovy Script API.
func (s *NexusSource) GetRepositoryDetail(ctx context.Context, format, repoType, name string) (*source.SourceRepositoryDetail, error) {
	// Try v1 detail endpoint first (Nexus 3.15+)
	detail, err := s.getRepoDetailV1(ctx, format, repoType, name)
	if err == nil {
		// For group repos, also fetch member repositories
		if repoType == "group" {
			members, merr := s.listGroupMembers(ctx, format, name)
			if merr != nil {
				logrus.WithFields(logrus.Fields{
					"format":     format,
					"group_name": name,
					"error":      merr,
				}).Warn("Failed to fetch group members, continuing without membership data")
			} else {
				detail.Group = &source.SourceGroupConfig{MemberNames: members}
			}
		}
		return detail, nil
	}

	// If v1 endpoint returned 404, fall back to Groovy Script API for older Nexus versions
	if strings.Contains(err.Error(), "404") {
		logrus.WithFields(logrus.Fields{
			"format": format, "type": repoType, "name": name,
		}).Info("v1 repository detail endpoint not available, falling back to Groovy Script API")
		return s.getRepoDetailViaScript(ctx, format, repoType, name)
	}

	return nil, err
}

// getRepoDetailV1 calls the v1 repository detail endpoint (Nexus 3.15+).
func (s *NexusSource) getRepoDetailV1(ctx context.Context, format, repoType, name string) (*source.SourceRepositoryDetail, error) {
	url := fmt.Sprintf("%s/service/rest/v1/repositories/%s/%s/%s", s.baseURL, format, repoType, name)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get repository detail: %d - %s", resp.StatusCode, string(body))
	}
	var detail source.SourceRepositoryDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// listGroupMembers fetches member repository names for a group repository.
// It first tries the v1 endpoint, then falls back to Groovy Script API.
func (s *NexusSource) listGroupMembers(ctx context.Context, format, groupName string) ([]string, error) {
	// Try v1 endpoint first (Nexus 3.15+)
	members, err := s.listGroupMembersV1(ctx, format, groupName)
	if err == nil {
		return members, nil
	}

	// If v1 endpoint returned 404, fall back to Groovy Script API
	if strings.Contains(err.Error(), "404") {
		return s.listGroupMembersViaScript(ctx, groupName)
	}
	return nil, err
}

// listGroupMembersV1 calls the v1 group members endpoint (Nexus 3.15+).
func (s *NexusSource) listGroupMembersV1(ctx context.Context, format, groupName string) ([]string, error) {
	url := fmt.Sprintf("%s/service/rest/v1/repository/%s/group/%s/members", s.baseURL, format, groupName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list group members: %d - %s", resp.StatusCode, string(body))
	}
	var memberList []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&memberList); err != nil {
		return nil, err
	}
	var names []string
	for _, m := range memberList {
		names = append(names, m.Name)
	}
	return names, nil
}

// ---------------------------------------------------------------------------
// Groovy Script API fallback for Nexus 3.12 and earlier
// ---------------------------------------------------------------------------

const (
	scriptNameRepoDetail     = "moonlight-repo-detail"
	scriptNameGroupMembers   = "moonlight-group-members"
	scriptNameListUsers      = "moonlight-list-users"
	scriptNameListRoles      = "moonlight-list-roles"
	scriptNameListPrivileges = "moonlight-list-privileges"

	// Groovy script to get repository configuration including proxy remoteUrl and group memberNames.
	groovyRepoDetail = `
import org.sonatype.nexus.repository.Repository
import org.sonatype.nexus.repository.config.Configuration
import org.sonatype.nexus.repository.manager.RepositoryManager

def repoName = args
def repo = repository.repositoryManager.get(repoName)
if (repo == null) {
    return """{"error":"repository not found: ${repoName}"}"""
}

def config = repo.configuration
def result = [:]
result.name = config.repositoryName
result.format = config.repositoryFormat ?: (repo.format?.value() ?: "")
result.type = config.repositoryType ?: (repo.type?.value() ?: "")
result.url = repo.url ?: ""

def attrs = config.attributes ?: [:]
// Proxy config
if (attrs.proxy) {
    result.proxy = [remote_url: attrs.proxy.remoteUrl ?: ""]
}
// Storage config
if (attrs.storage) {
    result.storage = [blob_store_name: attrs.storage.blobStoreName ?: ""]
}
// Group config
if (attrs.group) {
    result.group = [member_names: attrs.group.memberNames ?: []]
}

import groovy.json.JsonBuilder
return new JsonBuilder(result).toString()
`

	// Groovy script to get group repository member names.
	groovyGroupMembers = `
import org.sonatype.nexus.repository.Repository

def groupName = args
def repo = repository.repositoryManager.get(groupName)
if (repo == null) {
    return """{"error":"repository not found: ${groupName}"}"""
}

def attrs = repo.configuration.attributes
if (attrs?.group?.memberNames) {
    import groovy.json.JsonBuilder
    return new JsonBuilder([member_names: attrs.group.memberNames]).toString()
}
return """{"member_names":[]}"""
`

	// Groovy script to list all users with their roles.
	groovyListUsers = `
import org.sonatype.nexus.security.user.UserManager
import groovy.json.JsonBuilder

def userManager = security.getUserManager()
def users = userManager.listUsers()
def result = []
for (u in users) {
    result.add([
        user_id:     u.userId ?: "",
        first_name:  u.firstName ?: "",
        last_name:   u.lastName ?: "",
        email:       u.emailAddress ?: "",
        status:      u.status?.value() ?: "active",
        roles:       u.roles?.collect { it.roleId } ?: [],
        external:    u.source != "default",
        external_id: u.source ?: ""
    ])
}
return new JsonBuilder([users: result]).toString()
`

	// Groovy script to list all roles with their privileges.
	groovyListRoles = `
import org.sonatype.nexus.security.role.Role
import groovy.json.JsonBuilder

def roleDAO = security.getAuthorizationManager().listRoles()
def result = []
for (r in roleDAO) {
    result.add([
        id:          r.roleId ?: "",
        name:        r.name ?: "",
        description: r.description ?: "",
        privileges:  r.privileges?.collect { it } ?: [],
        roles:       r.roles?.collect { it } ?: [],
        external:    false
    ])
}
return new JsonBuilder([roles: result]).toString()
`

	// Groovy script to list all privileges.
	groovyListPrivileges = `
import groovy.json.JsonBuilder

def privs = security.getAuthorizationManager().listPrivileges()
def result = []
for (p in privs) {
    result.add([
        id:          p.id ?: "",
        name:        p.name ?: "",
        description: p.description ?: "",
        type:        p.type ?: "",
        actions:     p.property("actions") ?: "",
        repository:  p.property("repository") ?: "",
        format:      p.property("format") ?: ""
    ])
}
return new JsonBuilder([privileges: result]).toString()
`
)

// ensureScripts registers the Groovy helper scripts if not already done.
func (s *NexusSource) ensureScripts(ctx context.Context) error {
	s.scriptOnce.Do(func() {
		scripts := map[string]string{
			scriptNameRepoDetail:     groovyRepoDetail,
			scriptNameGroupMembers:   groovyGroupMembers,
			scriptNameListUsers:      groovyListUsers,
			scriptNameListRoles:      groovyListRoles,
			scriptNameListPrivileges: groovyListPrivileges,
		}
		for name, content := range scripts {
			if err := s.registerScript(ctx, name, content); err != nil {
				// If script already exists, that's fine
				if !strings.Contains(err.Error(), "409") && !strings.Contains(err.Error(), "already exists") {
					s.scriptErr = fmt.Errorf("failed to register script %s: %w", name, err)
					return
				}
			}
		}
		s.scriptReady = true
	})
	return s.scriptErr
}

// registerScript creates a Groovy script on the Nexus server.
func (s *NexusSource) registerScript(ctx context.Context, name, content string) error {
	payload, _ := json.Marshal(map[string]string{
		"name":    name,
		"type":    "groovy",
		"content": content,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/service/rest/v1/script", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	s.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register script failed: %d - %s", resp.StatusCode, string(body))
	}
	return nil
}

// runScript executes a registered Groovy script with the given arguments.
func (s *NexusSource) runScript(ctx context.Context, name, args string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"args": args})
	url := fmt.Sprintf("%s/service/rest/v1/script/%s/run", s.baseURL, name)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	s.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("run script %s failed: %d - %s", name, resp.StatusCode, string(body))
	}
	var result struct {
		Name   string `json:"name"`
		Result string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode script result: %w", err)
	}
	return result.Result, nil
}

// getRepoDetailViaScript uses the Groovy Script API to get repository detail.
func (s *NexusSource) getRepoDetailViaScript(ctx context.Context, format, repoType, name string) (*source.SourceRepositoryDetail, error) {
	if err := s.ensureScripts(ctx); err != nil {
		return nil, fmt.Errorf("script setup failed: %w", err)
	}

	result, err := s.runScript(ctx, scriptNameRepoDetail, name)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Name   string `json:"name"`
		Format string `json:"format"`
		Type   string `json:"type"`
		URL    string `json:"url"`
		Proxy  *struct {
			RemoteURL string `json:"remote_url"`
		} `json:"proxy"`
		Storage *struct {
			BlobStoreName string `json:"blob_store_name"`
		} `json:"storage"`
		Group *struct {
			MemberNames []string `json:"member_names"`
		} `json:"group"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse script result: %w", err)
	}
	if raw.Error != "" {
		return nil, fmt.Errorf("script error: %s", raw.Error)
	}

	detail := &source.SourceRepositoryDetail{
		Name:   raw.Name,
		Format: raw.Format,
		Type:   raw.Type,
		URL:    raw.URL,
	}
	if raw.Proxy != nil {
		detail.Proxy = &source.SourceProxyConfig{RemoteURL: raw.Proxy.RemoteURL}
	}
	if raw.Storage != nil {
		detail.Storage = &source.SourceStorageConfig{BlobStoreName: raw.Storage.BlobStoreName}
	}
	if raw.Group != nil {
		detail.Group = &source.SourceGroupConfig{MemberNames: raw.Group.MemberNames}
	}

	return detail, nil
}

// listGroupMembersViaScript uses the Groovy Script API to get group member names.
func (s *NexusSource) listGroupMembersViaScript(ctx context.Context, groupName string) ([]string, error) {
	if err := s.ensureScripts(ctx); err != nil {
		return nil, fmt.Errorf("script setup failed: %w", err)
	}

	result, err := s.runScript(ctx, scriptNameGroupMembers, groupName)
	if err != nil {
		return nil, err
	}

	var raw struct {
		MemberNames []string `json:"member_names"`
		Error       string   `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse script result: %w", err)
	}
	if raw.Error != "" {
		return nil, fmt.Errorf("script error: %s", raw.Error)
	}
	return raw.MemberNames, nil
}

func (s *NexusSource) ListRoles(ctx context.Context) ([]source.SourceRole, error) {
	// Try v1 endpoint first (Nexus 3.17+)
	roles, err := s.listRolesV1(ctx)
	if err == nil {
		return roles, nil
	}
	if !strings.Contains(err.Error(), "404") {
		return nil, err
	}

	// Try beta endpoint (Nexus 3.14+)
	roles, err = s.listRolesBeta(ctx)
	if err == nil {
		return roles, nil
	}
	if !strings.Contains(err.Error(), "404") {
		return nil, err
	}

	// Fall back to Groovy Script API (Nexus 3.x all versions)
	logrus.Info("v1/beta security/roles endpoints not available, falling back to Groovy Script API")
	return s.listRolesViaScript(ctx)
}

func (s *NexusSource) listRolesV1(ctx context.Context) ([]source.SourceRole, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/service/rest/v1/security/roles", nil)
	if err != nil {
		return nil, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list roles: %d - %s", resp.StatusCode, string(body))
	}
	var roles []source.SourceRole
	if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return roles, nil
}

func (s *NexusSource) listRolesBeta(ctx context.Context) ([]source.SourceRole, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/service/rest/beta/security/roles", nil)
	if err != nil {
		return nil, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list roles (beta): %d - %s", resp.StatusCode, string(body))
	}
	var roles []source.SourceRole
	if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return roles, nil
}

func (s *NexusSource) listRolesViaScript(ctx context.Context) ([]source.SourceRole, error) {
	if err := s.ensureScripts(ctx); err != nil {
		return nil, fmt.Errorf("script setup failed: %w", err)
	}
	result, err := s.runScript(ctx, scriptNameListRoles, "")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Roles []source.SourceRole `json:"roles"`
		Error string              `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse script result: %w", err)
	}
	if raw.Error != "" {
		return nil, fmt.Errorf("script error: %s", raw.Error)
	}
	return raw.Roles, nil
}

func (s *NexusSource) ListPrivileges(ctx context.Context) ([]source.SourcePrivilege, error) {
	// Try v1 endpoint first (Nexus 3.17+)
	privs, err := s.listPrivilegesV1(ctx)
	if err == nil {
		return privs, nil
	}
	if !strings.Contains(err.Error(), "404") {
		return nil, err
	}

	// Try beta endpoint (Nexus 3.14+)
	privs, err = s.listPrivilegesBeta(ctx)
	if err == nil {
		return privs, nil
	}
	if !strings.Contains(err.Error(), "404") {
		return nil, err
	}

	// Fall back to Groovy Script API (Nexus 3.x all versions)
	logrus.Info("v1/beta security/privileges endpoints not available, falling back to Groovy Script API")
	return s.listPrivilegesViaScript(ctx)
}

func (s *NexusSource) listPrivilegesV1(ctx context.Context) ([]source.SourcePrivilege, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/service/rest/v1/security/privileges", nil)
	if err != nil {
		return nil, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list privileges: %d - %s", resp.StatusCode, string(body))
	}
	var privileges []source.SourcePrivilege
	if err := json.NewDecoder(resp.Body).Decode(&privileges); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return privileges, nil
}

func (s *NexusSource) listPrivilegesBeta(ctx context.Context) ([]source.SourcePrivilege, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/service/rest/beta/security/privileges", nil)
	if err != nil {
		return nil, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list privileges (beta): %d - %s", resp.StatusCode, string(body))
	}
	var privileges []source.SourcePrivilege
	if err := json.NewDecoder(resp.Body).Decode(&privileges); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return privileges, nil
}

func (s *NexusSource) listPrivilegesViaScript(ctx context.Context) ([]source.SourcePrivilege, error) {
	if err := s.ensureScripts(ctx); err != nil {
		return nil, fmt.Errorf("script setup failed: %w", err)
	}
	result, err := s.runScript(ctx, scriptNameListPrivileges, "")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Privileges []source.SourcePrivilege `json:"privileges"`
		Error      string                   `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse script result: %w", err)
	}
	if raw.Error != "" {
		return nil, fmt.Errorf("script error: %s", raw.Error)
	}
	return raw.Privileges, nil
}

func (s *NexusSource) ListUsers(ctx context.Context) ([]source.SourceUser, error) {
	// Try v1 endpoint first (Nexus 3.24+)
	users, err := s.listUsersV1(ctx)
	if err == nil {
		return users, nil
	}
	if !strings.Contains(err.Error(), "404") {
		return nil, err
	}

	// Try beta endpoint (Nexus 3.17+)
	users, err = s.listUsersBeta(ctx)
	if err == nil {
		return users, nil
	}
	if !strings.Contains(err.Error(), "404") {
		return nil, err
	}

	// Fall back to Groovy Script API (Nexus 3.x all versions)
	logrus.Info("v1/beta security/users endpoints not available, falling back to Groovy Script API")
	return s.listUsersViaScript(ctx)
}

func (s *NexusSource) listUsersV1(ctx context.Context) ([]source.SourceUser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/service/rest/v1/security/users", nil)
	if err != nil {
		return nil, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list users: %d - %s", resp.StatusCode, string(body))
	}
	var users []source.SourceUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return users, nil
}

func (s *NexusSource) listUsersBeta(ctx context.Context) ([]source.SourceUser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/service/rest/beta/security/users", nil)
	if err != nil {
		return nil, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list users (beta): %d - %s", resp.StatusCode, string(body))
	}
	var users []source.SourceUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return users, nil
}

func (s *NexusSource) listUsersViaScript(ctx context.Context) ([]source.SourceUser, error) {
	if err := s.ensureScripts(ctx); err != nil {
		return nil, fmt.Errorf("script setup failed: %w", err)
	}
	result, err := s.runScript(ctx, scriptNameListUsers, "")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Users []source.SourceUser `json:"users"`
		Error string              `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse script result: %w", err)
	}
	if raw.Error != "" {
		return nil, fmt.Errorf("script error: %s", raw.Error)
	}
	return raw.Users, nil
}

func (s *NexusSource) ListComponentsPage(ctx context.Context, repoName, continuationToken string) (source.SourceComponentPage, error) {
	url := fmt.Sprintf("%s/service/rest/v1/components?repository=%s", s.baseURL, repoName)
	if continuationToken != "" {
		url += "&continuationToken=" + continuationToken
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return source.SourceComponentPage{}, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return source.SourceComponentPage{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logrus.WithFields(logrus.Fields{"url": url, "status": resp.StatusCode, "body": string(body)}).Error("Nexus API error")
		return source.SourceComponentPage{}, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}
	var page struct {
		Items             []source.SourceComponent `json:"items"`
		ContinuationToken *string                  `json:"continuationToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return source.SourceComponentPage{}, fmt.Errorf("failed to decode response: %w", err)
	}
	result := source.SourceComponentPage{Items: page.Items}
	if page.ContinuationToken != nil {
		result.ContinuationToken = *page.ContinuationToken
	}
	return result, nil
}

func (s *NexusSource) DownloadAsset(ctx context.Context, assetURL string) (source.AssetStream, error) {
	if !strings.HasPrefix(assetURL, "http://") && !strings.HasPrefix(assetURL, "https://") {
		assetURL = strings.TrimRight(s.baseURL, "/") + "/" + strings.TrimLeft(assetURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", assetURL, nil)
	if err != nil {
		return source.AssetStream{}, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return source.AssetStream{}, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return source.AssetStream{}, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}
	return source.AssetStream{
		Reader:      resp.Body,
		ContentType: resp.Header.Get("Content-Type"),
		Size:        resp.ContentLength,
	}, nil
}

func (s *NexusSource) setAuth(req *http.Request) {
	req.SetBasicAuth(s.username, s.password)
}

// MapRepositoryType converts Nexus-native type to the target-system type.
func MapRepositoryType(nexusType string) string {
	switch nexusType {
	case "proxy":
		return "proxy"
	case "hosted":
		return "local"
	case "group":
		return "virtual"
	default:
		return "proxy"
	}
}
