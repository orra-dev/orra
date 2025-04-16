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
	StartJsonMarker               = "```json"
	EndJsonMarker                 = "```"
)

// Supported LLM and Embeddings models
const (
	O1MiniModel         = "o1-mini"
	O3MiniModel         = "o3-mini"
	DeepseekR1Model     = "deepseek-r1"
	QwQ32BModel         = "qwq-32b"
	TextEmbedding3Small = "text-embedding-3-small"
	JinaEmbeddingsSmall = "jina-embeddings-v2-small-en"
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
	Version                        = "0.2.4"
	LogsRetentionPeriod            = 7 * 24 * time.Hour
	DependencyPattern              = regexp.MustCompile(`^\$([^.]+)\.`)
	WSWriteTimeOut                 = time.Second * 120
	WSMaxMessageBytes        int64 = 10 * 1024 // 10K
	SupportedLLMModels             = []string{O1MiniModel, O3MiniModel, DeepseekR1Model, QwQ32BModel}
	SupportedEmbeddingModels       = []string{TextEmbedding3Small, JinaEmbeddingsSmall}
	ValidSuffixPatterns            = []*regexp.Regexp{
		regexp.MustCompile(`^-mlx$`),          // Hardware-specific optimization
		regexp.MustCompile(`^-q\d+$`),         // Quantization (e.g., -q4, -q8)
		regexp.MustCompile(`^-\d+[kK]$`),      // Context window size (e.g., -8k)
		regexp.MustCompile(`^-v\d+(\.\d+)*$`), // Version numbers (e.g., -v1, -v2.5)
		regexp.MustCompile(`^-[a-z]+\d*$`),    // Simple alphanumeric suffixes (e.g., -beta, -cuda)
	}
)

type LLMConfig struct {
	Model      string `envconfig:"default=o1-mini"`
	ApiKey     string `envconfig:"optional"`
	ApiBaseURL string `envconfig:"default=https://api.openai.com/v1"`
}

type EmbeddingsConfig struct {
	Model      string `envconfig:"default=text-embedding-3-small"`
	ApiKey     string `envconfig:"optional"`
	ApiBaseURL string `envconfig:"default=https://api.openai.com/v1"`
}

type Config struct {
	Port                  int `envconfig:"default=8005"`
	LLM                   LLMConfig
	Embeddings            EmbeddingsConfig
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
	if err := validateLLMConfig(cfg.LLM); err != nil {
		return Config{}, err
	}
	if err := validateEmbeddingsConfig(cfg.Embeddings); err != nil {
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

func getStoragePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, DefaultConfigDir, DBStoreDir), nil
}

func normalizeModelName(model string) string {
	// Handle path-like format (e.g., "qwen/qwq-32b")
	if parts := strings.Split(model, "/"); len(parts) > 1 {
		return strings.ToLower(strings.TrimSpace(parts[len(parts)-1])) // Get the last part after any slashes
	}
	return strings.ToLower(strings.TrimSpace(model))
}

func isSupportedModel(model string, allowedBaseModels []string) bool {
	// First normalize the model name (handles path-like formats)
	normalizedModel := normalizeModelName(model)

	if slices.Contains(allowedBaseModels, normalizedModel) {
		return true
	}

	// Check if it starts with a valid base model and has a valid suffix
	for _, baseModel := range allowedBaseModels {
		if !strings.HasPrefix(normalizedModel, baseModel+"-") {
			continue
		}

		suffix := normalizedModel[len(baseModel):]
		if suffix == "" {
			return false
		}

		// Check if suffix matches any of our valid patterns
		for _, pattern := range ValidSuffixPatterns {
			if pattern.MatchString(suffix) {
				return true
			}
		}
	}

	return false
}

func validateLLMConfig(llm LLMConfig) error {
	if !isSupportedModel(llm.Model, SupportedLLMModels) {
		return fmt.Errorf(
			"LLM model [%s] not supported, select one of %v or a supported variant",
			llm.Model,
			SupportedLLMModels)
	}

	if llm.ApiBaseURL == "" {
		return fmt.Errorf("LLM API base URL is required")
	}

	return nil
}

func validateEmbeddingsConfig(embeddings EmbeddingsConfig) error {
	if !isSupportedModel(embeddings.Model, SupportedEmbeddingModels) {
		return fmt.Errorf(
			"embeddings model [%s] not supported, select one of [%+v] or a variant with a suffix",
			embeddings.Model,
			SupportedEmbeddingModels)
	}

	if embeddings.ApiBaseURL == "" {
		return fmt.Errorf("embeddings API base URL is required")
	}

	return nil
}

type Status int

const (
	Registered Status = iota
	Preparing
	Pending
	Processing
	Completed
	Failed
	FailedNotRetryable
	NotActionable
	Paused
	Cancelled
	Continue
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
	case FailedNotRetryable:
		return "failed_not_retryable"
	case NotActionable:
		return "not_actionable"
	case Paused:
		return "paused"
	case Cancelled:
		return "cancelled"
	case Continue:
		return "continue"
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
	case "failed_not_retryable":
		*s = Failed
	case "not_actionable":
		*s = NotActionable
	case "paused":
		*s = Paused
	case "cancelled":
		*s = Cancelled
	case "continue":
		*s = Continue
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
		return fmt.Errorf("invalid PddlValidationErrorType: %s", s)
	}
	return nil
}
