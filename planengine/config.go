/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/vrischmann/envconfig"
)

const (
	DefaultConfigDir              = ".orra"
	DBStoreDir                    = "dbstore"
	TaskZero                      = "task0"
	ResultAggregatorID            = "result_aggregator"
	FailureTrackerID              = "failure_tracker"
	CompensationWorkerID          = "compensation_worker"
	WSPing                        = "ping"
	WSPong                        = "pong"
	HealthCheckGracePeriod        = 30 * time.Minute
	TaskTimeout                   = 30 * time.Second
	GroundingThreshold            = 0.90
	CompensationDataStoredLogType = "compensation_stored"
	CompensationAttemptedLogType  = "compensation_attempted"
	CompensationCompleteLogType   = "compensation_complete"
	CompensationPartialLogType    = "compensation_partial"
	CompensationFailureLogType    = "compensation_failure"
	CompensationExpiredLogType    = "compensation_expired"
	VersionHeader                 = "X-Orra-PlaneEngine-Version"
	PauseExecutionCode            = "PAUSE_EXECUTION"
	LLMOpenAIProvider             = "openai"
	LLMGroqProvider               = "groq"
	O1MiniReasoningModel          = "o1-mini"
	O3MiniReasoningModel          = "o3-mini"
	R1ReasoningModel              = "deepseek-r1-distill-llama-70b"
)

const (
	JSONMarshalingFailErrCode           = "Orra:JSONMarshalingFail"
	ProjectRegistrationFailedErrCode    = "Orra:ProjectRegistrationFailed"
	ProjectAPIKeyAdditionFailedErrCode  = "Orra:ProjectAPIKeyAdditionFailed"
	ProjectWebhookAdditionFailedErrCode = "Orra:ProjectWebhookAdditionFailed"
	UnknownOrchestrationErrCode         = "Orra:UnknownOrchestration"
	ActionNotActionableErrCode          = "Orra:ActionNotActionable"
	ActionCannotExecuteErrCode          = "Orra:ActionCannotExecute"
	PlanEngineShuttingDownErrCode       = "Orra:PlanEngineShuttingDown"
)

var (
	Version                          = "0.2.3"
	LogsRetentionPeriod              = 7 * 24 * time.Hour
	DependencyPattern                = regexp.MustCompile(`^\$([^.]+)\.`)
	WSWriteTimeOut                   = time.Second * 120
	WSMaxMessageBytes          int64 = 10 * 1024 // 10K
	AcceptedReasoningProviders       = []string{LLMOpenAIProvider, LLMGroqProvider}
	AcceptedReasoningModels          = []string{O1MiniReasoningModel, O3MiniReasoningModel, R1ReasoningModel}
)

type Reasoning struct {
	Provider string `envconfig:"default=openai"`
	Model    string `envconfig:"default=o1-mini"`
	ApiKey   string
}

type PlanCache struct {
	OpenaiApiKey string
}

type Config struct {
	Port                  int `envconfig:"default=8005"`
	Reasoning             Reasoning
	PlanCache             PlanCache
	PddlValidatorPath     string        `envconfig:"default=/usr/local/bin/Validate"`
	PddlValidationTimeout time.Duration `envconfig:"default=30s"`
	StoragePath           string        `envconfig:"optional"`
}

func Load() (Config, error) {
	var cfg Config
	err := envconfig.Init(&cfg)
	if err != nil {
		return Config{}, err
	}
	if err := validateReasoningConfig(cfg.Reasoning); err != nil {
		return Config{}, err
	}
	if cfg.StoragePath != "" {
		return cfg, nil
	}
	path, err := getStoragePath()
	if err != nil {
		return Config{}, err
	}
	cfg.StoragePath = path
	return cfg, err
}

