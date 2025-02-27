/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/carlmjohnson/requests"
)

type Project struct {
	ID        string `json:"id"`
	CliAPIKey string `json:"apiKey"`
}

type AdditionalAPIKey struct {
	APIKey string `json:"apiKey"`
}

type Webhook struct {
	Url string `json:"url"`
}

// Client manages communication with the plan engine API
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Orchestration represents an orchestration state
type Orchestration struct {
	ID        string            `json:"id"`
	Results   []json.RawMessage `json:"results"`
	Error     json.RawMessage   `json:"error,omitempty"`
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Webhook   string            `json:"webhook"`
}

type OrchestrationRequest struct {
	Action struct {
		Content string
	} `json:"action"`
	Data                   []map[string]string `json:"data"`
	Webhook                string              `json:"webhook"`
	Timeout                string              `json:"timeout,omitempty"`
	HealthCheckGracePeriod string              `json:"healthCheckGracePeriod,omitempty"`
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
	ID           string               `json:"id"`
	Action       string               `json:"action"`
	Status       Status               `json:"status"`
	Error        json.RawMessage      `json:"error,omitempty"`
	Timestamp    time.Time            `json:"timestamp"`
	Compensation *CompensationSummary `json:"compensation,omitempty"`
}

type CompensationSummary struct {
	Active    bool `json:"active"`    // Are any compensations still running?
	Total     int  `json:"total"`     // Total number of compensate-able tasks
	Completed int  `json:"completed"` // Number of completed compensations
	Failed    int  `json:"failed"`    // Number of failed compensations
}

func (cs *CompensationSummary) String() string {
	if cs == nil {
		return ""
	}

	remaining := cs.Total - cs.Completed - cs.Failed

	if cs.Active && remaining > 0 {
		return fmt.Sprintf("Active (%d/%d)", cs.Completed+cs.Failed, cs.Total)
	}

	if cs.Failed > 0 {
		return fmt.Sprintf("Failed (%d/%d)", cs.Failed, cs.Total)
	}

	if cs.Completed == cs.Total {
		return fmt.Sprintf("Completed (%d/%d)", cs.Total, cs.Total)
	}

	return ""
}

type OrchestrationListView struct {
	Pending       []OrchestrationView `json:"pending,omitempty"`
	Processing    []OrchestrationView `json:"processing,omitempty"`
	Completed     []OrchestrationView `json:"completed,omitempty"`
	Failed        []OrchestrationView `json:"failed,omitempty"`
	NotActionable []OrchestrationView `json:"notActionable,omitempty"`
}

// OrchestrationInspectResponse represents the detailed inspection view of an orchestration
type OrchestrationInspectResponse struct {
	ID        string                `json:"id"`
	Status    Status                `json:"status"`
	Action    string                `json:"action"`
	Timestamp time.Time             `json:"timestamp"`
	Error     json.RawMessage       `json:"error,omitempty"`
	Tasks     []TaskInspectResponse `json:"tasks,omitempty"`
	Results   []json.RawMessage     `json:"results,omitempty"`
	Duration  time.Duration         `json:"duration"`
}

// TaskInspectResponse represents the detailed view of a task within an orchestration
type TaskInspectResponse struct {
	ID                  string                    `json:"id"`
	ServiceID           string                    `json:"serviceId"`
	ServiceName         string                    `json:"serviceName"`
	Status              Status                    `json:"status"`
	StatusHistory       []TaskStatusEvent         `json:"statusHistory"`
	Input               json.RawMessage           `json:"input,omitempty"`
	Output              json.RawMessage           `json:"output,omitempty"`
	Error               string                    `json:"error,omitempty"`
	Duration            time.Duration             `json:"duration"`
	Compensation        *TaskCompensationStatus   `json:"compensation,omitempty"`
	CompensationHistory []CompensationStatusEvent `json:"compensationHistory,omitempty"`
	IsRevertible        bool                      `json:"isRevertible"`
}

type TaskCompensationStatus struct {
	Status      string    `json:"status"`       // pending, processing, completed, failed, partial, expired
	Attempt     int       `json:"attempt"`      // Current attempt number (1-based)
	MaxAttempts int       `json:"max_attempts"` // Maximum attempts allowed
	Timestamp   time.Time `json:"timestamp"`    // When compensation started
}

