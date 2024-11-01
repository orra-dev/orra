package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/carlmjohnson/requests"
)

// project represents a project in the control plane
type project struct {
	ID        string `json:"id"`
	CliAPIKey string `json:"apiKey"`
}

type additionalAPIKey struct {
	APIKey string `json:"apiKey"`
}

type webhook struct {
	Url string `json:"url"`
}

// Client manages communication with the control plane API
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
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

type Status string

func (s Status) String() string {
	var titled []string
	for _, part := range strings.Split(string(s), "_") {
		part = fmt.Sprintf("%s%s", strings.ToUpper(part[0:1]), part[1:])
		titled = append(titled, part)
	}
	return strings.Join(titled, " ")
}

type OrchestrationView struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	Status    Status    `json:"status"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type OrchestrationListView struct {
	Pending       []OrchestrationView `json:"pending,omitempty"`
	Processing    []OrchestrationView `json:"processing,omitempty"`
	Completed     []OrchestrationView `json:"completed,omitempty"`
	Failed        []OrchestrationView `json:"failed,omitempty"`
	NotActionable []OrchestrationView `json:"notActionable,omitempty"`
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{},
	}
}

func (c *Client) SetBaseUrl(url string) *Client {
	c.baseURL = url
	return c
}

func (c *Client) SetApiKey(key string) *Client {
	c.apiKey = key
	return c
}

func (c *Client) SetTimeout(t time.Duration) *Client {
	c.httpClient.Timeout = t
	return c
}

func (c *Client) GetTimeout() time.Duration {
	return c.httpClient.Timeout
}

func (c *Client) CreateProject(ctx context.Context) (*project, error) {
	var project project

	err := requests.
		URL(c.baseURL).
		Path("/register/project").
		Method(http.MethodPost).
		Client(c.httpClient).
		BodyJSON(map[string]any{}).
		ToJSON(&project).
		Fetch(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return &project, nil
}

func (c *Client) GenerateAdditionalApiKey(ctx context.Context) (*additionalAPIKey, error) {
	var response additionalAPIKey

	err := requests.
		URL(c.baseURL).
		Path("/apikeys").
		Method(http.MethodPost).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		Fetch(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	return &response, nil
}

func (c *Client) AddWebhook(ctx context.Context, webhookUrl string) (*webhook, error) {
	var response webhook

	err := requests.
		URL(c.baseURL).
		Path("/webhooks").
		Method(http.MethodPost).
		Client(c.httpClient).
		BodyJSON(webhook{Url: webhookUrl}).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		Fetch(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to add webhook [%s]: %w", webhookUrl, err)
	}

	return &response, nil
}

// ListOrchestrations retrieves all orchestrations for a project
func (c *Client) ListOrchestrations(ctx context.Context) (*OrchestrationListView, error) {
	var response OrchestrationListView

	err := requests.
		URL(c.baseURL).
		Path("/orchestrations").
		Method(http.MethodGet).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		Fetch(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to list orchestrations: %w", err)
	}

	return &response, nil
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

func (v OrchestrationListView) Empty() bool {
	return len(v.Pending) == 0 &&
		len(v.Processing) == 0 &&
		len(v.Completed) == 0 &&
		len(v.Failed) == 0 &&
		len(v.NotActionable) == 0
}