func validateReasoningConfig(reasoning Reasoning) error {
	if !slices.Contains(AcceptedReasoningProviders, reasoning.Provider) {
		return fmt.Errorf(
			"invalid reasoning provider [%s], select one of [%+v]",
			reasoning.Provider,
			AcceptedReasoningProviders)
	}
	if !slices.Contains(AcceptedReasoningModels, reasoning.Model) {
		return fmt.Errorf(
			"invalid reasoning model [%s], select one of [%+v]",
			reasoning.Model,
			AcceptedReasoningModels)
	}
	if reasoning.ApiKey == "" {
		return fmt.Errorf("reasoning api key is required")
	}
	return nil
}

func getStoragePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, DefaultConfigDir, DBStoreDir), nil
}

type Status int

const (
	Registered Status = iota
	Preparing
	Pending
	Processing
	Completed
	Failed
	NotActionable
	Paused
	Cancelled
)

func (s Status) String() string {
	switch s {
	case Registered:
		return "registered"
	case Preparing:
		return "preparing"
	case Pending:
		return "pending"
	case Processing:
		return "processing"
	case Completed:
		return "completed"
	case Failed:
		return "failed"
	case NotActionable:
		return "not_actionable"
	case Paused:
		return "paused"
	case Cancelled:
		return "cancelled"
	default:
		return ""
	}
}

func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *Status) UnmarshalJSON(data []byte) error {
	var val string
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "registered":
		*s = Registered
	case "preparing":
		*s = Preparing
	case "pending":
		*s = Pending
	case "processing":
		*s = Processing
	case "completed":
		*s = Completed
	case "failed":
		*s = Failed
	case "not_actionable":
		*s = NotActionable
	case "paused":
		*s = Paused
	case "cancelled":
		*s = Cancelled
	default:
		return fmt.Errorf("invalid Status: %s", s)
	}
	return nil
}

type ServiceType int

const (
	Agent ServiceType = iota
	Service
)

func (st ServiceType) String() string {
	switch st {
	case Agent:
		return "agent"
	case Service:
		return "service"
	}
	return ""
}

func (st ServiceType) MarshalJSON() ([]byte, error) {
	return json.Marshal(st.String())
}

func (st *ServiceType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "agent":
		*st = Agent
	case "service":
		*st = Service
	default:
		return fmt.Errorf("invalid ServiceType: %s", s)
	}
	return nil
}

type CompensationStatus int

const (
	CompensationPending CompensationStatus = iota
	CompensationProcessing
	CompensationCompleted
	CompensationFailed
	CompensationPartial
	CompensationExpired
)

func (s CompensationStatus) String() string {
	switch s {
	case CompensationPending:
		return "pending"
	case CompensationProcessing:
		return "processing"
	case CompensationCompleted:
		return "completed"
	case CompensationFailed:
		return "failed"
	case CompensationPartial:
		return "partial"
	case CompensationExpired:
		return "expired"
	default:
		return "unknown"
	}
}

func (s CompensationStatus) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

func (s *CompensationStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	switch str {
	case "pending":
		*s = CompensationPending
	case "processing":
		*s = CompensationProcessing
	case "completed":
		*s = CompensationCompleted
	case "failed":
		*s = CompensationFailed
	case "partial":
		*s = CompensationPartial
	case "expired":
		*s = CompensationExpired
	default:
		return fmt.Errorf("invalid Compensation Status: %s", s)
	}
	return nil
}

type PddlValidationErrorType int

const (
	PddlSyntax PddlValidationErrorType = iota
	PddlSemantic
	PddlProcess
	PddlTimeout
)

func (s PddlValidationErrorType) String() string {
	switch s {
	case PddlSyntax:
		return "syntax"
	case PddlSemantic:
		return "semantic"
	case PddlProcess:
		return "process"
	case PddlTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

func (s PddlValidationErrorType) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

func (s *PddlValidationErrorType) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	switch str {
	case "syntax":
		*s = PddlSyntax
	case "semantic":
		*s = PddlSemantic
	case "process":
		*s = PddlProcess
	case "timeout":
		*s = PddlTimeout
	default:
		return fmt.Errorf("invalid Compensation Status: %s", s)
	}
	return nil
}
