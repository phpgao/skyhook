package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/phpgao/skyhook/internal/model"
)

// Client is the Go SDK for interacting with a SkyHook server
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new SkyHook client instance
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Submit sends a batch of URLs, Magnets, or Torrent to the SkyHook server
func (c *Client) Submit(ctx context.Context, req model.TaskRequest) (*model.TaskResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/api/tasks"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: 401 Unauthorized (check your token)")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned error HTTP %d", resp.StatusCode)
	}

	var taskResp model.TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &taskResp, nil
}

// Health checks if server is reachable
func (c *Client) Health(ctx context.Context) error {
	url := c.baseURL + "/api/health"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server unhealthy: status %d", resp.StatusCode)
	}
	return nil
}

// ListTasksResponse represents the response envelope of GET /api/tasks
type ListTasksResponse struct {
	Total int                `json:"total"`
	Tasks []model.TaskRecord `json:"tasks"`
}

// ListTasks queries all tracked tasks from the SkyHook server
func (c *Client) ListTasks(ctx context.Context) ([]model.TaskRecord, error) {
	reqURL := c.baseURL + "/api/tasks"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: 401 Unauthorized (check your token)")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned error HTTP %d", resp.StatusCode)
	}

	var listResp ListTasksResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode list tasks response: %w", err)
	}

	return listResp.Tasks, nil
}

// GetTask queries a specific task by GID
func (c *Client) GetTask(ctx context.Context, gid string) (*model.TaskRecord, error) {
	reqURL := fmt.Sprintf("%s/api/tasks/%s", c.baseURL, gid)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("task %q not found", gid)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: 401 Unauthorized (check your token)")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned error HTTP %d", resp.StatusCode)
	}

	var record model.TaskRecord
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return nil, fmt.Errorf("failed to decode task response: %w", err)
	}

	return &record, nil
}

