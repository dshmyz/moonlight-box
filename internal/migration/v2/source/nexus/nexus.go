package nexus

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// GetRepositoryDetail uses the format/type/name path to avoid 405 errors.
func (s *NexusSource) GetRepositoryDetail(ctx context.Context, format, repoType, name string) (*source.SourceRepositoryDetail, error) {
	// Nexus REST API: /service/rest/v1/repositories/{format}/{type}/{name}
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

	// For group repos, also fetch member repositories
	if repoType == "group" {
		members, err := s.listGroupMembers(ctx, format, name)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"format":     format,
				"group_name": name,
				"error":      err,
			}).Warn("Failed to fetch group members, continuing without membership data")
		} else {
			detail.Group = &source.SourceGroupConfig{MemberNames: members}
		}
	}

	return &detail, nil
}

// listGroupMembers fetches member repository names for a group repository.
func (s *NexusSource) listGroupMembers(ctx context.Context, format, groupName string) ([]string, error) {
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
	var members []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
		return nil, err
	}
	var names []string
	for _, m := range members {
		names = append(names, m.Name)
	}
	return names, nil
}

func (s *NexusSource) ListRoles(ctx context.Context) ([]source.SourceRole, error) {
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

func (s *NexusSource) ListPrivileges(ctx context.Context) ([]source.SourcePrivilege, error) {
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

func (s *NexusSource) ListUsers(ctx context.Context) ([]source.SourceUser, error) {
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
