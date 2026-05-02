package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type NexusClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

type NexusRepository struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Type   string `json:"type"`
	URL    string `json:"url"`
}

type NexusComponent struct {
	ID         string       `json:"id"`
	Repository string       `json:"repository"`
	Format     string       `json:"format"`
	Group      string       `json:"group"`
	Name       string       `json:"name"`
	Version    string       `json:"version"`
	Assets     []NexusAsset `json:"assets"`
}

type NexusAsset struct {
	DownloadURL string            `json:"downloadUrl"`
	Path        string            `json:"path"`
	Checksum    map[string]string `json:"checksum"`
	ContentType string            `json:"contentType"`
	FileSize    int64             `json:"fileSize"`
}

type NexusComponentPage struct {
	Items             []NexusComponent `json:"items"`
	ContinuationToken *string          `json:"continuationToken"`
}

func NewNexusClient(baseURL, username, password string) *NexusClient {
	return &NexusClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *NexusClient) TestConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/service/rest/v1/status", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connection failed with status: %d", resp.StatusCode)
	}
	return nil
}

func (c *NexusClient) ListRepositories(ctx context.Context) ([]NexusRepository, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/service/rest/v1/repositories", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list repositories: %d", resp.StatusCode)
	}

	var repos []NexusRepository
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func (c *NexusClient) ListComponents(ctx context.Context, repoName string) ([]NexusComponent, error) {
	var allComponents []NexusComponent
	token := ""

	for {
		url := fmt.Sprintf("%s/service/rest/v1/components?repository=%s", c.baseURL, repoName)
		if token != "" {
			url += "&continuationToken=" + token
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(c.username, c.password)

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}

		var page NexusComponentPage
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		allComponents = append(allComponents, page.Items...)

		if page.ContinuationToken == nil || *page.ContinuationToken == "" {
			break
		}
		token = *page.ContinuationToken
	}

	return allComponents, nil
}

func (c *NexusClient) DownloadAsset(ctx context.Context, assetURL string) (io.ReadCloser, string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", assetURL, nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", 0, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	contentLength := resp.ContentLength

	return resp.Body, contentType, contentLength, nil
}
