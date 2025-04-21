/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	back "github.com/cenkalti/backoff/v4"
	"github.com/olahol/melody"
	"github.com/rs/zerolog"
	"golang.org/x/sync/singleflight"
	"gonum.org/v1/gonum/mat"
)

type contextKey struct{}

var apiKeyContextKey = contextKey{}

type PlanEngine struct {
	projects             map[string]*Project
	services             map[string]map[string]*ServiceInfo
	groundings           map[string]map[string]*GroundingSpec
	groundingsMu         sync.RWMutex
	servicesMu           sync.RWMutex
	orchestrationStore   map[string]*Orchestration
	orchestrationStoreMu sync.RWMutex
	LogManager           *LogManager
	logWorkers           map[string]map[string]context.CancelFunc
	workerMu             sync.RWMutex
	WebSocketManager     *WebSocketManager
	VectorCache          *VectorCache
	PddlValidator        PddlValidator
	SimilarityMatcher    SimilarityMatcher
	pStorage             ProjectStorage
	svcStorage           ServiceStorage
	orchestrationStorage OrchestrationStorage
	groundingStorage     GroundingStorage
	Logger               zerolog.Logger
}

type ServiceFinder func(serviceID string) (*ServiceInfo, error)

type WebSocketManager struct {
	melody            *melody.Melody
	logger            zerolog.Logger
	connMap           map[string]*melody.Session
	connMu            sync.RWMutex
	messageExpiration time.Duration
	pingInterval      time.Duration
	pongWait          time.Duration
	serviceHealth     map[string]bool
	healthMu          sync.RWMutex
}

// ProjectStorage defines the interface for project persistence operations
type ProjectStorage interface {
	// StoreProject persists a project and its related data atomically
	StoreProject(project *Project) error

	// LoadProject retrieves a project by its ID
	LoadProject(id string) (*Project, error)

	// LoadProjectByAPIKey retrieves a project using an API key (primary or additional)
	LoadProjectByAPIKey(apiKey string) (*Project, error)

	// ListProjects returns all projects
	ListProjects() ([]*Project, error)

	// AddProjectAPIKey adds a new API key to a project
	AddProjectAPIKey(projectID string, apiKey string) error

	// AddProjectWebhook adds a new webhook URL to a project
	AddProjectWebhook(projectID string, webhook string) error

	// AddProjectCompensationFailureWebhook adds a new compensation webhook URL to a project
	AddProjectCompensationFailureWebhook(projectID string, webhook string) error
}

type Project struct {
	ID                          string    `json:"id"`
	Name                        string    `json:"name"`
	APIKey                      string    `json:"apiKey"`
	AdditionalAPIKeys           []string  `json:"additionalAPIKeys"`
	Webhooks                    []string  `json:"webhooks"`
	CompensationFailureWebhooks []string  `json:"compensationFailureWebhooks"`
	CreatedAt                   time.Time `json:"createdAt"`
	UpdatedAt                   time.Time `json:"updatedAt"`
}

type OrchestrationState struct {
	ID            string            `json:"id"`
	ProjectID     string            `json:"projectId"`
	Plan          *ExecutionPlan    `json:"plan"`
	TasksStatuses map[string]Status `json:"tasksStatuses"`
	Status        Status            `json:"status"`
	CreatedAt     time.Time         `json:"createdAt"`
	LastUpdated   time.Time         `json:"lastUpdated"`
	Error         string            `json:"error"`
}

type LogStore interface {
	StoreLogEntry(orchestrationID string, entry LogEntry) error
	StoreState(state *OrchestrationState) error

	LoadEntries(orchestrationID string) ([]LogEntry, error)
	ListOrchestrationStates() ([]*OrchestrationState, error)
	LoadState(orchestrationID string) (*OrchestrationState, error)
}

type LogEntry struct {
	Offset     uint64          `json:"offset"`
	EntryType  string          `json:"entryType"`
	Id         string          `json:"id"`
	Value      json.RawMessage `json:"logValue"`
	Timestamp  time.Time       `json:"timestamp"`
	ProducerID string          `json:"producerId"`
	AttemptNum int             `json:"attemptNum"`
}

type LogManager struct {
	logs           map[string]*Log
	orchestrations map[string]*OrchestrationState
	mu             sync.RWMutex
	retention      time.Duration
	cleanupTicker  *time.Ticker
	planEngine     *PlanEngine
	storage        LogStore
	Logger         zerolog.Logger
}

type Log struct {
	Entries       []LogEntry
	CurrentOffset uint64
	seenEntries   map[string]bool
	lastAccessed  time.Time // For cleanup
	storage       LogStore
	logger        zerolog.Logger
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
	ProjectID    string
	Dependencies DependencyKeySet
	LogManager   *LogManager
	logState     *LogState
}

