package nexus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
	"github.com/sirupsen/logrus"
)

type NexusVersion struct {
	Major int
	Minor int
	Patch int
}

func (v NexusVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v NexusVersion) IsNexus3() bool {
	return v.Major == 3
}

func (v NexusVersion) IsNexus2() bool {
	return v.Major == 2
}

func (v NexusVersion) GreaterThanOrEqual(minor, patch int) bool {
	if v.Minor > minor {
		return true
	}
	if v.Minor == minor && v.Patch >= patch {
		return true
	}
	return false
}

func (s *NexusSource) DetectVersion(ctx context.Context) (NexusVersion, error) {
	candidates := []string{
		s.baseURL + "/service/rest/v1/status",
		s.baseURL + "/service/local/status",
		s.baseURL + "/status",
	}

	for _, url := range candidates {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			continue
		}
		s.setAuth(req)
		resp, err := s.client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			version := parseVersionFromResponse(string(body))
			if version.Major > 0 {
				return version, nil
			}
		}
	}

	return NexusVersion{}, fmt.Errorf("unable to detect Nexus version")
}

func parseVersionFromResponse(body string) NexusVersion {
	var result NexusVersion

	var v1Status struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(body), &v1Status); err == nil && v1Status.Version != "" {
		parseVersionString(v1Status.Version, &result)
		return result
	}

	var localStatus struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &localStatus); err == nil && localStatus.Data.Version != "" {
		parseVersionString(localStatus.Data.Version, &result)
		return result
	}

	if strings.Contains(body, "nexus") && strings.Contains(body, "version") {
		parts := strings.Split(body, "\n")
		for _, part := range parts {
			if strings.Contains(strings.ToLower(part), "version") {
				versionStr := strings.TrimSpace(strings.SplitN(part, ":", 2)[1])
				parseVersionString(versionStr, &result)
				if result.Major > 0 {
					return result
				}
			}
		}
	}

	return result
}

func parseVersionString(versionStr string, result *NexusVersion) {
	fmt.Sscanf(versionStr, "%d.%d.%d", &result.Major, &result.Minor, &result.Patch)
}

type nexus2Repository struct {
	Data struct {
		Name   string `json:"name"`
		Format string `json:"format"`
		Type   string `json:"type"`
		URL    string `json:"url"`
	} `json:"data"`
}

func (s *NexusSource) listRepositoriesNexus2(ctx context.Context) ([]source.SourceRepository, error) {
	url := s.baseURL + "/service/local/repositories"
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
		return nil, fmt.Errorf("failed to list repositories: %d", resp.StatusCode)
	}

	var result struct {
		Data []nexus2Repository `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var repos []source.SourceRepository
	for _, repo := range result.Data {
		repos = append(repos, source.SourceRepository{
			Name:   repo.Data.Name,
			Format: normalizeNexus2Format(repo.Data.Format),
			Type:   normalizeNexus2Type(repo.Data.Type),
			URL:    repo.Data.URL,
		})
	}
	return repos, nil
}

func normalizeNexus2Format(format string) string {
	format = strings.ToLower(format)
	switch format {
	case "maven2", "maven":
		return "maven2"
	case "npm":
		return "npm"
	case "pypi":
		return "pypi"
	case "raw":
		return "raw"
	default:
		return format
	}
}

func normalizeNexus2Type(repoType string) string {
	repoType = strings.ToLower(repoType)
	switch repoType {
	case "proxy":
		return "proxy"
	case "hosted":
		return "hosted"
	case "group":
		return "group"
	default:
		return repoType
	}
}

type nexus2RepoDetail struct {
	Data struct {
		Name   string `json:"name"`
		Format string `json:"format"`
		Type   string `json:"type"`
		URL    string `json:"url"`
		Proxy  *struct {
			RemoteURL string `json:"remoteStorageUrl"`
		} `json:"proxy"`
		Group *struct {
			MemberIDs []string `json:"memberRepositoryIds"`
		} `json:"group"`
	} `json:"data"`
}

func (s *NexusSource) getRepoDetailNexus2(ctx context.Context, format, repoType, name string) (*source.SourceRepositoryDetail, error) {
	url := fmt.Sprintf("%s/service/local/repositories/%s", s.baseURL, name)
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
		return nil, fmt.Errorf("failed to get repo detail: %d", resp.StatusCode)
	}

	var detail nexus2RepoDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}

	result := &source.SourceRepositoryDetail{
		Name:   detail.Data.Name,
		Format: normalizeNexus2Format(detail.Data.Format),
		Type:   normalizeNexus2Type(detail.Data.Type),
		URL:    detail.Data.URL,
	}

	if detail.Data.Proxy != nil && detail.Data.Proxy.RemoteURL != "" {
		result.Proxy = &source.SourceProxyConfig{
			RemoteURL: detail.Data.Proxy.RemoteURL,
		}
	}

	if detail.Data.Group != nil && len(detail.Data.Group.MemberIDs) > 0 {
		result.Group = &source.SourceGroupConfig{
			MemberNames: detail.Data.Group.MemberIDs,
		}
	}

	return result, nil
}

type nexus2User struct {
	Data struct {
		UserID     string   `json:"userId"`
		FirstName  string   `json:"firstName"`
		LastName   string   `json:"lastName"`
		Email      string   `json:"email"`
		Status     string   `json:"status"`
		Roles      []string `json:"roles"`
		External   bool     `json:"external"`
		ExternalID string   `json:"externalId"`
	} `json:"data"`
}

func (s *NexusSource) listUsersNexus2(ctx context.Context) ([]source.SourceUser, error) {
	url := s.baseURL + "/service/local/users"
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
		return nil, fmt.Errorf("failed to list users: %d", resp.StatusCode)
	}

	var result struct {
		Data []nexus2User `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var users []source.SourceUser
	for _, u := range result.Data {
		users = append(users, source.SourceUser{
			UserID:     u.Data.UserID,
			FirstName:  u.Data.FirstName,
			LastName:   u.Data.LastName,
			Email:      u.Data.Email,
			Status:     u.Data.Status,
			Roles:      u.Data.Roles,
			External:   u.Data.External,
			ExternalID: u.Data.ExternalID,
		})
	}
	return users, nil
}

