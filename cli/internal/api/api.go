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
	"io"
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

// Client manages communication with the control plane API
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
	ID        string          `json:"id"`
	Action    string          `json:"action"`
	Status    Status          `json:"status"`
	Error     json.RawMessage `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
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
	ID            string            `json:"id"`
	ServiceID     string            `json:"serviceId"`
	ServiceName   string            `json:"serviceName"`
	Status        Status            `json:"status"`
	StatusHistory []TaskStatusEvent `json:"statusHistory"`
	Input         json.RawMessage   `json:"input,omitempty"`
	Output        json.RawMessage   `json:"output,omitempty"`
	Error         string            `json:"error,omitempty"`
	Duration      time.Duration     `json:"duration"`
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

func (c *Client) CreateProject(ctx context.Context) (*Project, error) {
	var project Project

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

func (c *Client) GenerateAdditionalApiKey(ctx context.Context) (*AdditionalAPIKey, error) {
	var response AdditionalAPIKey

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

func (c *Client) AddWebhook(ctx context.Context, webhookUrl string) (*Webhook, error) {
	var response Webhook

	err := requests.
		URL(c.baseURL).
		Path("/webhooks").
		Method(http.MethodPost).
		Client(c.httpClient).
		BodyJSON(Webhook{Url: webhookUrl}).
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

func (c *Client) GetOrchestrationInspection(ctx context.Context, id string) (*OrchestrationInspectResponse, error) {
	var inspection OrchestrationInspectResponse

	err := requests.
		URL(c.baseURL).
		Pathf("/orchestrations/inspections/%s", id).
		Method(http.MethodGet).
		Client(c.httpClient).
		Header("Authorization", "Bearer "+c.apiKey).
		AddValidator(nil).
		Handle(requests.ChainHandlers(
			ResponseHandlerWithExcludedCodes(http.StatusNotFound),
			requests.ToJSON(&inspection),
		)).
		Fetch(ctx)

	if requests.HasStatusErr(err, http.StatusNotFound) {
		return nil, NotFoundError{Err: fmt.Errorf("orchestration not found: %s", id)}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get orchestration inspection: %w", err)
	}

	return &inspection, nil
}

func (c *Client) CreateOrchestration(ctx context.Context, or OrchestrationRequest) (*Orchestration, error) {
	var response *Orchestration

	err := requests.
		URL(c.baseURL).
		Path("/orchestrations").
		Method(http.MethodPost).
		Client(c.httpClient).
		BodyJSON(or).
		Header("Authorization", "Bearer "+c.apiKey).
		ToJSON(&response).
		Fetch(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create orchestration for action [%s]: %w", or.Action.Content, err)
	}

	return response, nil
}

func (v OrchestrationListView) Empty() bool {
	return len(v.Pending) == 0 &&
		len(v.Processing) == 0 &&
		len(v.Completed) == 0 &&
		len(v.Failed) == 0 &&
		len(v.NotActionable) == 0
}

func ResponseHandlerWithExcludedCodes(excludedCodes ...int) requests.ResponseHandler {
	return func(resp *http.Response) error {
		return forErr(requests.DefaultValidator, resp, excludedCodes...)
	}
}

func forErr(validator requests.ResponseHandler, resp *http.Response, excludedCodes ...int) error {
	if err := validator(resp); err != nil {
		resData, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("error reading body: %w", readErr)
		}
		if requests.HasStatusErr(err, excludedCodes...) {
			return err
		}
		return fmt.Errorf("unanticipated error for respons %s: %w", string(resData), err)
	}
	return nil
}
