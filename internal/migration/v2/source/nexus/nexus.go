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

	// version stores detected Nexus version
	version      NexusVersion
	versionReady bool
	versionErr   error
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
	version, err := s.getVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect Nexus version: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"version": version.String(),
		"baseURL": s.baseURL,
	}).Info("Nexus connection test successful")

	return nil
}

func (s *NexusSource) getVersion(ctx context.Context) (NexusVersion, error) {
	if s.versionReady {
		return s.version, s.versionErr
	}

	var versionOnce sync.Once
	versionOnce.Do(func() {
		s.version, s.versionErr = s.DetectVersion(ctx)
		s.versionReady = true
	})

	return s.version, s.versionErr
}

func (s *NexusSource) ListRepositories(ctx context.Context) ([]source.SourceRepository, error) {
	version, err := s.getVersion(ctx)
	if err != nil {
		return nil, err
	}

	if version.IsNexus2() {
		return s.listRepositoriesNexus2(ctx)
	}

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

	for i := range repos {
		repos[i].Type = strings.ToLower(repos[i].Type)
		repos[i].Format = strings.ToLower(repos[i].Format)
	}

	return repos, nil
}

// GetRepositoryDetail fetches repository detail.
// It first tries the v1 detail endpoint (available in Nexus 3.15+).
// If that returns 404 (Nexus 3.12 and earlier), it falls back to the Groovy Script API.
// For Nexus 2.x, it uses the /service/local/repositories/{name} endpoint.
func (s *NexusSource) GetRepositoryDetail(ctx context.Context, format, repoType, name string) (*source.SourceRepositoryDetail, error) {
	version, err := s.getVersion(ctx)
	if err != nil {
		return nil, err
	}

	if version.IsNexus2() {
		return s.getRepoDetailNexus2(ctx, format, repoType, name)
	}

	// Try v1 detail endpoint first (Nexus 3.15+)
	detail, err := s.getRepoDetailV1(ctx, format, repoType, name)
	if err == nil {
		// For group repos, ensure we have member data.
		// Source 1: group.memberNames from the detail response (camelCase mapped)
		// Source 2: dedicated /members endpoint fallback
		if repoType == "group" && (detail.Group == nil || len(detail.Group.MemberNames) == 0) {
			members, merr := s.listGroupMembers(ctx, format, name)
			if merr != nil {
				logrus.WithFields(logrus.Fields{
					"format":     format,
					"group_name": name,
					"error":      merr,
				}).Warn("Group members endpoint unavailable, detail response also has no memberNames")
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

// getRepoDetailV1 calls the v1 repository detail endpoint.
// Nexus 3 format-specific path: GET /service/rest/v1/repositories/{format}/{type}/{name}
// e.g. /service/rest/v1/repositories/maven/hosted/maven-releases
// Note: The format in path uses "maven" not "maven2"
func (s *NexusSource) getRepoDetailV1(ctx context.Context, format, repoType, name string) (*source.SourceRepositoryDetail, error) {
	pathFormat := format
	if format == "maven2" {
		pathFormat = "maven"
	}
	url := fmt.Sprintf("%s/service/rest/v1/repositories/%s/%s/%s", s.baseURL, pathFormat, repoType, name)
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
	var nexusDetail nexusRepoDetailV1
	if err := json.NewDecoder(resp.Body).Decode(&nexusDetail); err != nil {
		return nil, err
	}
	return nexusDetail.toSourceDetail(), nil
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

// listGroupMembersV1 calls the v1 group members endpoint.
// Nexus 3 format-specific path: GET /service/rest/v1/repositories/{format}/group/{groupName}/members
// e.g. /service/rest/v1/repositories/maven/group/maven-public/members
// Note: The format in path uses "maven" not "maven2"
func (s *NexusSource) listGroupMembersV1(ctx context.Context, format, groupName string) ([]string, error) {
	pathFormat := format
	if format == "maven2" {
		pathFormat = "maven"
	}
	url := fmt.Sprintf("%s/service/rest/v1/repositories/%s/group/%s/members", s.baseURL, pathFormat, groupName)
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
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read group members response: %w", err)
	}

	// 先尝试 [{"name":"..."}] 格式
	var memberList []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(bodyBytes, &memberList); err == nil && len(memberList) > 0 {
		var names []string
		for _, m := range memberList {
			if m.Name != "" {
				names = append(names, m.Name)
			}
		}
		if len(names) > 0 {
			return names, nil
		}
	}

	// 再尝试 ["repo1", "repo2"] 纯字符串数组格式
	var stringList []string
	if err := json.Unmarshal(bodyBytes, &stringList); err == nil && len(stringList) > 0 {
		return stringList, nil
	}

	logrus.WithFields(logrus.Fields{
		"url":      url,
		"response": string(bodyBytes),
	}).Warn("Group members response has unexpected format, returning empty list")
	return nil, nil
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
	version, err := s.getVersion(ctx)
	if err != nil {
		return nil, err
	}

	if version.IsNexus2() {
		return s.listRolesNexus2(ctx)
	}

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
	var nexusRoles []nexusRoleV1
	if err := json.NewDecoder(resp.Body).Decode(&nexusRoles); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	var roles []source.SourceRole
	for _, r := range nexusRoles {
		roles = append(roles, r.toSourceRole())
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
	var nexusRoles []nexusRoleV1
	if err := json.NewDecoder(resp.Body).Decode(&nexusRoles); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	var roles []source.SourceRole
	for _, r := range nexusRoles {
		roles = append(roles, r.toSourceRole())
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
	version, err := s.getVersion(ctx)
	if err != nil {
		return nil, err
	}

	if version.IsNexus2() {
		return s.listPrivilegesNexus2(ctx)
	}

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
	version, err := s.getVersion(ctx)
	if err != nil {
		return nil, err
	}

	if version.IsNexus2() {
		return s.listUsersNexus2(ctx)
	}

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
	var nexusUsers []nexusUserV1
	if err := json.NewDecoder(resp.Body).Decode(&nexusUsers); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	var users []source.SourceUser
	for _, u := range nexusUsers {
		users = append(users, u.toSourceUser())
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
	var nexusUsers []nexusUserV1
	if err := json.NewDecoder(resp.Body).Decode(&nexusUsers); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	var users []source.SourceUser
	for _, u := range nexusUsers {
		users = append(users, u.toSourceUser())
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
	version, err := s.getVersion(ctx)
	if err != nil {
		return source.SourceComponentPage{}, err
	}

	if version.IsNexus2() {
		return s.listComponentsPageNexus2(ctx, repoName, continuationToken)
	}

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

// ---------------------------------------------------------------------------
// Nexus 3 REST API JSON mapping structs
// The Nexus REST API uses camelCase field names, but source.SourceUser/SourceRole
// use snake_case JSON tags (for Groovy script compatibility). These Nexus-specific
// structs bridge the gap for v1 API deserialization.
// ---------------------------------------------------------------------------

// nexusUserV1 matches the camelCase JSON response of Nexus 3 /service/rest/v1/security/users.
type nexusUserV1 struct {
	UserID        string   `json:"userId"`
	FirstName     string   `json:"firstName"`
	LastName      string   `json:"lastName"`
	EmailAddress  string   `json:"emailAddress"`
	Source        string   `json:"source"`
	Status        string   `json:"status"`
	Roles         []string `json:"roles"`
	ExternalRoles []string `json:"externalRoles"`
	ReadOnly      bool     `json:"readOnly"`
}

func (u nexusUserV1) toSourceUser() source.SourceUser {
	return source.SourceUser{
		UserID:     u.UserID,
		FirstName:  u.FirstName,
		LastName:   u.LastName,
		Email:      u.EmailAddress,
		Status:     u.Status,
		Roles:      u.Roles,
		External:   u.Source != "default",
		ExternalID: u.Source,
	}
}

// nexusRoleV1 matches the camelCase JSON response of Nexus 3 /service/rest/v1/security/roles.
type nexusRoleV1 struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Privileges  []string `json:"privileges"`
	Roles       []string `json:"roles"`
	ReadOnly    bool     `json:"readOnly"`
	Source      string   `json:"source"`
}

func (r nexusRoleV1) toSourceRole() source.SourceRole {
	return source.SourceRole{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Privileges:  r.Privileges,
		Roles:       r.Roles,
		External:    r.Source != "default",
	}
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

// MapRepositoryFormat converts Nexus-native format name to the target-system format name.
// Nexus uses names like "maven2", "raw", "docker" etc., while the target system
// uses "maven", "generic", etc. (registered plugin names in main.go).
func MapRepositoryFormat(nexusFormat string) string {
	switch nexusFormat {
	case "maven2":
		return "maven"
	case "raw":
		return "generic"
	case "npm", "pypi", "go", "apt", "yum":
		return nexusFormat // same name
	default:
		return "generic"
	}
}

// ---------------------------------------------------------------------------
// Nexus 3 repository detail API response mapping
// The Nexus REST API uses camelCase (remoteUrl) but source.SourceProxyConfig
// uses snake_case JSON tag (remote_url). This struct bridges the gap.
// ---------------------------------------------------------------------------

// nexusRepoDetailV1 matches the camelCase JSON response of
// GET /service/rest/v1/repositories/{format}/{type}/{name}
type nexusRepoDetailV1 struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Type   string `json:"type"`
	URL    string `json:"url"`
	Online bool   `json:"online"`
	Proxy  *struct {
		RemoteURL      string `json:"remoteUrl"`
		ContentMaxAge  int    `json:"contentMaxAge"`
		MetadataMaxAge int    `json:"metadataMaxAge"`
	} `json:"proxy"`
	Storage *struct {
		BlobStoreName               string `json:"blobStoreName"`
		StrictContentTypeValidation bool   `json:"strictContentTypeValidation"`
	} `json:"storage"`
	Group *struct {
		MemberNames []string `json:"memberNames"`
	} `json:"group"`
}

func (d *nexusRepoDetailV1) toSourceDetail() *source.SourceRepositoryDetail {
	detail := &source.SourceRepositoryDetail{
		Name:   d.Name,
		Format: d.Format,
		Type:   d.Type,
		URL:    d.URL,
	}
	if d.Proxy != nil {
		detail.Proxy = &source.SourceProxyConfig{
			RemoteURL: d.Proxy.RemoteURL,
		}
	}
	if d.Storage != nil {
		detail.Storage = &source.SourceStorageConfig{
			BlobStoreName: d.Storage.BlobStoreName,
		}
	}
	if d.Group != nil {
		detail.Group = &source.SourceGroupConfig{
			MemberNames: d.Group.MemberNames,
		}
	}
	return detail
}