type CompensationStatusEvent struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"taskId"`
	Status      string    `json:"status"`      // "processing", "completed", "failed"
	Attempt     int       `json:"attempt"`     // Current attempt number (1-based)
	MaxAttempts int       `json:"maxAttempts"` // Maximum allowed attempts
	Timestamp   time.Time `json:"timestamp"`
	Error       string    `json:"error,omitempty"`
}

func (tcs *TaskCompensationStatus) String() string {
	if tcs == nil {
		return ""
	}

	switch tcs.Status {
	case "pending":
		return "Pending"
	case "processing":
		return fmt.Sprintf("Processing (%d/%d)", tcs.Attempt, tcs.MaxAttempts)
	case "completed":
		return "Completed"
	case "partial":
		return "Completed Partially"
	case "expired":
		return "Expired"
	case "failed":
		return fmt.Sprintf("Failed (%d/%d)", tcs.Attempt, tcs.MaxAttempts)
	default:
		return ""
	}
}

func (cse CompensationStatusEvent) String() string {
	switch cse.Status {
	case "pending":
		return "Pending"
	case "processing":
		return fmt.Sprintf("Processing (%d/%d)", cse.Attempt, cse.MaxAttempts)
	case "completed":
		return "Completed"
	case "partial":
		return "Completed Partially"
	case "expired":
		return "Expired"
	case "failed":
		return fmt.Sprintf("Failed (%d/%d)", cse.Attempt, cse.MaxAttempts)
	default:
		return ""
	}
}

// TaskStatusEvent represents a status change in a task's history
type TaskStatusEvent struct {
	ID              string    `json:"id"`
	OrchestrationID string    `json:"orchestrationId"`
	TaskID          string    `json:"taskId"`
	Status          Status    `json:"status"`
	Timestamp       time.Time `json:"timestamp"`
	ServiceID       string    `json:"serviceId,omitempty"`
	Error           string    `json:"error,omitempty"`
}

type GroundingUseCase struct {
	Action       string            `json:"action" yaml:"action"`
	Params       map[string]string `json:"params" yaml:"params"`
	Capabilities []string          `json:"capabilities" yaml:"capabilities"`
	Intent       string            `json:"intent" yaml:"intent"`
}

type GroundingSpec struct {
	Name        string             `json:"name" yaml:"name"`
	Domain      string             `json:"domain" yaml:"domain"`
	Version     string             `json:"version" yaml:"version"`
	UseCases    []GroundingUseCase `json:"useCases" yaml:"use-cases"`
	Constraints []string           `json:"constraints" yaml:"constraints"`
}

type NotFoundError struct {
	Err error
}

func (e NotFoundError) Error() string {
	return e.Err.Error()
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

func (c *Client) AddProject(ctx context.Context, name string) (*Project, error) {
	var project Project
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Path("/register/project").
		Method(http.MethodPost).
		Client(c.httpClient).
		BodyJSON(map[string]any{
			"name":      name,
			"createdAt": time.Now().UTC(),
		}).
		ToJSON(&project).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return nil, FormatAPIError(apiErr, "project")
	}

	return &project, nil
}

func (c *Client) GenerateAdditionalApiKey(ctx context.Context) (*AdditionalAPIKey, error) {
	var response AdditionalAPIKey
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Path("/apikeys").
		Method(http.MethodPost).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return nil, FormatAPIError(apiErr, "ApiKey")
	}

	return &response, nil
}

func (c *Client) AddWebhook(ctx context.Context, webhookUrl string) (*Webhook, error) {
	var response Webhook
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Path("/webhooks").
		Method(http.MethodPost).
		Client(c.httpClient).
		BodyJSON(Webhook{Url: webhookUrl}).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return nil, FormatAPIError(apiErr, "webhook")
	}

	return &response, nil
}

// ListOrchestrations retrieves all orchestrations for a project
func (c *Client) ListOrchestrations(ctx context.Context) (*OrchestrationListView, error) {
	var response OrchestrationListView
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Path("/orchestrations").
		Method(http.MethodGet).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return nil, FormatAPIError(apiErr, "orchestrations list")
	}

	return &response, nil
}

func (c *Client) GetOrchestrationInspection(ctx context.Context, id string) (*OrchestrationInspectResponse, error) {
	var inspection OrchestrationInspectResponse
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Pathf("/orchestrations/inspections/%s", id).
		Method(http.MethodGet).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&inspection).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return nil, FormatAPIError(apiErr, "orchestration")
	}

	return &inspection, err
}

func (c *Client) CreateOrchestration(ctx context.Context, or OrchestrationRequest) (*Orchestration, error) {
	var response *Orchestration
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Path("/orchestrations").
		Method(http.MethodPost).
		Client(c.httpClient).
		BodyJSON(or).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return nil, FormatAPIError(apiErr, "orchestration action: "+or.Action.Content)
	}

	return response, nil
}

func (c *Client) ApplyGroundingSpec(ctx context.Context, spec GroundingSpec) (*GroundingSpec, error) {
	var response *GroundingSpec
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Path("/groundings").
		Method(http.MethodPost).
		Client(c.httpClient).
		BodyJSON(spec).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return nil, FormatAPIError(apiErr, "grounding spec")
	}

	return response, err
}

func (c *Client) ListGroundingSpecs(ctx context.Context) ([]GroundingSpec, error) {
	var response []GroundingSpec
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Path("/groundings").
		Method(http.MethodGet).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return nil, FormatAPIError(apiErr, "grounding specs")
	}

	return response, err
}

func (c *Client) RemoveAllGroundingSpecs(ctx context.Context) error {
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Path("/groundings").
		Method(http.MethodDelete).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return FormatAPIError(apiErr, "grounding specs")
	}

	return err
}

func (c *Client) RemoveGroundingSpec(ctx context.Context, specName string) error {
	var apiErr ErrorResponse

	err := requests.
		URL(c.baseURL).
		Path(fmt.Sprintf("/groundings/%s", specName)).
		Method(http.MethodDelete).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		ErrorJSON(&apiErr).
		Fetch(ctx)

	if err != nil {
		return FormatAPIError(apiErr, "grounding")
	}

	return err
}

func (v OrchestrationListView) Empty() bool {
	return len(v.Pending) == 0 &&
		len(v.Processing) == 0 &&
		len(v.Completed) == 0 &&
		len(v.Failed) == 0 &&
		len(v.NotActionable) == 0
}
