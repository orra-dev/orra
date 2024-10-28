package main

import (
	"container/list"
	"context"
	"encoding/json"
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

type WebSocketQueuedMessage struct {
	Message json.RawMessage
	Time    time.Time
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
	ID      string `json:"id"`
	APIKey  string `json:"apiKey"`
	Webhook string `json:"webhook"`
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

type TaskWorker struct {
	Service         *ServiceInfo
	TaskID          string
	Dependencies    DependencyKeys
	LogManager      *LogManager
	logState        *LogState
	backOff         *backoff.ExponentialBackOff
	pauseStart      time.Time // Track pause duration
	consecutiveErrs int       // Track consecutive failures
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
	Type       string     `json:"type"`
	Properties Properties `json:"properties,omitempty"`
	Required   []string   `json:"required,omitempty"`
	Format     string     `json:"format,omitempty"`
	Minimum    int        `json:"minimum,omitempty"`
	Maximum    int        `json:"maximum,omitempty"`
}

type Properties map[string]Spec

type ServiceSchema struct {
	Input  Spec `json:"input"`
	Output Spec `json:"output"`
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
	ID        string              `json:"id"`
	ProjectID string              `json:"-"`
	Action    Action              `json:"action"`
	Params    ActionParams        `json:"data"`
	Plan      *ServiceCallingPlan `json:"plan"`
	Results   []json.RawMessage   `json:"results"`
	Status    Status              `json:"status"`
	Error     json.RawMessage     `json:"error,omitempty"`
	Timestamp time.Time           `json:"timestamp"`
	taskZero  json.RawMessage
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