type IncidentTracker struct {
	ProjectID  string
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
	Dependencies           TaskDependenciesWithKeys
	Timeout                time.Duration
	HealthCheckGracePeriod time.Duration
	LogManager             *LogManager
	logState               *LogState
	backOff                *back.ExponentialBackOff
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
	Type                string               `json:"type"`
	ID                  string               `json:"id"`
	Input               json.RawMessage      `json:"input"`
	CompensationContext *CompensationContext `json:"compensationContext,omitempty"`
	ExecutionID         string               `json:"executionId"`
	IdempotencyKey      IdempotencyKey       `json:"idempotencyKey"`
	ServiceID           string               `json:"serviceId"`
	OrchestrationID     string               `json:"-"`
	ProjectID           string               `json:"-"`
	Status              Status               `json:"-"`
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

type TaskResultPayload struct {
	Task         json.RawMessage   `json:"task"`
	Compensation *CompensationData `json:"compensation"`
}

type Spec struct {
	Type       string          `json:"type"`
	Properties map[string]Spec `json:"properties,omitempty"`
	Required   []string        `json:"required,omitempty"`
	Format     string          `json:"format,omitempty"`
	Minimum    int             `json:"minimum,omitempty"`
	Maximum    int             `json:"maximum,omitempty"`
	Items      *Spec           `json:"items,omitempty"`
}

type ServiceSchema struct {
	Input  Spec `json:"input"`
	Output Spec `json:"output"`
}

type ServiceStorage interface {
	// StoreService stores or updates a service and its related data atomically
	StoreService(service *ServiceInfo) error

	// LoadService retrieves a service by its ID
	//LoadService(serviceID string) (*ServiceInfo, error)

	// LoadServiceByProjectID retrieves a service by a ProjectID and its ID
	LoadServiceByProjectID(projectID, serviceID string) (*ServiceInfo, error)

	// ListProjectServices lists all services for a given project
	//ListProjectServices(projectID string) ([]*ServiceInfo, error)

	// ListServices returns all services
	ListServices() ([]*ServiceInfo, error)
}

type ServiceInfo struct {
	Type             ServiceType       `json:"type"`
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Schema           ServiceSchema     `json:"schema"`
	Revertible       bool              `json:"revertible"`
	ProjectID        string            `json:"projectID"`
	Version          int64             `json:"version"`
	IdempotencyStore *IdempotencyStore `json:"-"`
}

// OrchestrationStorage defines the interface for orchestration persistence operations
type OrchestrationStorage interface {
	// StoreOrchestration persists an orchestration and its related data
	StoreOrchestration(orchestration *Orchestration) error

	// LoadOrchestration retrieves an orchestration by its ID
	LoadOrchestration(id string) (*Orchestration, error)

	// ListProjectOrchestrations returns all orchestrations for a project
	ListProjectOrchestrations(projectID string) ([]*Orchestration, error)
}

type Orchestration struct {
	ID                     string            `json:"id"`
	ProjectID              string            `json:"projectID"`
	Action                 Action            `json:"action"`
	Params                 ActionParams      `json:"data"`
	Plan                   *ExecutionPlan    `json:"plan"`
	Results                []json.RawMessage `json:"results"`
	Status                 Status            `json:"status"`
	Error                  json.RawMessage   `json:"error,omitempty"`
	Timestamp              time.Time         `json:"timestamp"`
	Timeout                *Duration         `json:"timeout,omitempty"`
	HealthCheckGracePeriod *Duration         `json:"healthCheckGracePeriod,omitempty"`
	Webhook                string            `json:"webhook"`
	TaskZero               json.RawMessage   `json:"taskZero"`
	GroundingHit           *GroundingHit     `json:"groundingHit,omitempty"`
	AbortPayload           json.RawMessage   `json:"abortReason,omitempty"`
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
	Value any    `json:"value"`
}

// ExecutionPlan represents the execution plan for services and agents
type ExecutionPlan struct {
	ProjectID        string          `json:"-"`
	Tasks            []*SubTask      `json:"tasks"`
	ParallelGroups   []ParallelGroup `json:"parallel_groups"`
	GroundingHit     *GroundingHit   `json:"-"`
	GroundingID      string          `json:"-"`
	GroundingVersion string          `json:"-"`
}

type ParallelGroup []string

type DependencyKeySet map[string]struct{}
type TaskDependenciesWithKeys map[string][]TaskDependencyMapping

// SubTask represents a single task in the ExecutionPlan
type SubTask struct {
	ID             string         `json:"id"`
	Service        string         `json:"service"`
	Input          map[string]any `json:"input"`
	ServiceName    string         `json:"service_name,omitempty"`
	Capabilities   []string       `json:"capabilities,omitempty"`
	ExpectedInput  Spec           `json:"expected_input,omitempty"`
	ExpectedOutput Spec           `json:"expected_output,omitempty"`
}

type TaskDependencyMapping struct {
	TaskKey       string `json:"taskKey"`
	DependencyKey string `json:"dependencyKey"`
}

type TaskZeroCacheMapping struct {
	Field       string `json:"field"`                   // Field name in Task0's input
	ActionField string `json:"actionField"`             // Field name from original action params
	Value       string `json:"originalValue,omitempty"` // Original value used to discover the mapping
}

type TaskZeroCacheMappings []TaskZeroCacheMapping

type CacheEntry struct {
	ID                     string
	Response               string
	ActionVector           *mat.VecDense
	ServicesHash           string
	Task0Input             json.RawMessage
	CacheMappings          TaskZeroCacheMappings
	Timestamp              time.Time
	CachedActionWithFields string
	Grounded               bool
}

type CacheResult struct {
	Response      string
	ID            string
	Task0Input    json.RawMessage
	CacheMappings TaskZeroCacheMappings
	Grounded      bool
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
	llmClient     *LLMClient
	matcher       SimilarityMatcher
	ttl           time.Duration
	maxSize       int // Per project
	group         singleflight.Group
	logger        zerolog.Logger
}

type CacheQuery struct {
	actionWithFields string
	actionParams     ActionParams
	actionVector     *mat.VecDense
	servicesHash     string
	grounded         bool
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

// CompensationData wraps the data needed for compensation with metadata
type CompensationData struct {
	Input   json.RawMessage      `json:"data"`
	Context *CompensationContext `json:"context"`
	TTLMs   int64                `json:"ttl"`
}

type CompensationContext struct {
	OrchestrationID string          `json:"orchestrationId"`
	Reason          Status          `json:"reason"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Timestamp       time.Time       `json:"timestamp"`
}

// FailedCompensation represents the data sent to compensation failure webhooks
type FailedCompensation struct {
	ID               string                      `json:"id"`
	ProjectID        string                      `json:"projectId"`
	OrchestrationID  string                      `json:"orchestrationId"`
	TaskID           string                      `json:"taskId"`
	ServiceID        string                      `json:"serviceId"`
	ServiceName      string                      `json:"serviceName"`
	CompensationData *CompensationData           `json:"compensationData"`
	Status           CompensationStatus          `json:"status"`
	ResolutionState  CompensationResolutionState `json:"resolutionState"`
	Failure          string                      `json:"failure,omitempty"`
	Resolution       string                      `json:"resolution,omitempty"`
	AttemptsMade     int                         `json:"attemptsMade"`
	MaxAttempts      int                         `json:"maxAttempts"`
	Timestamp        time.Time                   `json:"timestamp"`
	ResolvedAt       time.Time                   `json:"resolvedAt,omitempty"`
}

type CompensationCandidate struct {
	TaskID       string
	Service      *ServiceInfo
	Compensation *CompensationData
}

type CompensationWorker struct {
	ProjectID       string
	OrchestrationID string
	LogManager      *LogManager
	Candidates      []CompensationCandidate
	backOff         *back.ExponentialBackOff
	attemptCounts   map[string]int     // track attempts per task
	cancel          context.CancelFunc // Store cancel function
}

// GroundingStorage defines the interface for persisting grounding specs
type GroundingStorage interface {
	StoreGrounding(grounding *GroundingSpec) error
	LoadGrounding(projectID, name string) (*GroundingSpec, error)
	ListProjectGroundings(projectID string) ([]*GroundingSpec, error)
	ListGroundings() ([]*GroundingSpec, error)
	RemoveGrounding(projectID, name string) error
	RemoveProjectGroundings(projectID string) error
}

// GroundingUseCase represents grounding of how an action should be handled
type GroundingUseCase struct {
	Action       string            `json:"action" yaml:"action"`
	Params       map[string]string `json:"params" yaml:"params"`
	Capabilities []string          `json:"capabilities" yaml:"capabilities"`
	Intent       string            `json:"intent" yaml:"intent"`
}

// GroundingSpec represents a collection of planning grounding for domain-specific actions
type GroundingSpec struct {
	ProjectID   string             `json:"projectID"`
	Name        string             `json:"name" yaml:"name"`
	Domain      string             `json:"domain" yaml:"domain"`
	Version     string             `json:"version" yaml:"version"`
	UseCases    []GroundingUseCase `json:"useCases" yaml:"use-cases"`
	Constraints []string           `json:"constraints" yaml:"constraints"`
}

// GroundingHit represents an orchestration's grounding match
type GroundingHit struct {
	Name        string           `json:"name" yaml:"name"`
	Domain      string           `json:"domain" yaml:"domain"`
	Version     string           `json:"version" yaml:"version"`
	UseCase     GroundingUseCase `json:"useCases" yaml:"use-cases"`
	Constraints []string         `json:"constraints" yaml:"constraints"`
}

// GetEmbeddingText returns a string suitable for embedding that captures the example's semantic meaning
func (e *GroundingUseCase) GetEmbeddingText() string {
	return fmt.Sprintf("%s %s %s",
		e.Action,
		e.Intent,
		strings.Join(e.Capabilities, " "))
}

// PddlValidator interface allows different validation implementations
type PddlValidator interface {
	Validate(context.Context, string, string, string) error
	HealthCheck(context.Context) error
}

// PddlValidationError provides structured error responses
type PddlValidationError struct {
	Type    PddlValidationErrorType // Syntax, Semantic, Process, Timeout
	Message string
	Line    int    // Line number in PDDL where error occurred
	File    string // Which file: domain or problem
}

// PddlValidationService handles PDDL validation workflow
type PddlValidationService struct {
	valPath string
	timeout time.Duration
	logger  zerolog.Logger
}
