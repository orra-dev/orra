package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/carlmjohnson/requests"
)

// Project represents a project in the control plane
type Project struct {
	ID      string `json:"id"`
	APIKey  string `json:"apiKey"`
	Webhook string `json:"webhook"`
}

// Orchestration represents an orchestration state
type Orchestration struct {
	ID        string           `json:"id"`
	Status    string           `json:"status"`
	Action    string           `json:"action"`
	Timestamp time.Time        `json:"timestamp"`
	Error     map[string]any   `json:"error,omitempty"`
	Results   []map[string]any `json:"results,omitempty"`
}

// Client manages communication with the control plane API
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{},
	}
}

func (c *Client) BaseUrl(url string) *Client {
	c.baseURL = url
	return c
}

func (c *Client) ApiKey(key string) *Client {
	c.apiKey = key
	return c
}

func (c *Client) Timeout(t time.Duration) *Client {
	c.httpClient.Timeout = t
	return c
}

func (c *Client) GetTimeout() time.Duration {
	return c.httpClient.Timeout
}

// CreateProject creates a new project in the control plane
func (c *Client) CreateProject(ctx context.Context, webhook string) (*Project, error) {
	var project Project

	err := requests.
		URL(c.baseURL).
		Path("/register/project").
		Method(http.MethodPost).
		Client(c.httpClient).
		BodyJSON(map[string]string{"webhook": webhook}).
		ToJSON(&project).
		Fetch(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return &project, nil
}

// ListOrchestrations retrieves all orchestrations for a project
func (c *Client) ListOrchestrations(ctx context.Context) ([]Orchestration, error) {
	var orchestrations []Orchestration

	err := requests.
		URL(c.baseURL).
		Path("/orchestrations").
		Method(http.MethodGet).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&orchestrations).
		Fetch(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to list orchestrations: %w", err)
	}

	return orchestrations, nil
}

// GetOrchestration retrieves a specific orchestration
func (c *Client) GetOrchestration(ctx context.Context, id string) (*Orchestration, error) {
	var orchestration Orchestration

	err := requests.
		URL(c.baseURL).
		Pathf("/orchestrations/%s", id).
		Method(http.MethodGet).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&orchestration).
		Fetch(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get orchestration: %w", err)
	}

	return &orchestration, nil
}

// GetOrchestrationLogs retrieves logs for a specific orchestration
func (c *Client) GetOrchestrationLogs(ctx context.Context, id string) ([]map[string]any, error) {
	var logs []map[string]any

	err := requests.
		URL(c.baseURL).
		Pathf("/orchestrations/%s/logs", id).
		Method(http.MethodGet).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&logs).
		Fetch(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get orchestration logs: %w", err)
	}

	return logs, nil
}
