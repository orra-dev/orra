/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/olahol/melody"
	"github.com/rs/zerolog"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/sync/singleflight"
	"gonum.org/v1/gonum/mat"
)

type ControlPlane struct {
	projects             map[string]*Project
	services             map[string]map[string]*ServiceInfo
	servicesMu           sync.RWMutex
	orchestrationStore   map[string]*Orchestration
	orchestrationStoreMu sync.RWMutex
	LogManager           *LogManager
	logWorkers           map[string]map[string]context.CancelFunc
	workerMu             sync.RWMutex
	WebSocketManager     *WebSocketManager
	VectorCache          *VectorCache
	openAIKey            string
	mu                   sync.RWMutex
	Logger               zerolog.Logger
}

type WebSocketMessageQueue struct {
	*list.List
	mu sync.Mutex
}

type ServiceFinder func(serviceID string) (*ServiceInfo, error)

type WebSocketManager struct {
	melody            *melody.Melody
	logger            zerolog.Logger
	connMap           map[string]*melody.Session
	connMu            sync.RWMutex
	messageQueues     map[string]*WebSocketMessageQueue
	messageQueuesMu   sync.RWMutex
	messageExpiration time.Duration
	pingInterval      time.Duration
	pongWait          time.Duration
	serviceHealth     map[string]bool
	healthMu          sync.RWMutex
}

type Project struct {
	ID                string   `json:"id"`
	APIKey            string   `json:"apiKey"`
	AdditionalAPIKeys []string `json:"additionalAPIKeys"`
	Webhooks          []string `json:"webhooks"`
}

type OrchestrationState struct {
	ID            string
	ProjectID     string
	Plan          *ServiceCallingPlan
	TasksStatuses map[string]Status
	Status        Status
	CreatedAt     time.Time
	LastUpdated   time.Time
	Error         string
}

type LogEntry struct {
	offset     uint64
	entryType  string
	id         string
	value      json.RawMessage
	timestamp  time.Time
	producerID string
	attemptNum int
}

type LogManager struct {
	logs           map[string]*Log
	orchestrations map[string]*OrchestrationState
	mu             sync.RWMutex
	retention      time.Duration
	cleanupTicker  *time.Ticker
	controlPlane   *ControlPlane
	Logger         zerolog.Logger
}

type Log struct {
	Entries       []LogEntry
	CurrentOffset uint64
	seenEntries   map[string]bool
	lastAccessed  time.Time // For cleanup
	mu            sync.RWMutex
}

type DependencyState map[string]json.RawMessage

type LogState struct {
	LastOffset      uint64
	Processed       map[string]bool
	DependencyState DependencyState
}

type LogWorker interface {
	Start(ctx context.Context, orchestrationID string)
	PollLog(ctx context.Context, orchestrationID string, logStream *Log, entriesChan chan<- LogEntry)
}

type ResultAggregator struct {
	Dependencies DependencyKeys
	LogManager   *LogManager
	logState     *LogState
}

type FailureTracker struct {
	LogManager *LogManager
	logState   *LogState
}

type LoggedFailure struct {
	Failure     string `json:"failure"`
	SkipWebhook bool   `json:"skipWebhook"`
}

type TaskWorker struct {
	Service                *ServiceInfo
	TaskID                 string
	Dependencies           DependencyKeys
	Timeout                time.Duration
	HealthCheckGracePeriod time.Duration
	LogManager             *LogManager
	logState               *LogState
	backOff                *backoff.ExponentialBackOff
	pauseStart             time.Time // Track pause duration
	consecutiveErrs        int       // Track consecutive failures
}

type TaskStatusEvent struct {
	ID              string    `json:"id"`
	OrchestrationID string    `json:"orchestrationId"`
	TaskID          string    `json:"taskId"`
	Status          Status    `json:"status"`
	Timestamp       time.Time `json:"timestamp"`
	ServiceID       string    `json:"serviceId,omitempty"`
	Error           string    `json:"error,omitempty"`
}

type Task struct {
	Type            string          `json:"type"`
	ID              string          `json:"id"`
	Input           json.RawMessage `json:"input"`
	ExecutionID     string          `json:"executionId"`
	IdempotencyKey  IdempotencyKey  `json:"idempotencyKey"`
	ServiceID       string          `json:"serviceId"`
	OrchestrationID string          `json:"-"`
	ProjectID       string          `json:"-"`
	Status          Status          `json:"-"`
}