type nexus2Role struct {
	Data struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Privileges  []string `json:"privileges"`
		Roles       []string `json:"roles"`
	} `json:"data"`
}

func (s *NexusSource) listRolesNexus2(ctx context.Context) ([]source.SourceRole, error) {
	url := s.baseURL + "/service/local/roles"
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
		return nil, fmt.Errorf("failed to list roles: %d", resp.StatusCode)
	}

	var result struct {
		Data []nexus2Role `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var roles []source.SourceRole
	for _, r := range result.Data {
		roles = append(roles, source.SourceRole{
			ID:          r.Data.ID,
			Name:        r.Data.Name,
			Description: r.Data.Description,
			Privileges:  r.Data.Privileges,
			Roles:       r.Data.Roles,
			External:    false,
		})
	}
	return roles, nil
}

type nexus2Privilege struct {
	Data struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Actions     string `json:"actions"`
		Repository  string `json:"repository"`
		Format      string `json:"format"`
	} `json:"data"`
}

func (s *NexusSource) listPrivilegesNexus2(ctx context.Context) ([]source.SourcePrivilege, error) {
	url := s.baseURL + "/service/local/privileges"
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
		return nil, fmt.Errorf("failed to list privileges: %d", resp.StatusCode)
	}

	var result struct {
		Data []nexus2Privilege `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var privileges []source.SourcePrivilege
	for _, p := range result.Data {
		privileges = append(privileges, source.SourcePrivilege{
			ID:          p.Data.ID,
			Name:        p.Data.Name,
			Description: p.Data.Description,
			Type:        p.Data.Type,
			Actions:     p.Data.Actions,
			Repository:  p.Data.Repository,
			Format:      p.Data.Format,
		})
	}
	return privileges, nil
}

func (s *NexusSource) listComponentsPageNexus2(ctx context.Context, repoName, continuationToken string) (source.SourceComponentPage, error) {
	url := fmt.Sprintf("%s/service/local/repositories/%s/content", s.baseURL, repoName)
	if continuationToken != "" {
		url += "?continuationToken=" + continuationToken
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return source.SourceComponentPage{}, err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return source.SourceComponentPage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return source.SourceComponentPage{}, fmt.Errorf("failed to list components: %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID         string `json:"id"`
			Repository string `json:"repository"`
			Format     string `json:"format"`
			Group      string `json:"group"`
			Name       string `json:"name"`
			Version    string `json:"version"`
			Assets     []struct {
				DownloadURL string            `json:"downloadUrl"`
				Path        string            `json:"path"`
				Checksum    map[string]string `json:"checksum"`
				ContentType string            `json:"contentType"`
				FileSize    int64             `json:"fileSize"`
			} `json:"assets"`
		} `json:"data"`
		ContinuationToken string `json:"continuationToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return source.SourceComponentPage{}, err
	}

	var items []source.SourceComponent
	for _, item := range result.Data {
		var assets []source.SourceAsset
		for _, a := range item.Assets {
			assets = append(assets, source.SourceAsset{
				DownloadURL: a.DownloadURL,
				Path:        a.Path,
				Checksum:    a.Checksum,
				ContentType: a.ContentType,
				FileSize:    a.FileSize,
			})
		}
		items = append(items, source.SourceComponent{
			ID:         item.ID,
			Repository: item.Repository,
			Format:     item.Format,
			Group:      item.Group,
			Name:       item.Name,
			Version:    item.Version,
			Assets:     assets,
		})
	}

	return source.SourceComponentPage{
		Items:             items,
		ContinuationToken: result.ContinuationToken,
	}, nil
}

func (s *NexusSource) testConnectionNexus2(ctx context.Context) error {
	url := s.baseURL + "/service/local/status"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

func (s *NexusSource) ensureNexus2Compatibility(ctx context.Context) (NexusVersion, error) {
	version, err := s.DetectVersion(ctx)
	if err != nil {
		return version, err
	}

	logrus.WithFields(logrus.Fields{
		"version": version.String(),
		"baseURL": s.baseURL,
	}).Info("Detected Nexus version")

	return version, nil
}

func (s *NexusSource) getNexus2BlobStores(ctx context.Context) ([]string, error) {
	url := s.baseURL + "/service/local/blobstores"
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
		return nil, fmt.Errorf("failed to list blobstores: %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var names []string
	for _, store := range result.Data {
		names = append(names, store.Name)
	}
	return names, nil
}

func (s *NexusSource) exportNexus2Config(ctx context.Context, exportPath string) error {
	url := fmt.Sprintf("%s/service/local/export/%s", s.baseURL, exportPath)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("export failed: %d", resp.StatusCode)
	}

	return nil
}

func (s *NexusSource) importNexus2Config(ctx context.Context, importPath string, content []byte) error {
	url := fmt.Sprintf("%s/service/local/import/%s", s.baseURL, importPath)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(content))
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
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("import failed: %d", resp.StatusCode)
	}

	return nil
}