type TaskResult struct {
	Type           string          `json:"type"`
	TaskID         string          `json:"taskId"`
	ExecutionID    string          `json:"executionId"`
	ServiceID      string          `json:"serviceId"`
	IdempotencyKey IdempotencyKey  `json:"idempotencyKey"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          string          `json:"error,omitempty"`
	Status         string          `json:"status,omitempty"`
}

type Spec struct {
	Type       string          `json:"type"`
	Properties map[string]Spec `json:"properties,omitempty"`
	Required   []string        `json:"required,omitempty"`
	Format     string          `json:"format,omitempty"`
	Minimum    int             `json:"minimum,omitempty"`
	Maximum    int             `json:"maximum,omitempty"`
}

type ServiceSchema struct {
	Input  Spec `json:"input"`
	Output Spec `json:"output"`
	Revert Spec `json:"revert,omitempty"` // Optional revert specification
}

type ServiceInfo struct {
	Type             ServiceType       `json:"type"`
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Schema           ServiceSchema     `json:"schema"`
	ProjectID        string            `json:"-"`
	Version          int64             `json:"version"`
	IdempotencyStore *IdempotencyStore `json:"-"`
}

type Orchestration struct {
	ID                     string              `json:"id"`
	ProjectID              string              `json:"-"`
	Action                 Action              `json:"action"`
	Params                 ActionParams        `json:"data"`
	Plan                   *ServiceCallingPlan `json:"plan"`
	Results                []json.RawMessage   `json:"results"`
	Status                 Status              `json:"status"`
	Error                  json.RawMessage     `json:"error,omitempty"`
	Timestamp              time.Time           `json:"timestamp"`
	Timeout                *Duration           `json:"timeout,omitempty"`
	HealthCheckGracePeriod *Duration           `json:"healthCheckGracePeriod,omitempty"`
	Webhook                string              `json:"webhook"`
	taskZero               json.RawMessage
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
	case float64:
		d.Duration = time.Duration(value)
	default:
		return fmt.Errorf("invalid duration")
	}

	return nil
}

type Action struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type ActionParams []ActionParam

type ActionParam struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// ServiceCallingPlan represents the execution plan for services and agents
type ServiceCallingPlan struct {
	ProjectID      string          `json:"-"`
	Tasks          []*SubTask      `json:"tasks"`
	ParallelGroups []ParallelGroup `json:"parallel_groups"`
}

type ParallelGroup []string

type DependencyKeys map[string]struct{}

// SubTask represents a single task in the ServiceCallingPlan
type SubTask struct {
	ID             string         `json:"id"`
	Service        string         `json:"service,omitempty"`
	ServiceDetails string         `json:"service_details,omitempty"`
	Input          map[string]any `json:"input"`
	Status         Status         `json:"status,omitempty"`
	Error          string         `json:"error,omitempty"`
}

type ParamMapping struct {
	Task0Field  string `json:"task0Field"`              // Field name in Task0's input
	ActionField string `json:"actionField"`             // Field name from original action params
	Value       string `json:"originalValue,omitempty"` // Original value used to discover the mapping
}

type CacheEntry struct {
	ID            string
	Response      *openai.ChatCompletionResponse
	ActionVector  *mat.VecDense
	ServicesHash  string
	Task0Input    json.RawMessage
	ParamMappings []ParamMapping
	Timestamp     time.Time
	Action        string
}

type CacheResult struct {
	Response      *openai.ChatCompletionResponse
	ID            string
	Task0Input    json.RawMessage
	ParamMappings []ParamMapping
	Hit           bool
}

type ProjectCache struct {
	mu        sync.RWMutex
	entries   []*CacheEntry
	threshold float64
	logger    zerolog.Logger
}

type VectorCache struct {
	mu            sync.RWMutex
	projectCaches map[string]*ProjectCache
	embedder      *openai.Client
	ttl           time.Duration
	maxSize       int // Per project
	group         singleflight.Group
	logger        zerolog.Logger
}

// CompensationResult stores the outcome of a compensation attempt
type CompensationResult struct {
	Status  CompensationStatus   `json:"status"`
	Error   string               `json:"error,omitempty"`
	Partial *PartialCompensation `json:"partial,omitempty"`
}

// PartialCompensation tracks progress of partial compensation completion
type PartialCompensation struct {
	Completed []string `json:"completed"`
	Remaining []string `json:"remaining"`
}

// CompensationState tracks the full state of a task's compensation
type CompensationState struct {
	TaskID        string              `json:"taskId"`
	Status        CompensationStatus  `json:"status"`
	AttemptCount  int                 `json:"attemptCount"`
	LastAttempted time.Time           `json:"lastAttempted"`
	TTL           time.Duration       `json:"ttl"`
	ExpiresAt     time.Time           `json:"expiresAt"`
	Result        *CompensationResult `json:"result,omitempty"`
}

// CompensationMeta contains metadata for compensation operations
type CompensationMeta struct {
	TTL       time.Duration `json:"ttl"`
	ExpiresAt time.Time     `json:"expiresAt"`
}

// CompensationData wraps the data needed for compensation with metadata
type CompensationData struct {
	Data json.RawMessage  `json:"data"`
	Meta CompensationMeta `json:"meta"`
}
